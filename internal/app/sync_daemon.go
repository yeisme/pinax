package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

type SyncDaemonRequest struct {
	VaultPath    string
	Target       string
	Yes          bool
	Once         bool
	PollInterval time.Duration
	SyncTimeout  time.Duration
	LogLimit     int
	LiveEvents   syncdaemon.EventSink
}

func (s *Service) SyncDaemonRun(ctx context.Context, req SyncDaemonRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(SyncRequest{VaultPath: req.VaultPath, Target: req.Target})
	if err != nil {
		return errorProjection("sync.daemon.run", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync daemon run requires --yes", Hint: fmt.Sprintf("Run pinax sync daemon run --target %s --vault <vault> --yes after confirming automatic sync writes", syncOutputTarget(target))}
		return domain.NewErrorProjection("sync.daemon.run", err), err
	}
	lock, err := syncdaemon.AcquireRunnerLock(root)
	if err != nil {
		return commandErrorProjection("sync.daemon.run", err)
	}
	defer lock.Release()
	repo := syncdaemon.NewRepository(root)
	repo.ClearStopRequest()
	state := syncdaemon.NewState(target, os.Getpid(), syncdaemon.DetectionWatch, syncdaemon.StatusRunning)
	_ = repo.WriteState(state)
	emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.NewEvent("started", syncdaemon.StatusRunning, target))
	loop := syncdaemon.Loop{Repo: repo, Target: target, Poller: cloudDaemonPoller{root: root, req: SyncRequest{VaultPath: root, Target: target}}, Executor: cloudDaemonExecutor{s: s, root: root, target: target}, PollInterval: defaultDaemonPollInterval(req.PollInterval), SyncTimeout: defaultDaemonSyncTimeout(req.SyncTimeout), EventSink: req.LiveEvents}
	state, err = syncDaemonRunCycle(ctx, root, repo, loop, state, "startup")
	if req.Once || err != nil {
		return syncDaemonProjection("sync.daemon.run", "Sync daemon cycle completed.", root, state, nil), err
	}
	watcher, watchErr := syncdaemon.NewFSNotifyWatcher(root)
	var watchEvents <-chan []syncdaemon.WatchEvent
	var watchErrors <-chan error
	if watchErr != nil {
		state.DetectionMode = string(syncdaemon.DetectionScan)
		state.Status = syncdaemon.StatusDegraded
		state.LastErrorCode = "watch_degraded"
		state.Message = watchErr.Error()
		_ = repo.WriteState(state)
		emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.SyncDaemonEvent{Type: "watch_degraded", Status: state.Status, Target: target, ErrorCode: state.LastErrorCode, Message: state.Message})
	} else {
		defer func() { _ = watcher.Close() }()
		watchEvents = syncdaemon.Debounce(ctx, watcher.Events(), 250*time.Millisecond)
		watchErrors = watcher.Errors()
	}
	ticker := time.NewTicker(defaultDaemonPollInterval(req.PollInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			state.Status = syncdaemon.StatusStopped
			_ = repo.WriteState(state)
			emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.NewEvent("stopped", syncdaemon.StatusStopped, target))
			return syncDaemonProjection("sync.daemon.run", "Sync daemon stopped.", root, state, nil), nil
		case events, ok := <-watchEvents:
			if ok && len(events) > 0 {
				for _, event := range events {
					emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.SyncDaemonEvent{Type: "local_change_detected", Status: state.Status, Target: target, Path: event.Path, Trigger: "local_change"})
				}
				state, _ = syncDaemonRunCycle(ctx, root, repo, loop, state, "local_change")
			}
		case watchErr, ok := <-watchErrors:
			if ok && watchErr != nil {
				state.DetectionMode = string(syncdaemon.DetectionScan)
				state.Status = syncdaemon.StatusDegraded
				state.LastErrorCode = "watch_degraded"
				state.Message = watchErr.Error()
				_ = repo.WriteState(state)
				emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.SyncDaemonEvent{Type: "watch_degraded", Status: state.Status, Target: target, ErrorCode: state.LastErrorCode, Message: state.Message})
				watchEvents = nil
				watchErrors = nil
			}
		case <-ticker.C:
			if repo.StopRequested() {
				state.Status = syncdaemon.StatusStopped
				_ = repo.WriteState(state)
				emitSyncDaemonEvent(repo, req.LiveEvents, syncdaemon.NewEvent("stopped", syncdaemon.StatusStopped, target))
				repo.ClearStopRequest()
				return syncDaemonProjection("sync.daemon.run", "Sync daemon stopped.", root, state, nil), nil
			}
			state, _ = syncDaemonRunCycle(ctx, root, repo, loop, state, "poll")
		}
	}
}

