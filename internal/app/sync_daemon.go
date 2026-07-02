package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		err := &domain.CommandError{Code: "approval_required", Message: "sync daemon run requires --yes", Hint: "Run pinax sync daemon run --target cloud --vault <vault> --yes after confirming automatic sync writes"}
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
		err := &domain.CommandError{Code: "approval_required", Message: "sync daemon start requires --yes", Hint: "Run pinax sync daemon start --target cloud --vault <vault> --yes after confirming automatic sync writes"}
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
	projection.Facts["target"] = syncDaemonDefault(state.Target, "cloud")
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
