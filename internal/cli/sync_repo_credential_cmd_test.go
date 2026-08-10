package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// runCLIWithStdin runs the root command with a stdin string and returns
// (stdout, stderr, err). It mirrors runCLIJSON but allows piping the credential
// payload and capturing the deprecation warning on stderr.
func runCLIWithStdin(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCommand("test")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestSyncPullHelpIncludesUnifiedUnlockFlags(t *testing.T) {
	out, _, err := runCLIWithStdin(t, "", "sync", "pull", "--help")
	if err != nil {
		t.Fatalf("sync pull help: %v", err)
	}
	for _, flag := range []string{"--unlock", "--unlock-ref", "--passphrase-file", "--env-var"} {
		if !strings.Contains(out, flag) {
			t.Fatalf("help missing %s: %s", flag, out)
		}
	}
}

func TestDefaultRepositoryKeychainRefFallsBackToRuntimeWorkspace(t *testing.T) {
	root := mustInitVault(t)
	if _, err := pinaxremote.Login(root, pinaxremote.LoginRequest{
		Endpoint: "s3://bucket/prefix", WorkspaceID: "ws-keychain", DeviceID: "mac1", BackendKind: "s3-direct",
		S3: &pinaxremote.S3Config{Bucket: "bucket", Prefix: "prefix"},
	}); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	ref, err := defaultRepositoryKeychainRef(root)
	if err != nil {
		t.Fatalf("default ref: %v", err)
	}
	if ref != "keychain://pinax/pinax:ws-keychain" {
		t.Fatalf("ref: %q", ref)
	}
}

func TestResolveBootstrapUnlockSource(t *testing.T) {
	t.Setenv("PINAX_REPO_PASS", "env-pass")
	dir := t.TempDir()
	file := filepath.Join(dir, "pass")
	if err := os.WriteFile(file, []byte("file-pass\n"), 0o600); err != nil {
		t.Fatalf("write passphrase file: %v", err)
	}
	tests := []struct {
		name       string
		unlock     string
		unlockRef  string
		passFile   string
		descriptor string
	}{
		{name: "prompt", unlock: "prompt", descriptor: "prompt"},
		{name: "env", unlock: "env", descriptor: "env"},
		{name: "file", unlock: "file", passFile: file, descriptor: "file"},
		{name: "keychain", unlock: "keychain", unlockRef: "keychain://pinax/repo-account", descriptor: "keychain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, _, err := resolveBootstrapUnlockSource(test.unlock, test.unlockRef, test.passFile, "")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if source.Descriptor() != test.descriptor {
				t.Fatalf("descriptor: got %q want %q", source.Descriptor(), test.descriptor)
			}
		})
	}
	if _, _, err := resolveBootstrapUnlockSource("unknown", "", "", ""); err == nil {
		t.Fatal("unknown unlock mode accepted")
	}
	if _, _, err := resolveBootstrapUnlockSource("keychain", "bad-ref", "", ""); err == nil {
		t.Fatal("invalid keychain ref accepted")
	}
	if _, _, err := resolveBootstrapUnlockSource("", "", "", ""); err != nil {
		t.Fatalf("empty unlock should keep compile-only compatibility: %v", err)
	}
}

func mustInitVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", ".gitkeep"), []byte(""), 0o644); err != nil {
		t.Fatalf("gitkeep: %v", err)
	}
	return root
}

func decodeProjection(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	return p
}

const credentialPayload = `{"access_key_id":"AKIDTEST","secret_access_key":"SKTEST"}`

func TestSyncRepoCredentialInitSetList(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_CRED_PASS", "test-passphrase-1")
	out, _, err := runCLIWithStdin(t, "", "sync", "repo", "credential", "init",
		"--vault", root, "--env-var", "PINAX_CRED_PASS", "--json")
	if err != nil {
		t.Fatalf("credential init: %v\n%s", err, out)
	}
	p := decodeProjection(t, out)
	if p["command"] != "sync.repo.credential" || p["status"] != "success" {
		t.Fatalf("init projection: %s", out)
	}
	if strings.Contains(out, "test-passphrase-1") {
		t.Fatalf("init leaked passphrase: %s", out)
	}

	out, _, err = runCLIWithStdin(t, credentialPayload, "sync", "repo", "credential", "set",
		"--vault", root, "--name", "tencent-cos", "--stdin", "--env-var", "PINAX_CRED_PASS", "--json")
	if err != nil {
		t.Fatalf("credential set: %v\n%s", err, out)
	}
	p = decodeProjection(t, out)
	if p["command"] != "sync.repo.credential" {
		t.Fatalf("set projection: %s", out)
	}
	if strings.Contains(out, "AKIDTEST") || strings.Contains(out, "SKTEST") {
		t.Fatalf("set leaked payload: %s", out)
	}

	out, _, err = runCLIWithStdin(t, "", "sync", "repo", "credential", "list", "--vault", root, "--json")
	if err != nil {
		t.Fatalf("credential list: %v\n%s", err, out)
	}
	if strings.Contains(out, "AKIDTEST") {
		t.Fatalf("list leaked payload: %s", out)
	}
	p = decodeProjection(t, out)
	if p["command"] != "sync.repo.credential" {
		t.Fatalf("list projection: %s", out)
	}
}

