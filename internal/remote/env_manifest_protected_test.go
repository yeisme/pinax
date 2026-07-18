package remote

import (
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/vaultignore"
)

// TestManifestExcludesEnvSecretsByHardDeny proves that plaintext env files and
// the encrypted env asset are never uploaded as ordinary Capsa content entries,
// even when the user attempts to re-include them in .pinaxignore. Only the
// non-sensitive .env.example template may sync.
func TestManifestExcludesEnvSecretsByHardDeny(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, filepath.Join(root, "notes", "a.md"), "content\n")
	writeManifestFixture(t, filepath.Join(root, ".env"), "SECRET=plaintext\n")
	writeManifestFixture(t, filepath.Join(root, ".env.local"), "LOCAL=plaintext\n")
	writeManifestFixture(t, filepath.Join(root, "deploy.env"), "DEPLOY=plaintext\n")
	writeManifestFixture(t, filepath.Join(root, ".env.example"), "# template\nKEY=value\n")
	writeManifestFixture(t, filepath.Join(root, ".pinax", "pinax-sync.env.age"), "ciphertext\n")
	// User attempts to re-include env secrets — must NOT override the hard-deny.
	writeManifestFixture(t, filepath.Join(root, ".pinaxignore"), "!.env\n!.env.local\n!deploy.env\n")

	manifest, err := BuildManifest(root)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	blocked := map[string]bool{
		".env":                      true,
		".env.local":                true,
		"deploy.env":                true,
		".pinax/pinax-sync.env.age": true, // under .pinax/ hard-deny
	}
	for _, entry := range manifest.Entries {
		if blocked[entry.Path] {
			t.Fatalf("secret env path %q leaked into manifest entries: %#v", entry.Path, manifest.Entries)
		}
	}

	// Verify the matcher-level hard-deny directly.
	matcher, _ := vaultignore.Load(root)
	for path := range blocked {
		if !matcher.Ignored(path, false) {
			t.Fatalf("hard-deny failed for %q", path)
		}
	}
	// The non-sensitive template must be allowed.
	if matcher.Ignored(".env.example", false) {
		t.Fatalf(".env.example must be allowed as a non-sensitive template")
	}
}
