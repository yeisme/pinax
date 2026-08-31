package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestOperationShowEnforcesScopeAndProjectsOnlyBoundedFields(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	principal := operation.IdentityDigest("principal", "authorized")
	scope := operation.IdentityDigest("scope", "vault")
	request := seedAppOperation(t, ctx, root, "op-app-show-1", principal, scope)

	service := NewService()
	projection, err := service.OperationShow(ctx, OperationRequest{
		VaultPath: root, OperationID: request.OperationID,
		Access: OperationAccess{PrincipalDigest: principal, ScopeDigest: scope},
	})
	if err != nil || projection.Facts["operation_id"] != request.OperationID || projection.Facts["operation_status"] != "applying" {
		t.Fatalf("operation show = %#v err=%v", projection, err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{principal, scope, request.IdempotencyKey, root} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("operation projection leaked private binding %q: %s", forbidden, encoded)
		}
	}

	denied, err := service.OperationShow(ctx, OperationRequest{
		VaultPath: root, OperationID: request.OperationID,
		Access: OperationAccess{PrincipalDigest: operation.IdentityDigest("principal", "other"), ScopeDigest: scope},
	})
	if err == nil || denied.Error == nil || denied.Error.Code != "operation_not_found" {
		t.Fatalf("scope mismatch leaked operation: projection=%#v err=%v", denied, err)
	}
}

func TestOperationReconcileKeepsUnknownOutcomeOnSameIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	request := seedAppOperation(t, ctx, root, "op-app-reconcile-1", operation.IdentityDigest("principal", "owner"), operation.IdentityDigest("scope", "vault"))
	service := NewService()

	projection, err := service.OperationReconcile(ctx, OperationRequest{VaultPath: root, OperationID: request.OperationID, Access: OperationAccess{OwnerLocal: true}})
	if err != nil || projection.Facts["operation_id"] != request.OperationID || projection.Facts["operation_status"] != "reconcile_required" || projection.Facts["reconcile_required"] != "true" {
		t.Fatalf("operation reconcile = %#v err=%v", projection, err)
	}
	if len(projection.Actions) != 1 || !strings.Contains(projection.Actions[0].Command, request.OperationID) {
		t.Fatalf("reconcile next action = %#v", projection.Actions)
	}
}

func TestOperationShowMissingDoesNotCreateLedger(t *testing.T) {
	root := t.TempDir()
	projection, err := NewService().OperationShow(context.Background(), OperationRequest{VaultPath: root, OperationID: "op-missing", Access: OperationAccess{OwnerLocal: true}})
	if err == nil || projection.Error == nil || projection.Error.Code != "operation_not_found" {
		t.Fatalf("missing operation = %#v err=%v", projection, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(statErr) {
		t.Fatalf("read-only missing lookup created ledger: %v", statErr)
	}
}

func seedAppOperation(t *testing.T, ctx context.Context, root, operationID, principal, scope string) operation.CreateRequest {
	t.Helper()
	digest, err := operation.CanonicalDigest(map[string]any{"title": "Operation fixture"})
	if err != nil {
		t.Fatal(err)
	}
	request := operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: "idem-" + operationID,
		CapabilityID: "inbox.capture", BindingID: "rest.inbox.capture",
		PrincipalDigest: principal, ScopeDigest: scope, RequestDigest: digest,
	}
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplying(ctx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	return request
}
