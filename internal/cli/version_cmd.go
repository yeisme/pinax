package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addVersionCommands(root *cobra.Command, ctx commandBuildContext) {
	var historyLimit int
	var changedSince string
	var diffBase string
	var diffTarget string
	var showRevision string
	var restoreRevision string
	var restorePlan bool

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Manage vault version evidence",
		Long:  "Manage local version evidence for a Pinax vault. Git is only an optional backend; the main user path uses version status, version snapshot, version history, version diff, version show, version restore, version changed, and version backends.",
		Example: "pinax version status --vault ./my-notes\n" +
			"pinax version snapshot --vault ./my-notes --message \"Pre-organization snapshot\"\n" +
			"pinax version history --vault ./my-notes --json\n" +
			"pinax version restore notes/a.md --revision rev_1 --plan --vault ./my-notes --json\n" +
			"pinax version backends --vault ./my-notes --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection := domain.NewProjection("system.version", fmt.Sprintf("pinax %s", ctx.version))
			projection.Facts["version"] = ctx.version
			return ctx.renderProjection(cmd, projection, nil)
		},
	}
	versionCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check current vault version backend status",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionStatus(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create local version snapshot evidence",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionSnapshot(cmd.Context(), app.SnapshotRequest{VaultPath: *ctx.vaultPath, Message: *ctx.snapshotMessage})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	snapshotCmd.Flags().StringVar(ctx.snapshotMessage, "message", "", "Version snapshot message")
	versionCmd.AddCommand(snapshotCmd)

	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "List local version snapshot history",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionHistory(cmd.Context(), app.VersionHistoryRequest{VaultPath: *ctx.vaultPath, Limit: historyLimit})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	historyCmd.Flags().IntVar(&historyLimit, "limit", 20, "Maximum number of snapshots to return")
	versionCmd.AddCommand(historyCmd)

	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Read the version diff summary between two revisions",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionDiff(cmd.Context(), app.VersionDiffRequest{VaultPath: *ctx.vaultPath, BaseRevision: diffBase, TargetRevision: diffTarget})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	diffCmd.Flags().StringVar(&diffBase, "base", "", "base revision")
	diffCmd.Flags().StringVar(&diffTarget, "target", "", "target revision")
	versionCmd.AddCommand(diffCmd)

	showCmd := &cobra.Command{
		Use:   "show <path>",
		Short: "Read vault file content evidence by revision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionShow(cmd.Context(), app.VersionShowRequest{VaultPath: *ctx.vaultPath, Path: args[0], Revision: showRevision})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	showCmd.Flags().StringVar(&showRevision, "revision", "", "Revision to read")
	versionCmd.AddCommand(showCmd)

	restoreCmd := &cobra.Command{
		Use:   "restore <path>",
		Short: "Generate a read-only plan to restore a vault file by revision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !restorePlan {
				err := &domain.CommandError{Code: "approval_required", Message: "version restore requires generating a plan first", Hint: "Rerun with --plan"}
				return ctx.renderProjection(cmd, domain.NewErrorProjection("version.restore", err), err)
			}
			projection, err := ctx.svc.VersionRestorePlan(cmd.Context(), app.VersionRestorePlanRequest{VaultPath: *ctx.vaultPath, Path: args[0], Revision: restoreRevision})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	restoreCmd.Flags().StringVar(&restoreRevision, "revision", "", "Revision to restore")
	restoreCmd.Flags().BoolVar(&restorePlan, "plan", false, "Only generate the restore plan; do not write the vault")
	versionCmd.AddCommand(restoreCmd)

	changedCmd := &cobra.Command{
		Use:   "changed",
		Short: "Read changed path candidates after a revision",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionChanged(cmd.Context(), app.VersionChangedRequest{VaultPath: *ctx.vaultPath, SinceRevision: changedSince})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	changedCmd.Flags().StringVar(&changedSince, "since", "", "Starting revision")
	versionCmd.AddCommand(changedCmd)

	versionCmd.AddCommand(&cobra.Command{
		Use:   "backends",
		Short: "List available version backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.VersionBackends(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	root.AddCommand(versionCmd)
}