func (s *Service) SyncDaemonStart(_ context.Context, req SyncDaemonRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(SyncRequest{VaultPath: req.VaultPath, Target: req.Target})
	if err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync daemon start requires --yes", Hint: fmt.Sprintf("Run pinax sync daemon start --target %s --vault <vault> --yes after confirming automatic sync writes", syncOutputTarget(target))}
		return domain.NewErrorProjection("sync.daemon.start", err), err
	}
	exe, err := os.Executable()
	if err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	repo := syncdaemon.NewRepository(root)
	if existing, readErr := repo.ReadState(); readErr == nil && existing.PID > 0 && (existing.Status == syncdaemon.StatusRunning || existing.Status == syncdaemon.StatusStopping) && syncdaemon.PIDAlive(existing.PID) {
		err := &domain.CommandError{Code: "lock_held", Message: "sync daemon is already running", Hint: "Run pinax sync daemon status --vault <vault> --json to inspect the current runner"}
		return domain.NewErrorProjection("sync.daemon.start", err), err
	}
	lock, lockErr := syncdaemon.AcquireRunnerLock(root)
	if lockErr != nil {
		return commandErrorProjection("sync.daemon.start", lockErr)
	}
	lock.Release()
	if err := os.MkdirAll(repo.Dir(), 0o700); err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	stdout, err := os.OpenFile(filepath.Join(repo.Dir(), "stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := os.OpenFile(filepath.Join(repo.Dir(), "stderr.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	defer func() { _ = stderr.Close() }()
	args := []string{"sync", "daemon", "run", "--target", target, "--vault", root, "--yes", "--poll-interval", defaultDaemonPollInterval(req.PollInterval).String(), "--sync-timeout", defaultDaemonSyncTimeout(req.SyncTimeout).String()}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return errorProjection("sync.daemon.start", err), err
	}
	_ = cmd.Process.Release()
	state := syncdaemon.NewState(target, cmd.Process.Pid, syncdaemon.DetectionWatch, syncdaemon.StatusRunning)
	_ = repo.WriteState(state)
	_ = repo.AppendEvent(syncdaemon.NewEvent("start_requested", syncdaemon.StatusRunning, target))
	projection := syncDaemonProjection("sync.daemon.start", "Sync daemon started.", root, state, nil)
	projection.Facts["pid"] = fmt.Sprint(cmd.Process.Pid)
	return projection, nil
}

func (s *Service) SyncDaemonStatus(_ context.Context, req SyncDaemonRequest) (domain.Projection, error) {
	root, _, err := cleanSyncRequest(SyncRequest{VaultPath: req.VaultPath, Target: req.Target})
	if err != nil {
		return errorProjection("sync.daemon.status", err), err
	}
	repo := syncdaemon.NewRepository(root)
	state, err := repo.ReadState()
	if err != nil {
		return errorProjection("sync.daemon.status", err), err
	}
	if state.PID > 0 && !syncdaemon.PIDAlive(state.PID) && state.Status == syncdaemon.StatusRunning {
		state.Status = syncdaemon.StatusStopped
		state.Message = "daemon process is not running"
		_ = repo.WriteState(state)
	}
	return syncDaemonProjection("sync.daemon.status", "Sync daemon status loaded.", root, state, nil), nil
}

