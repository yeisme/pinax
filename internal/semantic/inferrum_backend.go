package semantic

import (
	"context"
	"fmt"
	"strings"
	"time"

	inferrum "github.com/yeisme/inferrum"
	"github.com/yeisme/pinax/internal/domain"
)

func newInferrumStore(root string, cfg SidecarConfig) *inferrum.VectorStore {
	storeURI := sidecarStoreURI(root, DefaultBackend, cfg)
	return inferrum.NewVectorStore(
		KBDomain{},
		inferrum.NewSidecarClient(inferrum.SidecarConfig{Executable: cfg.Executable, Timeout: cfg.Timeout}),
		storeURI,
	)
}

func runInferrumSidecarDoctor(ctx context.Context, root string, cfg SidecarConfig) (map[string]any, error) {
	status, err := newInferrumStore(root, cfg).Doctor(ctx)
	if err != nil {
		return nil, translateInferrumError(err)
	}
	return map[string]any{"backend": status.Backend, "available": status.Available, "dependency": status.Dependency}, nil
}

func runInferrumSidecarRebuild(ctx context.Context, root string, chunks []Chunk, cfg SidecarConfig, documents int) error {
	records, dimension, err := toInferrumRecords(chunks)
	if err != nil {
		return err
	}
	rows, err := newInferrumStore(root, cfg).Rebuild(ctx, records, inferrum.RebuildOpts{DistanceMetric: "cosine", EmbeddingDim: dimension})
	if err != nil {
		return translateInferrumError(err)
	}
	if rows != len(records) || documents < 0 {
		return &domain.CommandError{Code: "kb_sidecar_row_count_mismatch", Message: "KB LanceDB sidecar returned an unexpected row count", Hint: "Re-run pinax kb rebuild after checking the local sidecar"}
	}
	return nil
}

func runInferrumSidecarSearch(ctx context.Context, root string, vector []float64, allowedIDs []string, limit int, cfg SidecarConfig) ([]SearchHit, int, error) {
	hits, total, err := newInferrumStore(root, cfg).Search(ctx, vector, allowedIDs, limit)
	if err != nil {
		return nil, 0, translateInferrumError(err)
	}
	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, searchHitFromInferrum(hit))
	}
	return out, total, nil
}

func toInferrumRecords(chunks []Chunk) ([]inferrum.Record, int, error) {
	records := make([]inferrum.Record, 0, len(chunks))
	dimension := 0
	for _, chunk := range chunks {
		if len(chunk.Vector) == 0 || chunk.EmbeddingDim <= 0 || len(chunk.Vector) != chunk.EmbeddingDim {
			return nil, 0, &domain.CommandError{Code: "embedding_dimension_invalid", Message: "Embedding vector dimension is invalid", Hint: "Re-run embedding and ensure every chunk has a non-empty fixed dimension"}
		}
		if dimension == 0 {
			dimension = len(chunk.Vector)
		}
		if len(chunk.Vector) != dimension {
			return nil, 0, &domain.CommandError{Code: "embedding_dimension_mismatch", Message: "Embedding vectors have mixed dimensions", Hint: "Use one exact embedding model and rebuild the local index"}
		}
		indexedAt, err := time.Parse(time.RFC3339, chunk.IndexedAt)
		if err != nil {
			indexedAt = time.Now().UTC()
		}
		records = append(records, inferrum.Record{
			ID:             chunk.ChunkID,
			Vector:         append([]float64(nil), chunk.Vector...),
			EmbeddingModel: chunk.EmbeddingModel,
			EmbeddingDim:   dimension,
			IndexedAt:      indexedAt,
			Metadata: map[string]any{
				"chunk_id":       chunk.ChunkID,
				"note_id":        chunk.NoteID,
				"source_ref":     chunk.VaultPath,
				"title":          chunk.Title,
				"heading_path":   chunk.HeadingPath,
				"preview":        chunk.Preview,
				"content_hash":   chunk.ContentHash,
				"chunk_hash":     chunk.ChunkHash,
				"token_count":    chunk.TokenCount,
				"tags":           chunk.Tags,
				"kind":           chunk.Kind,
				"status":         chunk.Status,
				"source_type":    chunk.SourceType,
				"source_version": chunk.SourceVersion,
				"source_digest":  chunk.SourceDigest,
				"provider":       chunk.Provider,
				"model":          chunk.EmbeddingModel,
			},
		})
	}
	return records, dimension, nil
}

func searchHitFromInferrum(hit inferrum.Hit) SearchHit {
	metadata := hit.Metadata
	return SearchHit{
		ChunkID:     hit.ID,
		NoteID:      stringMetadata(metadata, "note_id"),
		Path:        stringMetadata(metadata, "source_ref"),
		Title:       stringMetadata(metadata, "title"),
		HeadingPath: stringMetadata(metadata, "heading_path"),
		Preview:     stringMetadata(metadata, "preview"),
		Score:       hit.Score,
		Provider:    stringMetadata(metadata, "provider"),
		Model:       stringMetadata(metadata, "model"),
		Tags:        stringSliceMetadata(metadata, "tags"),
		Kind:        stringMetadata(metadata, "kind"),
		Status:      stringMetadata(metadata, "status"),
	}
}

func stringMetadata(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func stringSliceMetadata(metadata map[string]any, key string) []string {
	value := metadata[key]
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func translateInferrumError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	code := "kb_sidecar_failed"
	if strings.HasPrefix(message, "inferrum_sidecar_unavailable:") {
		code = "kb_sidecar_unavailable"
	} else if strings.HasPrefix(message, "inferrum_sidecar_timeout:") {
		code = "kb_sidecar_timeout"
	} else if strings.HasPrefix(message, "inferrum_sidecar_protocol_invalid:") {
		code = "kb_sidecar_protocol_invalid"
	}
	return &domain.CommandError{Code: code, Message: "KB LanceDB sidecar operation failed", Hint: fmt.Sprintf("%s; run pinax kb doctor --json to inspect the local sidecar", sanitizeSidecarStderr(message))}
}