func TestSyncRepoCredentialSetRequiresStdin(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_CRED_PASS", "test-passphrase-1")
	// init first
	_, _, _ = runCLIWithStdin(t, "", "sync", "repo", "credential", "init",
		"--vault", root, "--env-var", "PINAX_CRED_PASS", "--json")
	// set without --stdin must fail (no value flag accepted)
	_, _, err := runCLIWithStdin(t, "", "sync", "repo", "credential", "set",
		"--vault", root, "--name", "x", "--env-var", "PINAX_CRED_PASS", "--json")
	if err == nil {
		t.Fatal("credential set without --stdin accepted")
	}
}

func TestSyncRepoCredentialSetRejectsMalformedPayload(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_CRED_PASS", "test-passphrase-1")
	_, _, _ = runCLIWithStdin(t, "", "sync", "repo", "credential", "init",
		"--vault", root, "--env-var", "PINAX_CRED_PASS", "--json")
	out, _, err := runCLIWithStdin(t, `{"access_key_id":"only-one"}`,
		"sync", "repo", "credential", "set", "--vault", root, "--name", "x",
		"--stdin", "--env-var", "PINAX_CRED_PASS", "--json")
	if err == nil {
		t.Fatal("malformed payload accepted")
	}
	if !strings.Contains(out, "invalid_credential_payload") && !strings.Contains(out, "s3_credentials") {
		t.Fatalf("expected payload validation error, got: %s", out)
	}
}

func TestSyncRepoSecretSetValueDeprecationWarning(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_SYNC_FAKE_KEY", "fake-key")
	_, stderr, err := runCLIWithStdin(t, "", "sync", "repo", "secret", "set",
		"--vault", root, "--name", "enc", "--value", "plaintext-val",
		"--provider", "fake", "--json")
	if err != nil {
		t.Fatalf("secret set --value: %v", err)
	}
	if !strings.Contains(stderr, "deprecated") {
		t.Fatalf("expected deprecation warning on stderr, got: %q", stderr)
	}
}

func TestSyncRepoSecretSetStdinNoDeprecationWarning(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_SYNC_FAKE_KEY", "fake-key")
	_, stderr, err := runCLIWithStdin(t, "plaintext-val",
		"sync", "repo", "secret", "set", "--vault", root, "--name", "enc2",
		"--stdin", "--provider", "fake", "--json")
	if err != nil {
		t.Fatalf("secret set --stdin: %v", err)
	}
	if strings.Contains(stderr, "deprecated") {
		t.Fatalf("--stdin should not emit deprecation warning, got: %q", stderr)
	}
}

func TestSyncRepoBootstrapAdditiveFlagsSurfaceFacts(t *testing.T) {
	root := mustInitVault(t)
	t.Setenv("PINAX_CRED_PASS", "x")
	// init a declaration first so bootstrap can compile a runtime.
	_, _, err := runCLIWithStdin(t, "", "sync", "repo", "init", "--vault", root,
		"--backend-kind", "s3-direct", "--endpoint", "s3://bucket/prefix",
		"--workspace", "ws1", "--encryption-key-id", "key1", "--json")
	if err != nil {
		t.Fatalf("repo init: %v", err)
	}
	out, _, err := runCLIWithStdin(t, "", "sync", "repo", "bootstrap", "--vault", root,
		"--device", "mac2", "--yes", "--json")
	if err != nil {
		t.Fatalf("bootstrap: %v\n%s", err, out)
	}
	p := decodeProjection(t, out)
	if p["command"] != "sync.repo.bootstrap" {
		t.Fatalf("bootstrap projection: %s", out)
	}
	// Without --pull the compile-only behavior must still work and not set pull.
	out2, _, err := runCLIWithStdin(t, "", "sync", "repo", "bootstrap", "--vault", root,
		"--device", "mac3", "--yes", "--json")
	if err != nil {
		t.Fatalf("bootstrap compile-only: %v\n%s", err, out2)
	}
	p2 := decodeProjection(t, out2)
	facts2 := p2["facts"].(map[string]interface{})
	if _, hasPull := facts2["pull_applied"]; hasPull {
		t.Fatalf("compile-only bootstrap should not set pull fact: %s", out2)
	}
}
