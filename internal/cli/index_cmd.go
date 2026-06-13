package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addIndexCommands(root *cobra.Command, ctx commandBuildContext) {
	var lookupScope string
	var lookupKind string
	var refreshChangedSince string
	var repairKind string
	var repairDryRun bool
	var repairYes bool
	var pageTemplate string

	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Manage the local SQLite index",
		Long:  "Manage the Pinax local SQLite index projection. The bare command shows a read-only status summary and recommends the next refresh, diagnosis, repair, or rebuild step. refresh is the normal low-cost maintenance action; rebuild is a full reset action.",
		Example: "pinax index --vault ./my-notes\n" +
			"pinax index lookup diagram --scope all --vault ./my-notes --json\n" +
			"pinax index refresh --vault ./my-notes\n" +
			"pinax index refresh --changed-since rev_1 --vault ./my-notes --json\n" +
			"pinax index doctor --vault ./my-notes\n" +
			"pinax index rebuild --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexSummary(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}

	indexCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check local index status",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexStatus(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Low-cost maintenance for the local index projection",
		Long:  "Scan registered Pinax notes and incrementally maintain the local index projection. Prefer this command for ordinary missing or stale indexes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexRefresh(cmd.Context(), app.IndexRefreshRequest{VaultPath: *ctx.vaultPath, ChangedSince: refreshChangedSince})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	refreshCmd.Flags().StringVar(&refreshChangedSince, "changed-since", "", "Refresh only candidates changed after the specified revision through the version backend")
	indexCmd.AddCommand(refreshCmd)

	indexCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local index freshness and structural issues",
		Long:  "Read-only diagnosis for missing, stale, schema-incompatible, corrupt, or unreadable local index issues, with safe next steps.",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexDoctor(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	indexCmd.AddCommand(&cobra.Command{
		Use:   "rebuild",
		Short: "Fully reset and rebuild the local index projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.RebuildIndex(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	indexCmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Sync external changes into the local index projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncIndex(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	repairCmd := &cobra.Command{
		Use:   "repair",
		Short: "Run projection-safe index repair",
		Long:  "Only repairs rebuildable projection-layer index data. Defaults to dry-run; writing requires explicit --yes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexRepair(cmd.Context(), app.IndexRepairRequest{VaultPath: *ctx.vaultPath, Kind: repairKind, DryRun: repairDryRun, Yes: repairYes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repairCmd.Flags().StringVar(&repairKind, "kind", "recreate", "Repair kind: recreate")
	repairCmd.Flags().BoolVar(&repairDryRun, "dry-run", false, "Preview the repair plan only; do not write")
	repairCmd.Flags().BoolVar(&repairYes, "yes", false, "Confirm projection-safe writes")
	indexCmd.AddCommand(repairCmd)

	lookupCmd := &cobra.Command{
		Use:   "lookup <query>",
		Short: "Look up note, asset, and adoptable vault file candidates",
		Long:  "Read-only lookup for vault object candidates. The default scope is registered; use --scope all or --kind asset when unmanaged Markdown or assets are needed.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexLookup(cmd.Context(), app.IndexLookupRequest{VaultPath: *ctx.vaultPath, Query: args[0], Scope: lookupScope, Kind: lookupKind})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	lookupCmd.Flags().StringVar(&lookupScope, "scope", "registered", "Lookup scope: registered, adoptable, assets, registered_or_adoptable, or all")
	lookupCmd.Flags().StringVar(&lookupKind, "kind", "all", "Object kind: note, asset, file, or all")
	indexCmd.AddCommand(lookupCmd)

	pageCmd := &cobra.Command{Use: "page", Short: "Generate and refresh index pages"}
	addPageTemplateFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&pageTemplate, "template", "", "Index page template name")
		_ = c.RegisterFlagCompletionFunc("template", templateNameCompletion(func() string { return *ctx.vaultPath }, "index_template", true, true))
	}
	pagePreviewCmd := &cobra.Command{Use: "preview <name>", Short: "Preview an index page without writing files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.PreviewIndexPage(cmd.Context(), app.IndexPageRequest{VaultPath: *ctx.vaultPath, Name: args[0], Template: pageTemplate})
		return ctx.renderProjection(cmd, projection, err)
	}}
	pageCreateCmd := &cobra.Command{Use: "create <name>", Short: "Create an index page", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.CreateIndexPage(cmd.Context(), app.IndexPageRequest{VaultPath: *ctx.vaultPath, Name: args[0], Template: pageTemplate})
		return ctx.renderProjection(cmd, projection, err)
	}}
	pageRefreshCmd := &cobra.Command{Use: "refresh <name>", Short: "Refresh index page managed blocks", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RefreshIndexPage(cmd.Context(), app.IndexPageRequest{VaultPath: *ctx.vaultPath, Name: args[0], Template: pageTemplate})
		return ctx.renderProjection(cmd, projection, err)
	}}
	addPageTemplateFlag(pagePreviewCmd)
	addPageTemplateFlag(pageCreateCmd)
	addPageTemplateFlag(pageRefreshCmd)
	pageCmd.AddCommand(pagePreviewCmd, pageCreateCmd, pageRefreshCmd)
	indexCmd.AddCommand(pageCmd)

	indexCmd.AddCommand(&cobra.Command{
		Use:   "explain",
		Short: "Explain local index projection status and safe next steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.IndexExplain(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	indexCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize the local index database",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.InitIndex(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	root.AddCommand(indexCmd)
}
