package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/domain"
)

type AgentNoteQuery struct {
	VaultPath  string
	ProposalID string
	Status     string
	Kind       string
	Limit      int
	Cursor     string
}

type noteProposalCursor struct {
	Version int    `json:"v"`
	Vault   string `json:"vault"`
	Status  string `json:"status"`
	Kind    string `json:"kind"`
	Limit   int    `json:"limit"`
	Digest  string `json:"digest"`
	Offset  int    `json:"offset"`
}

func proposalQueryCommand(req AgentNoteQuery) string {
	cmd := "pinax agent note proposal list --vault " + shellQuote(req.VaultPath)
	if req.Status != "" {
		cmd += " --status " + shellQuote(req.Status)
	}
	if req.Kind != "" {
		cmd += " --kind " + shellQuote(req.Kind)
	}
	cmd += fmt.Sprintf(" --limit %d", req.Limit)
	return cmd
}
func proposalQueryError(code string, req AgentNoteQuery) error {
	return &domain.CommandError{Code: code, Message: "Proposal query is invalid or changed; refresh the first page with the original filters", Hint: proposalQueryCommand(req)}
}
func proposalDigest(v any) string {
	b, _ := json.Marshal(v)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

// QueryAgentNoteProposals never opens the canonical payload store or persists cursors.
func (s *Service) QueryAgentNoteProposals(_ context.Context, req AgentNoteQuery) (domain.Projection, error) {
	command := "agent.note.proposal.list"
	if req.ProposalID != "" {
		command = "agent.note.proposal.show"
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	req.VaultPath = root
	if req.ProposalID != "" {
		if req.Status != "" || req.Kind != "" || req.Cursor != "" || req.Limit != 0 {
			err = proposalQueryError("proposal_query_invalid", req)
			return errorProjection(command, err), err
		}
		row, err := readNoteProposalProjection(root, req.ProposalID)
		if err != nil {
			p := errorProjection(command, err)
			p.Facts["proposal_id"] = req.ProposalID
			if paneRefPattern.MatchString(req.ProposalID) {
				p.Actions = []domain.Action{{Name: "inspect_owner", Command: "pinax agent note proposal refresh " + shellQuote(req.ProposalID) + " --vault " + shellQuote(root)}}
			}
			return p, err
		}
		return noteProposalProjection(command, root, row), nil
	}
	if req.Limit == 0 {
		req.Limit = 50
	}
	if req.Limit < 1 || req.Limit > 50 || !containsString([]string{"", "approval_required", "applying", "rejecting", "reconcile_required", "auto_applied", "approved", "rejected"}, req.Status) || !containsString([]string{"", "inbox_capture", "journal_append", "note_draft", "note_edit"}, req.Kind) {
		err = proposalQueryError("proposal_query_invalid", req)
		return errorProjection(command, err), err
	}
	rows, err := readNoteProposalProjections(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	selected := []agentmemory.NoteProposalRow{}
	for _, row := range rows {
		if (req.Status == "" || req.Status == row.Status) && (req.Kind == "" || req.Kind == row.Kind) {
			selected = append(selected, row)
		}
	}
	expected := noteProposalCursor{Version: 1, Vault: proposalDigest(root), Status: req.Status, Kind: req.Kind, Limit: req.Limit, Digest: proposalDigest(selected)}
	start := 0
	if req.Cursor != "" {
		if len(req.Cursor) > 2048 {
			err = proposalQueryError("proposal_cursor_invalid", req)
			return errorProjection(command, err), err
		}
		var c noteProposalCursor
		data, decodeErr := base64.RawURLEncoding.DecodeString(req.Cursor)
		if decodeErr != nil || json.Unmarshal(data, &c) != nil || c.Version != 1 || c.Vault != expected.Vault || c.Status != req.Status || c.Kind != req.Kind || c.Limit != req.Limit || c.Offset <= 0 || c.Offset%req.Limit != 0 {
			err = proposalQueryError("proposal_cursor_invalid", req)
			return errorProjection(command, err), err
		}
		if c.Digest != expected.Digest {
			err = proposalQueryError("proposal_cursor_stale", req)
			return errorProjection(command, err), err
		}
		if c.Offset >= len(selected) {
			err = proposalQueryError("proposal_cursor_invalid", req)
			return errorProjection(command, err), err
		}
		start = c.Offset
	}
	end := min(start+req.Limit, len(selected))
	more := end < len(selected)
	p := domain.NewProjection(command, "Note proposals listed.")
	p.Facts["count"], p.Facts["limit"] = fmt.Sprint(end-start), fmt.Sprint(req.Limit)
	p.Facts["total"], p.Facts["remaining"] = fmt.Sprint(len(selected)), fmt.Sprint(len(selected)-end)
	p.Facts["truncated"], p.Facts["has_more"] = fmt.Sprint(len(selected) > end-start), fmt.Sprint(more)
	p.Facts["local_write"], p.Facts["remote_write"] = "false", "false"
	p.Facts["filter_status"], p.Facts["filter_kind"] = req.Status, req.Kind
	p.Data = selected[start:end]
	for i, row := range selected[start:end] {
		prefix := fmt.Sprintf("item.%d.", i+1)
		p.Facts[prefix+"proposal_id"], p.Facts[prefix+"status"], p.Facts[prefix+"kind"] = row.ProposalID, row.Status, row.Kind
		p.Actions = append(p.Actions, domain.Action{Name: fmt.Sprintf("show_%d", i+1), Command: "pinax agent note proposal show " + shellQuote(row.ProposalID) + " --vault " + shellQuote(root)})
	}
	if more {
		expected.Offset = end
		b, _ := json.Marshal(expected)
		cursor := base64.RawURLEncoding.EncodeToString(b)
		p.Facts["next_cursor"] = cursor
		p.Actions = append(p.Actions, domain.Action{Name: "next", Command: proposalQueryCommand(req) + " --cursor " + shellQuote(cursor)})
	}
	return p, nil
}

func readNoteProposalProjection(root, id string) (agentmemory.NoteProposalRow, error) {
	var result struct {
		SchemaVersion string                      `json:"schema_version"`
		Proposal      agentmemory.NoteProposalRow `json:"proposal"`
	}
	if !paneRefPattern.MatchString(id) {
		return result.Proposal, noteProposalError("proposal_invalid", "Use the original proposal ID")
	}
	rel := filepath.Join(".pinax", "note-proposals", id+".json")
	if err := collaborationSafePath(root, rel); err != nil {
		return result.Proposal, err
	}
	b, err := os.ReadFile(filepath.Join(root, rel))
	if os.IsNotExist(err) {
		return result.Proposal, noteProposalError("proposal_not_found", "Original proposal projection is missing; ask the owner to refresh its metadata")
	}
	if err != nil {
		return result.Proposal, err
	}
	if len(b) > 64*1024 || json.Unmarshal(b, &result) != nil || result.SchemaVersion != agentprotocol.NoteProposalSchemaVersion || result.Proposal.ProposalID != id || result.Proposal.ScopeKind != "workspace" || result.Proposal.ScopeID != "default" {
		return agentmemory.NoteProposalRow{}, noteProposalError("proposal_projection_invalid", "Original proposal projection is invalid; ask the owner to refresh its metadata")
	}
	result.Proposal.Payload = ""
	result.Proposal.Rationale = noteProposalSummary(result.Proposal.Rationale)
	// File projections are data, never an authority to construct arbitrary commands.
	for _, value := range []string{result.Proposal.PrincipalID, result.Proposal.TargetObjectID, result.Proposal.ReceiptID, result.Proposal.SnapshotID} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return agentmemory.NoteProposalRow{}, noteProposalError("proposal_projection_invalid", "Invalid proposal metadata")
		}
	}
	return result.Proposal, nil
}
