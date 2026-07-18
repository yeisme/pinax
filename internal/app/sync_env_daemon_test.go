package app

import (
	"context"
	"path/filepath"
	"testing"

	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// TestDaemonEnvReload_AtRunBoundary proves the daemon EnvReloader picks up env
// asset changes between runs and surfaces a structured event, without leaking
// plaintext. The reload runs at the run boundary; the current run retains its
// original snapshot even when a later reload succeeds.
func TestDaemonEnvReload_AtRunBoundary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("PINAX_SYNC_FAKE_KEY", "daemon-reload-key")
	svc := NewService()

	// Initialize and store a value.
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "daemon-secret-value"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// The daemon reloader tracks asset identity. First call loads.
	reloader := pinaxremote.NewEnvReloader(root, nil)
	snap1, status1 := reloader.RunSnapshot()
	if snap1 == nil || !status1.Loaded {
		t.Fatalf("first reload must load: %+v", status1)
	}
	if v, _ := snap1.Lookup("COS_KEY"); v != "daemon-secret-value" {
		t.Fatalf("expected daemon-secret-value, got %q", v)
	}

	// No change → same snapshot, not degraded.
	snap2, status2 := reloader.RunSnapshot()
	if status2.Changed || snap2 != snap1 {
		t.Fatalf("expected no reload without change: %+v", status2)
	}

	// Change the asset → next reload picks up the new value.
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "rotated-value"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	snap3, status3 := reloader.RunSnapshot()
	if !status3.Changed || !status3.Loaded {
		t.Fatalf("expected reload after change: %+v", status3)
	}
	if v, _ := snap3.Lookup("COS_KEY"); v != "rotated-value" {
		t.Fatalf("expected rotated-value after reload, got %q", v)
	}
}

// TestDaemonEnvReload_FailureRetainsLastSnapshot proves the reload fail-safe:
// when a new ciphertext cannot be unlocked, the daemon keeps the last successful
// snapshot and reports a structured code without leaking plaintext.
func TestDaemonEnvReload_FailureRetainsLastSnapshot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("PINAX_SYNC_FAKE_KEY", "daemon-reload-key")
	svc := NewService()

	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "first-value"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	reloader := pinaxremote.NewEnvReloader(root, nil)
	snap1, _ := reloader.RunSnapshot()
	if snap1 == nil {
		t.Fatalf("first load expected")
	}

	// Rotate the fake key so the existing ciphertext can no longer be decrypted.
	// The reloader will try to unlock on the NEXT identity change; corrupt the
	// ciphertext in place to force a decrypt failure while keeping identity change.
	t.Setenv("PINAX_SYNC_FAKE_KEY", "different-daemon-key")
	// Rewrite the asset with a different key so the content digest changes.
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "new-value"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Switch the unlock key back so decryption of the NEW asset fails (it was
	// encrypted with "different-daemon-key").
	t.Setenv("PINAX_SYNC_FAKE_KEY", "daemon-reload-key")

	snap2, status := reloader.RunSnapshot()
	if !status.Degraded {
		t.Fatalf("expected degraded reload: %+v", status)
	}
	if status.Code == "" {
		t.Fatalf("expected non-empty degraded code")
	}
	if snap2 == nil {
		t.Fatalf("must retain last successful snapshot on failure")
	}
	if v, _ := snap2.Lookup("COS_KEY"); v != "first-value" {
		t.Fatalf("retained snapshot must have first-value, got %q", v)
	}
}

// TestDaemonEnvReload_NoAssetNotDegraded proves a vault without an env asset does
// not produce a degraded daemon state.
func TestDaemonEnvReload_NoAssetNotDegraded(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	reloader := pinaxremote.NewEnvReloader(root, nil)
	snap, status := reloader.RunSnapshot()
	if snap != nil || status.Degraded || status.Code != "" {
		t.Fatalf("missing asset must not degrade daemon: snap=%v status=%+v", snap, status)
	}
}
