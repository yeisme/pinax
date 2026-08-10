package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// seedDeviceProfileRuntime stands up an s3-direct device-profile runtime with a
// shared AWS credentials profile and a stored content key, returning the unlock
// source that authorizes the repository envelope. It is the shared fixture for
// the migration transaction tests.
func seedDeviceProfileRuntime(t *testing.T, root, workspace, passphrase string) projectsecrets.UnlockSource {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	contentRef, err := pinaxprofile.SetStoredSecret("content-key-"+workspace, "portable-content-key")
	if err != nil {
		t.Fatalf("store content key: %v", err)
	}
	if _, err := pinaxremote.Login(root, pinaxremote.LoginRequest{
		Endpoint:            "s3://bucket/prefix",
		WorkspaceID:         workspace,
		DeviceID:            "old-mac",
		EncryptionSecretRef: contentRef,
		BackendKind:         "s3-direct",
		S3:                  &pinaxremote.S3Config{Bucket: "bucket", Prefix: "prefix", Endpoint: "https://cos.example.com", Region: "ap-guangzhou", Profile: "cos"},
	}); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	credentialsPath := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(credentialsPath, []byte("[cos]\naws_access_key_id=AKID-MIGRATE\naws_secret_access_key=SECRET-MIGRATE\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	return projectsecrets.StaticSource([]byte(passphrase))
}

// TestSyncRepoMigrateIdempotentSameIdentity verifies task 6.9: re-running the
// migration with the same credential/content-key identity returns
// already_migrated=true without rebuilding the DEK, rewriting ciphertext, or
// contacting the remote.
func TestSyncRepoMigrateIdempotentSameIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := seedDeviceProfileRuntime(t, root, "ws-migrate", "migration-passphrase")
	svc := NewService()
	proj1, err := svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true})
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if proj1.Facts["already_migrated"] == "true" {
		t.Fatal("first migrate must not report already_migrated")
	}
	envelopeBefore, err := os.ReadFile(filepath.Join(root, projectSecretAsset))
	if err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	proj2, err := svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true})
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if proj2.Facts["already_migrated"] != "true" {
		t.Fatalf("expected already_migrated=true on re-run, facts=%#v", proj2.Facts)
	}
	if proj2.Facts["key_rotated"] != "false" {
		t.Fatalf("expected key_rotated=false on idempotent re-run, got %v", proj2.Facts["key_rotated"])
	}
	if proj2.Facts["remote_write"] != "false" {
		t.Fatalf("expected remote_write=false on idempotent re-run, got %v", proj2.Facts["remote_write"])
	}
	envelopeAfter, err := os.ReadFile(filepath.Join(root, projectSecretAsset))
	if err != nil {
		t.Fatalf("read envelope after: %v", err)
	}
	if !bytes.Equal(envelopeBefore, envelopeAfter) {
		t.Fatal("envelope changed during an idempotent re-migration")
	}
}

// TestSyncRepoMigrateIdempotentPlanIsReadOnly verifies the plan-first phase
// evaluates the migration without writing any declaration or envelope.
func TestSyncRepoMigrateIdempotentPlanIsReadOnly(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := seedDeviceProfileRuntime(t, root, "ws-migrate", "migration-passphrase")
	plan, err := planSyncRepoMigration(ctx, root, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.credentialID != "cos" {
		t.Fatalf("credentialID: %s", plan.credentialID)
	}
	if plan.workspaceID != "ws-migrate" {
		t.Fatalf("workspaceID: %s", plan.workspaceID)
	}
	if plan.alreadyMigrated || plan.conflictReason != "" {
		t.Fatalf("fresh runtime must plan a fresh migration: %+v", plan)
	}
	if _, err := os.Stat(pinaxremote.DeclarationPath(root)); !os.IsNotExist(err) {
		t.Fatal("plan phase wrote a declaration")
	}
	if _, err := os.Stat(filepath.Join(root, projectSecretAsset)); !os.IsNotExist(err) {
		t.Fatal("plan phase wrote an envelope")
	}
}

// TestSyncRepoMigrateConflictDifferentIdentity verifies task 6.9: when the
// existing repository-encrypted declaration has a different identity (workspace
// diverged), re-migration returns migration_conflict without overwriting the
// existing declaration or envelope.
func TestSyncRepoMigrateConflictDifferentIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := seedDeviceProfileRuntime(t, root, "ws-migrate", "migration-passphrase")
	svc := NewService()
	if _, err := svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	declarationBefore, _ := os.ReadFile(pinaxremote.DeclarationPath(root))
	envelopeBefore, _ := os.ReadFile(filepath.Join(root, projectSecretAsset))
	// Re-login with a different workspace so the would-be identity diverges.
	contentRef, err := pinaxprofile.SetStoredSecret("content-key-ws-x", "portable-content-key")
	if err != nil {
		t.Fatalf("store content key: %v", err)
	}
	if _, err := pinaxremote.Login(root, pinaxremote.LoginRequest{
		Endpoint: "s3://bucket/prefix", WorkspaceID: "ws-x", DeviceID: "old-mac",
		EncryptionSecretRef: contentRef, BackendKind: "s3-direct",
		S3: &pinaxremote.S3Config{Bucket: "bucket", Prefix: "prefix", Endpoint: "https://cos.example.com", Region: "ap-guangzhou", Profile: "cos"},
	}); err != nil {
		t.Fatalf("re-login: %v", err)
	}
	_, err = svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true})
	if !hasCommandCode(err, "migration_conflict") {
		t.Fatalf("expected migration_conflict on identity mismatch, got %v", err)
	}
	declarationAfter, _ := os.ReadFile(pinaxremote.DeclarationPath(root))
	envelopeAfter, _ := os.ReadFile(filepath.Join(root, projectSecretAsset))
	if !bytes.Equal(declarationBefore, declarationAfter) {
		t.Fatal("declaration overwritten on migration conflict")
	}
	if !bytes.Equal(envelopeBefore, envelopeAfter) {
		t.Fatal("envelope overwritten on migration conflict")
	}
}

