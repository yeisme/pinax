package syncdaemon

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type RemotePoller interface {
	PollHead(ctx context.Context) (string, error)
}

type SyncExecutor interface {
	Pull(ctx context.Context, remoteRevision string) error
	Push(ctx context.Context) (string, error)
}

// EventSink receives the same redacted daemon events that are persisted locally.
type EventSink func(SyncDaemonEvent)

type Loop struct {
	Repo         Repository
	Target       string
	Poller       RemotePoller
	Executor     SyncExecutor
	PollInterval time.Duration
	SyncTimeout  time.Duration
	Backoff      Backoff
	EventSink    EventSink
}

func (l *Loop) RunOnce(ctx context.Context, localDirty bool, knownRemote string) (DaemonState, error) {
	return l.RunOnceWithTrigger(ctx, localDirty, knownRemote, "poll")
}

func (l *Loop) RunOnceWithTrigger(ctx context.Context, localDirty bool, knownRemote, trigger string) (DaemonState, error) {
	started := time.Now().UTC()
	cycleID := "cycle_" + strconv.FormatInt(started.UnixNano(), 10)
	state, _ := l.Repo.ReadState()
	if state.Target == "" {
		state.Target = l.Target
	}
	state.Status = StatusRunning
	state.LocalDirty = localDirty
	l.emit(SyncDaemonEvent{Type: "sync_started", Status: state.Status, LocalDirty: localDirty, Facts: map[string]any{"known_remote": knownRemote}}, trigger, cycleID)

	remoteRevision := knownRemote
	if l.Poller != nil {
		attemptCtx, cancel := l.attemptContext(ctx)
		revision, err := l.Poller.PollHead(attemptCtx)
		cancel()
		state.LastPollAt = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			state.Status = StatusDegraded
			state.LastErrorCode = errorCode(err)
			next := l.Backoff.Next(time.Now().UTC())
			state.NextRetryAt = next.Format(time.RFC3339)
			state.Message = err.Error()
			_ = l.Repo.WriteState(state)
			l.emit(SyncDaemonEvent{Type: "poll_failed", Status: state.Status, ErrorCode: state.LastErrorCode, Message: state.Message}, trigger, cycleID)
			l.emit(SyncDaemonEvent{Type: "sync_failed", Status: state.Status, ErrorCode: state.LastErrorCode, Message: state.Message, DurationMS: time.Since(started).Milliseconds()}, trigger, cycleID)
			return state, err
		}
		l.Backoff.Reset()
		remoteRevision = revision
	}
	state.RemoteRevision = remoteRevision
	if remoteRevision != "" && remoteRevision != knownRemote && l.Executor != nil {
		l.emit(SyncDaemonEvent{Type: "remote_change_detected", Status: state.Status, RemoteRevision: remoteRevision}, trigger, cycleID)
		l.emit(SyncDaemonEvent{Type: "pull_started", Status: state.Status, Direction: "pull", RemoteRevision: remoteRevision}, trigger, cycleID)
		attemptCtx, cancel := l.attemptContext(ctx)
		err := l.Executor.Pull(attemptCtx, remoteRevision)
		cancel()
		if err != nil {
			state.LastErrorCode = errorCode(err)
			state.Message = err.Error()
			if state.LastErrorCode == "conflict_required" {
				state.Status = StatusConflict
			} else {
				state.Status = StatusDegraded
			}
			_ = l.Repo.WriteState(state)
			l.emit(SyncDaemonEvent{Type: "pull_failed", Status: state.Status, Direction: "pull", RemoteRevision: remoteRevision, ErrorCode: state.LastErrorCode, Message: state.Message}, trigger, cycleID)
			l.emit(SyncDaemonEvent{Type: "sync_failed", Status: state.Status, ErrorCode: state.LastErrorCode, Message: state.Message, DurationMS: time.Since(started).Milliseconds()}, trigger, cycleID)
			return state, err
		}
		state.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
		l.emit(SyncDaemonEvent{Type: "pull_completed", Status: state.Status, Direction: "pull", RemoteRevision: remoteRevision, LocalWrite: true}, trigger, cycleID)
	}
	if localDirty && l.Executor != nil {
		l.emit(SyncDaemonEvent{Type: "local_change_detected", Status: state.Status, LocalDirty: true}, trigger, cycleID)
		l.emit(SyncDaemonEvent{Type: "push_started", Status: state.Status, Direction: "push"}, trigger, cycleID)
		attemptCtx, cancel := l.attemptContext(ctx)
		revision, err := l.Executor.Push(attemptCtx)
		cancel()
		if err != nil {
			state.LastErrorCode = errorCode(err)
			state.Message = err.Error()
			if state.LastErrorCode == "REVISION_CONFLICT" || state.LastErrorCode == "revision_conflict" {
				state.Status = StatusDegraded
				state.LocalDirty = true
				_ = l.Repo.WriteState(state)
				l.emit(SyncDaemonEvent{Type: "push_failed", Status: state.Status, Direction: "push", ErrorCode: state.LastErrorCode, Message: state.Message}, trigger, cycleID)
				l.emit(SyncDaemonEvent{Type: "sync_failed", Status: state.Status, ErrorCode: state.LastErrorCode, Message: state.Message, DurationMS: time.Since(started).Milliseconds()}, trigger, cycleID)
				return state, err
			}
			state.Status = StatusDegraded
			_ = l.Repo.WriteState(state)
			l.emit(SyncDaemonEvent{Type: "push_failed", Status: state.Status, Direction: "push", ErrorCode: state.LastErrorCode, Message: state.Message}, trigger, cycleID)
			l.emit(SyncDaemonEvent{Type: "sync_failed", Status: state.Status, ErrorCode: state.LastErrorCode, Message: state.Message, DurationMS: time.Since(started).Milliseconds()}, trigger, cycleID)
			return state, err
		}
		state.RemoteRevision = revision
		state.LocalDirty = false
		state.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
		l.emit(SyncDaemonEvent{Type: "push_completed", Status: state.Status, Direction: "push", RevisionID: revision, RemoteRevision: revision, RemoteWrite: true}, trigger, cycleID)
	}
	state.LastErrorCode = ""
	state.NextRetryAt = ""
	state.Message = ""
	state.Status = StatusRunning
	_ = l.Repo.WriteState(state)
	l.emit(SyncDaemonEvent{Type: "sync_succeeded", Status: state.Status, RemoteRevision: state.RemoteRevision, DurationMS: time.Since(started).Milliseconds()}, trigger, cycleID)
	return state, nil
}

func (l *Loop) emit(event SyncDaemonEvent, trigger, cycleID string) {
	if event.Target == "" {
		event.Target = l.Target
	}
	if event.Trigger == "" {
		event.Trigger = trigger
	}
	if event.CycleID == "" {
		event.CycleID = cycleID
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	event = PrepareEvent(event)
	_ = l.Repo.AppendEvent(event)
	if l.EventSink != nil {
		l.EventSink(event)
	}
}

func (l Loop) attemptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if l.SyncTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, l.SyncTimeout)
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, code := range []string{"conflict_required", "REVISION_CONFLICT", "revision_conflict", "transport_unavailable", "poll_unsupported", "lock_held"} {
		if strings.Contains(message, code) {
			return code
		}
	}
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return "sync_daemon_error"
}
