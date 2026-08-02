package cli

import (
	"os"
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
	var rebuildAllowDiskHighWater bool
	var refreshBackend string
	var refreshProvider string
	var refreshModel string
	var refreshAllowDiskHighWater bool
	var searchBackend string
	var searchProvider string
	var searchModel string
	var searchLimit int
	var searchGeneration string
	var searchLegacyV1Readonly bool
	var contextBackend string
	var contextProvider string
	var contextModel string
	var contextLimit int
	var contextGeneration string
	var contextLegacyV1Readonly bool
	var evaluateSuite string
	var evaluateGeneration string
	var evaluateK int
	var activateGeneration string
	var activateSuite string
	var activateRunID string
	var activateExpectedSequence uint64
	var rollbackExpectedSequence uint64
	var generationPruneKeep int
	var generationPruneDryRun bool

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
		req := kbIndexRequest(ctx, rebuildBackend, rebuildProvider, rebuildModel, 0, "")
		req.DiskAdmission = kbDiskAdmissionPolicy(rebuildAllowDiskHighWater)
		projection, err := ctx.svc.KBRebuild(cmd.Context(), req)
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBIndexFlags(rebuildCmd, &rebuildBackend, &rebuildProvider, &rebuildModel)
	rebuildCmd.Flags().BoolVar(&rebuildAllowDiskHighWater, "allow-disk-high-water", false, "Allow a rebuild above the configured disk high-water mark")

	refreshCmd := &cobra.Command{Use: "refresh", Short: "Refresh the local semantic projection", RunE: func(cmd *cobra.Command, args []string) error {
		req := kbIndexRequest(ctx, refreshBackend, refreshProvider, refreshModel, 0, "")
		req.DiskAdmission = kbDiskAdmissionPolicy(refreshAllowDiskHighWater)
		projection, err := ctx.svc.KBRefresh(cmd.Context(), req)
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBIndexFlags(refreshCmd, &refreshBackend, &refreshProvider, &refreshModel)
	refreshCmd.Flags().BoolVar(&refreshAllowDiskHighWater, "allow-disk-high-water", false, "Allow a refresh above the configured disk high-water mark")

	doctorCmd := &cobra.Command{Use: "doctor", Short: "Check the local semantic KB backend", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBDoctor(cmd.Context(), kbIndexRequest(ctx, semantic.DefaultBackend, "", "", 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}

	providerCmd := &cobra.Command{Use: "provider", Short: "Inspect semantic embedding providers"}
	providerListCmd := &cobra.Command{Use: "list", Short: "List semantic embedding providers", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBProviderList(cmd.Context(), kbIndexRequest(ctx, semantic.DefaultBackend, "", "", 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}
	var doctorProvider string
	var doctorModel string
	providerDoctorCmd := &cobra.Command{Use: "doctor <provider>", Short: "Check a semantic embedding provider", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		doctorProvider = args[0]
		projection, err := ctx.svc.KBProviderDoctor(cmd.Context(), kbIndexRequest(ctx, semantic.DefaultBackend, doctorProvider, doctorModel, 0, ""))
		return ctx.renderProjection(cmd, projection, err)
	}}
	providerDoctorCmd.ValidArgsFunction = staticCompletion("provider", "gemini", "openai", "ollama", "fake")
	providerDoctorCmd.Flags().StringVar(&doctorModel, "model", "", "Embedding model to check")
	_ = providerDoctorCmd.RegisterFlagCompletionFunc("model", staticCompletion("model", semantic.DefaultModel, semantic.OpenAIDefaultModel, semantic.OllamaDefaultModel, semantic.FakeProviderModel))
	providerCmd.AddCommand(providerListCmd, providerDoctorCmd)

	searchCmd := &cobra.Command{Use: "search <query>", Short: "Search the local semantic knowledge base", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "kb.search", "argument_required", "kb search requires a query", "pinax kb search <query> --vault <vault>")
		}
		req := kbIndexRequest(ctx, searchBackend, searchProvider, searchModel, searchLimit, args[0])
		req.GenerationID = searchGeneration
		req.LegacyV1Readonly = searchLegacyV1Readonly
		projection, err := ctx.svc.KBSearch(cmd.Context(), req)
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBSearchFlags(searchCmd, &searchBackend, &searchProvider, &searchModel, &searchLegacyV1Readonly)
	searchCmd.Flags().IntVar(&searchLimit, "limit", 0, "Limit semantic matches")
	searchCmd.Flags().StringVar(&searchGeneration, "generation", "", "Pin search to a ready or active generation")

	contextCmd := &cobra.Command{Use: "context <task>", Short: "Return bounded agent context from the semantic knowledge base", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "kb.context", "argument_required", "kb context requires a task query", "pinax kb context <task> --vault <vault>")
		}
		req := kbIndexRequest(ctx, contextBackend, contextProvider, contextModel, contextLimit, args[0])
		req.GenerationID = contextGeneration
		req.LegacyV1Readonly = contextLegacyV1Readonly
		projection, err := ctx.svc.KBContext(cmd.Context(), req)
		return ctx.renderProjection(cmd, projection, err)
	}}
	addKBSearchFlags(contextCmd, &contextBackend, &contextProvider, &contextModel, &contextLegacyV1Readonly)
	contextCmd.Flags().IntVar(&contextLimit, "limit", 8, "Limit bounded context chunks")
	contextCmd.Flags().StringVar(&contextGeneration, "generation", "", "Pin context to a ready or active generation")

	evaluateCmd := &cobra.Command{Use: "evaluate", Short: "Evaluate retrieval and citations for a KB generation", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBEvaluate(cmd.Context(), app.KBEvaluateRequest{
			VaultPath:         *ctx.vaultPath,
			Suite:             evaluateSuite,
			GenerationID:      evaluateGeneration,
			K:                 evaluateK,
			SidecarExecutable: ctx.configResult.Config.KB.Sidecar.Executable,
			SidecarTimeout:    time.Duration(ctx.configResult.Config.KB.Sidecar.TimeoutSeconds) * time.Second,
		})
		return ctx.renderProjection(cmd, projection, err)
	}}
	evaluateCmd.Flags().StringVar(&evaluateSuite, "suite", "", "Evaluation suite path or suite id")
	evaluateCmd.Flags().StringVar(&evaluateGeneration, "generation", "", "Evaluate a ready or active generation")
	evaluateCmd.Flags().IntVar(&evaluateK, "k", 5, "Top-k retrieval cutoff")

	activateCmd := &cobra.Command{Use: "activate", Short: "Activate a passed KB candidate", RunE: func(cmd *cobra.Command, args []string) error {
		var expected *uint64
		if cmd.Flags().Changed("expected-sequence") {
			expected = &activateExpectedSequence
		}
		projection, err := ctx.svc.KBActivate(cmd.Context(), app.KBActivateRequest{VaultPath: *ctx.vaultPath, GenerationID: activateGeneration, Suite: activateSuite, RunID: activateRunID, ExpectedSequence: expected})
		return ctx.renderProjection(cmd, projection, err)
	}}
	activateCmd.Flags().StringVar(&activateGeneration, "generation", "", "Candidate generation id")
	activateCmd.Flags().StringVar(&activateSuite, "suite", "", "Evaluation suite path or suite id")
	activateCmd.Flags().StringVar(&activateRunID, "run-id", "", "Passed evaluation run id")
	activateCmd.Flags().Uint64Var(&activateExpectedSequence, "expected-sequence", 0, "Expected activation sequence for compare-and-swap")

	rollbackCmd := &cobra.Command{Use: "rollback", Short: "Roll back to the previous KB generation", RunE: func(cmd *cobra.Command, args []string) error {
		var expected *uint64
		if cmd.Flags().Changed("expected-sequence") {
			expected = &rollbackExpectedSequence
		}
		projection, err := ctx.svc.KBRollback(cmd.Context(), app.KBRollbackRequest{VaultPath: *ctx.vaultPath, ExpectedSequence: expected})
		return ctx.renderProjection(cmd, projection, err)
	}}
	rollbackCmd.Flags().Uint64Var(&rollbackExpectedSequence, "expected-sequence", 0, "Expected activation sequence for compare-and-swap")

	generationsCmd := &cobra.Command{Use: "generations", Short: "Inspect and maintain local KB generations"}
	generationsPruneCmd := &cobra.Command{Use: "prune", Short: "Plan or prune old KB candidate generations", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.KBPruneGenerations(cmd.Context(), app.KBGenerationPruneRequest{VaultPath: *ctx.vaultPath, Keep: generationPruneKeep, DryRun: generationPruneDryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	generationsPruneCmd.Flags().IntVar(&generationPruneKeep, "keep", 1, "Keep this many newest non-active candidate generations")
	generationsPruneCmd.Flags().BoolVar(&generationPruneDryRun, "dry-run", true, "Only show deletion candidates; do not remove anything")
	generationsPruneCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm deleting old candidate generations and their evaluation receipts")
	generationsCmd.AddCommand(generationsPruneCmd)

	kbCmd.AddCommand(importCmd, rebuildCmd, refreshCmd, doctorCmd, providerCmd, searchCmd, contextCmd, evaluateCmd, activateCmd, rollbackCmd, generationsCmd)
	root.AddCommand(kbCmd)
}

func kbDiskAdmissionPolicy(allowHighWater bool) *app.KBDiskAdmissionPolicy {
	policy := app.DefaultKBDiskAdmissionPolicy()
	policy.AllowHighWater = allowHighWater
	policy.ModelCachePath = os.Getenv("OLLAMA_MODELS")
	return &policy
}

func kbIndexRequest(ctx commandBuildContext, backend, provider, model string, limit int, query string) app.KBIndexRequest {
	timeout := time.Duration(ctx.configResult.Config.KB.Sidecar.TimeoutSeconds) * time.Second
	return app.KBIndexRequest{VaultPath: *ctx.vaultPath, Backend: backend, Provider: provider, Model: model, Limit: limit, Query: query, SidecarExecutable: ctx.configResult.Config.KB.Sidecar.Executable, SidecarTimeout: timeout}
}

func addKBIndexFlags(cmd *cobra.Command, backend, provider, model *string) {
	cmd.Flags().StringVar(backend, "backend", semantic.DefaultBackend, "Semantic vector backend")
	cmd.Flags().StringVar(provider, "provider", semantic.DefaultProvider, "Embedding provider: gemini, openai, ollama, or fake")
	cmd.Flags().StringVar(model, "model", semantic.DefaultModel, "Embedding model")
	_ = cmd.RegisterFlagCompletionFunc("backend", staticCompletion("backend", semantic.DefaultBackend))
	_ = cmd.RegisterFlagCompletionFunc("provider", staticCompletion("provider", "gemini", "openai", "ollama", "fake"))
}

func addKBSearchFlags(cmd *cobra.Command, backend, provider, model *string, legacyV1Readonly *bool) {
	cmd.Flags().StringVar(backend, "backend", semantic.DefaultBackend, "Semantic vector backend")
	cmd.Flags().StringVar(provider, "provider", "", "Override embedding provider; defaults to the indexed provider")
	cmd.Flags().StringVar(model, "model", "", "Override embedding model; defaults to the indexed model")
	cmd.Flags().BoolVar(legacyV1Readonly, "legacy-v1-readonly", false, "Read an existing legacy v1 projection without invoking its sidecar; mutations remain disabled")
	_ = cmd.RegisterFlagCompletionFunc("backend", staticCompletion("backend", semantic.DefaultBackend))
	_ = cmd.RegisterFlagCompletionFunc("provider", staticCompletion("provider", "gemini", "openai", "ollama", "fake"))
}
