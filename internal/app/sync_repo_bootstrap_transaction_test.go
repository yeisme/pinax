package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/remote"
)

// seedRepoEncryptedRepo writes a declaration with credential_mode:
// repository-encrypted and a credentialctl envelope holding an S3 bundle.
func seedRepoEncryptedRepo(t *testing.T, repo string) {
	t.Helper()
	decl := "schema_version: \"" + remote.SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://bucket/main/\n" +
		"  s3:\n    bucket: bucket\n    endpoint: https://cos.example.com\n    region: ap-shanghai\n    credential_mode: repository-encrypted\n" +
		"workspace:\n  workspace_id: ws-x\n" +
		"secrets:\n  credential_id: cred-1\n  encryption_key_id: enc-1\n"
	dir := filepath.Join(repo, ".pinax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, remote.DeclarationFileName), []byte(decl), 0o600); err != nil {
		t.Fatalf("write decl: %v", err)
	}
	store := projectsecrets.NewStore()
	if _, err := store.Init(repo, ".pinax/project-secrets.yaml", "pinax", "ws-x", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("init envelope: %v", err)
	}
	payload := `{"access_key_id":"AKIDBOOT","secret_access_key":"SKBOOT"}`
	if _, err := store.SetEntry(repo, ".pinax/project-secrets.yaml", "cred-1", "credential", "s3_credentials.v1", "1", []byte(payload), []byte(testPassphrase)); err != nil {
		t.Fatalf("set entry: %v", err)
	}
}

// TestBootstrapTransactionVerifiesCredentialAndEmitsReceipt verifies the staged
// bootstrap transaction (task 5.2): with a repository-encrypted declaration and
// a valid unlock source, bootstrap proves the envelope unlocks, compiles the
// pull-only runtime and emits a redacted receipt (credential_mode, unlock_source,
// credential_verified, pull_only). No plaintext in facts.
func TestBootstrapTransactionVerifiesCredentialAndEmitsReceipt(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	seedRepoEncryptedRepo(t, repo)
	svc := NewService()
	proj, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{
		VaultPath:           repo,
		DeviceID:            "mac2",
		Yes:                 true,
		ProjectUnlockSource: projectsecrets.StaticSource([]byte(testPassphrase)),
		Pull:                true,
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if proj.Facts["credential_mode"] != "repository-encrypted" {
		t.Fatalf("credential_mode fact: %v", proj.Facts["credential_mode"])
	}
	if proj.Facts["credential_verified"] != "true" {
		t.Fatalf("credential_verified fact: %v", proj.Facts["credential_verified"])
	}
	if proj.Facts["unlock_source"] == "" {
		t.Fatal("unlock_source fact missing")
	}
	if proj.Facts["pull_only"] != "true" {
		t.Fatalf("pull_only fact: %v", proj.Facts["pull_only"])
	}
	if proj.Facts["pull_planned"] != "true" {
		t.Fatalf("pull_planned fact: %v", proj.Facts["pull_planned"])
	}
	if proj.Facts["remote_write"] != "false" {
		t.Fatalf("remote_write should be false: %v", proj.Facts["remote_write"])
	}
}

// TestBootstrapTransactionFailsClosedOnWrongPassphrase verifies the abort-
// before-compile property: a wrong passphrase fails with sync_repo_unlock_failed
// before any runtime config is compiled or written.
func TestBootstrapTransactionFailsClosedOnWrongPassphrase(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	seedRepoEncryptedRepo(t, repo)
	svc := NewService()
	proj, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{
		VaultPath:           repo,
		DeviceID:            "mac2",
		Yes:                 true,
		ProjectUnlockSource: projectsecrets.StaticSource([]byte("wrong-passphrase")),
	})
	if err == nil {
		t.Fatal("bootstrap with wrong passphrase should fail")
	}
	if proj.Error == nil || proj.Error.Code != "sync_repo_unlock_failed" {
		t.Fatalf("expected sync_repo_unlock_failed, got: %+v", proj.Error)
	}
	// The wrong passphrase must not appear anywhere in the projection.
	if strings.Contains(proj.Summary, "wrong-passphrase") {
		t.Fatalf("summary leaked passphrase: %s", proj.Summary)
	}
}

// TestBootstrapTransactionSkipsUnlockVerifyForDeviceProfile verifies the
// staged unlock-verify only runs for repository-encrypted mode; device-profile
// keeps the legacy compile-only path.
func TestBootstrapTransactionSkipsUnlockVerifyForDeviceProfile(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	// device-profile declaration (default, no credential_mode).
	decl := "schema_version: \"" + remote.SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://bucket/main/\n" +
		"  s3:\n    bucket: bucket\n" +
		"workspace:\n  workspace_id: ws-x\n" +
		"secrets:\n  encryption_key_id: enc-1\n"
	dir := filepath.Join(repo, ".pinax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, remote.DeclarationFileName), []byte(decl), 0o600); err != nil {
		t.Fatalf("write decl: %v", err)
	}
	svc := NewService()
	// No ProjectUnlockSource and device-profile mode: must NOT require unlock.
	proj, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{
		VaultPath: repo,
		DeviceID:  "mac2",
		Yes:       true,
	})
	if err != nil {
		t.Fatalf("device-profile bootstrap should not require unlock: %v", err)
	}
	if proj.Facts["credential_mode"] != "device-profile" {
		t.Fatalf("credential_mode fact: %v", proj.Facts["credential_mode"])
	}
	if _, present := proj.Facts["credential_verified"]; present {
		t.Fatal("credential_verified should not be set for device-profile")
	}
}
