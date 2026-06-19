package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addPromptCommands(root *cobra.Command, ctx commandBuildContext) {
	var fromPath string
	var domain string
	var tag string
	var lifecycle string
	var lifecycleTo string
	var lifecycleReason string
	var limit int

	promptCmd := &cobra.Command{
		Use:   "prompt",
		Short: "Manage reusable prompt assets",
		Long:  "Manage reusable yeisme.prompt_asset.v1 records in the local Pinax vault. Prompt asset metadata is written by Pinax commands and resolved through CLI contracts.",
		Example: "pinax prompt import --from ./prompt.yaml --vault ./my-notes --json\n" +
			"pinax prompt search portrait --domain visual_generation --vault ./my-notes --json\n" +
			"pinax prompt resolve pinax://prompt/novel_character_portrait_v1 --vault ./my-notes --agent",
	}

	createCmd := &cobra.Command{
		Use:   "create --from <file>",
		Short: "Create a prompt asset from a schema file",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptCreate(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, From: fromPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	createCmd.Flags().StringVar(&fromPath, "from", "", "Prompt asset YAML file to create from")
	promptCmd.AddCommand(createCmd)

	importCmd := &cobra.Command{
		Use:   "import --from <file>",
		Short: "Import a prompt asset schema file",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptImport(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, From: fromPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	importCmd.Flags().StringVar(&fromPath, "from", "", "Prompt asset YAML file to import")
	promptCmd.AddCommand(importCmd)

	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search prompt assets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			projection, err := ctx.svc.PromptSearch(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, Query: query, Domain: domain, Tag: tag, Lifecycle: lifecycle, Limit: limit})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	searchCmd.Flags().StringVar(&domain, "domain", "", "Filter prompt assets by domain")
	searchCmd.Flags().StringVar(&tag, "tag", "", "Filter prompt assets by tag")
	searchCmd.Flags().StringVar(&lifecycle, "lifecycle", "", "Filter prompt assets by lifecycle")
	searchCmd.Flags().IntVar(&limit, "limit", 20, "Maximum prompt assets to return; 0 returns all")
	promptCmd.AddCommand(searchCmd)

	promptCmd.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show prompt asset details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptShow(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	promptCmd.AddCommand(&cobra.Command{
		Use:   "resolve <uri-or-id>",
		Short: "Resolve a prompt asset URI for agent use",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptResolve(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})

	lifecycleCmd := &cobra.Command{
		Use:   "lifecycle <id>",
		Short: "Update a prompt asset lifecycle with an explicit reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptLifecycle(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, Ref: args[0], To: lifecycleTo, Reason: lifecycleReason})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	lifecycleCmd.Flags().StringVar(&lifecycleTo, "to", "", "Target lifecycle: draft, tested, accepted, promoted, or retired")
	lifecycleCmd.Flags().StringVar(&lifecycleReason, "reason", "", "Reason for the lifecycle update")
	promptCmd.AddCommand(lifecycleCmd)

	feedbackCmd := &cobra.Command{
		Use:   "feedback",
		Short: "Import prompt usage feedback",
	}
	feedbackImportCmd := &cobra.Command{
		Use:   "import --from <file>",
		Short: "Import Eikona-style prompt usage feedback",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PromptFeedbackImport(cmd.Context(), app.PromptAssetRequest{VaultPath: *ctx.vaultPath, From: fromPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	feedbackImportCmd.Flags().StringVar(&fromPath, "from", "", "Prompt usage feedback JSON file to import")
	feedbackCmd.AddCommand(feedbackImportCmd)
	promptCmd.AddCommand(feedbackCmd)

	root.AddCommand(promptCmd)
}
