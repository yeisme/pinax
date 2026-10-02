package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

type AgentNoteRequest struct {
	VaultPath string
	Principal agentprotocol.Principal
	Scope     agentprotocol.Scope
	Change    agentprotocol.NoteChange
}

func noteProposalError(code, message string) error {
	return &domain.CommandError{Code: code, Message: message, Hint: "Read the target card and original proposal before proposing a new change"}
}

func noteProposalSummary(value string) string {
	if len(redaction.ScanSensitiveClasses(value)) > 0 {
		return "[REDACTED]"
	}
	r := []rune(value)
	if len(r) > 500 {
		r = r[:500]
	}
	return string(r)
}

func validateNoteProposal(req AgentNoteRequest) error {
	if len(redaction.ScanSensitiveClasses(req.Principal.PrincipalID)) > 0 {
		return noteProposalError("validation_failed", "Use a non-sensitive principal identity")
	}
	if err := req.Principal.Validate(); err != nil {
		return err
	}
	if err := agentmemory.DefaultPolicy().CheckPropose(req.Principal); err != nil {
		return err
	}
	if err := req.Scope.Validate(); err != nil {
		return err
	}
	// v1 is a single-owner vault surface. Caller scope cannot widen it.
	if req.Scope.Kind != agentprotocol.ScopeKindWorkspace || req.Scope.ID != "default" {
		return noteProposalError("scope_mismatch", "Note proposals require the owner-bound workspace:default scope")
	}
	if err := req.Change.Validate(); err != nil {
		return err
	}
	if req.Change.TargetObjectID != "" && paneUnsafe(req.Change.TargetObjectID) {
		return noteProposalError("validation_failed", "Target must be a stable object ID")
	}
	// A body is ordinary Markdown, not an agent-authored frontmatter document.
	if strings.HasPrefix(strings.TrimSpace(req.Change.Body), "---") {
		return RejectHandwrittenMetadata(req.Change.Body)
	}
	if req.Change.Kind == "journal_append" {
		_, err := journalDate("daily", DailyRequest{Date: req.Change.Date})
		return err
	}
	return nil
}

func (s *Service) noteProposalTarget(ctx context.Context, root string, change agentprotocol.NoteChange) (domain.Note, string, error) {
	note, rev, err := s.CollaborationRead(ctx, root, change.TargetObjectID, "edit", CollaborationPolicy{AllowBody: true})
	if err != nil {
		return note, rev, err
	}
	if note.ID != change.TargetObjectID {
		return note, rev, noteProposalError("scope_mismatch", "Target must resolve to the supplied object ID")
	}
	if change.Kind == "journal_append" && note.Kind != "daily" {
		return note, rev, noteProposalError("scope_mismatch", "Journal append targets must be daily journal notes")
	}
	if change.ExpectedRevision != "" && rev != change.ExpectedRevision {
		return note, rev, noteProposalError("plan_stale", "The target changed; read its card and propose again with the current revision")
	}
	return note, rev, nil
}

func noteProposalProjection(command, root string, row agentmemory.NoteProposalRow) domain.Projection {
	row.Payload = ""
	p := domain.NewProjection(command, "Note proposal read.")
	p.Facts["schema_version"] = agentprotocol.NoteProposalSchemaVersion
	p.Facts["proposal_id"], p.Facts["risk"], p.Facts["proposal_status"] = row.ProposalID, row.Risk, row.Status
	p.Facts["target_object_id"] = row.TargetObjectID
	p.Facts["expected_revision"] = row.ExpectedRevision
	p.Facts["receipt_id"], p.Facts["snapshot_id"] = row.ReceiptID, row.SnapshotID
	p.Facts["approval_required"] = fmt.Sprint(row.Status == "approval_required")
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	p.Data = row
	p.Actions = []domain.Action{{Name: "review", Command: "pinax agent note proposal show " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)}}
	if row.TargetObjectID != "" {
		p.Actions = append(p.Actions, domain.Action{Name: "target", Command: "pinax search show " + shellQuote(row.TargetObjectID) + " --vault " + shellQuote(root)})
	}
	if row.Status == "approval_required" {
		for _, verb := range []string{"approve", "reject"} {
			p.Actions = append(p.Actions, domain.Action{Name: "preview_" + verb, Command: "pinax agent note proposal " + verb + " " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)})
		}
	}
	if row.Status == "applying" || row.Status == "reconcile_required" || row.Status == "rejecting" {
		p.Actions = append(p.Actions, domain.Action{Name: "operation", Command: "pinax operation show " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)}, domain.Action{Name: "refresh_preview", Command: "pinax agent note proposal refresh " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)})
	}
	return p
}

