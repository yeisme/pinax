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
	schema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderDocumentComposedHTTP(t *testing.T) {
	root := readerHTTPVault(t)
	write := func(id, body string) string {
		t.Helper()
		raw := "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: " + id + "\nstatus: active\n---\n\n" + body
		if err := os.WriteFile(filepath.Join(root, "notes", id+".md"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(raw))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	rev := write("note_composed", "![[note_child]]\n")
	write("note_child", strings.Repeat("child text\n\n", 650)+"![[note_leaf]]\n")
	write("note_leaf", "```math\nx+y\n```\n\n![[note_composed]]\n")
	server, sessions, meta, full := newReaderTestServer(t, root)
	route := "/v1/reader/notes/note_composed/composed-document"
	query := url.Values{"revision": {rev}}
	get := func(q url.Values) app.ReaderComposedDocumentPage {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+route+"?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+full)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 200 {
			t.Fatalf("GET status %d", response.StatusCode)
		}
		if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") || response.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("unsafe headers")
		}
		var envelope map[string]any
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if envelope["command"] != "reader.composed_document" {
			t.Fatal("wrong command")
		}
		if err := schema.ValidatePayload(schema.Default(), schema.ReaderComposedDocumentV1Alpha1, envelope["data"]); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(envelope["data"])
		var page app.ReaderComposedDocumentPage
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := get(query)
	if len(first.Embeds) != 1 || first.Embeds[0].Target.ObjectID != "note_child" || len(first.Origins) == 0 || first.RichDocument.Document.NextCursor == "" {
		t.Fatal("missing first provenance")
	}
	query.Set("cursor", first.RichDocument.Document.NextCursor)
	second := get(query)
	if !second.RichDocument.Document.Complete || len(second.Embeds) != 3 || len(second.RichDocument.Annotations) != 1 {
		t.Fatal("missing second page lineage")
	}
	if second.Embeds[2].Status != "depth_limit" || second.Embeds[2].Target != nil {
		t.Fatal("third level expanded")
	}
	if status, _ := readerDo(t, server, full, "/v1/reader/notes/note_composed/rich-document?"+query.Encode()); status == 200 {
		t.Fatal("composed cursor crossed rich")
	}
	if status, _ := readerDo(t, server, meta, route+"?"+query.Encode()); status != 403 {
		t.Fatal("metadata body accepted", status)
	}
	for _, q := range []string{"", "revision=" + rev + "&revision=" + rev, "revision=" + rev + "&max_depth=3", "revision=" + rev + "&object_id=note_child", "revision=%zz", "revision=" + rev + "&anchor=%ff", "revision=" + rev + "&cursor=x&anchor=root-block-1"} {
		if status, _ := readerDo(t, server, full, route+"?"+q); status != 400 {
			t.Fatalf("bad query status %d", status)
		}
	}
	request, _ := http.NewRequest("POST", server.URL+route+"?revision="+rev, nil)
	request.Header.Set("Authorization", "Bearer "+full)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 405 {
		t.Fatal("mutation accepted", response.StatusCode)
	}
	write("note_child", "changed")
	if status, _ := readerDo(t, server, full, route+"?"+query.Encode()); status != 410 && status != 400 {
		t.Fatal("changed corpus cursor accepted", status)
	}
	sessions.Revoke("sess-full")
	if status, _ := readerDo(t, server, full, route+"?"+query.Encode()); status != 401 {
		t.Fatal("revoked read accepted", status)
	}
}

func TestReaderDocumentComposedHTTPBudget(t *testing.T) {
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
	status, body := readerDo(t, server, full, "/v1/reader/notes/note_alpha/composed-document?revision=sha256:"+hex.EncodeToString(sum[:]))
	if status != 413 || !strings.Contains(body, "document_budget_exceeded") || !strings.Contains(body, "reader.composed_document") {
		t.Fatal("budget response", status)
	}
}
