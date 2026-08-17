package agentmemory

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testStore(t testing.TB) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_SaveAndGetMemory(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "mem_test1",
		Kind:          agentprotocol.MemoryKindDecision,
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"},
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       "Use GORM Gen for typed DAO",
		Confidence:    agentprotocol.ConfidenceHigh,
		CreatorID:     "p_owner",
		Sources: agentprotocol.SourceRefList{
			{Kind: "note", Ref: "note_abc", Label: "GORM Gen decision"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveMemory(ctx, m); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetMemory(ctx, "mem_test1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != m.ID || got.Kind != m.Kind || got.Subject != m.Subject {
		t.Fatalf("roundtrip mismatch: got=%+v", got)
	}
	if len(got.Sources) != 1 || got.Sources[0].Ref != "note_abc" {
		t.Fatalf("sources mismatch: %+v", got.Sources)
	}
	if got.Scope.Kind != agentprotocol.ScopeKindProject || got.Scope.ID != "proj_1" {
		t.Fatalf("scope mismatch: %+v", got.Scope)
	}
}

func TestStore_SaveMemoryTransactionRollback(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// 先保存一条合法 record
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "mem_ok",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_1"},
		State:         agentprotocol.LifecycleConfirmed,
		CreatorID:     "p_1",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}

	// 保存一条非法 record 应失败，不影响第一条
	bad := m
	bad.ID = "mem_bad"
	bad.Kind = "invalid_kind"
	if err := s.SaveMemory(ctx, bad); err == nil {
		t.Fatal("invalid record should fail")
	}

	// 第一条仍在
	if _, err := s.GetMemory(ctx, "mem_ok"); err != nil {
		t.Fatalf("first record should survive rollback: %v", err)
	}
}

func TestStore_ListMemoriesByScope(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_list"}

	for i, state := range []agentprotocol.LifecycleState{
		agentprotocol.LifecycleConfirmed,
		agentprotocol.LifecycleProposed,
		agentprotocol.LifecycleConfirmed,
	} {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            "mem_l" + string(rune('A'+i)),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         state,
			CreatorID:     "p_1",
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
		if err := s.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// 全部
	all, err := s.ListMemories(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(all))
	}

	// 只 confirmed
	confirmed, err := s.ListMemories(ctx, scope, agentprotocol.LifecycleConfirmed)
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed) != 2 {
		t.Fatalf("expected 2 confirmed, got %d", len(confirmed))
	}
}

func TestStore_SaveAndListProposals(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_p"}

	p := agentprotocol.Proposal{
		SchemaVersion:  agentprotocol.SchemaVersion,
		ProposalID:     "prop_1",
		Principal:      agentprotocol.DefaultAdapterPrincipal("p_codex", "codex"),
		Scope:          scope,
		Kind:           agentprotocol.MemoryKindDecision,
		RequestedState: agentprotocol.LifecycleConfirmed,
		Sources:        agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.SaveProposal(ctx, p, agentprotocol.ProposalStatusApprovalRequired, agentprotocol.ReasonValid); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListProposals(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ProposalID != "prop_1" {
		t.Fatalf("proposal list mismatch: %+v", list)
	}

	// 更新 status
	if err := s.UpdateProposalStatus(ctx, "prop_1", agentprotocol.ProposalStatusApproved, agentprotocol.ReasonValid, "mem_from_prop"); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListProposals(ctx, scope)
	if list[0].Status != string(agentprotocol.ProposalStatusApproved) || list[0].ResultingMemoryID != "mem_from_prop" {
		t.Fatalf("update mismatch: %+v", list[0])
	}
}

func TestStore_SaveAndListHandoffs(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_h"}

	h := agentprotocol.Handoff{
		SchemaVersion: agentprotocol.HandoffSchemaVersion,
		HandoffID:     "h_1",
		FromPrincipal: agentprotocol.DefaultAdapterPrincipal("p_cohors", "cohors"),
		ToPrincipal:   agentprotocol.DefaultAdapterPrincipal("p_codex", "codex"),
		Scope:         scope,
		Objective:     "Review slice",
		Decisions:     []string{"chose GORM"},
		Sources:       agentprotocol.SourceRefList{{Kind: "note", Ref: "notes/pinax-slice.md"}},
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.SaveHandoff(ctx, h); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListHandoffs(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].HandoffID != "h_1" {
		t.Fatalf("handoff list mismatch: %+v", list)
	}
	if len(list[0].Sources) != 1 || list[0].Sources[0].Ref != "notes/pinax-slice.md" {
		t.Fatalf("handoff sources mismatch: %+v", list[0].Sources)
	}
}

func TestStore_SaveAndListFeedback(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_f"}

	f := agentprotocol.Feedback{
		SchemaVersion: agentprotocol.SchemaVersion,
		FeedbackID:    "fb_1",
		Principal:     agentprotocol.DefaultAdapterPrincipal("p_1", "codex"),
		Scope:         scope,
		Kind:          agentprotocol.FeedbackUseful,
		MemoryID:      "mem_1",
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.SaveFeedback(ctx, f); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListFeedback(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].FeedbackID != "fb_1" {
		t.Fatalf("feedback list mismatch: %+v", list)
	}
}

func TestStore_SavePrincipal(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	p := agentprotocol.DefaultAdapterPrincipal("p_codex_1", "codex")
	if err := s.SavePrincipal(ctx, p); err != nil {
		t.Fatal(err)
	}

	// 更新已存在的
	p.WorkspaceID = "ws_new"
	if err := s.SavePrincipal(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func TestStore_OpenWithFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "mem_file",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindOwner, ID: "u_1"},
		State:         agentprotocol.LifecycleConfirmed,
		CreatorID:     "p_1",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}
}
