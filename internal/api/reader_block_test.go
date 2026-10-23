package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/render"
)

func TestReaderDocumentBlockHTTPFixedRevision(t *testing.T) {
	root := readerHTTPVault(t)
	if err := appendVaultNote(t, root, "notes/alpha.md", "\n[[note_beta#^target|Block]]\n"); err != nil {
		t.Fatal(err)
	}
	if err := appendVaultNote(t, root, "notes/beta.md", "\nExact HTTP block ^target\n"); err != nil {
		t.Fatal(err)
	}
	server, sessions, meta, full := newReaderTestServer(t, root)
	status, raw := readerDo(t, server, full, "/v1/reader/notes/note_alpha/document")
	if status != http.StatusOK {
		t.Fatalf("source status=%d %s", status, raw)
	}
	var envelope struct {
		Data struct {
			Document app.ReaderDocumentPage `json:"document"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	ref := ""
	var walk func([]render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, n := range nodes {
			if n.Type == "link" && n.Text == "Block" {
				ref = n.Ref
			}
			walk(n.Children)
		}
	}
	walk(envelope.Data.Document.Nodes)
	parsed, err := url.Parse(ref)
	if err != nil || parsed == nil || parsed.Query().Get("anchor") == "" {
		t.Fatal("missing fixed block ref")
	}
	query := url.Values{"revision": {parsed.Query().Get("v")}, "anchor": {parsed.Query().Get("anchor")}}
	path := "/v1/reader/notes/note_beta/document?" + query.Encode()
	status, raw = readerDo(t, server, full, path)
	if status != http.StatusOK {
		t.Fatalf("target=%d %s", status, raw)
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Document.Nodes) == 0 || envelope.Data.Document.Nodes[0].Text != "Exact HTTP block ^target" {
		t.Fatal("wrong target block")
	}
	if status, _ := readerDo(t, server, meta, path); status != http.StatusForbidden {
		t.Fatalf("metadata bearer read=%d", status)
	}
	original, err := os.ReadFile(filepath.Join(root, "notes/beta.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes/beta.md"), append(original, []byte("\nnew head\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if status, _ := readerDo(t, server, full, path); status != http.StatusConflict {
		t.Fatalf("stale revision=%d", status)
	}
	sessions.Revoke("sess-full")
	if status, _ := readerDo(t, server, full, path); status != http.StatusUnauthorized {
		t.Fatalf("revoked=%d", status)
	}
}