func (s *Service) AgentNotePropose(ctx context.Context, req AgentNoteRequest) (domain.Projection, error) {
	command := "agent.note.propose"
	if err := validateNoteProposal(req); err != nil {
		var stable *agentprotocol.StableError
		if errors.As(err, &stable) {
			err = &domain.CommandError{Code: stable.Code, Message: stable.Message}
		}
		return errorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	before := ""
	if req.Change.TargetObjectID != "" {
		n, rev, err := s.noteProposalTarget(ctx, root, req.Change)
		if err != nil {
			return errorProjection(command, err), err
		}
		before = n.Body
		req.Change.ExpectedRevision = rev
	}
	if req.Change.Kind == "journal_append" && req.Change.Date == "" && req.Change.TargetObjectID == "" {
		date, err := journalDate("daily", DailyRequest{})
		if err != nil {
			return errorProjection(command, err), err
		}
		req.Change.Date = date.Format("2006-01-02")
	}
	payload, err := json.Marshal(req.Change)
	if err != nil {
		return errorProjection(command, err), err
	}
	row := agentmemory.NoteProposalRow{ProposalID: newProposalID(req.Principal.PrincipalID), PrincipalID: req.Principal.PrincipalID, ScopeKind: string(req.Scope.Kind), ScopeID: req.Scope.ID, Kind: req.Change.Kind, Risk: string(req.Change.Risk()), Status: "approval_required", TargetObjectID: req.Change.TargetObjectID, ExpectedRevision: req.Change.ExpectedRevision, Rationale: noteProposalSummary(req.Change.Rationale), Payload: string(payload), AddedLines: len(strings.Split(req.Change.Body, "\n")), Hunks: 1, CreatedAt: nowUTC()}
	if before != "" && req.Change.Kind == "note_edit" {
		row.RemovedLines = len(strings.Split(before, "\n"))
	}
	if err := collaborationSafePath(root, ".pinax/memory/agent_memory.sqlite"); err != nil {
		return errorProjection(command, err), err
	}
	st, err := agentmemory.OpenNoteProposals(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	defer func() { _ = st.Close() }()
	if err := st.SaveNoteProposal(ctx, &row); err != nil {
		return errorProjection(command, err), err
	}
	if err := s.publishNoteProposal(root, row); err != nil {
		p := noteProposalProjection(command, root, row)
		p.Status = "failed"
		p.Error = &domain.CommandError{Code: "proposal_projection_unavailable", Message: "Refresh the original proposal projection; do not propose again", Hint: "pinax agent note proposal refresh " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)}
		return p, err
	}
	if req.Change.Risk() == agentprotocol.RiskLow {
		if err := s.applyNoteProposal(ctx, root, st, &row, true); err != nil {
			p := noteProposalProjection(command, root, row)
			p.Status, p.Error = "failed", &domain.CommandError{Code: domain.ErrorCode(err), Message: "Inspect the original proposal before retrying"}
			return p, err
		}
	}
	p := noteProposalProjection(command, root, row)
	p.Facts["local_write"] = "true"
	p.Summary = "Note proposal recorded."
	return p, nil
}

func (s *Service) applyNoteProposal(ctx context.Context, root string, st *agentmemory.Store, row *agentmemory.NoteProposalRow, auto bool) error {
	var c agentprotocol.NoteChange
	if err := json.Unmarshal([]byte(row.Payload), &c); err != nil {
		return noteProposalError("proposal_invalid", "Stored proposal cannot be decoded")
	}
	if c.TargetObjectID != "" {
		if _, _, err := s.noteProposalTarget(ctx, root, c); err != nil {
			if domain.ErrorCode(err) == "plan_stale" {
				row.StaleCount++
				if saveErr := s.persistNoteProposal(ctx, root, st, row); saveErr != nil {
					return saveErr
				}
			}
			return err
		}
	}
	claimed, err := st.ClaimNoteProposal(ctx, row.ProposalID, "approval_required", "applying")
	if err != nil {
		return err
	}
	if !claimed {
		return noteProposalError("proposal_consumed", "Proposal was already consumed; refresh the review inbox")
	}
	row.Status = "applying"
	if err := s.persistNoteProposal(ctx, root, st, row); err != nil {
		return err
	}
	err = s.applyNoteProposalContent(ctx, root, row, c)
	if err != nil {
		row.Status = "reconcile_required"
		if saveErr := s.persistNoteProposal(context.Background(), root, st, row); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return err
	}
	row.Status = "approved"
	if auto {
		row.Status = "auto_applied"
	}
	if err := s.noteProposalReceipt(root, row); err != nil {
		row.Status = "reconcile_required"
		_ = s.persistNoteProposal(context.Background(), root, st, row)
		return err
	}
	if err := s.persistNoteProposal(ctx, root, st, row); err != nil {
		row.Status = "reconcile_required"
		_ = s.persistNoteProposal(context.Background(), root, st, row)
		return err
	}
	return nil
}

func (s *Service) applyNoteProposalContent(ctx context.Context, root string, row *agentmemory.NoteProposalRow, c agentprotocol.NoteChange) error {
	// Reuse the proof snapshot primitive even for creation, before any note write.
	snapshot, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Before note proposal " + row.ProposalID})
	if err != nil {
		return err
	}
	row.SnapshotID = snapshot.Facts["snapshot_id"]
	change := CollaborationChange{OperationID: row.ProposalID, Action: "create", Title: row.ProposalID + "-" + c.Title, Body: c.Body, Dir: "index"}
	switch c.Kind {
	case "note_edit":
		change = CollaborationChange{OperationID: row.ProposalID, Action: "replace", NoteRef: c.TargetObjectID, ExpectedRevision: c.ExpectedRevision, Body: c.Body}
	case "journal_append":
		ref := c.TargetObjectID
		if ref == "" {
			_, rel, _, err := s.ensureJournalNote(root, "daily", DailyRequest{Date: c.Date})
			if err != nil {
				return err
			}
			note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
			if err != nil {
				return err
			}
			ref = note.ID
		}
		change = CollaborationChange{OperationID: row.ProposalID, Action: "append", NoteRef: ref, ExpectedRevision: c.ExpectedRevision, Body: "\n\n" + c.Body + "\n"}
	case "inbox_capture":
		change.Dir = "inbox"
		change.Status = "inbox"
	case "note_draft":
		change.Dir = "drafts"
		change.Status = "draft"
	}
	policy := CollaborationPolicy{AllowBody: true, AllowWrite: true}
	p, err := s.CollaborationPreview(ctx, root, change, policy)
	if err != nil {
		return err
	}
	view, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "reviewed_proposal", policy)
	if err != nil {
		return err
	}
	if string(view.Status) != "succeeded" {
		return noteProposalError("outcome_unknown", "Inspect the original proposal operation and recovery snapshot")
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: p.Path})
	if err != nil {
		return err
	}
	row.TargetObjectID = note.ID
	return nil
}

