package app

import (
	"encoding/json"
	"strings"
	"testing"

	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func syncOutputViewFixture() (syncplan.Plan, pinaxcloud.Manifest, pinaxcloud.Manifest, pinaxcloud.Manifest, syncOutputViewOptions) {
	plan := syncplan.Plan{
		SchemaVersion:    "pinax.sync.plan.v1",
		Status:           "planned",
		Direction:        syncplan.DirectionPush,
		Target:           "cloud",
		BaseRevision:     "rev_base",
		RemoteRevision:   "rev_remote",
		RequiresApproval: true,
		Operations: []syncplan.Operation{
			{Kind: "upload_blob", ObjectID: "obj_alpha", Path: "notes/alpha.md", BlobID: "blob_alpha", Status: "planned"},
			{Kind: "upload_blob", ObjectID: "obj_beta", Path: "notes/beta.md", BlobID: "blob_beta", Status: "planned"},
			{Kind: "delete_remote", ObjectID: "obj_gamma", Path: "notes/gamma.md", Status: "planned"},
			{Kind: "move", ObjectID: "obj_delta", FromPath: "notes/old-delta.md", ToPath: "notes/delta.md", Status: "planned"},
			{Kind: "conflict", ObjectID: "obj_eps", Path: "notes/epsilon.md", Status: "planned"},
			{Kind: "upload_manifest", Path: "manifest", Status: "planned"},
		},
	}
	base := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion, Entries: []pinaxcloud.ManifestEntry{
		{ObjectID: "obj_beta", Path: "notes/beta.md", PathHash: "hash_beta", BlobID: "blob_beta_old", Size: 40, RevisionID: "rev_base"},
		{ObjectID: "obj_zeta", Path: "notes/zeta.md", PathHash: "hash_zeta", BlobID: "blob_zeta", Size: 70, RevisionID: "rev_base"},
	}}
	local := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion, Entries: []pinaxcloud.ManifestEntry{
		{ObjectID: "obj_alpha", Path: "notes/alpha.md", PathHash: "hash_alpha", BlobID: "blob_alpha", Size: 100, RevisionID: "rev_local"},
		{ObjectID: "obj_beta", Path: "notes/beta.md", PathHash: "hash_beta", BlobID: "blob_beta", Size: 50, RevisionID: "rev_local"},
		{ObjectID: "obj_zeta", Path: "notes/zeta.md", PathHash: "hash_zeta", BlobID: "blob_zeta", Size: 70, RevisionID: "rev_base"},
		{ObjectID: "obj_delta", Path: "notes/delta.md", PathHash: "hash_delta", BlobID: "blob_delta", Size: 60, RevisionID: "rev_local"},
	}}
	remote := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion, Entries: []pinaxcloud.ManifestEntry{
		{ObjectID: "obj_beta", Path: "notes/beta.md", PathHash: "hash_beta", BlobID: "blob_beta_old", Size: 40, RevisionID: "rev_remote"},
		{ObjectID: "obj_gamma", Path: "notes/gamma.md", PathHash: "hash_gamma", BlobID: "blob_gamma", Size: 200, RevisionID: "rev_remote"},
		{ObjectID: "obj_zeta", Path: "notes/zeta.md", PathHash: "hash_zeta", BlobID: "blob_zeta", Size: 70, RevisionID: "rev_base"},
	}}
	opts := syncOutputViewOptions{Scope: "remote-aware", Result: "applied", RemoteAfter: "rev_after", LocalAfter: "rev_after"}
	return plan, base, local, remote, opts
}

