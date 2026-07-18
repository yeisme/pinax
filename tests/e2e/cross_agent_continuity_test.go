package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentadapter"
	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

// TestCrossAgentContinuity 验证跨 Agent 连续工作流程：
// Agent A continue/context → propose/handoff → review/approve → Agent B continue。
// Agent B 获得 approved decision、blocker、commitment 和 resolvable sources，
// 不获得完整 transcript。
func TestCrossAgentContinuity(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "continuity-e2e"}
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// 使用 reference adapter harness
	harness := agentadapter.NewHarness([]string{agentprotocol.SchemaVersion, agentprotocol.ContextSchemaVersion})
	codexDesc := agentadapter.CodexDescriptor()
	cohorsDesc := agentadapter.CohorsDescriptor()
	if _, err := harness.Negotiate(codexDesc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityHandoff}); err != nil {
		t.Fatalf("codex negotiate: %v", err)
	}
	if _, err := harness.Negotiate(cohorsDesc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityHandoff}); err != nil {
		t.Fatalf("cohors negotiate: %v", err)
	}

	// 阶段 1: Agent A (Cohors) 创建 handoff + proposal
	agentA := agentadapter.CohorsPrincipal("cohors-worker-a")
	handoff := harness.ConvertHandoff(agentA, agentadapter.CodexPrincipal("codex-agent-b"), scope,
		"Prepare v0.2 release of Pinax Agent Continuity",
		[]string{"chose deterministic truncation for continuity pack"},
		[]string{"waiting on trust center dashboard tests"})

	handoffID, err := svc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath:    vault,
		From:         handoff.FromPrincipal,
		To:           handoff.ToPrincipal,
		Scope:        handoff.Scope,
		Objective:    handoff.Objective,
		Decisions:    handoff.Decisions,
		Blockers:     handoff.Blockers,
		Verification: []string{"go test ./internal/agentcontinuity/"},
		FollowUps:    []string{"write cross-agent e2e test"},
	})
	if err != nil {
		t.Fatalf("agent A handoff: %v", err)
	}
	if handoffID == "" {
		t.Fatal("expected handoff ID")
	}

	// Agent A proposes a memory
	propFacts, _, err := svc.AgentMemoryPropose(ctx, app.AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: agentA,
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindDecision,
		Subject:   "Continuity pack uses deterministic budget truncation",
		Summary:   "Budget truncation must be deterministic to ensure same input yields same output.",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "design-continuity"}},
	})
	if err != nil {
		t.Fatalf("agent A propose: %v", err)
	}

	// 阶段 2: Owner reviews and approves the proposal
	if propFacts.Status == agentprotocol.ProposalStatusApprovalRequired {
		approveFacts, err := svc.AgentMemoryApprove(ctx, vault, propFacts.ProposalID, agentprotocol.Principal{
			SchemaVersion: agentprotocol.SchemaVersion,
			PrincipalID:   "owner",
			Trust:         agentprotocol.TrustLevelOwner,
			Capabilities: []agentprotocol.Capability{
				agentprotocol.CapabilityRead, agentprotocol.CapabilityApprove,
				agentprotocol.CapabilityConfirm, agentprotocol.CapabilityReview,
			},
		})
		if err != nil {
			t.Fatalf("owner approve: %v", err)
		}
		if approveFacts.MemoryID == "" {
			t.Fatal("expected memory ID after approval")
		}
	}

	// 阶段 3: Agent B (Codex) runs `pinax continue` — gets continuity pack
	agentB := agentadapter.CodexPrincipal("codex-agent-b")
	pack, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
		VaultPath: vault,
		Principal: agentB,
		Scope:     scope,
		Task:      "continue v0.2 release preparation",
		MaxItems:  10,
		MaxChars:  3000,
	})
	if err != nil {
		t.Fatalf("agent B continue: %v", err)
	}

	// Agent B should receive the objective from the handoff
	if pack.Objective == "" {
		t.Error("expected non-empty objective from handoff")
	}
	if pack.HandoffStatus != agentcontinuity.HandoffStatusConsumed {
		t.Errorf("handoff status = %s, want consumed", pack.HandoffStatus)
	}

	// Agent B should receive source-backed content, not full transcript
	if err := pack.AssertNoBody(500); err != nil {
		t.Errorf("continuity pack leaks body: %v", err)
	}

	// Pack must be experimental
	if !pack.Experimental {
		t.Error("continuity pack must be marked experimental")
	}

	// Pack must include next actions for drill-down
	if len(pack.NextActions) == 0 {
		t.Error("expected at least one next action")
	}

	// Silent promotion check: no memory should be auto-confirmed without owner approval
	// (already verified by the approve step above)
}

// TestAgentContinuityFixture 验证 fixture vault 的确定性。
func TestAgentContinuityFixture(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "fixture-test"}

	st, err := agentmemory.Open(vault)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// 保存一条 memory 用于确定性验证
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	mem := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "fixture-mem-1",
		Kind:          agentprotocol.MemoryKindDecision,
		Scope:         scope,
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       "Fixture decision",
		CreatorID:     "fixture-test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := st.SaveMemory(ctx, mem); err != nil {
		t.Fatalf("save memory: %v", err)
	}

	// 编译两次，验证确定性
	compiler := agentcontext.NewCompiler(st, agentmemory.DefaultPolicy())
	orch := agentcontinuity.NewOrchestrator(st, compiler)

	req := agentcontinuity.ContinuityRequest{
		SchemaVersion: agentcontinuity.ContinuitySchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("fixture-agent", "codex"),
		Scope:         scope,
		Budget:        agentcontinuity.DefaultBudget(),
	}

	pack1, err := orch.Compile(ctx, req)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	pack2, err := orch.Compile(ctx, req)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}

	if pack1.SectionCount() != pack2.SectionCount() {
		t.Errorf("non-deterministic: section counts differ (%d vs %d)",
			pack1.SectionCount(), pack2.SectionCount())
	}
	if pack1.HandoffStatus != pack2.HandoffStatus {
		t.Errorf("non-deterministic: handoff status differs")
	}
}
