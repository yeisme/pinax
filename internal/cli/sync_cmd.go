package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/output"
	"github.com/yeisme/pinax/internal/profile"
)

func resolveSyncRequest(req app.SyncRequest) app.SyncRequest {
	endpoint, workspace, device, secretRef, err := profile.ResolveTarget(req.Target)
	if err != nil || endpoint == "" {
		return req
	}
	if endpoint == req.Target && workspace == "" && device == "" && secretRef == "" {
		return req
	}
	resolved := req
	resolved.Endpoint = endpoint
	resolved.WorkspaceID = workspace
	resolved.DeviceID = device
	resolved.SecretRef = secretRef
	if target := syncTargetForEndpoint(endpoint); target != "" {
		resolved.Target = target
	}
	return resolved
}

func syncTargetForEndpoint(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	switch trimmed {
	case "git", "s3", "cloud":
		return trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return "cloud"
	case "s3":
		return "s3"
	default:
		return ""
	}
}

func syncTargetCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	items := []string{
		"cloud\tconfigured Cloud Sync backend",
		"s3\tS3-compatible direct backend",
		"git\tGit backend",
	}
	return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func syncDaemonLiveSink(w io.Writer, mode output.Mode, seq *int) syncdaemon.EventSink {
	switch mode {
	case output.ModeSummary:
		return func(event syncdaemon.SyncDaemonEvent) {
			status := event.Status
			if status == "" {
				status = "running"
			}
			if event.Direction != "" {
				_, _ = fmt.Fprintf(w, "%s %s %s\n", event.Type, event.Direction, status)
				return
			}
			_, _ = fmt.Fprintf(w, "%s %s\n", event.Type, status)
		}
	case output.ModeEvents:
		return func(event syncdaemon.SyncDaemonEvent) {
			*seq = *seq + 1
			payload := map[string]any{
				"type":            event.Type,
				"seq":             *seq,
				"status":          event.Status,
				"schema_version":  event.SchemaVersion,
				"trigger":         event.Trigger,
				"cycle_id":        event.CycleID,
				"direction":       event.Direction,
				"error_code":      event.ErrorCode,
				"duration_ms":     event.DurationMS,
				"local_dirty":     event.LocalDirty,
				"remote_revision": event.RemoteRevision,
				"revision_id":     event.RevisionID,
				"remote_write":    event.RemoteWrite,
				"local_write":     event.LocalWrite,
				"created_at":      event.CreatedAt,
			}
			_ = writeSyncDaemonStreamEvent(w, payload)
		}
	default:
		return nil
	}
}

