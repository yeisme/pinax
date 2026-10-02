package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

func TestNoteProposalToolsQueryAndScopedPreview(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := app.NewService()
	if _, err := s.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Query"}); err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Target", Dir: "index", Body: "private original"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	_, rev, err := s.CollaborationRead(ctx, root, id, "read", app.CollaborationPolicy{AllowBody: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, err = s.AgentNotePropose(ctx, app.AgentNoteRequest{VaultPath: root, Principal: agentprotocol.DefaultAdapterPrincipal("test", "mcp"), Scope: agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}, Change: agentprotocol.NoteChange{Kind: "note_edit", TargetObjectID: id, ExpectedRevision: rev, Body: "PRIVATE_QUERY_PAYLOAD"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(s, root)
	call := func(name string, args map[string]any) (Response, error) {
		return server.Handle(ctx, Request{Method: "tools/call", Params: map[string]any{"name": name, "arguments": args}})
	}
	r, err := call("pinax.agent.note_proposals", map[string]any{"status": "approval_required", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.Result)
	if strings.Contains(string(b), root) || strings.Contains(string(b), "PRIVATE_QUERY_PAYLOAD") {
		t.Fatal("owner path or body leaked")
	}
	actions := r.Result["tool_actions"].([]map[string]any)
	next := actions[len(actions)-1]
	r2, err := call(next["tool"].(string), next["arguments"].(map[string]any))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Result["facts"].(map[string]string)["remaining"] != "1" {
		t.Fatal("next page mismatch")
	}
	for _, args := range []map[string]any{{"limit": 0}, {"proposal_id": "known", "limit": 1}, {"status": "unknown"}, {"vault": root}, {"cursor": "invalid"}} {
		if _, err := call("pinax.agent.note_proposals", args); err == nil {
			t.Fatal("invalid query accepted")
		}
	}
	r, err = call("pinax.organize.plan", map[string]any{"note_refs": []any{id}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(r.Result)
	if strings.Contains(string(b), root) || !strings.Contains(string(b), "scoped.v1") {
		t.Fatal("scoped MCP projection wrong")
	}
	if _, err = call("pinax.organize.plan", map[string]any{"note_refs": []any{}}); err == nil {
		t.Fatal("empty selection widened")
	}
}
