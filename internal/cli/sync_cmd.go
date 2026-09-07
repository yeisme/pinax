package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
	"github.com/yeisme/pinax/internal/profile"
	"golang.org/x/term"
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
	case "git", "s3", "capsa", "cloud", "pinax-cloud":
		return trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return "capsa"
	case "s3":
		return "s3"
	default:
		return ""
	}
}

func syncTargetCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	items := []string{
		"capsa\tCapsa encrypted sync backend",
		"cloud\tlegacy alias for Capsa",
		"pinax-cloud\tlegacy alias for Capsa",
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

type syncProgressStream struct {
	w        io.Writer
	mode     output.Mode
	seq      int
	terminal bool
	command  string
}

func newSyncProgressStream(cmd *cobra.Command, ctx commandBuildContext) *syncProgressStream {
	mode := ctx.outputMode()
	if mode == output.ModeJSON || mode == output.ModeAgent || mode == output.ModeExplain {
		return nil
	}
	progress := strings.ToLower(strings.TrimSpace(*ctx.syncProgress))
	if progress == "" {
		progress = "auto"
	}
	if progress == "never" {
		return nil
	}
	stream := &syncProgressStream{w: cmd.OutOrStdout(), mode: mode, seq: 1, command: syncProgressCommand(cmd)}
	if mode == output.ModeEvents {
		_ = stream.write(map[string]any{"type": "start", "status": "running"})
		return stream
	}
	stream.w = cmd.ErrOrStderr()
	if file, ok := stream.w.(*os.File); ok {
		stream.terminal = term.IsTerminal(int(file.Fd()))
	}
	return stream
}

// syncProgressCommand returns the stable projection command used by machine
// event consumers. Cobra's CommandPath includes the executable name, while
// the CLI output contract uses the existing dot-delimited command IDs.
func syncProgressCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return "sync"
	}
	parts := make([]string, 0, 3)
	for current := cmd; current != nil; current = current.Parent() {
		name := strings.TrimSpace(current.Name())
		if name == "" || name == "pinax" {
			break
		}
		parts = append(parts, name)
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	if len(parts) == 0 {
		return "sync"
	}
	return strings.Join(parts, ".")
}

func (s *syncProgressStream) sink() app.SyncEventSink {
	if s == nil {
		return nil
	}
	return func(event app.SyncEvent) {
		if s.mode == output.ModeEvents {
			s.seq++
			payload := map[string]any{"type": "progress", "seq": s.seq, "phase": event.Phase, "direction": event.Direction, "run_id": event.RunID, "completed": event.Completed, "total": event.Total, "bytes_completed": event.BytesCompleted, "bytes_total": event.BytesTotal, "operation": event.Operation, "change_code": event.ChangeCode, "path": event.Path, "path_hash": event.PathHash, "status": event.Status, "revision_id": event.RevisionID, "remote_write": event.RemoteWrite, "local_write": event.LocalWrite}
			if len(event.Facts) > 0 {
				payload["facts"] = event.Facts
			}
			_ = s.write(payload)
			return
		}
		status := event.Status
		if status == "" {
			status = "running"
		}
		line := fmt.Sprintf("sync %s %s", event.Phase, status)
		if event.Total > 0 {
			line = fmt.Sprintf("%s (%d/%d)", line, event.Completed, event.Total)
		}
		if s.terminal {
			_, _ = fmt.Fprintf(s.w, "\r%-72s", line)
		} else {
			_, _ = fmt.Fprintln(s.w, line)
		}
	}
}

