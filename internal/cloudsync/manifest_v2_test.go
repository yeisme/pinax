package cloudsync

import "testing"

func TestManifestV2ValidatesObjectRevisionAndDeviceFacts(t *testing.T) {
	manifest := Manifest{SchemaVersion: ManifestSchemaVersionV2, Entries: []ManifestEntry{{ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101", ObjectKind: "note", Path: "notes/a.md", RevisionID: "objrev_a", DeviceID: "device_a", BlobID: "blob_a", SHA256: "sha", Size: 1, UpdatedAt: "2026-07-10T00:00:00Z"}}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestV2RejectsMissingOrDuplicateObjectFacts(t *testing.T) {
	valid := ManifestEntry{ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101", ObjectKind: "note", Path: "notes/a.md", RevisionID: "objrev_a", DeviceID: "device_a", BlobID: "blob_a", SHA256: "sha", Size: 1, UpdatedAt: "2026-07-10T00:00:00Z"}
	cases := []struct {
		name string
		edit func(*ManifestEntry)
	}{
		{name: "object id", edit: func(entry *ManifestEntry) { entry.ObjectID = "" }},
		{name: "kind", edit: func(entry *ManifestEntry) { entry.ObjectKind = "" }},
		{name: "path", edit: func(entry *ManifestEntry) { entry.Path = "" }},
		{name: "revision", edit: func(entry *ManifestEntry) { entry.RevisionID = "" }},
		{name: "device", edit: func(entry *ManifestEntry) { entry.DeviceID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := valid
			tc.edit(&entry)
			if err := (Manifest{SchemaVersion: ManifestSchemaVersionV2, Entries: []ManifestEntry{entry}}).Validate(); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	duplicateID := valid
	duplicateID.Path = "notes/b.md"
	if err := (Manifest{SchemaVersion: ManifestSchemaVersionV2, Entries: []ManifestEntry{valid, duplicateID}}).Validate(); err == nil {
		t.Fatal("duplicate object id accepted")
	}
	duplicatePath := valid
	duplicatePath.ObjectID = "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102"
	if err := (Manifest{SchemaVersion: ManifestSchemaVersionV2, Entries: []ManifestEntry{valid, duplicatePath}}).Validate(); err == nil {
		t.Fatal("duplicate active path accepted")
	}
}

func TestManifestV1DecoderRemainsCompatible(t *testing.T) {
	manifest := Manifest{SchemaVersion: ManifestSchemaVersionV1, Entries: []ManifestEntry{{Path: "notes/a.md", BlobID: "blob_a", SHA256: "sha"}}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}
