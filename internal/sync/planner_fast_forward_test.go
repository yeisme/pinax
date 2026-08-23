package syncplan

import (
	"testing"

	"github.com/yeisme/pinax/internal/remote"
)

// v2 (object-keyed) planner: entries carry ObjectID, so BuildPlan dispatches
// to diffObjectManifests — the branch whose download_blob ops now carry the
// fast-forward proof.
func objectFixture(localBlob, baseBlob, localRev, baseRev, remoteBlob, remoteRev string) Request {
	entry := func(blob, rev string) remote.ManifestEntry {
		return remote.ManifestEntry{ObjectID: "obj_alpha", Path: "notes/alpha.md", PathHash: "path_alpha", BlobID: blob, RevisionID: rev}
	}
	return Request{
		Direction:      DirectionPull,
		Target:         "cloud",
		BaseRevision:   "rev_base",
		RemoteRevision: "rev_remote",
		LocalManifest:  remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{entry(localBlob, localRev)}},
		BaseManifest:   remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{entry(baseBlob, baseRev)}},
		RemoteManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: []remote.ManifestEntry{entry(remoteBlob, remoteRev)}},
	}
}

func objectDownloadOperation(t *testing.T, req Request) (Operation, bool) {
	t.Helper()
	plan, err := BuildPlan(req)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	for _, op := range plan.Operations {
		if op.Kind == "download_blob" {
			return op, true
		}
		if op.Kind == "revision_conflict" {
			return op, false
		}
	}
	t.Fatalf("no download_blob or revision_conflict operation in plan: %#v", plan.Operations)
	return Operation{}, false
}

func TestObjectPullFastForwardWhenLocalRevisionMatchesBase(t *testing.T) {
	t.Parallel()
	op, ok := objectDownloadOperation(t, objectFixture("blob_v1", "blob_v1", "rev_1", "rev_1", "blob_v2", "rev_2"))
	if !ok || !op.FastForward {
		t.Fatalf("local==base by revision must fast-forward: %#v", op)
	}
	if op.LocalBlobID != "blob_v1" || op.BaseRevision != "rev_1" {
		t.Fatalf("op fields = %#v", op)
	}
}

func TestObjectPullFastForwardWhenLocalBlobMatchesBase(t *testing.T) {
	t.Parallel()
	// Content-addressed proof holds even with empty revision ids.
	op, ok := objectDownloadOperation(t, objectFixture("blob_v1", "blob_v1", "", "", "blob_v2", "rev_2"))
	if !ok || !op.FastForward {
		t.Fatalf("local==base by blob must fast-forward: %#v", op)
	}
}

func TestObjectPullConflictWhenLocalDivergedFromBase(t *testing.T) {
	t.Parallel()
	// Both sides diverged from base: the object planner classifies a
	// revision conflict, whose apply path still preserves a conflict copy.
	op, isDownload := objectDownloadOperation(t, objectFixture("blob_local", "blob_v1", "rev_local", "rev_1", "blob_v2", "rev_2"))
	if isDownload {
		t.Fatalf("diverged local must classify revision_conflict, got download: %#v", op)
	}
}

func TestObjectPullPreservesWhenV1RevisionsProveNothing(t *testing.T) {
	t.Parallel()
	// Entries carrying ObjectID but empty revision ids with differing blobs
	// prove nothing: no fast-forward on the download.
	op, ok := objectDownloadOperation(t, objectFixture("blob_local", "blob_v1", "", "", "blob_v2", "rev_2"))
	if !ok {
		t.Fatalf("expected download_blob classification: %#v", op)
	}
	if op.FastForward {
		t.Fatalf("empty revisions with differing blobs must not fast-forward: %#v", op)
	}
}

func TestObjectPullPreservesWithoutBase(t *testing.T) {
	t.Parallel()
	req := objectFixture("blob_v1", "blob_v1", "rev_1", "rev_1", "blob_v2", "rev_2")
	req.BaseManifest = remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2}
	op, ok := objectDownloadOperation(t, req)
	if !ok {
		t.Fatalf("expected download_blob classification: %#v", op)
	}
	if op.FastForward {
		t.Fatalf("missing base must not fast-forward: %#v", op)
	}
}

// v1 (path-keyed) planner: its "changed remotely, untouched locally" download
// is blob-proven, so it must carry the fast-forward flag directly.
func TestPathPullFastForwardWhenLocalBlobMatchesBase(t *testing.T) {
	t.Parallel()
	entry := func(blob string) remote.ManifestEntry {
		return remote.ManifestEntry{Path: "notes/alpha.md", PathHash: "path_alpha", BlobID: blob}
	}
	req := Request{
		Direction:      DirectionPull,
		Target:         "cloud",
		BaseRevision:   "rev_base",
		RemoteRevision: "rev_remote",
		LocalManifest:  remote.Manifest{Entries: []remote.ManifestEntry{entry("blob_v1")}},
		BaseManifest:   remote.Manifest{Entries: []remote.ManifestEntry{entry("blob_v1")}},
		RemoteManifest: remote.Manifest{Entries: []remote.ManifestEntry{entry("blob_v2")}},
	}
	plan, err := BuildPlan(req)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	for _, op := range plan.Operations {
		if op.Kind == "download_blob" {
			if !op.FastForward || op.LocalBlobID != "blob_v1" {
				t.Fatalf("v1 untouched-local download must fast-forward: %#v", op)
			}
			return
		}
	}
	t.Fatalf("no download_blob in plan: %#v", plan.Operations)
}
