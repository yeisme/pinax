package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// addContinueCheckpointSubcommand 注册 additive experimental
// `pinax continue checkpoint`。它是 `agent handoff create` 的 intent facade：
// 自动使用 resolved binding/scope，durable candidate 只走 proposal service。
func addContinueCheckpointSubcommand(parent *cobra.Command, ctx commandBuildContext) {
	var scopeFlag, objective, currentState string
	var decisions, completedWork, blockers, verification, followUps, sources []string
	var durableCandidates []string
	var toRuntime, runID string

	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Experimental: create a bounded continuity checkpoint (handoff + optional proposals)",
		Long: `Create a bounded handoff for a clean stop, pause, or Agent switch.

Durable decisions/preferences/lessons are submitted as memory proposals
(--durable kind:subject=summary, repeatable); they never become confirmed
memory without an explicit review approval. Transcript files, raw prompts and
		free-form JSON dumps are not accepted: sections exceeding the item/character
caps fail validation instead of being truncated.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// repeated flags 与 comma 兼容：--decisions 可重复，也可逗号分隔。
			parsedSources, err := parseSourceRefs(flattenRepeated(sources))
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.checkpoint", err)
			}
			parsedDurable, err := parseDurableCandidates(durableCandidates)
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.checkpoint", err)
			}
			resolution, err := resolveContinueScope(cmd, ctx, scopeFlag)
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.checkpoint", err)
			}
			result, err := agentSvc.ContinuityCheckpoint(cmd.Context(), app.ContinuityCheckpointRequest{
				VaultPath:         resolution.VaultPath,
				Principal:         agentPrincipal(),
				Scope:             resolution.Scope,
				Objective:         objective,
				CurrentState:      currentState,
				Decisions:         flattenRepeated(decisions),
				CompletedWork:     flattenRepeated(completedWork),
				Blockers:          flattenRepeated(blockers),
				Verification:      flattenRepeated(verification),
				FollowUps:         flattenRepeated(followUps),
				Sources:           parsedSources,
				DurableCandidates: parsedDurable,
				ToRuntime:         toRuntime,
				RunID:             runID,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.checkpoint", err)
			}
			proj := domain.NewProjection("continue.checkpoint", "Continuity checkpoint saved.")
			proj.Facts["handoff_id"] = result.HandoffID
			proj.Facts["confirmed_created"] = fmt.Sprintf("%d", result.ConfirmedCreated)
			proj.Facts["scope"] = result.Scope
			proj.Facts["experimental"] = "true"
			if result.RunID != "" {
				proj.Facts["continuity_run_id"] = result.RunID
			}
			for i, proposalID := range result.ProposalIDs {
				proj.Facts[fmt.Sprintf("proposal_%d", i+1)] = proposalID
			}
			proj.Data = result
			proj.Actions = []domain.Action{
				{Name: "resume", Command: "pinax continue"},
				{Name: "review", Command: fmt.Sprintf("pinax review --scope %s", result.Scope)},
			}
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "Target scope as kind:id (defaults to resolved binding)")
	cmd.Flags().StringVar(&objective, "objective", "", "Checkpoint objective (required, bounded)")
	cmd.Flags().StringVar(&currentState, "current-state", "", "Bounded current task state")
	cmd.Flags().StringArrayVar(&decisions, "decisions", nil, "Decision item (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&completedWork, "completed-work", nil, "Completed work item (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&blockers, "blockers", nil, "Blocker item (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&verification, "verification", nil, "Verification fact (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&followUps, "follow-ups", nil, "Follow-up action (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&sources, "sources", nil, "Source ref as kind:ref (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&durableCandidates, "durable", nil, "Durable candidate as kind:subject=summary (repeatable; proposal-only)")
	cmd.Flags().StringVar(&toRuntime, "to-runtime", "", "Receiving agent runtime descriptor (e.g. codex, claude-code)")
	cmd.Flags().StringVar(&runID, "run", "", "Optional recorded continuity run ID to link")
	parent.AddCommand(cmd)
}

// parseSourceRefs 解析 []string 中的 "kind:ref" 项为 SourceRefList。
// 与 parseSources（逗号串）配套，用于 StringArray repeated flags。
func parseSourceRefs(values []string) (agentprotocol.SourceRefList, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var refs agentprotocol.SourceRefList
	for _, value := range values {
		for _, pair := range splitCSV(value) {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
					"source refs must use kind:ref")
			}
			ref := agentprotocol.SourceRef{Kind: strings.TrimSpace(parts[0]), Ref: strings.TrimSpace(parts[1])}
			// repository kind 支持 `path@<revision>` 后缀做漂移检测（additive）：
			// revision 记入 span 的 rev: 前缀，旧 consumer 可忽略。
			if ref.Kind == agentprotocol.SourceKindRepository {
				if rel, rev, ok := strings.Cut(ref.Ref, "@"); ok {
					if strings.TrimSpace(rel) == "" || strings.TrimSpace(rev) == "" {
						return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
							"repository source revisions must use path@revision")
					}
					ref.Ref = rel
					ref.Span = "rev:" + rev
				}
			}
			refs = append(refs, ref)
		}
	}
	if err := refs.Validate(); err != nil {
		return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "sources invalid: "+err.Error())
	}
	return refs, nil
}

// parseDurableCandidates 解析 "kind:subject=summary" 项。
// subject 与 summary 都可包含逗号以外的任意字符；格式非法的项被跳过
// （app 层 caps 校验仍会在空 subject 等非法形态上报 validation error）。
func parseDurableCandidates(values []string) ([]app.DurableCandidate, error) {
	var out []app.DurableCandidate
	for _, value := range values {
		for _, item := range splitCSV(value) {
			kind, rest, ok := strings.Cut(item, ":")
			if !ok {
				return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
					"durable candidates must use kind:subject=summary")
			}
			subject, summary, hasSummary := strings.Cut(rest, "=")
			if strings.TrimSpace(kind) == "" || strings.TrimSpace(subject) == "" || !hasSummary || strings.TrimSpace(summary) == "" {
				return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
					"durable candidates must use non-empty kind:subject=summary")
			}
			out = append(out, app.DurableCandidate{
				Kind: strings.TrimSpace(kind), Subject: strings.TrimSpace(subject), Summary: strings.TrimSpace(summary),
			})
		}
	}
	return out, nil
}

// flattenRepeated 把 StringArray 中残留的逗号分隔值展开为单项列表。
func flattenRepeated(values []string) []string {
	var out []string
	for _, value := range values {
		out = append(out, splitCSV(value)...)
	}
	return out
}
