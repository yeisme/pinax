package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

func testAgentMemoryService(t *testing.T) (*AgentMemoryService, string) {
	t.Helper()
	svc := NewAgentMemoryService()
	vault := t.TempDir()
	t.Cleanup(func() {
		_ = svc.closeStore(vault)
	})
	return svc, vault
}

func ownerPrincipal() agentprotocol.Principal {
	return agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion,
		PrincipalID:   "owner_test",
		Trust:         agentprotocol.TrustLevelOwner,
		Capabilities:  []agentprotocol.Capability{agentprotocol.CapabilityApprove, agentprotocol.CapabilityConfirm},
	}
}

func adapterPrincipal() agentprotocol.Principal {
	return agentprotocol.DefaultAdapterPrincipal("adapter_test", "codex")
}

func TestAgentMemory_ProposeAndApprove(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_app"}

	// adapter proposes
	facts, review, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapterPrincipal(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindDecision,
		Subject:   "Use GORM Gen",
		Summary:   "Migrate index to typed DAO",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "note_gorm"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != agentprotocol.ProposalStatusApprovalRequired {
		t.Fatalf("expected approval_required, got %s (reason=%s)", review.Status, review.Reason)
	}
	proposalID := facts.ProposalID

	// owner approves
	approveFacts, err := svc.AgentMemoryApprove(ctx, vault, proposalID, ownerPrincipal())
	if err != nil {
		t.Fatal(err)
	}
	if approveFacts.LifecycleTo != agentprotocol.LifecycleConfirmed {
		t.Fatalf("expected confirmed, got %s", approveFacts.LifecycleTo)
	}
	if approveFacts.MemoryID == "" {
		t.Fatal("expected non-empty memory_id")
	}

	// recall should find it
	results, err := svc.AgentMemoryRecall(ctx, vault, agentmemory.RecallQuery{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 memory after approve, got %d", len(results))
	}
}

func TestAgentMemory_ProposeUnsourced(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_unsourced"}

	facts, review, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapterPrincipal(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "unsourced fact",
		// no sources
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.Reason != agentprotocol.ReasonUnsourced {
		t.Fatalf("expected unsourced reason, got %s", review.Reason)
	}
	if facts.Status != agentprotocol.ProposalStatusApprovalRequired {
		t.Fatalf("unsourced should be approval_required, got %s", facts.Status)
	}
}

func TestAgentMemory_ProposeDuplicate(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_dup"}

	// First proposal → approve
	facts1, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindFact, Subject: "dup subject", Object: "answer",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AgentMemoryApprove(ctx, vault, facts1.ProposalID, ownerPrincipal()); err != nil {
		t.Fatal(err)
	}

	// Second identical proposal → rejected as duplicate
	_, review2, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindFact, Subject: "dup subject", Object: "answer",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if review2.Status != agentprotocol.ProposalStatusRejected || review2.Reason != agentprotocol.ReasonDuplicate {
		t.Fatalf("expected duplicate rejection, got %+v", review2)
	}
}

func TestAgentMemory_ProposeConflict(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_conf"}

	// First: "answer A"
	facts1, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindFact, Subject: "conflict subject", Object: "answer_a",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AgentMemoryApprove(ctx, vault, facts1.ProposalID, ownerPrincipal()); err != nil {
		t.Fatal(err)
	}

	// Second: same subject but different object → conflict
	_, review2, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindFact, Subject: "conflict subject", Object: "answer_b",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if review2.Status != agentprotocol.ProposalStatusConflictRequired {
		t.Fatalf("expected conflict_required, got %+v", review2)
	}
}

func TestAgentMemory_AdapterCannotApprove(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_perm"}

	facts, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindFact, Subject: "perm test",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// adapter tries to approve own proposal → should fail
	_, err = svc.AgentMemoryApprove(ctx, vault, facts.ProposalID, adapterPrincipal())
	if err == nil {
		t.Fatal("adapter should not be able to approve")
	}
}

func TestAgentHandoff_CreateAndList(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_h"}

	from := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "p_cohors", Runtime: "cohors",
		Trust:        agentprotocol.TrustLevelAdapter,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose, agentprotocol.CapabilityHandoff, agentprotocol.CapabilityFeedback},
	}
	to := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "p_codex", Runtime: "codex",
		Trust:        agentprotocol.TrustLevelAdapter,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose},
	}

	handoffID, err := svc.AgentHandoffCreate(ctx, AgentHandoffCreateRequest{
		VaultPath: vault, From: from, To: to, Scope: scope,
		Objective: "Review GORM Gen migration",
		Decisions: []string{"chose Gen over raw SQL"},
		Blockers:  []string{"waiting on dbresolver upgrade"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if handoffID == "" {
		t.Fatal("expected non-empty handoff_id")
	}

	list, err := svc.AgentHandoffList(ctx, vault, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].HandoffID != handoffID {
		t.Fatalf("handoff list mismatch: %+v", list)
	}
}

func TestAgentFeedback_AddAndList(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_fb"}

	fbPrincipal := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "p_fb", Runtime: "codex",
		Trust:        agentprotocol.TrustLevelAdapter,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose, agentprotocol.CapabilityFeedback},
	}

	feedbackID, err := svc.AgentFeedbackAdd(ctx, AgentFeedbackAddRequest{
		VaultPath: vault, Principal: fbPrincipal, Scope: scope,
		Kind: agentprotocol.FeedbackUseful, MemoryID: "mem_x",
		Comment: "great context for the task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if feedbackID == "" {
		t.Fatal("expected non-empty feedback_id")
	}

	list, err := svc.AgentFeedbackList(ctx, vault, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].FeedbackID != feedbackID {
		t.Fatalf("feedback list mismatch: %+v", list)
	}
}

func TestAgentMemory_LifecycleOperations(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_lc"}
	owner := ownerPrincipal()

	// Create two confirmed memories
	var memIDs []string
	for _, obj := range []string{"v1", "v2"} {
		facts, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
			VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
			Kind: agentprotocol.MemoryKindFact, Subject: "lc subject", Object: obj,
			Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: obj}},
		})
		if err != nil {
			t.Fatal(err)
		}
		af, err := svc.AgentMemoryApprove(ctx, vault, facts.ProposalID, owner)
		if err != nil {
			t.Fatal(err)
		}
		memIDs = append(memIDs, af.MemoryID)
	}

	// Status check
	counts, err := svc.AgentMemoryStatus(ctx, vault, scope)
	if err != nil {
		t.Fatal(err)
	}
	if counts[agentprotocol.LifecycleConfirmed] != 2 {
		t.Fatalf("expected 2 confirmed, got %d", counts[agentprotocol.LifecycleConfirmed])
	}

	// Expire one
	if err := svc.AgentMemoryExpire(ctx, vault, memIDs[0], owner); err != nil {
		t.Fatal(err)
	}
	counts, _ = svc.AgentMemoryStatus(ctx, vault, scope)
	if counts[agentprotocol.LifecycleExpired] != 1 {
		t.Fatalf("expected 1 expired, got %d", counts[agentprotocol.LifecycleExpired])
	}
}

func TestAgentMemoryService_ProposalReviewFields(t *testing.T) {
	// Verify the review struct is properly populated
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_test"}

	facts, review, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
		Kind: agentprotocol.MemoryKindDecision, Subject: "review test",
		Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if facts.ProposalID == "" || review == nil {
		t.Fatal("expected non-empty proposal id and review")
	}

	// List proposals
	list, err := svc.AgentMemoryListProposals(ctx, vault, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(list))
	}

	// Show proposal
	shown, err := svc.AgentMemoryShowProposal(ctx, vault, facts.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if shown.ProposalID != facts.ProposalID {
		t.Fatalf("show mismatch: %s", shown.ProposalID)
	}

	// Verify vault path is resolved correctly
	abs, _ := filepath.Abs(vault)
	_ = abs // just ensure no panic
}
