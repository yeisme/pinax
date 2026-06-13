package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestBuildsPathHashesAndBlobCache(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nsecret local body\n")
	writeManifestFixture(t, filepath.Join(root, "notes", "nested", "beta.md"), "# Beta\n")
	writeManifestFixture(t, filepath.Join(root, ".pinax", "config.yaml"), "ignored: true\n")
	manifest, err := BuildManifest(root)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if manifest.SchemaVersion != ManifestSchemaVersion || len(manifest.Entries) != 2 {
		t.Fatalf("manifest = %#v", manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	for _, forbidden := range []string{"secret local body"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("manifest leaked local body %q:\n%s", forbidden, encoded)
		}
	}
	if manifest.Entries[0].PathHash == manifest.Entries[1].PathHash || manifest.Entries[0].BlobID == "" {
		t.Fatalf("entries not hashed/stable: %#v", manifest.Entries)
	}
	for _, entry := range manifest.Entries {
		if !strings.HasPrefix(entry.PathHash, "path_") || !strings.HasPrefix(entry.BlobID, "blob_") {
			t.Fatalf("entry ids not prefixed: %#v", entry)
		}
		if _, err := os.Stat(filepath.Join(root, ".pinax", "cloud", "blob-cache", entry.BlobID)); err != nil {
			t.Fatalf("blob cache missing for %#v: %v", entry, err)
		}
	}
}

func TestManifestPathHashIsStable(t *testing.T) {
	if PathHash("notes\\Alpha.md") != PathHash("notes/Alpha.md") {
		t.Fatalf("path hash should normalize separators")
	}
	if PathHash("notes/Alpha.md") == PathHash("notes/Beta.md") {
		t.Fatalf("path hash collision for different paths")
	}
}

func writeManifestFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
