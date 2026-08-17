package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

const manifestMigrationObjectID = "01982d84-2b48-7000-8000-000000000021"

func TestSyncManifestAuditIsReadOnlyAndBlocksMissingIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "legacy.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_legacy\ntitle: Legacy\n---\n\n# Legacy\n")
	svc := NewService()
	projection, err := svc.SyncManifestAudit(context.Background(), SyncManifestMigrationRequest{VaultPath: root, DeviceID: "device-a"})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Facts["eligible"] != "false" || projection.Facts["writes"] != "false" {
		t.Fatalf("facts = %#v", projection.Facts)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "cloud", "manifest-capability.json")); !os.IsNotExist(err) {
		t.Fatalf("audit wrote capability state: %v", err)
	}
}

func TestSyncManifestPromotionAndRollbackBeforeRemoteWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: "+manifestMigrationObjectID+"\ntitle: Alpha\n---\n\n# Alpha\n")
	svc := NewService()
	planProjection, err := svc.SyncManifestPlan(context.Background(), SyncManifestMigrationRequest{VaultPath: root, DeviceID: "device-a", Save: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := planProjection.Data.(map[string]any)["plan"].(SyncManifestMigrationPlan)
	if _, err := svc.SyncManifestPromote(context.Background(), SyncManifestMigrationRequest{VaultPath: root, PlanID: plan.PlanID, RemoteCapability: "v2", Yes: true}); err != nil {
		t.Fatal(err)
	}
	manifest, err := buildLocalCloudManifest(root, pinaxcloud.State{Config: pinaxcloud.Config{DeviceID: "device-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != pinaxcloud.ManifestSchemaVersionV2 || manifest.Entries[0].ObjectID != manifestMigrationObjectID {
		t.Fatalf("manifest = %#v", manifest)
	}
	if _, err := svc.SyncManifestRollback(context.Background(), SyncManifestMigrationRequest{VaultPath: root, Yes: true}); err != nil {
		t.Fatal(err)
	}
	manifest, err = buildLocalCloudManifest(root, pinaxcloud.State{})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != pinaxcloud.ManifestSchemaVersionV1 {
		t.Fatalf("schema version = %q", manifest.SchemaVersion)
	}
}

func TestSyncManifestRollbackRejectsAfterV2RemoteWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := SyncManifestCapabilityState{SchemaVersion: syncManifestCapabilitySchemaVersion, Status: "promoted", ManifestVersion: pinaxcloud.ManifestSchemaVersionV2, DeviceID: "device-a", FirstV2RemoteRevision: "rev-v2"}
	if err := writeSyncManifestCapabilityState(root, state); err != nil {
		t.Fatal(err)
	}
	_, err := NewService().SyncManifestRollback(context.Background(), SyncManifestMigrationRequest{VaultPath: root, Yes: true})
	if err == nil || err.Error() != "manifest_rollback_unsafe: manifest v2 was already written remotely" {
		t.Fatalf("rollback error = %v", err)
	}
}

func TestNegotiateSyncManifestCapabilityRejectsMixedAuthoritativeHeads(t *testing.T) {
	t.Parallel()
	v1 := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersionV1, Entries: []pinaxcloud.ManifestEntry{{Path: "notes/a.md"}}}
	v2 := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersionV2, Entries: []pinaxcloud.ManifestEntry{{ObjectID: manifestMigrationObjectID, Path: "notes/a.md"}}}
	if err := negotiateSyncManifestCapability(v2, v1); err == nil {
		t.Fatal("v2 local should reject non-empty v1 remote")
	}
	if err := negotiateSyncManifestCapability(v1, v2); err == nil {
		t.Fatal("v1 local should reject non-empty v2 remote")
	}
	if err := negotiateSyncManifestCapability(v2, pinaxcloud.Manifest{}); err != nil {
		t.Fatalf("empty remote should be promotable: %v", err)
	}
}

func TestSyncDaemonBlocksRemoteWriteWhileManifestPromotionPending(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: "+manifestMigrationObjectID+"\ntitle: Alpha\n---\n\n# Alpha\n")
	svc := NewService()
	if _, err := svc.SyncManifestPlan(context.Background(), SyncManifestMigrationRequest{VaultPath: root, DeviceID: "device-a", Save: true}); err != nil {
		t.Fatal(err)
	}
	projection, err := svc.SyncDaemonRun(context.Background(), SyncDaemonRequest{VaultPath: root, Target: "cloud", Yes: true, Once: true})
	if err == nil || projection.Facts["remote_write"] != "false" {
		t.Fatalf("projection = %#v, err = %v", projection, err)
	}
	if projection.Facts["last_error_code"] != "manifest_promotion_pending" {
		t.Fatalf("facts = %#v", projection.Facts)
	}
}
