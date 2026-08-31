package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestFolderRenameRemoteIdempotentReplayRenamesOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	createFolderRenameFixture(t, ctx, service, root, "spaces/source")
	preview, err := service.RenameFolder(ctx, FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true})
	if err != nil || preview.Facts["revision_before"] == "" || preview.Facts["snapshot_id"] == "" {
		t.Fatalf("preview = %#v err=%v", preview, err)
	}
	identity := testInboxOperationIdentity(root, "op-folder-idempotent-1", "rpc.folder.rename")
	request := FolderOperationRequest{
		VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", Yes: true, RequireSnapshot: true,
		ExpectedRevision: preview.Facts["revision_before"],
	}
	first, err := service.RenameFolderRemote(ctx, request, identity)
	if err != nil || first.Facts["operation_status"] != "succeeded" || first.Facts["revision_after"] == "" {
		t.Fatalf("first rename = %#v err=%v", first, err)
	}
	second, err := service.RenameFolderRemote(ctx, request, identity)
	if err != nil || second.Facts["idempotent_replay"] != "true" || second.Facts["operation_id"] != identity.OperationID {
		t.Fatalf("replay rename = %#v err=%v", second, err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "source")); !os.IsNotExist(err) {
		t.Fatalf("source still exists after rename: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "spaces", "target")); err != nil || !info.IsDir() {
		t.Fatalf("target missing after rename: info=%v err=%v", info, err)
	}
}

func TestFolderRenameRemoteRevisionConflictWritesNothing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	createFolderRenameFixture(t, ctx, service, root, "spaces/source")
	preview, err := service.RenameFolder(ctx, FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spaces", "source", "changed.md"), []byte("changed after preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "fresh snapshot after conflicting change"}); err != nil {
		t.Fatal(err)
	}
	identity := testInboxOperationIdentity(root, "op-folder-stale-1", "rpc.folder.rename")
	projection, err := service.RenameFolderRemote(ctx, FolderOperationRequest{
		VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", Yes: true, RequireSnapshot: true,
		ExpectedRevision: preview.Facts["revision_before"],
	}, identity)
	if err == nil || projection.Error == nil || projection.Error.Code != "revision_conflict" {
		t.Fatalf("stale rename = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "source")); err != nil {
		t.Fatalf("stale rename changed source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "target")); !os.IsNotExist(err) {
		t.Fatalf("stale rename created target: %v", err)
	}
}

func TestFolderRenameRemoteRequiresFreshSnapshotBeforeOpeningLedger(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	createFolderRenameFixture(t, ctx, service, root, "spaces/source")
	preview, err := service.RenameFolder(ctx, FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spaces", "source", "changed.md"), []byte("changed after snapshot"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := testInboxOperationIdentity(root, "op-folder-snapshot-stale-1", "rpc.folder.rename")
	projection, err := service.RenameFolderRemote(ctx, FolderOperationRequest{
		VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", Yes: true, RequireSnapshot: true,
		ExpectedRevision: preview.Facts["revision_before"],
	}, identity)
	if err == nil || projection.Error == nil || projection.Error.Code != "snapshot_required" {
		t.Fatalf("stale snapshot rename = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("stale snapshot opened operation ledger: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "source")); err != nil {
		t.Fatalf("stale snapshot changed source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "target")); !os.IsNotExist(err) {
		t.Fatalf("stale snapshot created target: %v", err)
	}
}

func TestFolderRenameOperationReconcileUsesFilesystemAndRegistryProof(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	createFolderRenameFixture(t, ctx, service, root, "spaces/source")
	preview, err := service.RenameFolder(ctx, FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared := folderResultFromProjection(preview)
	preparedJSON, _ := json.Marshal(prepared)
	identity := testInboxOperationIdentity(root, "op-folder-reconcile-1", "rpc.folder.rename")
	digest, _ := operation.CanonicalDigest(map[string]any{
		"capability_id": folderRenameCapabilityID, "binding_id": identity.BindingID,
		"source_path": prepared.SourcePath, "target_path": prepared.TargetPath,
		"expected_revision": prepared.RevisionBefore,
	})
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: identity.OperationID, IdempotencyKey: identity.IdempotencyKey,
		CapabilityID: folderRenameCapabilityID, BindingID: identity.BindingID,
		PrincipalDigest: identity.Access.PrincipalDigest, ScopeDigest: identity.Access.ScopeDigest,
		RequestDigest: digest, RevisionBefore: prepared.RevisionBefore,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplyingWithOutcome(ctx, identity.OperationID, operation.Outcome{Result: preparedJSON}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	if _, err := service.RenameFolder(ctx, FolderOperationRequest{
		VaultPath: root, Path: prepared.SourcePath, TargetPath: prepared.TargetPath,
		Yes: true, RequireSnapshot: true, ExpectedRevision: prepared.RevisionBefore,
	}); err != nil {
		t.Fatal(err)
	}
	reconciled, err := service.OperationReconcile(ctx, OperationRequest{VaultPath: root, OperationID: identity.OperationID, Access: OperationAccess{OwnerLocal: true}})
	if err != nil || reconciled.Facts["operation_status"] != "succeeded" || reconciled.Facts["revision_after"] == "" || reconciled.Facts["receipt_ref"] == "" {
		t.Fatalf("reconcile = %#v err=%v", reconciled, err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "target")); err != nil {
		t.Fatalf("reconcile lost target: %v", err)
	}
}

func TestFolderRenameRemoteDryRunCreatesNoOperationLedger(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := NewService()
	createFolderRenameFixture(t, ctx, service, root, "spaces/source")
	identity := testInboxOperationIdentity(root, "op-folder-dry-1", "rpc.folder.rename")
	projection, err := service.RenameFolderRemote(ctx, FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true}, identity)
	if err != nil || projection.Facts["operation_status"] != "planned" || projection.Facts["revision_before"] == "" {
		t.Fatalf("dry run = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("dry run created operation ledger: %v", err)
	}
}

func createFolderRenameFixture(t *testing.T, ctx context.Context, service *Service, root, folder string) {
	t.Helper()
	if _, err := service.CreateFolder(ctx, FolderOperationRequest{VaultPath: root, Path: folder, Purpose: "notes"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(folder), "note.md")
	if err := os.WriteFile(path, []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "before folder rename"}); err != nil {
		t.Fatal(err)
	}
}