func (s *Service) SyncDaemonStop(_ context.Context, req SyncDaemonRequest) (domain.Projection, error) {
	root, _, err := cleanSyncRequest(SyncRequest{VaultPath: req.VaultPath, Target: req.Target})
	if err != nil {
		return errorProjection("sync.daemon.stop", err), err
	}
	repo := syncdaemon.NewRepository(root)
	state, err := repo.ReadState()
	if err != nil {
		return errorProjection("sync.daemon.stop", err), err
	}
	_ = repo.RequestStop()
	if state.PID > 0 {
		if proc, findErr := os.FindProcess(state.PID); findErr == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}
	state.Status = syncdaemon.StatusStopping
	_ = repo.WriteState(state)
	_ = repo.AppendEvent(syncdaemon.NewEvent("stop_requested", syncdaemon.StatusStopping, state.Target))
	projection := syncDaemonProjection("sync.daemon.stop", "Sync daemon stop requested.", root, state, nil)
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax sync daemon status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) SyncDaemonLogs(_ context.Context, req SyncDaemonRequest) (domain.Projection, error) {
	root, _, err := cleanSyncRequest(SyncRequest{VaultPath: req.VaultPath, Target: req.Target})
	if err != nil {
		return errorProjection("sync.daemon.logs", err), err
	}
	repo := syncdaemon.NewRepository(root)
	events, err := repo.ReadEvents(req.LogLimit)
	if err != nil {
		return errorProjection("sync.daemon.logs", err), err
	}
	state, _ := repo.ReadState()
	return syncDaemonProjection("sync.daemon.logs", "Sync daemon logs loaded.", root, state, events), nil
}

func emitSyncDaemonEvent(repo syncdaemon.Repository, sink syncdaemon.EventSink, event syncdaemon.SyncDaemonEvent) {
	event = syncdaemon.PrepareEvent(event)
	_ = repo.AppendEvent(event)
	if sink != nil {
		sink(event)
	}
}

func syncDaemonProjection(command, summary, root string, state syncdaemon.DaemonState, events []syncdaemon.SyncDaemonEvent) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["target"] = syncOutputTarget(syncDaemonDefault(state.Target, syncTargetCapsa))
	addCapsaBridgeFacts(&projection, state.Target)
	projection.Facts["daemon_status"] = syncDaemonDefault(state.Status, syncdaemon.StatusStopped)
	projection.Facts["detection_mode"] = syncDaemonDefault(state.DetectionMode, string(syncdaemon.DetectionWatch))
	projection.Facts["local_dirty"] = fmt.Sprint(state.LocalDirty)
	projection.Facts["remote_revision"] = state.RemoteRevision
	projection.Facts["last_poll_at"] = state.LastPollAt
	projection.Facts["last_error_code"] = state.LastErrorCode
	projection.Facts["next_retry_at"] = state.NextRetryAt
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "sync-daemon", "daemon.json"))}
	projection.Data = map[string]any{"state": state, "events": events, "runtime_dir": filepath.ToSlash(filepath.Join(".pinax", "sync-daemon"))}
	return projection
}

func syncDaemonDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func defaultDaemonPollInterval(value time.Duration) time.Duration {
	if value <= 0 {
		return time.Second
	}
	return value
}

func defaultDaemonSyncTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return 30 * time.Second
	}
	return value
}

func syncDaemonRunCycle(ctx context.Context, root string, repo syncdaemon.Repository, loop syncdaemon.Loop, state syncdaemon.DaemonState, trigger string) (syncdaemon.DaemonState, error) {
	localHash, localDirty, hashErr := syncDaemonLocalDirty(root, state)
	if hashErr != nil {
		state.Status = syncdaemon.StatusDegraded
		state.LastErrorCode = "local_manifest_failed"
		state.Message = hashErr.Error()
		_ = repo.WriteState(state)
		return state, hashErr
	}
	state, err := loop.RunOnceWithTrigger(ctx, localDirty, state.RemoteRevision, trigger)
	if err != nil {
		return state, err
	}
	if postHash, _, postErr := syncDaemonLocalHash(root); postErr == nil {
		localHash = postHash
	}
	state.LocalHash = localHash
	state.LocalDirty = false
	_ = repo.WriteState(state)
	return state, nil
}

func syncDaemonLocalDirty(root string, state syncdaemon.DaemonState) (string, bool, error) {
	hash, hasContent, err := syncDaemonLocalHash(root)
	if err != nil {
		return "", false, err
	}
	if !hasContent {
		return hash, false, nil
	}
	return hash, hash != strings.TrimSpace(state.LocalHash), nil
}

func syncDaemonLocalHash(root string) (string, bool, error) {
	manifest, err := pinaxcloud.BuildManifest(root)
	if err != nil {
		return "", false, err
	}
	h := sha256.New()
	for _, entry := range manifest.Entries {
		_, _ = h.Write([]byte(entry.PathHash))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(entry.BlobID))
		_, _ = h.Write([]byte{0})
		_, _ = fmt.Fprint(h, entry.Mode)
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), len(manifest.Entries) > 0, nil
}

type cloudDaemonPoller struct {
	root string
	req  SyncRequest
}

