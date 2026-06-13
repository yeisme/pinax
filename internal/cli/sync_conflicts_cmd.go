package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addSyncConflictsCommands(syncCmd *cobra.Command, ctx commandBuildContext) {
	conflictsCmd := &cobra.Command{
		Use:   "conflicts",
		Short: "Manage conflict files produced by sync",
	}

	conflictsListCmd := &cobra.Command{
		Use:   "list",
		Short: "Scan and list all local conflict files",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncConflictsList(cmd.Context(), app.SyncConflictsListRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}

	conflictsShowCmd := &cobra.Command{
		Use:   "show <file>",
		Short: "Show conflict file details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncConflictsShow(cmd.Context(), app.SyncConflictFileRequest{VaultPath: *ctx.vaultPath, File: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}

	conflictsDiffCmd := &cobra.Command{
		Use:   "diff <file>",
		Short: "Show the diff between the main file and a conflict file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncConflictsDiff(cmd.Context(), app.SyncConflictFileRequest{VaultPath: *ctx.vaultPath, File: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}

	var keepLocal, keepRemote bool
	var mergedPath string
	conflictsResolveCmd := &cobra.Command{
		Use:   "resolve <file>",
		Short: "Resolve a conflict file; writes require --yes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncConflictsResolve(cmd.Context(), app.SyncConflictResolveRequest{VaultPath: *ctx.vaultPath, File: args[0], KeepLocal: keepLocal, KeepRemote: keepRemote, MergedPath: mergedPath, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	conflictsResolveCmd.Flags().BoolVar(&keepLocal, "keep-local", false, "Use the conflict version to overwrite the main file and clear the conflict file")
	conflictsResolveCmd.Flags().BoolVar(&keepRemote, "keep-remote", false, "Discard the conflict version")
	conflictsResolveCmd.Flags().StringVar(&mergedPath, "merged", "", "Use the specified file contents to overwrite the main file and clear the conflict file")
	conflictsResolveCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm conflict resolution writes")

	conflictsCmd.AddCommand(conflictsListCmd, conflictsShowCmd, conflictsDiffCmd, conflictsResolveCmd)
	syncCmd.AddCommand(conflictsCmd)
}
