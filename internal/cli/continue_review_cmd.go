package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/continuityevidence"
	"github.com/yeisme/pinax/internal/domain"
)

// addContinueCommands 注册 experimental `pinax continue` intent facade。
// 内部调用已有 AgentMemoryService.AgentContinuity，不改变旧 command tree。
//
// 兼容性（additive-only）：
//  1. 旧 leaf 行为（显式 --vault/--scope/--task/--intent/--handoff）完全保留；
//     binding 自动解析只在两个显式参数都未设置时介入。
//  2. --record-run/--runtime/--task-class 是 opt-in receipt 能力；
//     默认调用保持 read-only，不写任何 continuity evidence 表。
func addContinueCommands(root *cobra.Command, ctx commandBuildContext) {
	var task, scopeFlag, intent, handoffFlag string
	var maxItems, maxChars int
	var recordRun bool
	var runtimeID, taskClass string

	cmd := &cobra.Command{
		Use:   "continue",
		Short: "Experimental: compile a bounded continuity pack for continuing work across Agents",
		Long: `Compile a bounded, permission-first continuity pack that includes objectives,
key decisions, preferences, open tasks, failed attempts, conflicts, and source refs.

This is an experimental additive facade over 'pinax agent context'. Existing commands
remain unchanged. When the current Git worktree has a unique enabled continuity
binding and no explicit --vault/--scope is given, the binding auto-resolves.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// recorded run 的 enum 校验在任何写入之前完成（fail before write）。
			if recordRun {
				if runtimeID == "" || taskClass == "" {
					return renderCommandError(cmd, ctx.outputMode(), "continue", "validation_failed",
						"--record-run requires --runtime and --task-class",
						"pinax continue --record-run --runtime codex --task-class implementation_debugging")
				}
				if !continuityevidence.ValidTaskClass(taskClass) {
					return renderCommandError(cmd, ctx.outputMode(), "continue", "validation_failed",
						"unknown task class: "+taskClass,
						"valid: implementation_debugging, product_spec_docs, release_operations")
				}
			}

			resolution, err := resolveContinueScope(cmd, ctx, scopeFlag)
			if err != nil {
				return renderAgentError(cmd, ctx, "continue", err)
			}
			repoRoot := ""
			if resolution.BindingStatus == continuitybinding.StatusReady {
				// binding ready 但 worktree 检测失败属于环境故障：显式失败，
				// 不得把 repository source 全部记成 missing 污染 run receipt。
				root, err := continuitybinding.DetectWorktreeRoot(cmd.Context(), ".")
				if err != nil {
					return renderAgentError(cmd, ctx, "continue", fmt.Errorf("binding ready but worktree root detection failed: %w", err))
				}
				repoRoot = root
			}
			pack, err := agentSvc.AgentContinuity(cmd.Context(), app.ContinuityRequest{
				VaultPath: resolution.VaultPath,
				Principal: agentPrincipal(),
				Scope:     resolution.Scope,
				Task:      task,
				Intent:    intent,
				MaxItems:  maxItems,
				MaxChars:  maxChars,
				HandoffID: handoffFlag,
				RepoRoot:  repoRoot,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue", err)
			}

			// additive binding facts（旧 consumer 可忽略）。
			if resolution.BindingStatus != "" {
				pack.BindingStatus = string(resolution.BindingStatus)
				pack.BindingIDDigest = resolution.BindingDigest
			}

			// opt-in run receipt：只在显式 --record-run 时写入。
			if recordRun {
				runID, err := agentSvc.ContinuityRecordRun(cmd.Context(), app.ContinuityRunRecord{
					VaultPath:      resolution.VaultPath,
					BindingDigest:  resolution.BindingDigest,
					Scope:          resolution.Scope,
					Runtime:        runtimeID,
					TaskClass:      taskClass,
					HandoffStatus:  string(pack.HandoffStatus),
					SourceTotal:    pack.SourceCoverage.Total,
					SourceResolved: pack.SourceCoverage.Resolved,
					SourceStale:    pack.SourceCoverage.Stale,
					SourceMissing:  pack.SourceCoverage.Missing,
					WarningCodes:   pack.WarningCodes,
					ProposalCount:  0,
				})
				if err != nil {
					return renderAgentError(cmd, ctx, "continue", err)
				}
				pack.ContinuityRunID = runID
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
			// additive facts（旧 consumer 忽略新 key）。
			if pack.BindingStatus != "" {
				proj.Facts["binding_status"] = pack.BindingStatus
			}
			if pack.BindingIDDigest != "" {
				proj.Facts["binding_id_digest"] = pack.BindingIDDigest
			}
			if pack.ContinuityRunID != "" {
				proj.Facts["continuity_run_id"] = pack.ContinuityRunID
			}
			if pack.EvidenceStatus != "" {
				proj.Facts["evidence_status"] = pack.EvidenceStatus
			}
			if pack.FreshnessStatus != "" {
				proj.Facts["freshness_status"] = pack.FreshnessStatus
			}
			if pack.PackStatus != "" {
				proj.Facts["pack_status"] = pack.PackStatus
			}
			if len(pack.WarningCodes) > 0 {
				proj.Facts["warning_codes"] = strings.Join(pack.WarningCodes, ",")
			}
			if pack.RecommendedNextAction != nil {
				proj.Facts["recommended_next_action"] = pack.RecommendedNextAction.Name
			}
			if pack.ReviewAttentionCount > 0 {
				proj.Facts["review_attention_count"] = fmt.Sprintf("%d", pack.ReviewAttentionCount)
			}
			proj.Summary = agentcontinuity.SummaryLine(pack)
			proj.Data = pack
			if resolution.BindingStatus == continuitybinding.StatusMissing {
				// 未绑定：一个 copyable bind action，不扫描其他 vault。
				proj.Actions = append(proj.Actions, domain.Action{
					Name:    "bind",
					Command: "pinax continue bind --repo . --vault <vault-ref> --scope <kind:id>",
				})
			}
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "Current task description")
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "Target scope as kind:id (e.g. project:my-proj)")
	cmd.Flags().StringVar(&intent, "intent", "", "Intent description for relevance boosting")
	cmd.Flags().StringVar(&handoffFlag, "handoff", "", "Explicit handoff ID to consume (auto-select if empty)")
	cmd.Flags().IntVar(&maxItems, "max-items", 20, "Maximum continuity pack items")
	cmd.Flags().IntVar(&maxChars, "max-chars", 6000, "Maximum continuity pack characters")
	cmd.Flags().BoolVar(&recordRun, "record-run", false, "Opt-in: record a continuity run receipt (default continue is read-only)")
	cmd.Flags().StringVar(&runtimeID, "runtime", "", "Agent runtime descriptor for the recorded run (e.g. codex, claude-code)")
	cmd.Flags().StringVar(&taskClass, "task-class", "", "Task class for the recorded run (implementation_debugging, product_spec_docs, release_operations)")
	// additive experimental 子命令与 leaf RunE 共存：不带子命令时行为不变。
	addContinueBindingSubcommands(cmd, ctx)
	addContinueCheckpointSubcommand(cmd, ctx)
	addContinueWorkbenchSubcommand(cmd, ctx)
	addContinueFeedbackSubcommand(cmd, ctx)
	addContinueReportSubcommand(cmd, ctx)
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
