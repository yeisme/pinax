package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterEditorSourceHTTP(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	raw, err := os.ReadFile(filepath.Join(root, "notes/alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	revision := "sha256:" + hex.EncodeToString(digest[:])
	path := "/v1/writer/notes/note_alpha/editor-source?revision=" + url.QueryEscape(revision)
	server, editor, reader := newEditorTestServer(t, root)
	for _, test := range []struct {
		name, bearer, path, method string
		status                     int
	}{
		{"allowed", editor, path, "GET", 200},
		{"reader", reader, path, "GET", 403},
		{"anonymous", "", path, "GET", 401},
		{"invalid bearer", "not-a-secret", path, "GET", 401},
		{"method", editor, path, "POST", 405},
		{"revision required", editor, "/v1/writer/notes/note_alpha/editor-source", "GET", 400},
		{"unknown query", editor, path + "&body=true", "GET", 400},
		{"duplicate revision", editor, path + "&revision=" + url.QueryEscape(revision), "GET", 400},
		{"invalid query encoding", editor, path + "&other=%ZZ", "GET", 400},
		{"missing note", editor, strings.Replace(path, "note_alpha", "missing", 1), "GET", 404},
		{"path is not identity", editor, strings.Replace(path, "note_alpha", "notes%2Falpha.md", 1), "GET", 404},
		{"stale revision", editor, strings.Replace(path, url.QueryEscape(revision), url.QueryEscape("sha256:"+strings.Repeat("a", 64)), 1), "GET", 409},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(test.method, server.URL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+test.bearer)
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != test.status {
				t.Fatalf("status=%d, expected=%d", resp.StatusCode, test.status)
			}
			if !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("missing no-store")
			}
			if strings.Contains(string(body), root) || strings.Contains(string(body), editor) {
				t.Fatal("source response leaked credentials or root")
			}
			if test.status != 200 {
				if strings.Contains(string(body), "zq-secret-marker") {
					t.Fatal("denied source leaked body")
				}
				return
			}
			var envelope map[string]any
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatal(err)
			}
			assertWriterDataConformance(t, envelope, catalogschema.WriterEditorSourceV1Alpha1)
			data := writerData(t, envelope)
			if data["revision"] != revision || data["object_id"] != "note_alpha" || data["body"] != "\nAlpha contains zq-secret-marker body text.\n" || data["editable"] != true {
				t.Fatal("source identity or body changed")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(root, "notes/alpha.md"), append(raw, []byte(strings.Repeat("x", catalogschema.WriterMaxBodyBytes))...), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, body := readerDo(t, server, editor, path); status != 413 || strings.Contains(body, "zq-secret-marker") {
		t.Fatal("oversized source must fail bounded and read-only")
	}
}

func TestWriterEditorSourceHTTPRevocationAndScopeIntersection(t *testing.T) {
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
	if err := sessions.Create(ReaderSession{SessionID: "writer-only", BindingID: "source-fixture", Scopes: []string{app.WriterScopeEditor}, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	path := "/v1/writer/notes/note_alpha/editor-source?revision=sha256:" + strings.Repeat("a", 64)
	if status, _ := readerDo(t, server, bearer, path); status != 403 {
		t.Fatal("writer-only session gained body access")
	}
	sessions.Revoke("writer-only")
	if status, _ := readerDo(t, server, bearer, path); status != 401 {
		t.Fatal("revoked session still reads")
	}
}
