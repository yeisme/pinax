package cli

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

func addAgentNoteCommands(parent *cobra.Command, ctx commandBuildContext) {
	note := &cobra.Command{Use: "note", Short: "Experimental proof-bound note proposals"}
	var change agentprotocol.NoteChange
	var stdin bool
	propose := &cobra.Command{Use: "propose", Short: "Propose a typed note change; low-risk kinds auto-apply through the proof gate", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !stdin {
			return renderCommandError(cmd, ctx.outputMode(), "agent.note.propose", "stdin_required", "Use --stdin for the Markdown body", "pinax agent note propose --kind note_draft --title <title> --stdin")
		}
		body, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), app.CollaborationMaxBodyBytes+1))
		if err != nil {
			return renderCommandError(cmd, ctx.outputMode(), "agent.note.propose", "stdin_failed", "Could not read the Markdown body", "")
		}
		change.Body = string(body)
		p, err := ctx.svc.AgentNotePropose(cmd.Context(), app.AgentNoteRequest{VaultPath: agentVaultPath(ctx), Principal: agentPrincipal(), Scope: agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}, Change: change})
		return ctx.renderProjection(cmd, p, err)
	}}
	propose.Flags().StringVar(&change.Kind, "kind", "", "Kind: inbox_capture, journal_append, note_draft, or note_edit")
	propose.Flags().StringVar(&change.Title, "title", "", "Title for a new inbox note or draft")
	propose.Flags().StringVar(&change.TargetObjectID, "target", "", "Stable target object ID for a note edit or existing daily journal")
	propose.Flags().StringVar(&change.ExpectedRevision, "expected-revision", "", "Pinned content revision for a note edit")
	propose.Flags().StringVar(&change.Rationale, "rationale", "", "Review rationale, at most 500 characters")
	propose.Flags().StringVar(&change.Date, "date", "", "Daily journal date as YYYY-MM-DD")
	propose.Flags().BoolVar(&stdin, "stdin", false, "Read Markdown body from stdin")
	note.AddCommand(propose)
	proposal := &cobra.Command{Use: "proposal", Short: "Review bounded note proposal summaries"}
	refresh := &cobra.Command{Use: "refresh <proposal-id>", Short: "Refresh only the original proposal metadata; never reapply its note", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		p, err := ctx.svc.AgentNoteProposalRefresh(cmd.Context(), agentVaultPath(ctx), args[0], yes)
		return ctx.renderProjection(cmd, p, err)
	}}
	refresh.Flags().Bool("yes", false, "Confirm metadata projection refresh")
	proposal.AddCommand(refresh)
	proposal.AddCommand(&cobra.Command{Use: "stats", Short: "Read body-free note proposal outcome counters", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := ctx.svc.AgentNoteProposalCounters(cmd.Context(), agentVaultPath(ctx))
		return ctx.renderProjection(cmd, p, err)
	}})
	var query app.AgentNoteQuery
	list := &cobra.Command{Use: "list", Short: "Read bounded note proposals with exact filters and readonly pagination", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if query.Limit < 1 || query.Limit > 50 {
			return renderCommandError(cmd, ctx.outputMode(), "agent.note.proposal.list", "proposal_query_invalid", "Limit must be between 1 and 50", "Use --limit 5")
		}
		query.VaultPath = agentVaultPath(ctx)
		p, err := ctx.svc.QueryAgentNoteProposals(cmd.Context(), query)
		return ctx.renderProjection(cmd, p, err)
	}}
	list.Flags().StringVar(&query.Status, "status", "", "Exact proposal lifecycle status")
	list.Flags().StringVar(&query.Kind, "kind", "", "Exact note proposal kind")
	list.Flags().IntVar(&query.Limit, "limit", 50, "Maximum proposals per page (1-50)")
	list.Flags().StringVar(&query.Cursor, "cursor", "", "Opaque cursor from the previous page; refresh after mutations")
	proposal.AddCommand(list)
	proposal.AddCommand(&cobra.Command{Use: "show <proposal-id>", Short: "Read the original proposal summary without its body", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := ctx.svc.QueryAgentNoteProposals(cmd.Context(), app.AgentNoteQuery{VaultPath: agentVaultPath(ctx), ProposalID: args[0]})
		return ctx.renderProjection(cmd, p, err)
	}})
	for _, verb := range []string{"approve", "reject"} {
		cmd := &cobra.Command{Use: verb + " <proposal-id>", Short: "Preview a note proposal " + verb + "; --yes confirms", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			p, err := ctx.svc.AgentNoteReview(cmd.Context(), agentVaultPath(ctx), args[0], ownerPrincipal(), cmd.Name() == "approve", yes)
			return ctx.renderProjection(cmd, p, err)
		}}
		cmd.Flags().Bool("yes", false, "Confirm the reviewed proposal action")
		proposal.AddCommand(cmd)
	}
	note.AddCommand(proposal)
	parent.AddCommand(note)
}
