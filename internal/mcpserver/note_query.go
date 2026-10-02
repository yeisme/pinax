package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func noteQueryMCPError(err error) error {
	code := domain.ErrorCode(err)
	if code == "" {
		code = "proposal_query_failed"
	}
	return newMCPError(-32001, code, "Refresh the query using tools/list; recovery or approval must run on the vault owner")
}
func (s *Server) queryNoteProposals(ctx context.Context, resp Response, args map[string]any) (Response, error) {
	q := app.AgentNoteQuery{VaultPath: s.vault, ProposalID: mcpStringArg(args, "proposal_id"), Status: mcpStringArg(args, "status"), Kind: mcpStringArg(args, "kind"), Cursor: mcpStringArg(args, "cursor")}
	if q.ProposalID != "" && len(args) > 1 {
		return resp, newMCPError(-32602, "proposal_query_invalid", "proposal_id cannot be combined with filters or pagination")
	}
	if raw, ok := args["limit"]; ok {
		b, _ := json.Marshal(raw)
		if json.Unmarshal(b, &q.Limit) != nil || q.Limit < 1 || q.Limit > 50 {
			return resp, newMCPError(-32602, "proposal_query_invalid", "limit must be between 1 and 50")
		}
	}
	p, err := s.service.QueryAgentNoteProposals(ctx, q)
	if err != nil {
		return resp, noteQueryMCPError(err)
	}
	resp.Result = noteProposalMCPResult(p)
	return resp, nil
}
func noteProposalMCPResult(p domain.Projection) map[string]any {
	// Never return owner filesystem commands as client-local actions.
	p.Actions = nil
	result := experimentalProjectionResult(p)
	actions := []map[string]any{}
	add := func(id string) {
		actions = append(actions, map[string]any{"tool": "pinax.agent.note_proposals", "arguments": map[string]any{"proposal_id": id}})
	}
	if rows, ok := p.Data.([]agentmemory.NoteProposalRow); ok {
		for _, row := range rows {
			add(row.ProposalID)
		}
	}
	if id := p.Facts["proposal_id"]; id != "" {
		add(id)
	}
	if cursor := p.Facts["next_cursor"]; cursor != "" {
		var limit int
		_ = json.Unmarshal([]byte(p.Facts["limit"]), &limit)
		args := map[string]any{"cursor": cursor, "limit": limit}
		if v := p.Facts["filter_status"]; v != "" {
			args["status"] = v
		}
		if v := p.Facts["filter_kind"]; v != "" {
			args["kind"] = v
		}
		actions = append(actions, map[string]any{"tool": "pinax.agent.note_proposals", "arguments": args})
	}
	result["tool_actions"] = actions
	result["owner_action"] = map[string]any{"execution_location": "vault_owner", "instruction": "Approval, rejection and metadata refresh require the owner-local CLI and explicit confirmation; this tool never grants write permission"}
	return result
}
