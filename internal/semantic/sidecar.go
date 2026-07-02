package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

type SidecarConfig struct {
	Executable string
	Timeout    time.Duration
}

type sidecarRequest struct {
	SchemaVersion  string         `json:"schema_version"`
	StoreURI       string         `json:"store_uri"`
	Backend        string         `json:"backend"`
	Provider       string         `json:"provider,omitempty"`
	Model          string         `json:"model,omitempty"`
	EmbeddingDim   int            `json:"embedding_dim,omitempty"`
	DistanceMetric string         `json:"distance_metric,omitempty"`
	Collection     string         `json:"collection,omitempty"`
	Documents      int            `json:"documents,omitempty"`
	Chunks         []sidecarChunk `json:"chunks,omitempty"`
	Query          string         `json:"query,omitempty"`
	QueryVector    []float64      `json:"query_vector,omitempty"`
	Limit          int            `json:"limit,omitempty"`
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
	resp, err := runSidecar(ctx, cfg, "doctor", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend, DistanceMetric: "cosine", Collection: "chunks"})
	if err != nil {
		return nil, err
	}
	return map[string]any{"backend": defaultString(resp.Backend, backend), "available": resp.Status == "success", "dependency": resp.Dependency}, nil
}

func runSidecarRebuild(ctx context.Context, root string, chunks []Chunk, backend string, cfg SidecarConfig, documents int) error {
	req := sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend, Documents: documents, Chunks: toSidecarChunks(chunks), DistanceMetric: "cosine", Collection: "chunks"}
	applyChunkMetadata(&req, chunks)
	_, err := runSidecar(ctx, cfg, "rebuild", req)
	return err
}

func runSidecarSearch(ctx context.Context, root, query string, vector []float64, backend string, limit int, cfg SidecarConfig) ([]SearchHit, int, error) {
	meta := readStoreMetadata(root, backend)
	resp, err := runSidecar(ctx, cfg, "search", sidecarRequest{SchemaVersion: SidecarSchema, StoreURI: sidecarStorePath(root, backend), Backend: backend, Provider: meta.Provider, Model: meta.Model, EmbeddingDim: meta.EmbeddingDim, DistanceMetric: "cosine", Collection: "chunks", Query: query, QueryVector: vector, Limit: limit})
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

func applyChunkMetadata(req *sidecarRequest, chunks []Chunk) {
	if len(chunks) == 0 {
		return
	}
	req.Provider = chunks[0].Provider
	req.Model = chunks[0].EmbeddingModel
	req.EmbeddingDim = chunks[0].EmbeddingDim
}

func sidecarStorePath(root, backend string) string {
	return filepath.Join(root, ".pinax", "kb", normalizedBackend(backend))
}
