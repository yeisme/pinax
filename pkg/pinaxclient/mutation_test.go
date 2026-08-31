package pinaxclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallMutationRPCAmbiguousTransportRecoversSucceededWithoutBlindRetry(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_ambiguous_reset", IdempotencyKey: "idem_ambiguous_reset"}
	request := mutationTestRequest(identity)
	var mu sync.Mutex
	rpcCount := 0
	statusCount := 0
	seen := make([]MutationIdentity, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			var wire RPCRequest
			_ = json.NewDecoder(r.Body).Decode(&wire)
			mu.Lock()
			rpcCount++
			seen = append(seen, mutationIdentityFromRequest(wire))
			mu.Unlock()
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
		case "/v1/operations/" + identity.OperationID:
			mu.Lock()
			statusCount++
			mu.Unlock()
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": mutationTestOperation(identity.OperationID, "succeeded", false, false)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	mu.Lock()
	defer mu.Unlock()
	if err != nil || projection.Facts["operation_id"] != identity.OperationID || projection.Facts["recovered_from_operation"] != "true" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if rpcCount != 1 || statusCount != 1 || len(seen) != 1 || seen[0] != identity {
		t.Fatalf("counts rpc=%d status=%d seen=%#v", rpcCount, statusCount, seen)
	}
}

func TestCallMutationRPC5xxReconcilesBeforeReturningWithoutMutationRetry(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_ambiguous_5xx", IdempotencyKey: "idem_ambiguous_5xx"}
	request := mutationTestRequest(identity)
	rpcCount := 0
	statusCount := 0
	reconcileCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			rpcCount++
			w.WriteHeader(http.StatusServiceUnavailable)
			writeTestErrorProjection(t, w, "inbox.capture", "owner_unavailable", "Owner response was interrupted", "Inspect operation")
		case "/v1/operations/" + identity.OperationID:
			statusCount++
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": mutationTestOperation(identity.OperationID, "applying", false, false)})
		case "/v1/operations/" + identity.OperationID + ":reconcile":
			reconcileCount++
			writeTestProjection(t, w, "operation.reconcile", map[string]any{"operation": mutationTestOperation(identity.OperationID, "succeeded", false, false)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	if err != nil || projection.Facts["operation_status"] != "succeeded" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if rpcCount != 1 || statusCount != 1 || reconcileCount != 1 {
		t.Fatalf("counts rpc=%d status=%d reconcile=%d", rpcCount, statusCount, reconcileCount)
	}
}

func TestCallMutationRPCTimeoutRecoversOperationWithoutSecondMutation(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_ambiguous_timeout", IdempotencyKey: "idem_ambiguous_timeout"}
	request := mutationTestRequest(identity)
	var rpcCount atomic.Int32
	var statusCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			rpcCount.Add(1)
			time.Sleep(30 * time.Millisecond)
			writeTestProjection(t, w, "inbox.capture", map[string]any{"operation_id": identity.OperationID})
		case "/v1/operations/" + identity.OperationID:
			statusCount.Add(1)
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": mutationTestOperation(identity.OperationID, "succeeded", false, false)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, HTTPClient: &http.Client{Timeout: 10 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	if err != nil || projection.Facts["operation_status"] != "succeeded" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if rpcCount.Load() != 1 || statusCount.Load() != 1 {
		t.Fatalf("counts rpc=%d status=%d", rpcCount.Load(), statusCount.Load())
	}
}

func TestCallMutationRPCNoBlindRetryWhenOperationIsNotReplaySafe(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_no_blind_retry", IdempotencyKey: "idem_no_blind_retry"}
	request := mutationTestRequest(identity)
	rpcCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			rpcCount++
			_, _ = w.Write([]byte("{"))
		case "/v1/operations/" + identity.OperationID:
			operation := mutationTestOperation(identity.OperationID, "failed", true, false)
			operation.Error = &CommandError{Code: "apply_failed", Message: "Apply outcome is not replay safe"}
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": operation})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	if err == nil || projection.Error == nil || projection.Error.Code != "apply_failed" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if rpcCount != 1 {
		t.Fatalf("unsafe operation was blindly retried: rpc=%d", rpcCount)
	}
}

func TestCallMutationRPCNoBlindRetryWhenOperationIsNotVisible(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_not_visible", IdempotencyKey: "idem_not_visible"}
	request := mutationTestRequest(identity)
	rpcCount := 0
	statusCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			rpcCount++
			_, _ = w.Write([]byte("{"))
		case "/v1/operations/" + identity.OperationID:
			statusCount++
			w.WriteHeader(http.StatusNotFound)
			writeTestErrorProjection(t, w, "operation.show", "operation_not_found", "Operation was not found", "Check retained identity")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CallMutationRPC(context.Background(), request, identity)
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeMutationOutcomeUnknown || clientErr.OperationRef != identity.OperationID || !clientErr.ReconcileRequired {
		t.Fatalf("error=%#v", err)
	}
	if rpcCount != 1 || statusCount != 1 {
		t.Fatalf("missing operation caused blind retry: rpc=%d status=%d", rpcCount, statusCount)
	}
}

func TestCallMutationRPCReplaySafeRetryReusesExactBindingOnce(t *testing.T) {
	identity := MutationIdentity{OperationID: "op_replay_safe_retry", IdempotencyKey: "idem_replay_safe_retry"}
	request := mutationTestRequest(identity)
	rpcCount := 0
	seen := make([]MutationIdentity, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/rpc":
			rpcCount++
			var wire RPCRequest
			_ = json.NewDecoder(r.Body).Decode(&wire)
			seen = append(seen, mutationIdentityFromRequest(wire))
			if rpcCount == 1 {
				_, _ = w.Write([]byte("{"))
				return
			}
			writeTestProjection(t, w, "inbox.capture", map[string]any{"operation_id": identity.OperationID})
		case "/v1/operations/" + identity.OperationID:
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": mutationTestOperation(identity.OperationID, "failed", true, true)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.CallMutationRPC(context.Background(), request, identity)
	if err != nil || projection.Command != "inbox.capture" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if rpcCount != 2 || len(seen) != 2 || seen[0] != identity || seen[1] != identity {
		t.Fatalf("replay changed binding or count: rpc=%d seen=%#v", rpcCount, seen)
	}
}

func mutationTestRequest(identity MutationIdentity) RPCRequest {
	return RPCRequest{Method: "Pinax.Inbox.Capture", Params: map[string]any{
		"title": "recovery", "yes": true,
		"operation_id": identity.OperationID, "idempotency_key": identity.IdempotencyKey,
	}}
}

func mutationIdentityFromRequest(request RPCRequest) MutationIdentity {
	operationID, _ := request.Params["operation_id"].(string)
	idempotencyKey, _ := request.Params["idempotency_key"].(string)
	return MutationIdentity{OperationID: operationID, IdempotencyKey: idempotencyKey}
}

func mutationTestOperation(operationID, status string, retryable, replaySafe bool) Operation {
	return Operation{
		SchemaVersion: OperationSchemaV1, OperationID: operationID,
		CapabilityID: "inbox.capture", BindingID: "rpc.inbox.capture",
		Status: status, Retryable: retryable, ReplaySafe: replaySafe,
		ReconcileRequired: status == "applying" || status == "reconcile_required",
		Result:            json.RawMessage(`{"path":"inbox/recovered.md","note_id":"note-recovered"}`),
		ReceiptRef:        "record:event-recovered", ResourceRef: "pinax://note/note-recovered",
		CreatedAt: "2026-08-25T00:00:00Z", UpdatedAt: "2026-08-25T00:00:01Z", AcceptedAt: "2026-08-25T00:00:00Z",
	}
}
