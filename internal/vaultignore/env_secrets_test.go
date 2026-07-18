package vaultignore

import (
	"strings"
	"testing"
)

func TestPlaintextEnvHardDenied(t *testing.T) {
	// Even with no .pinaxignore file at all, plaintext env must be hard-denied.
	matcher := Parse("")
	cases := map[string]bool{
		".env":                      true,
		".env.local":                true,
		".env.production":           true,
		"config.env":                true,
		"subdir/.env":               true,
		"subdir/app.env":            true,
		".env.example":              false, // non-sensitive template, exempt
		"subdir/.env.example":       false,
		"notes/a.md":                false,
		".pinax/pinax-sync.env.age": true, // under .pinax/ hard-deny
	}
	for rel, want := range cases {
		if got := matcher.Ignored(rel, false); got != want {
			t.Fatalf("Ignored(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestPlaintextEnvHardDenyNotOverridable(t *testing.T) {
	// A user re-include must NOT override the env hard-deny.
	matcher := Parse("!.env\n!.env.local\n")
	if !matcher.Ignored(".env", false) {
		t.Fatalf("user re-include must not override env hard-deny")
	}
	if !matcher.Ignored(".env.local", false) {
		t.Fatalf("user re-include must not override env hard-deny")
	}
}

func TestEnvSecretGitignoreBlock_ContainsRequiredRules(t *testing.T) {
	block := EnvSecretGitignoreBlock()
	required := []string{
		".env",
		".env.*",
		"*.env",
		".pinax/runtime/",
		"!.env.example",
		"!**/.env.example",
		"!.pinax/pinax-sync.env.age",
		beginEnvBlock,
		endEnvBlock,
	}
	for _, rule := range required {
		if !strings.Contains(block, rule) {
			t.Fatalf("env gitignore block missing %q:\n%s", rule, block)
		}
	}
}

func TestApplyEnvSecretGitignore_PreservesUserRules(t *testing.T) {
	existing := "# my rules\nbuild/\n*.tmp\n"
	updated := ApplyEnvSecretGitignore(existing)
	// User rules preserved.
	if !strings.Contains(updated, "# my rules") || !strings.Contains(updated, "build/") || !strings.Contains(updated, "*.tmp") {
		t.Fatalf("user rules not preserved:\n%s", updated)
	}
	// Block present exactly once.
	if c := strings.Count(updated, beginEnvBlock); c != 1 {
		t.Fatalf("expected block once, got %d", c)
	}
	// Block is present.
	if !strings.Contains(updated, ".env") {
		t.Fatalf("managed rules missing")
	}
}

func TestApplyEnvSecretGitignore_Idempotent(t *testing.T) {
	first := ApplyEnvSecretGitignore("")
	second := ApplyEnvSecretGitignore(first)
	if first != second {
		t.Fatalf("ApplyEnvSecretGitignore is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestApplyEnvSecretGitignore_RefreshesExistingBlock(t *testing.T) {
	// Simulate an outdated block missing the encrypted-asset re-include.
	stale := beginEnvBlock + "\n.env\n" + endEnvBlock + "\n"
	updated := ApplyEnvSecretGitignore(stale)
	if !strings.Contains(updated, "!.pinax/pinax-sync.env.age") {
		t.Fatalf("stale block was not refreshed:\n%s", updated)
	}
	if c := strings.Count(updated, beginEnvBlock); c != 1 {
		t.Fatalf("expected single block after refresh, got %d", c)
	}
}