func (p cloudDaemonPoller) PollHead(ctx context.Context) (string, error) {
	state, err := cloudStateForSync(p.root, p.req)
	if err != nil {
		return "", err
	}
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return "", err
	}
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		return "", err
	}
	return head.CurrentRevision, nil
}

type cloudDaemonExecutor struct {
	s      *Service
	root   string
	target string
}

func (e cloudDaemonExecutor) Pull(ctx context.Context, remoteRevision string) error {
	_, err := e.s.SyncPull(ctx, SyncRequest{VaultPath: e.root, Target: e.target, Yes: true, RemoteRevision: remoteRevision})
	return err
}

func (e cloudDaemonExecutor) Push(ctx context.Context) (string, error) {
	projection, err := e.s.SyncPush(ctx, SyncRequest{VaultPath: e.root, Target: e.target, Yes: true})
	if err != nil {
		return "", err
	}
	if projection.Facts != nil {
		return strings.TrimSpace(projection.Facts["revision_id"]), nil
	}
	return "", nil
}

// SyncDaemonInstallRequest configures a sync daemon service unit for a vault.
type SyncDaemonInstallRequest struct {
	VaultPath string
}

// SyncDaemonInstall writes a platform-specific service unit (systemd user unit
// on Linux, launchd plist on macOS) for the sync daemon. It does not start the
// service; the returned enable_command tells the user how to start it. Windows
// is unsupported and returns a platform_unsupported error.
func (s *Service) SyncDaemonInstall(_ context.Context, req SyncDaemonInstallRequest) (domain.Projection, error) {
	root, err := filepath.Abs(strings.TrimSpace(req.VaultPath))
	if err != nil {
		return errorProjection("sync.daemon.install", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("sync.daemon.install", err), err
	}
	exe, err := os.Executable()
	if err != nil {
		return errorProjection("sync.daemon.install", err), err
	}
	homeDir, _ := os.UserHomeDir()
	slug := daemonServiceSlug(root)
	unitPath, enableCommand, err := installDaemonUnit(runtime.GOOS, homeDir, exe, root, slug)
	if err != nil {
		return commandErrorProjection("sync.daemon.install", err)
	}
	envWritten := daemonWriteEnvFile(root)
	projection := domain.NewProjection("sync.daemon.install", "Sync daemon service unit installed.")
	projection.Facts["vault_path"] = root
	projection.Facts["binary"] = exe
	projection.Facts["slug"] = slug
	projection.Facts["unit_path"] = unitPath
	projection.Facts["enable_command"] = enableCommand
	projection.Facts["env_file"] = fmt.Sprint(envWritten)
	projection.Actions = []domain.Action{{Name: "enable", Command: enableCommand}}
	return projection, nil
}

// SyncDaemonUninstall removes the sync daemon service unit for a vault. It does
// not stop a running service; the caller is expected to stop it first.
func (s *Service) SyncDaemonUninstall(_ context.Context, req SyncDaemonInstallRequest) (domain.Projection, error) {
	root, err := filepath.Abs(strings.TrimSpace(req.VaultPath))
	if err != nil {
		return errorProjection("sync.daemon.uninstall", err), err
	}
	homeDir, _ := os.UserHomeDir()
	slug := daemonServiceSlug(root)
	unitPath, removed, err := uninstallDaemonUnit(runtime.GOOS, homeDir, slug)
	if err != nil {
		return commandErrorProjection("sync.daemon.uninstall", err)
	}
	projection := domain.NewProjection("sync.daemon.uninstall", "Sync daemon service unit removed.")
	projection.Facts["vault_path"] = root
	projection.Facts["slug"] = slug
	projection.Facts["unit_path"] = unitPath
	projection.Facts["removed"] = fmt.Sprint(removed)
	return projection, nil
}

// daemonServiceSlug produces a filesystem-safe service label from a vault path:
// the lowercase basename with every non-alphanumeric run collapsed to a single
// hyphen. An empty result falls back to "vault".
func daemonServiceSlug(vaultPath string) string {
	base := filepath.Base(strings.TrimRight(filepath.ToSlash(vaultPath), "/"))
	base = strings.ToLower(base)
	var b strings.Builder
	prevDash := true
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "vault"
	}
	return slug
}

