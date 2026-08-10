package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

func TestSyncRepoMigrateDeviceProfileCommand(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_REPO_PASS", "migration-passphrase")
	contentRef, err := pinaxprofile.SetStoredSecret("migration-content", "portable-content-key")
	if err != nil {
		t.Fatalf("store content key: %v", err)
	}
	_, err = pinaxremote.Login(root, pinaxremote.LoginRequest{
		Endpoint: "s3://bucket/prefix", WorkspaceID: "ws-cli-migrate", DeviceID: "old-mac",
		EncryptionSecretRef: contentRef, BackendKind: "s3-direct",
		S3: &pinaxremote.S3Config{Bucket: "bucket", Prefix: "prefix", Profile: "cos"},
	})
	if err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	credentialsPath := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(credentialsPath, []byte("[cos]\naws_access_key_id=AKID-CLI\naws_secret_access_key=SECRET-CLI\n"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)

	out, stderr, err := runCLIWithStdin(t, "", "sync", "repo", "migrate", "device-profile", "--vault", root, "--unlock", "env", "--yes", "--json")
	if err != nil {
		t.Fatalf("migrate command: %v\nstdout=%s\nstderr=%s", err, out, stderr)
	}
	projection := decodeProjection(t, out)
	if projection["command"] != "sync.repo.migrate.device-profile" || projection["status"] != "success" {
		t.Fatalf("projection: %s", out)
	}
	for _, secret := range []string{"AKID-CLI", "SECRET-CLI", "portable-content-key", "migration-passphrase"} {
		if strings.Contains(out+stderr, secret) {
			t.Fatalf("command leaked secret %q", secret)
		}
	}
}
