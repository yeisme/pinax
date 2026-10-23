package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func TestWriterTagsApplyAndOriginalReceipt(t *testing.T) {
	svc, req := tagPlanFixture(t)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	original := map[string]string{}
	for _, item := range plan.Items {
		raw, err := os.ReadFile(filepath.Join(req.Writer.VaultPath, item.Path))
		if err != nil {
			t.Fatal(err)
		}
		original[item.ObjectID] = string(raw)
	}
	view, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatalf("apply=%s err=%v", view.Status, err)
	}
	var result writerTagsResult
	if err := json.Unmarshal(view.Result, &result); err != nil || len(result.Items) != 2 {
		t.Fatal("missing item receipts", err)
	}
	access := writerTagsOperationAccess(req.Writer.Session)
	for i, item := range plan.Items {
		raw, err := os.ReadFile(filepath.Join(req.Writer.VaultPath, item.Path))
		if err != nil {
			t.Fatal(err)
		}
		beforeMeta, beforeBody := splitFrontmatter(original[item.ObjectID])
		meta, body := splitFrontmatter(string(raw))
		if body != beforeBody || meta["private_custom"] != beforeMeta["private_custom"] || !reflect.DeepEqual(parseTags(meta["tags"]), item.After) {
			t.Fatal("tag apply changed unrelated source")
		}
		child, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: req.Writer.VaultPath, OperationID: result.Items[i].OperationID, Access: access.Access})
		if err != nil || child.Facts["operation_status"] != "succeeded" || !strings.HasPrefix(child.Facts["receipt_ref"], "snapshot:") {
			t.Fatal("child owner receipt unavailable", err)
		}
	}
	// Replay is original evidence, not the current projection after later edits.
	path := filepath.Join(req.Writer.VaultPath, plan.Items[0].Path)
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, []byte("\nExternal edit after success\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	before := tagPlanTree(t, req.Writer.VaultPath)
	replay, err := NewService().WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
	if err != nil || !reflect.DeepEqual(view, replay) {
		t.Fatal("original parent receipt changed", err)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
		t.Fatal("replay changed vault bytes")
	}
	foreignSession := req.Writer.Session
	foreignSession.SessionID = "different-session"
	foreign := writerTagsOperationAccess(foreignSession)
	if _, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: req.Writer.VaultPath, OperationID: req.Writer.OperationID, Access: foreign.Access}); err == nil {
		t.Fatal("foreign session read parent receipt")
	}
	for _, child := range result.Items {
		if _, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: req.Writer.VaultPath, OperationID: child.OperationID, Access: foreign.Access}); err == nil {
			t.Fatal("foreign session read child receipt")
		}
	}
	encoded, _ := json.Marshal(view)
	for _, text := range []string{"Exact private body", "never-return-this-field", req.OldTag, req.NewTag, req.Writer.VaultPath} {
		if strings.Contains(string(encoded), text) {
			t.Fatal("receipt persisted plan content")
		}
	}
}

func TestWriterTagsApplyInterruptedParentNeverResumes(t *testing.T) {
	for _, status := range []operation.Status{operation.StatusAccepted, operation.StatusApplying} {
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
			_, err = store.CreateAccepted(t.Context(), writerTagsOperation(req.Writer, plan.Digest))
			if err != nil {
				t.Fatal(err)
			}
			if status == operation.StatusApplying {
				if _, err = store.StartApplying(t.Context(), req.Writer.OperationID); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			before := tagPlanTree(t, req.Writer.VaultPath)
			view, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
			if err != nil || view.Status != status || view.OperationID != req.Writer.OperationID {
				t.Fatal("interrupted parent identity changed", err)
			}
			if !reflect.DeepEqual(before, tagPlanTree(t, req.Writer.VaultPath)) {
				t.Fatal("interrupted replay wrote remaining items")
			}
		})
	}
}