// systemdUnitContent renders a systemd user service unit for the sync daemon.
// The EnvironmentFile directive is always emitted; daemonWriteEnvFile is
// responsible for creating the referenced file when a secret is available.
func systemdUnitContent(binary, vaultPath, slug string) string {
	envFile := filepath.Join(vaultPath, ".pinax", "cloud", "sync-env")
	return fmt.Sprintf(`[Unit]
Description=Pinax Capsa sync daemon (%s)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s sync daemon run --target capsa --vault %s --yes
Restart=on-failure
RestartSec=10
EnvironmentFile=%s

[Install]
WantedBy=default.target
`, slug, binary, vaultPath, envFile)
}

// launchdPlistContent renders a launchd agent plist for the sync daemon.
// RunAtLoad starts the daemon at login; KeepAlive with SuccessfulExit=false
// restarts it after any non-zero exit.
func launchdPlistContent(binary, vaultPath, slug string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.yeisme.capsa-sync.%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>sync</string>
        <string>daemon</string>
        <string>run</string>
        <string>--target</string>
        <string>capsa</string>
        <string>--vault</string>
        <string>%s</string>
        <string>--yes</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
</dict>
</plist>
`, slug, binary, vaultPath)
}

// systemdUnitPath returns the user unit file path for a slug.
func systemdUnitPath(homeDir, slug string) string {
	return filepath.Join(homeDir, ".config", "systemd", "user", "capsa-sync-"+slug+".service")
}

// launchdPlistPath returns the LaunchAgent plist path for a slug.
func launchdPlistPath(homeDir, slug string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", "com.yeisme.capsa-sync."+slug+".plist")
}

// installDaemonUnit writes the service unit for the given platform. It returns
// the written unit path and the user-facing enable command. Windows and any
// other unsupported platform yield a platform_unsupported error.
func installDaemonUnit(platform, homeDir, binary, vaultPath, slug string) (string, string, error) {
	switch platform {
	case "linux":
		unitPath := systemdUnitPath(homeDir, slug)
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o700); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(unitPath, []byte(systemdUnitContent(binary, vaultPath, slug)), 0o644); err != nil {
			return "", "", err
		}
		return unitPath, "systemctl --user enable --now capsa-sync-" + slug, nil
	case "darwin":
		unitPath := launchdPlistPath(homeDir, slug)
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o700); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(unitPath, []byte(launchdPlistContent(binary, vaultPath, slug)), 0o644); err != nil {
			return "", "", err
		}
		return unitPath, "launchctl load " + unitPath, nil
	default:
		err := &domain.CommandError{Code: "platform_unsupported", Message: fmt.Sprintf("sync daemon service install is not supported on %s", platform), Hint: "Use 'pinax sync daemon run' to run the daemon in the foreground"}
		return "", "", err
	}
}

// uninstallDaemonUnit removes the service unit for the given platform. It does
// not stop a running service. A missing unit file is reported as removed=false
// rather than an error. Unsupported platforms yield platform_unsupported.
func uninstallDaemonUnit(platform, homeDir, slug string) (string, bool, error) {
	var unitPath string
	switch platform {
	case "linux":
		unitPath = systemdUnitPath(homeDir, slug)
	case "darwin":
		unitPath = launchdPlistPath(homeDir, slug)
	default:
		err := &domain.CommandError{Code: "platform_unsupported", Message: fmt.Sprintf("sync daemon service uninstall is not supported on %s", platform), Hint: "Use 'pinax sync daemon run' to run the daemon in the foreground"}
		return "", false, err
	}
	if _, statErr := os.Stat(unitPath); statErr != nil {
		return unitPath, false, nil
	}
	if err := os.Remove(unitPath); err != nil {
		return unitPath, false, err
	}
	return unitPath, true, nil
}

// daemonWriteEnvFile writes <vault>/.pinax/cloud/sync-env (mode 0600) containing
// PINAX_SYNC_SECRET when that variable is set in the current environment. It
// returns true when the file was written and false (no error) when the secret
// is absent, so callers can record the outcome without treating absence as a
// failure.
func daemonWriteEnvFile(root string) bool {
	value, ok := os.LookupEnv("PINAX_SYNC_SECRET")
	if !ok || strings.TrimSpace(value) == "" {
		return false
	}
	envDir := filepath.Join(root, ".pinax", "cloud")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		return false
	}
	content := "PINAX_SYNC_SECRET=" + value + "\n"
	return os.WriteFile(filepath.Join(envDir, "sync-env"), []byte(content), 0o600) == nil
}
