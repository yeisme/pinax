package app

import (
	"os"
	"path/filepath"
	"testing"

	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

func TestSyncPullObjectMoveUsesCurrentLocalPath(t *testing.T) {
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	current := filepath.Join(root, "notes", "local-current.md")
	writeAppFixture(t, current, "---\nschema_version: pinax.note.v1\nnote_id: "+objectID+"\ntitle: A\n---\n\n# A\n")
	entry := pinaxcloud.ManifestEntry{ObjectID: objectID, ObjectKind: "note", Path: "notes/remote-current.md", RevisionID: "r2", BlobID: "blob"}
	moved, err := moveLocalManifestObject(root, objectID, entry.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !moved || fileExistsApp(current) || !fileExistsApp(filepath.Join(root, "notes", "remote-current.md")) {
		t.Fatal("object move did not relocate current local path")
	}
}

func TestSyncPullObjectDeleteUsesUUIDInsteadOfStalePath(t *testing.T) {
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	current := filepath.Join(root, "notes", "current.md")
	writeAppFixture(t, current, "---\nschema_version: pinax.note.v1\nnote_id: "+objectID+"\ntitle: A\n---\n\n# A\n")
	deleted, err := deleteLocalManifestObject(root, objectID, "notes/stale.md")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("object delete was not applied")
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("current object path still exists: %v", err)
	}
}
