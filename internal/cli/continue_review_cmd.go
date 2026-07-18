package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// addContinueCommands 注册 experimental `pinax continue` intent facade。
// 内部调用已有 AgentMemoryService.AgentContinuity，不改变旧 command tree。
func addContinueCommands(root *cobra.Command, ctx commandBuildContext) {
	var task, scopeFlag, intent, handoffFlag string
	var maxItems, maxChars int

	cmd := &cobra.Command{
		Use:   "continue",
		Short: "Experimental: compile a bounded continuity pack for continuing work across Agents",
		Long: `Compile a bounded, permission-first continuity pack that includes objectives,
key decisions, preferences, open tasks, failed attempts, conflicts, and source refs.

This is an experimental additive facade over 'pinax agent context'. Existing commands
remain unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
			if scopeFlag != "" {
				scope = parseScope(scopeFlag)
			}
			pack, err := agentSvc.AgentContinuity(cmd.Context(), app.ContinuityRequest{
				VaultPath: agentVaultPath(ctx),
				Principal: agentPrincipal(),
				Scope:     scope,
				Task:      task,
				Intent:    intent,
				MaxItems:  maxItems,
				MaxChars:  maxChars,
				HandoffID: handoffFlag,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue", err)
			}
			proj := domain.NewProjection("continue", "Continuity pack compiled.")
			proj.Facts["schema_version"] = pack.SchemaVersion
			proj.Facts["section_count"] = fmt.Sprintf("%d", pack.SectionCount())
			proj.Facts["handoff_status"] = string(pack.HandoffStatus)
			proj.Facts["truncated"] = fmt.Sprintf("%t", pack.Truncated)
			proj.Facts["source_coverage"] = fmt.Sprintf("%d/%d", pack.SourceCoverage.Resolved, pack.SourceCoverage.Total)
			proj.Facts["experimental"] = "true"
			if pack.Objective != "" {
				proj.Facts["objective"] = truncateForFact(pack.Objective, 100)
			}
			proj.Summary = agentcontinuity.SummaryLine(pack)
			proj.Data = pack
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "Current task description")
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "Target scope as kind:id (e.g. project:my-proj)")
	cmd.Flags().StringVar(&intent, "intent", "", "Intent description for relevance boosting")
	cmd.Flags().StringVar(&handoffFlag, "handoff", "", "Explicit handoff ID to consume (auto-select if empty)")
	cmd.Flags().IntVar(&maxItems, "max-items", 20, "Maximum continuity pack items")
	cmd.Flags().IntVar(&maxChars, "max-chars", 6000, "Maximum continuity pack characters")
	root.AddCommand(cmd)
}

// addReviewCommands 注册 experimental `pinax review` intent facade。
// 默认只读（list/show）；写 action 需要显式 --action flag 和 --yes confirmation。
func addReviewCommands(root *cobra.Command, ctx commandBuildContext) {
	var scopeFlag string
	var action, itemID string
	var yes bool

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Experimental: aggregate pending memory proposals into a reviewable inbox",
		Long: `Aggregate pending proposals, conflicts, stale/expired candidates, and reversible
receipts into a unified Memory Inbox. Default is read-only.

This is an experimental additive facade. Existing 'pinax agent memory' commands remain unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
			if scopeFlag != "" {
				scope = parseScope(scopeFlag)
			}

			// 默认只读 list
			if action == "" || action == "list" {
				return runReviewList(cmd, ctx, scope)
			}

			// show 不需要 --yes
			if action == "show" {
				return runReviewShow(cmd, ctx, scope, itemID)
			}

			// 写 action 需要显式 --yes
			if !yes {
				return fmt.Errorf("--action %s requires --yes confirmation", action)
			}
			if itemID == "" {
				return fmt.Errorf("--action requires --item <item-id>")
			}

			switch action {
			case "approve":
				return runReviewApprove(cmd, ctx, itemID)
			case "reject":
				return runReviewReject(cmd, ctx, itemID)
			default:
				return fmt.Errorf("unknown action %q (valid: list, show, approve, reject)", action)
			}
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "Target scope as kind:id (e.g. project:my-proj)")
	cmd.Flags().StringVar(&action, "action", "", "Review action: list (default), show, approve, reject")
	cmd.Flags().StringVar(&itemID, "item", "", "Item ID for show/approve/reject actions")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm write actions (required for approve/reject)")
	root.AddCommand(cmd)
}

