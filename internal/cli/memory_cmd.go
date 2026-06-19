package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addMemoryCommands(root *cobra.Command, ctx commandBuildContext) {
	var captureType string
	var captureSubject string
	var capturePredicate string
	var captureObject string
	var captureBody string
	var captureStatus string
	var captureConfidence string
	var captureSource string
	var captureSourceSpan string
	var captureEntities []string
	var captureDryRun bool

	var listType string
	var listEntity string
	var listLimit int
	var includeDraft bool
	var includeSuperseded bool
	var includeExpired bool
	var includeRejected bool

	var recallEntity string
	var recallType string
	var recallLimit int
	var contextEntity string
	var contextType string
	var contextLimit int

	memoryCmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage non-vector agent memory",
		Long:  "Manage the local non-vector agent memory ledger. Pinax stores typed memory records with source citations, lifecycle state, and explainable recall.",
	}

	captureCmd := &cobra.Command{Use: "capture", Short: "Capture a typed memory record", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MemoryCapture(cmd.Context(), app.MemoryCaptureRequest{VaultPath: *ctx.vaultPath, Type: captureType, Subject: captureSubject, Predicate: capturePredicate, Object: captureObject, Body: captureBody, Status: captureStatus, Confidence: captureConfidence, Source: captureSource, SourceSpan: captureSourceSpan, Entities: captureEntities, DryRun: captureDryRun})
		return ctx.renderProjection(cmd, projection, err)
	}}
	captureCmd.Flags().StringVar(&captureType, "type", "fact", "Memory type: fact, decision, event, or task")
	captureCmd.Flags().StringVar(&captureSubject, "subject", "", "Memory subject or primary entity")
	captureCmd.Flags().StringVar(&capturePredicate, "predicate", "", "Memory predicate for fact-style records")
	captureCmd.Flags().StringVar(&captureObject, "object", "", "Memory object or concise value")
	captureCmd.Flags().StringVar(&captureBody, "body", "", "Bounded memory body")
	captureCmd.Flags().StringVar(&captureStatus, "status", "confirmed", "Memory lifecycle status")
	captureCmd.Flags().StringVar(&captureConfidence, "confidence", "confirmed", "Memory confidence label")
	captureCmd.Flags().StringVar(&captureSource, "source", "", "Source citation URI or vault-relative path")
	captureCmd.Flags().StringVar(&captureSourceSpan, "source-span", "", "Optional source line or span")
	captureCmd.Flags().StringArrayVar(&captureEntities, "entity", nil, "Entity to link to the record; repeatable")
	captureCmd.Flags().BoolVar(&captureDryRun, "dry-run", false, "Preview the memory record without writing the ledger")
	_ = captureCmd.RegisterFlagCompletionFunc("type", staticCompletion("type", "fact", "decision", "event", "task"))
	_ = captureCmd.RegisterFlagCompletionFunc("status", staticCompletion("status", "confirmed", "draft", "superseded", "expired", "rejected"))

	listCmd := &cobra.Command{Use: "list", Short: "List memory records", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MemoryList(cmd.Context(), app.MemoryListRequest{VaultPath: *ctx.vaultPath, Type: listType, Entity: listEntity, IncludeDraft: includeDraft, IncludeSuperseded: includeSuperseded, IncludeExpired: includeExpired, IncludeRejected: includeRejected, Limit: listLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	addMemoryFilterFlags(listCmd, &listType, &listEntity, &listLimit)
	listCmd.Flags().BoolVar(&includeDraft, "include-draft", false, "Include draft memory records")
	listCmd.Flags().BoolVar(&includeSuperseded, "include-superseded", false, "Include superseded memory records")
	listCmd.Flags().BoolVar(&includeExpired, "include-expired", false, "Include expired memory records")
	listCmd.Flags().BoolVar(&includeRejected, "include-rejected", false, "Include rejected memory records")

	recallCmd := &cobra.Command{Use: "recall <query>", Short: "Recall memory records with deterministic non-vector ranking", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "memory.recall", "argument_required", "memory recall requires a query", "pinax memory recall <query> --vault <vault>")
		}
		projection, err := ctx.svc.MemoryRecall(cmd.Context(), app.MemoryRecallRequest{VaultPath: *ctx.vaultPath, Query: args[0], Entity: recallEntity, Type: recallType, Limit: recallLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	addMemoryFilterFlags(recallCmd, &recallType, &recallEntity, &recallLimit)

	contextCmd := &cobra.Command{Use: "context <task>", Short: "Return bounded agent context from memory", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "memory.context", "argument_required", "memory context requires a task query", "pinax memory context <task> --vault <vault>")
		}
		projection, err := ctx.svc.MemoryContext(cmd.Context(), app.MemoryRecallRequest{VaultPath: *ctx.vaultPath, Query: args[0], Entity: contextEntity, Type: contextType, Limit: contextLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	addMemoryFilterFlags(contextCmd, &contextType, &contextEntity, &contextLimit)
	contextCmd.Flags().Lookup("limit").DefValue = "12"

	statsCmd := &cobra.Command{Use: "stats", Short: "Show memory ledger stats", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MemoryStats(cmd.Context(), app.MemoryListRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}

	linkCmd := &cobra.Command{Use: "link <record-id>", Short: "Link a memory record to an entity", RunE: func(cmd *cobra.Command, args []string) error {
		return renderCommandError(cmd, ctx.outputMode(), "memory.link", "memory_link_unavailable", "memory link is not implemented in this slice", "Use pinax memory capture --entity to link records while capturing")
	}}

	pruneCmd := &cobra.Command{Use: "prune", Short: "Plan memory ledger pruning", RunE: func(cmd *cobra.Command, args []string) error {
		return renderCommandError(cmd, ctx.outputMode(), "memory.prune", "memory_prune_unavailable", "memory prune is not implemented in this slice", "Use pinax memory list to inspect records before pruning")
	}}

	memoryCmd.AddCommand(captureCmd, listCmd, recallCmd, contextCmd, linkCmd, pruneCmd, statsCmd)
	root.AddCommand(memoryCmd)
}

func addMemoryFilterFlags(cmd *cobra.Command, typeName, entity *string, limit *int) {
	cmd.Flags().StringVar(typeName, "type", "", "Filter memory type: fact, decision, event, or task")
	cmd.Flags().StringVar(entity, "entity", "", "Filter by linked entity")
	cmd.Flags().IntVar(limit, "limit", 0, "Limit memory records")
	_ = cmd.RegisterFlagCompletionFunc("type", staticCompletion("type", "fact", "decision", "event", "task"))
}
