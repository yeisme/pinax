package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestOperationShowAndReconcileLocalCLI(t *testing.T) {
	root := t.TempDir()
	operationID := "op-cli-local-1"
	seedCLIOperation(t, root, operationID)

	show, err := executeOperationCommand(t, "--vault", root, "operation", "show", operationID, "--json")
	if err != nil || !strings.Contains(show, `"command":"operation.show"`) || !strings.Contains(show, `"operation_id":"`+operationID+`"`) {
		t.Fatalf("operation show: err=%v out=%s", err, show)
	}
	reconcile, err := executeOperationCommand(t, "--vault", root, "operation", "reconcile", operationID, "--json")
	if err != nil || !strings.Contains(reconcile, `"command":"operation.reconcile"`) || !strings.Contains(reconcile, `"operation_status":"reconcile_required"`) {
		t.Fatalf("operation reconcile: err=%v out=%s", err, reconcile)
	}
}

func TestOperationShowRemoteCLIUsesRegisteredRPC(t *testing.T) {
	operationID := "op-cli-remote-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/rpc" {
			t.Fatalf("remote operation request = %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method != "Pinax.Operation.Get" || request.Params["operation_id"] != operationID {
			t.Fatalf("remote operation RPC = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spec_version": "1.0", "command": "operation.show", "status": "success",
			"data": map[string]any{"operation": map[string]any{"schema_version": "pinax.operation.v1", "operation_id": operationID}},
		})
	}))
	defer server.Close()

	out, err := executeOperationCommand(t, "--api-url", server.URL, "operation", "show", operationID, "--json")
	if err != nil || !strings.Contains(out, `"command":"operation.show"`) || !strings.Contains(out, operationID) {
		t.Fatalf("remote operation show: err=%v out=%s", err, out)
	}
}

func executeOperationCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := NewRootCommand("test")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func seedCLIOperation(t *testing.T, root, operationID string) {
	t.Helper()
	digest, err := operation.CanonicalDigest(map[string]any{"title": "CLI operation fixture"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	_, err = store.CreateAccepted(context.Background(), operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "inbox.capture", BindingID: "rest.inbox.capture",
		PrincipalDigest: operation.IdentityDigest("principal", "cli"), ScopeDigest: operation.IdentityDigest("scope", "vault"), RequestDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplying(context.Background(), operationID); err != nil {
		t.Fatal(err)
	}
}
