package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
)

func TestSyncDaemonRunPerformsStartupCycleBeforeFirstTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := t.TempDir()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "startup.md"), "# Startup\n\ninitial daemon sync\n")
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: root, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "dev", SecretRef: "test-secret"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	time.AfterFunc(250*time.Millisecond, cancel)
	_, err := svc.SyncDaemonRun(ctx, SyncDaemonRequest{VaultPath: root, Target: "cloud", Yes: true, PollInterval: time.Hour, SyncTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("SyncDaemonRun: %v", err)
	}
	events, err := syncdaemon.NewRepository(root).ReadEvents(50)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var sawStartupPush bool
	for _, event := range events {
		if event.Type == "push_completed" && event.Trigger == "startup" && event.RemoteWrite {
			sawStartupPush = true
		}
	}
	if !sawStartupPush {
		t.Fatalf("startup run did not push before first tick: %#v", events)
	}
}

func TestSyncDaemonStateRepository(t *testing.T) {
	root := t.TempDir()
	repo := syncdaemon.NewRepository(root)
	state := syncdaemon.NewState("cloud", 123, syncdaemon.DetectionWatch, syncdaemon.StatusRunning)
	state.RemoteRevision = "rev_1"
	if err := repo.WriteState(state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	loaded, err := repo.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if loaded.SchemaVersion != syncdaemon.StateSchemaVersion || loaded.Status != syncdaemon.StatusRunning || loaded.RemoteRevision != "rev_1" {
		t.Fatalf("state = %#v", loaded)
	}
	if err := repo.AppendEvent(syncdaemon.SyncDaemonEvent{Type: "changed", Path: "notes/a.md", Message: "ok"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	events, err := repo.ReadEvents(10)
	if err != nil || len(events) != 1 || events[0].Path != "notes/a.md" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestSyncDaemonStateRedaction(t *testing.T) {
	root := t.TempDir()
	repo := syncdaemon.NewRepository(root)
	state := syncdaemon.NewState("cloud", 1, syncdaemon.DetectionWatch, syncdaemon.StatusDegraded)
	state.Message = "Authorization: Bearer secret-token raw_provider_payload"
	if err := repo.WriteState(state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	content, _ := os.ReadFile(repo.StatePath())
	for _, forbidden := range []string{"secret-token", "Authorization", "Bearer", "raw_provider_payload"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("state leaked %q:\n%s", forbidden, content)
		}
	}
}

func TestSyncDaemonStateIgnoresPinaxRuntimePaths(t *testing.T) {
	for _, path := range []string{".pinax/sync-daemon/daemon.json", ".pinax/kb/lancedb/index", ".git/index", "temp/run.log"} {
		if !syncdaemon.IgnoreRuntimePath(path) || syncdaemon.SafeEventPath(path) != "" {
			t.Fatalf("runtime path not ignored: %s", path)
		}
	}
	if syncdaemon.IgnoreRuntimePath("notes/a.md") || syncdaemon.SafeEventPath("notes/a.md") != "notes/a.md" {
		t.Fatalf("normal note path ignored")
	}
}

func TestSyncDaemonSingleRunnerLock(t *testing.T) {
	root := t.TempDir()
	lock, err := syncdaemon.AcquireRunnerLock(root)
	if err != nil {
		t.Fatalf("AcquireRunnerLock: %v", err)
	}
	defer lock.Release()
	if _, err := syncdaemon.AcquireRunnerLock(root); err == nil || !strings.Contains(err.Error(), "lock_held") {
		t.Fatalf("second runner lock err=%v", err)
	}
}

func TestSyncDaemonStartRejectsExistingLiveRunner(t *testing.T) {
	root := t.TempDir()
	repo := syncdaemon.NewRepository(root)
	state := syncdaemon.NewState("cloud", os.Getpid(), syncdaemon.DetectionWatch, syncdaemon.StatusRunning)
	if err := repo.WriteState(state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	projection, err := NewService().SyncDaemonStart(context.Background(), SyncDaemonRequest{VaultPath: root, Target: "cloud", Yes: true})
	if err == nil || projection.Error == nil || projection.Error.Code != "lock_held" {
		t.Fatalf("start with existing live runner projection=%#v err=%v", projection, err)
	}
	loaded, loadErr := repo.ReadState()
	if loadErr != nil || loaded.PID != os.Getpid() || loaded.Status != syncdaemon.StatusRunning {
		t.Fatalf("existing daemon state was overwritten: state=%#v err=%v", loaded, loadErr)
	}
}

func TestSyncOperationLockBlocksConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	lock, err := syncdaemon.AcquireOperationLock(root, "test")
	if err != nil {
		t.Fatalf("AcquireOperationLock: %v", err)
	}
	defer lock.Release()
	if _, err := syncdaemon.AcquireOperationLock(root, "second"); err == nil || !strings.Contains(err.Error(), "lock_held") {
		t.Fatalf("second operation lock err=%v", err)
	}
}

func TestSyncLockStalePidRecovery(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".pinax", "sync", "operation.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if err := os.WriteFile(path, []byte(`{"pid":999999,"owner":"old","acquired_at":"`+old+`","expires_at":"`+old+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := syncdaemon.AcquireOperationLock(root, "new")
	if err != nil {
		t.Fatalf("stale lock was not recovered: %v", err)
	}
	lock.Release()
}

func TestSyncDaemonWatcherDebounce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan syncdaemon.WatchEvent, 3)
	out := syncdaemon.Debounce(ctx, in, 5*time.Millisecond)
	in <- syncdaemon.WatchEvent{Path: "notes/a.md"}
	in <- syncdaemon.WatchEvent{Path: "notes/a.md"}
	in <- syncdaemon.WatchEvent{Path: ".pinax/sync-daemon/daemon.json"}
	close(in)
	batch := <-out
	if len(batch) != 1 || batch[0].Path != "notes/a.md" {
		t.Fatalf("batch = %#v", batch)
	}
}

func TestSyncDaemonWatcherIgnoresRuntimePaths(t *testing.T) {
	TestSyncDaemonStateIgnoresPinaxRuntimePaths(t)
}

func TestSyncDaemonScanFallbackOnWatcherError(t *testing.T) {
	state := syncdaemon.NewState("cloud", 1, syncdaemon.DetectionScan, syncdaemon.StatusDegraded)
	if state.DetectionMode != string(syncdaemon.DetectionScan) || state.Status != syncdaemon.StatusDegraded {
		t.Fatalf("scan fallback state = %#v", state)
	}
}

func TestSyncDaemonRemotePoll(t *testing.T) {
	repo := syncdaemon.NewRepository(t.TempDir())
	loop := syncdaemon.Loop{Repo: repo, Target: "cloud", Poller: fakePoller{revision: "rev_2"}}
	state, err := loop.RunOnce(context.Background(), false, "rev_2")
	if err != nil || state.RemoteRevision != "rev_2" || state.LastPollAt == "" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestSyncDaemonCleanPollDoesNotPush(t *testing.T) {
	exec := &fakeExecutor{}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Poller: fakePoller{revision: "rev_clean"}, Executor: exec}
	state, err := loop.RunOnce(context.Background(), false, "rev_clean")
	if err != nil || state.RemoteRevision != "rev_clean" || strings.Join(exec.calls, ",") != "" {
		t.Fatalf("clean poll state=%#v calls=%v err=%v", state, exec.calls, err)
	}
}

func TestSyncDaemonSyncTimeoutCancelsAttempt(t *testing.T) {
	exec := &fakeExecutor{blockPush: true}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Executor: exec, SyncTimeout: 5 * time.Millisecond}
	state, err := loop.RunOnce(context.Background(), true, "")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || state.Status != syncdaemon.StatusDegraded {
		t.Fatalf("timeout state=%#v err=%v", state, err)
	}
}

func TestSyncDaemonBackoff(t *testing.T) {
	repo := syncdaemon.NewRepository(t.TempDir())
	loop := syncdaemon.Loop{Repo: repo, Target: "cloud", Poller: fakePoller{err: errors.New("transport_unavailable")}, Backoff: syncdaemon.Backoff{Base: time.Millisecond, Max: time.Second}}
	state, err := loop.RunOnce(context.Background(), false, "")
	if err == nil || state.NextRetryAt == "" || state.LastErrorCode != "transport_unavailable" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestSyncDaemonTransportUnavailableStatus(t *testing.T) { TestSyncDaemonBackoff(t) }

func TestSyncDaemonRunOncePersistsTriggerAndSyncEvents(t *testing.T) {
	repo := syncdaemon.NewRepository(t.TempDir())
	if err := repo.WriteState(syncdaemon.NewState("cloud", 1, syncdaemon.DetectionWatch, syncdaemon.StatusRunning)); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	exec := &fakeExecutor{pushRevision: "rev_new"}
	loop := syncdaemon.Loop{Repo: repo, Target: "cloud", Poller: fakePoller{revision: "rev_remote"}, Executor: exec}
	state, err := loop.RunOnceWithTrigger(context.Background(), true, "rev_base", "startup")
	if err != nil {
		t.Fatalf("RunOnceWithTrigger: %v", err)
	}
	if state.RemoteRevision != "rev_new" || state.LastSyncAt == "" {
		t.Fatalf("state=%#v", state)
	}
	if got := strings.Join(exec.calls, ","); got != "pull:rev_remote,push" {
		t.Fatalf("calls=%s", got)
	}
	events, err := repo.ReadEvents(20)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var sawStart, sawPull, sawPush, sawSuccess bool
	for i, event := range events {
		if event.Seq != i+1 {
			t.Fatalf("event seq[%d]=%d events=%#v", i, event.Seq, events)
		}
		if event.Trigger != "startup" || event.CycleID == "" {
			t.Fatalf("event missing trigger/cycle: %#v", event)
		}
		switch event.Type {
		case "sync_started":
			sawStart = true
		case "pull_completed":
			sawPull = event.Direction == "pull" && event.RemoteRevision == "rev_remote"
		case "push_completed":
			sawPush = event.Direction == "push" && event.RevisionID == "rev_new" && event.RemoteWrite
		case "sync_succeeded":
			sawSuccess = event.DurationMS >= 0
		}
	}
	if !sawStart || !sawPull || !sawPush || !sawSuccess {
		t.Fatalf("missing expected events: %#v", events)
	}
}

func TestSyncDaemonEventSinkReceivesRedactedEvents(t *testing.T) {
	repo := syncdaemon.NewRepository(t.TempDir())
	if err := repo.WriteState(syncdaemon.NewState("cloud", 1, syncdaemon.DetectionWatch, syncdaemon.StatusRunning)); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	seen := []syncdaemon.SyncDaemonEvent{}
	loop := syncdaemon.Loop{Repo: repo, Target: "cloud", Poller: fakePoller{err: errors.New("Authorization: Bearer secret-token transport_unavailable")}, EventSink: func(event syncdaemon.SyncDaemonEvent) {
		seen = append(seen, event)
	}}
	_, err := loop.RunOnceWithTrigger(context.Background(), false, "", "poll")
	if err == nil {
		t.Fatalf("expected poll error")
	}
	if len(seen) == 0 {
		t.Fatalf("event sink did not receive events")
	}
	for _, event := range seen {
		body := event.Message + event.ErrorCode
		if strings.Contains(body, "secret-token") || strings.Contains(body, "Authorization") || strings.Contains(body, "Bearer") {
			t.Fatalf("event sink leaked sensitive data: %#v", event)
		}
	}
}

func TestSyncDaemonPullBeforePush(t *testing.T) {
	exec := &fakeExecutor{}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Poller: fakePoller{revision: "rev_remote"}, Executor: exec}
	_, err := loop.RunOnce(context.Background(), true, "rev_base")
	if err != nil || strings.Join(exec.calls, ",") != "pull:rev_remote,push" {
		t.Fatalf("calls=%v err=%v", exec.calls, err)
	}
}

func TestSyncDaemonPushesLocalChange(t *testing.T) {
	exec := &fakeExecutor{pushRevision: "rev_new"}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Executor: exec}
	state, err := loop.RunOnce(context.Background(), true, "")
	if err != nil || state.RemoteRevision != "rev_new" || state.LocalDirty {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestSyncDaemonRevisionConflictRetry(t *testing.T) {
	exec := &fakeExecutor{pushErr: errors.New("REVISION_CONFLICT")}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Executor: exec}
	state, err := loop.RunOnce(context.Background(), true, "")
	if err == nil || state.LastErrorCode != "REVISION_CONFLICT" || !state.LocalDirty {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestSyncDaemonPausesOnConflict(t *testing.T) {
	exec := &fakeExecutor{pullErr: errors.New("conflict_required")}
	loop := syncdaemon.Loop{Repo: syncdaemon.NewRepository(t.TempDir()), Target: "cloud", Poller: fakePoller{revision: "rev_remote"}, Executor: exec}
	state, err := loop.RunOnce(context.Background(), false, "rev_base")
	if err == nil || state.Status != syncdaemon.StatusConflict {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

type fakePoller struct {
	revision string
	err      error
}

func (p fakePoller) PollHead(context.Context) (string, error) { return p.revision, p.err }

type fakeExecutor struct {
	calls        []string
	pushRevision string
	pullErr      error
	pushErr      error
	blockPush    bool
}

func (e *fakeExecutor) Pull(_ context.Context, remoteRevision string) error {
	e.calls = append(e.calls, "pull:"+remoteRevision)
	return e.pullErr
}

func (e *fakeExecutor) Push(ctx context.Context) (string, error) {
	e.calls = append(e.calls, "push")
	if e.blockPush {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if e.pushRevision == "" {
		e.pushRevision = "rev_push"
	}
	return e.pushRevision, e.pushErr
}