func (s *syncProgressStream) write(payload map[string]any) error {
	payload["spec_version"] = "1.0"
	payload["mode"] = "events"
	payload["command"] = s.command
	enc := json.NewEncoder(s.w)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

func (s *syncProgressStream) finish(projection domain.Projection, err error) error {
	if s == nil {
		return nil
	}
	if s.mode == output.ModeEvents {
		s.seq++
		typ := "end"
		if err != nil || projection.Status == "failed" {
			typ = "error"
		}
		payload := map[string]any{"type": typ, "seq": s.seq, "status": projection.Status, "summary": projection.Summary}
		if len(projection.Facts) > 0 {
			payload["facts"] = projection.Facts
		}
		if projection.Error != nil {
			payload["error"] = projection.Error
		}
		return s.write(payload)
	}
	if s.terminal {
		_, _ = fmt.Fprintln(s.w)
	}
	return nil
}

func finishSyncUnlockError(cmd *cobra.Command, ctx commandBuildContext, stream *syncProgressStream, command string, err error) error {
	if err == nil {
		return nil
	}
	commandErr, ok := err.(*domain.CommandError)
	if !ok {
		commandErr = &domain.CommandError{Code: "sync_unlock", Message: err.Error(), Hint: "Check the repository unlock source and try again"}
	}
	projection := domain.NewErrorProjection(command, commandErr)
	return finishSyncCommand(cmd, ctx, stream, projection, commandErr)
}

func finishSyncCommand(cmd *cobra.Command, ctx commandBuildContext, stream *syncProgressStream, projection domain.Projection, err error) error {
	if stream != nil {
		if streamErr := stream.finish(projection, err); streamErr != nil {
			return streamErr
		}
		if stream.mode == output.ModeEvents {
			return err
		}
	}
	return ctx.renderProjection(cmd, projection, err)
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
	var syncLogFollow bool
	var syncPruneKeep int
	var syncPruneMaxAgeDays int
	var daemonPollInterval time.Duration
	var daemonSyncTimeout time.Duration
	var daemonOnce bool
	var daemonUnlock string
	var daemonUnlockRef string
	var daemonPassphraseFile string
	var daemonEnvVar string
	var syncPullPassphraseFile string
	var syncPullEnvVar string
	var syncPullUnlock string
	var syncPullUnlockRef string
	var syncDiffUnlock string
	var syncDiffUnlockRef string
	var syncDiffPassphraseFile string
	var syncDiffEnvVar string
	var syncPushUnlock string
	var syncPushUnlockRef string
	var syncPushPassphraseFile string
	var syncPushEnvVar string
	addSyncUnlockFlags := func(c *cobra.Command, unlock, unlockRef, passphraseFile, envVar *string) {
		c.Flags().StringVar(unlock, "unlock", "", "Unlock source: prompt|keychain|file|env")
		c.Flags().StringVar(unlockRef, "unlock-ref", "", "Keychain reference keychain://<service>/<account>")
		c.Flags().StringVar(passphraseFile, "passphrase-file", "", "Read repository-encrypted unlock passphrase from a 0600 regular file")
		c.Flags().StringVar(envVar, "env-var", "", "Read repository-encrypted unlock passphrase from a named environment variable")
	}
	addPathPolicyFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&syncPathPolicy, "path-policy", "default", "Path redaction policy for sync receipts: default, hash, or omitted")
		_ = c.RegisterFlagCompletionFunc("path-policy", staticCompletion("path-policy", "default", "hash", "omitted"))
	}
	addSyncViewFlags := func(c *cobra.Command) {
		c.Flags().StringVar(ctx.syncPreview, "preview", "status", "Sync preview: status, diff, or none")
		c.Flags().IntVar(ctx.syncLimit, "limit", 10, "Maximum sync changes to display; 0 shows statistics only")
		c.Flags().BoolVar(ctx.syncContentDiff, "content-diff", false, "Include bounded Markdown content diff")
		c.Flags().StringVar(ctx.syncProgress, "progress", "auto", "Progress output: auto, always, or never")
		_ = c.RegisterFlagCompletionFunc("preview", staticCompletion("preview", "status", "diff", "none"))
		_ = c.RegisterFlagCompletionFunc("progress", staticCompletion("progress", "auto", "always", "never"))
	}
	configureSyncRenderOptions := func(cmd *cobra.Command) {
		if ctx.renderOptions == nil {
			return
		}
		opts := ctx.renderOptions
		opts.SyncPreview = strings.TrimSpace(*ctx.syncPreview)
		if opts.SyncPreview == "" {
			opts.SyncPreview = "status"
		}
		opts.SyncLimit = *ctx.syncLimit
		if flag := cmd.Flags().Lookup("limit"); flag != nil {
			opts.SyncLimitSet = flag.Changed
		} else if flag := cmd.InheritedFlags().Lookup("limit"); flag != nil {
			opts.SyncLimitSet = flag.Changed
		}
		opts.ContentDiff = *ctx.syncContentDiff
	}
	syncRequestOptions := func() (string, int, bool, string) {
		preview := strings.TrimSpace(*ctx.syncPreview)
		if preview == "" {
			preview = "status"
		}
		return preview, *ctx.syncLimit, *ctx.syncContentDiff, strings.TrimSpace(*ctx.syncProgress)
	}
	validateSyncViewFlags := func(cmd *cobra.Command) error {
		preview, limit, _, progress := syncRequestOptions()
		if preview != "status" && preview != "diff" && preview != "none" {
			return renderCommandError(cmd, ctx.outputMode(), "sync.output", "invalid_preview", "sync preview must be status, diff, or none", "Use --preview status, --preview diff, or --preview none")
		}
		if limit < 0 {
			return renderCommandError(cmd, ctx.outputMode(), "sync.output", "invalid_limit", "sync limit must be zero or greater", "Use --limit 0 to hide paths or a positive limit")
		}
		if progress != "" && progress != "auto" && progress != "always" && progress != "never" {
			return renderCommandError(cmd, ctx.outputMode(), "sync.output", "invalid_progress", "sync progress must be auto, always, or never", "Use --progress auto, --progress always, or --progress never")
		}
		return nil
	}
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Generate and execute a one-command bidirectional sync plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			configureSyncRenderOptions(cmd)
			if err := validateSyncViewFlags(cmd); err != nil {
				return err
			}
			stream := newSyncProgressStream(cmd, ctx)
			preview, limit, contentDiff, progress := syncRequestOptions()
			request := resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, PathPolicy: syncPathPolicy, Preview: preview, Limit: limit, ContentDiff: contentDiff, Progress: progress, LiveEvents: stream.sink()})
			projection, err := ctx.svc.SyncAll(cmd.Context(), request)
			return finishSyncCommand(cmd, ctx, stream, projection, err)
		},
	}
	syncCmd.Flags().StringVar(ctx.syncTarget, "target", "capsa", "Sync target: capsa, git, s3, cloud, or pinax-cloud")
	_ = syncCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only run merge calculation")
	syncCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync writes")
	addPathPolicyFlag(syncCmd)
	addSyncViewFlags(syncCmd)

	syncInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Capsa sync configuration",
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

	syncKeysCmd := &cobra.Command{
		Use:   "keys",
		Short: "Show sync key derivation status and remote envelope key",
		Long:  "Report the active v2 and legacy key ids, and which derivation the remote manifest envelope is encrypted under. Use it to verify a re-encryption migration: remote_derivation should read v2 after pinax sync push re-encrypts legacy blobs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncKeysStatus(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	syncCmd.AddCommand(syncKeysCmd)

	syncDiffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Generate a sync diff plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			configureSyncRenderOptions(cmd)
			if err := validateSyncViewFlags(cmd); err != nil {
				return err
			}
			stream := newSyncProgressStream(cmd, ctx)
			source, err := resolveSyncUnlockSource(*ctx.vaultPath, syncDiffUnlock, syncDiffUnlockRef, syncDiffPassphraseFile, syncDiffEnvVar)
			if err != nil {
				return finishSyncUnlockError(cmd, ctx, stream, "sync.diff", err)
			}
			preview, limit, contentDiff, progress := syncRequestOptions()
			req := resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy, Preview: preview, Limit: limit, ContentDiff: contentDiff, Progress: progress, LiveEvents: stream.sink()})
			req.ProjectUnlockSource = source
			projection, err := ctx.svc.SyncDiff(cmd.Context(), req)
			return finishSyncCommand(cmd, ctx, stream, projection, err)
		},
	}
	syncDiffCmd.Flags().StringVar(ctx.syncTarget, "target", "capsa", "Sync target: capsa, git, s3, cloud, or pinax-cloud")
	syncDiffCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", true, "Only generate the sync plan; do not write the vault or remote")
	syncDiffCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known Capsa base revision")
	syncDiffCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Capsa remote revision for tests or fake backends")
	addPathPolicyFlag(syncDiffCmd)
	addSyncViewFlags(syncDiffCmd)
	addSyncUnlockFlags(syncDiffCmd, &syncDiffUnlock, &syncDiffUnlockRef, &syncDiffPassphraseFile, &syncDiffEnvVar)
	_ = syncDiffCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.AddCommand(syncDiffCmd)
	syncPushCmd := &cobra.Command{
		Use:   "push",
		Short: "Record sync push state",
		RunE: func(cmd *cobra.Command, args []string) error {
			configureSyncRenderOptions(cmd)
			if err := validateSyncViewFlags(cmd); err != nil {
				return err
			}
			stream := newSyncProgressStream(cmd, ctx)
			source, err := resolveSyncUnlockSource(*ctx.vaultPath, syncPushUnlock, syncPushUnlockRef, syncPushPassphraseFile, syncPushEnvVar)
			if err != nil {
				return finishSyncUnlockError(cmd, ctx, stream, "sync.push", err)
			}
			preview, limit, contentDiff, progress := syncRequestOptions()
			req := resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy, Preview: preview, Limit: limit, ContentDiff: contentDiff, Progress: progress, LiveEvents: stream.sink()})
			req.ProjectUnlockSource = source
			projection, err := ctx.svc.SyncPush(cmd.Context(), req)
			return finishSyncCommand(cmd, ctx, stream, projection, err)
		},
	}
	syncPushCmd.Flags().StringVar(ctx.syncTarget, "target", "capsa", "Sync target: capsa, git, s3, cloud, or pinax-cloud")
	syncPushCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only generate the sync plan; do not write the vault or remote")
	syncPushCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known Capsa base revision")
	syncPushCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Capsa remote revision for tests or fake backends")
	syncPushCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync state writes")
	addPathPolicyFlag(syncPushCmd)
	addSyncViewFlags(syncPushCmd)
	addSyncUnlockFlags(syncPushCmd, &syncPushUnlock, &syncPushUnlockRef, &syncPushPassphraseFile, &syncPushEnvVar)
	_ = syncPushCmd.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	syncCmd.AddCommand(syncPushCmd)
	syncPullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Record sync pull state",
		RunE: func(cmd *cobra.Command, args []string) error {
			configureSyncRenderOptions(cmd)
			if err := validateSyncViewFlags(cmd); err != nil {
				return err
			}
			stream := newSyncProgressStream(cmd, ctx)
			source, err := resolveSyncUnlockSource(*ctx.vaultPath, syncPullUnlock, syncPullUnlockRef, syncPullPassphraseFile, syncPullEnvVar)
			if err != nil {
				return finishSyncUnlockError(cmd, ctx, stream, "sync.pull", err)
			}
			preview, limit, contentDiff, progress := syncRequestOptions()
			req := resolveSyncRequest(app.SyncRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, DryRun: *ctx.syncDryRun, BaseRevision: *ctx.syncBaseRevision, RemoteRevision: *ctx.syncRemoteRevision, PathPolicy: syncPathPolicy, Preview: preview, Limit: limit, ContentDiff: contentDiff, Progress: progress, LiveEvents: stream.sink()})
			req.ProjectUnlockSource = source
			projection, err := ctx.svc.SyncPull(cmd.Context(), req)
			return finishSyncCommand(cmd, ctx, stream, projection, err)
		},
	}
	syncPullCmd.Flags().StringVar(ctx.syncTarget, "target", "capsa", "Sync target: capsa, git, s3, cloud, or pinax-cloud")
	syncPullCmd.Flags().BoolVar(ctx.syncDryRun, "dry-run", false, "Only generate the sync plan; do not write the vault or remote")
	syncPullCmd.Flags().StringVar(ctx.syncBaseRevision, "base-revision", "", "Locally known Capsa base revision")
	syncPullCmd.Flags().StringVar(ctx.syncRemoteRevision, "remote-revision", "", "Capsa remote revision for tests or fake backends")
	syncPullCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm sync state writes")
	addPathPolicyFlag(syncPullCmd)
	addSyncViewFlags(syncPullCmd)
	addSyncUnlockFlags(syncPullCmd, &syncPullUnlock, &syncPullUnlockRef, &syncPullPassphraseFile, &syncPullEnvVar)
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
	logsStatusCmd := &cobra.Command{Use: "status <run-id>", Short: "Replay a sync run status from the vault event JSONL", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsStatus(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, RunID: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsTailCmd := &cobra.Command{Use: "tail", Short: "Tail the safe sync event timeline", RunE: func(cmd *cobra.Command, args []string) error {
		if !syncLogFollow {
			projection, err := ctx.svc.SyncLogsTail(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Limit: syncLogLimit})
			return ctx.renderProjection(cmd, projection, err)
		}
		mode := ctx.outputMode()
		if mode == output.ModeJSON || mode == output.ModeExplain {
			return renderCommandError(cmd, mode, "sync.logs.tail", "sync_logs_follow_mode", "Follow mode requires a streaming output format", "Use --events, --agent, or default human output with --follow")
		}
		return ctx.svc.SyncLogsFollow(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Limit: syncLogLimit}, newSyncLogFollowEmitter(cmd.OutOrStdout(), mode))
	}}
	logsTailCmd.Flags().IntVar(&syncLogLimit, "limit", 20, "Maximum events to read")
	logsTailCmd.Flags().BoolVar(&syncLogFollow, "follow", false, "Continue streaming newly appended sync events")
	logsPruneCmd := &cobra.Command{Use: "prune", Short: "Prune old sync run receipts", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncLogsPrune(cmd.Context(), app.SyncLogsRequest{VaultPath: *ctx.vaultPath, Keep: syncPruneKeep, MaxAgeDays: syncPruneMaxAgeDays, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	logsPruneCmd.Flags().IntVar(&syncPruneKeep, "keep", 200, "Keep at most this many recent sync runs")
	logsPruneCmd.Flags().IntVar(&syncPruneMaxAgeDays, "max-age-days", 90, "Delete sync runs older than this many days")
	logsPruneCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm deleting sync run receipts")
	logsCmd.AddCommand(logsListCmd, logsShowCmd, logsStatusCmd, logsTailCmd, logsPruneCmd)
	syncCmd.AddCommand(logsCmd)

	var manifestDeviceID string
	var manifestPlanID string
	var manifestRemoteCapability string
	var manifestSave bool
	manifestCmd := &cobra.Command{Use: "manifest", Short: "Audit and migrate the object-first sync manifest"}
	manifestAuditCmd := &cobra.Command{Use: "audit", Short: "Audit v1 manifest object identity without writing", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncManifestAudit(cmd.Context(), app.SyncManifestMigrationRequest{VaultPath: *ctx.vaultPath, DeviceID: manifestDeviceID})
		return ctx.renderProjection(cmd, projection, err)
	}}
	manifestAuditCmd.Flags().StringVar(&manifestDeviceID, "device-id", "", "Device ID override for an unconfigured vault")
	manifestPlanCmd := &cobra.Command{Use: "plan", Short: "Generate a manifest v2 migration plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncManifestPlan(cmd.Context(), app.SyncManifestMigrationRequest{VaultPath: *ctx.vaultPath, DeviceID: manifestDeviceID, Save: manifestSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	manifestPlanCmd.Flags().StringVar(&manifestDeviceID, "device-id", "", "Device ID override for an unconfigured vault")
	manifestPlanCmd.Flags().BoolVar(&manifestSave, "save", false, "Save the migration plan for explicit promotion")
	manifestPromoteCmd := &cobra.Command{Use: "promote", Short: "Promote local sync manifest generation to v2", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncManifestPromote(cmd.Context(), app.SyncManifestMigrationRequest{VaultPath: *ctx.vaultPath, PlanID: manifestPlanID, RemoteCapability: manifestRemoteCapability, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	manifestPromoteCmd.Flags().StringVar(&manifestPlanID, "plan", "", "Saved manifest migration plan ID")
	manifestPromoteCmd.Flags().StringVar(&manifestRemoteCapability, "remote-capability", "", "Confirmed remote manifest capability: v2")
	manifestPromoteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm local manifest v2 promotion")
	manifestRollbackCmd := &cobra.Command{Use: "rollback", Short: "Roll back local promotion before the first v2 remote write", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.SyncManifestRollback(cmd.Context(), app.SyncManifestMigrationRequest{VaultPath: *ctx.vaultPath, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	manifestRollbackCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm local manifest promotion rollback")
	manifestCmd.AddCommand(manifestAuditCmd, manifestPlanCmd, manifestPromoteCmd, manifestRollbackCmd)
	syncCmd.AddCommand(manifestCmd)

	daemonCmd := &cobra.Command{Use: "daemon", Short: "Run the local Capsa sync daemon"}
	daemonRunCmd := &cobra.Command{Use: "run", Short: "Run the sync daemon in the foreground", RunE: func(cmd *cobra.Command, args []string) error {
		if daemonUnlock == "prompt" {
			return fmt.Errorf("--unlock prompt is not allowed for the daemon; use keychain, file, or env so the daemon never blocks on a TTY")
		}
		source, err := resolveSyncUnlockSource(*ctx.vaultPath, daemonUnlock, daemonUnlockRef, daemonPassphraseFile, daemonEnvVar)
		if err != nil {
			return err
		}
		mode := ctx.outputMode()
		streamSeq := 1
		liveWriter := cmd.ErrOrStderr()
		if mode == output.ModeEvents {
			liveWriter = cmd.OutOrStdout()
		}
		live := syncDaemonLiveSink(liveWriter, mode, &streamSeq)
		if mode == output.ModeEvents {
			if err := writeSyncDaemonStreamEvent(cmd.OutOrStdout(), map[string]any{"type": "start", "seq": streamSeq, "status": "running"}); err != nil {
				return err
			}
		}
		projection, err := ctx.svc.SyncDaemonRun(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, Once: daemonOnce, PollInterval: daemonPollInterval, SyncTimeout: daemonSyncTimeout, LiveEvents: live, ProjectUnlockSource: source})
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
		if daemonUnlock == "prompt" {
			return fmt.Errorf("--unlock prompt is not allowed for the daemon; use keychain, file, or env so the daemon never blocks on a TTY")
		}
		projection, err := ctx.svc.SyncDaemonStart(cmd.Context(), app.SyncDaemonRequest{VaultPath: *ctx.vaultPath, Target: *ctx.syncTarget, Yes: *ctx.yes, PollInterval: daemonPollInterval, SyncTimeout: daemonSyncTimeout, UnlockFlags: app.SyncDaemonUnlockFlags{Unlock: daemonUnlock, UnlockRef: daemonUnlockRef, PassphraseFile: daemonPassphraseFile, EnvVar: daemonEnvVar}})
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
		c.Flags().StringVar(ctx.syncTarget, "target", "capsa", "Sync target: capsa, cloud, or pinax-cloud")
		c.Flags().BoolVar(ctx.yes, "yes", false, "Confirm automatic sync writes")
		c.Flags().DurationVar(&daemonPollInterval, "poll-interval", time.Second, "Remote head poll interval")
		c.Flags().DurationVar(&daemonSyncTimeout, "sync-timeout", 30*time.Second, "Per-sync operation timeout")
		addSyncUnlockFlags(c, &daemonUnlock, &daemonUnlockRef, &daemonPassphraseFile, &daemonEnvVar)
		_ = c.RegisterFlagCompletionFunc("target", syncTargetCompletion)
	}
	daemonRunCmd.Flags().BoolVar(&daemonOnce, "once", false, "Run one daemon sync cycle and exit")
	daemonLogsCmd.Flags().IntVar(&syncLogLimit, "limit", 20, "Maximum daemon events to read")
	daemonInstallCmd := &cobra.Command{Use: "install", Short: "Install the sync daemon as a system service unit", RunE: func(cmd *cobra.Command, args []string) error {
		if !*ctx.yes {
			err := &domain.CommandError{Code: "approval_required", Message: "sync daemon install requires --yes", Hint: "Run pinax sync daemon install --vault <vault> --yes to write the service unit"}
			return ctx.renderProjection(cmd, domain.NewErrorProjection("sync.daemon.install", err), err)
		}
		projection, err := ctx.svc.SyncDaemonInstall(cmd.Context(), app.SyncDaemonInstallRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonUninstallCmd := &cobra.Command{Use: "uninstall", Short: "Remove the sync daemon system service unit", RunE: func(cmd *cobra.Command, args []string) error {
		if !*ctx.yes {
			err := &domain.CommandError{Code: "approval_required", Message: "sync daemon uninstall requires --yes", Hint: "Run pinax sync daemon uninstall --vault <vault> --yes to remove the service unit"}
			return ctx.renderProjection(cmd, domain.NewErrorProjection("sync.daemon.uninstall", err), err)
		}
		projection, err := ctx.svc.SyncDaemonUninstall(cmd.Context(), app.SyncDaemonInstallRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	daemonInstallCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm service unit install")
	daemonUninstallCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm service unit removal")
	daemonCmd.AddCommand(daemonRunCmd, daemonStartCmd, daemonStatusCmd, daemonStopCmd, daemonLogsCmd, daemonInstallCmd, daemonUninstallCmd)
	syncCmd.AddCommand(daemonCmd)

	addSyncConflictsCommands(syncCmd, ctx)
	addSyncRepoCommands(syncCmd, ctx)
	addSyncEnvCommands(syncCmd, ctx)

	root.AddCommand(syncCmd)
}

func newSyncLogFollowEmitter(w io.Writer, mode output.Mode) func(map[string]any) error {
	agentHeaderWritten := false
	return func(event map[string]any) error {
		projection := domain.NewProjection("sync.logs.tail", "Sync event streamed.")
		projection.Data = event
		output.ApplyProjectionRedaction(&projection)
		if sanitized, ok := projection.Data.(map[string]any); ok {
			event = sanitized
		}
		switch mode {
		case output.ModeAgent:
			if !agentHeaderWritten {
				if _, err := fmt.Fprintln(w, "spec_version=1.0\nmode=agent\ncommand=sync.logs.tail\nstatus=success"); err != nil {
					return err
				}
				agentHeaderWritten = true
			}
			seq := fmt.Sprint(event["seq"])
			for _, key := range []string{"type", "run_id", "direction", "kind", "path", "path_hash", "from_path", "to_path", "operation_status", "status", "backend_kind", "ts"} {
				if value := strings.TrimSpace(fmt.Sprint(event[key])); value != "" && value != "<nil>" {
					if _, err := fmt.Fprintf(w, "event.%s.%s=%s\n", seq, key, syncLogAgentValue(value)); err != nil {
						return err
					}
				}
			}
			return nil
		case output.ModeEvents:
			payload := map[string]any{"spec_version": "1.0", "mode": "events", "command": "sync.logs.tail", "type": "progress"}
			for key, value := range event {
				if key == "type" {
					payload["event_type"] = value
					continue
				}
				if key == "command" {
					payload["source_command"] = value
					continue
				}
				if key == "seq" {
					payload["timeline_seq"] = value
					continue
				}
				payload[key] = value
			}
			enc := json.NewEncoder(w)
			enc.SetEscapeHTML(false)
			return enc.Encode(payload)
		default:
			pathValue := firstSyncLogValue(event, "path", "path_hash", "to_path", "from_path")
			_, err := fmt.Fprintf(w, "%s %s %s %s %s %s\n", firstSyncLogValue(event, "type"), firstSyncLogValue(event, "direction"), firstSyncLogValue(event, "kind"), pathValue, firstSyncLogValue(event, "status"), firstSyncLogValue(event, "run_id"))
			return err
		}
	}
}

func firstSyncLogValue(event map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(event[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return "-"
}

func syncLogAgentValue(value string) string {
	if strings.ContainsAny(value, " \t\n\r\"'=$`\\") {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	return value
}
