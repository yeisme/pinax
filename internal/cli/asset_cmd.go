package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addAssetCommands(root *cobra.Command, ctx commandBuildContext) {
	var showPathStyle string
	var showContextNote string
	var showIncludePaths bool
	var linkNote string
	var previewAs string
	var previewContextNote string
	var previewMaxBytes int

	assetCmd := &cobra.Command{
		Use:   "asset",
		Short: "Manage vault media and binary assets",
		Long:  "Manage images, documents, and binary assets in a Pinax vault. Metadata is written by the CLI/service; do not hand-edit .pinax/assets/manifest.json.",
		Example: "pinax asset add ./diagram.png --vault ./my-notes --json\n" +
			"pinax asset list --vault ./my-notes --agent\n" +
			"pinax asset show diagram.png --vault ./my-notes --json\n" +
			"pinax asset link diagram.png --note \"Auth Plan\" --vault ./my-notes --json\n" +
			"pinax asset backlinks diagram.png --vault ./my-notes --json\n" +
			"pinax asset missing --vault ./my-notes --json\n" +
			"pinax asset repair --plan --vault ./my-notes --json\n" +
			"pinax asset verify --vault ./my-notes --json",
	}
	assetCmd.AddCommand(&cobra.Command{
		Use:   "add <file>",
		Short: "Add a file to the vault asset manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetAdd(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Source: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	assetCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List vault assets",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetList(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	showCmd := &cobra.Command{
		Use:               "show <asset>",
		Short:             "Show asset details",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetShow(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0], PathStyle: showPathStyle, ContextNote: showContextNote, IncludePaths: showIncludePaths})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	showCmd.Flags().StringVar(&showPathStyle, "path-style", "", "Path display style: vault-relative, note-relative, absolute, markdown, or wiki")
	showCmd.Flags().StringVar(&showContextNote, "context-note", "", "Context note for path display")
	showCmd.Flags().BoolVar(&showIncludePaths, "include-paths", false, "Include display_path in the requested style")
	_ = showCmd.RegisterFlagCompletionFunc("path-style", staticCompletion("path-style", "vault-relative", "note-relative", "absolute", "markdown", "wiki"))
	assetCmd.AddCommand(showCmd)

	linkCmd := &cobra.Command{
		Use:               "link <asset>",
		Short:             "Generate a plan to link an asset to a note",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetLink(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0], ContextNote: linkNote})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	linkCmd.Flags().StringVar(&linkNote, "note", "", "Target note to link to")
	assetCmd.AddCommand(linkCmd)

	assetCmd.AddCommand(&cobra.Command{
		Use:               "backlinks <asset>",
		Short:             "List notes referencing the asset",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetBacklinks(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	movePlan := false
	moveCmd := &cobra.Command{
		Use:               "move <asset> <target>",
		Short:             "Generate an asset move and reference rewrite plan",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !movePlan {
				err := &domain.CommandError{Code: "approval_required", Message: "asset move currently supports only --plan", Hint: "Rerun with --plan"}
				return ctx.renderProjection(cmd, domain.NewErrorProjection("asset.move", err), err)
			}
			projection, err := ctx.svc.AssetMovePlan(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0], Target: args[1]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	moveCmd.Flags().BoolVar(&movePlan, "plan", false, "Only generate the move and reference rewrite plan; do not write the vault")
	assetCmd.AddCommand(moveCmd)

	removePlan := false
	removeCmd := &cobra.Command{
		Use:               "remove <asset>",
		Short:             "Generate an asset removal or reference review plan",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !removePlan {
				err := &domain.CommandError{Code: "approval_required", Message: "asset remove currently supports only --plan", Hint: "Rerun with --plan"}
				return ctx.renderProjection(cmd, domain.NewErrorProjection("asset.remove", err), err)
			}
			projection, err := ctx.svc.AssetRemovePlan(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	removeCmd.Flags().BoolVar(&removePlan, "plan", false, "Only generate the removal or reference review plan; do not write the vault")
	assetCmd.AddCommand(removeCmd)

	assetCmd.AddCommand(&cobra.Command{
		Use:   "orphans",
		Short: "List assets with no note references",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetOrphans(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	assetCmd.AddCommand(&cobra.Command{
		Use:   "missing",
		Short: "List asset references pointing to missing files",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetMissing(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	repairPlan := false
	repairCmd := &cobra.Command{
		Use:   "repair",
		Short: "Generate an asset repair plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !repairPlan {
				err := &domain.CommandError{Code: "approval_required", Message: "asset repair currently supports only --plan", Hint: "Rerun with --plan"}
				return ctx.renderProjection(cmd, domain.NewErrorProjection("asset.repair", err), err)
			}
			projection, err := ctx.svc.AssetRepairPlan(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repairCmd.Flags().BoolVar(&repairPlan, "plan", false, "Only generate the repair plan; do not write the vault")
	assetCmd.AddCommand(repairCmd)

	previewCmd := &cobra.Command{
		Use:               "preview <asset>",
		Short:             "Read-only preview for a single asset",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: assetRefCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetPreview(cmd.Context(), app.AssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0], PreviewAs: previewAs, ContextNote: previewContextNote, MaxPreviewBytes: previewMaxBytes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	previewCmd.Flags().StringVar(&previewAs, "as", "markdown", "Preview mode: markdown or text")
	previewCmd.Flags().StringVar(&previewContextNote, "context-note", "", "Context note for path display")
	previewCmd.Flags().IntVar(&previewMaxBytes, "max-preview-bytes", 0, "Maximum preview bytes; 0 uses the default")
	_ = previewCmd.RegisterFlagCompletionFunc("as", staticCompletion("as", "markdown", "text"))
	assetCmd.AddCommand(previewCmd)

	assetCmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Validate file hashes in the asset manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.AssetVerify(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	root.AddCommand(assetCmd)
}
