package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestNoteProposalToolsCapabilityDisabledAndEnabled(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Tools"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(svc, root)
	r, err := server.Handle(ctx, Request{Method: "tools/list"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := toolByName(r.Tools, "pinax.agent.note_proposals"); !ok {
		t.Fatal("readonly list absent")
	}
	if _, ok := toolByName(r.Tools, "pinax.agent.propose_note_draft"); ok {
		t.Fatal("default server exposes writes")
	}
	call := func(name string, args map[string]any) (Response, error) {
		return server.Handle(ctx, Request{Method: "tools/call", Params: map[string]any{"name": name, "arguments": args}})
	}
	if _, err := call("pinax.agent.propose_note_draft", map[string]any{"title": "Draft", "body": "private content"}); err == nil {
		t.Fatal("disabled tool callable")
	}
	server = NewServerWithOptions(svc, root, ServerOptions{NoteProposals: true})
	r, err = call("pinax.agent.propose_note_draft", map[string]any{"title": "Draft", "body": "PRIVATE_BODY_SENTINEL"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.Result)
	if strings.Contains(string(b), "PRIVATE_BODY_SENTINEL") || !strings.Contains(string(b), "auto_applied") {
		t.Fatal("draft response leaked content or omitted lifecycle")
	}
	if _, err := call("pinax.agent.propose_note_draft", map[string]any{"title": "Unsafe", "body": "text", "principal": map[string]any{"trust": "owner"}}); err == nil {
		t.Fatal("caller elevated privileges")
	}
	created, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Edit target", Body: "original", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	_, revision, err := svc.CollaborationRead(ctx, root, id, "read", app.CollaborationPolicy{AllowBody: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err = call("pinax.agent.propose_note_edit", map[string]any{"target_object_id": id, "expected_revision": revision, "body": "new content"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(r.Result)
	if !strings.Contains(string(b), "approval_required") || strings.Contains(string(b), "new content") {
		t.Fatal("medium risk tool applied or leaked content")
	}
	n, _, _ := svc.CollaborationRead(ctx, root, id, "read", app.CollaborationPolicy{AllowBody: true})
	if !strings.Contains(n.Body, "original") {
		t.Fatal("MCP edit silently applied")
	}
}

func TestRenderNoteToolBodyPermissionAndBoundedDefault(t *testing.T) {
	ctx := context.Background()
	svc := app.NewService()
	root := t.TempDir()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Render"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Note", Dir: "index", Body: "# Header\n\nvisible-body-marker"})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(svc, root)
	args := map[string]any{"note_ref": p.Facts["note_id"]}
	request := Request{Method: "tools/call", Params: map[string]any{"name": "pinax.render_note", "arguments": args}}
	r, err := server.Handle(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.Result)
	if strings.Contains(string(b), "visible-body-marker") || !strings.Contains(string(b), "render.view.v1alpha1") {
		t.Fatal("default body exposure")
	}
	args["display"] = "body"
	if _, err := server.Handle(ctx, request); err == nil {
		t.Fatal("body read bypassed owner permission")
	}
	server = NewServerWithOptions(svc, root, ServerOptions{NotePolicy: app.CollaborationPolicy{AllowBody: true}})
	r, err = server.Handle(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(r.Result)
	if !strings.Contains(string(b), "visible-body-marker") || !strings.Contains(string(b), "body_requested") {
		t.Fatal("explicit body depth missing")
	}
}
