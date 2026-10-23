package pinaxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
)

func TestReaderNavigationDailyDocumentRealServer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "daily", "2026-08-20.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nschema_version: pinax.note.v1\nnote_id: daily-http\ntitle: Day\nkind: daily\nstatus: active\n---\n\n# Heading\n\n" + strings.Repeat("Synthetic daily paragraph.\n\n", 15000)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for name, scopes := range map[string][]string{"full": {app.ReaderScopeBody, app.ReaderScopeMetadata}, "body": {app.ReaderScopeBody}, "metadata": {app.ReaderScopeMetadata}} {
		bearer, err := app.GenerateReaderBearer()
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = bearer
		if err = sessions.Create(api.ReaderSession{SessionID: name, BindingID: "daily-binding", Scopes: scopes, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client := writerClient(t, server.URL, tokens["full"])
	daily, err := client.ReaderDaily(context.Background(), "2026-08-20", "UTC")
	if err != nil || daily.Note == nil {
		t.Fatal("daily lookup failed", err)
	}
	request := ReaderDailyDocumentRequest{Date: "2026-08-20", Timezone: "UTC", Document: ReaderDocumentRequest{ObjectID: daily.Note.ObjectID, Revision: daily.Note.Revision}}
	first, err := client.ReaderDailyDocument(context.Background(), request)
	if err != nil || first.NextCursor == "" {
		t.Fatal("daily document failed", err)
	}
	request.Document.Cursor = first.NextCursor
	second, err := client.ReaderDailyDocument(context.Background(), request)
	if err != nil || len(second.Nodes) == 0 || second.Nodes[0].Anchor == first.Nodes[0].Anchor {
		t.Fatal("daily pagination failed", err)
	}
	values := url.Values{"date": {request.Date}, "timezone": {request.Timezone}, "object_id": {daily.Note.ObjectID}, "revision": {daily.Note.Revision}}
	route := "/v1/reader/journals/daily/document?" + values.Encode()
	for _, tc := range []struct {
		name, token, method, suffix string
		status                      int
	}{
		{"full", "full", "GET", "", 200}, {"metadata", "metadata", "GET", "", 403}, {"body", "body", "GET", "", 403}, {"anonymous", "", "GET", "", 401}, {"method", "full", "POST", "", 405}, {"duplicate", "full", "GET", "&date=2026-08-20", 400}, {"unknown", "full", "GET", "&allow_system=true", 400}, {"badescape", "full", "GET", "&x=%zz", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, server.URL+route+tc.suffix, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tokens[tc.token])
			}
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != tc.status || !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("HTTP contract mismatch", response.StatusCode)
			}
		})
	}
	if err = os.WriteFile(path, []byte(content+"\nExternal change"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ReaderDailyDocument(context.Background(), request); err == nil {
		t.Fatal("stale daily revision accepted")
	}
	sessions.Revoke("full")
	if _, err = client.ReaderDailyDocument(context.Background(), request); err == nil {
		t.Fatal("revocation ignored")
	}
	if _, err = os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatal("reading daily initialized state")
	}
}

func TestReaderNavigationDailyDocumentRejectsMismatch(t *testing.T) {
	revision := "sha256:" + strings.Repeat("a", 64)
	for _, field := range []string{"valid", "object_id", "revision", "current_revision", "uri", "extra"} {
		t.Run(field, func(t *testing.T) {
			data := map[string]any{"schema_version": ReaderDocumentSchemaV1Alpha1, "object_id": "day", "revision": revision, "current_revision": revision, "uri": "pinax://note/day?v=" + revision, "nodes": []any{}, "outline": []any{}, "complete": true, "content_bytes": 0, "observed_at": "2026-08-20T00:00:00Z"}
			if field != "valid" {
				data[field] = "wrong"
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": "reader.daily.document", "summary": "Fixture", "data": map[string]any{"document": data}})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic")
			request := ReaderDailyDocumentRequest{Date: "2026-08-20", Timezone: "UTC", Document: ReaderDocumentRequest{ObjectID: "day", Revision: revision}}
			page, err := client.ReaderDailyDocument(context.Background(), request)
			if field == "valid" {
				if err != nil || page.ObjectID != "day" {
					t.Fatal("valid control rejected", err)
				}
			} else if err == nil {
				t.Fatal("unsafe response accepted")
			}
			request.Document.Revision = "latest"
			if _, err := client.ReaderDailyDocument(context.Background(), request); err == nil || calls != 1 {
				t.Fatal("invalid request reached server")
			}
		})
	}
}
