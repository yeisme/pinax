package e2e

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
	pkgagent "github.com/yeisme/pinax/pkg/agentmemory"
)

// TestAgentMemoryTransportParity 验证 CLI app service、MCP server 和 Go SDK
// 对同一 vault + scope 返回语义一致的 context pack 核心字段。
func TestAgentMemoryTransportParity(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}

	// 1. Seed a confirmed memory via app service (shared underlying store)
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()
	facts, _, err := svc.AgentMemoryPropose(ctx, app.AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("parity-adapter", "codex"),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "ParityCheck",
		Summary:   "same fact across all transports",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "parity-owner",
		Trust:        agentprotocol.TrustLevelOwner,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityApprove},
	}
	if _, err := svc.AgentMemoryApprove(ctx, vault, facts.ProposalID, owner); err != nil {
		t.Fatal(err)
	}

	// 2. Transport A: direct app service
	appPack, err := svc.AgentContextRuntime(ctx, app.AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("parity-a", "codex"),
		Scope:     scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Transport B: MCP server
	mainSvc := app.NewService()
	mcpServer := mcpserver.NewServer(mainSvc, vault)
	mcpResp, err := mcpServer.Handle(ctx, mcpserver.Request{
		ID:     1,
		Method: "tools/call",
		Params: map[string]any{
			"name":      "pinax.agent.context",
			"arguments": map[string]any{"workspace": "default"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mcpResult := mcpResp.Result
	if mcpResult["status"] != "success" {
		t.Fatalf("MCP context failed: %#v", mcpResult)
	}
	mcpPack, ok := mcpResult["pack"].(agentprotocol.ContextPack)
	if !ok {
		t.Fatalf("MCP pack type = %T", mcpResult["pack"])
	}

	// 4. Transport C: Go SDK
	sdkClient := pkgagent.NewClient()
	defer func() { _ = sdkClient.Close() }()
	sdkPack, err := sdkClient.CompileContext(ctx, vault,
		agentprotocol.DefaultAdapterPrincipal("parity-c", "codex"),
		scope, nil, 20, 8000)
	if err != nil {
		t.Fatal(err)
	}

	// Core field parity: same schema version, same fact count
	if appPack.SchemaVersion != mcpPack.SchemaVersion || appPack.SchemaVersion != sdkPack.SchemaVersion {
		t.Errorf("schema mismatch: app=%s mcp=%s sdk=%s", appPack.SchemaVersion, mcpPack.SchemaVersion, sdkPack.SchemaVersion)
	}
	if appPack.EntryCount() != mcpPack.EntryCount() || appPack.EntryCount() != sdkPack.EntryCount() {
		t.Errorf("entry count mismatch: app=%d mcp=%d sdk=%d", appPack.EntryCount(), mcpPack.EntryCount(), sdkPack.EntryCount())
	}

	// All three should contain the same ParityCheck fact
	for label, pack := range map[string]agentprotocol.ContextPack{"app": appPack, "mcp": mcpPack, "sdk": sdkPack} {
		found := false
		for _, f := range pack.Facts {
			if f.Subject == "ParityCheck" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s transport: ParityCheck fact missing from pack (facts=%d)", label, len(pack.Facts))
		}
	}
}
