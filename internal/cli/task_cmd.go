package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addTaskCommands(root *cobra.Command, ctx commandBuildContext) {
	var planOnly bool
	taskCmd := &cobra.Command{Use: "task", Short: "Manage inferred and adopted tasks"}
	adoptCmd := &cobra.Command{
		Use:     "adopt <item>",
		Short:   "Adopt an inferred checklist task",
		Example: "pinax task adopt task_abc123 --plan --vault ./my-notes --json\npinax task adopt task_abc123 --yes --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "task.adopt", "argument_required", "task adopt requires an item id", "pinax task adopt <item> --plan --vault <vault>")
			}
			yes := *ctx.yes && !planOnly
			projection, err := ctx.svc.TaskAdopt(cmd.Context(), app.TaskAdoptRequest{VaultPath: *ctx.vaultPath, ItemID: args[0], Yes: yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	adoptCmd.Flags().BoolVar(&planOnly, "plan", false, "Preview adoption without writing the task adoption ledger")
	adoptCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm task adoption and write the ledger")
	taskCmd.AddCommand(adoptCmd)
	root.AddCommand(taskCmd)
}
