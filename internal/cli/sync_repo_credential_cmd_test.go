package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"--device", "mac2", "--unlock", "prompt", "--pull", "--remember-keychain", "--yes", "--json")
	if err != nil {
		t.Fatalf("bootstrap: %v\n%s", err, out)
	}
	p := decodeProjection(t, out)
	if p["command"] != "sync.repo.bootstrap" {
		t.Fatalf("bootstrap projection: %s", out)
	}
	facts := p["facts"].(map[string]interface{})
	if facts["unlock"] != "prompt" {
		t.Fatalf("unlock fact: %v", facts["unlock"])
	}
	if facts["pull_planned"] != "true" {
		t.Fatalf("pull_planned fact: %v", facts["pull_planned"])
	}
	if facts["remember_keychain"] != "true" {
		t.Fatalf("remember_keychain fact: %v", facts["remember_keychain"])
	}
	// Without --pull the compile-only behavior must still work and not set pull.
	out2, _, err := runCLIWithStdin(t, "", "sync", "repo", "bootstrap", "--vault", root,
		"--device", "mac3", "--yes", "--json")
	if err != nil {
		t.Fatalf("bootstrap compile-only: %v\n%s", err, out2)
	}
	p2 := decodeProjection(t, out2)
	facts2 := p2["facts"].(map[string]interface{})
	if _, hasPull := facts2["pull_planned"]; hasPull {
		t.Fatalf("compile-only bootstrap should not set pull fact: %s", out2)
	}
}
