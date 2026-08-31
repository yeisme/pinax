package app

import (
	"context"
	"slices"
	"testing"

	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// TestInlineAttentionRelevantProposal 覆盖 6.1 inline 提示：
// 与当前 objective 相关的 pending proposal 进入 bounded review attention
// （count + refs，无 body）；无关 item 不进入 inline。
func TestInlineAttentionRelevantProposal(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)

	// 相关 proposal：subject 与 objective token 重合。
	if _, _, err := svc.AgentMemoryPropose(context.Background(), AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: agentPrincipalForTest(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindDecision,
		Subject:   "checkpoint caps need owner review",
		Summary:   "caps must fail the whole checkpoint",
	}); err != nil {
		t.Fatalf("propose relevant: %v", err)
	}
	// 无关 proposal：同 vault 同 scope，但不影响当前 objective/decision/blocker。
	if _, _, err := svc.AgentMemoryPropose(context.Background(), AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: agentPrincipalForTest(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "gardening notes for the weekend",
		Summary:   "unrelated content",
	}); err != nil {
		t.Fatalf("propose unrelated: %v", err)
	}

	pack, err := svc.AgentContinuity(context.Background(), ContinuityRequest{
		VaultPath: vault,
		Principal: agentPrincipalForTest(),
		Scope:     scope,
		Task:      "review checkpoint caps",
	})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if pack.ReviewAttentionCount != 1 {
		t.Fatalf("review_attention_count = %d, want 1", pack.ReviewAttentionCount)
	}
	if pack.ReviewAttention == nil {
		t.Fatal("inline review attention missing")
	}
	if pack.ReviewAttention.Subject != "checkpoint caps need owner review" {
		t.Fatalf("inline attention picked wrong item: %#v", pack.ReviewAttention)
	}
	if len(pack.ReviewAttention.ReasonCodes) == 0 {
		t.Fatalf("inline attention must carry reason codes: %#v", pack.ReviewAttention)
	}
	if !slices.Contains(pack.WarningCodes, agentcontinuity.WarningReviewAttention) {
		t.Fatalf("inline attention warning missing: %v", pack.WarningCodes)
	}
}

// TestInlineAttentionUnrelatedProposalHidden 覆盖无关 item 不展示，
// 且继续留在 weekly inbox（不被删除或降级）。
func TestInlineAttentionUnrelatedProposalHidden(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)

	if _, _, err := svc.AgentMemoryPropose(context.Background(), AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: agentPrincipalForTest(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "gardening notes for the weekend",
		Summary:   "unrelated content",
	}); err != nil {
		t.Fatalf("propose: %v", err)
	}

	pack, err := svc.AgentContinuity(context.Background(), ContinuityRequest{
		VaultPath: vault,
		Principal: agentPrincipalForTest(),
		Scope:     scope,
		Task:      "ship the continuity dogfood contracts",
	})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if pack.ReviewAttentionCount != 0 || pack.ReviewAttention != nil {
		t.Fatalf("unrelated item must not enter inline attention: %#v", pack.ReviewAttention)
	}

	// item 仍在 inbox（weekly review 渠道）。
	inbox, err := svc.MemoryInbox(context.Background(), InboxRequest{VaultPath: vault, Scope: scope, Limit: 100})
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if inbox.TotalItems != 1 {
		t.Fatalf("unrelated item must remain in weekly inbox: %d", inbox.TotalItems)
	}
}
