package sdkruntime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
)

func TestSDKNoteProposalQueryParity(t *testing.T) {
	session, root := startRuntime(t, mcpserver.ServerOptions{NoteProposals: true})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.agent.propose_note_draft", Arguments: map[string]any{"title": "Fixture", "body": "PRIVATE_QUERY_PAYLOAD"}})
		if err != nil || r.IsError {
			t.Fatal("draft failed", err)
		}
	}
	args := map[string]any{"status": "auto_applied", "limit": 1}
	for page := 0; page < 3; page++ {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.agent.note_proposals", Arguments: args})
		if err != nil || r.IsError {
			t.Fatal("query failed", err)
		}
		b, _ := json.Marshal(r.StructuredContent)
		if strings.Contains(string(b), root) || strings.Contains(string(b), "PRIVATE_QUERY_PAYLOAD") {
			t.Fatal("SDK leaked content")
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err = json.Unmarshal(b, &envelope); err != nil {
			t.Fatal(err)
		}
		legacy, err := mcpserver.NewServer(app.NewService(), root).Handle(ctx, mcpserver.Request{Method: "tools/call", Params: map[string]any{"name": "pinax.agent.note_proposals", "arguments": args}})
		if err != nil {
			t.Fatal(err)
		}
		lb, _ := json.Marshal(legacy.Result["structuredContent"].(map[string]any)["data"])
		var normalized map[string]any
		_ = json.Unmarshal(lb, &normalized)
		if !reflect.DeepEqual(envelope.Data, normalized) {
			t.Fatal("SDK and legacy differ")
		}
		actions := envelope.Data["tool_actions"].([]any)
		if page < 2 {
			args = actions[len(actions)-1].(map[string]any)["arguments"].(map[string]any)
		}
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.agent.note_proposals", Arguments: map[string]any{"proposal_id": "known", "limit": 1}})
	if err == nil && !bad.IsError {
		t.Fatal("SDK accepted mixed query")
	}
}
