package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestFolderProjectionFromRebuildAndRefresh(t *testing.T) {
	root := t.TempDir()
	writeIndexFixture(t, filepath.Join(root, "spaces", "research", "note.md"), "# Note")
	writeIndexFixture(t, filepath.Join(root, "assets", "images", "diagram.png"), "png")
	if err := os.MkdirAll(filepath.Join(root, "empty", "managed"), 0o755); err != nil {
		t.Fatalf("mkdir empty folder: %v", err)
	}
	writeIndexFixture(t, filepath.Join(root, ".pinax", "folders.json"), `{
  "schema_version": "pinax.folders.v1",
  "folders": [
    {"path": "spaces/research", "purpose": "notes", "managed_status": "managed"},
    {"path": "empty/managed", "purpose": "generic", "managed_status": "managed"}
  ]
}
`)
	notes := []domain.Note{{ID: "note_research", Title: "Research", Path: "spaces/research/note.md", Body: "# Note\n"}}

	counts, err := Rebuild(root, notes)
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if counts.Folders < 3 {
		t.Fatalf("rebuild folder count = %#v", counts)
	}
	folders, status, err := ListFolders(root)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if status.Status != "fresh" {
		t.Fatalf("folder projection status = %#v", status)
	}
	research := findFolderRecord(folders, "spaces/research")
	if research == nil || research.Purpose != "notes" || research.ManagedStatus != "managed" || research.NoteCount != 1 || !research.Exists {
		t.Fatalf("research folder projection = %#v", research)
	}
	assetFolder := findFolderRecord(folders, "assets/images")
	if assetFolder == nil || assetFolder.Purpose != "assets" || assetFolder.AssetCount != 1 {
		t.Fatalf("asset folder projection = %#v", assetFolder)
	}
	empty := findFolderRecord(folders, "empty/managed")
	if empty == nil || !empty.Empty || empty.ManagedStatus != "managed" {
		t.Fatalf("empty managed folder projection = %#v", empty)
	}

	if err := os.MkdirAll(filepath.Join(root, "fresh", "empty"), 0o755); err != nil {
		t.Fatalf("mkdir fresh folder: %v", err)
	}
	if _, err := Refresh(root, notes, RefreshOptions{}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	folders, _, err = ListFolders(root)
	if err != nil {
		t.Fatalf("list folders after refresh: %v", err)
	}
	if findFolderRecord(folders, "fresh/empty") == nil {
		t.Fatalf("refresh did not project new folder: %#v", folders)
	}
}

func findFolderRecord(records []FolderRecord, path string) *FolderRecord {
	for i := range records {
		if records[i].Path == path {
			return &records[i]
		}
	}
	return nil
}
