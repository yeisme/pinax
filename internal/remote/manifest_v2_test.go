package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildManifestV2RequiresCanonicalIdentityForEveryEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BuildManifestV2(root, "device-a", nil)
	if err == nil || err.Error() != "manifest_identity_required: notes/alpha.md" {
		t.Fatalf("BuildManifestV2() error = %v", err)
	}
}

func TestBuildManifestV2DecoratesObjectIdentityAndRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildManifestV2(root, "device-a", map[string]ManifestIdentity{
		"notes/alpha.md": {ObjectID: "01982d84-2b48-7000-8000-000000000001", ObjectKind: "note"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != ManifestSchemaVersionV2 || len(manifest.Entries) != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}
	entry := manifest.Entries[0]
	if entry.ObjectID == "" || entry.ObjectKind != "note" || entry.RevisionID == "" || entry.DeviceID != "device-a" || entry.UpdatedAt == "" {
		t.Fatalf("entry = %#v", entry)
	}
	if err := manifest.ValidateV2(); err != nil {
		t.Fatalf("ValidateV2() error = %v", err)
	}
	entry.UpdatedAt = ""
	manifest.Entries[0] = entry
	if err := manifest.ValidateV2(); err == nil {
		t.Fatal("ValidateV2 accepted entry without update time")
	}
}
