package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

func newRepoService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	if err := os.MkdirAll(filepath.Join(dir, ".pinax"), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewService(), dir
}

func validInitReq(vault string) SyncRepoInitRequest {
	return SyncRepoInitRequest{
		VaultPath:       vault,
		BackendKind:     "s3-direct",
		Endpoint:        "s3://my-bucket/prefix",
		WorkspaceID:     "personal",
		TenantID:        "t1",
		AppID:           "pinax",
		CredentialID:    "tencent-cos-pinax",
		EncryptionKeyID: "personal-sync-key",
		RemoteDelete:    "deny",
	}
}

func TestSyncRepoInitCreatesDeclaration(t *testing.T) {
	svc, dir := newRepoService(t)
	proj, err := svc.SyncRepoInit(context.Background(), validInitReq(dir))
	if err != nil {
		t.Fatalf("SyncRepoInit: %v", err)
	}
	if proj.Facts["workspace_id"] != "personal" {
		t.Fatalf("workspace = %s", proj.Facts["workspace_id"])
	}
	if _, err := os.Stat(pinaxremote.DeclarationPath(dir)); err != nil {
		t.Fatalf("declaration not written: %v", err)
	}
	// Re-init is idempotent and preserves key identity.
	proj2, err := svc.SyncRepoInit(context.Background(), SyncRepoInitRequest{
		VaultPath:   dir,
		BackendKind: "s3-direct",
		Endpoint:    "s3://my-bucket/prefix",
		WorkspaceID: "personal",
		// EncryptionKeyID intentionally omitted; must reuse prior.
		ExistingIfPresent: true,
	})
	if err != nil {
		t.Fatalf("idempotent SyncRepoInit: %v", err)
	}
	if proj2.Facts["encryption_key_id"] != "personal-sync-key" {
		t.Fatalf("idempotent re-init dropped key identity: %s", proj2.Facts["encryption_key_id"])
	}
}

func TestSyncRepoInitRejectsPlaintext(t *testing.T) {
	svc, dir := newRepoService(t)
	req := validInitReq(dir)
	req.EncryptionKeyID = "password=hunter2"
	_, err := svc.SyncRepoInit(context.Background(), req)
	if err == nil {
		t.Fatal("expected plaintext rejection")
	}
}

func TestSyncRepoSecretSetListRemove(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_SYNC_FAKE_KEY", "repo-secret-passphrase")
	proj, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{
		VaultPath: dir, Action: "set", Name: "personal-sync-key", Kind: "encryption_key", Plaintext: "plain-key-value", Provider: "fake",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if proj.Facts["provider"] != "fake" {
		t.Fatalf("provider = %s", proj.Facts["provider"])
	}
	// Asset must not contain plaintext.
	b, _ := os.ReadFile(pinaxremote.SecretsAssetPath(dir))
	if strings.Contains(string(b), "plain-key-value") {
		t.Fatalf("plaintext leaked: %s", string(b))
	}
	// List returns metadata, no plaintext.
	listProj, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{VaultPath: dir, Action: "list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listProj.Facts["count"] != "1" {
		t.Fatalf("count = %s", listProj.Facts["count"])
	}
	// Remove.
	remProj, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{VaultPath: dir, Action: "remove", Name: "personal-sync-key"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if remProj.Facts["remaining"] != "0" {
		t.Fatalf("remaining = %s", remProj.Facts["remaining"])
	}
}

func TestSyncRepoBootstrapWritesRuntimeConfig(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_SYNC_FAKE_KEY", "repo-secret-passphrase")
	if _, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{
		VaultPath: dir, Action: "set", Name: "personal-sync-key", Kind: "encryption_key", Plaintext: "decrypted-key", Provider: "fake",
	}); err != nil {
		t.Fatal(err)
	}
	proj, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir, DeviceID: "desktop-1"})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if proj.Facts["device_id"] != "desktop-1" {
		t.Fatalf("device = %s", proj.Facts["device_id"])
	}
	if proj.Facts["pull_only"] != "true" {
		t.Fatalf("bootstrap must be pull-only, got %s", proj.Facts["pull_only"])
	}
	// Runtime config written + encryption secret ref populated.
	state, err := pinaxremote.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Config.WorkspaceID != "personal" {
		t.Fatalf("runtime workspace = %s", state.Config.WorkspaceID)
	}
	if state.Config.EncryptionSecretRef == "" {
		t.Fatal("expected encryption secret ref in runtime config")
	}
	// Runtime config must not contain the plaintext key.
	cfgBytes, _ := os.ReadFile(filepath.Join(dir, ".pinax", "cloud", "config.yaml"))
	if strings.Contains(string(cfgBytes), "decrypted-key") {
		t.Fatalf("plaintext key leaked into runtime config: %s", string(cfgBytes))
	}
}

