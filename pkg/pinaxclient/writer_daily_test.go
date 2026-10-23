package pinaxclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestWriterDailyClientRoundTrip(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.WriterScopeEditor)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()
	preview, err := client.WriterDailyPropose(ctx, "2026-08-20", "Asia/Shanghai", "daily-client")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.WriterDailyApply(ctx, preview, "reviewed_proposal")
	if err != nil || receipt.Status != "succeeded" {
		t.Fatalf("daily client apply: %s %v", receipt.Status, err)
	}
	replay, err := client.WriterDailyApply(ctx, preview, "reviewed_proposal")
	if err != nil || replay.RevisionAfter != receipt.RevisionAfter {
		t.Fatalf("daily receipt replay: %v", err)
	}
	day, err := client.ReaderDaily(ctx, "2026-08-20", "Asia/Shanghai")
	if err != nil || day.Note == nil || day.Note.ObjectID != preview.ObjectID {
		t.Fatalf("daily lookup: %v", err)
	}
}

func dailyClientPreviewFixture() WriterDailyProposal {
	return WriterDailyProposal{SchemaVersion: WriterDailyProposalSchemaV1Alpha1, OperationID: "daily-fixture", Date: "2026-08-20", Timezone: "UTC", ObjectID: "01982d84-2b48-7000-8000-0000000000c1", Path: "daily/2026-08-20.md", Title: "Daily-2026-08-20", Before: "", After: "\n# 2026-08-20\n", TemplateDigest: "sha256:" + strings.Repeat("a", 64), PreviewDigest: "sha256:" + strings.Repeat("b", 64), CreatedAt: "2026-08-20T12:00:00Z", BindingID: "fixture-binding", FrontmatterProtected: true}
}

func TestWriterDailyClientInvalidRequestsStayLocal(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	client := writerClient(t, server.URL, "synthetic-fixture-token")
	for _, values := range [][3]string{{"2026-08-20", "UTC", ""}, {"bad", "UTC", "daily"}, {"2026-08-20", "", "daily"}, {"2026-08-20", "UTC", "../daily"}} {
		if _, err := client.WriterDailyPropose(context.Background(), values[0], values[1], values[2]); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if _, err := client.WriterDailyApply(context.Background(), dailyClientPreviewFixture(), "unreviewed"); err == nil {
		t.Fatal("invalid authorization accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached network")
	}
}

func TestWriterDailyClientInvalidAndLostReceiptPreserveIdentity(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"invalid-schema", "wrong-identity", "lost"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if mode == "lost" {
					w.WriteHeader(503)
					return
				}
				data := map[string]any{"schema_version": WriterApplyReceiptSchemaV1Alpha1, "operation_id": "other", "capability_id": "writer.daily.apply", "binding_id": "rest.writer.daily.apply", "status": "succeeded", "retryable": false, "replay_safe": false, "reconcile_required": false, "created_at": "2026-08-20T12:00:00Z", "updated_at": "2026-08-20T12:00:00Z", "accepted_at": "2026-08-20T12:00:00Z"}
				if mode == "invalid-schema" {
					data["private"] = true
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.daily.apply", "data": data})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic-fixture-token")
			_, err := client.WriterDailyApply(context.Background(), dailyClientPreviewFixture(), "reviewed_proposal")
			var failure *Error
			if !errors.As(err, &failure) || failure.OperationRef != "daily-fixture" || !failure.ReconcileRequired {
				t.Fatalf("ambiguous receipt lost original identity: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatal("mutation was retried automatically")
			}
		})
	}
}
