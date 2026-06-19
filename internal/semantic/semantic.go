package semantic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

const (
	DefaultBackend  = "lancedb"
	DefaultProvider = "gemini"
	DefaultModel    = "text-embedding-004"
	FakeBackend     = "fake"
	SidecarSchema   = "pinax.kb.sidecar.v1"
)

type Chunk struct {
	ChunkID        string    `json:"chunk_id"`
	NoteID         string    `json:"note_id,omitempty"`
	VaultPath      string    `json:"vault_path"`
	Title          string    `json:"title"`
	HeadingPath    string    `json:"heading_path,omitempty"`
	ChunkText      string    `json:"chunk_text"`
	Preview        string    `json:"preview"`
	ContentHash    string    `json:"content_hash"`
	ChunkHash      string    `json:"chunk_hash"`
	TokenCount     int       `json:"token_count"`
	Tags           []string  `json:"tags,omitempty"`
	Kind           string    `json:"kind,omitempty"`
	Status         string    `json:"status,omitempty"`
	EmbeddingModel string    `json:"embedding_model"`
	EmbeddingDim   int       `json:"embedding_dim"`
	Provider       string    `json:"provider"`
	Backend        string    `json:"backend"`
	Vector         []float64 `json:"vector"`
	IndexedAt      string    `json:"indexed_at"`
}

type SearchHit struct {
	ChunkID     string   `json:"chunk_id"`
	NoteID      string   `json:"note_id,omitempty"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	HeadingPath string   `json:"heading_path,omitempty"`
	Preview     string   `json:"preview"`
	Score       float64  `json:"score"`
	Provider    string   `json:"provider,omitempty"`
	Model       string   `json:"model,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Status      string   `json:"status,omitempty"`
}

type Provider interface {
	Name() string
	Model() string
	Embed(ctx context.Context, text string) ([]float64, error)
}

type SidecarConfig struct {
	Executable string
	Timeout    time.Duration
}

type FakeProvider struct{ ModelName string }

func (p FakeProvider) Name() string { return "fake" }

func (p FakeProvider) Model() string {
	if strings.TrimSpace(p.ModelName) != "" {
		return p.ModelName
	}
	return "fake-hash-v1"
}

func (p FakeProvider) Embed(_ context.Context, text string) ([]float64, error) {
	return hashEmbedding(text, 32), nil
}

type GeminiProvider struct {
	APIKey     string
	ModelName  string
	HTTPClient *http.Client
}

func (p GeminiProvider) Name() string { return "gemini" }

func (p GeminiProvider) Model() string {
	if strings.TrimSpace(p.ModelName) != "" {
		return p.ModelName
	}
	return DefaultModel
}

func (p GeminiProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("gemini api key is required; set GEMINI_API_KEY or use --provider fake for local tests")
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	model := p.Model()
	body := map[string]any{"content": map[string]any{"parts": []map[string]string{{"text": text}}}}
	payload, _ := json.Marshal(body)
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":embedContent?key=" + p.APIKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini embedding request failed with status %d", resp.StatusCode)
	}
	var decoded struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Embedding.Values) == 0 {
		return nil, errors.New("gemini embedding response did not include values")
	}
	return decoded.Embedding.Values, nil
}

func NewProvider(name, model string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "gemini":
		return GeminiProvider{APIKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), ModelName: model}, nil
	case "fake":
		return FakeProvider{ModelName: model}, nil
	default:
		return nil, &domain.CommandError{Code: "provider_invalid", Message: "Embedding provider is not supported", Hint: "Use --provider gemini or --provider fake"}
	}
}

func BuildChunks(ctx context.Context, notes []domain.Note, provider Provider, backend string) ([]Chunk, error) {
	if provider == nil {
		var err error
		provider, err = NewProvider(DefaultProvider, "")
		if err != nil {
			return nil, err
		}
	}
	backend = normalizedBackend(backend)
	indexedAt := time.Now().UTC().Format(time.RFC3339)
	chunks := make([]Chunk, 0, len(notes))
	for _, note := range notes {
		for _, piece := range splitNote(note) {
			vector, err := provider.Embed(ctx, piece.text)
			if err != nil {
				return nil, err
			}
			chunkHash := sha(piece.text)
			chunks = append(chunks, Chunk{ChunkID: "chunk_" + chunkHash[:16], NoteID: note.ID, VaultPath: note.Path, Title: note.Title, HeadingPath: piece.heading, ChunkText: piece.text, Preview: boundedPreview(piece.text), ContentHash: sha(note.Body), ChunkHash: chunkHash, TokenCount: tokenCount(piece.text), Tags: note.Tags, Kind: note.Kind, Status: note.Status, EmbeddingModel: provider.Model(), EmbeddingDim: len(vector), Provider: provider.Name(), Backend: backend, Vector: vector, IndexedAt: indexedAt})
		}
	}
	return chunks, nil
}

