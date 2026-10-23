package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderNavigationDailyDocumentWireBudget(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"small", "```text\n" + strings.Repeat("\x01", 100) + "\n```\n", "", 200},
		{"wire_expansion", "```text\n" + strings.Repeat("\x01", 200000) + "\n```\n", "document_budget_exceeded", 413},
		{"source_budget", strings.Repeat("a", catalogschema.ReaderDocumentSourceMaxBytes+1), "document_too_large", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "daily"), 0700); err != nil {
				t.Fatal(err)
			}
			source := "---\nschema_version: pinax.note.v1\nnote_id: daily-budget\ntitle: Budget\nkind: daily\nstatus: active\n---\n\n" + tc.body
			path := filepath.Join(root, "daily", "2026-08-20.md")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			svc := app.NewService()
			session := app.ReaderSessionInfo{SessionID: "daily-budget", BindingID: "daily-budget", Scopes: []string{app.ReaderScopeBody, app.ReaderScopeMetadata}}
			daily, err := svc.ReaderDaily(t.Context(), app.ReaderDailyRequest{VaultPath: root, Session: session, Date: "2026-08-20", Timezone: "UTC"})
			if err != nil || daily.Note == nil {
				t.Fatal("fixture lookup failed", err)
			}
			if tc.name == "wire_expansion" {
				projection, err := svc.ReaderDailyDocument(t.Context(), app.ReaderDailyDocumentRequest{Date: "2026-08-20", Timezone: "UTC", Document: app.ReaderDocumentRequest{VaultPath: root, Session: session, ObjectID: daily.Note.ObjectID, Revision: daily.Note.Revision, CursorSecret: []byte("synthetic-budget-cursor-key")}})
				if err != nil {
					t.Fatal("fixture hit app budget, not wire budget", err)
				}
				raw, err := json.Marshal(projection)
				if err != nil || len(raw) <= catalogschema.ReaderDocumentResponseMaxBytes {
					t.Fatal("fixture did not exceed wire budget")
				}
			}
			sessions, err := NewReaderSessionStore()
			if err != nil {
				t.Fatal(err)
			}
			bearer, err := app.GenerateReaderBearer()
			if err != nil {
				t.Fatal(err)
			}
			if err = sessions.Create(ReaderSession{SessionID: session.SessionID, BindingID: session.BindingID, Scopes: session.Scopes, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(NewServerWithOptions(svc, root, ServerOptions{ReaderSessions: sessions}).Handler())
			defer server.Close()
			query := url.Values{"date": {"2026-08-20"}, "timezone": {"UTC"}, "object_id": {daily.Note.ObjectID}, "revision": {daily.Note.Revision}}
			request, _ := http.NewRequest("GET", server.URL+"/v1/reader/journals/daily/document?"+query.Encode(), nil)
			request.Header.Set("Authorization", "Bearer "+bearer)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			raw, err := io.ReadAll(io.LimitReader(response.Body, int64(catalogschema.ReaderDocumentResponseMaxBytes)+1))
			if err != nil || len(raw) > catalogschema.ReaderDocumentResponseMaxBytes {
				t.Fatal("unbounded HTTP reply")
			}
			if response.StatusCode != tc.status || !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("budget status or cache mismatch", response.StatusCode)
			}
			if tc.status == 413 {
				var envelope struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
					Facts   map[string]string `json:"facts"`
					Actions []any             `json:"actions"`
					Data    json.RawMessage   `json:"data"`
				}
				if err = json.Unmarshal(raw, &envelope); err != nil {
					t.Fatal(err)
				}
				size, err := strconv.Atoi(envelope.Facts["source_size_bytes"])
				if envelope.Error.Code != tc.code || envelope.Facts["revision"] != daily.Note.Revision || len(envelope.Actions) != 0 || err != nil || size <= 0 {
					t.Fatal("budget facts or bypass action mismatch")
				}
				if strings.Contains(string(envelope.Data), "nodes") {
					t.Fatal("budget response leaked document")
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != source {
				t.Fatal("read modified source")
			}
			if _, err = os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
				t.Fatal("read initialized vault")
			}
		})
	}
}
