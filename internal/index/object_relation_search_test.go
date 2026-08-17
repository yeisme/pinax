package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestSearchTagFilterUsesObjectRelationAndReturnsCurrentPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	note := domain.Note{ID: objectID, Title: "Alpha", Path: "notes/current.md", Tags: []string{"project/alpha"}, Project: "alpha", Folder: "research", Kind: "reference", Status: "active", Body: "object relation search"}
	if _, err := Rebuild(root, []domain.Note{note}); err != nil {
		t.Fatal(err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&TagRecord{}).Where("object_id = ?", objectID).Update("note_path", "notes/old.md").Error; err != nil {
		t.Fatal(err)
	}
	result, err := Search(root, SearchRequest{Tags: []string{"project/alpha"}, Group: "alpha", Folder: "research", Kind: "reference", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Results[0].Note.ID != objectID || result.Results[0].Note.Path != "notes/current.md" {
		t.Fatalf("result = %#v", result)
	}
}
