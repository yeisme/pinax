package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func dailyEditFixture(t *testing.T) (string, *Service, WriterDailyRequest, WriterDailyProposal) {
	t.Helper()
	root := t.TempDir()
	svc := NewService()
	req := dailyWriterRequestFixture(root, "create-before-edit")
	proposal, err := svc.WriterDailyPropose(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.WriterDailyApply(t.Context(), req, proposal, "reviewed_proposal")
	if err != nil || result.Status != operation.StatusSucceeded {
		t.Fatal("fixture creation failed", err)
	}
	return root, svc, req, proposal
}

func TestWriterDailyEditProposalApplyAndOriginalReceipt(t *testing.T) {
	root, svc, req, created := dailyEditFixture(t)
	path := filepath.Join(root, created.Path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tree := func() map[string]string {
		result := map[string]string{}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				result[rel] = "directory"
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] = collaborationRevision(string(raw))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeTree := tree()
	req.OperationID = "daily-edit-original"
	body := "\nExact revised daily body.  \r\n\n"
	proposal, err := svc.WriterDailyEditPropose(t.Context(), WriterDailyEditRequest{Daily: req, ObjectID: created.ObjectID, Revision: collaborationRevision(string(before)), Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeTree, tree()) {
		t.Fatal("proposal changed vault metadata or sidecars")
	}
	if proposal.Preview.After != body || proposal.Preview.Before != created.After {
		t.Fatal("preview normalized body")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(before) {
		t.Fatal("proposal wrote body")
	}
	result, err := svc.WriterDailyEditApply(t.Context(), req, proposal, "reviewed_proposal")
	if err != nil || result.Status != operation.StatusSucceeded {
		t.Fatal("apply failed", result.Status, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	meta, actual := splitFrontmatter(string(after))
	oldMeta, _ := splitFrontmatter(string(before))
	if actual != body || meta["note_id"] != created.ObjectID || meta["kind"] != "daily" || meta["title"] != oldMeta["title"] || meta["tags"] != oldMeta["tags"] || result.RevisionAfter != collaborationRevision(string(after)) {
		t.Fatal("body/metadata/receipt mismatch")
	}
	replay, err := NewService().WriterDailyEditApply(t.Context(), req, proposal, "reviewed_proposal")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(result)
	second, _ := json.Marshal(replay)
	if string(first) != string(second) {
		t.Fatal("receipt replay changed identity")
	}
	store, err := operation.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	row, err := store.Get(t.Context(), req.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	var proof dailyEditResult
	if err := json.Unmarshal(row.ResultJSON, &proof); err != nil || proof.NoteID != created.ObjectID || proof.Path != created.Path || proof.ContentDigest != result.RevisionAfter || proof.RecordDigest != hashString(meta["title"]+"\x00"+body) {
		t.Fatal("real apply omitted fixed recovery evidence", err)
	}
	if row.CapabilityID != writerDailyEditCapability || strings.Contains(string(row.ResultJSON), body) || strings.Contains(string(row.ReceiptJSON), body) {
		t.Fatal("ledger identity or body boundary failed")
	}
}

func TestWriterDailyEditGuardsAndSnapshotDrift(t *testing.T) {
	for _, name := range []string{"body_tamper", "date", "session", "authorization", "before_apply", "during_snapshot", "reconcile_during_snapshot"} {
		t.Run(name, func(t *testing.T) {
			root, svc, req, created := dailyEditFixture(t)
			path := filepath.Join(root, created.Path)
			raw, _ := os.ReadFile(path)
			req.OperationID = "guarded-edit"
			proposal, err := svc.WriterDailyEditPropose(t.Context(), WriterDailyEditRequest{Daily: req, ObjectID: created.ObjectID, Revision: collaborationRevision(string(raw)), Body: "replacement"})
			if err != nil {
				t.Fatal(err)
			}
			auth := "reviewed_proposal"
			expected := string(raw)
			switch name {
			case "body_tamper":
				proposal.Preview.Change.Body = "unreviewed"
			case "date":
				req.Date = "2026-08-21"
			case "session":
				req.Session.SessionID = "foreign"
			case "authorization":
				auth = ""
			case "before_apply":
				expected += "external"
				if err = os.WriteFile(path, []byte(expected), 0600); err != nil {
					t.Fatal(err)
				}
			case "reconcile_during_snapshot":
				svc = NewServiceWithVersionBackend(dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() {
					_, reconcileErr := NewService().OperationReconcile(t.Context(), OperationRequest{VaultPath: root, OperationID: req.OperationID, Access: OperationAccess{OwnerLocal: true}})
					if reconcileErr == nil {
						t.Fatal("reconcile interfered with active daily edit")
					}
				}})
			case "during_snapshot":
				expected += "external"
				svc = NewServiceWithVersionBackend(dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() {
					if err := os.WriteFile(path, []byte(expected), 0600); err != nil {
						t.Fatal(err)
					}
				}})
			}
			result, err := svc.WriterDailyEditApply(context.Background(), req, proposal, auth)
			if name == "during_snapshot" {
				if err != nil || result.Status != operation.StatusReconcileRequired {
					t.Fatal("accepted conflict did not preserve unknown original operation", err, result.Status)
				}
			} else if name == "reconcile_during_snapshot" {
				if err != nil || result.Status != operation.StatusSucceeded {
					t.Fatal("active edit interrupted", err)
				}
				return
			} else if err == nil {
				t.Fatal("guard admitted invalid apply")
			}
			after, _ := os.ReadFile(path)
			if string(after) != expected {
				t.Fatal("guard overwrote source")
			}
		})
	}
}
