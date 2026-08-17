package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// agentSvc 缓存当前进程的 AgentMemoryService（短生命周期 CLI 进程，安全）。
var agentSvc = app.NewAgentMemoryService()

// addAgentCommands 注册 experimental `pinax agent` command tree。
// 所有新命令标记 experimental，不 rename/remove 旧 `pinax memory`/`pinax brain` 入口。
func addAgentCommands(root *cobra.Command, ctx commandBuildContext) {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Experimental vendor-neutral Agent memory runtime (context, proposals, handoff, feedback)",
	}
	addAgentContextCommand(agentCmd, ctx)
	addAgentMemoryCommands(agentCmd, ctx)
	addAgentHandoffCommands(agentCmd, ctx)
	addAgentFeedbackCommands(agentCmd, ctx)
	addAgentStatusCommand(agentCmd, ctx)
	root.AddCommand(agentCmd)
}

func agentVaultPath(ctx commandBuildContext) string {
	if ctx.vaultPath != nil {
		return *ctx.vaultPath
	}
	return "."
}

func parseScope(s string) agentprotocol.Scope {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		return agentprotocol.Scope{Kind: agentprotocol.ScopeKind(parts[0]), ID: parts[1]}
	}
	return agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: s}
}

// agentPrincipal 构造一个默认 adapter principal（CLI 本地操作）。
// CLI 本地操作由 vault owner 驱动，因此附带 handoff 能力。
func agentPrincipal() agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal("local-cli", "pinax-cli")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}

// ownerPrincipal 构造一个 owner principal（用于 approve/confirm 操作）。
func ownerPrincipal() agentprotocol.Principal {
	return agentprotocol.Principal{
		SchemaVersion: agentprotocol.SchemaVersion,
		PrincipalID:   "local-owner",
		Trust:         agentprotocol.TrustLevelOwner,
		Capabilities: []agentprotocol.Capability{
			agentprotocol.CapabilityRead, agentprotocol.CapabilityApprove,
			agentprotocol.CapabilityConfirm, agentprotocol.CapabilityReview,
		},
	}
}

func addAgentContextCommand(parent *cobra.Command, ctx commandBuildContext) {
	var maxItems, maxChars int
	var entitiesFlag, scopeFlag string
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Compile a bounded, permission-first context pack",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := parseScope(scopeFlag)
			if scopeFlag == "" {
				scope = agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}
			}
			var entities []string
			if entitiesFlag != "" {
				entities = strings.Split(entitiesFlag, ",")
			}
			pack, err := agentSvc.AgentContextRuntime(cmd.Context(), app.AgentContextRequest{
				VaultPath: agentVaultPath(ctx),
				Principal: agentPrincipal(),
				Scope:     scope,
				Entities:  entities,
				MaxItems:  maxItems,
				MaxChars:  maxChars,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.context", err)
			}
			proj := domain.NewProjection("agent.context", "Agent context pack compiled.")
			proj.Facts["schema_version"] = pack.SchemaVersion
			proj.Facts["entry_count"] = fmt.Sprintf("%d", pack.EntryCount())
			proj.Facts["truncated"] = fmt.Sprintf("%t", pack.Truncated)
			proj.Data = pack
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().IntVar(&maxItems, "max-items", 20, "Maximum context pack entries")
	cmd.Flags().IntVar(&maxChars, "max-chars", 8000, "Maximum context pack preview characters")
	cmd.Flags().StringVar(&entitiesFlag, "entities", "", "Comma-separated entity keywords for relevance boosting")
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "Target scope as kind:id (e.g. project:my-proj)")
	parent.AddCommand(cmd)
}

