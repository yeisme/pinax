package agentcontext

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

func testCompiler(t *testing.T) (*Compiler, *agentmemory.Store) {
	t.Helper()
	db := testDB(t)
	store, err := agentmemory.OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return NewCompiler(store, agentmemory.DefaultPolicy()), store
}

func TestCompile_PermissionDenied(t *testing.T) {
	c, _ := testCompiler(t)
	// principal without read capability
	p := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "p_noread",
		Trust: agentprotocol.TrustLevelAdapter,
	}
	_, err := c.Compile(context.Background(), agentprotocol.ContextRequest{
		Principal: p,
		Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "p1"},
	})
	if err == nil {
		t.Fatal("principal without read should be denied")
	}
}

func TestCompile_BasicPack(t *testing.T) {
	c, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_ctx"}
	now := time.Now().UTC()

	// Seed different kinds
	records := []agentprotocol.MemoryRecord{
		{ID: "ctx1", Kind: agentprotocol.MemoryKindFact, State: agentprotocol.LifecycleConfirmed, Subject: "GORM Gen fact", Summary: "Gen generates typed DAO", Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1"},
		{ID: "ctx2", Kind: agentprotocol.MemoryKindDecision, State: agentprotocol.LifecycleConfirmed, Subject: "Use GORM Gen", Summary: "chose Gen for index", Confidence: agentprotocol.ConfidenceVerified, CreatorID: "p1"},
		{ID: "ctx3", Kind: agentprotocol.MemoryKindPreference, State: agentprotocol.LifecycleConfirmed, Subject: "prefer typed queries", Confidence: agentprotocol.ConfidenceMedium, CreatorID: "p1"},
		{ID: "ctx4", Kind: agentprotocol.MemoryKindFailure, State: agentprotocol.LifecycleConfirmed, Subject: "raw SQL caused SQL injection", Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1"},
	}
	for i := range records {
		records[i].SchemaVersion = agentprotocol.SchemaVersion
		records[i].Scope = scope
		records[i].CreatedAt = now
		records[i].UpdatedAt = now
		if err := store.SaveMemory(ctx, records[i]); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := c.Compile(ctx, agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:         scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(pack.Facts) != 1 {
		t.Errorf("expected 1 fact, got %d", len(pack.Facts))
	}
	if len(pack.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(pack.Decisions))
	}
	if len(pack.Preferences) != 1 {
		t.Errorf("expected 1 preference, got %d", len(pack.Preferences))
	}
	if len(pack.FailedAttempts) != 1 {
		t.Errorf("expected 1 failure, got %d", len(pack.FailedAttempts))
	}
}

func TestCompile_BudgetTruncation(t *testing.T) {
	c, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_budget"}
	now := time.Now().UTC()

	for i := 0; i < 20; i++ {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            "b" + string(rune('A'+i)),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope, State: agentprotocol.LifecycleConfirmed,
			Subject: "subject", Summary: "summary",
			Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := store.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := c.Compile(ctx, agentprotocol.ContextRequest{
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
		Budget:    agentprotocol.ContextBudget{MaxItems: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.EntryCount() > 5 {
		t.Errorf("expected <= 5 entries, got %d", pack.EntryCount())
	}
	if !pack.Truncated {
		t.Error("expected truncated=true with 20 memories and max_items=5")
	}
}

func TestCompile_DeterministicOutput(t *testing.T) {
	c, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_det"}
	now := time.Now().UTC()

	for _, id := range []string{"d1", "d2", "d3"} {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion, ID: id, Kind: agentprotocol.MemoryKindFact,
			Scope: scope, State: agentprotocol.LifecycleConfirmed,
			Subject: "det subject", Summary: "det summary",
			Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := store.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	req := agentprotocol.ContextRequest{
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
	}

	pack1, _ := c.Compile(ctx, req)
	pack2, _ := c.Compile(ctx, req)

	// Same input → same entry order
	if len(pack1.Facts) != len(pack2.Facts) {
		t.Fatalf("different fact counts: %d vs %d", len(pack1.Facts), len(pack2.Facts))
	}
	for i := range pack1.Facts {
		if pack1.Facts[i].MemoryID != pack2.Facts[i].MemoryID {
			t.Errorf("fact[%d] order differs: %s vs %s", i, pack1.Facts[i].MemoryID, pack2.Facts[i].MemoryID)
		}
	}
}

func TestCompile_NoBodyLeak(t *testing.T) {
	c, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_nobody"}
	now := time.Now().UTC()

	longBody := ""
	for i := 0; i < 1000; i++ {
		longBody += "x"
	}
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion, ID: "nb1", Kind: agentprotocol.MemoryKindFact,
		Scope: scope, State: agentprotocol.LifecycleConfirmed,
		Subject: "nb", Summary: longBody, // long summary
		Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}

	pack, err := c.Compile(ctx, agentprotocol.ContextRequest{
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Preview must be bounded
	if err := pack.AssertNoBody(300); err != nil {
		t.Fatalf("body leak: %v", err)
	}
}

func TestCompile_EntityBoost(t *testing.T) {
	c, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_entity"}
	now := time.Now().UTC()

	// Two facts: one matches entity, one doesn't
	for _, s := range []struct{ id, subj string }{
		{"e1", "GORM Gen is great"},
		{"e2", "unrelated topic"},
	} {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion, ID: s.id, Kind: agentprotocol.MemoryKindFact,
			Scope: scope, State: agentprotocol.LifecycleConfirmed,
			Subject: s.subj, Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := store.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := c.Compile(ctx, agentprotocol.ContextRequest{
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
		Entities:  []string{"GORM"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Facts) < 1 {
		t.Fatal("expected at least 1 fact")
	}
	// Entity-matched fact should rank first
	if pack.Facts[0].MemoryID != "e1" {
		t.Errorf("expected e1 first (entity match), got %s", pack.Facts[0].MemoryID)
	}
}