func writeSyncDaemonStreamEvent(w io.Writer, payload map[string]any) error {
	payload["spec_version"] = "1.0"
	payload["mode"] = "events"
	payload["command"] = "sync.daemon.run"
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

func addSyncCommands(root *cobra.Command, ctx commandBuildContext) {
	var syncPathPolicy string
	var syncLogLimit int
	var syncPruneKeep int
	var syncPruneMaxAgeDays int
	var daemonPollInterval time.Duration
	var daemonSyncTimeout time.Duration
	var daemonOnce bool
	addPathPolicyFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&syncPathPolicy, "path-policy", "default", "Path redaction policy for sync receipts: default, hash, or omitted")
		_ = c.RegisterFlagCompletionFunc("path-policy", staticCompletion("path-policy", "default", "hash", "omitted"))
	}
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Generate and execute a one-command bidirectional sync plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			request := resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, PathPolicy: syncPathPolicy})
			projection, err := ctx.svc.SyncAll(cmd.Context(), request)
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncCmd.Flags().StringVar(ctx.syncTarget, "target", "cloud", "Sync target: git, s3, or cloud")
	_ = syncCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only run merge calculation")
	syncCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync writes")
	addPathPolicyFlag(syncCmd)

	syncInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize cloud sync configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncInit(cmd.Context(), app.SyncInitRequest{VaultPath: *ctx.vaultPath, Endpoint: *ctx.cloudEndpoint, WorkspaceID: *ctx.cloudWorkspace, DeviceID: *ctx.cloudDevice, SecretRef: *ctx.cloudSecretRef})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncInitCmd.Flags().StringVar(ctx.cloudEndpoint, "endpoint", "", "Storage service address")
	syncInitCmd.Flags().StringVar(ctx.cloudWorkspace, "workspace", "default", "workspace id")
	syncInitCmd.Flags().StringVar(ctx.cloudDevice, "device", "device1", "device id")
	syncInitCmd.Flags().StringVar(ctx.cloudSecretRef, "secret-ref", "", "Encryption secret or password")
	syncCmd.AddCommand(syncInitCmd)

	syncStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check sync health",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncStatus(cmd.Context(), app.SyncStatusRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncCmd.AddCommand(syncStatusCmd)

	syncDiffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Generate a sync diff plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncDiff(cmd.Context(), resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy}))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncDiffCmd.Flags().StringVar(ctx.syncTarget, "target", "git", "Sync target: git, s3, or cloud")
	syncDiffCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", true, "Only generate the sync plan; do not write the vault or remote")
	syncDiffCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known cloud base revision")
	syncDiffCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Cloud remote revision for tests or fake backends")
	addPathPolicyFlag(syncDiffCmd)
	_ = syncDiffCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.AddCommand(syncDiffCmd)
	syncPushCmd := &cobra.Command{
		Use:   "push",
		Short: "Record sync push state",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncPush(cmd.Context(), resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy}))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncPushCmd.Flags().StringVar(ctx.syncTarget, "target", "git", "Sync target: git, s3, or cloud")
	syncPushCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only generate the sync plan; do not write the vault or remote")
	syncPushCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known cloud base revision")
	syncPushCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Cloud remote revision for tests or fake backends")
	syncPushCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync state writes")
	addPathPolicyFlag(syncPushCmd)
	_ = syncPushCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.AddCommand(syncPushCmd)
	syncPullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Record sync pull state",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncPull(cmd.Context(), resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy}))
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncPullCmd.Flags().StringVar(ctx.syncTarget, "target", "git", "Sync target: git, s3, or cloud")
	syncPullCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only generate the sync plan; do not write the vault or remote")
	syncPullCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known cloud base revision")
	syncPullCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Cloud remote revision for tests or fake backends")
	syncPullCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync state writes")
	addPathPolicyFlag(syncPullCmd)
	_ = syncPullCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.AddCommand(syncPullCmd)

	logsCmd := &cobra.Command{Use: "logs", Short: "Inspect sync run receipts and timeline"}
	logsListCmd := &cobra.Command{Use: "list", Short: "List recent sync run receipts", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsList(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Limit: syncLogLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsListCmd.Flags().IntVar(&syncLogLimit, "limit", 20, "Maximum runs to list")
	logsShowCmd := &cobra.Command{Use: "show <run-id>", Short: "Show a sync run receipt", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsShow(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, RunID: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsTailCmd := &cobra.Command{Use: "tail", Short: "Tail the safe sync event timeline", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsTail(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Limit: syncLogLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsTailCmd.Flags().IntVar(&syncLogLimit, "limit", 20, "Maximum events to read")
	logsPruneCmd := &cobra.Command{Use: "prune", Short: "Prune old sync run receipts", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsPrune(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Keep: syncPruneKeep, MaxAgeDays: syncPruneMaxAgeDays, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsPruneCmd.Flags().IntVar(&syncPruneKeep, "keep", 200, "Keep at most this many recent sync runs")
	logsPruneCmd.Flags().IntVar(&syncPruneMaxAgeDays, "max-age-days", 90, "Delete sync runs older than this many days")
	logsPruneCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm deleting sync run receipts")
	logsCmd.AddCommand(logsListCmd, logsShowCmd, logsTailCmd, logsPruneCmd)
	syncCmd.AddCommand(logsCmd)

	daemonCmd := &cobra.Command{Use: "daemon", Short: "Run the local Cloud Sync daemon"}
	daemonRunCmd := &cobra.Command{Use: "run", Short: "Run the sync daemon in the foreground", RunE: func(cmd *cobra.Command, args []string) error {
		mode := ctx.outputMode()
		streamSeq := 1
		live := syncDaemonLiveSink(cmd.OutOrStdout(), mode, &streamSeq)
		if mode == output.ModeEvents {
			if err := writeSyncDaemonStreamEvent(cmd.OutOrStdout(), map[string]any{"type": "start", "seq": streamSeq, "status": "running"}); err != nil {
				return err
			}
		}
		projection, err := ctx.svc.SyncDaemonRun(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, Once: daemonOnce, PollInterval: daemonPollInterval, SyncTimeout: daemonSyncTimeout, LiveEvents: live})
		if mode == output.ModeEvents {
			endType := "end"
			if err != nil || projection.Status == "failed" {
				endType = "error"
			}
			streamSeq++
			_ = writeSyncDaemonStreamEvent(cmd.OutOrStdout(), map[string]any{"type": endType, "seq": streamSeq, "status": projection.Status, "summary": projection.Summary})
			return err
		}
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonStartCmd := &cobra.Command{Use: "start", Short: "Start the sync daemon in the background", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncDaemonStart(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, PollInterval: daemonPollInterval, SyncTimeout: daemonSyncTimeout})
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonStatusCmd := &cobra.Command{Use: "status", Short: "Show sync daemon status", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncDaemonStatus(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget})
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonStopCmd := &cobra.Command{Use: "stop", Short: "Request sync daemon shutdown", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncDaemonStop(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget})
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonLogsCmd := &cobra.Command{Use: "logs", Short: "Read sync daemon event logs", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncDaemonLogs(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, LogLimit: syncLogLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	for _, c := range []*cobra.Command{daemonRunCmd, daemonStartCmd} {
		c.Flags().StringVar(ctx.syncTarget, "target", "cloud", "Sync target: cloud")
		c.Flags().BoolVar(ctx.yes, "yes", false, "Confirm automatic sync writes")
		c.Flags().DurationVar(&daemonPollInterval, "poll-interval", time.Second, "Remote head poll interval")
		c.Flags().DurationVar(&daemonSyncTimeout, "sync-timeout", 30*time.Second, "Per-sync operation timeout")
		_ = c.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	}
	daemonRunCmd.Flags().BoolVar(&daemonOnce, "once", false, "Run one daemon sync cycle and exit")
	daemonLogsCmd.Flags().IntVar(&syncLogLimit, "limit", 20, "Maximum daemon events to read")
	daemonCmd.AddCommand(daemonRunCmd, daemonStartCmd, daemonStatusCmd, daemonStopCmd, daemonLogsCmd)
	syncCmd.AddCommand(daemonCmd)

	addSyncConflictsCommands(syncCmd, ctx)

	root.AddCommand(syncCmd)
}
