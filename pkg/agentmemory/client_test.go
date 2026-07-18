package agentmemory

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestClient_CompileContext_Empty(t *testing.T) {
	client := NewClient()
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	vault := t.TempDir()

	pack, err := client.CompileContext(ctx, vault,
		agentprotocol.DefaultAdapterPrincipal("sdk-test", "test"),
		agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"},
		nil, 10, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if pack.SchemaVersion != agentprotocol.ContextSchemaVersion {
		t.Errorf("schema = %s", pack.SchemaVersion)
	}
}

func TestClient_ProposeAndApprove(t *testing.T) {
	client := NewClient()
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}

	facts, _, err := client.Propose(ctx, vault,
		agentprotocol.DefaultAdapterPrincipal("sdk-adapter", "codex"),
		scope, agentprotocol.MemoryKindFact, "SDK test", "via SDK",
		agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}})
	if err != nil {
		t.Fatal(err)
	}
	if facts.Status != agentprotocol.ProposalStatusApprovalRequired {
		t.Fatalf("expected approval_required, got %s", facts.Status)
	}

	owner := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "sdk-owner",
		Trust:        agentprotocol.TrustLevelOwner,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityApprove},
	}
	approveFacts, err := client.Approve(ctx, vault, facts.ProposalID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if approveFacts.LifecycleTo != agentprotocol.LifecycleConfirmed {
		t.Fatalf("expected confirmed, got %s", approveFacts.LifecycleTo)
	}

	// Recall should find it
	results, err := client.Recall(ctx, vault, scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(results))
	}
}

func TestClient_HandoffAndFeedback(t *testing.T) {
	client := NewClient()
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	vault := t.TempDir()

	from := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "sdk-from",
		Trust:        agentprotocol.TrustLevelAdapter,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityHandoff},
	}
	to := agentprotocol.DefaultAdapterPrincipal("sdk-to", "codex")

	handoffID, err := client.CreateHandoff(ctx, HandoffRequest{
		VaultPath: vault, From: from, To: to,
		Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"},
		Objective: "SDK handoff test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if handoffID == "" {
		t.Fatal("expected handoff_id")
	}

	fbPrincipal := agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion, PrincipalID: "sdk-fb",
		Trust:        agentprotocol.TrustLevelAdapter,
		Capabilities: []agentprotocol.Capability{agentprotocol.CapabilityFeedback},
	}
	feedbackID, err := client.AddFeedback(ctx, vault, fbPrincipal,
		agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"},
		agentprotocol.FeedbackUseful, "mem1", "useful")
	if err != nil {
		t.Fatal(err)
	}
	if feedbackID == "" {
		t.Fatal("expected feedback_id")
	}
}

func TestClient_Version(t *testing.T) {
	client := NewClient()
	defer func() { _ = client.Close() }()
	v := client.Version()
	if v == "" {
		t.Fatal("expected non-empty version")
	}
}
