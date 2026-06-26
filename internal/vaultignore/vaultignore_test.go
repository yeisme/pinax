package vaultignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatcherUsesPinaxignoreWithNegationAndHardDeny(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFixture(t, filepath.Join(root, ".pinaxignore"), "*.log\nsecret/\n!important.log\n**/cache/**\n")

	matcher, err := Load(root)
	if err != nil {
		t.Fatalf("load matcher: %v", err)
	}

	cases := map[string]bool{
		"debug.log":                        true,
		"important.log":                    false,
		"secret/token.txt":                 true,
		"notes/a.md":                       false,
		"notes/cache/generated.bin":        true,
		".pinax/config.yaml":               true,
		".pinax/cloud/blob-cache/blob_abc": true,
		".git/config":                      true,
	}
	for rel, wantIgnored := range cases {
		if got := matcher.Ignored(rel, false); got != wantIgnored {
			t.Fatalf("Ignored(%q) = %v, want %v", rel, got, wantIgnored)
		}
	}
}

func TestDefaultTemplatesUsePinaxAndGitBoundaries(t *testing.T) {
	pinax := DefaultPinaxIgnore()
	for _, want := range []string{".pinax/", ".git/", ".obsidian/", ".env*", "dist/"} {
		if !containsLine(pinax, want) {
			t.Fatalf("default .pinaxignore missing %q:\n%s", want, pinax)
		}
	}

	gitignore := MetadataOnlyGitignoreBlock()
	for _, want := range []string{"*", "!.pinax/", "!.pinax/config.yaml", "!.pinaxignore", "!.gitignore"} {
		if !containsLine(gitignore, want) {
			t.Fatalf("metadata gitignore missing %q:\n%s", want, gitignore)
		}
	}
	for _, want := range []string{".pinax/version/", ".pinax/last_snapshot", ".pinax/records/version.json"} {
		if !containsLine(gitignore, want) {
			t.Fatalf("metadata gitignore missing snapshot ignore %q:\n%s", want, gitignore)
		}
	}
}

func writeIgnoreFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func containsLine(body, want string) bool {
	for _, line := range splitLines(body) {
		if line == want {
			return true
		}
	}
	return false
}
