package pinaxclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterDailyEditClientLostReceipt(t *testing.T) {
	server, bearer := writerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.WriterScopeEditor)
	var calls atomic.Int32
	var original WriterApplyReceipt
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/journals/daily/edit:apply" && calls.Add(1) == 1 {
			recorded := httptest.NewRecorder()
			server.Config.Handler.ServeHTTP(recorded, r)
			var envelope struct {
				Data WriterApplyReceipt `json:"data"`
			}
			if recorded.Code != 200 || json.Unmarshal(recorded.Body.Bytes(), &envelope) != nil || envelope.Data.Status != "succeeded" {
				t.Error("fixture edit did not commit")
			}
			original = envelope.Data
			w.WriteHeader(503)
			return
		}
		server.Config.Handler.ServeHTTP(w, r)
	}))
	defer front.Close()
	client := writerClient(t, front.URL, bearer)
	ctx := context.Background()
	created, err := client.WriterDailyPropose(ctx, "2026-08-20", "UTC", "sdk-create")
	if err != nil {
		t.Fatal(err)
	}
	createReceipt, err := client.WriterDailyApply(ctx, created, "reviewed_proposal")
	if err != nil {
		t.Fatal(err)
	}
	source, err := client.WriterDailyEditorSource(ctx, created.Date, created.Timezone, created.ObjectID, createReceipt.RevisionAfter)
	if err != nil {
		t.Fatal(err)
	}
	req := WriterDailyEditRequest{Date: created.Date, Timezone: created.Timezone, ObjectID: created.ObjectID, Revision: source.Revision, OperationID: "sdk-edit-original", Body: source.Body + "\nSDK exact daily update.  \r\n"}
	proposal, err := client.WriterDailyEditPropose(ctx, req)
	if err != nil || proposal.Preview.Before != source.Body {
		t.Fatal("source/preview mismatch", err)
	}
	_, err = client.WriterDailyEditApply(ctx, proposal, "reviewed_proposal")
	var failure *Error
	if !errors.As(err, &failure) || failure.OperationRef != req.OperationID || !failure.ReconcileRequired || calls.Load() != 1 {
		t.Fatal("lost response did not preserve original identity", err)
	}
	receipt, err := client.WriterDailyEditApply(ctx, proposal, "reviewed_proposal")
	if err != nil || calls.Load() != 2 || !reflect.DeepEqual(receipt, original) {
		t.Fatal("explicit replay did not return exact receipt", err)
	}
	updated, err := client.WriterDailyEditorSource(ctx, req.Date, req.Timezone, req.ObjectID, receipt.RevisionAfter)
	if err != nil || updated.Body != req.Body || updated.ProtectedMetadata.NoteID != source.ProtectedMetadata.NoteID {
		t.Fatal("readback changed body or identity", err)
	}
	if _, err = client.WriterDailyEditorSource(ctx, req.Date, req.Timezone, req.ObjectID, req.Revision); err == nil {
		t.Fatal("stale source accepted")
	}
}

func dailyEditClientFixture() WriterDailyEditProposal {
	digest := "sha256:" + strings.Repeat("a", 64)
	return WriterDailyEditProposal{SchemaVersion: catalogschema.WriterDailyEditProposalV1Alpha1, Date: "2026-08-20", Timezone: "UTC", ObjectID: "daily", PreviewDigest: digest, Preview: WriterDailyEditPreview{Change: WriterChange{OperationID: "edit-fixture", Action: "replace", NoteRef: "daily/day.md", ExpectedRevision: digest, Body: "new"}, PreviewDigest: digest, Path: "daily/day.md", Before: "old", After: "new", State: "preview"}}
}

func TestWriterDailyEditClientPreviewValidation(t *testing.T) {
	for _, mode := range []string{"valid", "identity", "body", "path", "scope", "extra"} {
		t.Run(mode, func(t *testing.T) {
			p := dailyEditClientFixture()
			switch mode {
			case "identity":
				p.ObjectID = "other"
			case "body":
				p.Preview.After = "unrequested"
			case "path":
				p.Preview.Path = "../escape.md"
				p.Preview.Change.NoteRef = p.Preview.Path
			case "scope":
				p.Preview.Change.Action = "archive"
			}
			raw, _ := json.Marshal(p)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if mode == "extra" {
				payload["vault_root"] = "private"
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.daily.edit.propose", "data": payload})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic")
			req := WriterDailyEditRequest{Date: "2026-08-20", Timezone: "UTC", ObjectID: "daily", Revision: dailyEditClientFixture().Preview.Change.ExpectedRevision, OperationID: "edit-fixture", Body: "new"}
			_, err := client.WriterDailyEditPropose(t.Context(), req)
			if mode == "valid" {
				if err != nil {
					t.Fatal("valid control rejected", err)
				}
			} else if err == nil {
				t.Fatal("unsafe preview accepted")
			}
			req.Body = strings.Repeat("字", 23000)
			if _, err = client.WriterDailyEditPropose(t.Context(), req); err == nil || calls != 1 {
				t.Fatal("invalid bytes reached transport")
			}
		})
	}
}

func TestWriterDailyEditClientReceiptValidation(t *testing.T) {
	for _, mode := range []string{"valid", "digest", "identity", "resource", "content", "missing-proof"} {
		t.Run(mode, func(t *testing.T) {
			p := dailyEditClientFixture()
			p.PreviewDigest = "sha256:" + strings.Repeat("b", 64)
			result := map[string]string{"note_id": p.ObjectID, "path": p.Preview.Path, "content_digest": p.Preview.Change.ExpectedRevision, "record_digest": p.Preview.Change.ExpectedRevision}
			if mode == "content" {
				result["content_digest"] = "wrong"
			}
			raw, _ := json.Marshal(result)
			receipt := WriterApplyReceipt{SchemaVersion: catalogschema.WriterApplyReceiptV1Alpha1, OperationID: p.Preview.Change.OperationID, CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", RequestDigest: p.PreviewDigest, Status: "succeeded", Result: raw, Receipt: json.RawMessage(`{"operation_id":"edit-fixture","snapshot":"snapshot:original","record_event_id":"record-fixture"}`), ReceiptRef: "snapshot:original", ResourceRef: "pinax://note/" + p.ObjectID, RevisionBefore: p.Preview.Change.ExpectedRevision, RevisionAfter: p.Preview.Change.ExpectedRevision, CreatedAt: "2026-08-20T00:00:00Z", UpdatedAt: "2026-08-20T00:00:00Z", AcceptedAt: "2026-08-20T00:00:00Z"}
			switch mode {
			case "digest":
				receipt.RequestDigest = p.Preview.PreviewDigest
			case "identity":
				receipt.OperationID = "foreign"
			case "resource":
				receipt.ResourceRef = "pinax://note/daily/day.md"
			case "missing-proof":
				receipt.Receipt = nil
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.daily.edit", "data": receipt})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic")
			_, err := client.WriterDailyEditApply(t.Context(), p, "reviewed_proposal")
			if mode == "valid" {
				if err != nil {
					t.Fatal("valid receipt rejected", err)
				}
			} else {
				var failure *Error
				if !errors.As(err, &failure) || failure.OperationRef != p.Preview.Change.OperationID || !failure.ReconcileRequired {
					t.Fatal("receipt mismatch did not preserve unknown", err)
				}
			}
			if calls != 1 {
				t.Fatal("client retried without confirmation")
			}
		})
	}
}
