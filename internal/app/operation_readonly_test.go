package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/sqlitedsn"
)

func TestOperationShowRefusesLedgerSymlink(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	seed := seedAppOperation(t, t.Context(), external, "op-readonly-link", operation.IdentityDigest("p"), operation.IdentityDigest("s"))
	if err := os.MkdirAll(filepath.Join(root, ".pinax/api"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, ".pinax/api/operations.sqlite"), filepath.Join(root, ".pinax/api/operations.sqlite")); err != nil {
		t.Skip("symlink not supported")
	}
	before := tagPlanTree(t, external)
	projection, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: root, OperationID: seed.OperationID, Access: OperationAccess{OwnerLocal: true}})
	if err == nil || projection.Error == nil || projection.Error.Code != "operation_store_unavailable" {
		t.Fatalf("followed foreign ledger: %v", err)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, external)) {
		t.Fatal("foreign ledger changed")
	}
}

func TestOperationShowDoesNotMigrateExistingLedger(t *testing.T) {
	root := t.TempDir()
	seed := seedAppOperation(t, t.Context(), root, "op-readonly-schema", operation.IdentityDigest("p"), operation.IdentityDigest("s"))
	path := filepath.Join(root, ".pinax/api/operations.sqlite")
	db, err := sqlitedsn.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// A read of an existing operation must not rebuild an unrelated missing
	// idempotency table; only the owner write path may migrate its store.
	if err := db.Migrator().DropTable(&operation.IdempotencyRow{}); err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	before := tagPlanTree(t, root)
	projection, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: root, OperationID: seed.OperationID, Access: OperationAccess{PrincipalDigest: seed.PrincipalDigest, ScopeDigest: seed.ScopeDigest}})
	if err != nil || projection.Facts["operation_id"] != seed.OperationID {
		t.Fatal("valid original row unreadable", err)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, root)) {
		t.Fatal("operation read migrated or modified source ledger")
	}
}
