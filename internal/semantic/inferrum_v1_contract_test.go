package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInferrumV1RebuildUsesCanonicalRecordsAndSafeMetadata(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	requestPath := filepath.Join(t.TempDir(), "request.json")
	script := "#!/bin/sh\ncat > " + requestPath + "\nprintf '%s\\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"rows\":1}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	chunks := []Chunk{{
		ChunkID:        "chunk_a",
		NoteID:         "note_a",
		VaultPath:      "notes/a.md",
		Title:          "A",
		HeadingPath:    "Overview",
		ChunkText:      "full body must stay outside the sidecar metadata",
		Preview:        "bounded preview",
		ContentHash:    "content-hash",
		ChunkHash:      "chunk-hash",
		TokenCount:     8,
		Tags:           []string{"kb"},
		Kind:           "note",
		Status:         "active",
		SourceType:     "markdown",
		SourceVersion:  "sha256:source-version",
		SourceDigest:   "sha256:source-digest",
		EmbeddingModel: "pinax-qwen3-embedding:lowmem",
		EmbeddingDim:   2,
		Provider:       "ollama",
		Backend:        DefaultBackend,
		Vector:         []float64{1, 0},
		IndexedAt:      "2026-08-01T00:00:00Z",
	}}

	_, err := Save(context.Background(), root, chunks, DefaultBackend, SidecarConfig{Executable: bin}, 1)
	payload, readErr := os.ReadFile(requestPath)
	if readErr != nil {
		t.Fatalf("read sidecar request: %v", readErr)
	}
	if err != nil {
		t.Fatalf("Inferrum v1 rebuild should be accepted: %v\nrequest=%s", err, payload)
	}

	var request map[string]any
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("sidecar request is not JSON: %v", err)
	}
	if request["schema_version"] != "inferrum.sidecar.v1" {
		t.Fatalf("schema_version = %v, want inferrum.sidecar.v1", request["schema_version"])
	}
	if request["domain"] != "kb" || request["table"] != "note_chunks" {
		t.Fatalf("domain/table = %v/%v, want kb/note_chunks", request["domain"], request["table"])
	}
	records, ok := request["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("records = %#v, want one canonical record", request["records"])
	}
	record, ok := records[0].(map[string]any)
	if !ok {
		t.Fatalf("record = %#v, want object", records[0])
	}
	if record["id"] != "chunk_a" || record["embedding_model"] != "pinax-qwen3-embedding:lowmem" || record["embedding_dim"] != float64(2) {
		t.Fatalf("record identity = %#v", record)
	}
	metadata, ok := record["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want object", record["metadata"])
	}
	for _, field := range []string{"note_id", "source_ref", "title", "heading_path", "preview", "content_hash", "chunk_hash", "tags", "kind", "status", "source_type", "source_version", "source_digest"} {
		if _, ok := metadata[field]; !ok {
			t.Errorf("metadata missing safe field %q: %#v", field, metadata)
		}
	}
	for key, want := range map[string]string{
		"source_type":    "markdown",
		"source_version": "sha256:source-version",
		"source_digest":  "sha256:source-digest",
	} {
		if got, _ := metadata[key].(string); got != want {
			t.Errorf("metadata[%q] = %q, want %q", key, got, want)
		}
	}
	for _, forbidden := range []string{"chunk_text", "vault_path", "raw_prompt", "provider_payload", "Authorization", "secret"} {
		if strings.Contains(string(payload), forbidden) {
			t.Errorf("sidecar payload contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestInferrumV1RebuildUsesExplicitGenerationStoreURI(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	requestPath := filepath.Join(t.TempDir(), "request.json")
	script := "#!/bin/sh\ncat > " + requestPath + "\nprintf '%s\\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"rows\":1}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}
	generationStore := filepath.Join(root, ".pinax", "kb", "generations", "gen-1", "lancedb")
	chunks := []Chunk{{
		ChunkID:        "chunk-1",
		VaultPath:      "notes/a.md",
		Title:          "A",
		ChunkText:      "alpha",
		Preview:        "alpha",
		ContentHash:    "sha256:content",
		ChunkHash:      "sha256:chunk",
		EmbeddingModel: "fake-hash-v1",
		EmbeddingDim:   2,
		Provider:       "fake",
		Backend:        DefaultBackend,
		Vector:         []float64{1, 0},
		IndexedAt:      "2026-08-02T00:00:00Z",
	}}
	if _, err := Save(context.Background(), root, chunks, DefaultBackend, SidecarConfig{Executable: bin, StoreURI: generationStore}, 1); err != nil {
		t.Fatalf("save staged store: %v", err)
	}
	payload, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("staged store request missing: %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("staged store request invalid: %v", err)
	}
	if request["store_uri"] != generationStore {
		t.Fatalf("store_uri = %v, want %s", request["store_uri"], generationStore)
	}
}