func (s *Service) noteProposalReceipt(root string, row *agentmemory.NoteProposalRow) error {
	id := "note-proposal-" + row.ProposalID + "-" + row.Status
	rel := filepath.Join(".pinax", "receipts", id+".json")
	if err := collaborationSafePath(root, rel); err != nil {
		return err
	}
	payload := map[string]any{"schema_version": "pinax.receipt.v1", "receipt_id": id, "proposal_id": row.ProposalID, "status": row.Status, "target_object_id": row.TargetObjectID, "rationale": row.Rationale, "added_lines": row.AddedLines, "removed_lines": row.RemovedLines, "hunks": row.Hunks, "snapshot_id": row.SnapshotID}
	if err := writeJSONAsset(filepath.Join(root, rel), payload); err != nil {
		return err
	}
	row.ReceiptID = id
	return nil
}

func (s *Service) AgentNoteProposals(ctx context.Context, root string, scope agentprotocol.Scope, id string) (domain.Projection, error) {
	command := "agent.note.proposal.list"
	if id != "" {
		command = "agent.note.proposal.show"
	}
	if scope.Kind != agentprotocol.ScopeKindWorkspace || scope.ID != "default" {
		err := noteProposalError("scope_mismatch", "Use workspace:default")
		return errorProjection(command, err), err
	}
	return s.QueryAgentNoteProposals(ctx, AgentNoteQuery{VaultPath: root, ProposalID: id})
}

func (s *Service) AgentNoteReview(ctx context.Context, root, id string, principal agentprotocol.Principal, approve, yes bool) (domain.Projection, error) {
	command := "agent.note.proposal.reject"
	if approve {
		command = "agent.note.proposal.approve"
	}
	if err := agentmemory.DefaultPolicy().CheckApprove(principal); err != nil {
		return errorProjection(command, err), err
	}
	root, err := cleanVaultPath(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	preview, err := s.AgentNoteProposals(ctx, root, agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}, id)
	if err != nil {
		return preview, err
	}
	row, ok := preview.Data.(agentmemory.NoteProposalRow)
	if !ok || row.Status != "approval_required" {
		err := noteProposalError("proposal_consumed", "Proposal is not pending; refresh the review inbox")
		return errorProjection(command, err), err
	}
	preview.Command = command
	preview.Summary = "Review preview; add --yes to confirm."
	if !yes {
		return preview, nil
	}
	st, err := agentmemory.OpenNoteProposals(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	defer func() { _ = st.Close() }()
	row, err = st.GetNoteProposal(ctx, id)
	if err != nil {
		return errorProjection(command, err), err
	}
	if approve {
		err = s.applyNoteProposal(ctx, root, st, &row, false)
	} else {
		var claimed bool
		claimed, err = st.ClaimNoteProposal(ctx, id, "approval_required", "rejecting")
		if err == nil && !claimed {
			err = noteProposalError("proposal_consumed", "Proposal was already consumed; refresh before acting")
		}
		if err == nil {
			row.Status = "rejected"
			err = s.noteProposalReceipt(root, &row)
			if err == nil {
				err = s.persistNoteProposal(ctx, root, st, &row)
			}
			if err != nil {
				row.Status = "reconcile_required"
				_ = s.persistNoteProposal(context.Background(), root, st, &row)
			}
		}
	}
	if err != nil {
		p := errorProjection(command, err)
		p.Facts["proposal_id"] = id
		p.Actions = []domain.Action{{Name: "inspect", Command: "pinax agent note proposal show " + shellQuote(id) + " --vault " + shellQuote(root)}}
		return p, err
	}
	p := noteProposalProjection(command, root, row)
	p.Facts["local_write"] = "true"
	p.Summary = "Note proposal reviewed."
	return p, nil
}

// File metadata is never an accepted substitute for a typed note payload.
func DecodeAgentNoteChange(data []byte) (agentprotocol.NoteChange, error) {
	var c agentprotocol.NoteChange
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, noteProposalError("validation_failed", "Use the typed note change schema; handwritten metadata is rejected")
	}
	var tail any
	if err := dec.Decode(&tail); !errors.Is(err, io.EOF) {
		return c, noteProposalError("validation_failed", "Only one change object is allowed")
	}
	return c, nil
}