func Save(ctx context.Context, root string, chunks []Chunk, backend string, sidecar SidecarConfig, documents int) (string, error) {
	backend = normalizedBackend(backend)
	switch backend {
	case FakeBackend:
		store := NewFileStore(root, backend)
		if err := store.Save(chunks); err != nil {
			return "", err
		}
		_ = writeStoreMetadata(root, backend, chunks)
		return store.Path(), nil
	case DefaultBackend:
		if err := runSidecarRebuild(ctx, root, chunks, backend, sidecar, documents); err != nil {
			return "", err
		}
		_ = writeStoreMetadata(root, backend, chunks)
		return sidecarStorePath(root, backend), nil
	default:
		return "", &domain.CommandError{Code: "backend_invalid", Message: "Semantic backend is not supported", Hint: "Use --backend lancedb or --backend fake"}
	}
}

func Doctor(ctx context.Context, root, backend string, sidecar SidecarConfig) (map[string]any, error) {
	backend = normalizedBackend(backend)
	if backend == FakeBackend {
		return map[string]any{"backend": FakeBackend, "available": true, "dependency": "built-in fake store"}, nil
	}
	if backend != DefaultBackend {
		return nil, &domain.CommandError{Code: "backend_invalid", Message: "Semantic backend is not supported", Hint: "Use --backend lancedb or --backend fake"}
	}
	return runSidecarDoctor(ctx, root, backend, sidecar)
}

func Search(ctx context.Context, root, query string, provider Provider, backend string, limit int, sidecar SidecarConfig) ([]SearchHit, int, error) {
	backend = normalizedBackend(backend)
	if backend == DefaultBackend {
		if provider == nil {
			var err error
			metaProvider, metaModel := readStoreProvider(root, backend)
			provider, err = NewProvider(metaProvider, metaModel)
			if err != nil {
				return nil, 0, err
			}
		}
		vector, err := provider.Embed(ctx, query)
		if err != nil {
			return nil, 0, err
		}
		return runSidecarSearch(ctx, root, query, vector, backend, limit, sidecar)
	}
	if backend != FakeBackend {
		return nil, 0, &domain.CommandError{Code: "backend_invalid", Message: "Semantic backend is not supported", Hint: "Use --backend lancedb or --backend fake"}
	}
	store := NewFileStore(root, backend)
	chunks, err := store.Load()
	if err != nil {
		return nil, 0, err
	}
	if provider == nil {
		provider, err = NewProvider(providerFromChunks(chunks), modelFromChunks(chunks))
		if err != nil {
			return nil, 0, err
		}
	}
	vector, err := provider.Embed(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	hits := make([]SearchHit, 0, len(chunks))
	for _, chunk := range chunks {
		score := cosine(vector, chunk.Vector)
		if strings.Contains(strings.ToLower(chunk.ChunkText), strings.ToLower(query)) {
			score += 0.2
		}
		hits = append(hits, SearchHit{ChunkID: chunk.ChunkID, NoteID: chunk.NoteID, Path: chunk.VaultPath, Title: chunk.Title, HeadingPath: chunk.HeadingPath, Preview: chunk.Preview, Score: score, Provider: chunk.Provider, Model: chunk.EmbeddingModel, Tags: chunk.Tags, Kind: chunk.Kind, Status: chunk.Status})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	total := len(hits)
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, total, nil
}

type sidecarRequest struct {
	SchemaVersion string         `json:"schema_version"`
	StoreURI      string         `json:"store_uri"`
	Backend       string         `json:"backend"`
	Documents     int            `json:"documents,omitempty"`
	Chunks        []sidecarChunk `json:"chunks,omitempty"`
	Query         string         `json:"query,omitempty"`
	QueryVector   []float64      `json:"query_vector,omitempty"`
	Limit         int            `json:"limit,omitempty"`
}

type sidecarChunk struct {
	ChunkID        string    `json:"chunk_id"`
	NoteID         string    `json:"note_id,omitempty"`
	VaultPath      string    `json:"vault_path"`
	Title          string    `json:"title"`
	HeadingPath    string    `json:"heading_path,omitempty"`
	Preview        string    `json:"preview"`
	ContentHash    string    `json:"content_hash"`
	ChunkHash      string    `json:"chunk_hash"`
	TokenCount     int       `json:"token_count"`
	Tags           []string  `json:"tags,omitempty"`
	Kind           string    `json:"kind,omitempty"`
	Status         string    `json:"status,omitempty"`
	EmbeddingModel string    `json:"embedding_model"`
	EmbeddingDim   int       `json:"embedding_dim"`
	Provider       string    `json:"provider"`
	Backend        string    `json:"backend"`
	Vector         []float64 `json:"vector"`
	IndexedAt      string    `json:"indexed_at"`
}

type sidecarResponse struct {
	SchemaVersion string          `json:"schema_version"`
	Status        string          `json:"status"`
	Backend       string          `json:"backend,omitempty"`
	Documents     int             `json:"documents,omitempty"`
	Chunks        int             `json:"chunks,omitempty"`
	Total         int             `json:"total,omitempty"`
	Hits          []SearchHit     `json:"hits,omitempty"`
	Dependency    string          `json:"dependency,omitempty"`
	Error         *sidecarErrResp `json:"error,omitempty"`
}

type sidecarErrResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func runSidecarDoctor(ctx context.Context, root, backend string, cfg SidecarConfig) (map[string]any, error) {
	resp, err := runSidecar(ctx, cfg, "doctor", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend})
	if err != nil {
		return nil, err
	}
	return map[string]any{"backend": defaultString(resp.Backend, backend), "available": resp.Status == "success", "dependency": resp.Dependency}, nil
}

func runSidecarRebuild(ctx context.Context, root string, chunks []Chunk, backend string, cfg SidecarConfig, documents int) error {
	_, err := runSidecar(ctx, cfg, "rebuild", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend, Documents: documents, Chunks: toSidecarChunks(chunks)})
	return err
}

