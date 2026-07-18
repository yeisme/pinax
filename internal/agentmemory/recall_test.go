package agentmemory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestStore_Recall(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_recall"}

	// Seed: 不同 kind/state/text 的 memories
	records := []agentprotocol.MemoryRecord{
		{ID: "r1", Kind: agentprotocol.MemoryKindFact, State: agentprotocol.LifecycleConfirmed, Subject: "GORM Gen generates typed DAO", CreatorID: "p1"},
		{ID: "r2", Kind: agentprotocol.MemoryKindDecision, State: agentprotocol.LifecycleConfirmed, Subject: "Use SQLite for local index", CreatorID: "p1"},
		{ID: "r3", Kind: agentprotocol.MemoryKindFact, State: agentprotocol.LifecycleProposed, Subject: "GORM Gen is experimental", CreatorID: "p1"},
		{ID: "r4", Kind: agentprotocol.MemoryKindTask, State: agentprotocol.LifecycleConfirmed, Subject: "Migrate index to Gen", CreatorID: "p1"},
	}
	now := time.Now().UTC()
	for i := range records {
		records[i].SchemaVersion = agentprotocol.SchemaVersion
		records[i].Scope = scope
		records[i].CreatedAt = now
		records[i].UpdatedAt = now
		if err := s.SaveMemory(ctx, records[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Default recall: only confirmed/conflicted
	got, err := s.Recall(ctx, RecallQuery{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 { // r1, r2, r4 (r3 is proposed)
		t.Fatalf("default recall: expected 3, got %d", len(got))
	}

	// Text search
	got, err = s.Recall(ctx, RecallQuery{Scope: scope, Text: "GORM"})
	if err != nil {
		t.Fatal(err)
	}
	// r1 (confirmed, subject has GORM) — r3 has GORM but is proposed, excluded by default
	if len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("text search 'GORM': expected r1, got %+v", got)
	}

	// Kind filter
	got, err = s.Recall(ctx, RecallQuery{Scope: scope, Kinds: []agentprotocol.MemoryKind{agentprotocol.MemoryKindDecision}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "r2" {
		t.Fatalf("kind filter: expected r2, got %+v", got)
	}

	// Explicit states (include proposed)
	got, err = s.Recall(ctx, RecallQuery{Scope: scope, States: []agentprotocol.LifecycleState{agentprotocol.LifecycleProposed}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "r3" {
		t.Fatalf("state filter proposed: expected r3, got %+v", got)
	}

	// Limit
	got, err = s.Recall(ctx, RecallQuery{Scope: scope, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("limit 1: expected 1, got %d", len(got))
	}
}

func TestStore_CountByScope(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_count"}
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            fmt.Sprintf("c%d", i),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         agentprotocol.LifecycleConfirmed,
			CreatorID:     "p1",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "c_prop",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         scope,
		State:         agentprotocol.LifecycleProposed,
		CreatorID:     "p1",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}

	counts, err := s.CountByScope(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if counts[agentprotocol.LifecycleConfirmed] != 3 {
		t.Errorf("confirmed count = %d, want 3", counts[agentprotocol.LifecycleConfirmed])
	}
	if counts[agentprotocol.LifecycleProposed] != 1 {
		t.Errorf("proposed count = %d, want 1", counts[agentprotocol.LifecycleProposed])
	}
}

func TestTableNames(t *testing.T) {
	names := TableNames()
	if len(names) != 7 {
		t.Fatalf("expected 7 tables, got %d", len(names))
	}
	for _, n := range names {
		if n == "" {
			t.Error("empty table name")
		}
	}
}

func TestWriteMigrationReceipt(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	receipt := MigrationReceipt{
		Action:     "auto_migrate",
		TableNames: TableNames(),
		Status:     "success",
	}
	if err := s.WriteMigrationReceipt(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
}
