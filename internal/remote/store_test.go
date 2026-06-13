package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnimplementedTransportsDoNotReturnNoopStores(t *testing.T) {
	ctx := context.Background()
	for _, endpoint := range []string{"https://cloud.example.test"} {
		store, err := NewStore(ctx, endpoint)
		if err == nil {
			if _, putErr := store.Put(ctx, "head.json", []byte(`{"revision_id":"rev_a"}`), ""); putErr == nil {
				t.Fatalf("NewStore(%q) returned writable no-op store", endpoint)
			}
		}
	}
}

func TestFileBackendConflict(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileBackend(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}
	ctx := context.Background()

	// Initial put
	rev1, err := backend.Put(ctx, "manifest.json", []byte("v1"), "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}

	// Update with correct baseRev
	rev2, err := backend.Put(ctx, "manifest.json", []byte("v2"), rev1)
	if err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	// Update with old baseRev should fail with ErrConflict
	_, err = backend.Put(ctx, "manifest.json", []byte("v3"), rev1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Expected ErrConflict, got %v", err)
	}

	// Read should yield v2
	data, _, err := backend.Get(ctx, "manifest.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "v2" {
		t.Fatalf("Expected v2, got %s", string(data))
	}

	// Stat should yield rev2
	statRev, err := backend.Stat(ctx, "manifest.json")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statRev != rev2 {
		t.Fatalf("Expected %s, got %s", rev2, statRev)
	}
}

func TestFileBackendRejectsKeysOutsideBaseDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "store")
	backend, err := NewFileBackend(base)
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}
	ctx := context.Background()
	outside := filepath.Join(filepath.Dir(base), "outside.txt")

	if _, err := backend.Put(ctx, "../outside.txt", []byte("escape"), ""); err == nil {
		t.Fatal("Put accepted parent traversal key")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("unsafe Put wrote outside base: %v", err)
	}
	if _, _, err := backend.Get(ctx, "../outside.txt"); err == nil {
		t.Fatal("Get accepted parent traversal key")
	}
	if _, err := backend.Stat(ctx, "/tmp/pinax-outside"); err == nil {
		t.Fatal("Stat accepted absolute key")
	}
	if err := backend.Delete(ctx, "nested/../../outside.txt"); err == nil {
		t.Fatal("Delete accepted escaped key")
	}
	if _, err := backend.List(ctx, "../"); err == nil {
		t.Fatal("List accepted escaped prefix")
	}
}