func runSidecarSearch(ctx context.Context, root, query string, vector []float64, backend string, limit int, cfg SidecarConfig) ([]SearchHit, int, error) {
	resp, err := runSidecar(ctx, cfg, "search", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend, Query: query, QueryVector: vector, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	return resp.Hits, resp.Total, nil
}

func runSidecar(ctx context.Context, cfg SidecarConfig, op string, req sidecarRequest) (sidecarResponse, error) {
	executable := strings.TrimSpace(cfg.Executable)
	if executable == "" {
		executable = "pinax-lancedb-sidecar"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, op)
	payload, err := json.Marshal(req)
	if err != nil {
		return sidecarResponse{}, err
	}
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return sidecarResponse{}, &domain.CommandError{Code: "kb_sidecar_timeout", Message: "KB LanceDB sidecar timed out", Hint: "Increase kb.sidecar.timeout_seconds or inspect the sidecar installation"}
		}
		var execErr *exec.Error
		if errors.Is(err, exec.ErrNotFound) || errors.As(err, &execErr) || os.IsNotExist(err) {
			return sidecarResponse{}, sidecarUnavailable(executable)
		}
		return sidecarResponse{}, &domain.CommandError{Code: "kb_sidecar_failed", Message: "KB LanceDB sidecar failed", Hint: sanitizeSidecarStderr(stderr.String())}
	}
	var resp sidecarResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return sidecarResponse{}, &domain.CommandError{Code: "kb_sidecar_protocol_invalid", Message: "KB LanceDB sidecar returned invalid JSON", Hint: "Run pinax kb doctor --json to inspect the sidecar"}
	}
	if resp.SchemaVersion != SidecarSchema {
		return sidecarResponse{}, &domain.CommandError{Code: "kb_sidecar_protocol_invalid", Message: "KB LanceDB sidecar schema version is unsupported", Hint: "Upgrade pinax-lancedb-sidecar to a compatible version"}
	}
	if resp.Status == "failed" {
		code := "kb_sidecar_failed"
		message := "KB LanceDB sidecar failed"
		if resp.Error != nil {
			if strings.TrimSpace(resp.Error.Code) != "" {
				code = resp.Error.Code
			}
			if strings.TrimSpace(resp.Error.Message) != "" {
				message = resp.Error.Message
			}
		}
		return sidecarResponse{}, &domain.CommandError{Code: code, Message: message, Hint: "Run pinax kb doctor --json to inspect the sidecar"}
	}
	return resp, nil
}

func sidecarUnavailable(executable string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_sidecar_unavailable", Message: "KB LanceDB sidecar is not available", Hint: "Install the sidecar with pipx install git+https://github.com/yeisme/pinax.git#subdirectory=tools/pinax-lancedb-sidecar or set kb.sidecar.executable"}
}

func sanitizeSidecarStderr(input string) string {
	out := redaction.Cloud(strings.TrimSpace(input))
	out = strings.ReplaceAll(out, "\n", " ")
	if len(out) > 512 {
		out = out[:512] + "..."
	}
	if out == "" {
		return "Run pinax kb doctor --json to inspect the sidecar"
	}
	return out
}

