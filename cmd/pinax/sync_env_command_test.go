package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncEnvCommandTree(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "V", "--json")

	helpOut := runCLI(t, "sync", "env", "--help")
	for _, want := range []string{"init", "set", "list", "unlock", "clean", "doctor"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("sync env help missing %q:\n%s", want, helpOut)
		}
	}
}

func TestSyncEnvCommand_FullFlowNoPlaintextLeak(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "V", "--json")
	t.Setenv("PINAX_SYNC_FAKE_KEY", "cli-test-key")

	secret := "cli-secret-value-XYZ789"

	// init
	initOut := runCLI(t, "sync", "env", "init", "--vault", root, "--json")
	mustContain(t, initOut, "sync.env.init")
	if strings.Contains(initOut, secret) {
		t.Fatalf("init output leaked secret")
	}

	// set
	setOut := runCLI(t, "sync", "env", "set", "COS_SECRET", "--value", secret, "--vault", root, "--json")
	if strings.Contains(setOut, secret) {
		t.Fatalf("set output leaked plaintext secret:\n%s", setOut)
	}
	mustContain(t, setOut, "COS_SECRET")

	// list
	listOut := runCLI(t, "sync", "env", "list", "--vault", root, "--json")
	if strings.Contains(listOut, secret) {
		t.Fatalf("list output leaked plaintext secret:\n%s", listOut)
	}
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "sync.env.list" {
		t.Fatalf("expected command sync.env.list, got %v", listEnvelope["command"])
	}

	// unlock (in-memory, no materialize)
	unlockOut := runCLI(t, "sync", "env", "unlock", "--vault", root, "--json")
	if strings.Contains(unlockOut, secret) {
		t.Fatalf("unlock output leaked plaintext secret:\n%s", unlockOut)
	}

	// unlock --materialize creates 0600 file
	matOut := runCLI(t, "sync", "env", "unlock", "--vault", root, "--materialize", "--json")
	if strings.Contains(matOut, secret) {
		t.Fatalf("materialize output leaked plaintext secret:\n%s", matOut)
	}
	mustContain(t, matOut, "0600")
	info, err := os.Stat(filepath.Join(root, ".pinax", "runtime", "pinax-sync.env"))
	if err != nil {
		t.Fatalf("materialized file missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %v", info.Mode().Perm())
	}

	// clean removes the managed file
	runCLI(t, "sync", "env", "clean", "--vault", root, "--json")
	if _, err := os.Stat(filepath.Join(root, ".pinax", "runtime", "pinax-sync.env")); err == nil {
		t.Fatalf("clean must remove the managed file")
	}

	// doctor
	docOut := runCLI(t, "sync", "env", "doctor", "--vault", root, "--json")
	if strings.Contains(docOut, secret) {
		t.Fatalf("doctor output leaked plaintext secret:\n%s", docOut)
	}
}

func TestSyncEnvCommand_AgentMode(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "V", "--json")
	t.Setenv("PINAX_SYNC_FAKE_KEY", "cli-test-key")

	runCLI(t, "sync", "env", "init", "--vault", root, "--json")
	agentOut := runCLI(t, "sync", "env", "list", "--vault", root, "--agent")
	if !strings.Contains(agentOut, "mode=agent") || !strings.Contains(agentOut, "command=sync.env.list") {
		t.Fatalf("agent output missing expected keys:\n%s", agentOut)
	}
}

func TestSyncEnvCommand_GitignoreManagedBlock(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "V", "--json")
	t.Setenv("PINAX_SYNC_FAKE_KEY", "cli-test-key")

	runCLI(t, "sync", "env", "init", "--vault", root, "--json")
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("gitignore missing: %v", err)
	}
	for _, rule := range []string{".env", "*.env", ".pinax/runtime/", "!.pinax/pinax-sync.env.age", "!.env.example"} {
		if !strings.Contains(string(body), rule) {
			t.Fatalf("gitignore missing %q:\n%s", rule, string(body))
		}
	}
}

func mustContain(t *testing.T, out, want string) {
	t.Helper()
	if !strings.Contains(out, want) {
		t.Fatalf("expected output to contain %q:\n%s", want, out)
	}
}
