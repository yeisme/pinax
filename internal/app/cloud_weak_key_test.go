package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

func newWeakKeyVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	return root
}

func findProjectionWarning(projection domain.Projection, code string) *domain.ProjectionWarning {
	for i := range projection.Warnings {
		if projection.Warnings[i].Code == code {
			return &projection.Warnings[i]
		}
	}
	return nil
}

// TestProfileCredentialRefGetsPersistentEncryptionSecret verifies that an S3
// backend without an explicit encryption ref gets a stable user-level secret.
func TestProfileCredentialRefGetsPersistentEncryptionSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := newWeakKeyVault(t)
	svc := NewService()
	projection, err := svc.CapsaBackendSetS3(context.Background(), CloudBackendSetRequest{
		VaultPath:   root,
		Kind:        "s3",
		Bucket:      "notes",
		Region:      "us-east-1",
		Prefix:      "pinax-sync/",
		Profile:     "work",
		WorkspaceID: "personal",
		DeviceID:    "laptop",
	})
	if err != nil {
		t.Fatalf("CapsaBackendSetS3: %v", err)
	}
	if warning := findProjectionWarning(projection, "weak_encryption_key"); warning != nil {
		t.Fatalf("did not expect weak_encryption_key after automatic persistence: %#v", warning)
	}
	if projection.Facts["encryption_secret_persistence"] != "user_config" {
		t.Fatalf("encryption_secret_persistence = %#v", projection.Facts["encryption_secret_persistence"])
	}
	ref, err := persistentEncryptionSecretRef(root, "personal", "")
	if err != nil || ref != "stored://capsa-sync-personal" {
		t.Fatalf("persistent ref = %q, err=%v", ref, err)
	}
}

func TestExistingEncryptionSecretRefIsReused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := newWeakKeyVault(t)
	svc := NewService()
	request := CloudBackendSetRequest{
		VaultPath:           root,
		Kind:                "s3",
		Bucket:              "notes",
		Region:              "us-east-1",
		Prefix:              "pinax-sync/",
		Profile:             "work",
		WorkspaceID:         "personal",
		DeviceID:            "laptop",
		EncryptionSecretRef: "plain:original-sync-key",
	}
	if _, err := svc.CapsaBackendSetS3(context.Background(), request); err != nil {
		t.Fatalf("initial CapsaBackendSetS3: %v", err)
	}
	request.EncryptionSecretRef = ""
	if _, err := svc.CapsaBackendSetS3(context.Background(), request); err != nil {
		t.Fatalf("repeat CapsaBackendSetS3: %v", err)
	}
	state, err := pinaxcloud.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Config.EncryptionSecretRef != "plain:original-sync-key" {
		t.Fatalf("encryption secret ref changed to %q", state.Config.EncryptionSecretRef)
	}
}

// TestWeakKeyWarningSuppressedByDedicatedEncryptionRef verifies that providing a
// dedicated --encryption-secret-ref silences the weak-key warning even when the
// provider credential reference is a profile:// ref.
func TestWeakKeyWarningSuppressedByDedicatedEncryptionRef(t *testing.T) {
	t.Parallel()
	root := newWeakKeyVault(t)
	svc := NewService()
	projection, err := svc.CapsaBackendSetS3(context.Background(), CloudBackendSetRequest{
		VaultPath:           root,
		Kind:                "s3",
		Bucket:              "notes",
		Region:              "us-east-1",
		Profile:             "work",
		WorkspaceID:         "personal",
		DeviceID:            "laptop",
		EncryptionSecretRef: "plain:dedicated-sync-secret",
	})
	if err != nil {
		t.Fatalf("CapsaBackendSetS3: %v", err)
	}
	if w := findProjectionWarning(projection, "weak_encryption_key"); w != nil {
		t.Errorf("did not expect weak_encryption_key warning with a dedicated encryption ref, got %#v", w)
	}
}

// TestWeakKeyWarningSuppressedByEnvSecretRef verifies that a provider SecretRef
// sourced from env:// (a strong secret manager boundary) does not trigger the
// weak-key warning.
func TestWeakKeyWarningSuppressedByEnvSecretRef(t *testing.T) {
	t.Parallel()
	root := newWeakKeyVault(t)
	svc := NewService()
	projection, err := svc.CapsaBackendSetS3(context.Background(), CloudBackendSetRequest{
		VaultPath:   root,
		Kind:        "s3",
		Bucket:      "notes",
		Region:      "us-east-1",
		Profile:     "work",
		WorkspaceID: "personal",
		DeviceID:    "laptop",
		SecretRef:   "env://PINAX_TEST_SECRET",
	})
	if err != nil {
		t.Fatalf("CapsaBackendSetS3: %v", err)
	}
	if w := findProjectionWarning(projection, "weak_encryption_key"); w != nil {
		t.Errorf("did not expect weak_encryption_key warning for env:// secret ref, got %#v", w)
	}
}

