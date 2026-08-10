package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addBackupCommands(root *cobra.Command, ctx commandBuildContext) {
	var historyLimit int
	var restoreRevision string
	var restorePlan bool
	var restoreApplyPlan string
	var restoreApplyYes bool

	status := func(cmd *cobra.Command) error {
		projection, err := ctx.svc.VersionStatus(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return renderBackupProjection(cmd, ctx, projection, err, "backup.status")
	}
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Protect the local vault with versioned backups",
		Long:  "Protect a personal vault with local version/Git snapshots. This command stays local and does not contact remote services.",
		Example: "pinax backup --vault ./my-notes\n" +
			"pinax backup create --message \"daily checkpoint\" --vault ./my-notes\n" +
			"pinax backup history --vault ./my-notes --json\n" +
			"pinax backup restore notes/a.md --revision HEAD --plan --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return status(cmd)
		},
	}
	backupCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check local backup status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return status(cmd)
		},
	})

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a local versioned backup",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.VersionSnapshot(cmd.Context(), app.SnapshotRequest{VaultPath: *ctx.vaultPath, Message: *ctx.snapshotMessage})
			return renderBackupProjection(cmd, ctx, projection, err, "backup.create")
		},
	}
	createCmd.Flags().StringVar(ctx.snapshotMessage, "message", "", "Backup message")
	backupCmd.AddCommand(createCmd)

	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "List local backup history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.VersionHistory(cmd.Context(), app.VersionHistoryRequest{VaultPath: *ctx.vaultPath, Limit: historyLimit})
			return renderBackupProjection(cmd, ctx, projection, err, "backup.history")
		},
	}
	historyCmd.Flags().IntVar(&historyLimit, "limit", 20, "Maximum number of backups to return")
	backupCmd.AddCommand(historyCmd)

	restoreCmd := &cobra.Command{
		Use:   "restore <path>",
		Short: "Generate a read-only local restore plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !restorePlan {
				err := &domain.CommandError{Code: "approval_required", Message: "backup restore requires generating a plan first", Hint: "Rerun with --plan"}
				return ctx.renderProjection(cmd, domain.NewErrorProjection("backup.restore", err), err)
			}
			projection, err := ctx.svc.VersionRestorePlan(cmd.Context(), app.VersionRestorePlanRequest{VaultPath: *ctx.vaultPath, Path: args[0], Revision: restoreRevision})
			return renderBackupProjection(cmd, ctx, projection, err, "backup.restore")
		},
	}
	restoreCmd.Flags().StringVar(&restoreRevision, "revision", "", "Revision to restore")
	restoreCmd.Flags().BoolVar(&restorePlan, "plan", false, "Only generate the restore plan; do not write the vault")
	restoreApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a saved local restore plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.VersionRestoreApply(cmd.Context(), app.VersionRestoreApplyRequest{VaultPath: *ctx.vaultPath, PlanID: restoreApplyPlan, Yes: restoreApplyYes})
			return renderBackupProjection(cmd, ctx, projection, err, "backup.restore.apply")
		},
	}
	restoreApplyCmd.Flags().StringVar(&restoreApplyPlan, "plan", "", "Saved restore plan id or path")
	restoreApplyCmd.Flags().BoolVar(&restoreApplyYes, "yes", false, "Approve writing the restored content to local Markdown")
	restoreCmd.AddCommand(restoreApplyCmd)
	backupCmd.AddCommand(restoreCmd)
	root.AddCommand(backupCmd)
}

func renderBackupProjection(cmd *cobra.Command, ctx commandBuildContext, projection domain.Projection, err error, command string) error {
	projection.Command = command
	projection.Summary = strings.NewReplacer(
		"Version backend", "Local backup",
		"Version snapshot", "Local backup",
		"Version restore", "Backup restore",
	).Replace(projection.Summary)
	for i := range projection.Actions {
		projection.Actions[i].Command = strings.ReplaceAll(projection.Actions[i].Command, "pinax version snapshot", "pinax backup create")
	}
	if projection.Error != nil {
		copy := *projection.Error
		copy.Message = backupWording(copy.Message)
		copy.Hint = backupWording(copy.Hint)
		projection.Error = &copy
		err = projection.Error
	}
	return ctx.renderProjection(cmd, projection, err)
}

func backupWording(value string) string {
	return strings.NewReplacer(
		"version snapshot", "backup create",
		"Version snapshot", "Backup create",
		"version restore", "backup restore",
		"Version restore", "Backup restore",
	).Replace(value)
}