func toSidecarChunks(chunks []Chunk) []sidecarChunk {
	out := make([]sidecarChunk, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, sidecarChunk{ChunkID: chunk.ChunkID, NoteID: chunk.NoteID, VaultPath: chunk.VaultPath, Title: chunk.Title, HeadingPath: chunk.HeadingPath, Preview: chunk.Preview, ContentHash: chunk.ContentHash, ChunkHash: chunk.ChunkHash, TokenCount: chunk.TokenCount, Tags: chunk.Tags, Kind: chunk.Kind, Status: chunk.Status, EmbeddingModel: chunk.EmbeddingModel, EmbeddingDim: chunk.EmbeddingDim, Provider: chunk.Provider, Backend: chunk.Backend, Vector: chunk.Vector, IndexedAt: chunk.IndexedAt})
	}
	return out
}

func sidecarStorePath(root, backend string) string {
	return filepath.Join(root, ".pinax", "kb", normalizedBackend(backend))
}

type storeMetadata struct {
	SchemaVersion string `json:"schema_version"`
	Backend       string `json:"backend"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	IndexedAt     string `json:"indexed_at"`
}

func writeStoreMetadata(root, backend string, chunks []Chunk) error {
	meta := storeMetadata{SchemaVersion: SidecarSchema, Backend: normalizedBackend(backend), Provider: DefaultProvider, Model: DefaultModel, IndexedAt: time.Now().UTC().Format(time.RFC3339)}
	if len(chunks) > 0 {
		meta.Provider = chunks[0].Provider
		meta.Model = chunks[0].EmbeddingModel
		meta.IndexedAt = chunks[0].IndexedAt
	}
	path := filepath.Join(sidecarStorePath(root, backend), "metadata.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func readStoreProvider(root, backend string) (string, string) {
	payload, err := os.ReadFile(filepath.Join(sidecarStorePath(root, backend), "metadata.json"))
	if err != nil {
		return DefaultProvider, DefaultModel
	}
	var meta storeMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return DefaultProvider, DefaultModel
	}
	return meta.Provider, meta.Model
}

type FileStore struct {
	Root    string
	Backend string
}

func NewFileStore(root, backend string) FileStore {
	return FileStore{Root: root, Backend: normalizedBackend(backend)}
}

func (s FileStore) Path() string {
	return filepath.Join(s.Root, ".pinax", "kb", s.Backend, "chunks.jsonl")
}

func (s FileStore) Save(chunks []Chunk) error {
	path := s.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	for _, chunk := range chunks {
		if err := enc.Encode(chunk); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s FileStore) Load() ([]Chunk, error) {
	path := s.Path()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &domain.CommandError{Code: "kb_index_missing", Message: "KB semantic index is missing", Hint: "Run pinax kb rebuild --vault <vault> first"}
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	dec := json.NewDecoder(file)
	chunks := []Chunk{}
	for {
		var chunk Chunk
		if err := dec.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

type piece struct{ heading, text string }

func splitNote(note domain.Note) []piece {
	lines := strings.Split(note.Body, "\n")
	heading := ""
	parts := []piece{}
	buf := []string{}
	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = nil
		if text == "" {
			return
		}
		parts = append(parts, piece{heading: heading, text: text})
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			flush()
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		if trimmed == "" && len(buf) > 0 {
			flush()
			continue
		}
		buf = append(buf, line)
	}
	flush()
	if len(parts) == 0 {
		parts = append(parts, piece{heading: note.Title, text: note.Title})
	}
	return parts
}

func providerFromChunks(chunks []Chunk) string {
	if len(chunks) > 0 && chunks[0].Provider != "" {
		return chunks[0].Provider
	}
	return DefaultProvider
}

func modelFromChunks(chunks []Chunk) string {
	if len(chunks) > 0 && chunks[0].EmbeddingModel != "" {
		return chunks[0].EmbeddingModel
	}
	return DefaultModel
}

func normalizedBackend(backend string) string {
	if strings.TrimSpace(backend) == "" {
		return DefaultBackend
	}
	return strings.ToLower(strings.TrimSpace(backend))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func boundedPreview(text string) string {
	fields := strings.Fields(text)
	if len(fields) > 24 {
		fields = fields[:24]
	}
	return strings.Join(fields, " ")
}

func tokenCount(text string) int { return len(strings.Fields(text)) }

func sha(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func hashEmbedding(text string, dim int) []float64 {
	vec := make([]float64, dim)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if token == "" {
			continue
		}
		sum := sha256.Sum256([]byte(token))
		idx := int(sum[0]) % dim
		vec[idx] += 1 + float64(sum[1])/255
	}
	return vec
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, aa, bb float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
