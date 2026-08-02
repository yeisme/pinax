package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type LegacyProjection struct {
	Present      bool
	Protocol     string
	Provider     string
	Model        string
	EmbeddingDim int
	IndexedAt    string
}

// DetectLegacyV1 reports only the bounded identity of a legacy projection. It
// never returns the legacy store path or its rows to a caller projection.
func DetectLegacyV1(root string) (LegacyProjection, error) {
	path := filepath.Join(root, ".pinax", "kb", "lancedb", "metadata.json")
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return LegacyProjection{}, nil
	}
	if err != nil {
		return LegacyProjection{}, err
	}
	var metadata struct {
		SchemaVersion string `json:"schema_version"`
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		EmbeddingDim  int    `json:"embedding_dim"`
		IndexedAt     string `json:"indexed_at"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return LegacyProjection{}, &domain.CommandError{Code: "kb_legacy_v1_invalid", Message: "Legacy KB projection metadata is invalid", Hint: "Rebuild a new Inferrum v1 candidate before using the legacy profile"}
	}
	if metadata.SchemaVersion != LegacySidecarSchema {
		return LegacyProjection{}, nil
	}
	return LegacyProjection{Present: true, Protocol: LegacySidecarSchema, Provider: strings.TrimSpace(metadata.Provider), Model: strings.TrimSpace(metadata.Model), EmbeddingDim: metadata.EmbeddingDim, IndexedAt: strings.TrimSpace(metadata.IndexedAt)}, nil
}

// SearchLegacyV1 is a deliberately narrow read-only compatibility path for a
// legacy JSONL fixture/projection. It never starts a sidecar and requires an
// explicit resolved allow-list to preserve fail-closed permission semantics.
func SearchLegacyV1(ctx context.Context, root, query string, provider Provider, limit int, allowedIDs []string) ([]SearchHit, int, error) {
	projection, err := DetectLegacyV1(root)
	if err != nil {
		return nil, 0, err
	}
	if !projection.Present {
		return nil, 0, &domain.CommandError{Code: "kb_legacy_v1_unavailable", Message: "Legacy v1 KB projection is unavailable", Hint: "Run a normal pinax kb rebuild to create an Inferrum v1 candidate"}
	}
	if allowedIDs == nil {
		return nil, 0, &domain.CommandError{Code: "permission_unresolved", Message: "KB permission candidates could not be resolved", Hint: "Resolve Pinax permissions before using the legacy read-only profile"}
	}
	if len(allowedIDs) == 0 {
		return []SearchHit{}, 0, nil
	}
	store := NewFileStore(root, DefaultBackend)
	chunks, err := store.Load()
	if err != nil {
		return nil, 0, &domain.CommandError{Code: "kb_legacy_v1_unavailable", Message: "Legacy v1 KB rows are unavailable", Hint: "Run a normal pinax kb rebuild to create an Inferrum v1 candidate"}
	}
	if provider == nil {
		provider, err = NewProvider(projection.Provider, projection.Model)
		if err != nil {
			return nil, 0, err
		}
	}
	if projection.Provider != "" && provider.Name() != projection.Provider || projection.Model != "" && provider.Model() != projection.Model {
		return nil, 0, &domain.CommandError{Code: "kb_model_mismatch", Message: "Legacy v1 query provider does not match the indexed projection", Hint: "Use the exact provider and model recorded by the legacy projection or rebuild an Inferrum v1 candidate"}
	}
	vector, err := provider.Embed(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	if projection.EmbeddingDim > 0 && len(vector) != projection.EmbeddingDim {
		return nil, 0, &domain.CommandError{Code: "kb_model_mismatch", Message: "Legacy v1 query embedding dimension does not match the indexed projection", Hint: "Use the exact embedding model recorded by the legacy projection or rebuild an Inferrum v1 candidate"}
	}
	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = struct{}{}
	}
	hits := make([]SearchHit, 0, len(chunks))
	for _, chunk := range chunks {
		if _, ok := allowed[chunk.ChunkID]; !ok || len(chunk.Vector) == 0 || len(chunk.Vector) != len(vector) {
			continue
		}
		sourceRef := safeSourceRef(chunk.VaultPath)
		if sourceRef == "" || strings.TrimSpace(chunk.Preview) == "" {
			continue
		}
		finite := true
		for _, value := range chunk.Vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				finite = false
				break
			}
		}
		if !finite {
			continue
		}
		score := cosine(vector, chunk.Vector)
		if strings.Contains(strings.ToLower(chunk.ChunkText), strings.ToLower(query)) {
			score += 0.2
		}
		hits = append(hits, SearchHit{ChunkID: chunk.ChunkID, NoteID: chunk.NoteID, Path: sourceRef, Title: chunk.Title, HeadingPath: chunk.HeadingPath, Preview: boundedPreview(chunk.Preview), Score: score, Provider: chunk.Provider, Model: chunk.EmbeddingModel, Tags: chunk.Tags, Kind: chunk.Kind, Status: chunk.Status})
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
