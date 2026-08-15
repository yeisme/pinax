package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileStreamsContentAndTruncates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("hello attachment"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old longer content that must be truncated"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	if err := CopyFile(dst, src); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "hello attachment" {
		t.Fatalf("dst = %q err=%v, want streamed src content", got, err)
	}
	// An existing destination keeps its own permissions (OpenFile semantics).
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("existing dst perms = %v, want preserved 0600", info.Mode().Perm())
	}
}

func TestCopyFileFreshDestinationGets0644(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := CopyFile(dst, src); err != nil {
		t.Fatalf("copy: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("fresh dst perms = %v, want 0644", info.Mode().Perm())
	}
}

func TestCopyFileMissingSourceFails(t *testing.T) {
	dir := t.TempDir()
	if err := CopyFile(filepath.Join(dir, "dst"), filepath.Join(dir, "missing")); err == nil {
		t.Fatal("copy from missing source should fail")
	}
}
