package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestWriterTagsLookupOriginalReceiptWithoutProposal(t *testing.T) {
	svc, req := tagPlanFixture(t)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	original, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
	if err != nil || original.Status != operation.StatusSucceeded {
		t.Fatal("fixture apply failed", err)
	}
	// Historical receipt is independent of current source existence, not an
	// authorization to open a deleted or hidden note.
	for _, item := range plan.Items {
		if err := os.Remove(filepath.Join(req.Writer.VaultPath, item.Path)); err != nil {
			t.Fatal(err)
		}
	}
	lookup := WriterTagsLookupRequest{VaultPath: req.Writer.VaultPath, Session: req.Writer.Session, OperationID: req.Writer.OperationID}
	before := tagPlanTree(t, req.Writer.VaultPath)
	got, err := NewService().WriterTagsLookup(t.Context(), lookup)
	if err != nil || !reflect.DeepEqual(original, got) {
		t.Fatal("lookup did not return original receipt", err)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("receipt lookup changed source tree")
	}
	for _, mutate := range []func(*WriterTagsLookupRequest){
		func(r *WriterTagsLookupRequest) { r.Session.SessionID = "other" },
		func(r *WriterTagsLookupRequest) { r.Session.BindingID = "other" },
		func(r *WriterTagsLookupRequest) { r.Session.Scopes = []string{WriterScopeEditor, ReaderScopeBody} },
		func(r *WriterTagsLookupRequest) {
			r.Session.Scopes = append(append([]string(nil), r.Session.Scopes...), ReaderScopeRelations)
		},
		func(r *WriterTagsLookupRequest) {
			r.OperationID = writerTagsChildID(req.Writer.OperationID, plan.Items[0].ObjectID)
		},
		func(r *WriterTagsLookupRequest) { r.OperationID = "not-found" },
	} {
		other := lookup
		mutate(&other)
		if _, err := svc.WriterTagsLookup(t.Context(), other); err == nil {
			t.Fatal("foreign scope or non-parent operation exposed")
		}
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("rejected lookup wrote source")
	}
}

func TestWriterTagsLookupIncompleteStatesNoResume(t *testing.T) {
	for _, status := range []operation.Status{operation.StatusAccepted, operation.StatusApplying, operation.StatusReconcileRequired} {
		t.Run(string(status), func(t *testing.T) {
			svc, req := tagPlanFixture(t)
			plan, err := svc.WriterTagsPlan(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			store, err := operation.Open(req.Writer.VaultPath)
			if err != nil {
				t.Fatal(err)
			}
			created, err := store.CreateAccepted(t.Context(), writerTagsOperation(req.Writer, plan.Digest))
			if err != nil {
				t.Fatal(err)
			}
			row := created.Operation
			if status != operation.StatusAccepted {
				row, err = store.StartApplying(t.Context(), req.Writer.OperationID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if status == operation.StatusReconcileRequired {
				row, err = store.RequireReconcile(t.Context(), req.Writer.OperationID, operation.Outcome{ErrorCode: "outcome_unknown", ErrorMessage: "Inspect the original operation"})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			before := tagPlanTree(t, req.Writer.VaultPath)
			result, err := svc.WriterTagsLookup(t.Context(), WriterTagsLookupRequest{VaultPath: req.Writer.VaultPath, Session: req.Writer.Session, OperationID: req.Writer.OperationID})
			if err != nil || !reflect.DeepEqual(result, operation.ViewFromRow(row)) {
				t.Fatal("incomplete state mutated", err)
			}
			if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
				t.Fatal("lookup resumed or initialized state")
			}
		})
	}
}

func TestWriterTagsLookupMissingCorruptAndLinkedLedger(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "linked"} {
		t.Run(mode, func(t *testing.T) {
			svc, req := tagPlanFixture(t)
			file := filepath.Join(req.Writer.VaultPath, ".pinax/api/operations.sqlite")
			if mode != "missing" {
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if mode == "corrupt" {
					if err := os.WriteFile(file, []byte("not a sqlite database"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					outside := filepath.Join(t.TempDir(), "ledger")
					if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, file); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := tagPlanTree(t, req.Writer.VaultPath)
			_, err := svc.WriterTagsLookup(t.Context(), WriterTagsLookupRequest{VaultPath: req.Writer.VaultPath, Session: req.Writer.Session, OperationID: req.Writer.OperationID})
			var opErr *operation.Error
			if !errors.As(err, &opErr) {
				t.Fatal("expected typed ledger error", err)
			}
			if (opErr.Code == operation.CodeNotFound) != (mode == "missing") {
				t.Fatal("corrupt ledger masqueraded as missing", opErr.Code)
			}
			if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
				t.Fatal("lookup changed failed ledger")
			}
		})
	}
}

func TestWriterTagsLookupCancelled(t *testing.T) {
	svc, req := tagPlanFixture(t)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := tagPlanTree(t, req.Writer.VaultPath)
	if _, err := svc.WriterTagsLookup(ctx, WriterTagsLookupRequest{VaultPath: req.Writer.VaultPath, Session: req.Writer.Session, OperationID: req.Writer.OperationID}); err == nil {
		t.Fatal("cancelled lookup succeeded")
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("cancelled lookup wrote source")
	}
}
