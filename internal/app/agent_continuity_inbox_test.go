package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestAgentContinuity_OK(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "continuity-app-test"}
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	pack, err := svc.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("test-agent", "codex"),
		Scope:     scope,
		Task:      "test continuity compilation",
		MaxItems:  10,
		MaxChars:  2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pack.Experimental {
		t.Error("pack must be experimental")
	}
	if pack.SchemaVersion == "" {
		t.Error("schema_version required")
	}
	// Fresh vault has no handoffs
	if pack.HandoffStatus != "missing" {
		t.Errorf("handoff status = %s, want missing", pack.HandoffStatus)
	}
}

func TestAgentContinuityResolvesNoteSourcesAgainstVault(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	writeAppFixture(t, filepath.Join(vault, "notes", "source.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_continuity_source\ntitle: Continuity Source\n---\n\n# Continuity Source\n")
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "continuity-source-resolution"}
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	from := adapterPrincipal()
	from.Capabilities = append(from.Capabilities, agentprotocol.CapabilityHandoff)
	_, err := svc.AgentHandoffCreate(ctx, AgentHandoffCreateRequest{
		VaultPath: vault,
		From:      from,
		To:        agentprotocol.DefaultAdapterPrincipal("agent-b", "codex"),
		Scope:     scope,
		Objective: "Continue with verified sources",
		Sources: agentprotocol.SourceRefList{
			{Kind: "note", Ref: "note_continuity_source"},
			{Kind: "note", Ref: "note_missing_source"},
		},
	})
	if err != nil {
		t.Fatalf("create handoff: %v", err)
	}

	pack, err := svc.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("agent-b", "codex"),
		Scope:     scope,
	})
	if err != nil {
		t.Fatalf("compile continuity: %v", err)
	}
	if pack.SourceCoverage.Total != 2 || pack.SourceCoverage.Resolved != 1 || pack.SourceCoverage.Missing != 1 {
		t.Fatalf("source coverage = %+v, want total=2 resolved=1 missing=1", pack.SourceCoverage)
	}
}

func TestAgentContinuity_InvalidPrincipal(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	_, err := svc.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: vault,
		Principal: agentprotocol.Principal{}, // invalid
		Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "x"},
	})
	if err == nil {
		t.Fatal("expected error for invalid principal")
	}
}

func TestMemoryInbox_OK(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "inbox-app-test"}
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	pack, err := svc.MemoryInbox(ctx, InboxRequest{
		VaultPath: vault,
		Scope:     scope,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pack.Experimental {
		t.Error("pack must be experimental")
	}
	// Fresh vault has no proposals
	if pack.TotalItems != 0 {
		t.Errorf("total items = %d, want 0 for fresh vault", pack.TotalItems)
	}
}

func TestMemoryInbox_InvalidScope(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	_, err := svc.MemoryInbox(ctx, InboxRequest{
		VaultPath: vault,
		Scope:     agentprotocol.Scope{Kind: "bogus", ID: "x"},
	})
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestMemoryInboxItemDetail_ReturnsAggregatedProposal(t *testing.T) {
	ctx := context.Background()
	svc, vault := testAgentMemoryService(t)
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "inbox-detail"}

	facts, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapterPrincipal(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindDecision,
		Subject:   "Keep review detail in the application service",
		Summary:   "CLI consumers should not rebuild the inbox projection",
		Sources:   agentprotocol.SourceRefList{{Kind: "note", Ref: "note_review_detail"}},
	})
	if err != nil {
		t.Fatalf("propose memory: %v", err)
	}

	item, err := svc.InboxItemDetail(ctx, InboxItemDetailRequest{
		VaultPath: vault,
		Scope:     scope,
		ItemID:    "prop-" + facts.ProposalID,
	})
	if err != nil {
		t.Fatalf("inbox item detail: %v", err)
	}
	if item.ProposalID != facts.ProposalID {
		t.Fatalf("proposal_id = %q, want %q", item.ProposalID, facts.ProposalID)
	}
	if item.Subject != "Keep review detail in the application service" {
		t.Fatalf("subject = %q", item.Subject)
	}
}

func TestMemoryInboxItemDetail_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	svc, vault := testAgentMemoryService(t)

	_, err := svc.InboxItemDetail(ctx, InboxItemDetailRequest{
		VaultPath: vault,
		Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "inbox-detail"},
		ItemID:    "prop-missing",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
}