// TestWeakKeyWarningLogic covers the pure helper across the scheme table.
func TestWeakKeyWarningLogic(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
		encryptionSecret string
		secretRef        string
		expectWarning    bool
	}{
		{"dedicated encryption ref set", "plain:dedicated", "profile://work", false},
		{"env provider ref", "", "env://PINAX_SYNC_SECRET", false},
		{"keychain provider ref", "", "keychain://pinax-sync", false},
		{"profile provider ref", "", "profile://work", true},
		{"rclone provider ref", "", "rclone://onedrive", true},
		{"plain provider ref", "", "plain:raw-secret", true},
		{"bare provider ref", "", "bare-secret", true},
		{"empty refs", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := pinaxcloud.State{Config: pinaxcloud.Config{EncryptionSecretRef: c.encryptionSecret, SecretRef: c.secretRef}}
			warning := weakEncryptionKeyWarning(state)
			if c.expectWarning && warning == nil {
				t.Errorf("expected weak_encryption_key warning for encryption=%q secret=%q", c.encryptionSecret, c.secretRef)
			}
			if !c.expectWarning && warning != nil {
				t.Errorf("did not expect warning for encryption=%q secret=%q, got %#v", c.encryptionSecret, c.secretRef, warning)
			}
		})
	}
}

// TestDoctorEncryptionKeyMismatch verifies that CloudDoctor reports an
// encryption_key_mismatch warning when the config's current key id differs from
// the key id recorded during the last sync.
func TestDoctorEncryptionKeyMismatch(t *testing.T) {
	t.Parallel()
	root := newWeakKeyVault(t)
	svc := NewService()
	if _, err := svc.CloudLogin(context.Background(), CloudLoginRequest{
		VaultPath:   root,
		Endpoint:    "file://" + filepath.Join(t.TempDir(), "store"),
		WorkspaceID: "ws",
		DeviceID:    "laptop",
		SecretRef:   "plain:current-secret",
	}); err != nil {
		t.Fatalf("CloudLogin: %v", err)
	}

	// Record a sync-state whose key id was derived from a *different* secret.
	staleKeyID := pinaxcloud.KeyID("plain:different-secret")
	if err := writeJSONAsset(filepath.Join(root, ".pinax", "sync-state.json"), currentSyncState{
		SchemaVersion: syncStateSchemaVersion,
		Target:        syncTargetCloud,
		LastKeyID:     staleKeyID,
	}); err != nil {
		t.Fatalf("write sync-state: %v", err)
	}

	projection, err := svc.CloudDoctor(context.Background(), CloudRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("CloudDoctor: %v", err)
	}
	if w := findProjectionWarning(projection, "encryption_key_mismatch"); w == nil {
		t.Errorf("expected encryption_key_mismatch warning, warnings = %#v", projection.Warnings)
	}
}

// TestDoctorEncryptionKeyMatchNoWarning verifies that when the recorded key id
// matches the current config, no mismatch warning is emitted.
func TestDoctorEncryptionKeyMatchNoWarning(t *testing.T) {
	t.Parallel()
	root := newWeakKeyVault(t)
	svc := NewService()
	if _, err := svc.CloudLogin(context.Background(), CloudLoginRequest{
		VaultPath:           root,
		Endpoint:            "file://" + filepath.Join(t.TempDir(), "store"),
		WorkspaceID:         "ws",
		DeviceID:            "laptop",
		SecretRef:           "plain:current-secret",
		EncryptionSecretRef: "plain:current-secret",
	}); err != nil {
		t.Fatalf("CloudLogin: %v", err)
	}

	currentKeyID := pinaxcloud.KeyID("plain:current-secret")
	if err := writeJSONAsset(filepath.Join(root, ".pinax", "sync-state.json"), currentSyncState{
		SchemaVersion: syncStateSchemaVersion,
		Target:        syncTargetCloud,
		LastKeyID:     currentKeyID,
	}); err != nil {
		t.Fatalf("write sync-state: %v", err)
	}

	projection, err := svc.CloudDoctor(context.Background(), CloudRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("CloudDoctor: %v", err)
	}
	if w := findProjectionWarning(projection, "encryption_key_mismatch"); w != nil {
		t.Errorf("did not expect encryption_key_mismatch when keys match, got %#v", w)
	}
}

// TestWriteCurrentSyncStateRecordsKeyID ensures the sync-state persists the key
// id derived from the active config so future doctor runs can detect rotation.
func TestWriteCurrentSyncStateRecordsKeyID(t *testing.T) {
	t.Parallel()
	root := newWeakKeyVault(t)
	state := pinaxcloud.State{Config: pinaxcloud.Config{
		SchemaVersion: pinaxcloud.ConfigSchemaVersion,
		Endpoint:      "file:///store",
		WorkspaceID:   "ws",
		DeviceID:      "laptop",
		SecretRef:     "plain:sync-secret",
	}}
	receipt := SyncRunReceipt{RunID: "run_1", Target: syncTargetCloud, Direction: "push", Status: "success"}
	if err := writeCurrentSyncState(root, state, receipt, "rev_1"); err != nil {
		t.Fatalf("writeCurrentSyncState: %v", err)
	}
	stored, err := readCurrentSyncState(root)
	if err != nil {
		t.Fatalf("readCurrentSyncState: %v", err)
	}
	want := pinaxcloud.KeyID("plain:sync-secret")
	if strings.TrimSpace(stored.LastKeyID) == "" {
		t.Fatal("expected non-empty last_key_id in sync-state")
	}
	if stored.LastKeyID != want {
		t.Errorf("last_key_id = %q, want %q", stored.LastKeyID, want)
	}
}
