package pinaxclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientReadsManifestReadinessOperationAndRPC(t *testing.T) {
	t.Parallel()

	const operationID = "op_01JTEST"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/manifest":
			writeTestProjection(t, w, "api.manifest", map[string]any{"manifest": Manifest{SchemaVersion: TransportManifestSchemaV1, Digest: "sha256:" + strings.Repeat("a", 64), Capabilities: []ManifestCapability{{ID: "note.read", Command: "note.show", RequestSchema: "pinax.note.read.request.v1", ResponseSchema: "pinax.projection.v1", Stability: "legacy"}}}})
		case "/v1/readiness":
			layers := map[string]Readiness{}
			for _, name := range ReadinessLayerNames() {
				layers[name] = Readiness{Status: "not_applicable", Maturity: "exploratory"}
			}
			layers["contract"] = Readiness{Status: "ready", Maturity: "first-support"}
			writeTestProjection(t, w, "connection.readiness", map[string]any{"readiness": ConnectionReadiness{SchemaVersion: ConnectionReadinessSchemaV1, Overall: Readiness{Status: "degraded", Maturity: "exploratory"}, Layers: layers}})
		case "/v1/operations/" + operationID:
			writeTestProjection(t, w, "operation.show", map[string]any{"operation": Operation{SchemaVersion: OperationSchemaV1, OperationID: operationID, CapabilityID: "inbox.capture", BindingID: "rest.inbox.capture", Status: "succeeded", ReplaySafe: true, CreatedAt: "2026-08-25T00:00:00Z", UpdatedAt: "2026-08-25T00:00:01Z", AcceptedAt: "2026-08-25T00:00:00Z"}})
		case "/v1/rpc":
			if r.Method != http.MethodPost {
				t.Fatalf("RPC method = %s", r.Method)
			}
			var request RPCRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Method != "Pinax.Note.Read" || request.Params["ref"] != "note-1" {
				t.Fatalf("RPC request = %#v", request)
			}
			writeTestProjection(t, w, "note.show", map[string]any{"note_id": "note-1"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := client.Manifest(context.Background())
	if err != nil || manifest.SchemaVersion != TransportManifestSchemaV1 || len(manifest.Capabilities) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	readiness, err := client.Readiness(context.Background())
	if err != nil || readiness.Layers["contract"].Status != "ready" {
		t.Fatalf("readiness=%#v err=%v", readiness, err)
	}
	operation, err := client.Operation(context.Background(), operationID)
	if err != nil || operation.OperationID != operationID || operation.CapabilityID != "inbox.capture" {
		t.Fatalf("operation=%#v err=%v", operation, err)
	}
	projection, err := client.CallRPC(context.Background(), RPCRequest{Method: "Pinax.Note.Read", Params: map[string]any{"ref": "note-1"}})
	if err != nil || projection.Command != "note.show" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
}

func TestNewMutationIdentityReturnsIndependentOpaquePair(t *testing.T) {
	first, err := NewMutationIdentity()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewMutationIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if !operationIDPattern.MatchString(first.OperationID) || !operationIDPattern.MatchString(first.IdempotencyKey) {
		t.Fatalf("invalid mutation identity = %#v", first)
	}
	if first.OperationID == first.IdempotencyKey || first == second {
		t.Fatalf("mutation identities are not independent: first=%#v second=%#v", first, second)
	}
}

func TestClientReconcileOperationUsesSameIdentityAndValidatesRecoveryFacts(t *testing.T) {
	t.Parallel()

	const operationID = "op_reconcile_client"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/operations/"+operationID+":reconcile" {
			t.Fatalf("reconcile request = %s %s", r.Method, r.URL.Path)
		}
		writeTestProjection(t, w, "operation.reconcile", map[string]any{"operation": Operation{
			SchemaVersion: OperationSchemaV1, OperationID: operationID,
			CapabilityID: "folder.rename", BindingID: "rest.folder.rename",
			Status: "reconcile_required", ReconcileRequired: true,
			RequiredAction: "inspect_and_reconcile", CreatedAt: "2026-08-25T00:00:00Z",
			UpdatedAt: "2026-08-25T00:00:01Z", AcceptedAt: "2026-08-25T00:00:00Z",
		}})
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := client.ReconcileOperation(context.Background(), operationID)
	if err != nil || operation.OperationID != operationID || !operation.ReconcileRequired {
		t.Fatalf("reconcile operation = %#v err=%v", operation, err)
	}
}

func TestReadinessUnknownStatusIsPreservedAsNonReady(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		layers := map[string]Readiness{}
		for _, name := range ReadinessLayerNames() {
			layers[name] = Readiness{Status: "ready", Maturity: "first-support"}
		}
		layers["production"] = Readiness{Status: "future_extension", Maturity: "exploratory"}
		writeTestProjection(t, w, "connection.readiness", map[string]any{"readiness": ConnectionReadiness{SchemaVersion: ConnectionReadinessSchemaV1, Overall: Readiness{Status: "future_overall", Maturity: "exploratory"}, Layers: layers}})
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := client.Readiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Overall.Status != "future_overall" || readiness.Overall.StatusKnown() || readiness.Overall.IsReady() || readiness.Layers["production"].Status != "future_extension" || readiness.Layers["production"].IsReady() {
		t.Fatalf("unknown readiness was not preserved fail-safe: %#v", readiness)
	}
}

func TestClientReturnsTypedErrorWithoutLeakingPayload(t *testing.T) {
	t.Parallel()

	const secret = "pinax-secret-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req-123")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeTestErrorProjection(t, w, "note.show", "owner_unavailable", "Owner is unavailable", "Retry later")
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CallRPC(context.Background(), RPCRequest{Method: "Pinax.Note.Read", Params: map[string]any{"secret": secret}})
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != "owner_unavailable" || !clientErr.Retryable || clientErr.RequestID != "req-123" || clientErr.RequiredAction != "Retry later" {
		t.Fatalf("error = %#v", err)
	}
	encoded, marshalErr := json.Marshal(clientErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(clientErr.Error(), secret) || strings.Contains(clientErr.Error(), "Authorization") {
		t.Fatalf("typed error leaked request data: %s %s", encoded, clientErr)
	}
}

func TestClientRejectsMalformedOperationIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestProjection(t, w, "operation.show", map[string]any{"operation": Operation{SchemaVersion: OperationSchemaV1, OperationID: "different", CapabilityID: "inbox.capture", Status: "succeeded"}})
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Operation(context.Background(), "op_expected")
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeUpstreamInvalidResponse || clientErr.OperationRef != "op_expected" || !clientErr.ReconcileRequired {
		t.Fatalf("error = %#v", err)
	}
}

func writeTestProjection(t *testing.T, w http.ResponseWriter, command string, data any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "command": command, "status": "success", "data": data}); err != nil {
		t.Fatal(err)
	}
}

func writeTestErrorProjection(t *testing.T, w http.ResponseWriter, command, code, message, hint string) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "command": command, "status": "failed", "error": map[string]any{"code": code, "message": message, "hint": hint}}); err != nil {
		t.Fatal(err)
	}
}
