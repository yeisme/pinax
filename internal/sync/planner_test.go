package syncplan

import (
	"errors"
	"testing"

	"github.com/yeisme/pinax/internal/remote"
)

func TestSyncPlannerDryRunPushPlan(t *testing.T) {
	manifest := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersion, EntryCount: 2, Entries: []remote.ManifestEntry{{Path: "a.md", PathHash: "path_a", BlobID: "blob_a", Size: 10}, {Path: "b.md", PathHash: "path_b", BlobID: "blob_b", Size: 20}}}
	plan, err := BuildPlan(Request{Direction: DirectionPush, Target: "cloud", LocalManifest: manifest, BaseRevision: "rev_1", RemoteRevision: "rev_1", DryRun: true})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.SchemaVersion != PlanSchemaVersion || plan.Direction != DirectionPush || !plan.DryRun || plan.RemoteWrite {
		t.Fatalf("plan flags = %#v", plan)
	}
	if len(plan.Operations) != 3 {
		t.Fatalf("operations = %#v", plan.Operations)
	}
	if plan.Operations[0].Kind != "upload_blob" || plan.Operations[2].Kind != "upload_manifest" {
		t.Fatalf("operation order = %#v", plan.Operations)
	}
}

func TestSyncPlannerRevisionConflict(t *testing.T) {
	manifest := remote.Manifest{SchemaVersion: remote.ManifestSchemaVersion, EntryCount: 1, Entries: []remote.ManifestEntry{{PathHash: "path_a", BlobID: "blob_a"}}}
	plan, err := BuildPlan(Request{Direction: DirectionPush, Target: "cloud", LocalManifest: manifest, BaseRevision: "rev_1", RemoteRevision: "rev_2", DryRun: false, Yes: true})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("err = %v", err)
	}
	if plan.Status != "conflict" || len(plan.ConflictQueue) != 1 || plan.ConflictQueue[0].Code != "REVISION_CONFLICT" {
		t.Fatalf("conflict plan = %#v", plan)
	}
	if plan.RemoteWrite {
		t.Fatalf("conflict plan should not allow remote write: %#v", plan)
	}
}

func TestSyncPlannerPullPlanRequiresApproval(t *testing.T) {
	plan, err := BuildPlan(Request{Direction: DirectionPull, Target: "cloud", BaseRevision: "rev_1", RemoteRevision: "rev_1"})
	if err != nil {
		t.Fatalf("build pull plan: %v", err)
	}
	if !plan.RequiresApproval || plan.RemoteWrite || len(plan.Operations) == 0 || plan.Operations[0].Kind != "download_manifest" {
		t.Fatalf("pull plan = %#v", plan)
	}
}
