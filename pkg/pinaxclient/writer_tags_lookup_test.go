package pinaxclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriterTagsClientLookupStates(t *testing.T) {
	for _, mode := range []string{"accepted", "applying", "reconcile_required", "partial", "bad-command", "bad-timestamp", "bad-flag", "bad-sequence"} {
		t.Run(mode, func(t *testing.T) {
			_, plan, receipt := tagsClientFixture()
			receipt.Status = "reconcile_required"
			receipt.Receipt = nil
			receipt.CompletedAt = ""
			receipt.ReconcileRequired = true
			receipt.RequiredAction = "inspect_and_reconcile"
			receipt.Error = &WriterReceiptError{Code: "outcome_unknown", Message: "Inspect original operation"}
			command := "writer.tags.lookup"
			switch mode {
			case "accepted":
				receipt.Status = "accepted"
				receipt.Result = nil
				receipt.ApplyingAt = ""
				receipt.ReconcileRequired = false
				receipt.RequiredAction = ""
				receipt.Error = nil
			case "applying":
				receipt.Status = "applying"
				receipt.Error = nil
				receipt.Result = nil
			case "partial", "bad-sequence":
				var result WriterTagsResult
				_ = json.Unmarshal(receipt.Result, &result)
				result.Items = append(result.Items, WriterTagItemResult{ObjectID: "note_two", OperationID: tagIdentity("tag-item-", plan.OperationID+"\x00note_two"), Status: "unknown"})
				if mode == "bad-sequence" {
					result.Items[0].Status = "unknown"
					result.Items[0].RevisionAfter = ""
					result.Items[1].Status = "succeeded"
					result.Items[1].RevisionAfter = plan.PreviewDigest
				}
				receipt.Result, _ = json.Marshal(result)
			case "bad-command":
				command = "writer.tags.apply"
			case "bad-timestamp":
				receipt.CreatedAt = "not-a-time"
			case "bad-flag":
				receipt.ReconcileRequired = false
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/writer/tags:lookup" {
					t.Error("unexpected write path")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": command, "data": receipt})
			}))
			defer server.Close()
			got, err := writerClient(t, server.URL, "synthetic").WriterTagsLookup(t.Context(), plan.OperationID)
			bad := len(mode) > 4 && mode[:4] == "bad-"
			if bad {
				if err == nil {
					t.Fatal("inconsistent lookup accepted")
				}
			} else if err != nil || got.Status != receipt.Status {
				t.Fatal("valid non-completion state rejected", err)
			}
			if calls != 1 {
				t.Fatal("lookup retried")
			}
		})
	}
}
