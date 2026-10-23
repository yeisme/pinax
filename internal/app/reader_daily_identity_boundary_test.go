package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestWriterDailyIdentityLedgerFailure(t *testing.T) {
	for _, name := range []string{"missing", "corrupt", "incomplete", "symlink"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			switch name {
			case "corrupt":
				writeAppFixture(t, filepath.Join(root, ".pinax/api/operations.sqlite"), "not a database")
			case "incomplete":
				store, err := operation.Open(root)
				if err != nil {
					t.Fatal(err)
				}
				_, err = store.CreateAccepted(context.Background(), operation.CreateRequest{OperationID: "incomplete-day", IdempotencyKey: "incomplete-day", CapabilityID: writerDailyApplyCapability, BindingID: "rest.writer.daily.apply", PrincipalDigest: operation.IdentityDigest("fixture"), ScopeDigest: operation.IdentityDigest("scope"), RequestDigest: operation.IdentityDigest("request")})
				if closeErr := store.Close(); err != nil || closeErr != nil {
					t.Fatal("fixture failed", err, closeErr)
				}
			case "symlink":
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(root, ".pinax")); err != nil {
					t.Skip("symlink unavailable")
				}
			}
			id, err := readerDailyRecordedIdentity(context.Background(), root, "2026-08-20")
			if name == "missing" {
				if err != nil || id != "" {
					t.Fatal("missing ledger initialized", err)
				}
				entries, _ := os.ReadDir(root)
				if len(entries) != 0 {
					t.Fatal("missing lookup wrote files")
				}
			} else {
				requireEditorSourceError(t, err, "reader_unavailable")
				if id != "" {
					t.Fatal("failed ledger disclosed identity")
				}
			}
		})
	}
}
