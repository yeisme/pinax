package agentcontinuity

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// TestCompile_DegradedContext verifies that when context compilation fails
// (here: principal lacks read capability), the orchestrator returns a bounded
// partial pack instead of leaking the raw error.
func TestCompile_DegradedContext(t *testing.T) {
	o, _ := testContinuityOrchestrator(t)
	ctx := context.Background()
	scope := validScope()

	// A principal without read capability causes the context compiler to
	// return an insufficient_scope error, triggering the degraded path.
	noRead := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion,
		PrincipalID:   "p_noread",
		Trust:         agentprotocol.TrustLevelAdapter,
	}

	pack, err := o.Compile(ctx, continuityReq(noRead, scope))
	if err != nil {
		t.Fatalf("degraded compile should return a partial pack, not an error: %v", err)
	}

	if pack.SchemaVersion != ContinuitySchemaVersion {
		t.Errorf("schema version = %q, want %q", pack.SchemaVersion, ContinuitySchemaVersion)
	}
	if !pack.Experimental {
		t.Error("degraded pack should be marked experimental")
	}
	if len(pack.Sections) != 0 {
		t.Errorf("degraded pack should have no sections, got %d", len(pack.Sections))
	}
	if pack.SectionCount() != 0 {
		t.Errorf("degraded pack section count = %d, want 0", pack.SectionCount())
	}

	// The degraded pack must offer a retry drill-down.
	hasRetry := false
	for _, a := range pack.NextActions {
		if strings.Contains(strings.ToLower(a.Name), "retry") {
			hasRetry = true
			break
		}
	}
	if !hasRetry {
		t.Errorf("degraded pack should include a retry next action; got %+v", pack.NextActions)
	}
}

// TestCompile_EmptyScope verifies that an invalid request (empty scope ID)
// is rejected with an error rather than producing a pack.
func TestCompile_EmptyScope(t *testing.T) {
	o, _ := testContinuityOrchestrator(t)
	ctx := context.Background()

	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     validPrincipal(),
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: ""},
		Budget:        DefaultBudget(),
	}

	pack, err := o.Compile(ctx, req)
	if err == nil {
		t.Fatal("expected error for invalid (empty) scope, got nil")
	}
	// On validation failure the pack must be zero-valued.
	if pack.SchemaVersion != "" {
		t.Errorf("zero-valued pack expected on validation failure, got schema=%q", pack.SchemaVersion)
	}
}
