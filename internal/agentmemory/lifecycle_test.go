package agentmemory

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func testLifecycle(t *testing.T) (*LifecycleService, *Store) {
	t.Helper()
	s := testStore(t)
	return NewLifecycleService(s), s
}

func TestLifecycle_ValidateTransition(t *testing.T) {
	ls, _ := testLifecycle(t)
	if err := ls.ValidateTransition(agentprotocol.LifecycleProposed, agentprotocol.LifecycleConfirmed); err != nil {
		t.Fatal(err)
	}
	if err := ls.ValidateTransition(agentprotocol.LifecycleConfirmed, agentprotocol.LifecycleRejected); err == nil {
		t.Error("confirmed → rejected should fail")
	}
}

func TestLifecycle_ConfirmProposed(t *testing.T) {
	ls, s := testLifecycle(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Now().UTC()

	// 有 source 的 proposed 可以 confirm
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "lc1",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "p1"},
		State:         agentprotocol.LifecycleProposed,
		CreatorID:     "owner",
		Sources:       agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}
	confirmed, err := ls.ConfirmProposed(ctx, "lc1", now)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.State != agentprotocol.LifecycleConfirmed {
		t.Fatalf("state = %s", confirmed.State)
	}

	// 无 source 的不能 confirm
	m2 := m
	m2.ID = "lc2"
	m2.Sources = nil
	if err := s.SaveMemory(ctx, m2); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.ConfirmProposed(ctx, "lc2", now); err == nil {
		t.Error("unsourced memory should not confirm")
	}

	// 非 proposed 状态不能 confirm
	m3 := m
	m3.ID = "lc3"
	m3.State = agentprotocol.LifecycleRejected
	if err := s.SaveMemory(ctx, m3); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.ConfirmProposed(ctx, "lc3", now); err == nil {
		t.Error("rejected → confirmed should fail")
	}
}

func TestLifecycle_Supersede(t *testing.T) {
	ls, s := testLifecycle(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Now().UTC()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "p1"}

	old := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion, ID: "old1", Kind: agentprotocol.MemoryKindFact,
		Scope: scope, State: agentprotocol.LifecycleConfirmed, CreatorID: "p1",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
		CreatedAt: now, UpdatedAt: now,
	}
	replacement := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion, ID: "new1", Kind: agentprotocol.MemoryKindFact,
		Scope: scope, State: agentprotocol.LifecycleConfirmed, CreatorID: "p1",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "n2"}},
		CreatedAt: now, UpdatedAt: now,
	}
	for _, m := range []agentprotocol.MemoryRecord{old, replacement} {
		if err := s.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	if err := ls.Supersede(ctx, "old1", "new1", now); err != nil {
		t.Fatal(err)
	}

	gotOld, _ := s.GetMemory(ctx, "old1")
	if gotOld.State != agentprotocol.LifecycleSuperseded {
		t.Fatalf("old state = %s", gotOld.State)
	}
	gotNew, _ := s.GetMemory(ctx, "new1")
	if gotNew.SupersedesID != "old1" {
		t.Fatalf("new supersedes = %s", gotNew.SupersedesID)
	}
}

func TestLifecycle_MarkConflictedAndResolve(t *testing.T) {
	ls, s := testLifecycle(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Now().UTC()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "p1"}

	for _, id := range []string{"cf1", "cf2"} {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion, ID: id, Kind: agentprotocol.MemoryKindFact,
			Scope: scope, State: agentprotocol.LifecycleConfirmed, CreatorID: "p1",
			Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "n_" + id}},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// 标记冲突
	if err := ls.MarkConflicted(ctx, []string{"cf1", "cf2"}, "contradictory evidence", now); err != nil {
		t.Fatal(err)
	}
	cf1, _ := s.GetMemory(ctx, "cf1")
	if cf1.State != agentprotocol.LifecycleConflicted {
		t.Fatalf("cf1 state = %s", cf1.State)
	}
	if len(cf1.ConflictsWith) == 0 {
		t.Error("cf1 should have conflict refs")
	}

	// 解决冲突：cf1 赢
	if err := ls.ResolveConflict(ctx, "cf1", now); err != nil {
		t.Fatal(err)
	}
	cf1After, _ := s.GetMemory(ctx, "cf1")
	if cf1After.State != agentprotocol.LifecycleConfirmed {
		t.Fatalf("winner state = %s", cf1After.State)
	}
	cf2After, _ := s.GetMemory(ctx, "cf2")
	if cf2After.State != agentprotocol.LifecycleSuperseded {
		t.Fatalf("loser state = %s, want superseded", cf2After.State)
	}
}

func TestPolicy_CheckConfirm(t *testing.T) {
	p := DefaultPolicy()
	adapter := agentprotocol.DefaultAdapterPrincipal("p1", "codex")
	if err := p.CheckConfirm(adapter); err == nil {
		t.Error("adapter should not confirm")
	}

	owner := agentprotocol.Principal{PrincipalID: "po", Trust: agentprotocol.TrustLevelOwner}
	if err := p.CheckConfirm(owner); err != nil {
		t.Error("owner should confirm")
	}
}

func TestPolicy_EvaluateProposal(t *testing.T) {
	p := DefaultPolicy()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "p1"}

	// unsourced → approval_required
	unsourced := agentprotocol.Proposal{
		ProposalID: "pr1", Principal: agentprotocol.DefaultAdapterPrincipal("p1", "codex"),
		Scope: scope, Kind: agentprotocol.MemoryKindFact, Subject: "s1",
	}
	review := p.EvaluateProposal(unsourced, nil)
	if review.Status != agentprotocol.ProposalStatusApprovalRequired || review.Reason != agentprotocol.ReasonUnsourced {
		t.Fatalf("unsourced: %+v", review)
	}

	// valid with source → approval_required (default policy)
	valid := unsourced
	valid.ProposalID = "pr2"
	valid.Sources = agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}}
	review = p.EvaluateProposal(valid, nil)
	if review.Status != agentprotocol.ProposalStatusApprovalRequired {
		t.Fatalf("valid default policy: %+v", review)
	}

	// duplicate → rejected
	existing := []agentprotocol.MemoryRecord{
		{ID: "dup1", Scope: scope, Kind: agentprotocol.MemoryKindFact, Subject: "s1", State: agentprotocol.LifecycleConfirmed},
	}
	review = p.EvaluateProposal(valid, existing)
	if review.Status != agentprotocol.ProposalStatusRejected || review.Reason != agentprotocol.ReasonDuplicate {
		t.Fatalf("duplicate: %+v", review)
	}

	// conflict → conflict_required
	existingConflict := []agentprotocol.MemoryRecord{
		{ID: "cex1", Scope: scope, Kind: agentprotocol.MemoryKindFact, Subject: "s1", Object: "old_answer", State: agentprotocol.LifecycleConfirmed},
	}
	conflictProp := valid
	conflictProp.ProposalID = "pr3"
	conflictProp.Object = "new_answer"
	review = p.EvaluateProposal(conflictProp, existingConflict)
	if review.Status != agentprotocol.ProposalStatusConflictRequired {
		t.Fatalf("conflict: %+v", review)
	}

	// AllowAutoConfirm → approved
	autoP := Policy{AllowAutoConfirm: true}
	review = autoP.EvaluateProposal(valid, nil)
	if review.Status != agentprotocol.ProposalStatusApproved {
		t.Fatalf("auto-confirm: %+v", review)
	}
}
