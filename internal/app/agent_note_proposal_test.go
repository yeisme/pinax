package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/domain"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func noteProposalFixture(t *testing.T) (*Service, string, AgentNoteRequest) {
	t.Helper()
	s := NewService()
	root := t.TempDir()
	if _, err := s.InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Proposal fixture"}); err != nil {
		t.Fatal(err)
	}
	r := AgentNoteRequest{VaultPath: root, Principal: adapterPrincipal(), Scope: agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}}
	return s, root, r
}

func TestAgentNoteProposalAutoApplyNoOverwriteReceiptRedaction(t *testing.T) {
	s, root, req := noteProposalFixture(t)
	ctx := context.Background()
	ids := map[string]bool{}
	for _, kind := range []string{"inbox_capture", "note_draft", "journal_append", "inbox_capture"} {
		req.Change = agentprotocol.NoteChange{Kind: kind, Title: "Same title", Body: "UNIQUE_BODY_NOT_IN_RECEIPT", Rationale: "token=example-value"}
		if kind == "journal_append" {
			req.Change.Title = ""
		}
		p, err := s.AgentNotePropose(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		row := p.Data.(agentmemory.NoteProposalRow)
		if row.Status != "auto_applied" || row.SnapshotID == "" || row.ReceiptID == "" || row.TargetObjectID == "" {
			t.Fatalf("incomplete lifecycle: %#v", row)
		}
		if ids[row.TargetObjectID] {
			t.Fatal("creation overwrote an object")
		}
		ids[row.TargetObjectID] = true
		n, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: row.TargetObjectID})
		if err != nil || !strings.Contains(n.Body, req.Change.Body) {
			t.Fatalf("readback failed: %v", err)
		}
		if kind == "note_draft" && n.Status != "draft" {
			t.Fatal("draft lifecycle lost")
		}
		b, err := os.ReadFile(filepath.Join(root, ".pinax", "receipts", row.ReceiptID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), req.Change.Body) || strings.Contains(string(b), "example-value") {
			t.Fatal("receipt leaked content")
		}
		encoded, _ := json.Marshal(p)
		if strings.Contains(string(encoded), req.Change.Body) {
			t.Fatal("projection leaked private payload")
		}
	}
}

