package remote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEnvProviderForTest encrypts/decrypts a single dotenv document under the
// "env" identity, reusing the existing FakeUnlockProvider AES-GCM path.
func fakeEnvProviderForTest(t *testing.T) FakeUnlockProvider {
	t.Setenv("PINAX_SYNC_FAKE_KEY", "test-key-env-loader")
	return FakeUnlockProvider{}
}

func writeTestEnvAsset(t *testing.T, root, provider, plaintext string) {
	t.Helper()
	p := fakeEnvProviderForTest(t)
	entry, err := p.Lock("env", plaintext, "pinax-sync-env", SecretKindCredential)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	asset := EnvAsset{
		Provider:   provider,
		Ciphertext: entry.Ciphertext,
		Digest:     EnvDocumentDigest([]byte(plaintext)),
		KeyNames:   []string{"COS_KEY", "COS_SECRET"},
	}
	if err := SaveEnvAsset(root, asset); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestResolveEnvSnapshot_Success(t *testing.T) {
	tmp := t.TempDir()
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=mykey\nCOS_SECRET=mysecret\n")
	snapshot, err := ResolveEnvSnapshot(tmp, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if v, _ := snapshot.Lookup("COS_KEY"); v != "mykey" {
		t.Fatalf("expected mykey, got %q", v)
	}
	if v, _ := snapshot.Lookup("COS_SECRET"); v != "mysecret" {
		t.Fatalf("expected mysecret, got %q", v)
	}
	keys := snapshot.AllKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestResolveEnvSnapshot_MissingAsset(t *testing.T) {
	tmp := t.TempDir()
	_, err := ResolveEnvSnapshot(tmp, nil)
	if !errors.Is(err, ErrEnvAssetMissing) {
		t.Fatalf("expected ErrEnvAssetMissing, got %v", err)
	}
}

func TestResolveEnvSnapshot_WrongKey(t *testing.T) {
	tmp := t.TempDir()
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=mykey\n")
	// Change the fake key so decryption fails.
	t.Setenv("PINAX_SYNC_FAKE_KEY", "different-key")
	_, err := ResolveEnvSnapshot(tmp, nil)
	if err == nil {
		t.Fatalf("expected decrypt error, got nil")
	}
}

func TestResolveEnvSnapshot_StrictParseFailure(t *testing.T) {
	tmp := t.TempDir()
	// Plaintext contains a forbidden shell substitution.
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=value\nBAD=$(whoami)\n")
	_, err := ResolveEnvSnapshot(tmp, nil)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestEnvSnapshot_Immutable(t *testing.T) {
	snap := NewEnvSnapshot(map[string]string{"A": "1"}, "digest", "test")
	_, _ = snap.Lookup("A")
	again, _ := snap.Lookup("A")
	if again != "1" {
		t.Fatalf("snapshot was mutated: got %q", again)
	}
}

func TestEnvSnapshot_ApplyAllowlist(t *testing.T) {
	snap := NewEnvSnapshot(map[string]string{"A": "1", "B": "2", "C": "3"}, "d", "s")
	out := snap.ApplyAllowlist([]string{"A", "C", "MISSING"})
	if len(out) != 2 {
		t.Fatalf("expected 2 allowlisted keys, got %d: %v", len(out), out)
	}
	if out["A"] != "1" || out["C"] != "3" {
		t.Fatalf("unexpected allowlist output: %v", out)
	}
}

func TestEnvSnapshot_OverlayEnvPrecedence(t *testing.T) {
	snap := NewEnvSnapshot(map[string]string{"SNAP_KEY": "from-snapshot", "OTHER": "also-snapshot"}, "d", "s")
	// Explicit process environment should win for SNAP_KEY.
	base := []string{"PATH=/usr/bin", "SNAP_KEY=from-process-env"}
	out := snap.OverlayEnv(base, []string{"SNAP_KEY", "OTHER"})
	merged := make(map[string]string, len(out))
	for _, kv := range out {
		for i, r := range kv {
			if r == '=' {
				merged[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	if merged["SNAP_KEY"] != "from-process-env" {
		t.Fatalf("explicit process env must win, got %q", merged["SNAP_KEY"])
	}
	if merged["OTHER"] != "also-snapshot" {
		t.Fatalf("non-conflicting snapshot key must be injected, got %q", merged["OTHER"])
	}
}

func TestEnvReloader_ReloadOnIdentityChange(t *testing.T) {
	tmp := t.TempDir()
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=v1\n")
	reloader := NewEnvReloader(tmp, nil)

	// First call loads.
	snap1, status1 := reloader.RunSnapshot()
	if snap1 == nil || !status1.Loaded {
		t.Fatalf("first call should load snapshot: %+v", status1)
	}
	if v, _ := snap1.Lookup("COS_KEY"); v != "v1" {
		t.Fatalf("expected v1, got %q", v)
	}

	// Second call with no change returns the same snapshot.
	snap2, status2 := reloader.RunSnapshot()
	if status2.Changed {
		t.Fatalf("expected no change on second call")
	}
	if snap2 != snap1 {
		t.Fatalf("expected same snapshot instance for unchanged asset")
	}

	// Change the asset content (new digest → new mtime/size).
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=v2\n")
	snap3, status3 := reloader.RunSnapshot()
	if !status3.Changed || !status3.Loaded {
		t.Fatalf("expected reload on change: %+v", status3)
	}
	if v, _ := snap3.Lookup("COS_KEY"); v != "v2" {
		t.Fatalf("expected v2 after reload, got %q", v)
	}
}

func TestEnvReloader_RetainsLastSnapshotOnReloadFailure(t *testing.T) {
	tmp := t.TempDir()
	writeTestEnvAsset(t, tmp, "fake", "COS_KEY=v1\n")
	reloader := NewEnvReloader(tmp, nil)

	snap1, _ := reloader.RunSnapshot()
	if snap1 == nil {
		t.Fatalf("first load expected")
	}

	// Corrupt the ciphertext so decryption fails on next reload.
	asset, _ := LoadEnvAsset(tmp)
	asset.Ciphertext = "aGVsbG8=" // valid base64 but wrong plaintext
	if err := SaveEnvAsset(tmp, asset); err != nil {
		t.Fatal(err)
	}

	snap2, status := reloader.RunSnapshot()
	if !status.Degraded {
		t.Fatalf("expected degraded status on reload failure: %+v", status)
	}
	if status.Code == "" {
		t.Fatalf("expected non-empty degraded code")
	}
	if snap2 == nil {
		t.Fatalf("must retain last successful snapshot on failure")
	}
	if v, _ := snap2.Lookup("COS_KEY"); v != "v1" {
		t.Fatalf("retained snapshot must have v1, got %q", v)
	}
	if reloader.DegradedCode() == "" {
		t.Fatalf("DegradedCode must report the failure code")
	}
}

func TestEnvReloader_NoAssetNotDegraded(t *testing.T) {
	tmp := t.TempDir()
	reloader := NewEnvReloader(tmp, nil)
	snap, status := reloader.RunSnapshot()
	if snap != nil {
		t.Fatalf("expected nil snapshot when no asset exists")
	}
	if status.Degraded || status.Code != "" {
		t.Fatalf("missing asset must not be degraded: %+v", status)
	}
}

func TestMaterializeEnv_Creates0600File(t *testing.T) {
	tmp := t.TempDir()
	snap := NewEnvSnapshot(map[string]string{"A": "1", "B": "2"}, "d", "s")
	path, err := MaterializeEnv(tmp, snap, []string{"A", "B"})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %v", info.Mode().Perm())
	}
	if filepath.Dir(path) != filepath.Join(tmp, ".pinax", "runtime") {
		t.Fatalf("unexpected materialized path: %s", path)
	}
}

func TestMaterializeEnv_RespectsAllowlist(t *testing.T) {
	tmp := t.TempDir()
	snap := NewEnvSnapshot(map[string]string{"A": "1", "SECRET": "s3cret"}, "d", "s")
	path, err := MaterializeEnv(tmp, snap, []string{"A"})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "s3cret") {
		t.Fatalf("materialized file leaked non-allowlisted key: %s", string(data))
	}
}

func TestMaterializeEnv_RejectsSymlink(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, ".pinax", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "outside.txt")
	if err := os.WriteFile(target, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, EnvRuntimePath(tmp)); err != nil {
		t.Fatal(err)
	}
	snap := NewEnvSnapshot(map[string]string{"A": "1"}, "d", "s")
	_, err := MaterializeEnv(tmp, snap, []string{"A"})
	if err == nil {
		t.Fatalf("expected symlink rejection")
	}
	// User file must be preserved.
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("user file must not be deleted: %v", err)
	}
}

func TestCleanMaterializedEnv_RemovesOnlyManaged(t *testing.T) {
	tmp := t.TempDir()
	snap := NewEnvSnapshot(map[string]string{"A": "1"}, "d", "s")
	if _, err := MaterializeEnv(tmp, snap, []string{"A"}); err != nil {
		t.Fatal(err)
	}
	removed, err := CleanMaterializedEnv(tmp)
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if !removed {
		t.Fatalf("expected file to be removed")
	}
	// Second clean is a no-op.
	removed2, _ := CleanMaterializedEnv(tmp)
	if removed2 {
		t.Fatalf("expected no-op on missing file")
	}
}

func TestCleanMaterializedEnv_RejectsSymlink(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, ".pinax", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(tmp, "user.txt")
	if err := os.WriteFile(userFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(userFile, EnvRuntimePath(tmp)); err != nil {
		t.Fatal(err)
	}
	_, err := CleanMaterializedEnv(tmp)
	if err == nil {
		t.Fatalf("expected symlink rejection")
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("symlink target must be preserved: %v", err)
	}
}
