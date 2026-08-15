package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func TestPrunePublishDocPackagesKeepsNewestPerNote(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(publishDocRoot(root), "packages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name, noteID string, age time.Duration) {
		pkg := domain.PublishDocPackage{ID: name, NoteID: noteID}
		body, _ := json.Marshal(pkg)
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		stale := time.Now().Add(-age)
		if err := os.Chtimes(path, stale, stale); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	// note_a: 5 packages → keep 3 newest; note_b: 2 packages → keep all;
	// a corrupt file is left alone.
	write("p1", "note_a", 5*time.Hour)
	write("p2", "note_a", 4*time.Hour)
	write("p3", "note_a", 3*time.Hour)
	write("p4", "note_a", 2*time.Hour)
	write("p5", "note_a", 1*time.Hour)
	write("q1", "note_b", 9*time.Hour)
	write("q2", "note_b", 8*time.Hour)
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	pruned := prunePublishDocPackages(root)
	if pruned != 2 {
		t.Fatalf("pruned = %d, want 2 (only note_a's two oldest)", pruned)
	}
	for _, kept := range []string{"p3.json", "p4.json", "p5.json", "q1.json", "q2.json", "corrupt.json"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("expected %s to survive prune: %v", kept, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Logf("remains: %s", e.Name())
	}
	for _, removed := range []string{"p1.json", "p2.json"} {
		if _, err := os.Stat(filepath.Join(dir, removed)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be pruned, stat err=%v", removed, err)
		}
	}
}
