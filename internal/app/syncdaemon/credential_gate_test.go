package syncdaemon

import (
	"context"
	"strings"
	"testing"
	"time"
)

// denyingGate is a CredentialGate that forbids remote writes.
type denyingGate struct{ reason string }

func (d denyingGate) RemoteWriteAllowed() (bool, string) { return false, d.reason }

// allowingGate permits remote writes.
type allowingGate struct{}

func (allowingGate) RemoteWriteAllowed() (bool, string) { return true, "" }

type fakePoller struct{ rev string }

func (f fakePoller) PollHead(context.Context) (string, error) { return f.rev, nil }

type recordingExecutor struct {
	pulls int
	pushs int
}

func (r *recordingExecutor) Pull(context.Context, string) error { r.pulls++; return nil }
func (r *recordingExecutor) Push(context.Context) (string, error) {
	r.pushs++
	return "rev-push", nil
}

// TestLoopCredentialGateDeniesAndDegrades verifies that when the gate forbids
// remote writes (repository-encrypted credential unavailable / interactive),
// the daemon enters degraded state, emits a credential_degraded event, and does
// NOT call Pull or Push (pinax-passphrase-s3-bootstrap task 4.4).
func TestLoopCredentialGateDeniesAndDegrades(t *testing.T) {
	root := t.TempDir()
	repo := Repository{Root: root}
	exec := &recordingExecutor{}
	var events []SyncDaemonEvent
	sink := func(e SyncDaemonEvent) { events = append(events, e) }
	loop := &Loop{
		Repo:         repo,
		Target:       "test",
		Poller:       fakePoller{rev: "rev-1"},
		Executor:     exec,
		PollInterval: time.Second,
		SyncTimeout:  time.Second,
		Backoff:      Backoff{},
		EventSink:    sink,
		Gate:         denyingGate{reason: "repository-encrypted envelope missing"},
	}
	state, err := loop.RunOnce(context.Background(), true, "")
	if err == nil {
		t.Fatal("denied gate should return an error")
	}
	if state.Status != StatusDegraded {
		t.Fatalf("status: got %q want %q", state.Status, StatusDegraded)
	}
	if state.LastErrorCode != "sync_repo_unlock_required" {
		t.Fatalf("error code: %q", state.LastErrorCode)
	}
	if exec.pulls != 0 || exec.pushs != 0 {
		t.Fatalf("remote write executed despite denied gate: pulls=%d pushs=%d", exec.pulls, exec.pushs)
	}
	found := false
	for _, e := range events {
		if e.Type == "credential_degraded" && e.RemoteWrite == false {
			found = true
		}
	}
	if !found {
		t.Fatal("no credential_degraded event emitted")
	}
}

// TestLoopCredentialGateAllowedProceeds verifies the loop proceeds normally
// (pull/push reachable) when the gate allows remote writes.
func TestLoopCredentialGateAllowedProceeds(t *testing.T) {
	root := t.TempDir()
	repo := Repository{Root: root}
	exec := &recordingExecutor{}
	loop := &Loop{
		Repo:         repo,
		Target:       "test",
		Poller:       fakePoller{rev: ""},
		Executor:     exec,
		PollInterval: time.Second,
		SyncTimeout:  time.Second,
		Backoff:      Backoff{},
		EventSink:    func(SyncDaemonEvent) {},
		Gate:         allowingGate{},
	}
	state, _ := loop.RunOnce(context.Background(), true, "")
	if state.Status == StatusDegraded && strings.Contains(state.Message, "repository-encrypted") {
		t.Fatalf("allowed gate should not degrade on credential grounds: %+v", state)
	}
}

// TestLoopNilGatePreservesLegacyBehavior verifies a nil gate (no credential
// check) keeps the existing loop behavior.
func TestLoopNilGatePreservesLegacyBehavior(t *testing.T) {
	root := t.TempDir()
	repo := Repository{Root: root}
	exec := &recordingExecutor{}
	loop := &Loop{
		Repo: repo, Target: "test", Poller: fakePoller{rev: ""},
		Executor: exec, PollInterval: time.Second, SyncTimeout: time.Second,
		Backoff: Backoff{}, EventSink: func(SyncDaemonEvent) {}, Gate: nil,
	}
	state, _ := loop.RunOnce(context.Background(), false, "")
	if state.LastErrorCode == "sync_repo_unlock_required" {
		t.Fatal("nil gate should not set credential error")
	}
}
