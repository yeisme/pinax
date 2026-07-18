package cloudsync

import "testing"

func TestManifestDeleteMarkersValidateAndExposeTrashBlobIDs(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Entries:       []ManifestEntry{{Path: "notes/alpha.md", BlobID: "blob_alpha", PlainSHA256: "sha", Size: 12, UpdatedAt: "2026-06-27T00:00:00Z"}},
		Deletes: []ManifestDelete{{
			PathHash:    "path_abc123",
			ObjectKind:  "project",
			ObjectID:    "project/history",
			TombstoneID: "trash_project_history",
			DeletedAt:   "2026-06-27T00:00:00Z",
			TrashBlobID: "blob_trash_backup",
		}},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	ids := manifest.BlobIDs()
	if len(ids) != 2 || ids[0] != "blob_alpha" || ids[1] != "blob_trash_backup" {
		t.Fatalf("blob ids = %#v", ids)
	}

	invalid := manifest
	invalid.Deletes[0].PathHash = ".pinax/trash/20260627/projects/history/registry.json"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("manifest accepted plaintext path hash")
	}
	invalid = manifest
	invalid.Deletes[0].TrashBlobID = "Authorization: Bearer token"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("manifest accepted unsafe trash blob id")
	}
}
