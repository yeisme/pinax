package agentcontinuity

import (
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func validPrincipal() agentprotocol.Principal {
	return agentprotocol.DefaultAdapterPrincipal("test-agent", "codex")
}

func validScope() agentprotocol.Scope {
	return agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"}
}

func TestContinuityRequest_Validate_OK(t *testing.T) {
	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     validPrincipal(),
		Scope:         validScope(),
		Budget:        DefaultBudget(),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContinuityRequest_Validate_MissingSchemaVersion(t *testing.T) {
	req := ContinuityRequest{
		Principal: validPrincipal(),
		Scope:     validScope(),
		Budget:    DefaultBudget(),
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for missing schema_version")
	}
}

func TestContinuityRequest_Validate_InvalidPrincipal(t *testing.T) {
	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     agentprotocol.Principal{}, // missing PrincipalID
		Scope:         validScope(),
		Budget:        DefaultBudget(),
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for invalid principal")
	}
}

func TestContinuityRequest_Validate_InvalidBudget(t *testing.T) {
	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     validPrincipal(),
		Scope:         validScope(),
		Budget:        ContinuityBudget{MaxItems: 0, MaxChars: 100},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for zero max_items")
	}
}

func TestContinuityRequest_Validate_UnknownOptionalMetadata(t *testing.T) {
	// Forward compatibility: unknown optional fields must not break validation
	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     validPrincipal(),
		Scope:         validScope(),
		Budget:        DefaultBudget(),
		Intent:        "some future field",
		HandoffID:     "handoff-123",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected error for optional metadata: %v", err)
	}
}

func TestSourceCoverage_ResolvableRatio(t *testing.T) {
	tests := []struct {
		sc     SourceCoverage
		expect float64
	}{
		{SourceCoverage{Total: 0}, 0},
		{SourceCoverage{Total: 10, Resolved: 9, Missing: 1}, 0.9},
		{SourceCoverage{Total: 4, Resolved: 4}, 1.0},
	}
	for _, tt := range tests {
		got := tt.sc.ResolvableRatio()
		if got != tt.expect {
			t.Errorf("ResolvableRatio() = %v, want %v", got, tt.expect)
		}
	}
}

func TestContinuityPack_SectionCount(t *testing.T) {
	pack := ContinuityPack{
		Sections: []ContinuitySection{
			{Kind: "decisions", Items: []string{"d1", "d2"}},
			{Kind: "blockers", Items: []string{"b1"}},
			{Kind: "empty"},
		},
	}
	if got := pack.SectionCount(); got != 3 {
		t.Errorf("SectionCount() = %d, want 3", got)
	}
}

func TestContinuityPack_AssertNoBody_OK(t *testing.T) {
	pack := ContinuityPack{
		Sections: []ContinuitySection{
			{Kind: "decisions", Items: []string{"short item"}},
		},
	}
	if err := pack.AssertNoBody(500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContinuityPack_AssertNoBody_BodyLeak(t *testing.T) {
	longItem := make([]byte, 600)
	for i := range longItem {
		longItem[i] = 'x'
	}
	pack := ContinuityPack{
		Sections: []ContinuitySection{
			{Kind: "decisions", Items: []string{string(longItem)}},
		},
	}
	if err := pack.AssertNoBody(500); err == nil {
		t.Fatal("expected body leak error")
	}
}

func TestContinuityPack_ExperimentalFlag(t *testing.T) {
	pack := ContinuityPack{
		SchemaVersion: ContinuitySchemaVersion,
		Experimental:  true,
	}
	if !pack.Experimental {
		t.Error("Experimental flag should be true")
	}
}
