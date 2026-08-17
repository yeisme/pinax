package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

func TestSyncRepoMigrateDeviceProfileCreatesPortableEnvelope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	contentRef, err := pinaxprofile.SetStoredSecret("existing-content-key", "portable-content-key")
	if err != nil {
		t.Fatalf("store content key: %v", err)
	}
	_, err = pinaxremote.Login(root, pinaxremote.LoginRequest{
		Endpoint:            "s3://bucket/prefix",
		WorkspaceID:         "ws-migrate",
		DeviceID:            "old-mac",
		EncryptionSecretRef: contentRef,
		BackendKind:         "s3-direct",
		S3: &pinaxremote.S3Config{
			Bucket: "bucket", Prefix: "prefix", Endpoint: "https://cos.example.com", Region: "ap-guangzhou", Profile: "cos",
		},
	})
	if err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	credentialsPath := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(credentialsPath, []byte("[cos]\naws_access_key_id=AKID-MIGRATE\naws_secret_access_key=SECRET-MIGRATE\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)

	projection, err := NewService().SyncRepoMigrateDeviceProfile(context.Background(), SyncRepoMigrateRequest{
		VaultPath:    root,
		UnlockSource: projectsecrets.StaticSource([]byte("migration-passphrase")),
		Yes:          true,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if projection.Facts["remote_write"] != "false" || projection.Facts["credential_mode"] != pinaxremote.CredentialModeRepositoryEncrypted {
		t.Fatalf("migration facts: %#v", projection.Facts)
	}
	declaration, err := pinaxremote.LoadSyncConfig(root)
	if err != nil {
		t.Fatalf("load declaration: %v", err)
	}
	if declaration.Backend.S3 == nil || declaration.Backend.S3.CredentialMode != pinaxremote.CredentialModeRepositoryEncrypted {
		t.Fatalf("credential mode not migrated: %#v", declaration.Backend.S3)
	}
	resolver, err := projectsecrets.NewResolver(filepath.Join(root, projectSecretAsset), projectsecrets.StaticSource([]byte("migration-passphrase")), projectsecrets.DenyAllPolicy{})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	info, err := resolver.EnvelopeInfo()
	if err != nil {
		t.Fatalf("envelope info: %v", err)
	}
	if info.Entries[declaration.Secrets.CredentialID].Format != pinaxremote.S3CredentialFormat {
		t.Fatalf("credential entry: %#v", info.Entries)
	}
	if info.Entries[declaration.Secrets.EncryptionKeyID].Format != capsaEncryptionKeyFormat {
		t.Fatalf("encryption entry: %#v", info.Entries)
	}
	declarationBytes, _ := os.ReadFile(pinaxremote.DeclarationPath(root))
	envelopeBytes, _ := os.ReadFile(filepath.Join(root, projectSecretAsset))
	combined := string(declarationBytes) + string(envelopeBytes)
	for _, secret := range []string{"AKID-MIGRATE", "SECRET-MIGRATE", "portable-content-key", "migration-passphrase"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("migration leaked plaintext %q", secret)
		}
	}
}
