package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLegacyV1Projection(t *testing.T) {
	root := t.TempDir()
	writeLegacyV1Fixture(t, root)
	projection, err := DetectLegacyV1(root)
	if err != nil {
		t.Fatalf("detect legacy projection: %v", err)
	}
	if !projection.Present || projection.Protocol != LegacySidecarSchema || projection.Provider != "fake" || projection.Model != FakeProviderModel || projection.EmbeddingDim != 32 {
		t.Fatalf("legacy projection = %#v", projection)
	}
}

func TestSearchLegacyV1ReadonlyUsesLocalProjectionWithoutSidecar(t *testing.T) {
	root := t.TempDir()
	writeLegacyV1Fixture(t, root)
	provider := FakeProvider{ModelName: FakeProviderModel}
	vector, err := provider.Embed(context.Background(), "legacy query")
	if err != nil {
		t.Fatalf("embed fixture query: %v", err)
	}
	chunk := Chunk{ChunkID: "chunk-legacy", NoteID: "note-legacy", VaultPath: "notes/legacy.md", Title: "Legacy", HeadingPath: "Legacy", Preview: "legacy bounded preview", EmbeddingModel: FakeProviderModel, EmbeddingDim: len(vector), Provider: "fake", Backend: DefaultBackend, Vector: vector, IndexedAt: "2026-08-02T00:00:00Z"}
	writeLegacyV1Chunk(t, root, chunk)
	hits, total, err := SearchLegacyV1(context.Background(), root, "legacy query", provider, 5, []string{"chunk-legacy"})
	if err != nil {
		t.Fatalf("legacy search: %v", err)
	}
	if total != 1 || len(hits) != 1 || hits[0].ChunkID != chunk.ChunkID || hits[0].Path != chunk.VaultPath {
		t.Fatalf("legacy search result total=%d hits=%#v", total, hits)
	}
}

func TestSearchLegacyV1ReadonlyRejectsMissingProjection(t *testing.T) {
	_, _, err := SearchLegacyV1(context.Background(), t.TempDir(), "legacy query", FakeProvider{ModelName: FakeProviderModel}, 5, nil)
	if err == nil {
		t.Fatal("missing legacy projection should fail")
	}
}

func TestSearchLegacyV1ReadonlyDropsUnsafeCitationPaths(t *testing.T) {
	root := t.TempDir()
	writeLegacyV1Fixture(t, root)
	provider := FakeProvider{ModelName: FakeProviderModel}
	vector, err := provider.Embed(context.Background(), "legacy query")
	if err != nil {
		t.Fatalf("embed fixture query: %v", err)
	}
	safe := Chunk{ChunkID: "chunk-legacy", NoteID: "note-legacy", VaultPath: "notes/legacy.md", Title: "Legacy", Preview: "safe", EmbeddingModel: FakeProviderModel, EmbeddingDim: len(vector), Provider: "fake", Backend: DefaultBackend, Vector: vector, IndexedAt: "2026-08-02T00:00:00Z"}
	writeLegacyV1Chunk(t, root, safe)
	unsafe := Chunk{ChunkID: "chunk-unsafe", NoteID: "note-unsafe", VaultPath: "/private/secret.md", Title: "Unsafe", Preview: "secret", EmbeddingModel: FakeProviderModel, EmbeddingDim: len(vector), Provider: "fake", Backend: DefaultBackend, Vector: vector, IndexedAt: "2026-08-02T00:00:00Z"}
	path := filepath.Join(root, ".pinax", "kb", "lancedb", "chunks.jsonl")
	payload, err := json.Marshal(unsafe)
	if err != nil {
		t.Fatalf("marshal unsafe chunk: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open legacy chunks: %v", err)
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		t.Fatalf("append unsafe chunk: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close legacy chunks: %v", err)
	}
	hits, _, err := SearchLegacyV1(context.Background(), root, "legacy query", provider, 5, []string{"chunk-legacy", "chunk-unsafe"})
	if err != nil {
		t.Fatalf("legacy search: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "chunk-legacy" {
		t.Fatalf("legacy search unsafe hits = %#v, want only safe chunk", hits)
	}
}

func TestSearchLegacyV1ReadonlyRejectsProviderIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	writeLegacyV1Fixture(t, root)
	vector, err := (FakeProvider{ModelName: FakeProviderModel}).Embed(context.Background(), "legacy query")
	if err != nil {
		t.Fatalf("embed fixture query: %v", err)
	}
	writeLegacyV1Chunk(t, root, Chunk{ChunkID: "chunk-legacy", VaultPath: "notes/legacy.md", Preview: "safe", EmbeddingModel: FakeProviderModel, EmbeddingDim: len(vector), Provider: "fake", Backend: DefaultBackend, Vector: vector, IndexedAt: "2026-08-02T00:00:00Z"})
	provider := FakeProvider{ModelName: "other-model"}
	_, _, err = SearchLegacyV1(context.Background(), root, "legacy query", provider, 5, []string{"chunk-legacy"})
	if err == nil || !strings.Contains(err.Error(), "kb_model_mismatch") {
		t.Fatalf("legacy model mismatch error = %v, want kb_model_mismatch", err)
	}
}

func writeLegacyV1Fixture(t *testing.T, root string) {
	t.Helper()
	store := filepath.Join(root, ".pinax", "kb", "lancedb")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("mkdir legacy store: %v", err)
	}
	payload := map[string]any{"schema_version": LegacySidecarSchema, "backend": DefaultBackend, "provider": "fake", "model": FakeProviderModel, "embedding_dim": 32, "indexed_at": "2026-08-02T00:00:00Z"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "metadata.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}
}

func writeLegacyV1Chunk(t *testing.T, root string, chunk Chunk) {
	t.Helper()
	path := filepath.Join(root, ".pinax", "kb", "lancedb", "chunks.jsonl")
	payload, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal legacy chunk: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy chunk: %v", err)
	}
}
