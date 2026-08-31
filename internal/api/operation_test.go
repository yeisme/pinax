package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
)

func TestOperationShowReconcileRESTAndRPCScopeParity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	server := NewServerWithOptions(app.NewService(), root, ServerOptions{AuthMode: AuthModeTemp})
	records, err := server.tokenStore.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("temp token records = %#v err=%v", records, err)
	}
	operationID := "op-api-parity-1"
	seedAPIOperation(t, ctx, root, operationID,
		operation.IdentityDigest("api-principal", records[0].ID),
		operation.IdentityDigest("pinax-vault", filepath.Clean(root)),
	)

	show := httptest.NewRecorder()
	showRequest := httptest.NewRequest(http.MethodGet, "/v1/operations/"+operationID, nil)
	showRequest.Header.Set("Authorization", "Bearer "+server.tempSecret)
	server.Handler().ServeHTTP(show, showRequest)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), `"command":"operation.show"`) || !strings.Contains(show.Body.String(), `"operation_id":"`+operationID+`"`) {
		t.Fatalf("REST operation show: status=%d body=%s", show.Code, show.Body.String())
	}

	rpc := httptest.NewRecorder()
	rpcRequest := httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"id":"op-rpc","method":"Pinax.Operation.Get","params":{"operation_id":"`+operationID+`"}}`))
	rpcRequest.Header.Set("Authorization", "Bearer "+server.tempSecret)
	server.Handler().ServeHTTP(rpc, rpcRequest)
	if rpc.Code != http.StatusOK || !strings.Contains(rpc.Body.String(), `"command":"operation.show"`) || !strings.Contains(rpc.Body.String(), `"operation_id":"`+operationID+`"`) {
		t.Fatalf("RPC operation show: status=%d body=%s", rpc.Code, rpc.Body.String())
	}
	var restPayload, rpcPayload struct {
		Data struct {
			Operation operation.View `json:"operation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(show.Body.Bytes(), &restPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rpc.Body.Bytes(), &rpcPayload); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restPayload.Data.Operation, rpcPayload.Data.Operation) {
		t.Fatalf("REST/RPC operation mismatch: REST=%#v RPC=%#v", restPayload.Data.Operation, rpcPayload.Data.Operation)
	}

	reconcile := httptest.NewRecorder()
	reconcileRequest := httptest.NewRequest(http.MethodPost, "/v1/operations/"+operationID+":reconcile", nil)
	reconcileRequest.Header.Set("Authorization", "Bearer "+server.tempSecret)
	server.Handler().ServeHTTP(reconcile, reconcileRequest)
	if reconcile.Code != http.StatusOK || !strings.Contains(reconcile.Body.String(), `"command":"operation.reconcile"`) || !strings.Contains(reconcile.Body.String(), `"operation_status":"reconcile_required"`) {
		t.Fatalf("REST operation reconcile: status=%d body=%s", reconcile.Code, reconcile.Body.String())
	}
}

func TestOperationScopeMismatchReturnsSameNotFoundShape(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	server := NewServerWithOptions(app.NewService(), root, ServerOptions{AuthMode: AuthModeTemp})
	records, err := server.tokenStore.List()
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	operationID := "op-api-scope-1"
	seedAPIOperation(t, ctx, root, operationID,
		operation.IdentityDigest("api-principal", records[0].ID),
		operation.IdentityDigest("pinax-vault", filepath.Clean(root)),
	)
	otherRecord, otherSecret := GenerateTokenRecord("other", map[TokenScope]ScopeTarget{ScopeRead: {}}, "", "test")
	if err := server.tokenStore.Create(otherRecord); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{operationID, "op-does-not-exist"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/operations/"+id, nil)
		request.Header.Set("Authorization", "Bearer "+otherSecret)
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"operation_not_found"`) {
			t.Fatalf("operation %s mismatch shape: status=%d body=%s", id, response.Code, response.Body.String())
		}
	}
}

func seedAPIOperation(t *testing.T, ctx context.Context, root, operationID, principalDigest, scopeDigest string) {
	t.Helper()
	digest, err := operation.CanonicalDigest(map[string]any{"title": "API operation fixture"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	_, err = store.CreateAccepted(ctx, operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "inbox.capture", BindingID: "rest.inbox.capture",
		PrincipalDigest: principalDigest, ScopeDigest: scopeDigest, RequestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplying(ctx, operationID); err != nil {
		t.Fatal(err)
	}
}
