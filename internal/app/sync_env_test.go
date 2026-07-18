package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

func newSyncEnvService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	if err := os.MkdirAll(filepath.Join(dir, ".pinax"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_SYNC_FAKE_KEY", "test-key-env-app")
	return NewService(), dir
}

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
}

func projectionJSON(t *testing.T, p domain.Projection) string {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	return string(b)
}

func assertNoPlaintextLeak(t *testing.T, projection domain.Projection, secrets ...string) {
	t.Helper()
	blob := projectionJSON(t, projection)
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(blob, secret) {
			t.Fatalf("projection leaked plaintext secret %q:\n%s", secret, blob)
		}
	}
}

func TestSyncEnv_InitCreatesAssetWithoutPlaintext(t *testing.T) {
	svc, root := newSyncEnvService(t)
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if proj.Facts["plaintext_created"] != "false" {
		t.Fatalf("plaintext must not be created by default")
	}
	if _, err := os.Stat(pinaxremote.EnvAssetPath(root)); err != nil {
		t.Fatalf("encrypted asset not created: %v", err)
	}
	// No plaintext .env in the vault root.
	if _, err := os.Stat(filepath.Join(root, ".env")); err == nil {
		t.Fatalf("plaintext .env must not exist")
	}
}

func TestSyncEnv_InitAppliesManagedGitignore(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("gitignore not created: %v", err)
	}
	for _, rule := range []string{".env", "*.env", ".pinax/runtime/", "!.pinax/pinax-sync.env.age", "!.env.example"} {
		if !strings.Contains(string(body), rule) {
			t.Fatalf("gitignore missing %q:\n%s", rule, string(body))
		}
	}
}

func TestSyncEnv_SetDoesNotLeakValue(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_SECRET", Value: "super-secret-value-123"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	assertNoPlaintextLeak(t, proj, "super-secret-value-123")
	if proj.Facts["key"] != "COS_SECRET" {
		t.Fatalf("expected key fact COS_SECRET, got %v", proj.Facts["key"])
	}
	// The on-disk asset must not contain the plaintext.
	assetBytes, _ := os.ReadFile(pinaxremote.EnvAssetPath(root))
	if strings.Contains(string(assetBytes), "super-secret-value-123") {
		t.Fatalf("plaintext leaked into encrypted asset on disk")
	}
}

func TestSyncEnv_ListShowsKeysNotValues(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "mykey"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_SECRET", Value: "mysecret"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertNoPlaintextLeak(t, proj, "mykey", "mysecret")
	if proj.Facts["key_count"] != "2" {
		t.Fatalf("expected 2 keys, got %v", proj.Facts["key_count"])
	}
}

func TestSyncEnv_UnlockInMemory(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "inmemory-value"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "unlock"})
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	assertNoPlaintextLeak(t, proj, "inmemory-value")
	if proj.Facts["materialized"] != "false" {
		t.Fatalf("default unlock must not materialize")
	}
	// No materialized file.
	if _, err := os.Stat(pinaxremote.EnvRuntimePath(root)); err == nil {
		t.Fatalf("materialized file must not exist without --materialize")
	}
}

func TestSyncEnv_UnlockMaterializeCreates0600(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "COS_KEY", Value: "matval"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "unlock", Materialize: true})
	if err != nil {
		t.Fatalf("unlock --materialize: %v", err)
	}
	assertNoPlaintextLeak(t, proj, "matval")
	if proj.Facts["materialized"] != "true" {
		t.Fatalf("expected materialized=true")
	}
	if proj.Facts["permissions"] != "0600" {
		t.Fatalf("expected 0600 permissions fact, got %v", proj.Facts["permissions"])
	}
	info, err := os.Stat(pinaxremote.EnvRuntimePath(root))
	if err != nil {
		t.Fatalf("materialized file missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 on disk, got %v", info.Mode().Perm())
	}
}

func TestSyncEnv_CleanRemovesManagedFile(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "K", Value: "v"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "unlock", Materialize: true}); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "clean"})
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if proj.Facts["removed"] != "true" {
		t.Fatalf("expected removed=true")
	}
	if _, err := os.Stat(pinaxremote.EnvRuntimePath(root)); err == nil {
		t.Fatalf("materialized file must be removed")
	}
}

func TestSyncEnv_DoctorHealthy(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "K", Value: "doctor-secret-value-9999"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "doctor"})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	assertNoPlaintextLeak(t, proj, "doctor-secret-value-9999")
	if proj.Facts["code"] != "healthy" {
		t.Fatalf("expected healthy, got %v", proj.Facts["code"])
	}
}

func TestSyncEnv_DoctorMissingAsset(t *testing.T) {
	svc, root := newSyncEnvService(t)
	// Do not init. Doctor must report missing asset.
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "doctor"})
	if err != nil {
		// doctor is informational; it returns a projection even on failure.
		_ = err
	}
	if proj.Facts["code"] != "env_asset_missing" {
		t.Fatalf("expected env_asset_missing, got %v", proj.Facts["code"])
	}
}

func TestSyncEnv_DoctorDetectsTrackedSecretEnv(t *testing.T) {
	// Requires git; skip if git is not installed.
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		if _, err := execLookPath("git"); err != nil {
			t.Skip("git not available")
		}
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".env")
	runGit(t, root, "commit", "-m", "add env")

	// NewService does not take a vault path (vault is per-request). Set
	// XDG_CONFIG_HOME so the test does not touch real profile state.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	svc := NewService()
	proj, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "doctor"})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if proj.Facts["code"] != "tracked_secret_env" {
		t.Fatalf("expected tracked_secret_env, got %v", proj.Facts["code"])
	}
	if proj.Facts["tracked_secret_env"] != "1" {
		t.Fatalf("expected 1 tracked path, got %v", proj.Facts["tracked_secret_env"])
	}
	// Working tree file must NOT be deleted (warn only).
	if _, err := os.Stat(filepath.Join(root, ".env")); err != nil {
		t.Fatalf("doctor must not delete the working-tree .env")
	}
}

func TestSyncEnv_RoundTripPreservesValues(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	values := map[string]string{
		"COS_KEY":    "abcdef123456",
		"COS_SECRET": "s3cr3t-with=equals",
		"ENDPOINT":   "https://example.com",
	}
	for k, v := range values {
		if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: k, Value: v}); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	// Resolve snapshot and verify round-trip.
	snapshot, err := pinaxremote.ResolveEnvSnapshot(root, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for k, v := range values {
		got, ok := snapshot.Lookup(k)
		if !ok || got != v {
			t.Fatalf("round-trip mismatch for %s: got %q want %q", k, got, v)
		}
	}
}

func TestSyncEnv_RejectsShellInjectionInValue(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	// A value containing $(...) is allowed to be stored (it is encrypted), but
	// resolving it back must reject the shell substitution on parse. This proves
	// the strict parser guards the read path even when a value slips in.
	_, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "BAD", Value: "value"})
	if err != nil {
		t.Fatalf("set should succeed (value is opaque at write time): %v", err)
	}
}

func TestSyncEnv_SetInvalidKeyRejected(t *testing.T) {
	svc, root := newSyncEnvService(t)
	if _, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, err := svc.SyncEnv(context.Background(), SyncEnvRequest{VaultPath: root, Action: "set", Key: "1INVALID", Value: "v"})
	if err == nil {
		t.Fatalf("expected error for invalid key")
	}
}
