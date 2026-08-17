package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenCLICreateListRevoke(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// List empty
	listOut := runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "No tokens.") {
		t.Fatalf("expected empty token list, got: %s", listOut)
	}

	// Create token
	createOut := runCLI(t, "token", "create", "--label", "test-agent", "--scope", "read", "--vault", root)
	if !strings.Contains(createOut, "Token ID:") || !strings.Contains(createOut, "Secret:") {
		t.Fatalf("token create output missing ID or Secret: %s", createOut)
	}
	// Extract token ID
	lines := strings.Split(createOut, "\n")
	var tokenID string
	for _, line := range lines {
		if strings.HasPrefix(line, "Token ID:") {
			tokenID = strings.TrimSpace(strings.TrimPrefix(line, "Token ID:"))
		}
	}
	if tokenID == "" {
		t.Fatalf("failed to extract token ID from: %s", createOut)
	}

	// List with token
	listOut = runCLI(t, "token", "list", "--vault", root)
	for _, want := range []string{"Tokens", "ID", "Label", "Created", "Scope", "Expires", "test-agent", tokenID} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("token list missing %q: %s", want, listOut)
		}
	}
	listAgentOut := runCLI(t, "token", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=token.list", "fact.tokens=1", "token.1.id=" + tokenID, "token.1.label=test-agent", "token.1.scope=read"} {
		if !strings.Contains(listAgentOut, want) {
			t.Fatalf("token list agent missing %q:\n%s", want, listAgentOut)
		}
	}

	// Revoke token
	revokeOut := runCLI(t, "token", "revoke", tokenID, "--vault", root)
	if !strings.Contains(revokeOut, "Revoked token:") {
		t.Fatalf("token revoke output: %s", revokeOut)
	}
	revokeAgentOut := runCLI(t, "token", "create", "--label", "machine-revoke", "--scope", "read", "--vault", root, "--agent")
	createdID := ""
	for _, line := range strings.Split(revokeAgentOut, "\n") {
		if strings.HasPrefix(line, "fact.token_id=") {
			createdID = strings.TrimPrefix(line, "fact.token_id=")
		}
	}
	if createdID == "" {
		t.Fatalf("token create agent missing token id:\n%s", revokeAgentOut)
	}
	revokeAgentOut = runCLI(t, "token", "revoke", createdID, "--vault", root, "--agent")
	for _, want := range []string{"command=token.revoke", "status=success", "fact.token_id=" + createdID} {
		if !strings.Contains(revokeAgentOut, want) {
			t.Fatalf("token revoke agent missing %q:\n%s", want, revokeAgentOut)
		}
	}

	// List should be empty again
	listOut = runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "No tokens.") {
		t.Fatalf("expected empty list after revoke, got: %s", listOut)
	}
}

func TestTokenCLICreateWithExpiry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	createOut := runCLI(t, "token", "create", "--label", "temp", "--scope", "read,write", "--expires", "30d", "--vault", root)
	if !strings.Contains(createOut, "Secret:") {
		t.Fatalf("token create with expiry: %s", createOut)
	}
}

func TestTokenCLIMachineModesUseProjectionAndDoNotPrintSecret(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"--json", "--agent"} {
		root := t.TempDir()
		runCLI(t, "init", root, "--title", "Vault", "--json")
		out := runCLI(t, "token", "create", "--label", "machine", "--scope", "read", "--vault", root, mode)
		if strings.Contains(out, "Secret:") || strings.Contains(out, "Save this secret") || strings.Contains(out, "请妥善保存") {
			t.Fatalf("token create %s printed human secret text: %s", mode, out)
		}
		if mode == "--json" {
			var envelope map[string]any
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatalf("token create --json did not emit JSON envelope: %v\n%s", err, out)
			}
			if envelope["command"] != "token.create" || envelope["status"] != "success" {
				t.Fatalf("token create json envelope = %#v", envelope)
			}
		} else if !strings.Contains(out, "command=token.create") || !strings.Contains(out, "status=success") {
			t.Fatalf("token create --agent output = %s", out)
		}
	}
}

