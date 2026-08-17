package remote

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCredentialModeProfileRollbackSmoke verifies the rollback contract: when a
// declaration switches credential_mode from repository-encrypted back to
// device-profile, the workspace, encryption key identity and remote topology
// are preserved. No Capsa content key rotation, no remote revision deletion.
// (pinax-passphrase-s3-bootstrap task 5.4)
func TestCredentialModeProfileRollbackSmoke(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Declaration with repository-encrypted mode.
	repoEnc := "credential_mode: repository-encrypted\n"
	decl := "schema_version: \"" + SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://yeisme-notes/main/\n" +
		"  s3:\n    bucket: yeisme-notes\n    endpoint: https://cos.ap-shanghai.myqcloud.com\n    region: ap-shanghai\n    " + repoEnc +
		"workspace:\n  workspace_id: ws-yeisme\n  tenant_id: t1\n  app_id: a1\n" +
		"secrets:\n  credential_id: cred-1\n  encryption_key_id: enc-key-1\n"
	if err := writeTestDeclaration(root, decl); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
	cfgRepo, err := LoadSyncConfig(root)
	if err != nil {
		t.Fatalf("load repo-encrypted: %v", err)
	}
	cfgRepo = cfgRepo.Normalized()
	if cfgRepo.Backend.S3.CredentialMode != CredentialModeRepositoryEncrypted {
		t.Fatalf("expected repository-encrypted, got %q", cfgRepo.Backend.S3.CredentialMode)
	}
	// Snapshot the identity fields that must survive a mode switch.
	workspaceBefore := cfgRepo.Workspace.WorkspaceID
	encKeyBefore := cfgRepo.Secrets.EncryptionKeyID
	credIDBefore := cfgRepo.Secrets.CredentialID
	namespaceBefore := cfgRepo.EffectiveNamespace()

	// Switch back to device-profile (rollback). Identity fields are unchanged.
	declRollback := "schema_version: \"" + SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://yeisme-notes/main/\n" +
		"  s3:\n    bucket: yeisme-notes\n    endpoint: https://cos.ap-shanghai.myqcloud.com\n    region: ap-shanghai\n    credential_mode: device-profile\n" +
		"workspace:\n  workspace_id: ws-yeisme\n  tenant_id: t1\n  app_id: a1\n" +
		"secrets:\n  credential_id: cred-1\n  encryption_key_id: enc-key-1\n"
	if err := writeTestDeclaration(root, declRollback); err != nil {
		t.Fatalf("write rollback declaration: %v", err)
	}
	cfgProfile, err := LoadSyncConfig(root)
	if err != nil {
		t.Fatalf("load device-profile: %v", err)
	}
	cfgProfile = cfgProfile.Normalized()
	if cfgProfile.Backend.S3.CredentialMode != CredentialModeDeviceProfile {
		t.Fatalf("expected device-profile after rollback, got %q", cfgProfile.Backend.S3.CredentialMode)
	}
	if cfgProfile.Workspace.WorkspaceID != workspaceBefore {
		t.Fatalf("workspace changed across rollback: %q -> %q", workspaceBefore, cfgProfile.Workspace.WorkspaceID)
	}
	if cfgProfile.Secrets.EncryptionKeyID != encKeyBefore {
		t.Fatalf("encryption key id changed across rollback: %q -> %q", encKeyBefore, cfgProfile.Secrets.EncryptionKeyID)
	}
	if cfgProfile.Secrets.CredentialID != credIDBefore {
		t.Fatalf("credential id changed across rollback: %q -> %q", credIDBefore, cfgProfile.Secrets.CredentialID)
	}
	if cfgProfile.EffectiveNamespace() != namespaceBefore {
		t.Fatalf("namespace changed across rollback: %q -> %q", namespaceBefore, cfgProfile.EffectiveNamespace())
	}
	// Empty credential_mode defaults to device-profile (rollback is the default).
	declDefault := "schema_version: \"" + SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://yeisme-notes/main/\n" +
		"  s3:\n    bucket: yeisme-notes\n" +
		"workspace:\n  workspace_id: ws-yeisme\n" +
		"secrets:\n  encryption_key_id: enc-key-1\n"
	if err := writeTestDeclaration(root, declDefault); err != nil {
		t.Fatalf("write default declaration: %v", err)
	}
	cfgDefault, _ := LoadSyncConfig(root)
	cfgDefault = cfgDefault.Normalized()
	if cfgDefault.Backend.S3.CredentialMode != CredentialModeDeviceProfile {
		t.Fatalf("empty credential_mode should default to device-profile, got %q", cfgDefault.Backend.S3.CredentialMode)
	}
}

func writeTestDeclaration(root, content string) error {
	dir := filepath.Join(root, ".pinax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, DeclarationFileName), []byte(content), 0o600)
}
