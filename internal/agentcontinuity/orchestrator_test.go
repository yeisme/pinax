package agentcontinuity

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testContinuityStore opens a fresh in-memory sqlite agent memory store.
func testContinuityStore(t testing.TB) *agentmemory.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentmemory.OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// testContinuityOrchestrator wires a real store + compiler + orchestrator.
func testContinuityOrchestrator(t *testing.T) (*Orchestrator, *agentmemory.Store) {
	t.Helper()
	store := testContinuityStore(t)
	t.Cleanup(func() { _ = store.Close() })
	compiler := agentcontext.NewCompiler(store, agentmemory.DefaultPolicy())
	return NewOrchestrator(store, compiler), store
}

// continuityReq builds a minimal valid request with the default budget.
func continuityReq(principal agentprotocol.Principal, scope agentprotocol.Scope) ContinuityRequest {
	return ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     principal,
		Scope:         scope,
		Budget:        DefaultBudget(),
	}
}

// confirmedFact builds a confirmed high-confidence fact memory with a fixed
// timestamp so ranking is deterministic.
func confirmedFact(id, subject string, scope agentprotocol.Scope) agentprotocol.MemoryRecord {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            id,
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         scope,
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       subject,
		Confidence:    agentprotocol.ConfidenceHigh,
		CreatorID:     "test-agent",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func hasSection(pack ContinuityPack, kind string) bool {
	for _, s := range pack.Sections {
		if s.Kind == kind {
			return true
		}
	}
	return false
}

func hasActionNamed(pack ContinuityPack, needle string) bool {
	lower := strings.ToLower(needle)
	for _, a := range pack.NextActions {
		if strings.Contains(strings.ToLower(a.Name), lower) {
			return true
		}
	}
	return false
}

// --- Tests ---

func TestCompile_OK(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	if err := store.SaveMemory(ctx, confirmedFact("mem_ok_1", "GORM Gen produces typed DAO", scope)); err != nil {
		t.Fatal(err)
	}

	pack, err := o.Compile(ctx, continuityReq(validPrincipal(), scope))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if pack.SchemaVersion != ContinuitySchemaVersion {
		t.Errorf("schema version = %q, want %q", pack.SchemaVersion, ContinuitySchemaVersion)
	}
	if !pack.Experimental {
		t.Error("pack should be marked experimental")
	}
	if pack.SectionCount() == 0 {
		t.Fatal("expected at least one section item, got 0")
	}
	if !hasSection(pack, "fact") {
		t.Errorf("expected a fact section; got %+v", pack.Sections)
	}
	if pack.Scope != scope {
		t.Errorf("scope = %+v, want %+v", pack.Scope, scope)
	}
}

func TestCompile_HandoffSelection(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	h := agentprotocol.Handoff{
		SchemaVersion: agentprotocol.HandoffSchemaVersion,
		HandoffID:     "h_auto",
		FromPrincipal: agentprotocol.DefaultAdapterPrincipal("p_from", "codex"),
		ToPrincipal:   agentprotocol.DefaultAdapterPrincipal("p_to", "codex"),
		Scope:         scope,
		Objective:     "Ship continuity tests",
		CurrentState:  "Orchestrator tests in progress",
		Decisions:     []string{"Use bounded sections"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.SaveHandoff(ctx, h); err != nil {
		t.Fatal(err)
	}

	pack, err := o.Compile(ctx, continuityReq(validPrincipal(), scope))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if pack.HandoffStatus != HandoffStatusConsumed {
		t.Errorf("handoff status = %q, want %q", pack.HandoffStatus, HandoffStatusConsumed)
	}
	if pack.HandoffID != "h_auto" {
		t.Errorf("handoff id = %q, want %q", pack.HandoffID, "h_auto")
	}
	if pack.Objective != "Ship continuity tests" {
		t.Errorf("objective = %q, want %q", pack.Objective, "Ship continuity tests")
	}
	if pack.CurrentState != "Orchestrator tests in progress" {
		t.Errorf("current_state = %q, want %q", pack.CurrentState, "Orchestrator tests in progress")
	}
}

func TestCompile_ExplicitHandoff(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	h := agentprotocol.Handoff{
		SchemaVersion: agentprotocol.HandoffSchemaVersion,
		HandoffID:     "h_explicit",
		FromPrincipal: agentprotocol.DefaultAdapterPrincipal("p_from", "codex"),
		ToPrincipal:   agentprotocol.DefaultAdapterPrincipal("p_to", "codex"),
		Scope:         scope,
		Objective:     "Explicit handoff objective",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.SaveHandoff(ctx, h); err != nil {
		t.Fatal(err)
	}

	req := continuityReq(validPrincipal(), scope)
	req.HandoffID = "h_explicit"

	pack, err := o.Compile(ctx, req)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if pack.HandoffStatus != HandoffStatusExplicit {
		t.Errorf("handoff status = %q, want %q", pack.HandoffStatus, HandoffStatusExplicit)
	}
	if pack.HandoffID != "h_explicit" {
		t.Errorf("handoff id = %q, want %q", pack.HandoffID, "h_explicit")
	}
}

func TestCompile_MissingHandoff(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	// Save a memory but no handoffs.
	if err := store.SaveMemory(ctx, confirmedFact("mem_missing_1", "Some confirmed fact", scope)); err != nil {
		t.Fatal(err)
	}

	pack, err := o.Compile(ctx, continuityReq(validPrincipal(), scope))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if pack.HandoffStatus != HandoffStatusMissing {
		t.Errorf("handoff status = %q, want %q", pack.HandoffStatus, HandoffStatusMissing)
	}
	if pack.HandoffID != "" {
		t.Errorf("handoff id = %q, want empty", pack.HandoffID)
	}
}

func TestCompile_Truncation(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_trunc"}

	// Seed many facts with subjects large enough to exceed a tight char budget.
	for i := range 15 {
		subject := strings.Repeat("x", 50) + "-" + string(rune('A'+i))
		id := "mem_trunc_" + string(rune('A'+i))
		if err := store.SaveMemory(ctx, confirmedFact(id, subject, scope)); err != nil {
			t.Fatal(err)
		}
	}

	req := continuityReq(validPrincipal(), scope)
	req.Budget = ContinuityBudget{MaxItems: 5, MaxChars: 100, MaxSources: 15}

	pack, err := o.Compile(ctx, req)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if !pack.Truncated {
		t.Error("expected pack to be truncated under tight budget")
	}
	if pack.SectionCount() == 0 {
		t.Fatal("expected at least one surviving section item after truncation")
	}
	// No item should leak unbounded body text.
	if err := pack.AssertNoBody(500); err != nil {
		t.Errorf("body leak after truncation: %v", err)
	}
}

func TestCompile_NextActions(t *testing.T) {
	o, store := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	if err := store.SaveMemory(ctx, confirmedFact("mem_na_1", "Important confirmed fact", scope)); err != nil {
		t.Fatal(err)
	}

	pack, err := o.Compile(ctx, continuityReq(validPrincipal(), scope))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if len(pack.NextActions) == 0 {
		t.Fatal("expected next actions, got none")
	}
	// With entries present the pack should suggest reviewing proposals.
	if !hasActionNamed(pack, "review") {
		t.Errorf("expected a review-related next action; got %+v", pack.NextActions)
	}
	// The orchestrator always appends a get-context drill-down.
	if !hasActionNamed(pack, "context") {
		t.Errorf("expected a context-related next action; got %+v", pack.NextActions)
	}
}
