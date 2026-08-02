package semantic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestBackendRegistryAndInvalidBackend(t *testing.T) {
	infos := ListBackends()
	seen := map[string]BackendInfo{}
	for _, info := range infos {
		seen[info.Name] = info
	}
	if !seen[DefaultBackend].RequiresSidecar || !seen[FakeBackend].LocalOnly {
		t.Fatalf("backend infos = %#v", infos)
	}

	_, err := Save(context.Background(), t.TempDir(), nil, "unknown", SidecarConfig{}, 0)
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "backend_invalid" || !strings.Contains(cmdErr.Hint, "lancedb") || !strings.Contains(cmdErr.Hint, "fake") {
		t.Fatalf("invalid backend error = %#v err=%v", cmdErr, err)
	}
}

func TestInferrumSidecarRequestUsesV1Protocol(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(t.TempDir(), "sidecar")
	logPath := filepath.Join(t.TempDir(), "request.json")
	script := "#!/bin/sh\ncat > " + logPath + "\nprintf '%s\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"rows\":1}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	chunks := []Chunk{{ChunkID: "chunk_a", VaultPath: "notes/a.md", Title: "A", Preview: "bounded", ContentHash: "c", ChunkHash: "h", TokenCount: 1, EmbeddingModel: "text-embedding-3-small", EmbeddingDim: 2, Provider: "openai", Backend: DefaultBackend, Vector: []float64{1, 0}, IndexedAt: "2026-06-24T00:00:00Z"}}
	if _, err := Save(context.Background(), root, chunks, DefaultBackend, SidecarConfig{Executable: bin}, 1); err != nil {
		t.Fatalf("Save lancedb failed: %v", err)
	}
	payload, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	for _, want := range []string{`"schema_version":"inferrum.sidecar.v1"`, `"domain":"kb"`, `"table":"note_chunks"`, `"embedding_dim":2`, `"distance_metric":"cosine"`, `"records"`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("sidecar request missing %s:\n%s", want, payload)
		}
	}
	for _, forbidden := range []string{`"chunks"`, `"collection"`, `"chunk_text"`, `"vault_path"`} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("sidecar request contains legacy or unsafe field %s:\n%s", forbidden, payload)
		}
	}
}
