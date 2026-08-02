package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKBImportWritesSafeSourceLineageAndDryRunIsReadOnly(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir vault notes: %v", err)
	}
	sourcePath := filepath.Join(source, "private.md")
	body := "# Private source\n\nCanonical Markdown remains the source of truth.\n"
	if err := os.WriteFile(sourcePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source before dry-run: %v", err)
	}
	if _, err := NewService().KBImport(context.Background(), KBImportRequest{VaultPath: root, Source: sourcePath, DryRun: true}); err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source after dry-run: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("dry-run changed source")
	}

	projection, err := NewService().KBImport(context.Background(), KBImportRequest{VaultPath: root, Source: sourcePath, Yes: true})
	if err != nil {
		t.Fatalf("confirmed import: %v", err)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal import projection: %v", err)
	}
	if strings.Contains(string(payload), sourcePath) {
		t.Fatalf("import projection leaked absolute source path: %s", payload)
	}
	importedPath := filepath.Join(root, "notes", "kb", "imports", "private.md")
	content, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("read imported note: %v", err)
	}
	text := string(content)
	for _, field := range []string{"source_type: markdown", "source_ref: external/", "source_digest: sha256:", "source_version: sha256:", "acquired_at:", "importer_version: pinax.kb.import.v1"} {
		if !strings.Contains(text, field) {
			t.Fatalf("imported note missing lineage field %q:\n%s", field, text)
		}
	}
}
