package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterDailyHTTPContractAndRevocation(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{SessionID: "daily-http", BindingID: "daily-binding", Scopes: []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata}, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	const payload = `{"date":"2026-08-20","timezone":"Asia/Shanghai","operation_id":"daily-http-one"}`
	for _, test := range []struct {
		name, method, path, token, body string
		status                          int
	}{
		{"propose", "POST", "/v1/writer/journals/daily:propose", bearer, payload, 200},
		{"anonymous", "POST", "/v1/writer/journals/daily:propose", "", payload, 401},
		{"method", "GET", "/v1/writer/journals/daily:propose", bearer, "", 405},
		{"query", "POST", "/v1/writer/journals/daily:propose?force=true", bearer, payload, 400},
		{"extra", "POST", "/v1/writer/journals/daily:propose", bearer, strings.TrimSuffix(payload, "}") + `,"vault":"/private"}`, 400},
		{"invalid date", "POST", "/v1/writer/journals/daily:propose", bearer, strings.Replace(payload, "2026-08-20", "2026-02-30", 1), 400},
		{"invalid timezone", "POST", "/v1/writer/journals/daily:propose", bearer, strings.Replace(payload, "Asia/Shanghai", "Local", 1), 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, server.URL+test.path, strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			raw, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("status %d, want %d", response.StatusCode, test.status)
			}
			if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("response is cacheable")
			}
			if strings.Contains(string(raw), root) || strings.Contains(string(raw), bearer) {
				t.Fatal("response leaked private context")
			}
		})
	}
	status, envelope := writerPost(t, server, bearer, "/v1/writer/journals/daily:propose", payload)
	if status != 200 {
		t.Fatalf("propose=%d", status)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterDailyProposalV1Alpha1)
	proposal := writerData(t, envelope)
	apply, _ := json.Marshal(map[string]any{"proposal": proposal, "authorization": "reviewed_proposal"})
	status, envelope = writerPost(t, server, bearer, "/v1/writer/journals/daily:apply", string(apply))
	if status != 200 {
		t.Fatalf("apply=%d", status)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterApplyReceiptV1Alpha1)
	if writerData(t, envelope)["status"] != "succeeded" {
		t.Fatalf("apply did not succeed: %v", writerData(t, envelope)["status"])
	}
	status, replay := writerPost(t, server, bearer, "/v1/writer/journals/daily:apply", string(apply))
	if status != 200 || writerData(t, replay)["revision_after"] != writerData(t, envelope)["revision_after"] {
		t.Fatal("same receipt did not replay")
	}
	status, _ = writerPost(t, server, bearer, "/v1/writer/journals/daily:propose", strings.Replace(payload, "daily-http-one", "daily-http-new", 1))
	if status != 409 {
		t.Fatalf("second daily=%d", status)
	}
	sessions.Revoke("daily-http")
	status, _ = writerPost(t, server, bearer, "/v1/writer/journals/daily:apply", string(apply))
	if status != 401 {
		t.Fatal("revoked session replayed receipt")
	}
}

func TestWriterDailyHTTPRequiresAllScopes(t *testing.T) {
	t.Parallel()
	for _, scopes := range [][]string{{app.WriterScopeEditor, app.ReaderScopeBody}, {app.WriterScopeEditor, app.ReaderScopeMetadata}, {app.ReaderScopeMetadata, app.ReaderScopeBody}} {
		sessions, err := NewReaderSessionStore()
		if err != nil {
			t.Fatal(err)
		}
		bearer, err := app.GenerateReaderBearer()
		if err != nil {
			t.Fatal(err)
		}
		if err := sessions.Create(ReaderSession{SessionID: "daily-missing", BindingID: "daily-binding", Scopes: scopes, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(NewServerWithOptions(app.NewService(), t.TempDir(), ServerOptions{ReaderSessions: sessions}).Handler())
		status, _ := writerPost(t, server, bearer, "/v1/writer/journals/daily:propose", `{"date":"2026-08-20","timezone":"UTC","operation_id":"daily-missing"}`)
		server.Close()
		if status != 403 {
			t.Fatal("partial scopes granted daily write preview")
		}
	}
}
