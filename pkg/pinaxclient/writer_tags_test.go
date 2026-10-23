package pinaxclient

import (
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

func TestWriterTagsClientLostReceiptLookupOnly(t *testing.T) {
	server, bearer := writerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.WriterScopeEditor)
	direct := writerClient(t, server.URL, bearer)
	// Prepare source tags through the original owner proposal engine.
	seed, err := direct.WriterPropose(t.Context(), WriterChange{Action: "tags", OperationID: "seed-tags", NoteRef: "Alpha.md", TagOperation: "set", Tags: []string{"old", "keep"}})
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := direct.WriterApply(t.Context(), WriterApplyRequest{Change: seed.Change, PreviewDigest: seed.PreviewDigest, Authorization: "reviewed_proposal"})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var original WriterTagsReceipt
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/tags:apply" {
			calls.Add(1)
			recorded := httptest.NewRecorder()
			server.Config.Handler.ServeHTTP(recorded, r)
			var envelope struct {
				Data WriterTagsReceipt `json:"data"`
			}
			if recorded.Code != 200 || json.Unmarshal(recorded.Body.Bytes(), &envelope) != nil || envelope.Data.Status != "succeeded" {
				t.Error("fixture did not commit")
			}
			original = envelope.Data
			w.WriteHeader(503)
			return
		}
		server.Config.Handler.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	client := writerClient(t, proxy.URL, bearer)
	req := WriterTagsPlanRequest{OperationID: "tags-sdk-original", Operation: "rename", OldTag: "old", NewTag: "new/path", Targets: []WriterTagTarget{{ObjectID: "01982d84-2b48-7000-8000-0000000000c1", Revision: seeded.RevisionAfter}}}
	plan, err := client.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WriterTagsApply(t.Context(), plan, "reviewed_proposal")
	var failure *Error
	if !errors.As(err, &failure) || failure.OperationRef != req.OperationID || !failure.ReconcileRequired || calls.Load() != 1 {
		t.Fatal("ambiguous apply lost identity", err)
	}
	receipt, err := client.WriterTagsLookup(t.Context(), req.OperationID)
	if err != nil || !reflect.DeepEqual(receipt, original) || calls.Load() != 1 {
		t.Fatal("lookup did not recover without write retry", err)
	}
	var result WriterTagsResult
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		t.Fatal(err)
	}
	source, err := direct.WriterEditorSource(t.Context(), req.Targets[0].ObjectID, result.Items[0].RevisionAfter)
	if err != nil || !reflect.DeepEqual(source.ProtectedMetadata.Tags, []string{"keep", "new/path"}) || !strings.Contains(source.Body, "Alpha body") {
		t.Fatal("source readback failed", err)
	}
}

func tagsClientFixture() (WriterTagsPlanRequest, WriterTagsPlan, WriterTagsReceipt) {
	digest := "sha256:" + strings.Repeat("a", 64)
	req := WriterTagsPlanRequest{OperationID: "tags-fixture", Operation: "rename", OldTag: "old", NewTag: "new", Targets: []WriterTagTarget{{ObjectID: "note_one", Revision: digest}}}
	plan := WriterTagsPlan{SchemaVersion: catalogschema.WriterTagsPlanV1Alpha1, OperationID: req.OperationID, Operation: req.Operation, OldTag: req.OldTag, OldTagID: tagIdentity("tag-v1:", req.OldTag), NewTag: req.NewTag, NewTagID: tagIdentity("tag-v1:", req.NewTag), PreviewDigest: digest, Items: []WriterTagPlanItem{{ObjectID: "note_one", Revision: digest, Path: "notes/one.md", Before: []string{"old"}, After: []string{"new"}}}}
	result, _ := json.Marshal(WriterTagsResult{Items: []WriterTagItemResult{{ObjectID: "note_one", OperationID: tagIdentity("tag-item-", req.OperationID+"\x00note_one"), Status: "succeeded", RevisionAfter: digest}}})
	receipt := WriterTagsReceipt{WriterApplyReceipt: WriterApplyReceipt{SchemaVersion: catalogschema.WriterTagsReceiptV1Alpha1, OperationID: req.OperationID, CapabilityID: "writer.tags.apply", BindingID: "rest.writer.tags.apply", RequestDigest: digest, Status: "succeeded", Result: result, Receipt: result, CreatedAt: "2026-10-06T00:00:00Z", UpdatedAt: "2026-10-06T00:00:01Z", AcceptedAt: "2026-10-06T00:00:00Z", ApplyingAt: "2026-10-06T00:00:00Z", CompletedAt: "2026-10-06T00:00:01Z"}}
	return req, plan, receipt
}

