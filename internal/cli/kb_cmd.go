package cli

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/semantic"
)

func addKBCommands(root *cobra.Command, ctx commandBuildContext) {
	var includes []string
	var dryRun bool
	var rebuildBackend string
	var rebuildProvider string
	var rebuildModel string
	var refreshBackend string
	var refreshProvider string
	var refreshModel string
	var searchBackend string
	var searchProvider string
	var searchModel string
	var searchLimit int
	var contextBackend string
	var contextProvider string
	var contextModel string
	var contextLimit int

	kbCmd := &cobra.Command{
		Use:   "kb",
		Short: "Manage the local semantic knowledge base",
		Long:  "Manage the local semantic knowledge base projection. Pinax keeps Markdown as the source of truth and rebuilds the local LanceDB projection from the vault.",
	}

	importCmd := &cobra.Command{Use: "import <source>", Short: "Import Markdown or text into the vault for KB indexing", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "kb.import", "argument_required", "kb import requires a source path", "pinax kb import <source> --vault <vault> --dry-run")
		}
		projection, err := ctx.svc.KBImport(cmd.Context(), app.KBImportRequest{VaultPath: *ctx.vaultPath, Source: args[0], Includes: includes, DryRun: dryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	importCmd.Flags().StringArrayVar(&includes, "include", nil, "Include glob for source files; repeatable, defaults to *.md and *.txt")
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only output the import plan; do not write the vault")
	importCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm import writes")

	rebuildCmd := &cobra.Command{Use: "rebuild", Short: "Rebuild the local semantic projection", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBRebuild(cmd.Context(), kbIndexRequest(ctx, rebuildBackend, rebuildProvider, rebuildModel, 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBIndexFlags(rebuildCmd, &rebuildBackend, &rebuildProvider, &rebuildModel)

	refreshCmd := &cobra.Command{Use: "refresh", Short: "Refresh the local semantic projection", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBRefresh(cmd.Context(), kbIndexRequest(ctx, refreshBackend, refreshProvider, refreshModel, 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBIndexFlags(refreshCmd, &refreshBackend, &refreshProvider, &refreshModel)

	doctorCmd := &cobra.Command{Use: "doctor", Short: "Check the local semantic KB backend", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBDoctor(cmd.Context(), kbIndexRequest(ctx, semantic.DefaultBackend, "", "", 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}

	searchCmd := &cobra.Command{Use: "search <query>", Short: "Search the local semantic knowledge base", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "kb.search", "argument_required", "kb search requires a query", "pinax kb search <query> --vault <vault>")
		}
		projection, err := ctx.svc.KBSearch(cmd.Context(), kbIndexRequest(ctx, searchBackend, searchProvider, searchModel, searchLimit, args[0]))
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBSearchFlags(searchCmd, &searchBackend, &searchProvider, &searchModel)
	searchCmd.Flags().IntVar(&searchLimit, "limit", 0, "Limit semantic matches")

	contextCmd := &cobra.Command{Use: "context <task>", Short: "Return bounded agent context from the semantic knowledge base", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "kb.context", "argument_required", "kb context requires a task query", "pinax kb context <task> --vault <vault>")
		}
		projection, err := ctx.svc.KBContext(cmd.Context(), kbIndexRequest(ctx, contextBackend, contextProvider, contextModel, contextLimit, args[0]))
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBSearchFlags(contextCmd, &contextBackend, &contextProvider, &contextModel)
	contextCmd.Flags().IntVar(&contextLimit, "limit", 8, "Limit bounded context chunks")

	kbCmd.AddCommand(importCmd, rebuildCmd, refreshCmd, doctorCmd, searchCmd, contextCmd)
	root.AddCommand(kbCmd)
}

func kbIndexRequest(ctx commandBuildContext, backend, provider, model string, limit int, query string) app.KBIndexRequest {
	timeout := time.Duration(ctx.configResult.Config.KB.Sidecar.TimeoutSeconds) * time.Second
	return app.KBIndexRequest{VaultPath: *ctx.vaultPath, Backend: backend, Provider: provider, Model: model, Limit: limit, Query: query, SidecarExecutable: ctx.configResult.Config.KB.Sidecar.Executable, SidecarTimeout: timeout}
}

func addKBIndexFlags(cmd *cobra.Command, backend, provider, model *string) {
	cmd.Flags().StringVar(backend, "backend", semantic.DefaultBackend, "Semantic vector backend")
	cmd.Flags().StringVar(provider, "provider", semantic.DefaultProvider, "Embedding provider: gemini or fake")
	cmd.Flags().StringVar(model, "model", semantic.DefaultModel, "Embedding model")
	_ = cmd.RegisterFlagCompletionFunc("backend", staticCompletion("backend", semantic.DefaultBackend))
	_ = cmd.RegisterFlagCompletionFunc("provider", staticCompletion("provider", "gemini", "fake"))
}

func addKBSearchFlags(cmd *cobra.Command, backend, provider, model *string) {
	cmd.Flags().StringVar(backend, "backend", semantic.DefaultBackend, "Semantic vector backend")
	cmd.Flags().StringVar(provider, "provider", "", "Override embedding provider; defaults to the indexed provider")
	cmd.Flags().StringVar(model, "model", "", "Override embedding model; defaults to the indexed model")
	_ = cmd.RegisterFlagCompletionFunc("backend", staticCompletion("backend", semantic.DefaultBackend))
	_ = cmd.RegisterFlagCompletionFunc("provider", staticCompletion("provider", "gemini", "fake"))
}
