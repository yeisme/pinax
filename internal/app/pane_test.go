package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestAssemblePaneSnapshotOmitsAbsolutePathsAndCredentials(t *testing.T) {
	projection := domain.NewProjection("note.list", "Local notes listed.")
	projection.Data = map[string]any{
		"notes": []domain.Note{
			{ID: "note_inbox_1", Title: "Inbox One", Kind: "inbox", Status: "inbox", Tags: []string{"inbox"}, Path: "/home/user/vault/inbox.md"},
			{ID: "/etc/passwd", Title: "bad", Path: "/etc/passwd"},
		},
	}
	envelope, err := AssemblePaneSnapshot(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	entities := envelope.Payload["entities"].([]PaneEntity)
	if envelope.Status != "ready" || len(entities) != 1 || entities[0].Ref != "note_inbox_1" {
		t.Fatalf("envelope=%+v", envelope)
	}
	raw, _ := json.Marshal(envelope)
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"/home/user", "/etc/passwd", "token", "authorization", "setinterval"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("unsafe payload leaked %q: %s", banned, raw)
		}
	}
	if err := RejectHandwrittenMetadata("schema_version: pinax.note.v1\n"); err == nil {
		t.Fatal("handwritten metadata must fail closed")
	}
}

func TestAssemblePaneSnapshotMapsFailedProjectionToOffline(t *testing.T) {
	t.Parallel()
	projection := domain.NewProjection("note.list", "Local notes listed.")
	projection.Status = "failed"
	projection.Data = nil
	envelope, err := AssemblePaneSnapshot(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Status != "offline" {
		t.Fatalf("failed projection must map to offline, got %q", envelope.Status)
	}
	if entities, ok := envelope.Payload["entities"].([]PaneEntity); !ok || len(entities) != 0 {
		t.Fatalf("failed projection must carry no entities: %+v", envelope.Payload["entities"])
	}
}

func TestPaneArtifactFromNoteRejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	_, err := PaneArtifactFromNote(domain.Note{ID: "/tmp/secret.md", Title: "bad"})
	if err == nil {
		t.Fatal("expected unsafe ref rejection")
	}
}

func TestPaneRefAllowlistAndDenylist(t *testing.T) {
	t.Parallel()
	valid := []string{"note_inbox_1", "0198c7d4-9f3e-7a2b-8c1d-2e3f4a5b6c7d", "a1b2c3d4e5f6"}
	for _, ref := range valid {
		if _, err := PaneArtifactFromNote(domain.Note{ID: ref, Title: "ok"}); err != nil {
			t.Fatalf("valid ref %q rejected: %v", ref, err)
		}
	}
	unsafe := []string{"api_key_live", "has space", "note/id", "https://x.example/note", `C:\vault\note.md`, "bearer-cred"}
	for _, ref := range unsafe {
		if _, err := PaneArtifactFromNote(domain.Note{ID: ref, Title: "bad"}); err == nil {
			t.Fatalf("unsafe ref %q accepted", ref)
		}
	}
}

func TestAssemblePaneSnapshotFailsClosedOnUnsupportedNotesShape(t *testing.T) {
	t.Parallel()
	projection := domain.NewProjection("note.list", "Local notes listed.")
	projection.Data = map[string]any{"notes": []any{map[string]any{"id": "note_inbox_1"}}}
	if _, err := AssemblePaneSnapshot(projection, PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"}); err == nil {
		t.Fatal("round-tripped notes shape must fail closed")
	}
}

func TestAssemblePaneNegativeRejectsHandwrittenMetadata(t *testing.T) {
	context := PaneContext{WorkspaceRef: "workspace:demo", Revision: "1"}
	offline, err := AssemblePaneNegative("offline", context)
	if err != nil || offline.Status != "offline" {
		t.Fatalf("offline=%+v err=%v", offline, err)
	}
	denied, err := AssemblePaneNegative("permission_denied", context)
	if err != nil || denied.Status != "permission_denied" {
		t.Fatalf("denied=%+v err=%v", denied, err)
	}
	if _, err := AssemblePaneNegative("handwritten_metadata", context); err == nil {
		t.Fatal("handwritten metadata must fail closed")
	}
}
