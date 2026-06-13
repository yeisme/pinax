package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPerNoteAttachmentPlacementKeepsCompatiblePathAndConflicts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "attachments", "note_alpha"), 0o755); err != nil {
		t.Fatalf("mkdir attachments: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "attachments", "note_alpha", "diagram.png"), []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	rel, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "notes/alpha.md", Filename: "diagram.png", Policy: AttachmentPlacementPerNote})
	if err != nil {
		t.Fatalf("place per-note attachment: %v", err)
	}
	if rel != "attachments/note_alpha/diagram-2.png" {
		t.Fatalf("per-note rel = %q", rel)
	}
}

func TestVaultFolderAttachmentPlacement(t *testing.T) {
	root := t.TempDir()
	rel, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "notes/alpha.md", Filename: "diagram.png", Policy: AttachmentPlacementVaultFolder})
	if err != nil {
		t.Fatalf("place vault-folder attachment: %v", err)
	}
	if rel != "attachments/diagram.png" {
		t.Fatalf("vault-folder rel = %q", rel)
	}
}

func TestNoteFolderAttachmentPlacement(t *testing.T) {
	root := t.TempDir()
	rel, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "notes/project/alpha.md", Filename: "diagram.png", Policy: AttachmentPlacementNoteFolder})
	if err != nil {
		t.Fatalf("place note-folder attachment: %v", err)
	}
	if rel != "notes/project/assets/diagram.png" {
		t.Fatalf("note-folder rel = %q", rel)
	}
}

func TestAttachmentPlacementRejectsUnsafeInput(t *testing.T) {
	root := t.TempDir()
	if _, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "../alpha.md", Filename: "diagram.png", Policy: AttachmentPlacementNoteFolder}); err == nil {
		t.Fatalf("accepted unsafe note path")
	}
	if _, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "notes/alpha.md", Filename: "", Policy: AttachmentPlacementPerNote}); err == nil {
		t.Fatalf("accepted empty filename")
	}
	if _, err := PlaceAttachment(AttachmentPlacementRequest{Root: root, NoteID: "note_alpha", NotePath: "notes/alpha.md", Filename: "diagram.png", Policy: AttachmentPlacementPolicy("custom")}); err == nil {
		t.Fatalf("accepted unsupported placement policy")
	}
}
