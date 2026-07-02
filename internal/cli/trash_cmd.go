package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addTrashCommands(root *cobra.Command, ctx commandBuildContext) {
	trashCmd := &cobra.Command{Use: "trash", Short: "Inspect and restore vault trash"}
	trashCmd.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List trash entries",
		Example: "pinax trash list --vault ./my-notes --json\npinax trash list --vault ./my-notes --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.TrashList(cmd.Context(), app.TrashRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	trashCmd.AddCommand(&cobra.Command{
		Use:     "restore <object>",
		Short:   "Restore a trash entry",
		Example: "pinax trash restore project/history --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "trash.restore", "argument_required", "trash restore requires an object", "pinax trash restore <object> --vault <vault> --json")
			}
			projection, err := ctx.svc.TrashRestore(cmd.Context(), app.TrashRequest{VaultPath: *ctx.vaultPath, ObjectRef: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	var purgeDryRun bool
	var purgeHard bool
	purgeCmd := &cobra.Command{
		Use:     "purge <object>",
		Short:   "Permanently remove a trash entry",
		Example: "pinax trash purge project/history --dry-run --vault ./my-notes --json\npinax trash purge project/history --hard --yes --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "trash.purge", "argument_required", "trash purge requires an object", "pinax trash purge <object> --dry-run --vault <vault> --json")
			}
			projection, err := ctx.svc.TrashPurge(cmd.Context(), app.TrashRequest{VaultPath: *ctx.vaultPath, ObjectRef: args[0], DryRun: purgeDryRun, Hard: purgeHard, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	purgeCmd.Flags().BoolVar(&purgeDryRun, "dry-run", false, "Preview purge without deleting trash data")
	purgeCmd.Flags().BoolVar(&purgeHard, "hard", false, "Permanently delete the trash entry")
	purgeCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm permanent purge")
	trashCmd.AddCommand(purgeCmd)
	root.AddCommand(trashCmd)
}
