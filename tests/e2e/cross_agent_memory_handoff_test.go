package e2e

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentadapter"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

// TestCrossAgentMemoryHandoff 验证 Cohors → Codex 和 Codex → Cohors
// 能通过 common handoff schema 完成交接。
func TestCrossAgentMemoryHandoff(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "cross-agent"}
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	harness := agentadapter.NewHarness([]string{agentprotocol.SchemaVersion, agentprotocol.ContextSchemaVersion})

	// 场景 1: Cohors worker produces handoff → Codex reviewer consumes
	codexDesc := agentadapter.CodexDescriptor()
	cohorsDesc := agentadapter.CohorsDescriptor()

	// Negotiate both adapters
	if _, err := harness.Negotiate(codexDesc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityHandoff}); err != nil {
		t.Fatalf("codex negotiate: %v", err)
	}
	if _, err := harness.Negotiate(cohorsDesc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityHandoff}); err != nil {
		t.Fatalf("cohors negotiate: %v", err)
	}

	// Cohors worker → Codex reviewer handoff
	cohorsProducer := agentadapter.CohorsPrincipal("cohors-worker-1")
	codexConsumer := agentadapter.CodexPrincipal("codex-reviewer-1")
	handoff1 := harness.ConvertHandoff(cohorsProducer, codexConsumer, scope,
		"Review GORM Gen migration slice",
		[]string{"chose GORM Gen for typed DAO", "upgraded dbresolver to v1.6.2"},
		[]string{"waiting on gorm v1.31 compatibility check"})

	handoffID1, err := svc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath:    vault,
		From:         handoff1.FromPrincipal,
		To:           handoff1.ToPrincipal,
		Scope:        handoff1.Scope,
		Objective:    handoff1.Objective,
		Decisions:    handoff1.Decisions,
		Blockers:     handoff1.Blockers,
		Verification: []string{"go test ./internal/index"},
		FollowUps:    []string{"verify guard_test.go passes"},
	})
	if err != nil {
		t.Fatalf("cohors→codex handoff: %v", err)
	}
	if handoffID1 == "" {
		t.Fatal("expected handoff ID")
	}

	// 场景 2: Codex reviewer → Cohors worker (reverse handoff)
	handoff2 := harness.ConvertHandoff(codexConsumer, cohorsProducer, scope,
		"Code review feedback for GORM Gen slice",
		[]string{"approved migration approach"},
		[]string{})
	handoffID2, err := svc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath: vault,
		From:      handoff2.FromPrincipal,
		To:        handoff2.ToPrincipal,
		Scope:     handoff2.Scope,
		Objective: handoff2.Objective,
		Decisions: handoff2.Decisions,
	})
	if err != nil {
		t.Fatalf("codex to cohors handoff: %v", err)
	}
	if handoffID2 == "" {
		t.Fatal("expected second handoff ID")
	}

	// 验证两条 handoff 都持久化
	list, err := svc.AgentHandoffList(ctx, vault, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 handoffs, got %d", len(list))
	}

	// 验证 handoff 的 objective、decisions 和 blockers 可继续
	for _, h := range list {
		if h.Objective == "" {
			t.Error("handoff objective should not be empty")
		}
	}

	// 验证 handoff 不自动 confirm memory（handoff 是 bounded working state，不是 confirmed memory）
	memories, err := svc.AgentMemoryRecallQuery(ctx, vault, app.RecallQuery{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Errorf("handoff should NOT auto-confirm as memory; got %d memories", len(memories))
	}
}
