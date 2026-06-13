package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEvidenceDetectsGitRevisionAndFileBlob(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "pinax@example.local")
	runGit(t, root, "config", "user.name", "Pinax")
	path := filepath.Join(root, "notes", "a.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "initial")

	evidence := Evidence(context.Background(), root, "notes/a.md")
	if evidence.Backend != "git" || evidence.RevisionID == "" || evidence.FileBlobID == "" || evidence.WorktreeState != "clean" {
		t.Fatalf("evidence = %#v", evidence)
	}

	if err := os.WriteFile(path, []byte("# A\nchanged\n"), 0o644); err != nil {
		t.Fatalf("write dirty: %v", err)
	}
	dirty := Evidence(context.Background(), root, "notes/a.md")
	if dirty.WorktreeState != "dirty" || dirty.FileBlobID == evidence.FileBlobID {
		t.Fatalf("dirty evidence = %#v", dirty)
	}
}

func TestEvidenceFallsBackWithoutGit(t *testing.T) {
	evidence := Evidence(context.Background(), t.TempDir(), "notes/a.md")
	if evidence.Backend != "none" || evidence.WorktreeState != "unknown" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
