package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addGitCommands(root *cobra.Command, ctx commandBuildContext) {
	gitCmd := &cobra.Command{Use: "git", Short: "Compatibility commands for legacy Git snapshots", Hidden: true}
	gitSnapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Compatibility command for creating a pre-organization version snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionSnapshot(cmd.Context(), app.SnapshotRequest{VaultPath: *ctx.vaultPath, Message: *ctx.snapshotMessage})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	gitSnapshotCmd.Flags().StringVar(ctx.snapshotMessage, "message", "", "Version snapshot message")
	gitCmd.AddCommand(gitSnapshotCmd)
	root.AddCommand(gitCmd)

}