// Read projections are CLI-authored and contain no payload. Reading them never
// opens SQLite, whose WAL read path can create SHM files even in mode=ro.
func (s *Service) publishNoteProposal(root string, row agentmemory.NoteProposalRow) error {
	if !paneRefPattern.MatchString(row.ProposalID) {
		return noteProposalError("proposal_invalid", "Invalid proposal identity")
	}
	rel := filepath.Join(".pinax", "note-proposals", row.ProposalID+".json")
	if err := collaborationSafePath(root, rel); err != nil {
		return err
	}
	return writeJSONAsset(filepath.Join(root, rel), map[string]any{"schema_version": agentprotocol.NoteProposalSchemaVersion, "proposal": row})
}

func (s *Service) persistNoteProposal(ctx context.Context, root string, st *agentmemory.Store, row *agentmemory.NoteProposalRow) error {
	return st.UpdateNoteProposal(ctx, row, func(r agentmemory.NoteProposalRow) error { return s.publishNoteProposal(root, r) })
}

func readNoteProposalProjections(root string) ([]agentmemory.NoteProposalRow, error) {
	rows := []agentmemory.NoteProposalRow{}
	rel := filepath.Join(".pinax", "note-proposals")
	if err := collaborationSafePath(root, rel); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, rel))
	if os.IsNotExist(err) {
		return rows, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		row, err := readNoteProposalProjection(root, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ProposalID < rows[j].ProposalID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	return rows, nil
}

func (s *Service) AgentNoteProposalRefresh(ctx context.Context, root, id string, yes bool) (domain.Projection, error) {
	p := domain.NewProjection("agent.note.proposal.refresh", "Projection refresh preview; --yes reads the canonical store and refreshes metadata only.")
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	if !yes {
		return p, nil
	}
	if !paneRefPattern.MatchString(id) {
		err := noteProposalError("proposal_invalid", "Use the original proposal ID")
		return errorProjection(p.Command, err), err
	}
	if err := collaborationSafePath(root, ".pinax/memory/agent_memory.sqlite"); err != nil {
		return errorProjection(p.Command, err), err
	}
	st, err := agentmemory.OpenNoteProposals(root)
	if err != nil {
		return errorProjection(p.Command, err), err
	}
	defer func() { _ = st.Close() }()
	if err := st.RefreshNoteProjection(ctx, id, func(row agentmemory.NoteProposalRow) error { return s.publishNoteProposal(root, row) }); err != nil {
		return errorProjection(p.Command, err), err
	}
	p.Summary = "Original proposal projection refreshed; no note was reapplied."
	p.Facts["local_write"] = "true"
	return p, nil
}

func (s *Service) AgentNoteProposalCounters(ctx context.Context, root string) (domain.Projection, error) {
	p := domain.NewProjection("agent.note.proposal.stats", "Note proposal outcome counters read.")
	rows, err := readNoteProposalProjections(root)
	if err != nil {
		return errorProjection(p.Command, err), err
	}
	counts := map[string]int64{"proposed": int64(len(rows)), "auto_applied": 0, "approved": 0, "rejected": 0, "stale": 0}
	for _, row := range rows {
		if row.Status == "auto_applied" || row.Status == "approved" || row.Status == "rejected" {
			counts[row.Status]++
		}
		counts["stale"] += int64(row.StaleCount)
	}
	p.Data = counts
	for k, v := range counts {
		p.Facts[k] = fmt.Sprint(v)
	}
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	return p, nil
}
