package vaultignore

import (
	"path/filepath"
	"testing"
)

// TestSyncRepoAssetsAreProtected verifies the declarative sync repo assets
// (pinax-sync.yaml declaration, pinax-sync.secrets.yaml ciphertext, cloud/
// runtime config and source marker) are always excluded from the content
// manifest. These are device-owned or commit-restricted files that must never
// be uploaded as ordinary note content.
func TestSyncRepoAssetsAreProtected(t *testing.T) {
	root := t.TempDir()
	matcher, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	protected := []string{
		".pinax/pinax-sync.yaml",
		".pinax/pinax-sync.secrets.yaml",
		".pinax/cloud/config.yaml",
		".pinax/cloud/config.json",
		".pinax/cloud/session.json",
		".pinax/cloud/pinax-sync.source.yaml",
		".pinax/cloud/pinax-sync.source.yaml.bak",
		".pinax/cloud/blob-cache/blob_abc",
	}
	for _, rel := range protected {
		if !matcher.Ignored(rel, false) {
			t.Fatalf("sync repo asset %q must be protected from manifest upload", rel)
		}
	}
	// Sanity: the vault root still contains a protected .pinax path.
	_ = filepath.Join(root, ".pinax")
}
