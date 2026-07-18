package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	"github.com/yeisme/pinax/internal/records"
)

func TestCreateNoteUsesOneCanonicalIdentityAcrossPreviewFileProjectionAndLedger(t *testing.T) {
	root := t.TempDir()
	service := NewService()
	req := CreateNoteRequest{VaultPath: root, Title: "Identity Contract", Body: "body", DryRun: true}

	preview, err := service.CreateNote(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateNote(dry-run) error = %v", err)
	}
	previewID := preview.Facts["note_id"]
	if identity.Classify(previewID) != identity.IDClassCanonical {
		t.Fatalf("dry-run note_id = %q, want canonical UUIDv7", previewID)
	}

	req.DryRun = false
	created, err := service.CreateNote(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateNote(apply) error = %v", err)
	}
	if created.Facts["note_id"] != previewID {
		t.Fatalf("apply note_id = %q, dry-run note_id = %q", created.Facts["note_id"], previewID)
	}

	noteData, ok := created.Data.(map[string]any)
	if !ok {
		t.Fatalf("created.Data type = %T, want map[string]any", created.Data)
	}
	note, ok := noteData["note"].(domain.Note)
	if !ok {
		t.Fatalf("created.Data[note] type = %T, want domain.Note", noteData["note"])
	}
	if note.ID != previewID {
		t.Fatalf("projection note ID = %q, want %q", note.ID, previewID)
	}

	path := filepath.Join(root, filepath.FromSlash(created.Facts["path"]))
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created note: %v", err)
	}
	if !strings.Contains(string(payload), "note_id: "+previewID+"\n") {
		t.Fatalf("frontmatter does not contain canonical note_id %q:\n%s", previewID, payload)
	}

	state, err := records.NewService(root).Replay(context.Background())
	if err != nil {
		t.Fatalf("replay record ledger: %v", err)
	}
	record, ok := state.Records[previewID]
	if !ok {
		t.Fatalf("ledger missing record for %q: %#v", previewID, state.Records)
	}
	if record.Path != created.Facts["path"] {
		t.Fatalf("ledger path = %q, want %q", record.Path, created.Facts["path"])
	}
	if len(state.Records) != 2 {
		t.Fatalf("ledger record count = %d, want 2 (created note plus canonical daily journal)", len(state.Records))
	}
}

func TestRecordAdoptAllocatesCanonicalIdentityOnceForNoteWithoutID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes", "legacy.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nschema_version: pinax.note.v1\ntitle: Legacy\ntags: []\n---\n\n# Legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewService().RecordAdopt(context.Background(), RecordRequest{VaultPath: root}); err != nil {
		t.Fatalf("RecordAdopt(first) error = %v", err)
	}
	state, err := records.NewService(root).Replay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 1 {
		t.Fatalf("record count after first adopt = %d, want 1", len(state.Records))
	}
	var objectID string
	for id := range state.Records {
		objectID = id
	}
	if identity.Classify(objectID) != identity.IDClassCanonical {
		t.Fatalf("adopted object ID = %q, want canonical UUIDv7", objectID)
	}

	if _, err := NewService().RecordAdopt(context.Background(), RecordRequest{VaultPath: root}); err != nil {
		t.Fatalf("RecordAdopt(second) error = %v", err)
	}
	state, err = records.NewService(root).Replay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 1 {
		t.Fatalf("record count after repeated adopt = %d, want 1", len(state.Records))
	}
	if _, ok := state.Records[objectID]; !ok {
		t.Fatalf("repeated adopt replaced canonical object ID %q", objectID)
	}
}
