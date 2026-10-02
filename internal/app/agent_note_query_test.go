package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/domain"
)

func queryProposalFixture(t *testing.T, n int) (*Service, string) {
	t.Helper()
	s, root, _ := noteProposalFixture(t)
	for i := 0; i < n; i++ {
		row := agentmemory.NoteProposalRow{ProposalID: fmt.Sprintf("proposal-%03d", i), PrincipalID: "fixture", ScopeKind: "workspace", ScopeID: "default", Kind: "note_edit", Status: "approval_required", CreatedAt: time.Unix(int64(i), 0)}
		if err := s.publishNoteProposal(root, row); err != nil {
			t.Fatal(err)
		}
	}
	return s, root
}
func TestAgentNoteProposalQueryPaginationAndLegacyShape(t *testing.T) {
	s, root := queryProposalFixture(t, 57)
	ctx := context.Background()
	p, err := s.QueryAgentNoteProposals(ctx, AgentNoteQuery{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Data.([]agentmemory.NoteProposalRow)) != 50 || p.Facts["total"] != "57" {
		t.Fatal("legacy default changed")
	}
	seen := map[string]bool{}
	q := AgentNoteQuery{VaultPath: root, Status: "approval_required", Kind: "note_edit", Limit: 5}
	for {
		p, err = s.QueryAgentNoteProposals(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range p.Data.([]agentmemory.NoteProposalRow) {
			if seen[row.ProposalID] {
				t.Fatal("duplicate page item")
			}
			seen[row.ProposalID] = true
		}
		for _, a := range p.Actions {
			if !strings.Contains(a.Command, "--vault "+shellQuote(root)) || strings.Contains(a.Command, "--yes") {
				t.Fatal("unsafe action")
			}
		}
		q.Cursor = p.Facts["next_cursor"]
		if q.Cursor == "" {
			break
		}
	}
	if len(seen) != 57 || !seen["proposal-000"] {
		t.Fatal("old item lost")
	}
	if _, err = os.Stat(filepath.Join(root, ".pinax", "memory")); !os.IsNotExist(err) {
		t.Fatal("read opened DB")
	}
}
func TestAgentNoteProposalQueryCursorRejectsChanges(t *testing.T) {
	s, root := queryProposalFixture(t, 9)
	ctx := context.Background()
	q := AgentNoteQuery{VaultPath: root, Limit: 5}
	p, err := s.QueryAgentNoteProposals(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	q.Cursor = p.Facts["next_cursor"]
	wrong := q
	wrong.Limit = 4
	if _, err = s.QueryAgentNoteProposals(ctx, wrong); domain.ErrorCode(err) != "proposal_cursor_invalid" {
		t.Fatal(err)
	}
	wrong = q
	wrong.VaultPath = t.TempDir()
	if _, err = s.QueryAgentNoteProposals(ctx, wrong); domain.ErrorCode(err) != "proposal_cursor_invalid" {
		t.Fatal(err)
	}
	wrong = q
	wrong.Cursor = "not base64"
	if _, err = s.QueryAgentNoteProposals(ctx, wrong); domain.ErrorCode(err) != "proposal_cursor_invalid" {
		t.Fatal(err)
	}
	row, err := readNoteProposalProjection(root, "proposal-000")
	if err != nil {
		t.Fatal(err)
	}
	row.Status = "rejected"
	if err = s.publishNoteProposal(root, row); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueryAgentNoteProposals(ctx, q); domain.ErrorCode(err) != "proposal_cursor_stale" {
		t.Fatal(err)
	}
}
func TestAgentNoteProposalQuerySingleSurvivesUnrelatedCorruption(t *testing.T) {
	s, root := queryProposalFixture(t, 2)
	writeAppFixture(t, filepath.Join(root, ".pinax", "note-proposals", "proposal-000.json"), "broken")
	if _, err := s.QueryAgentNoteProposals(context.Background(), AgentNoteQuery{VaultPath: root, ProposalID: "proposal-001"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryAgentNoteProposals(context.Background(), AgentNoteQuery{VaultPath: root}); domain.ErrorCode(err) != "proposal_projection_invalid" {
		t.Fatal(err)
	}
	if _, err := s.QueryAgentNoteProposals(context.Background(), AgentNoteQuery{VaultPath: root, ProposalID: "../unsafe"}); err == nil {
		t.Fatal("unsafe ID")
	}
}