func TestSyncRepoBootstrapFailsWithoutUnlock(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_SYNC_FAKE_KEY", "set-then-removed")
	if _, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{
		VaultPath: dir, Action: "set", Name: "personal-sync-key", Kind: "encryption_key", Plaintext: "v", Provider: "fake",
	}); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("PINAX_SYNC_FAKE_KEY")
	_, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir, DeviceID: "desktop-1"})
	if err == nil {
		t.Fatal("expected unlock-required failure")
	}
	if !strings.Contains(err.Error(), "unlock") && !strings.Contains(err.Error(), "PINAX_SYNC_FAKE_KEY") {
		t.Fatalf("expected unlock-required error, got %v", err)
	}
}

func TestSyncRepoBootstrapMissingDeclaration(t *testing.T) {
	svc, dir := newRepoService(t)
	_, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir, DeviceID: "d"})
	if err == nil {
		t.Fatal("expected declaration_missing failure")
	}
}

func TestSyncRepoPlanNoWrites(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	// Ensure no runtime config exists before plan.
	_ = os.Remove(filepath.Join(dir, ".pinax", "cloud", "config.yaml"))
	proj, err := svc.SyncRepoPlan(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if proj.Facts["runtime_configured"] != "false" {
		t.Fatalf("expected runtime not configured, got %s", proj.Facts["runtime_configured"])
	}
	// plan must not have written runtime config.
	if _, err := os.Stat(filepath.Join(dir, ".pinax", "cloud", "config.yaml")); err == nil {
		t.Fatal("plan must not write runtime config")
	}
}

func TestSyncRepoApplyRequiresApprovalForWorkspaceChange(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_SYNC_FAKE_KEY", "pass")
	if _, err := svc.SyncRepoSecret(context.Background(), SyncRepoSecretRequest{VaultPath: dir, Action: "set", Name: "personal-sync-key", Kind: "encryption_key", Plaintext: "v", Provider: "fake"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir, DeviceID: "desktop-1"}); err != nil {
		t.Fatal(err)
	}
	// Re-init with a different workspace.
	req2 := validInitReq(dir)
	req2.WorkspaceID = "team"
	if _, err := svc.SyncRepoInit(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	// apply without --yes must be blocked.
	_, err := svc.SyncRepoApply(context.Background(), SyncRepoRuntimeRequest{VaultPath: dir, DeviceID: "desktop-1", Yes: false})
	if err == nil {
		t.Fatal("expected approval_required")
	}
	ce, ok := domainErr(err, "approval_required")
	if !ok {
		t.Fatalf("expected approval_required, got %v (ce=%v)", err, ce)
	}
}

func TestSyncRepoDoctorDetectsDrift(t *testing.T) {
	svc, dir := newRepoService(t)
	if _, err := svc.SyncRepoInit(context.Background(), validInitReq(dir)); err != nil {
		t.Fatal(err)
	}
	proj, err := svc.SyncRepoDoctor(context.Background(), VaultRequest{VaultPath: dir})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	// Declaration present but runtime not configured → drift.
	if proj.Facts["declaration_present"] != "true" {
		t.Fatalf("expected declaration_present=true, got %s", proj.Facts["declaration_present"])
	}
	if proj.Facts["workspace_id"] != "personal" {
		t.Fatalf("doctor workspace = %s", proj.Facts["workspace_id"])
	}
}

func domainErr(err error, code string) (*domain.CommandError, bool) {
	if err == nil {
		return nil, false
	}
	ce := &domain.CommandError{}
	if strings.Contains(err.Error(), code) {
		return ce, true
	}
	return ce, false
}