func TestWriterTagsClientPlanValidation(t *testing.T) {
	for _, mode := range []string{"valid", "target", "mapping", "difference", "duplicate", "path", "schema", "extra"} {
		t.Run(mode, func(t *testing.T) {
			req, plan, _ := tagsClientFixture()
			switch mode {
			case "target":
				plan.Items[0].ObjectID = "other"
			case "mapping":
				plan.NewTagID = plan.OldTagID
			case "difference":
				plan.Items[0].After = []string{"unexpected"}
			case "duplicate":
				plan.Items = append(plan.Items, plan.Items[0])
			case "path":
				plan.Items[0].Path = "../escape.md"
			case "schema":
				plan.SchemaVersion = "unknown"
			}
			raw, _ := json.Marshal(plan)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if mode == "extra" {
				payload["secret"] = "private"
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.tags.plan", "data": payload})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic")
			_, err := client.WriterTagsPlan(t.Context(), req)
			if mode == "valid" {
				if err != nil {
					t.Fatal("valid control refused", err)
				}
			} else if err == nil {
				t.Fatal("bad plan accepted")
			}
			req.Targets = append(req.Targets, req.Targets[0])
			if _, err := client.WriterTagsPlan(t.Context(), req); err == nil || calls != 1 {
				t.Fatal("duplicate request reached transport")
			}
		})
	}
}

func TestWriterTagsClientReceiptValidation(t *testing.T) {
	for _, mode := range []string{"valid", "digest", "parent", "child", "duplicate", "false-success", "proof", "reconcile-flag", "extra"} {
		t.Run(mode, func(t *testing.T) {
			_, plan, receipt := tagsClientFixture()
			switch mode {
			case "digest":
				receipt.RequestDigest = "sha256:" + strings.Repeat("b", 64)
			case "parent":
				receipt.OperationID = "other"
			case "child", "duplicate", "false-success":
				var result WriterTagsResult
				_ = json.Unmarshal(receipt.Result, &result)
				if mode == "child" {
					result.Items[0].OperationID = "forged"
				}
				if mode == "duplicate" {
					result.Items = append(result.Items, result.Items[0])
				}
				if mode == "false-success" {
					result.Items[0].Status = "unknown"
					result.Items[0].RevisionAfter = ""
				}
				receipt.Result, _ = json.Marshal(result)
				receipt.Receipt = receipt.Result
			case "proof":
				receipt.Receipt = nil
			case "reconcile-flag":
				receipt.ReconcileRequired = true
			}
			raw, _ := json.Marshal(receipt)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if mode == "extra" {
				payload["vault_root"] = "private"
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.tags.apply", "data": payload})
			}))
			defer server.Close()
			_, err := writerClient(t, server.URL, "synthetic").WriterTagsApply(t.Context(), plan, "reviewed_proposal")
			if mode == "valid" {
				if err != nil {
					t.Fatal("valid receipt refused", err)
				}
			} else {
				var failure *Error
				if !errors.As(err, &failure) || failure.OperationRef != plan.OperationID || !failure.ReconcileRequired {
					t.Fatal("bad response lost unknown identity", err)
				}
			}
			if calls != 1 {
				t.Fatal("automatic retry")
			}
		})
	}
}