func addAgentMemoryCommands(parent *cobra.Command, ctx commandBuildContext) {
	memoryCmd := &cobra.Command{
		Use:   "memory",
		Short: "Agent memory recall, proposals and lifecycle",
	}

	// recall
	var scopeFlag, kindFilter string
	recallCmd := &cobra.Command{
		Use:   "recall",
		Short: "Recall bounded agent memories",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := parseScope(scopeFlag)
			var kinds []agentprotocol.MemoryKind
			if kindFilter != "" {
				for _, k := range strings.Split(kindFilter, ",") {
					kinds = append(kinds, agentprotocol.MemoryKind(strings.TrimSpace(k)))
				}
			}
			// recall returns protocol memories — use app service
			results, err := agentSvc.AgentMemoryRecallQuery(cmd.Context(), agentVaultPath(ctx), app.RecallQuery{Scope: scope, Kinds: kinds})
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.memory.recall", err)
			}
			proj := domain.NewProjection("agent.memory.recall", fmt.Sprintf("Recalled %d agent memories.", len(results)))
			proj.Facts["count"] = fmt.Sprintf("%d", len(results))
			proj.Data = results
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	recallCmd.Flags().StringVar(&scopeFlag, "scope", "workspace:default", "Target scope as kind:id")
	recallCmd.Flags().StringVar(&kindFilter, "kind-filter", "", "Comma-separated kinds (fact,decision,preference,procedure,event,task,failure)")
	memoryCmd.AddCommand(recallCmd)

	// propose
	var propScope, propKind, propSubject, propSummary, propSources string
	proposeCmd := &cobra.Command{
		Use:   "propose",
		Short: "Submit a memory proposal for review (agents can only propose)",
		RunE: func(cmd *cobra.Command, args []string) error {
			facts, review, err := agentSvc.AgentMemoryPropose(cmd.Context(), app.AgentMemoryProposeRequest{
				VaultPath: agentVaultPath(ctx),
				Principal: agentPrincipal(),
				Scope:     parseScope(propScope),
				Kind:      agentprotocol.MemoryKind(propKind),
				Subject:   propSubject,
				Summary:   propSummary,
				Sources:   parseSources(propSources),
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.memory.propose", err)
			}
			proj := domain.NewProjection("agent.memory.propose", fmt.Sprintf("Proposal %s: %s.", facts.Status, facts.Reason))
			proj.Facts["proposal_id"] = facts.ProposalID
			proj.Facts["status"] = string(facts.Status)
			proj.Facts["reason"] = string(facts.Reason)
			proj.Data = review
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	proposeCmd.Flags().StringVar(&propScope, "scope", "workspace:default", "Target scope")
	proposeCmd.Flags().StringVar(&propKind, "kind", "fact", "Memory kind")
	proposeCmd.Flags().StringVar(&propSubject, "subject", "", "Memory subject")
	proposeCmd.Flags().StringVar(&propSummary, "summary", "", "Memory summary")
	proposeCmd.Flags().StringVar(&propSources, "sources", "", "Comma-separated source refs (kind:ref)")
	memoryCmd.AddCommand(proposeCmd)

	// proposals (list)
	var listScope string
	listCmd := &cobra.Command{
		Use:   "proposals",
		Short: "List agent memory proposals",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := agentSvc.AgentMemoryListProposals(cmd.Context(), agentVaultPath(ctx), parseScope(listScope))
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.memory.proposals", err)
			}
			proj := domain.NewProjection("agent.memory.proposals", fmt.Sprintf("Listed %d proposals.", len(results)))
			proj.Facts["count"] = fmt.Sprintf("%d", len(results))
			proj.Data = results
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	listCmd.Flags().StringVar(&listScope, "scope", "workspace:default", "Target scope")
	memoryCmd.AddCommand(listCmd)

	// approve
	approveCmd := &cobra.Command{
		Use:   "approve <proposal-id>",
		Short: "Approve a memory proposal (owner only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "agent.memory.approve", "argument_required", "proposal id is required", "pinax agent memory approve <proposal-id> --vault <vault> --yes")
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return renderCommandError(cmd, ctx.outputMode(), "agent.memory.approve", "approval_required", "use --yes to confirm approval", "pinax agent memory approve <proposal-id> --yes")
			}
			approveFacts, err := agentSvc.AgentMemoryApprove(cmd.Context(), agentVaultPath(ctx), args[0], ownerPrincipal())
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.memory.approve", err)
			}
			proj := domain.NewProjection("agent.memory.approve", "Proposal approved.")
			proj.Facts["proposal_id"] = approveFacts.ProposalID
			proj.Facts["memory_id"] = approveFacts.MemoryID
			proj.Facts["lifecycle_to"] = string(approveFacts.LifecycleTo)
			proj.Facts["receipt_id"] = approveFacts.ReceiptID
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	approveCmd.Flags().Bool("yes", false, "Confirm approval")
	memoryCmd.AddCommand(approveCmd)

	// reject
	rejectCmd := &cobra.Command{
		Use:   "reject <proposal-id>",
		Short: "Reject a memory proposal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "agent.memory.reject", "argument_required", "proposal id is required", "pinax agent memory reject <proposal-id>")
			}
			if err := agentSvc.AgentMemoryReject(cmd.Context(), agentVaultPath(ctx), args[0], ownerPrincipal(), "manual_reject"); err != nil {
				return renderAgentError(cmd, ctx, "agent.memory.reject", err)
			}
			proj := domain.NewProjection("agent.memory.reject", "Proposal rejected.")
			proj.Facts["proposal_id"] = args[0]
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	memoryCmd.AddCommand(rejectCmd)

	parent.AddCommand(memoryCmd)
}

func addAgentHandoffCommands(parent *cobra.Command, ctx commandBuildContext) {
	handoffCmd := &cobra.Command{
		Use:   "handoff",
		Short: "Cross-agent bounded working state handoff",
	}

	var hScope, hObjective, hCurrentState, hDecisions, hCompletedWork, hBlockers string
	var hVerification, hFollowUps, hSources, hRequestedNextCapability string
	var hToPrincipal, hToRuntime string
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a handoff (bounded working state, not confirmed memory)",
		RunE: func(cmd *cobra.Command, args []string) error {
			handoffID, err := agentSvc.AgentHandoffCreate(cmd.Context(), app.AgentHandoffCreateRequest{
				VaultPath:               agentVaultPath(ctx),
				From:                    agentPrincipal(),
				To:                      agentprotocol.DefaultAdapterPrincipal(hToPrincipal, hToRuntime),
				Scope:                   parseScope(hScope),
				Objective:               hObjective,
				CurrentState:            hCurrentState,
				Decisions:               splitCSV(hDecisions),
				CompletedWork:           splitCSV(hCompletedWork),
				Blockers:                splitCSV(hBlockers),
				Verification:            splitCSV(hVerification),
				FollowUps:               splitCSV(hFollowUps),
				Sources:                 parseSources(hSources),
				RequestedNextCapability: hRequestedNextCapability,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.handoff.create", err)
			}
			proj := domain.NewProjection("agent.handoff.create", "Handoff created.")
			proj.Facts["handoff_id"] = handoffID
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	createCmd.Flags().StringVar(&hScope, "scope", "workspace:default", "Target scope")
	createCmd.Flags().StringVar(&hObjective, "objective", "", "Handoff objective")
	createCmd.Flags().StringVar(&hCurrentState, "current-state", "", "Current bounded task state")
	createCmd.Flags().StringVar(&hDecisions, "decisions", "", "Comma-separated decisions")
	createCmd.Flags().StringVar(&hCompletedWork, "completed-work", "", "Comma-separated completed work items")
	createCmd.Flags().StringVar(&hBlockers, "blockers", "", "Comma-separated blockers")
	createCmd.Flags().StringVar(&hVerification, "verification", "", "Comma-separated verification facts")
	createCmd.Flags().StringVar(&hFollowUps, "follow-ups", "", "Comma-separated follow-up actions")
	createCmd.Flags().StringVar(&hSources, "sources", "", "Comma-separated source refs (kind:ref)")
	createCmd.Flags().StringVar(&hRequestedNextCapability, "requested-next-capability", "", "Capability requested from the receiving agent")
	createCmd.Flags().StringVar(&hToPrincipal, "to-principal", "target-agent", "Receiving principal ID")
	createCmd.Flags().StringVar(&hToRuntime, "to-runtime", "codex", "Receiving agent runtime")
	handoffCmd.AddCommand(createCmd)

	var listScope string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List handoffs",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := agentSvc.AgentHandoffList(cmd.Context(), agentVaultPath(ctx), parseScope(listScope))
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.handoff.list", err)
			}
			proj := domain.NewProjection("agent.handoff.list", fmt.Sprintf("Listed %d handoffs.", len(results)))
			proj.Facts["count"] = fmt.Sprintf("%d", len(results))
			proj.Data = results
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	listCmd.Flags().StringVar(&listScope, "scope", "workspace:default", "Target scope")
	handoffCmd.AddCommand(listCmd)

	showCmd := &cobra.Command{
		Use:   "show <handoff-id>",
		Short: "Show one bounded handoff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := agentSvc.AgentHandoffGet(cmd.Context(), agentVaultPath(ctx), args[0])
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.handoff.show", err)
			}
			proj := domain.NewProjection("agent.handoff.show", "Handoff loaded.")
			proj.Facts["handoff_id"] = result.HandoffID
			proj.Facts["scope"] = result.ScopeKind + ":" + result.ScopeID
			proj.Facts["source_count"] = fmt.Sprintf("%d", len(result.Sources))
			proj.Data = result
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	handoffCmd.AddCommand(showCmd)

	parent.AddCommand(handoffCmd)
}

func addAgentFeedbackCommands(parent *cobra.Command, ctx commandBuildContext) {
	feedbackCmd := &cobra.Command{
		Use:   "feedback",
		Short: "Recall quality feedback (does not rewrite memory content)",
	}

	var fbScope, fbKind, fbMemoryID, fbComment string
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add recall feedback",
		RunE: func(cmd *cobra.Command, args []string) error {
			feedbackID, err := agentSvc.AgentFeedbackAdd(cmd.Context(), app.AgentFeedbackAddRequest{
				VaultPath: agentVaultPath(ctx),
				Principal: agentPrincipal(),
				Scope:     parseScope(fbScope),
				Kind:      agentprotocol.FeedbackKind(fbKind),
				MemoryID:  fbMemoryID,
				Comment:   fbComment,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.feedback.add", err)
			}
			proj := domain.NewProjection("agent.feedback.add", "Feedback recorded.")
			proj.Facts["feedback_id"] = feedbackID
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	addCmd.Flags().StringVar(&fbScope, "scope", "workspace:default", "Target scope")
	addCmd.Flags().StringVar(&fbKind, "kind", "useful", "Feedback kind (useful,irrelevant,stale,incorrect,missing,completed,scope_too_wide)")
	addCmd.Flags().StringVar(&fbMemoryID, "memory-id", "", "Target memory ID")
	addCmd.Flags().StringVar(&fbComment, "comment", "", "Feedback comment")
	feedbackCmd.AddCommand(addCmd)

	var listScope string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List feedback",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := agentSvc.AgentFeedbackList(cmd.Context(), agentVaultPath(ctx), parseScope(listScope))
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.feedback.list", err)
			}
			proj := domain.NewProjection("agent.feedback.list", fmt.Sprintf("Listed %d feedback entries.", len(results)))
			proj.Facts["count"] = fmt.Sprintf("%d", len(results))
			proj.Data = results
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	listCmd.Flags().StringVar(&listScope, "scope", "workspace:default", "Target scope")
	feedbackCmd.AddCommand(listCmd)

	parent.AddCommand(feedbackCmd)
}

func addAgentStatusCommand(parent *cobra.Command, ctx commandBuildContext) {
	var statusScope string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent memory runtime status",
		RunE: func(cmd *cobra.Command, args []string) error {
			counts, err := agentSvc.AgentMemoryStatus(cmd.Context(), agentVaultPath(ctx), parseScope(statusScope))
			if err != nil {
				return renderAgentError(cmd, ctx, "agent.status", err)
			}
			proj := domain.NewProjection("agent.status", "Agent memory runtime status.")
			for state, count := range counts {
				proj.Facts[string(state)] = fmt.Sprintf("%d", count)
			}
			proj.Data = counts
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&statusScope, "scope", "workspace:default", "Target scope")
	parent.AddCommand(cmd)
}

// parseSources 解析 "kind:ref,kind:ref" 为 SourceRefList。
func parseSources(s string) agentprotocol.SourceRefList {
	if s == "" {
		return nil
	}
	var refs agentprotocol.SourceRefList
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			refs = append(refs, agentprotocol.SourceRef{Kind: parts[0], Ref: parts[1]})
		}
	}
	return refs
}

// renderAgentError 渲染 agent 命令的 StableError 为 projection。
func renderAgentError(cmd *cobra.Command, ctx commandBuildContext, command string, err error) error {
	if se, ok := err.(*agentprotocol.StableError); ok {
		cmdErr := &domain.CommandError{Code: se.Code, Message: se.Message}
		proj := domain.NewErrorProjection(command, cmdErr)
		return ctx.renderProjection(cmd, proj, se)
	}
	return renderCommandError(cmd, ctx.outputMode(), command, "agent_error", err.Error(), "")
}