func TestWriterTagsApplyConcurrentSameIdentity(t *testing.T) {
	svc, req := tagPlanFixture(t)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	views := make(chan operation.View, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			view, err := NewService().WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
			if err != nil {
				errors <- err
			} else {
				views <- view
			}
		}()
	}
	close(start)
	wg.Wait()
	close(views)
	close(errors)
	successes := 0
	for view := range views {
		if view.Status != operation.StatusSucceeded {
			t.Fatal("unexpected concurrent status", view.Status)
		}
		successes++
	}
	if successes == 0 {
		t.Fatal("no writer succeeded")
	}
	for err := range errors {
		if !strings.Contains(err.Error(), "lock_held") {
			t.Fatal("unexpected concurrent error", err)
		}
	}
	view, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatal("same identity could not recover receipt", err)
	}
}

func TestWriterTagsApplyGuardsAndPartialRecovery(t *testing.T) {
	for _, name := range []string{"tamper", "scope", "session", "authorization", "stale-last", "snapshot-second", "reconcile-active"} {
		t.Run(name, func(t *testing.T) {
			svc, req := tagPlanFixture(t)
			plan, err := svc.WriterTagsPlan(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			auth := "reviewed_proposal"
			switch name {
			case "tamper":
				plan.Items[0].After = []string{"forged"}
			case "scope":
				req.Writer.Session.Scopes = []string{WriterScopeEditor}
			case "session":
				req.Writer.Session.SessionID = "other"
			case "authorization":
				auth = ""
			case "stale-last":
				path := filepath.Join(req.Writer.VaultPath, plan.Items[1].Path)
				raw, _ := os.ReadFile(path)
				if err := os.WriteFile(path, append(raw, 'x'), 0600); err != nil {
					t.Fatal(err)
				}
			case "snapshot-second", "reconcile-active":
				calls := 0
				svc = NewServiceWithVersionBackend(dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() {
					calls++
					if name == "reconcile-active" {
						if _, err := NewService().OperationReconcile(t.Context(), OperationRequest{VaultPath: req.Writer.VaultPath, OperationID: req.Writer.OperationID, Access: OperationAccess{OwnerLocal: true}}); err == nil {
							t.Fatal("reconcile crossed active writer lock")
						}
					} else if calls == 2 {
						path := filepath.Join(req.Writer.VaultPath, plan.Items[1].Path)
						raw, _ := os.ReadFile(path)
						if err := os.WriteFile(path, append(raw, []byte("\nExternal during snapshot\n")...), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}})
			}
			before := map[string]string{}
			for _, item := range plan.Items {
				raw, _ := os.ReadFile(filepath.Join(req.Writer.VaultPath, item.Path))
				before[item.ObjectID] = string(raw)
			}
			view, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, auth)
			if name == "reconcile-active" {
				if err != nil || view.Status != operation.StatusSucceeded {
					t.Fatal("active apply failed", err)
				}
				return
			}
			if name == "snapshot-second" {
				if err != nil || view.Status != operation.StatusReconcileRequired {
					t.Fatal("partial result not unknown", err)
				}
				var result writerTagsResult
				if err := json.Unmarshal(view.Result, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Items) != 2 || result.Items[0].Status != "succeeded" || result.Items[1].Status != "reconcile_required" {
					t.Fatalf("lost partial item evidence: %#v", result)
				}
				tree := tagPlanTree(t, req.Writer.VaultPath)
				replay, err := NewService().WriterTagsApply(t.Context(), req.Writer, plan, auth)
				if err != nil || !reflect.DeepEqual(view, replay) || !reflect.DeepEqual(tree, tagPlanTree(t, req.Writer.VaultPath)) {
					t.Fatal("partial replay resumed writes", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid apply accepted")
			}
			for _, item := range plan.Items {
				raw, _ := os.ReadFile(filepath.Join(req.Writer.VaultPath, item.Path))
				if string(raw) != before[item.ObjectID] {
					t.Fatal("invalid apply changed a note")
				}
			}
		})
	}
}
