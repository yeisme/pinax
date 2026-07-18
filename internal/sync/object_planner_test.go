package syncplan

import (
	"testing"

	"github.com/yeisme/pinax/internal/remote"
)

func TestObjectDiffClassifiesRenameRevisionConflictAndPathCollision(t *testing.T) {
	base := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-a", ObjectKind: "note", Path: "notes/a.md", RevisionID: "r1", BlobID: "b1"}}}
	t.Run("rename", func(t *testing.T) {
		local := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-a", ObjectKind: "note", Path: "notes/renamed.md", RevisionID: "r1", BlobID: "b1"}}}
		plan, err := BuildPlan(Request{Direction: DirectionDiff, BaseManifest: base, LocalManifest: local, RemoteManifest: base})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Operations) != 1 || plan.Operations[0].Kind != "move" || plan.Operations[0].ObjectID != "id-a" || plan.Operations[0].FromPath != "notes/a.md" || plan.Operations[0].ToPath != "notes/renamed.md" {
			t.Fatalf("plan = %#v", plan)
		}
	})
	t.Run("revision conflict", func(t *testing.T) {
		local := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-a", ObjectKind: "note", Path: "notes/a.md", RevisionID: "local-r2", BlobID: "local"}}}
		remoteManifest := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-a", ObjectKind: "note", Path: "notes/a.md", RevisionID: "remote-r2", BlobID: "remote"}}}
		plan, err := BuildPlan(Request{Direction: DirectionDiff, BaseManifest: base, LocalManifest: local, RemoteManifest: remoteManifest})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Operations) != 1 || plan.Operations[0].Kind != "revision_conflict" || plan.Operations[0].ObjectID != "id-a" {
			t.Fatalf("plan = %#v", plan)
		}
	})
	t.Run("path collision", func(t *testing.T) {
		local := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-a", ObjectKind: "note", Path: "notes/shared.md", RevisionID: "r2", BlobID: "a"}}}
		remoteManifest := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: "id-b", ObjectKind: "note", Path: "notes/shared.md", RevisionID: "r2", BlobID: "b"}}}
		plan, err := BuildPlan(Request{Direction: DirectionDiff, LocalManifest: local, RemoteManifest: remoteManifest})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Operations) != 1 || plan.Operations[0].Kind != "path_collision" {
			t.Fatalf("plan = %#v", plan)
		}
	})
}

func TestObjectMoveCarriesBaseRevisionForSafeRuntimeUpdate(t *testing.T) {
	objectID := "01982d84-2b48-7000-8000-000000000071"
	base := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: objectID, ObjectKind: "note", Path: "notes/old.md", RevisionID: "r1"}}}
	local := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: objectID, ObjectKind: "note", Path: "notes/old.md", RevisionID: "r1"}}}
	remoteManifest := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{{ObjectID: objectID, ObjectKind: "note", Path: "notes/new.md", RevisionID: "r2"}}}
	plan, err := BuildPlan(Request{Direction: DirectionPull, BaseManifest: base, LocalManifest: local, RemoteManifest: remoteManifest, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Kind != "move" || plan.Operations[0].BaseRevision != "r1" || plan.Operations[0].LocalRevision != "r1" {
		t.Fatalf("operations = %#v", plan.Operations)
	}
}

func TestObjectDiffTreatsUnchangedLocalObjectAsRemoteDeletion(t *testing.T) {
	objectID := "01982d84-2b48-7000-8000-000000000072"
	entry := remote.ManifestEntry{ObjectID: objectID, ObjectKind: "note", Path: "notes/a.md", RevisionID: "r1", BlobID: "b1"}
	plan, err := BuildPlan(Request{Direction: DirectionDiff, BaseManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{entry}}, LocalManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{entry}}, RemoteManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2}, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Kind != "delete_local" {
		t.Fatalf("operations = %#v", plan.Operations)
	}
}