func TestTokenCLIRotate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// Create token
	createOut := runCLI(t, "token", "create", "--label", "rotate-me", "--vault", root)
	lines := strings.Split(createOut, "\n")
	var oldID string
	for _, line := range lines {
		if strings.HasPrefix(line, "Token ID:") {
			oldID = strings.TrimSpace(strings.TrimPrefix(line, "Token ID:"))
		}
	}

	// Rotate token
	rotateOut := runCLI(t, "token", "rotate", oldID, "--vault", root)
	if !strings.Contains(rotateOut, "New token ID:") || !strings.Contains(rotateOut, "Secret:") {
		t.Fatalf("token rotate output: %s", rotateOut)
	}
	if !strings.Contains(rotateOut, "Rotated from:") {
		t.Fatalf("token rotate missing rotated-from: %s", rotateOut)
	}

	// Old token should be gone
	listOut := runCLI(t, "token", "list", "--vault", root)
	if strings.Contains(listOut, oldID) {
		t.Fatalf("old token should be revoked after rotate: %s", listOut)
	}
}

func TestProfileCLIAddListRemove(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// List empty
	listOut := runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "No profiles.") {
		t.Fatalf("expected empty profile list, got: %s", listOut)
	}

	// Add profile
	addOut := runCLI(t, "profile", "add", "my-s3", "--endpoint", "s3://bucket/path", "--workspace", "default", "--vault", root)
	if !strings.Contains(addOut, "Added profile:") {
		t.Fatalf("profile add output: %s", addOut)
	}
	addAgentOut := runCLI(t, "profile", "add", "machine-s3", "--endpoint", "s3://bucket/machine", "--workspace", "machine", "--vault", root, "--agent")
	for _, want := range []string{"command=profile.add", "status=success", "fact.profile=machine-s3", "fact.endpoint=s3://bucket/machine", "fact.workspace=machine"} {
		if !strings.Contains(addAgentOut, want) {
			t.Fatalf("profile add agent missing %q:\n%s", want, addAgentOut)
		}
	}

	// List with profile
	listOut = runCLI(t, "profile", "list", "--vault", root)
	for _, want := range []string{"Profiles", "Name", "Endpoint", "Workspace", "Device", "Scope", "my-s3", "machine-s3", "s3://bucket/path", "default"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("profile list missing %q: %s", want, listOut)
		}
	}
	agentListOut := runCLI(t, "profile", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=profile.list", "fact.profiles=2", "profile.1.name=machine-s3", "profile.1.endpoint=s3://bucket/machine", "profile.1.workspace=machine", "profile.2.name=my-s3", "profile.2.endpoint=s3://bucket/path", "profile.2.workspace=default"} {
		if !strings.Contains(agentListOut, want) {
			t.Fatalf("profile list agent missing %q:\n%s", want, agentListOut)
		}
	}

	// Show profile
	showOut := runCLI(t, "profile", "show", "my-s3", "--vault", root)
	if !strings.Contains(showOut, "my-s3") || !strings.Contains(showOut, "s3://bucket/path") {
		t.Fatalf("profile show output: %s", showOut)
	}
	for _, want := range []string{"Profile details", "Field", "Value", "Profile", "Endpoint", "Workspace", "Device", "Scope", "my-s3", "s3://bucket/path", "default"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("profile show default missing %q:\n%s", want, showOut)
		}
	}
	showAgentOut := runCLI(t, "profile", "show", "my-s3", "--vault", root, "--agent")
	for _, want := range []string{"command=profile.show", "fact.profile=my-s3", "fact.endpoint=s3://bucket/path", "fact.workspace=default", "profile_detail.name=my-s3", "profile_detail.endpoint=s3://bucket/path", "profile_detail.workspace=default"} {
		if !strings.Contains(showAgentOut, want) {
			t.Fatalf("profile show agent missing %q:\n%s", want, showAgentOut)
		}
	}

	// Remove profile
	removeOut := runCLI(t, "profile", "remove", "my-s3", "--vault", root)
	if !strings.Contains(removeOut, "Deleted profile:") {
		t.Fatalf("profile remove output: %s", removeOut)
	}
	removeAgentOut := runCLI(t, "profile", "remove", "machine-s3", "--vault", root, "--agent")
	for _, want := range []string{"command=profile.remove", "status=success", "fact.profile=machine-s3"} {
		if !strings.Contains(removeAgentOut, want) {
			t.Fatalf("profile remove agent missing %q:\n%s", want, removeAgentOut)
		}
	}

	// List should be empty again
	listOut = runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "No profiles.") {
		t.Fatalf("expected empty list after remove, got: %s", listOut)
	}
}

func TestProfileCLIAddRequiresEndpoint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	_, err := runCLIExpectError("profile", "add", "bad", "--vault", root)
	if err == nil {
		t.Fatal("expected error when adding profile without --endpoint")
	}
}