// TestSyncRepoMigrateConflictUnverifiableEnvelope verifies task 6.9: when the
// identity matches but the supplied passphrase cannot unlock the existing
// envelope, re-migration returns migration_conflict (not already_migrated) and
// MUST NOT overwrite the unverifiable envelope.
func TestSyncRepoMigrateConflictUnverifiableEnvelope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	seedDeviceProfileRuntime(t, root, "ws-migrate", "migration-passphrase")
	svc := NewService()
	if _, err := svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: projectsecrets.StaticSource([]byte("migration-passphrase")), Yes: true}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	envelopeBefore, _ := os.ReadFile(filepath.Join(root, projectSecretAsset))
	proj, err := svc.SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: projectsecrets.StaticSource([]byte("wrong-passphrase")), Yes: true})
	if !hasCommandCode(err, "migration_conflict") {
		t.Fatalf("expected migration_conflict on unverifiable envelope, got %v", err)
	}
	if proj.Facts["already_migrated"] == "true" {
		t.Fatal("wrong passphrase must not report already_migrated")
	}
	envelopeAfter, _ := os.ReadFile(filepath.Join(root, projectSecretAsset))
	if !bytes.Equal(envelopeBefore, envelopeAfter) {
		t.Fatal("unverifiable envelope overwritten")
	}
}

// TestSyncRepoMigrateRollbackLeavesNoCorruptDeclaration verifies task 6.9
// atomicity: when a write fails mid-migration, no partial/corrupt declaration
// is left behind and the device-profile runtime remains usable.
func TestSyncRepoMigrateRollbackLeavesNoCorruptDeclaration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := seedDeviceProfileRuntime(t, root, "ws-migrate", "migration-passphrase")
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o700); err != nil {
		t.Fatalf("mkdir .pinax: %v", err)
	}
	if err := os.Chmod(filepath.Join(root, ".pinax"), 0o500); err != nil {
		t.Fatalf("chmod .pinax: %v", err)
	}
	defer func() { _ = os.Chmod(filepath.Join(root, ".pinax"), 0o700) }()
	_, err := NewService().SyncRepoMigrateDeviceProfile(ctx, SyncRepoMigrateRequest{VaultPath: root, UnlockSource: source, Yes: true})
	if err == nil {
		t.Fatal("expected migration failure with read-only .pinax")
	}
	if _, statErr := os.Stat(pinaxremote.DeclarationPath(root)); statErr == nil {
		t.Fatal("read-only migration left a declaration file behind")
	}
	state, loadErr := pinaxremote.Load(root)
	if loadErr != nil {
		t.Fatalf("runtime load after failed migration: %v", loadErr)
	}
	if state.Config.BackendKind != "s3-direct" || state.Config.S3 == nil || state.Config.S3.Profile != "cos" {
		t.Fatalf("device-profile runtime corrupted by failed migration: %#v", state.Config)
	}
}

// TestSyncRepoMigrateAtomicWritePreservesDestination verifies the atomic write
// helper leaves the destination file unchanged when the commit fails, so a
// declaration write that fails partway cannot corrupt the previous declaration.
func TestSyncRepoMigrateAtomicWritePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decl.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()
	if err := atomicWriteFile(path, []byte("replacement"), 0o600); err == nil {
		t.Fatal("expected atomic write to fail against a read-only directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after failed write: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("original destination corrupted by failed atomic write: %q", data)
	}
}
