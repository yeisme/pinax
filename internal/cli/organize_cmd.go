package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addMetadataRepairOrganizeCommands(root *cobra.Command, ctx commandBuildContext) {
	metadataCmd := &cobra.Command{Use: "metadata", Short: "Plan and apply note metadata"}
	metadataCmd.AddCommand(&cobra.Command{
		Use:   "plan [query]",
		Short: "Preview a metadata backfill plan",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			projection, err := ctx.svc.PlanMetadata(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath, Query: query})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	metadataApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "Apply a metadata backfill plan",
		Long:    "Apply a metadata backfill plan. This command writes local Markdown frontmatter and requires explicit --yes. Run pinax metadata plan first to review the plan.",
		Example: "pinax metadata plan --vault ./my-notes --json\npinax metadata apply --vault ./my-notes --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ApplyMetadata(cmd.Context(), app.ApplyRequest{VaultPath: *ctx.vaultPath, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	metadataApplyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm local writes")
	metadataCmd.AddCommand(metadataApplyCmd)
	root.AddCommand(metadataCmd)

	repairCmd := &cobra.Command{Use: "repair", Short: "Plan and apply vault maintenance actions"}
	repairPlanCmd := &cobra.Command{
		Use:     "plan",
		Short:   "Generate a maintenance plan from doctor issues",
		Long:    "Generate a reviewable repair plan from vault doctor issues. By default this only outputs a plan and does not write Markdown or .pinax assets; with --save, the service writes .pinax/repair-plans/<plan_id>.json.",
		Example: "pinax repair plan --vault ./my-notes --json\npinax repair plan --vault ./my-notes --save --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PlanRepair(cmd.Context(), app.RepairPlanRequest{VaultPath: *ctx.vaultPath, Save: *ctx.repairSave})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repairPlanCmd.Flags().BoolVar(ctx.repairSave, "save", false, "Save the repair plan to .pinax/repair-plans")
	repairCmd.AddCommand(repairPlanCmd)
	repairApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "Apply a protected low-risk repair plan",
		Long:    "Apply a saved repair plan. This command writes the local vault, requires explicit --yes, and needs version snapshot protection or --snapshot-message to create a snapshot first.",
		Example: "pinax repair plan --vault ./my-notes --save --json\npinax version snapshot --vault ./my-notes --message \"Pre-repair snapshot\"\npinax repair apply --vault ./my-notes --plan repair-abc123 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ApplyRepair(cmd.Context(), app.RepairApplyRequest{VaultPath: *ctx.vaultPath, PlanID: *ctx.repairPlanID, Yes: *ctx.yes, SnapshotMessage: *ctx.snapshotMessage})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repairApplyCmd.Flags().StringVar(ctx.repairPlanID, "plan", "", "Repair plan id or relative path under .pinax/repair-plans")
	repairApplyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm local writes")
	repairApplyCmd.Flags().StringVar(ctx.snapshotMessage, "snapshot-message", "", "Message for an automatic version snapshot before apply")
	repairCmd.AddCommand(repairApplyCmd)
	root.AddCommand(repairCmd)

	organizeCmd := &cobra.Command{Use: "organize", Short: "Plan and apply note structure organization"}
	organizeSuggestCmd := &cobra.Command{
		Use:     "suggest",
		Short:   "Generate an agent-reviewable organization suggestion plan",
		Long:    "Generate an agent-reviewable organization suggestion plan. By default this only outputs a plan and does not write Markdown or .pinax assets; with --save, the service writes .pinax/organize-plans/<plan_id>.json.",
		Example: "pinax organize suggest --vault ./my-notes --json\npinax organize suggest --vault ./my-notes --save --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SuggestOrganize(cmd.Context(), app.OrganizeSuggestRequest{VaultPath: *ctx.vaultPath, Save: *ctx.organizeSave})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	organizeSuggestCmd.Hidden = true
	organizeSuggestCmd.Flags().BoolVar(ctx.organizeSave, "save", false, "Save the organize plan to .pinax/organize-plans")
	organizeCmd.AddCommand(organizeSuggestCmd)
	organizeCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved organize plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListOrganizePlans(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	organizePlanCmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview a structure organization plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			if *ctx.organizeSave {
				projection, err := ctx.svc.SuggestOrganize(cmd.Context(), app.OrganizeSuggestRequest{VaultPath: *ctx.vaultPath, Save: true})
				return ctx.renderProjection(cmd, projection, err)
			}
			projection, err := ctx.svc.PlanOrganize(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	organizePlanCmd.Flags().BoolVar(ctx.organizeSave, "save", false, "Save the organize plan to .pinax/organize-plans")
	organizeCmd.AddCommand(organizePlanCmd)
	organizeApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "Apply a structure organization plan",
		Long:    "Apply a structure organization plan. This command moves local note files, requires explicit --yes, and requires a saved and reviewed plan from pinax organize plan --save. A version snapshot must exist before applying, or use --snapshot-message so Pinax creates one first.",
		Example: "pinax organize plan --vault ./my-notes --save --json\npinax version snapshot --vault ./my-notes --message \"Pre-organization snapshot\"\npinax organize apply --vault ./my-notes --plan organize-abc123 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ApplyOrganize(cmd.Context(), app.ApplyRequest{VaultPath: *ctx.vaultPath, PlanID: *ctx.repairPlanID, Yes: *ctx.yes, SnapshotMessage: *ctx.snapshotMessage})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	organizeApplyCmd.Flags().StringVar(ctx.repairPlanID, "plan", "", "Organize plan id or relative path under .pinax/organize-plans")
	organizeApplyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm local writes")
	organizeApplyCmd.Flags().StringVar(ctx.snapshotMessage, "snapshot-message", "", "Message for an automatic version snapshot before apply")
	organizeCmd.AddCommand(organizeApplyCmd)
	root.AddCommand(organizeCmd)

}
