package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderDocumentRichHTTP(t *testing.T) {
	root := readerHTTPVault(t)
	path := filepath.Join(root, "notes/alpha.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n```mermaid\ngraph TD\n A-->B\n```\n\n"+strings.Repeat("paragraph\n\n", 650)+"> [!note]+ Title\n> Callout\n\n```math\nx+y\n```\n")...)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	revision := "sha256:" + hex.EncodeToString(sum[:])
	server, sessions, meta, full := newReaderTestServer(t, root)
	route := "/v1/reader/notes/note_alpha/rich-document"
	base := url.Values{"revision": {revision}}
	get := func(token string, q url.Values) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequest("GET", server.URL+route+"?"+q.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") || response.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("unsafe response headers")
		}
		var envelope map[string]any
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, envelope
	}
	status, envelope := get(full, base)
	if status != 200 {
		t.Fatalf("rich GET %d %v", status, envelope)
	}
	validate := func(envelope map[string]any) app.ReaderRichDocumentPage {
		t.Helper()
		if envelope["command"] != "reader.rich_document" {
			t.Fatal("wrong command")
		}
		if err := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRichDocumentV1Alpha1, envelope["data"]); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(envelope["data"])
		var page app.ReaderRichDocumentPage
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := validate(envelope)
	if len(first.Annotations) != 1 || first.Annotations[0].Kind != "mermaid" || first.Document.NextCursor == "" {
		t.Fatal("unexpected first page")
	}
	base.Set("cursor", first.Document.NextCursor)
	status, envelope = get(full, base)
	if status != 200 {
		t.Fatal("rich continuation failed", status)
	}
	second := validate(envelope)
	if len(second.Annotations) != 2 || !second.Document.Complete || second.Document.Revision != revision {
		t.Fatal("annotation pagination incomplete")
	}
	if status, _ := readerDo(t, server, full, "/v1/reader/notes/note_alpha/document?"+base.Encode()); status == 200 {
		t.Fatal("rich cursor crossed old document")
	}
	if status, _ := get(meta, base); status != 403 {
		t.Fatal("metadata-only body access", status)
	}
	for _, query := range []string{"", "revision=" + revision + "&revision=" + revision, "revision=" + revision + "&object_id=note_beta", "revision=" + revision + "&extra=1", "revision=%zz", "revision=" + revision + "&anchor=%ff", "revision=" + revision + "&cursor=" + url.QueryEscape(first.Document.NextCursor) + "&anchor=code-1"} {
		if status, _ := readerDo(t, server, full, route+"?"+query); status != 400 {
			t.Fatal("invalid query accepted", status, query)
		}
	}
	if err := os.WriteFile(path, append(raw, []byte("\nnew head\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	base.Del("cursor")
	if status, _ := get(full, base); status != 409 {
		t.Fatal("old unavailable revision not rejected", status)
	}
	sessions.Revoke("sess-full")
	if status, _ := readerDo(t, server, full, route+"?"+base.Encode()); status != 401 {
		t.Fatal("revoked read accepted", status)
	}
}

func TestReaderDocumentRichHTTPWireBudget(t *testing.T) {
	root := readerHTTPVault(t)
	path := filepath.Join(root, "notes/alpha.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n"+strings.Repeat("```math\n"+strings.Repeat("<", 60000)+"\n```\n\n", 4))...)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	server, _, _, full := newReaderTestServer(t, root)
	status, body := readerDo(t, server, full, "/v1/reader/notes/note_alpha/rich-document?revision=sha256:"+hex.EncodeToString(sum[:]))
	if status != 413 || !strings.Contains(body, "document_budget_exceeded") {
		t.Fatal("wire expansion not refused", status)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["command"] != "reader.rich_document" || envelope["status"] == "success" {
		t.Fatal("budget failure looked successful")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(raw) {
		t.Fatal("budget refusal changed source")
	}
}
