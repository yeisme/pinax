package remote

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestDeleteMarkersAreAdditiveAndRedacted(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, filepath.Join(root, "notes", "active.md"), "# Active\n")
	writeManifestFixture(t, filepath.Join(root, ".pinax", "records", "tombstones.json"), `{"project/history":{"object_kind":"project","object_id":"project/history","tombstone_id":"trash_project_history","trash_path":".pinax/trash/20260627/projects/history/registry.json","deleted_at":"2026-06-27T00:00:00Z","source_command":"project.delete"}}`)
	writeManifestFixture(t, filepath.Join(root, ".pinax", "trash", "20260627", "projects", "history", "registry.json"), `{"slug":"history","name":"History","secret":"plain body must stay encrypted in blobs"}`)

	manifest, err := BuildManifest(root)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if len(manifest.Deletes) != 1 {
		t.Fatalf("delete markers = %#v", manifest.Deletes)
	}
	deleteMarker := manifest.Deletes[0]
	if deleteMarker.ObjectKind != "project" || deleteMarker.ObjectID != "project/history" || deleteMarker.TombstoneID != "trash_project_history" || deleteMarker.PathHash == "" || deleteMarker.TrashBlobID == "" {
		t.Fatalf("delete marker = %#v", deleteMarker)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	for _, forbidden := range []string{".pinax/trash/20260627/projects/history/registry.json", "plain body must stay encrypted in blobs", "Authorization", "token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("manifest delete marker leaked %q:\n%s", forbidden, encoded)
		}
	}

	legacy := []byte(`{"schema_version":"pinax.cloud.manifest.v1","generated_at":"2026-06-27T00:00:00Z","entry_count":0,"entries":[]}`)
	var parsed Manifest
	if err := json.Unmarshal(legacy, &parsed); err != nil {
		t.Fatalf("legacy manifest parse failed: %v", err)
	}
	if parsed.Deletes != nil {
		t.Fatalf("legacy manifest should keep deletes optional: %#v", parsed.Deletes)
	}
}
