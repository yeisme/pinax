package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func TestIdentityFirstTwoDeviceKernel(t *testing.T) {
	ctx := context.Background()
	t.Setenv("PINAX_SYNC_SECRET", "identity-first-e2e-secret")
	store := t.TempDir()
	deviceA := filepath.Join(t.TempDir(), "device-a")
	deviceB := filepath.Join(t.TempDir(), "device-b")
	svc := app.NewService()
	for _, root := range []string{deviceA, deviceB} {
		if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Identity Kernel"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: deviceA, Slug: "kernel", Name: "Kernel", NotesPrefix: "notes/projects/kernel"}); err != nil {
		t.Fatal(err)
	}
	targetProjection, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: deviceA, Title: "Target", Project: "kernel", Kind: "reference", Tags: []string{"kernel", "reference"}, Body: "target body"})
	if err != nil {
		t.Fatal(err)
	}
	targetID := targetProjection.Facts["note_id"]
	sourceProjection, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: deviceA, Title: "Source", Project: "kernel", Kind: "task", Tags: []string{"kernel", "task"}, Body: "[[Target]]\n\n- [ ] ship identity kernel ^ship-kernel"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := sourceProjection.Facts["note_id"]
	if targetID == "" || sourceID == "" || targetID == sourceID {
		t.Fatalf("object ids target=%q source=%q", targetID, sourceID)
	}
	if _, err := svc.TagNote(ctx, app.NoteTagRequest{VaultPath: deviceA, NoteRef: sourceID, Operation: "add", Tags: []string{"agent-managed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QueryRun(ctx, app.QueryRequest{VaultPath: deviceA, SQL: "SELECT object_id, title, path FROM notes", LazyIndex: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DataviewRun(ctx, app.DataviewRequest{VaultPath: deviceA, Query: "TABLE object_id, title, path FROM #kernel", LazyIndex: true}); err != nil {
		t.Fatal(err)
	}
	backlinks, err := svc.NoteBacklinks(ctx, app.NoteLinkRequest{VaultPath: deviceA, NoteRef: targetID})
	if err != nil || backlinks.Facts["backlinks"] == "0" {
		t.Fatalf("backlinks=%#v err=%v", backlinks, err)
	}

	loginA := app.CloudLoginRequest{VaultPath: deviceA, Endpoint: "file://" + store, WorkspaceID: "identity-kernel", DeviceID: "device-a", SecretRef: "test-secret"}
	loginB := app.CloudLoginRequest{VaultPath: deviceB, Endpoint: "file://" + store, WorkspaceID: "identity-kernel", DeviceID: "device-b", SecretRef: "test-secret"}
	if _, err := svc.CloudLogin(ctx, loginA); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, loginA.VaultPath, loginA.DeviceID)
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloudLogin(ctx, loginB); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, loginB.VaultPath, loginB.DeviceID)
	if _, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	assertNoteIdentity(t, ctx, svc, deviceB, targetID)
	assertNoteIdentity(t, ctx, svc, deviceB, sourceID)

	if _, err := svc.RenameNote(ctx, app.NoteMutationRequest{VaultPath: deviceA, NoteRef: targetID, Title: "Target Renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	renamed := assertNoteIdentity(t, ctx, svc, deviceB, targetID)
	if filepath.Base(renamed.Path) != "target-renamed.md" {
		t.Fatalf("renamed path = %q", renamed.Path)
	}

	conflictPlan, err := syncplan.BuildPlan(syncplan.Request{Direction: syncplan.DirectionPush, Yes: true,
		LocalManifest:  pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersionV2, Entries: []pinaxcloud.ManifestEntry{{ObjectID: targetID, ObjectKind: "note", Path: renamed.Path, RevisionID: "local-r2", DeviceID: "device-b"}}},
		BaseManifest:   pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersionV2, Entries: []pinaxcloud.ManifestEntry{{ObjectID: targetID, ObjectKind: "note", Path: renamed.Path, RevisionID: "base-r1", DeviceID: "device-a"}}},
		RemoteManifest: pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersionV2, Entries: []pinaxcloud.ManifestEntry{{ObjectID: targetID, ObjectKind: "note", Path: renamed.Path, RevisionID: "remote-r2", DeviceID: "device-a"}}},
	})
	if err != nil || len(conflictPlan.Operations) != 1 || conflictPlan.Operations[0].Kind != "revision_conflict" {
		t.Fatalf("conflict plan=%#v err=%v", conflictPlan, err)
	}

	deleted, err := svc.DeleteNote(ctx, app.NoteDeleteRequest{VaultPath: deviceA, NoteRef: sourceID, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if projection, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("delete pull: %v projection=%#v", err, projection)
	}
	if _, err := svc.ShowNote(ctx, app.ShowNoteRequest{VaultPath: deviceB, NoteRef: sourceID}); err == nil {
		t.Fatal("deleted object still exists on device B")
	}
	if _, err := svc.TrashRestore(ctx, app.TrashRequest{VaultPath: deviceA, ObjectRef: sourceID}); err != nil {
		t.Fatalf("restore %s: %v, delete=%#v", sourceID, err, deleted)
	}
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	assertNoteIdentity(t, ctx, svc, deviceB, sourceID)

	for _, root := range []string{deviceA, deviceB} {
		if _, err := os.Stat(filepath.Join(root, ".pinax", "records", "events.jsonl")); err != nil {
			t.Fatalf("ledger missing for %s: %v", root, err)
		}
	}
}

func promoteManifestV2(t *testing.T, ctx context.Context, svc *app.Service, root, deviceID string) {
	t.Helper()
	planProjection, err := svc.SyncManifestPlan(ctx, app.SyncManifestMigrationRequest{VaultPath: root, DeviceID: deviceID, Save: true})
	if err != nil {
		audit, _ := svc.SyncManifestAudit(ctx, app.SyncManifestMigrationRequest{VaultPath: root, DeviceID: deviceID})
		t.Fatalf("manifest plan: %v audit=%#v", err, audit.Data)
	}
	plan, ok := planProjection.Data.(map[string]any)["plan"].(app.SyncManifestMigrationPlan)
	if !ok {
		t.Fatalf("plan projection = %#v", planProjection.Data)
	}
	if _, err := svc.SyncManifestPromote(ctx, app.SyncManifestMigrationRequest{VaultPath: root, PlanID: plan.PlanID, RemoteCapability: "v2", Yes: true}); err != nil {
		t.Fatal(err)
	}
}

func assertNoteIdentity(t *testing.T, ctx context.Context, svc *app.Service, root, objectID string) appNote {
	t.Helper()
	note, err := svc.ShowNote(ctx, app.ShowNoteRequest{VaultPath: root, NoteRef: objectID})
	if err != nil {
		resolver, _ := svc.ResolveVaultObjectProjection(ctx, app.ResolverRequest{VaultPath: root, Query: objectID, Scope: "registered", Kind: "note"})
		var files []string
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() && filepath.Ext(path) == ".md" {
				rel, _ := filepath.Rel(root, path)
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		t.Fatalf("show object %s: %v resolver=%#v files=%#v", objectID, err, resolver.Data, files)
	}
	if note.ID != objectID {
		t.Fatalf("note = %#v", note)
	}
	return appNote{ID: note.ID, Path: note.Path}
}

type appNote struct {
	ID   string
	Path string
}
