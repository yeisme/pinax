package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

type permissionProbeProvider struct{}

func (permissionProbeProvider) Name() string  { return "fake" }
func (permissionProbeProvider) Model() string { return "fake-hash-v1" }
func (permissionProbeProvider) Embed(context.Context, string) ([]float64, error) {
	return []float64{1, 0}, nil
}

func TestSearchWithExplicitEmptyAllowedIDsDoesNotInvokeSidecar(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "sidecar-called")
	bin := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	script := "#!/bin/sh\ntouch " + marker + "\nprintf '%s\\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"total\":0}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	hits, total, err := SearchWithAllowedIDs(context.Background(), root, "private query", permissionProbeProvider{}, DefaultBackend, 5, []string{}, SidecarConfig{Executable: bin})
	if err != nil {
		t.Fatalf("explicit empty permission should be a zero-hit result: %v", err)
	}
	if len(hits) != 0 || total != 0 {
		t.Fatalf("explicit empty permission result = hits=%v total=%d, want zero hits", hits, total)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("explicit empty permission invoked the sidecar")
	}
}

func TestSearchWithResolvedAllowedIDsForwardsOnlyThoseIDs(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	requestPath := filepath.Join(t.TempDir(), "request.json")
	script := "#!/bin/sh\ncat > " + requestPath + "\nprintf '%s\\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"total\":1,\"hits\":[{\"id\":\"chunk_allowed\",\"score\":0.9,\"metadata\":{\"source_ref\":\"notes/a.md\"}}]}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	hits, total, err := SearchWithAllowedIDs(context.Background(), root, "private query", permissionProbeProvider{}, DefaultBackend, 5, []string{"chunk_allowed"}, SidecarConfig{Executable: bin})
	if err != nil {
		t.Fatalf("resolved permission search failed: %v", err)
	}
	if total != 1 || len(hits) != 1 || hits[0].ChunkID != "chunk_allowed" {
		t.Fatalf("search result = hits=%#v total=%d, want one allowed hit", hits, total)
	}
	payload, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("read sidecar request: %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("sidecar request is not JSON: %v", err)
	}
	allowed, ok := request["allowed_ids"].([]any)
	if !ok || len(allowed) != 1 || allowed[0] != "chunk_allowed" {
		t.Fatalf("allowed_ids = %#v, want only chunk_allowed", request["allowed_ids"])
	}
}

func TestSearchWithResolvedAllowedIDsFiltersFakeBackend(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{
		{ID: "note_a", Path: "notes/a.md", Title: "A", Body: "alpha"},
		{ID: "note_b", Path: "notes/b.md", Title: "B", Body: "beta"},
	}
	chunks, err := BuildChunks(context.Background(), notes, FakeProvider{ModelName: FakeProviderModel}, FakeBackend)
	if err != nil {
		t.Fatalf("BuildChunks failed: %v", err)
	}
	if _, err := Save(context.Background(), root, chunks, FakeBackend, SidecarConfig{}, len(notes)); err != nil {
		t.Fatalf("Save fake backend failed: %v", err)
	}
	hits, total, err := SearchWithAllowedIDs(context.Background(), root, "alpha", FakeProvider{ModelName: FakeProviderModel}, FakeBackend, 5, []string{chunks[0].ChunkID}, SidecarConfig{})
	if err != nil {
		t.Fatalf("fake permission search failed: %v", err)
	}
	if total != 1 || len(hits) != 1 || hits[0].ChunkID != chunks[0].ChunkID {
		t.Fatalf("fake permission result = hits=%#v total=%d, want only %q", hits, total, chunks[0].ChunkID)
	}
}
