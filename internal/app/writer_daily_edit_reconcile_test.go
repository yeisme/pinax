package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/records"
)

func TestWriterDailyEditInterruptedEvidence(t *testing.T) {
	for _, scenario := range []string{"record", "no-record", "changed", "wrong-record", "wrong-id", "missing-proof"} {
		t.Run(scenario, func(t *testing.T) {
			root, svc, req, created := dailyEditFixture(t)
			path := filepath.Join(root, created.Path)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			req.OperationID = "interrupted-edit"
			body := "\nReviewed daily edit with whitespace.  \n"
			proposal, err := svc.WriterDailyEditPropose(t.Context(), WriterDailyEditRequest{Daily: req, ObjectID: created.ObjectID, Revision: collaborationRevision(string(original)), Body: body})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := svc.VersionSnapshot(t.Context(), SnapshotRequest{VaultPath: root, Message: "Synthetic daily edit crash"})
			if err != nil {
				t.Fatal(err)
			}
			_, oldBody := splitFrontmatter(string(original))
			updated := strings.TrimSuffix(string(original), oldBody) + body
			updated, _ = patchFrontmatterFields(updated, map[string]string{"updated_at": "2026-08-20T12:00:00Z"})
			note := parseNote(created.Path, updated)
			note.Body = body
			proof := dailyEditResult{NoteID: created.ObjectID, Path: created.Path, ContentDigest: collaborationRevision(updated), RecordDigest: hashString(note.Title + "\x00" + body)}
			if scenario == "wrong-id" {
				proof.NoteID = "not-the-original"
			}
			encoded, _ := json.Marshal(proof)
			if scenario == "missing-proof" {
				encoded = []byte(`{"path":"daily/2026-08-20.md"}`)
			}
			access := WriterOperationAccess(req.Session.SessionID, req.Session.BindingID, req.Session.Scopes)
			store, err := operation.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			_, err = store.CreateAccepted(t.Context(), operation.CreateRequest{OperationID: req.OperationID, IdempotencyKey: req.OperationID, CapabilityID: writerDailyEditCapability, BindingID: "rest.writer.daily.edit", PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest, RequestDigest: proposal.Digest, RevisionBefore: proposal.Preview.Change.ExpectedRevision})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.StartApplyingWithOutcome(t.Context(), req.OperationID, operation.Outcome{Result: encoded, ReceiptRef: "snapshot:" + snapshot.Facts["snapshot_id"]})
			if err != nil {
				t.Fatal(err)
			}
			if err = atomicWriteFile(path, []byte(updated), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario != "no-record" {
				eventNote := note
				if scenario == "wrong-record" {
					eventNote.Body = "unrelated"
				}
				if _, err = appendNoteRecordEvent(t.Context(), root, domain.RecordEventNoteMetadataUpdated, req.OperationID, eventNote, ""); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "changed" {
				updated += "external"
				if err = os.WriteFile(path, []byte(updated), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = store.RequireReconcile(t.Context(), req.OperationID, operation.Outcome{ErrorCode: "outcome_unknown", ErrorMessage: "Synthetic interruption"}); err != nil {
				t.Fatal(err)
			}
			eventsBefore, err := records.NewService(root).Replay(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = svc.OperationReconcile(t.Context(), OperationRequest{VaultPath: root, OperationID: req.OperationID, Access: OperationAccess{OwnerLocal: true}}); err != nil {
				t.Fatal(err)
			}
			row, err := store.Get(t.Context(), req.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			want := operation.StatusReconcileRequired
			if scenario == "record" {
				want = operation.StatusSucceeded
			}
			if row.Status != want {
				t.Fatal("recovery did not require all evidence", row.Status)
			}
			if scenario == "record" && (row.RevisionAfter != proof.ContentDigest || operation.ViewFromRow(row).Error != nil) {
				t.Fatal("recovered receipt mismatch")
			}
			replay, err := NewService().WriterDailyEditApply(t.Context(), req, proposal, "reviewed_proposal")
			if err != nil || replay.Status != want {
				t.Fatal("replay changed recovery result", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != updated {
				t.Fatal("recovery replayed mutation")
			}
			eventsAfter, err := records.NewService(root).Replay(t.Context())
			if err != nil || len(eventsBefore.Records) != len(eventsAfter.Records) {
				t.Fatal("recovery appended an event", err)
			}
		})
	}
}
