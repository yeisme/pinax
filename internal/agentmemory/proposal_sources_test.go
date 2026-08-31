package agentmemory

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestProposalSourcesRoundTrip(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	ctx := context.Background()
	proposal := agentprotocol.Proposal{
		SchemaVersion:  agentprotocol.SchemaVersion,
		ProposalID:     "proposal-source-roundtrip",
		Principal:      agentprotocol.DefaultAdapterPrincipal("adapter", "test"),
		Scope:          agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"},
		Kind:           agentprotocol.MemoryKindFact,
		Subject:        "source persistence",
		RequestedState: agentprotocol.LifecycleConfirmed,
		Sources: agentprotocol.SourceRefList{
			{Kind: agentprotocol.SourceKindNote, Ref: "note-1", Label: "Transcript", Span: "section:1"},
			{Kind: agentprotocol.SourceKindRepository, Ref: "docs/decision.md", Span: "rev:abc123"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.SaveProposal(ctx, proposal, agentprotocol.ProposalStatusApprovalRequired, agentprotocol.ReasonValid); err != nil {
		t.Fatal(err)
	}
	sources, err := store.GetProposalSources(ctx, proposal.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != len(proposal.Sources) {
		t.Fatalf("sources = %#v", sources)
	}
	for i := range sources {
		if sources[i] != proposal.Sources[i] {
			t.Fatalf("source[%d] = %#v, want %#v", i, sources[i], proposal.Sources[i])
		}
	}

	proposal.Sources = agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: "note-2"}}
	if err := store.SaveProposal(ctx, proposal, agentprotocol.ProposalStatusApprovalRequired, agentprotocol.ReasonValid); err != nil {
		t.Fatal(err)
	}
	sources, err = store.GetProposalSources(ctx, proposal.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Ref != "note-2" {
		t.Fatalf("replaced sources = %#v", sources)
	}
}
