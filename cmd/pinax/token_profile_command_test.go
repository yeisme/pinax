package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenCLICreateListRevoke(t *testing.T) {
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
	if !strings.Contains(listOut, "test-agent") {
		t.Fatalf("token list should show test-agent: %s", listOut)
	}
	if !strings.Contains(listOut, tokenID) {
		t.Fatalf("token list should show ID %s: %s", tokenID, listOut)
	}

	// Revoke token
	revokeOut := runCLI(t, "token", "revoke", tokenID, "--vault", root)
	if !strings.Contains(revokeOut, "Revoked token:") {
		t.Fatalf("token revoke output: %s", revokeOut)
	}

	// List should be empty again
	listOut = runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "No tokens.") {
		t.Fatalf("expected empty list after revoke, got: %s", listOut)
	}
}

func TestTokenCLICreateWithExpiry(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	createOut := runCLI(t, "token", "create", "--label", "temp", "--scope", "read,write", "--expires", "30d", "--vault", root)
	if !strings.Contains(createOut, "Secret:") {
		t.Fatalf("token create with expiry: %s", createOut)
	}
}

func TestTokenCLIMachineModesUseProjectionAndDoNotPrintSecret(t *testing.T) {
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

	// List with profile
	listOut = runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "my-s3") {
		t.Fatalf("profile list should show my-s3: %s", listOut)
	}

	// Show profile
	showOut := runCLI(t, "profile", "show", "my-s3", "--vault", root)
	if !strings.Contains(showOut, "my-s3") || !strings.Contains(showOut, "s3://bucket/path") {
		t.Fatalf("profile show output: %s", showOut)
	}

	// Remove profile
	removeOut := runCLI(t, "profile", "remove", "my-s3", "--vault", root)
	if !strings.Contains(removeOut, "Deleted profile:") {
		t.Fatalf("profile remove output: %s", removeOut)
	}

	// List should be empty again
	listOut = runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "No profiles.") {
		t.Fatalf("expected empty list after remove, got: %s", listOut)
	}
}

func TestProfileCLIAddRequiresEndpoint(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	_, err := runCLIExpectError("profile", "add", "bad", "--vault", root)
	if err == nil {
		t.Fatal("expected error when adding profile without --endpoint")
	}
}
