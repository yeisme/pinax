package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
)

func TestFolderRenameRESTRevisionIdempotencyAndDryRun(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := app.NewService()
	createFolderRenameAPIFixture(t, ctx, service, root)
	server := NewServerWithOptions(service, root, ServerOptions{AllowWrite: true})
	handler := server.Handler()

	preview := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, "/v1/folders/spaces/source:rename?target_path=spaces/target&dry_run=true", nil)
	handler.ServeHTTP(preview, previewRequest)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"operation_status":"planned"`) {
		t.Fatalf("preview: status=%d body=%s", preview.Code, preview.Body.String())
	}
	var previewProjection struct {
		Facts map[string]string `json:"facts"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewProjection); err != nil {
		t.Fatal(err)
	}
	revision := previewProjection.Facts["revision_before"]
	if revision == "" {
		t.Fatalf("preview revision missing: %s", preview.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("dry run created operation ledger: %v", err)
	}

	operationID := "op-rest-folder-idempotent-1"
	for index := 0; index < 2; index++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/folders/spaces/source:rename?target_path=spaces/target&expected_revision="+revision+"&yes=true", nil)
		request.Header.Set("X-Pinax-Operation-ID", operationID)
		request.Header.Set("Idempotency-Key", "idem-"+operationID)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"operation_status":"succeeded"`) {
			t.Fatalf("rename %d: status=%d body=%s", index, response.Code, response.Body.String())
		}
		if index == 1 && !strings.Contains(response.Body.String(), `"idempotent_replay":"true"`) {
			t.Fatalf("replay = %s", response.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "source")); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "target")); err != nil {
		t.Fatalf("target missing: %v", err)
	}
}

func TestFolderRenameRPCRevisionConflictAndIdempotency(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := app.NewService()
	createFolderRenameAPIFixture(t, ctx, service, root)
	dispatcher := NewRPCDispatcherWithOptions(service, root, DispatcherOptions{AllowWrite: true})
	preview, err := dispatcher.Call(ctx, RPCRequest{Method: "Pinax.Folder.Rename", Params: map[string]any{"path": "spaces/source", "target_path": "spaces/target", "dry_run": true}})
	if err != nil || preview.Facts["revision_before"] == "" {
		t.Fatalf("preview = %#v err=%v", preview, err)
	}
	if err := os.WriteFile(filepath.Join(root, "spaces", "source", "changed.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VersionSnapshot(ctx, app.SnapshotRequest{VaultPath: root, Message: "fresh snapshot after conflicting change"}); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{
		"path": "spaces/source", "target_path": "spaces/target", "yes": true,
		"expected_revision": preview.Facts["revision_before"],
		"operation_id":      "op-rpc-folder-stale-1", "idempotency_key": "idem-op-rpc-folder-stale-1",
	}
	projection, err := dispatcher.Call(ctx, RPCRequest{Method: "Pinax.Folder.Rename", Params: params})
	if err == nil || projection.Error == nil || projection.Error.Code != "revision_conflict" {
		t.Fatalf("stale RPC rename = %#v err=%v", projection, err)
	}
	if _, err := os.Stat(filepath.Join(root, "spaces", "source")); err != nil {
		t.Fatalf("stale RPC changed source: %v", err)
	}
}

func TestFolderRenameRESTReconcileAfterFinalizeCrash(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := app.NewService()
	createFolderRenameAPIFixture(t, ctx, service, root)
	preview, err := service.RenameFolder(ctx, app.FolderOperationRequest{VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", DryRun: true, RequireSnapshot: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared := map[string]string{
		"source_path": "spaces/source", "target_path": "spaces/target",
		"snapshot_id": preview.Facts["snapshot_id"], "revision_before": preview.Facts["revision_before"],
	}
	preparedJSON, _ := json.Marshal(prepared)
	operationID := "op-rest-folder-reconcile-1"
	digest, _ := operation.CanonicalDigest(map[string]any{"source_path": "spaces/source", "target_path": "spaces/target"})
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "folder.rename", BindingID: "rest.folder.rename", RequestDigest: digest,
		PrincipalDigest: operation.IdentityDigest("api-principal", "auth-unset"),
		ScopeDigest:     operation.IdentityDigest("pinax-vault", filepath.Clean(root)), RevisionBefore: preview.Facts["revision_before"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplyingWithOutcome(ctx, operationID, operation.Outcome{Result: preparedJSON}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if _, err := service.RenameFolder(ctx, app.FolderOperationRequest{
		VaultPath: root, Path: "spaces/source", TargetPath: "spaces/target", Yes: true, RequireSnapshot: true,
		ExpectedRevision: preview.Facts["revision_before"],
	}); err != nil {
		t.Fatal(err)
	}

	server := NewServerWithOptions(service, root, ServerOptions{AllowWrite: true})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/operations/"+operationID+":reconcile", nil)
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"operation_status":"succeeded"`) || !strings.Contains(response.Body.String(), `"revision_after":"sha256:`) {
		t.Fatalf("reconcile: status=%d body=%s", response.Code, response.Body.String())
	}
}

func createFolderRenameAPIFixture(t *testing.T, ctx context.Context, service *app.Service, root string) {
	t.Helper()
	if _, err := service.CreateFolder(ctx, app.FolderOperationRequest{VaultPath: root, Path: "spaces/source", Purpose: "notes"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spaces", "source", "note.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VersionSnapshot(ctx, app.SnapshotRequest{VaultPath: root, Message: "before folder rename"}); err != nil {
		t.Fatal(err)
	}
}
