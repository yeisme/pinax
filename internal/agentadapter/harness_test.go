package agentadapter

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestHarness_NegotiateCodexCompatible(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion, agentprotocol.ContextSchemaVersion})
	desc := CodexDescriptor()

	result, err := h.Negotiate(desc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compatible {
		t.Errorf("Codex should be compatible, reason=%s", result.Reason)
	}
	if result.NegotiatedVersion == "" {
		t.Error("expected negotiated version")
	}
}

func TestHarness_NegotiateCohorsCompatible(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion, agentprotocol.ContextSchemaVersion})
	desc := CohorsDescriptor()

	result, err := h.Negotiate(desc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose, agentprotocol.CapabilityHandoff})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compatible {
		t.Errorf("Cohors should be compatible, reason=%s", result.Reason)
	}
}

func TestHarness_NegotiateVersionMismatch(t *testing.T) {
	h := NewHarness([]string{"yeisme.agent_memory.v999"})
	desc := CodexDescriptor()

	_, err := h.Negotiate(desc, []agentprotocol.Capability{agentprotocol.CapabilityRead})
	if err == nil {
		t.Fatal("version mismatch should fail")
	}
	se, ok := err.(*agentprotocol.StableError)
	if !ok || se.Code != agentprotocol.ErrCodeAdapterUnavailable {
		t.Fatalf("expected adapter_unavailable, got %v", err)
	}
}

func TestHarness_NegotiateDegraded(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion})
	desc := CodexDescriptor()

	// Request capabilities where some but not all are met (read+propose+confirm).
	// Adapter has read+propose but not confirm → degraded (2 of 3).
	result, err := h.Negotiate(desc, []agentprotocol.Capability{agentprotocol.CapabilityRead, agentprotocol.CapabilityPropose, agentprotocol.CapabilityConfirm})
	if err != nil {
		t.Fatal(err)
	}
	if result.Compatible {
		t.Error("should not be fully compatible (missing confirm)")
	}
	if !result.Degraded {
		t.Error("should be degraded (2 of 3 capabilities met)")
	}
}

func TestHarness_RenderContextRequest(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion})
	principal := CodexPrincipal("codex-agent-1")
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"}

	req, err := h.RenderContextRequest(principal, scope, "review migration", map[string]string{"hook": "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Principal.PrincipalID != "codex-agent-1" {
		t.Errorf("principal = %s", req.Principal.PrincipalID)
	}
	if req.Task != "review migration" {
		t.Errorf("task = %s", req.Task)
	}
}

func TestHarness_ConvertProposal(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion})
	principal := CodexPrincipal("codex-propose")
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"}

	prop := h.ConvertProposal(principal, scope, agentprotocol.MemoryKindDecision,
		"Use GORM Gen", "typed DAO migration",
		agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}})

	if prop.Kind != agentprotocol.MemoryKindDecision {
		t.Errorf("kind = %s", prop.Kind)
	}
	if prop.Principal.Runtime != "codex" {
		t.Errorf("runtime = %s", prop.Principal.Runtime)
	}
}

func TestHarness_ConvertHandoff(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion})
	from := CohorsPrincipal("cohors-worker")
	to := CodexPrincipal("codex-reviewer")
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"}

	handoff := h.ConvertHandoff(from, to, scope, "Review GORM migration",
		[]string{"chose Gen"}, []string{"waiting on dbresolver"})

	if handoff.FromPrincipal.Runtime != "cohors" {
		t.Errorf("from runtime = %s", handoff.FromPrincipal.Runtime)
	}
	if handoff.ToPrincipal.Runtime != "codex" {
		t.Errorf("to runtime = %s", handoff.ToPrincipal.Runtime)
	}
	if len(handoff.Decisions) != 1 {
		t.Errorf("decisions = %d", len(handoff.Decisions))
	}
}

func TestHarness_CheckDegraded(t *testing.T) {
	h := NewHarness([]string{agentprotocol.SchemaVersion})
	desc := CodexDescriptor()

	status := h.CheckDegraded(context.Background(), desc, agentprotocol.NewStableError("test_err", "timeout"))
	if status.AdapterID != "codex-reference" {
		t.Errorf("adapter_id = %s", status.AdapterID)
	}
	if status.Action != "continue_without_adapter" {
		t.Errorf("action = %s", status.Action)
	}
}

func TestHarness_CodexAndCohorsShareCoreSchema(t *testing.T) {
	// 验证两个 reference adapter 共用 100% core schema
	codex := CodexDescriptor()
	cohors := CohorsDescriptor()

	// 相同的 supported versions
	if codex.SupportedVersions[0] != cohors.SupportedVersions[0] {
		t.Error("core schema versions should match")
	}

	// 相同的 core capabilities
	if len(codex.Capabilities) != len(cohors.Capabilities) {
		t.Error("core capabilities count should match")
	}
	codexCapSet := make(map[agentprotocol.Capability]bool)
	for _, c := range codex.Capabilities {
		codexCapSet[c] = true
	}
	for _, c := range cohors.Capabilities {
		if !codexCapSet[c] {
			t.Errorf("capability %s missing from Codex", c)
		}
	}

	// Metadata 可以不同（runtime-specific）
	if codex.Metadata["hook_version"] == cohors.Metadata["hook_version"] {
		t.Error("runtime-specific metadata should differ")
	}
}
