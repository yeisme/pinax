package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
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
	items := make([]chunkInput, 0, len(notes))
	for _, note := range notes {
		for _, piece := range splitNote(note) {
			items = append(items, chunkInput{note: note, piece: piece})
		}
	}
	vectors, err := embedInputs(ctx, provider, items)
	if err != nil {
		return nil, err
	}
	chunks := make([]Chunk, 0, len(items))
	for i, item := range items {
		vector := vectors[i]
		chunkHash := sha(item.piece.text)
		chunks = append(chunks, Chunk{ChunkID: "chunk_" + chunkHash[:16], NoteID: item.note.ID, VaultPath: item.note.Path, Title: item.note.Title, HeadingPath: item.piece.heading, ChunkText: item.piece.text, Preview: boundedPreview(item.piece.text), ContentHash: sha(item.note.Body), ChunkHash: chunkHash, TokenCount: tokenCount(item.piece.text), Tags: item.note.Tags, Kind: item.note.Kind, Status: item.note.Status, EmbeddingModel: provider.Model(), EmbeddingDim: len(vector), Provider: provider.Name(), Backend: backend, Vector: vector, IndexedAt: indexedAt})
	}
	return chunks, nil
}

type chunkInput struct {
	note  domain.Note
	piece piece
}

func embedInputs(ctx context.Context, provider Provider, items []chunkInput) ([][]float64, error) {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.piece.text)
	}
	if batch, ok := provider.(BatchProvider); ok {
		vectors, err := batch.EmbedBatch(ctx, texts)
		if err != nil {
			return nil, err
		}
		if len(vectors) != len(items) {
			return nil, providerEmptyEmbedding(provider.Name())
		}
		return vectors, nil
	}
	vectors := make([][]float64, 0, len(items))
	for _, text := range texts {
		vector, err := provider.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
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
		return nil, 0, invalidBackendError()
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
