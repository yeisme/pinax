package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func syncOperationFixture(t *testing.T) (string, *Service) {
	t.Helper()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(t.Context(), InitVaultRequest{VaultPath: root, Title: "Sync fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateNote(t.Context(), CreateNoteRequest{VaultPath: root, Title: "Sync note", Body: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloudLogin(t.Context(), CloudLoginRequest{VaultPath: root, Endpoint: "file://" + t.TempDir(), WorkspaceID: "fixture", DeviceID: "fixture", SecretRef: "fixture", EncryptionSecretRef: "plain:fixture"}); err != nil {
		t.Fatal(err)
	}
	return root, svc
}

func TestSyncOperationReplayAndIdentityConflict(t *testing.T) {
	root, svc := syncOperationFixture(t)
	req := SyncRequest{VaultPath: root, Target: "capsa", Yes: true, Operation: RemoteOperationIdentity{OperationID: "sync-test", IdempotencyKey: "sync-test"}}
	first, err := svc.SyncPush(t.Context(), req)
	if err != nil || first.Facts["operation_status"] != "succeeded" {
		t.Fatalf("push failed: %v, %v", err, first.Facts)
	}
	second, err := NewService().SyncPush(t.Context(), req)
	if err != nil || second.Facts["idempotent_replay"] != "true" || second.Facts["run_id"] != first.Facts["run_id"] {
		t.Fatalf("replay failed: %v, %v", err, second.Facts)
	}
	req.RemoteRevision = "changed"
	if _, err := svc.SyncPush(t.Context(), req); domain.ErrorCode(err) != "idempotency_conflict" {
		t.Fatalf("changed request: %v", err)
	}
}

func TestSyncOperationDryRunAndPartialIdentity(t *testing.T) {
	root, svc := syncOperationFixture(t)
	req := SyncRequest{VaultPath: root, Target: "capsa", Yes: true, DryRun: true, Operation: RemoteOperationIdentity{OperationID: "partial"}}
	if _, err := svc.SyncPush(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax/api/operations.sqlite")); !os.IsNotExist(err) {
		t.Fatal("dry run created operation ledger")
	}
	req.DryRun = false
	if _, err := svc.SyncPush(t.Context(), req); domain.ErrorCode(err) != "operation_identity_required" {
		t.Fatalf("partial identity: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax/api/operations.sqlite")); !os.IsNotExist(err) {
		t.Fatal("invalid identity created operation ledger")
	}
}

// A separate OS process dies after the domain receipt and before ledger completion.
// The parent recovers from disk, never by executing the mutation again.
func TestSyncOperationCrashRecovery(t *testing.T) {
	for _, command := range []string{"sync.push", "sync.pull", "sync.conflicts.resolve"} {
		t.Run(command, func(t *testing.T) {
			root, svc := syncOperationFixture(t)
			if command == "sync.pull" {
				if _, err := svc.SyncPush(t.Context(), SyncRequest{VaultPath: root, Target: "capsa", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			if command == "sync.conflicts.resolve" {
				if err := os.WriteFile(filepath.Join(root, "sample.md"), []byte("remote"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "sample.20261023.conflict.md"), []byte("local"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSyncOperationCrashHelper$")
			child.Env = append(os.Environ(), "PINAX_SYNC_CRASH_ROOT="+root, "PINAX_SYNC_CRASH_COMMAND="+command)
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("crash helper failed: %v, %s", err, output)
			}
			req := OperationRequest{VaultPath: root, OperationID: "crash-operation", Access: OperationAccess{OwnerLocal: true}}
			before, err := svc.OperationShow(t.Context(), req)
			if err != nil || before.Facts["operation_status"] != "applying" {
				t.Fatalf("expected crash window: %v, %v", err, before.Facts)
			}
			after, err := NewService().OperationReconcile(t.Context(), req)
			if err != nil || after.Facts["operation_status"] != "succeeded" {
				t.Fatalf("recovery failed: %v, %v", err, after.Facts)
			}
			denied := req
			denied.Access = OperationAccess{PrincipalDigest: "other", ScopeDigest: "other"}
			if _, err := svc.OperationShow(t.Context(), denied); domain.ErrorCode(err) != "operation_not_found" {
				t.Fatalf("scope check: %v", err)
			}
		})
	}
}

func TestSyncOperationCrashHelper(t *testing.T) {
	root := os.Getenv("PINAX_SYNC_CRASH_ROOT")
	if root == "" {
		return
	}
	command := os.Getenv("PINAX_SYNC_CRASH_COMMAND")
	svc := NewService()
	_, err := svc.syncOperationLocked(context.Background(), root, command, RemoteOperationIdentity{OperationID: "crash-operation", IdempotencyKey: "crash-operation"}, map[string]string{"command": command}, func(id string) (domain.Projection, error) {
		var err error
		if command == "sync.conflicts.resolve" {
			_, err = svc.syncConflictsResolveLocked(SyncConflictResolveRequest{VaultPath: root, File: "sample.20261023.conflict.md", KeepLocal: true, Yes: true, operationID: id})
		} else {
			direction := syncplan.DirectionPush
			if command == "sync.pull" {
				direction = syncplan.DirectionPull
			}
			_, err = buildCloudSyncProjection(context.Background(), command, root, SyncRequest{VaultPath: root, Target: "capsa", Yes: true, operationID: id}, direction)
		}
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
		return domain.Projection{}, nil
	})
	t.Fatal("helper did not exit at crash window", err)
}
