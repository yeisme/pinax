package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestRenameBacklinkPreservesObjectEdgeAndCurrentPath(t *testing.T) {
	root := t.TempDir()
	sourceID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	targetID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102"
	source := domain.Note{ID: sourceID, Title: "Source", Path: "notes/source.md", Body: "[[Target]]"}
	target := domain.Note{ID: targetID, Title: "Target", Path: "notes/target.md"}
	if _, err := Rebuild(root, []domain.Note{source, target}); err != nil {
		t.Fatal(err)
	}
	target.Title = "Renamed"
	target.Path = "notes/renamed.md"
	if _, err := UpdateNote(root, NoteUpdate{OldPath: "notes/target.md", Note: target}); err != nil {
		t.Fatal(err)
	}
	links, err := LinksByTargetObjectID(root, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("links = %#v", links)
	}
	link := links[0]
	if link.SourceObjectID != sourceID || link.TargetObjectID != targetID || link.TargetPath != "notes/renamed.md" || link.TargetRaw != "Target" || link.Status != string(domain.LinkStatusResolved) {
		t.Fatalf("link = %#v", link)
	}
}
