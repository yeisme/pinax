package sdkruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yeisme/pinax/internal/mcpserver"
)

func TestSDKNoteProposalCapabilityGateAndRenderCall(t *testing.T) {
	ctx := context.Background()
	for _, enabled := range []bool{false, true} {
		session, _ := startRuntime(t, mcpserver.ServerOptions{NoteProposals: enabled})
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		names := toolNames(tools.Tools)
		if !names["pinax.render_note"] || !names["pinax.agent.note_proposals"] || names["pinax.agent.propose_note_draft"] != enabled {
			t.Fatal("SDK discovery gate mismatch")
		}
		if !enabled {
			continue
		}
		for _, tool := range tools.Tools {
			if strings.HasPrefix(tool.Name, "pinax.agent.propose_note_") && tool.Annotations.IdempotentHint {
				t.Fatal("proposal tool invites unsafe automatic replay")
			}
		}
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.agent.propose_note_draft", Arguments: map[string]any{"title": "Fixture draft", "body": "PRIVATE_BODY_SENTINEL"}})
		if err != nil || r.IsError {
			t.Fatal("SDK propose failed", err)
		}
		b, _ := json.Marshal(r.StructuredContent)
		if strings.Contains(string(b), "PRIVATE_BODY_SENTINEL") || !strings.Contains(string(b), "auto_applied") {
			t.Fatal("SDK content or lifecycle mismatch")
		}
		var envelope struct {
			Data struct {
				Data struct {
					TargetObjectID string `json:"target_object_id"`
				} `json:"data"`
			} `json:"data"`
		}
		if err := json.Unmarshal(b, &envelope); err != nil || envelope.Data.Data.TargetObjectID == "" {
			t.Fatal("proposal has no target")
		}
		r, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.render_note", Arguments: map[string]any{"note_ref": envelope.Data.Data.TargetObjectID}})
		if err != nil || r.IsError {
			t.Fatal("SDK render failed", err)
		}
		b, _ = json.Marshal(r.StructuredContent)
		if !strings.Contains(string(b), "render.view.v1alpha1") || strings.Contains(string(b), "PRIVATE_BODY_SENTINEL") {
			t.Fatal("SDK bounded render mismatch")
		}
	}
}
