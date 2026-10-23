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

func TestWriterDailyClientLostCommittedReceipt(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.WriterScopeEditor)
	var calls atomic.Int32
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/journals/daily:apply" && calls.Add(1) == 1 {
			recorded := httptest.NewRecorder()
			server.Config.Handler.ServeHTTP(recorded, r)
			var envelope struct {
				Data WriterApplyReceipt `json:"data"`
			}
			if json.Unmarshal(recorded.Body.Bytes(), &envelope) != nil || recorded.Code != 200 || envelope.Data.Status != "succeeded" {
				t.Error("fixture did not commit before dropping receipt")
			}
			w.WriteHeader(503)
			return
		}
		server.Config.Handler.ServeHTTP(w, r)
	}))
	defer front.Close()
	client := writerClient(t, front.URL, bearer)
	ctx := context.Background()
	preview, err := client.WriterDailyPropose(ctx, "2026-08-20", "UTC", "daily-lost-receipt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WriterDailyApply(ctx, preview, "reviewed_proposal")
	var failure *Error
	if !errors.As(err, &failure) || failure.OperationRef != preview.OperationID || !failure.ReconcileRequired || calls.Load() != 1 {
		t.Fatalf("lost receipt did not stop at original identity: %v", err)
	}
	replay, err := client.WriterDailyApply(ctx, preview, "reviewed_proposal")
	if err != nil || replay.Status != "succeeded" || calls.Load() != 2 {
		t.Fatalf("explicit original replay failed: %v", err)
	}
	day, err := client.ReaderDaily(ctx, "2026-08-20", "UTC")
	if err != nil || day.Note == nil || day.Note.ObjectID != preview.ObjectID || day.Note.Revision != replay.RevisionAfter {
		t.Fatalf("replay created a second daily: %v", err)
	}
}

func TestWriterDailyClientRejectsUnsafePreview(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"path", "body", "identity", "extra"} {
		t.Run(mode, func(t *testing.T) {
			preview := dailyClientPreviewFixture()
			switch mode {
			case "path":
				preview.Path = "../daily.md"
			case "body":
				preview.After = strings.Repeat("字", 23000)
			case "identity":
				preview.OperationID = "other"
			}
			encoded, _ := json.Marshal(preview)
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			if mode == "extra" {
				payload["vault_root"] = "/not-allowed"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "writer.daily.propose", "data": payload})
			}))
			defer server.Close()
			_, err := writerClient(t, server.URL, "synthetic-fixture-token").WriterDailyPropose(context.Background(), preview.Date, preview.Timezone, "daily-fixture")
			if err == nil {
				t.Fatal("unsafe proposal reached consumer")
			}
		})
	}
}