// TestBuildSyncOutputViewGolden locks the pinax.sync.output.v1 wire shape:
// A/M/D/R/C classification, counts, bytes, ordering, revisions, and the
// shown/total contract. Update the golden only with a deliberate spec change.
func TestBuildSyncOutputViewGolden(t *testing.T) {
	t.Parallel()
	plan, base, local, remote, opts := syncOutputViewFixture()
	view := buildSyncOutputView(plan, base, local, remote, opts)
	if view.SchemaVersion != "pinax.sync.output.v1" {
		t.Fatalf("schema = %s", view.SchemaVersion)
	}
	if view.Counts.Added != 1 || view.Counts.Modified != 1 || view.Counts.Deleted != 1 || view.Counts.Renamed != 1 || view.Counts.Conflicts != 1 {
		t.Fatalf("counts = %+v", view.Counts)
	}
	if view.Bytes.Uploaded != 150 {
		t.Fatalf("bytes uploaded = %d, want alpha(100)+beta(50)", view.Bytes.Uploaded)
	}
	if view.Total != 5 || view.Shown != 5 || view.Truncated {
		t.Fatalf("shown/total/truncated = %d/%d/%v", view.Shown, view.Total, view.Truncated)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	golden := strings.Join([]string{
		`{"schema_version":"pinax.sync.output.v1","direction":"push","scope":"remote-aware","result":"applied",`,
		`"revisions":{"base":"rev_base","remote_before":"rev_remote","remote_after":"rev_after","local_after":"rev_after"},`,
		`"counts":{"added":1,"modified":1,"deleted":1,"renamed":1,"conflicts":1,"unchanged":1,"total":6},`,
		`"bytes":{"uploaded":150,"downloaded":0},`,
		`"changes":[`,
		`{"code":"A","state":"applied","operation":"upload_blob","side":"local","path":"notes/alpha.md","size_bytes":100},`,
		`{"code":"M","state":"applied","operation":"upload_blob","side":"local","path":"notes/beta.md","size_bytes":50},`,
		`{"code":"R","state":"applied","operation":"move","side":"both","path":"notes/delta.md","from_path":"notes/old-delta.md","to_path":"notes/delta.md","size_bytes":60},`,
		`{"code":"C","state":"conflict","operation":"conflict","side":"both","path":"notes/epsilon.md"},`,
		`{"code":"D","state":"applied","operation":"delete_remote","side":"local","path":"notes/gamma.md","size_bytes":200}`,
		`],"shown":5,"total":5,"truncated":false}`,
	}, "")
	if string(encoded) != golden {
		t.Fatalf("golden mismatch:\n got: %s\nwant: %s", encoded, golden)
	}
}

func TestBuildSyncOutputViewDefaultsAndUpToDate(t *testing.T) {
	t.Parallel()
	empty := syncplan.Plan{Direction: syncplan.DirectionPull, Target: "cloud"}
	view := buildSyncOutputView(empty, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, syncOutputViewOptions{})
	if view.Scope != "cached" || view.Result != "planned" {
		t.Fatalf("defaults scope/result = %s/%s", view.Scope, view.Result)
	}
	upToDate := buildSyncOutputView(empty, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, syncOutputViewOptions{Scope: "remote-aware", Result: "up_to_date", RemoteAfter: "rev_head", LocalAfter: "rev_head"})
	if upToDate.Result != "up_to_date" || upToDate.Revisions.RemoteAfter != "rev_head" {
		t.Fatalf("up_to_date view = %+v", upToDate)
	}
}

func TestBuildSyncOutputViewPathRedaction(t *testing.T) {
	t.Parallel()
	plan, base, local, remote, _ := syncOutputViewFixture()
	hashed := buildSyncOutputView(plan, base, local, remote, syncOutputViewOptions{Scope: "remote-aware", Result: "applied", PathPolicy: "hash"})
	for _, change := range hashed.Changes {
		if change.Path != "" && !strings.HasPrefix(change.Path, "path_sha256:") {
			t.Fatalf("hash policy left plaintext path %q", change.Path)
		}
	}
	omitted := buildSyncOutputView(plan, base, local, remote, syncOutputViewOptions{Scope: "remote-aware", Result: "applied", PathPolicy: "omit"})
	for _, change := range omitted.Changes {
		if change.Path != "" || change.FromPath != "" || change.ToPath != "" {
			t.Fatalf("omit policy left path %q/%q/%q", change.Path, change.FromPath, change.ToPath)
		}
	}
}

func TestAttachSyncOutputViewFactsAndData(t *testing.T) {
	t.Parallel()
	plan, base, local, remote, opts := syncOutputViewFixture()
	view := buildSyncOutputView(plan, base, local, remote, opts)
	facts := map[string]string{}
	data := map[string]any{}
	attachSyncOutputView(facts, data, view)
	if facts["sync.result"] != "applied" || facts["sync.added"] != "1" || facts["sync.unchanged"] != "1" || facts["sync.bytes_uploaded"] != "150" {
		t.Fatalf("facts = %#v", facts)
	}
	if _, ok := data["sync_view"].(syncOutputView); !ok {
		t.Fatalf("data sync_view missing: %#v", data)
	}
}