func TestAgentNoteProposalPreviewGatePlanStaleAndReject(t *testing.T) {
	s, root, req := noteProposalFixture(t)
	ctx := context.Background()
	created, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Target", Body: "original", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Facts["note_id"]
	note, revision, err := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	if err != nil {
		t.Fatal(err)
	}
	req.Change = agentprotocol.NoteChange{Kind: "note_edit", TargetObjectID: id, ExpectedRevision: revision, Body: "replacement", Rationale: "Review a body replacement"}
	p, err := s.AgentNotePropose(ctx, req)
	if err != nil || p.Facts["proposal_status"] != "approval_required" {
		t.Fatalf("propose: %v", err)
	}
	proposalID := p.Facts["proposal_id"]
	preview, err := s.AgentNoteReview(ctx, root, proposalID, ownerPrincipal(), true, false)
	if err != nil || preview.Facts["local_write"] != "false" {
		t.Fatalf("preview: %v", err)
	}
	current, _, _ := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	if current.Body != note.Body {
		t.Fatal("preview wrote the body")
	}
	if _, err := s.AgentNoteReview(ctx, root, proposalID, adapterPrincipal(), true, true); err == nil {
		t.Fatal("adapter approved itself")
	}
	path := filepath.Join(root, note.Path)
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(b, []byte("\nexternal edit")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentNoteReview(ctx, root, proposalID, ownerPrincipal(), true, true); domain.ErrorCode(err) != "plan_stale" {
		t.Fatalf("stale: %v", err)
	}
	rejected, err := s.AgentNoteReview(ctx, root, proposalID, ownerPrincipal(), false, true)
	if err != nil || rejected.Facts["proposal_status"] != "rejected" || rejected.Facts["receipt_id"] == "" {
		t.Fatalf("reject: %v", err)
	}
	if _, err := s.AgentNoteReview(ctx, root, proposalID, ownerPrincipal(), true, true); err == nil {
		t.Fatal("consumed proposal reapplied")
	}
	stats, err := s.AgentNoteProposalCounters(ctx, root)
	if err != nil || stats.Facts["proposed"] != "1" || stats.Facts["rejected"] != "1" || stats.Facts["stale"] != "1" {
		t.Fatal("outcome counters incorrect", err)
	}
	t.Logf("proposal_outcome_counts proposed=%s auto_applied=%s approved=%s rejected=%s stale=%s", stats.Facts["proposed"], stats.Facts["auto_applied"], stats.Facts["approved"], stats.Facts["rejected"], stats.Facts["stale"])
}

func TestAgentNoteProposalApproveReadbackAndMetadataFailClosed(t *testing.T) {
	s, root, req := noteProposalFixture(t)
	ctx := context.Background()
	p, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Target", Body: "old", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	_, rev, _ := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	req.Change = agentprotocol.NoteChange{Kind: "note_edit", TargetObjectID: id, ExpectedRevision: rev, Body: "new"}
	p, err = s.AgentNotePropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentNoteReview(ctx, root, p.Facts["proposal_id"], ownerPrincipal(), true, true); err != nil {
		t.Fatal(err)
	}
	note, _, _ := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	if !strings.Contains(note.Body, "new") || strings.Contains(note.Body, "old") {
		t.Fatal("replacement did not apply")
	}
	for _, raw := range []string{`{"kind":"note_draft","title":"x","body":"text","metadata":{"schema_version":"bad"}}`, `{"kind":"note_draft"} garbage`, `{} {}`} {
		if _, err := DecodeAgentNoteChange([]byte(raw)); err == nil {
			t.Fatal("malformed metadata accepted")
		}
	}
	req.Change = agentprotocol.NoteChange{Kind: "note_draft", Title: "x", Body: "---\nschema_version: bad\n---\ntext"}
	if _, err := s.AgentNotePropose(ctx, req); err == nil {
		t.Fatal("frontmatter accepted")
	}
}

func TestAgentNoteProposalListDoesNotInitializeStore(t *testing.T) {
	s, root, _ := noteProposalFixture(t)
	if _, err := s.AgentNoteProposals(context.Background(), root, agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "memory")); !os.IsNotExist(err) {
		t.Fatal("read initialized proposal storage")
	}
}

func TestAgentNoteProposalFailureRecoveryNeverReapplies(t *testing.T) {
	_, root, req := noteProposalFixture(t)
	s := NewServiceWithVersionBackend(pinaxversion.NoneBackend{})
	req.Change = agentprotocol.NoteChange{Kind: "note_draft", Title: "Failure fixture", Body: "not saved"}
	p, err := s.AgentNotePropose(context.Background(), req)
	if err == nil || p.Facts["proposal_id"] == "" {
		t.Fatal("failed outcome has no original identity")
	}
	read, err := s.AgentNoteProposals(context.Background(), root, req.Scope, p.Facts["proposal_id"])
	if err != nil || read.Facts["proposal_status"] != "reconcile_required" {
		t.Fatal("recovery state missing", err)
	}
	if _, err := s.AgentNoteReview(context.Background(), root, p.Facts["proposal_id"], ownerPrincipal(), true, true); err == nil {
		t.Fatal("unknown outcome reapplied")
	}
}

func TestAgentNoteProposalConcurrentReviewClaimsExactlyOnce(t *testing.T) {
	s, root, req := noteProposalFixture(t)
	ctx := context.Background()
	p, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Concurrent", Dir: "index", Body: "old"})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	_, rev, err := s.CollaborationRead(ctx, root, id, "read", CollaborationPolicy{AllowBody: true})
	if err != nil {
		t.Fatal(err)
	}
	req.Change = agentprotocol.NoteChange{Kind: "note_edit", TargetObjectID: id, ExpectedRevision: rev, Body: "once"}
	p, err = s.AgentNotePropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	proposalID := p.Facts["proposal_id"]
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := NewService().AgentNoteReview(ctx, root, proposalID, ownerPrincipal(), true, true)
			results <- err
		}()
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("proposal applied more or less than once")
	}
	stats, err := s.AgentNoteProposalCounters(ctx, root)
	if err != nil || stats.Facts["approved"] != "1" {
		t.Fatal("approved count incorrect", err)
	}
}
