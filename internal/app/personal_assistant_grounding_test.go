package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestPersonalAssistantGroundingFourStateFixture(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	noteSvc := NewService()
	if _, err := noteSvc.InitVault(ctx, InitVaultRequest{VaultPath: vault, Title: "Grounding"}); err != nil {
		t.Fatal(err)
	}
	created, err := noteSvc.CreateNote(ctx, CreateNoteRequest{VaultPath: vault, Title: "Open source", Body: "source body"})
	if err != nil {
		t.Fatal(err)
	}
	noteID := created.Facts["note_id"]

	memSvc := NewAgentMemoryService()
	t.Cleanup(func() { _ = memSvc.closeStore(vault) })
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
	proposal, _, err := memSvc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapterPrincipal(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "partially grounded fixture",
		Summary:   "bounded fixture",
		Sources: agentprotocol.SourceRefList{
			{Kind: agentprotocol.SourceKindNote, Ref: noteID},
			{Kind: agentprotocol.SourceKindNote, Ref: "note-missing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memSvc.AgentMemoryApprove(ctx, vault, proposal.ProposalID, ownerPrincipal()); err != nil {
		t.Fatal(err)
	}

	partial, err := memSvc.AgentContextForPersonalAssistant(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("personal-assistant", "test"),
		Scope:     scope,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertGroundingInvariant(t, partial.Grounding, AgentGroundingPartiallyGrounded, 2, 1)
	if len(partial.Pack.Sources) != 1 || partial.Pack.Sources[0].Ref != noteID {
		t.Fatalf("partial openable sources = %#v", partial.Pack.Sources)
	}

	grounded := groundingEvidence(agentGroundingResolution{
		Coverage: agentSourceCoverage(1, 1, 0, 0, 0),
	})
	assertGroundingInvariant(t, grounded, AgentGroundingGrounded, 1, 1)

	ungrounded := groundingEvidence(agentGroundingResolution{
		Coverage: agentSourceCoverage(2, 0, 2, 0, 0),
	})
	assertGroundingInvariant(t, ungrounded, AgentGroundingUngrounded, 0, 0)
	if ungrounded.CandidateSourceTotal != 2 {
		t.Fatalf("ungrounded candidate count = %#v", ungrounded)
	}

	unavailable := AgentGroundingUnavailableEvidence()
	assertGroundingInvariant(t, unavailable, AgentGroundingUnavailable, 0, 0)
}

func TestPersonalAssistantTranscriptDeleteRemovesReadonlyContent(t *testing.T) {
	t.Parallel()
	const sentinel = "TRANSCRIPT_DELETE_SENTINEL_74C9"
	ctx := context.Background()
	vault := t.TempDir()
	noteSvc := NewService()
	if _, err := noteSvc.InitVault(ctx, InitVaultRequest{VaultPath: vault, Title: "Archive"}); err != nil {
		t.Fatal(err)
	}
	created, err := noteSvc.CreateNote(ctx, CreateNoteRequest{
		VaultPath: vault,
		Title:     "Assistant transcript",
		Body:      "private transcript " + sentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	noteID := created.Facts["note_id"]
	if _, err := noteSvc.RebuildIndex(ctx, VaultRequest{VaultPath: vault}); err != nil {
		t.Fatal(err)
	}

	memSvc := NewAgentMemoryService()
	t.Cleanup(func() { _ = memSvc.closeStore(vault) })
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
	proposal, _, err := memSvc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
		VaultPath: vault,
		Principal: adapterPrincipal(),
		Scope:     scope,
		Kind:      agentprotocol.MemoryKindFact,
		Subject:   "transcript-derived fact",
		Summary:   "derived " + sentinel,
		Sources:   agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: noteID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memSvc.AgentMemoryApprove(ctx, vault, proposal.ProposalID, ownerPrincipal()); err != nil {
		t.Fatal(err)
	}

	from := agentprotocol.DefaultAdapterPrincipal("personal-assistant", "test")
	from.Capabilities = append(from.Capabilities, agentprotocol.CapabilityHandoff)
	if _, err := memSvc.AgentHandoffCreate(ctx, AgentHandoffCreateRequest{
		VaultPath:    vault,
		From:         from,
		To:           agentprotocol.DefaultAdapterPrincipal("next-agent", "test"),
		Scope:        scope,
		Objective:    "continue assistant work",
		CurrentState: "bounded state " + sentinel,
		Sources:      agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: noteID}},
	}); err != nil {
		t.Fatal(err)
	}

	beforeContext, beforeRecall, beforeHandoff := personalAssistantReadonlyProjections(t, ctx, memSvc, vault, scope)
	assertGroundingInvariant(t, beforeContext.Grounding, AgentGroundingGrounded, 1, 1)
	assertContainsSentinel(t, beforeContext, sentinel)
	assertContainsSentinel(t, beforeRecall, sentinel)
	assertContainsSentinel(t, beforeHandoff, sentinel)

	deleted, err := noteSvc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: vault, NoteRef: noteID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Facts["index_updated"] != "true" {
		t.Fatalf("delete index facts = %#v", deleted.Facts)
	}
	search, err := noteSvc.SearchNotes(ctx, SearchRequest{
		VaultPath: vault,
		Query:     sentinel,
		Engine:    "index",
		LazyIndex: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if search.Returned != 0 || len(search.Notes) != 0 || len(search.Results) != 0 {
		t.Fatalf("deleted transcript remained searchable: %#v", search)
	}

	afterContext, afterRecall, afterHandoff := personalAssistantReadonlyProjections(t, ctx, memSvc, vault, scope)
	for _, evidence := range []AgentGroundingEvidence{afterContext.Grounding, afterRecall.Grounding, afterHandoff.Grounding} {
		assertGroundingInvariant(t, evidence, AgentGroundingUngrounded, 0, 0)
		if evidence.CandidateSourceTotal != 1 || evidence.SourceMissing != 1 {
			t.Fatalf("deleted source evidence = %#v", evidence)
		}
	}
	if afterContext.Pack.EntryCount() != 0 || len(afterRecall.Memories) != 0 || len(afterHandoff.Handoffs) != 0 {
		t.Fatalf("deleted sourced content remained: context=%#v recall=%#v handoff=%#v", afterContext, afterRecall, afterHandoff)
	}
	assertNoSentinel(t, afterContext, sentinel)
	assertNoSentinel(t, afterRecall, sentinel)
	assertNoSentinel(t, afterHandoff, sentinel)

	// Canonical memory row is not silently deleted; only the PA readonly
	// projection suppresses content whose source has been revoked.
	raw, err := memSvc.AgentMemoryRecallQuery(ctx, vault, RecallQuery{Scope: scope})
	if err != nil || len(raw) != 1 {
		t.Fatalf("canonical memory changed after note delete: memories=%#v err=%v", raw, err)
	}
	trash, err := noteSvc.TrashList(ctx, TrashRequest{VaultPath: vault})
	if err != nil || trash.Facts["entries"] != "1" || trash.Facts["entry.1.object_id"] != noteID {
		t.Fatalf("delete tombstone = %#v err=%v", trash.Facts, err)
	}
}

func personalAssistantReadonlyProjections(
	t *testing.T,
	ctx context.Context,
	svc *AgentMemoryService,
	vault string,
	scope agentprotocol.Scope,
) (PersonalAssistantContextProjection, PersonalAssistantMemoryRecallProjection, PersonalAssistantHandoffReadProjection) {
	t.Helper()
	contextProjection, err := svc.AgentContextForPersonalAssistant(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("personal-assistant", "test"),
		Scope:     scope,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	recallProjection, err := svc.AgentMemoryRecallForPersonalAssistant(ctx, vault, RecallQuery{Scope: scope}, "")
	if err != nil {
		t.Fatal(err)
	}
	handoffProjection, err := svc.AgentHandoffReadForPersonalAssistant(ctx, vault, scope, "")
	if err != nil {
		t.Fatal(err)
	}
	return contextProjection, recallProjection, handoffProjection
}

func agentSourceCoverage(total, resolved, missing, stale, ambiguous int) agentcontinuity.SourceCoverage {
	return agentcontinuity.SourceCoverage{
		Total: total, Resolved: resolved, Missing: missing, Stale: stale, Ambiguous: ambiguous,
	}
}

func assertGroundingInvariant(
	t *testing.T,
	evidence AgentGroundingEvidence,
	wantState AgentGroundingState,
	wantTotal int,
	wantOpenable int,
) {
	t.Helper()
	if evidence.SchemaVersion != AgentGroundingEvidenceSchemaVersion || evidence.State != wantState || evidence.SourceTotal != wantTotal || evidence.SourceOpenable != wantOpenable {
		t.Fatalf("grounding evidence = %#v, want state=%s total=%d openable=%d", evidence, wantState, wantTotal, wantOpenable)
	}
	switch wantState {
	case AgentGroundingGrounded:
		if evidence.SourceTotal < 1 || evidence.SourceOpenable != evidence.SourceTotal {
			t.Fatalf("grounded invariant = %#v", evidence)
		}
	case AgentGroundingPartiallyGrounded:
		if evidence.SourceOpenable <= 0 || evidence.SourceOpenable >= evidence.SourceTotal {
			t.Fatalf("partial invariant = %#v", evidence)
		}
	case AgentGroundingUngrounded, AgentGroundingUnavailable:
		if evidence.SourceTotal != 0 || evidence.SourceOpenable != 0 {
			t.Fatalf("zero-source invariant = %#v", evidence)
		}
	}
}

func assertContainsSentinel(t *testing.T, value any, sentinel string) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), sentinel) {
		t.Fatalf("fixture sentinel missing before delete: %s", payload)
	}
}

func assertNoSentinel(t *testing.T, value any, sentinel string) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), sentinel) {
		t.Fatalf("deleted transcript sentinel leaked: %s", payload)
	}
}
