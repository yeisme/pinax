package app

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func checkpointScopeFixture(t *testing.T) (svc *AgentMemoryService, vault string, scope agentprotocol.Scope) {
	t.Helper()
	vault = t.TempDir()
	if _, err := NewService().InitVault(context.Background(), InitVaultRequest{VaultPath: vault, Title: "Checkpoint Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	svc = NewAgentMemoryService()
	t.Cleanup(func() { _ = svc.Close() })
	scope = agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"}
	return svc, vault, scope
}

func validCheckpointReq(vault string, scope agentprotocol.Scope) ContinuityCheckpointRequest {
	return ContinuityCheckpointRequest{
		VaultPath:    vault,
		Principal:    agentPrincipalForTest(),
		Scope:        scope,
		Objective:    "finish continuity checkpoint facade",
		CurrentState: "caps validated, handoff write pending",
		Decisions:    []string{"reuse canonical handoff service"},
		Blockers:     []string{"waiting for e2e evidence"},
		Verification: []string{"go test ./internal/app"},
		FollowUps:    []string{"wire CLI subcommand"},
		Sources:      agentprotocol.SourceRefList{{Kind: "note", Ref: "design-checkpoint"}},
	}
}

func agentPrincipalForTest() agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal("local-cli", "pinax-cli")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}

// TestContinuityCheckpointCreatesBoundedHandoff 覆盖合法 checkpoint 主路径。
func TestContinuityCheckpointCreatesBoundedHandoff(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)

	result, err := svc.ContinuityCheckpoint(context.Background(), validCheckpointReq(vault, scope))
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if result.HandoffID == "" {
		t.Fatal("expected handoff ID")
	}
	if result.ConfirmedCreated != 0 {
		t.Fatalf("checkpoint must never create confirmed memory: %d", result.ConfirmedCreated)
	}
	handoff, err := svc.AgentHandoffGet(context.Background(), vault, result.HandoffID)
	if err != nil {
		t.Fatalf("load handoff: %v", err)
	}
	if handoff.Objective != "finish continuity checkpoint facade" {
		t.Fatalf("handoff objective mismatch: %q", handoff.Objective)
	}
	if len(handoff.Sources) != 1 {
		t.Fatalf("handoff sources not persisted: %#v", handoff.Sources)
	}
}

// TestContinuityCheckpointProposalOnlyGuard 覆盖 durable candidate 只产生 proposal。
// 没有显式 review approval 时 confirmed memory count 必须保持不变。
func TestContinuityCheckpointProposalOnlyGuard(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)

	before, err := svc.AgentMemoryStatus(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("status before: %v", err)
	}
	beforeConfirmed := before[agentprotocol.LifecycleConfirmed]

	req := validCheckpointReq(vault, scope)
	req.DurableCandidates = []DurableCandidate{
		{Kind: "decision", Subject: "checkpoint caps enforced before write", Summary: "caps must fail the whole checkpoint instead of truncating"},
		{Kind: "fact", Subject: "reuse handoff service for closeout", Summary: "checkpoint is an intent facade over agent handoff create"},
	}
	result, err := svc.ContinuityCheckpoint(context.Background(), req)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if len(result.ProposalIDs) != 2 {
		t.Fatalf("expected 2 proposals, got %v", result.ProposalIDs)
	}
	after, err := svc.AgentMemoryStatus(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("status after: %v", err)
	}
	if after[agentprotocol.LifecycleConfirmed] != beforeConfirmed {
		t.Fatalf("confirmed memory count changed: %d -> %d", beforeConfirmed, after[agentprotocol.LifecycleConfirmed])
	}
	proposals, err := svc.AgentMemoryListProposals(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 pending proposals, got %d", len(proposals))
	}
	for _, proposal := range proposals {
		if proposal.Status != string(agentprotocol.ProposalStatusApprovalRequired) {
			t.Fatalf("durable candidate must stay proposed, got status %q", proposal.Status)
		}
	}
}

