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

func TestInboxCaptureRESTIdempotentReplayAndDryRun(t *testing.T) {
	root := t.TempDir()
	server := NewServerWithOptions(app.NewService(), root, ServerOptions{AllowWrite: true})
	handler := server.Handler()
	operationID := "op-rest-inbox-idempotent-1"

	for index := 0; index < 2; index++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/inbox:capture?title=REST+capture&body=written+once&yes=true", nil)
		request.Header.Set("X-Pinax-Operation-ID", operationID)
		request.Header.Set("Idempotency-Key", "idem-"+operationID)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"operation_status":"succeeded"`) {
			t.Fatalf("capture %d: status=%d body=%s", index, response.Code, response.Body.String())
		}
		if index == 1 && !strings.Contains(response.Body.String(), `"idempotent_replay":"true"`) {
			t.Fatalf("replay response = %s", response.Body.String())
		}
	}
	files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	if len(files) != 1 {
		t.Fatalf("REST replay created duplicate notes: %#v", files)
	}

	dryRoot := t.TempDir()
	dryServer := NewServerWithOptions(app.NewService(), dryRoot, ServerOptions{AllowWrite: true})
	dry := httptest.NewRecorder()
	dryRequest := httptest.NewRequest(http.MethodPost, "/v1/inbox:capture?title=Preview&dry_run=true", nil)
	dryServer.Handler().ServeHTTP(dry, dryRequest)
	if dry.Code != http.StatusOK || !strings.Contains(dry.Body.String(), `"operation_status":"planned"`) {
		t.Fatalf("dry run: status=%d body=%s", dry.Code, dry.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dryRoot, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("REST dry run created ledger: %v", err)
	}
}

func TestInboxCaptureRPCIdempotentReplay(t *testing.T) {
	root := t.TempDir()
	dispatcher := NewRPCDispatcherWithOptions(app.NewService(), root, DispatcherOptions{AllowWrite: true})
	params := map[string]any{
		"title": "RPC capture", "body": "written once", "yes": true,
		"operation_id": "op-rpc-inbox-idempotent-1", "idempotency_key": "idem-op-rpc-inbox-idempotent-1",
	}
	first, err := dispatcher.Call(context.Background(), RPCRequest{Method: "Pinax.Inbox.Capture", Params: params})
	if err != nil || first.Facts["operation_status"] != "succeeded" {
		t.Fatalf("first RPC capture = %#v err=%v", first, err)
	}
	second, err := dispatcher.Call(context.Background(), RPCRequest{Method: "Pinax.Inbox.Capture", Params: params})
	if err != nil || second.Facts["idempotent_replay"] != "true" {
		t.Fatalf("RPC replay = %#v err=%v", second, err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	if len(files) != 1 {
		t.Fatalf("RPC replay created duplicate notes: %#v", files)
	}
}

func TestInboxCaptureRESTReconcileAfterResponseLoss(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	service := app.NewService()
	server := NewServerWithOptions(service, root, ServerOptions{AllowWrite: true})
	operationID := "op-rest-inbox-reconcile-1"
	planned, err := service.InboxCapture(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Response lost", Body: "written once", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared := map[string]string{
		"path": planned.Facts["path"], "planned_path": planned.Facts["planned_path"], "note_id": planned.Facts["note_id"],
	}
	preparedJSON, _ := json.Marshal(prepared)
	digest, _ := operation.CanonicalDigest(map[string]any{"title": "Response lost"})
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "inbox.capture", BindingID: "rest.inbox.capture", RequestDigest: digest,
		PrincipalDigest: operation.IdentityDigest("api-principal", "auth-unset"),
		ScopeDigest:     operation.IdentityDigest("pinax-vault", filepath.Clean(root)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplyingWithOutcome(ctx, operationID, operation.Outcome{Result: preparedJSON}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	_, err = service.InboxCapture(ctx, app.CreateNoteRequest{
		VaultPath: root, Title: "Response lost", Body: "written once",
		PlannedPath: planned.Facts["planned_path"], ObjectID: planned.Facts["note_id"],
		RecordIdempotencyKey: "operation:" + operationID, RecordEvidence: []string{"operation_id=" + operationID},
	})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/operations/"+operationID+":reconcile", nil)
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"operation_status":"succeeded"`) || !strings.Contains(response.Body.String(), `"receipt_ref":"record:`) {
		t.Fatalf("reconcile: status=%d body=%s", response.Code, response.Body.String())
	}
	files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	if len(files) != 1 {
		t.Fatalf("reconcile replayed capture: %#v", files)
	}
}
