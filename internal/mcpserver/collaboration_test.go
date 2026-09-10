package mcpserver

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func collaborationServerFixture(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	svc := app.NewService()
	ctx := context.Background()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Example", Body: "# Heading\n\nOriginal paragraph.\n", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	return NewServerWithOptions(svc, root, ServerOptions{Collaboration: true, NotePolicy: app.CollaborationPolicy{AllowBody: true, AllowWrite: true}}), p.Facts["path"]
}

func interactionCall(t *testing.T, s *Server, name string, args map[string]any) map[string]any {
	t.Helper()
	r, err := s.Handle(context.Background(), Request{ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.interaction." + name, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Result["isError"] == true {
		t.Fatalf("tool failed: %v", r.Result["content"])
	}
	return r.Result["structuredContent"].(map[string]any)["data"].(map[string]any)
}

func TestCollaborationMCPDiscoveryResourcesAndWorkflow(t *testing.T) {
	s, path := collaborationServerFixture(t)
	ctx := context.Background()
	r, err := s.Handle(ctx, Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range s.collaborationTools() {
		if err := validateSchemaKeywordSupport(tool.InputSchema); err != nil {
			t.Fatal(err)
		}
		if !containsTool(r.Tools, tool.Name) {
			t.Fatal("missing tool", tool.Name)
		}
	}
	defs := r.Result["tools"].([]map[string]any)
	for _, d := range defs {
		if d["name"] == "pinax.interaction.apply" && d["annotations"].(map[string]any)["readOnlyHint"] != false {
			t.Fatal("write tool advertises readonly")
		}
	}
	for _, uri := range []string{"pinax://interaction/capabilities", "pinax://interaction/note/" + url.PathEscape(path)} {
		r, err := s.Handle(ctx, Request{ID: 1, Method: "resources/read", Params: map[string]any{"uri": uri}})
		if err != nil {
			t.Fatal(err)
		}
		content := r.Result["contents"].([]map[string]any)[0]
		if content["text"] == "" {
			t.Fatal("empty resource")
		}
	}
	search := interactionCall(t, s, "search", map[string]any{"query": "Original", "limit": 1})
	if len(search["notes"].([]any)) != 1 {
		t.Fatalf("search missing note: %v", search)
	}
	interactionCall(t, s, "read", map[string]any{"note_ref": path})
	n := interactionCall(t, s, "read", map[string]any{"note_ref": path, "display": "body", "intent": "edit"})
	preview := interactionCall(t, s, "preview", map[string]any{"change": map[string]any{"action": "append", "note_ref": path, "expected_revision": n["revision"], "body": "\nAdded once."}})
	args := map[string]any{"change": preview["change"], "preview_digest": preview["preview_digest"], "authorization": "reviewed_proposal"}
	receipt := interactionCall(t, s, "apply", args)
	if receipt["status"] != "succeeded" {
		t.Fatal("not saved")
	}
	id := receipt["operation_id"]
	interactionCall(t, s, "status", map[string]any{"operation_id": id})
	old := interactionCall(t, s, "version", map[string]any{"operation_id": id})
	if strings.Contains(old["body"].(string), "Added once") || old["historical"] != true {
		t.Fatal("wrong historical content")
	}
	interactionCall(t, s, "apply", args)
	n = interactionCall(t, s, "read", map[string]any{"note_ref": path, "display": "body", "intent": "read"})
	if strings.Count(n["body"].(string), "Added once") != 1 {
		t.Fatal("duplicate append")
	}
}

func TestCollaborationMCPPermissionAndSchemaFallback(t *testing.T) {
	s, path := collaborationServerFixture(t)
	ctx := context.Background()
	legacy := NewServer(s.service, s.vault)
	if _, err := legacy.Handle(ctx, Request{ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.interaction.read", "arguments": map[string]any{"note_ref": path}}}); err == nil {
		t.Fatal("legacy silently enabled")
	}
	readonly := NewServerWithOptions(s.service, s.vault, ServerOptions{Collaboration: true})
	r, err := readonly.Handle(ctx, Request{ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.interaction.read", "arguments": map[string]any{"note_ref": path, "display": "body", "intent": "read"}}})
	if err != nil || r.Result["isError"] != true {
		t.Fatal("body permission bypass", err)
	}
	for _, args := range []map[string]any{{"query": "x", "unknown": true}, {"query": "x", "limit": 21}, {"query": "x", "offset": -1}} {
		if _, err := s.Handle(ctx, Request{ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.interaction.search", "arguments": args}}); err == nil {
			t.Fatal("invalid schema accepted")
		}
	}
	// Adding collaboration permissions never changes the old bounded read surface.
	r, err = s.Handle(ctx, Request{ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.note.read", "arguments": map[string]any{"note_id": path}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(r.Result)
	if strings.Contains(string(raw), `"body":`) {
		t.Fatal("legacy read gained body")
	}
	md := collaborationMarkdown("note", map[string]any{"title": "<img src=x>", "path": "index/a.md", "body": "```\n<script>alert(1)</script>\n```"})
	if strings.Contains(md, "## <img") || !strings.Contains(md, "````text") {
		t.Fatal("unsafe Markdown delimiters")
	}
}

func TestCollaborationMCPModernResourcesAndLargeFrames(t *testing.T) {
	s, _ := collaborationServerFixture(t)
	meta := map[string]any{protocolVersionMetaKey: modernProtocolVersion, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	for _, uri := range []string{"pinax://interaction/capabilities"} {
		r, err := s.Handle(context.Background(), Request{ID: 1, Method: "resources/read", Params: map[string]any{"uri": uri, "_meta": meta}})
		if err != nil || r.Result["resultType"] != "complete" {
			t.Fatal("modern resource missing completion", err)
		}
	}
	// A valid UTF-8 body can exceed bufio.Scanner's default once JSON escaped.
	req := Request{JSONRPC: "2.0", ID: 2, Method: "tools/call", Params: map[string]any{"name": "pinax.interaction.preview", "arguments": map[string]any{"change": map[string]any{"action": "create", "title": "Large frame", "body": strings.Repeat("\t", 40000)}}, "_meta": meta}}
	raw, _ := json.Marshal(req)
	var out strings.Builder
	if err := ServeWithOptions(context.Background(), s.service, s.vault, strings.NewReader(string(raw)+"\n"), &out, ServerOptions{Collaboration: true, NotePolicy: s.notePolicy}); err != nil {
		t.Fatal("large frame rejected", err)
	}
	var r Response
	if err := json.Unmarshal([]byte(out.String()), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error != nil || r.Result["isError"] == true {
		t.Fatal("large preview failed")
	}
}

func TestCollaborationDoesNotAdvertiseOrServeHTML(t *testing.T) {
	s, _ := collaborationServerFixture(t)
	ctx := context.Background()
	for _, req := range []Request{
		{ID: 1, Method: "initialize", Params: map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"extensions": map[string]any{"io.modelcontextprotocol/ui": map[string]any{}}}}},
		{ID: 2, Method: "tools/list"}, {ID: 3, Method: "resources/list"}, {ID: 4, Method: "resources/templates/list"},
		{ID: 5, Method: "resources/read", Params: map[string]any{"uri": "pinax://interaction/capabilities"}},
	} {
		r, err := s.Handle(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(r)
		for _, forbidden := range []string{"ui://", "io.modelcontextprotocol/ui", "resourceUri", "ui_resource", "text/html", "optional_host_extension"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("retired HTML advertised in %s", req.Method)
			}
		}
	}
	_, err := s.Handle(ctx, Request{ID: 6, Method: "resources/read", Params: map[string]any{"uri": "ui://pinax/collaboration-v1.html"}})
	if e, ok := err.(*MCPError); !ok || e.Data["legacy_code"] != "resource_not_found" {
		t.Fatalf("retired HTML URI was not rejected: %v", err)
	}
}