// TestContinuityCheckpointLinksRecordedRun 固定 optional --run 的真实语义：
// checkpoint 必须把时间与 proposal 数写回既有 receipt，而不是只在输出中回显 run ID。
func TestContinuityCheckpointLinksRecordedRun(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)
	runID, err := svc.ContinuityRecordRun(context.Background(), ContinuityRunRecord{
		VaultPath: vault, Scope: scope, Runtime: "codex", TaskClass: "product_spec_docs",
		HandoffStatus: "missing",
	})
	if err != nil {
		t.Fatalf("record run: %v", err)
	}
	req := validCheckpointReq(vault, scope)
	req.RunID = runID
	req.DurableCandidates = []DurableCandidate{{
		Kind: "decision", Subject: "recorded checkpoint link", Summary: "link checkpoint evidence to the run receipt",
	}}
	result, err := svc.ContinuityCheckpoint(context.Background(), req)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if result.RunID != runID {
		t.Fatalf("checkpoint run id = %q, want %q", result.RunID, runID)
	}
	store, err := svc.continuityEvidenceStoreFor(vault)
	if err != nil {
		t.Fatalf("open evidence store: %v", err)
	}
	row, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get linked run: %v", err)
	}
	if row.CheckpointedAt == nil {
		t.Fatal("checkpointed_at must be recorded")
	}
	if row.ProposalCount != 1 {
		t.Fatalf("proposal_count = %d, want 1", row.ProposalCount)
	}
}

// TestContinuityCheckpointRejectsUnknownRunBeforeHandoff 防止伪造 link：
// 不存在的 run 必须在任何 handoff/proposal 写入前失败。
func TestContinuityCheckpointRejectsUnknownRunBeforeHandoff(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)
	req := validCheckpointReq(vault, scope)
	req.RunID = "crun_missing"
	if _, err := svc.ContinuityCheckpoint(context.Background(), req); err == nil {
		t.Fatal("unknown run link must fail")
	}
	handoffs, err := svc.AgentHandoffList(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("list handoffs: %v", err)
	}
	if len(handoffs) != 0 {
		t.Fatalf("unknown run must fail before handoff write: %d", len(handoffs))
	}
}

// TestContinuityCheckpointCapsRejectTranscriptDump 覆盖超限与 transcript dump 防护。
// 任何超限必须整体失败且不写入 handoff/proposal。
func TestContinuityCheckpointCapsRejectTranscriptDump(t *testing.T) {
	t.Parallel()
	svc, vault, scope := checkpointScopeFixture(t)
	huge := strings.Repeat("x", checkpointMaxSectionItemChar+1)

	cases := []struct {
		name   string
		mutate func(*ContinuityCheckpointRequest)
	}{
		{"oversize objective", func(r *ContinuityCheckpointRequest) { r.Objective = strings.Repeat("o", checkpointMaxObjectiveChars+1) }},
		{"oversize state", func(r *ContinuityCheckpointRequest) { r.CurrentState = strings.Repeat("s", checkpointMaxStateChars+1) }},
		{"too many blockers", func(r *ContinuityCheckpointRequest) { r.Blockers = make([]string, checkpointMaxSectionItems+1) }},
		{"oversize blocker item", func(r *ContinuityCheckpointRequest) { r.Blockers = []string{huge} }},
		{"too many sources", func(r *ContinuityCheckpointRequest) {
			r.Sources = make(agentprotocol.SourceRefList, checkpointMaxSources+1)
		}},
		{"invalid source ref", func(r *ContinuityCheckpointRequest) { r.Sources = agentprotocol.SourceRefList{{Kind: "", Ref: "x"}} }},
		{"too many durable candidates", func(r *ContinuityCheckpointRequest) {
			r.DurableCandidates = make([]DurableCandidate, checkpointMaxDurableItems+1)
		}},
		{"oversize durable subject", func(r *ContinuityCheckpointRequest) {
			r.DurableCandidates = []DurableCandidate{{Kind: "decision", Subject: huge, Summary: "ok"}}
		}},
		{"unknown durable kind", func(r *ContinuityCheckpointRequest) {
			r.DurableCandidates = []DurableCandidate{{Kind: "guess", Subject: "bounded subject", Summary: "bounded summary"}}
		}},
		{"empty durable summary", func(r *ContinuityCheckpointRequest) {
			r.DurableCandidates = []DurableCandidate{{Kind: "decision", Subject: "bounded subject", Summary: "  "}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validCheckpointReq(vault, scope)
			tc.mutate(&req)
			_, err := svc.ContinuityCheckpoint(context.Background(), req)
			se, ok := err.(*agentprotocol.StableError)
			if !ok || se.Code != agentprotocol.ErrCodeValidationFailed {
				t.Fatalf("checkpoint cap violation must fail validation: %#v", err)
			}
		})
	}

	// 全部失败路径不得产生任何 handoff/proposal 写入。
	handoffs, err := svc.AgentHandoffList(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("list handoffs: %v", err)
	}
	if len(handoffs) != 0 {
		t.Fatalf("failed checkpoint must not write handoffs: %d", len(handoffs))
	}
	proposals, err := svc.AgentMemoryListProposals(context.Background(), vault, scope)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("failed checkpoint must not write proposals: %d", len(proposals))
	}
}
