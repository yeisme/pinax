package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

func TestPersonalAssistantMCPGroundingIsAdditiveAndDeleteSafe(t *testing.T) {
	t.Parallel()
	const sentinel = "MCP_TRANSCRIPT_DELETE_SENTINEL_B30F"
	ctx := context.Background()
	vault := t.TempDir()
	noteSvc := app.NewService()
	if _, err := noteSvc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "MCP grounding"}); err != nil {
		t.Fatal(err)
	}
	created, err := noteSvc.CreateNote(ctx, app.CreateNoteRequest{
		VaultPath: vault,
		Title:     "MCP transcript",
		Body:      "private transcript " + sentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	noteID := created.Facts["note_id"]

	memorySvc := app.NewAgentMemoryService()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
	adapter := agentprotocol.DefaultAdapterPrincipal("mcp-fixture", "test")
	proposal, _, err := memorySvc.AgentMemoryPropose(ctx, app.AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapter,
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "MCP transcript fact",
		Summary:   "derived " + sentinel,
		Sources:   agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: noteID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion,
		PrincipalID:   "mcp-owner",
		Trust:         agentprotocol.TrustLevelOwner,
		Capabilities:  []agentprotocol.Capability{agentprotocol.CapabilityApprove, agentprotocol.CapabilityConfirm},
	}
	if _, err := memorySvc.AgentMemoryApprove(ctx, vault, proposal.ProposalID, owner); err != nil {
		t.Fatal(err)
	}
	adapter.Capabilities = append(adapter.Capabilities, agentprotocol.CapabilityHandoff)
	if _, err := memorySvc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath:    vault,
		From:         adapter,
		To:           agentprotocol.DefaultAdapterPrincipal("mcp-next", "test"),
		Scope:        scope,
		Objective:    "continue MCP task",
		CurrentState: "state " + sentinel,
		Sources:      agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: noteID}},
	}); err != nil {
		t.Fatal(err)
	}

	server := NewServer(noteSvc, vault)
	for _, tool := range []string{"pinax.agent.context", "pinax.agent.memory_recall", "pinax.agent.handoff_read"} {
		result := callPersonalAssistantReadonlyTool(t, ctx, server, tool)
		assertLegacyMCPKeys(t, result)
		assertMCPGrounding(t, result, app.AgentGroundingGrounded, 1, 1)
		assertMCPContains(t, result, sentinel, true)
	}

	if _, err := noteSvc.DeleteNote(ctx, app.NoteDeleteRequest{VaultPath: vault, NoteRef: noteID, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"pinax.agent.context", "pinax.agent.memory_recall", "pinax.agent.handoff_read"} {
		result := callPersonalAssistantReadonlyTool(t, ctx, server, tool)
		assertLegacyMCPKeys(t, result)
		assertMCPGrounding(t, result, app.AgentGroundingUngrounded, 0, 0)
		assertMCPContains(t, result, sentinel, false)
	}
}

func callPersonalAssistantReadonlyTool(t *testing.T, ctx context.Context, server *Server, tool string) map[string]any {
	t.Helper()
	response, err := server.Handle(ctx, Request{
		ID:     tool,
		Method: "tools/call",
		Params: map[string]any{"name": tool, "arguments": map[string]any{"workspace": "default"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return response.Result
}

func assertLegacyMCPKeys(t *testing.T, result map[string]any) {
	t.Helper()
	if result["status"] != "success" || result["body_exposure"] != "bounded_projection" || result["command"] == "" {
		t.Fatalf("legacy MCP keys changed: %#v", result)
	}
	if result["grounding"] == nil || result["content"] == nil || result["structuredContent"] == nil {
		t.Fatalf("additive MCP projection missing: %#v", result)
	}
}

func assertMCPGrounding(t *testing.T, result map[string]any, state app.AgentGroundingState, total, openable int) {
	t.Helper()
	grounding, ok := result["grounding"].(app.AgentGroundingEvidence)
	if !ok {
		t.Fatalf("grounding type = %T (%#v)", result["grounding"], result["grounding"])
	}
	if grounding.State != state || grounding.SourceTotal != total || grounding.SourceOpenable != openable {
		t.Fatalf("grounding = %#v", grounding)
	}
}

func assertMCPContains(t *testing.T, result map[string]any, sentinel string, want bool) {
	t.Helper()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Contains(string(payload), sentinel); got != want {
		t.Fatalf("sentinel present=%t want=%t payload=%s", got, want, payload)
	}
}