func runReviewList(cmd *cobra.Command, ctx commandBuildContext, scope agentprotocol.Scope) error {
	pack, err := agentSvc.MemoryInbox(cmd.Context(), app.InboxRequest{
		VaultPath: agentVaultPath(ctx),
		Scope:     scope,
		Limit:     100,
	})
	if err != nil {
		return renderAgentError(cmd, ctx, "review", err)
	}
	proj := domain.NewProjection("review", "Memory inbox aggregated.")
	proj.Facts["total_items"] = fmt.Sprintf("%d", pack.TotalItems)
	proj.Facts["high_risk_count"] = fmt.Sprintf("%d", pack.HighRiskCount)
	proj.Facts["experimental"] = "true"
	for cat, count := range pack.CountsByCategory {
		proj.Facts[fmt.Sprintf("category_%s", cat)] = fmt.Sprintf("%d", count)
	}
	proj.Data = pack
	return ctx.renderProjection(cmd, proj, nil)
}

func runReviewShow(cmd *cobra.Command, ctx commandBuildContext, scope agentprotocol.Scope, itemID string) error {
	item, err := agentSvc.InboxItemDetail(cmd.Context(), app.InboxItemDetailRequest{
		VaultPath: agentVaultPath(ctx),
		Scope:     scope,
		ItemID:    itemID,
	})
	if err != nil {
		return renderAgentError(cmd, ctx, "review", err)
	}
	proj := domain.NewProjection("review.show", "Inbox item detail.")
	proj.Facts["item_id"] = item.ItemID
	proj.Facts["category"] = string(item.Category)
	proj.Facts["risk"] = string(item.Risk)
	proj.Facts["suggested_action"] = string(item.SuggestedAction)
	proj.Facts["experimental"] = "true"
	proj.Data = item
	return ctx.renderProjection(cmd, proj, nil)
}

func runReviewApprove(cmd *cobra.Command, ctx commandBuildContext, itemID string) error {
	proposalID := strings.TrimPrefix(itemID, "prop-")
	if proposalID == itemID {
		return fmt.Errorf("item %s is not a reviewable proposal", itemID)
	}
	facts, err := agentSvc.AgentMemoryApprove(cmd.Context(), agentVaultPath(ctx), proposalID, ownerPrincipal())
	if err != nil {
		return renderAgentError(cmd, ctx, "review.approve", err)
	}
	proj := domain.NewProjection("review.approve", "Proposal approved.")
	proj.Facts["proposal_id"] = facts.ProposalID
	proj.Facts["memory_id"] = facts.MemoryID
	proj.Facts["receipt_id"] = facts.ReceiptID
	proj.Facts["experimental"] = "true"
	return ctx.renderProjection(cmd, proj, nil)
}

func runReviewReject(cmd *cobra.Command, ctx commandBuildContext, itemID string) error {
	proposalID := strings.TrimPrefix(itemID, "prop-")
	if proposalID == itemID {
		return fmt.Errorf("item %s is not a reviewable proposal", itemID)
	}
	err := agentSvc.AgentMemoryReject(cmd.Context(), agentVaultPath(ctx), proposalID, ownerPrincipal(), "owner review")
	if err != nil {
		return renderAgentError(cmd, ctx, "review.reject", err)
	}
	proj := domain.NewProjection("review.reject", "Proposal rejected.")
	proj.Facts["proposal_id"] = proposalID
	proj.Facts["experimental"] = "true"
	return ctx.renderProjection(cmd, proj, nil)
}

func truncateForFact(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
