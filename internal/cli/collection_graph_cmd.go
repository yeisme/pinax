package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addCollectionCommands(root *cobra.Command, ctx commandBuildContext) {
	var fromPath string
	var toPath string
	var format string
	var dryRun bool
	var yes bool

	collectionCmd := &cobra.Command{
		Use:   "collection",
		Short: "Import, inspect, and export content collections",
		Long:  "Import, inspect, and export pinax.content_bundle.v1 content collections as Markdown notes, prompt assets, receipts, and graph-ready local projections.",
		Example: "pinax collection import --from ./bundle.json --dry-run --vault ./my-notes --json\n" +
			"pinax collection import --from ./bundle.json --yes --vault ./my-notes --json\n" +
			"pinax collection export --to ./eikona-bundle.json --format eikona.prompt_bundle.v1 --vault ./my-notes --json",
	}

	importCmd := &cobra.Command{
		Use:   "import --from <bundle>",
		Short: "Import a content bundle into the local vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CollectionImport(cmd.Context(), app.CollectionRequest{VaultPath: *ctx.vaultPath, From: fromPath, DryRun: dryRun, Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	importCmd.Flags().StringVar(&fromPath, "from", "", "pinax.content_bundle.v1 JSON or YAML file")
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the import plan without writing")
	importCmd.Flags().BoolVar(&yes, "yes", false, "Confirm collection import writes")
	collectionCmd.AddCommand(importCmd)

	diffCmd := &cobra.Command{
		Use:   "diff --from <bundle>",
		Short: "Compare a content bundle with the local vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CollectionDiff(cmd.Context(), app.CollectionRequest{VaultPath: *ctx.vaultPath, From: fromPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	diffCmd.Flags().StringVar(&fromPath, "from", "", "pinax.content_bundle.v1 JSON or YAML file")
	collectionCmd.AddCommand(diffCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor --from <bundle>",
		Short: "Check content bundle quality before or after import",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CollectionDoctor(cmd.Context(), app.CollectionRequest{VaultPath: *ctx.vaultPath, From: fromPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	doctorCmd.Flags().StringVar(&fromPath, "from", "", "pinax.content_bundle.v1 JSON or YAML file")
	collectionCmd.AddCommand(doctorCmd)

	exportCmd := &cobra.Command{
		Use:   "export --to <file>",
		Short: "Export prompt assets as an external prompt bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.CollectionExport(cmd.Context(), app.CollectionRequest{VaultPath: *ctx.vaultPath, To: toPath, Format: format})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	exportCmd.Flags().StringVar(&toPath, "to", "", "Output file to write")
	exportCmd.Flags().StringVar(&format, "format", "eikona.prompt_bundle.v1", "Export format")
	_ = exportCmd.RegisterFlagCompletionFunc("format", staticCompletion("format", "eikona.prompt_bundle.v1"))
	collectionCmd.AddCommand(exportCmd)

	root.AddCommand(collectionCmd)
}

func addGraphCommands(root *cobra.Command, ctx commandBuildContext) {
	var kind string
	var match string

	graphCmd := &cobra.Command{
		Use:   "graph",
		Short: "Build and query local knowledge graph projections",
		Long:  "Build and query local knowledge graph projections derived from Pinax vault assets. The prompt graph is rebuildable and does not become the vault source of truth.",
		Example: "pinax graph rebuild --vault ./my-notes --json\n" +
			"pinax graph query --kind technique --match storyboard --vault ./my-notes --json",
	}

	graphCmd.AddCommand(&cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the prompt knowledge graph projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.GraphRebuild(cmd.Context(), app.GraphRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	graphCmd.AddCommand(&cobra.Command{
		Use:   "summary",
		Short: "Summarize the local note link graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.GraphSummaryProjection(cmd.Context(), *ctx.vaultPath)
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	queryCmd := &cobra.Command{
		Use:   "query",
		Short: "Query the prompt knowledge graph projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.GraphQuery(cmd.Context(), app.GraphRequest{VaultPath: *ctx.vaultPath, Kind: kind, Match: match})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	queryCmd.Flags().StringVar(&kind, "kind", "", "Graph node kind filter, such as category, technique, style, subject, source, or prompt")
	queryCmd.Flags().StringVar(&match, "match", "", "Case-insensitive label match")
	_ = queryCmd.RegisterFlagCompletionFunc("kind", staticCompletion("graph node kind", "category", "technique", "style", "subject", "source", "prompt"))
	graphCmd.AddCommand(queryCmd)

	root.AddCommand(graphCmd)
}
