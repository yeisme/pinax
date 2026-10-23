package pinaxclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderDocumentRichClientRealPagesAndSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes/rich.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw := "---\nschema_version: pinax.note.v1\nnote_id: note_rich\ntitle: Rich\n---\n\n```math\nx+y\n```\n\n" + strings.Repeat("paragraph\n\n", 650) + "> [!note]+ Title\n> Context\n\n$$\n\\frac{a_1}{b_2}\n$$\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(raw))
	revision := "sha256:" + hex.EncodeToString(sum[:])
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(api.ReaderSession{SessionID: "rich-sdk", BindingID: "binding", Scopes: []string{app.ReaderScopeBody}, ExpiresAt: time.Now().Add(time.Hour)}, token); err != nil {
		t.Fatal(err)
	}
	svc := app.NewService()
	server := httptest.NewServer(api.NewServerWithOptions(svc, root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client := writerClient(t, server.URL, token)
	req := ReaderDocumentRequest{ObjectID: "note_rich", Revision: revision}
	first, err := client.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Annotations) != 1 || first.Annotations[0].Kind != "math" || first.Document.NextCursor == "" {
		t.Fatal("missing first page")
	}
	req.Cursor = first.Document.NextCursor
	second, err := client.ReaderRichDocument(t.Context(), req)
	if err != nil || !second.Document.Complete || len(second.Annotations) != 2 || second.Annotations[0].Kind != "callout" || second.Annotations[1].Kind != "math" || second.Annotations[1].Source != "\n\\frac{a_1}{b_2}\n" {
		t.Fatal("bad second page", err)
	}
	if _, err := client.ReaderDocument(t.Context(), req); err == nil {
		t.Fatal("rich cursor accepted by legacy endpoint")
	}
	req.Cursor = ""
	if _, err := svc.VersionSnapshot(t.Context(), app.SnapshotRequest{VaultPath: root, Message: "rich SDK original"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(raw, "x+y", "new math", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	req.Anchor = first.Annotations[0].Anchor
	pinned, err := client.ReaderRichDocument(t.Context(), req)
	if err != nil || pinned.Document.CurrentRevision == revision || pinned.Annotations[0].Source != "x+y\n" {
		t.Fatal("pinned source drifted", err)
	}
	sessions.Revoke("rich-sdk")
	if _, err := client.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("revoked request admitted")
	}
}

func richClientFixture() ReaderRichDocumentPage {
	digest := "sha256:" + strings.Repeat("a", 64)
	return ReaderRichDocumentPage{SchemaVersion: catalogschema.ReaderRichDocumentV1Alpha1, Document: ReaderDocumentPage{SchemaVersion: ReaderDocumentSchemaV1Alpha1, ObjectID: "note_rich", Revision: digest, CurrentRevision: digest, URI: "pinax://note/note_rich?v=" + digest, Nodes: []ReaderDocumentNode{{Type: "code", Anchor: "code-1", Language: "math", Text: "x+y\n"}}, Outline: []ReaderOutlineEntry{}, Complete: true, ContentBytes: 4, ObservedAt: "2026-10-06T00:00:00Z"}, Annotations: []ReaderRichAnnotation{{Anchor: "code-1", PageAnchors: []string{"code-1"}, Kind: "math", Source: "x+y\n"}}}
}

func TestReaderDocumentRichClientRejectsUnsafeResponses(t *testing.T) {
	for _, mode := range []string{"valid", "object", "revision", "uri", "command", "schema", "extra", "foreign-anchor", "duplicate", "byte-budget", "fence", "complete-cursor", "timestamp", "wire"} {
		t.Run(mode, func(t *testing.T) {
			page := richClientFixture()
			command := "reader.rich_document"
			switch mode {
			case "object":
				page.Document.ObjectID = "other"
			case "revision":
				page.Document.Revision = "sha256:" + strings.Repeat("b", 64)
			case "uri":
				page.Document.URI = "file:///private"
			case "command":
				command = "reader.document"
			case "schema":
				page.SchemaVersion = "unknown"
			case "foreign-anchor":
				page.Annotations[0].PageAnchors = []string{"other"}
			case "duplicate":
				page.Annotations = append(page.Annotations, page.Annotations[0])
			case "byte-budget":
				page.Annotations[0].Source = strings.Repeat("字", 23000)
			case "fence":
				page.Document.Nodes[0].Type = "fence"
				page.Document.Nodes[0].Fence = "dataview"
			case "complete-cursor":
				page.Document.NextCursor = "untrusted"
			case "timestamp":
				page.Document.ObservedAt = "bad-time"
			}
			raw, _ := json.Marshal(page)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			if mode == "extra" {
				payload["execute"] = true
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "wire" {
					_, _ = w.Write([]byte(strings.Repeat(" ", 1024*1024+1)))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": ProjectionSpecVersion, "status": "success", "command": command, "data": payload})
			}))
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL, Token: "fixture", MaxResponseBytes: 2 * 1024 * 1024})
			if err != nil {
				t.Fatal(err)
			}
			req := ReaderDocumentRequest{ObjectID: "note_rich", Revision: richClientFixture().Document.Revision}
			_, err = client.ReaderRichDocument(t.Context(), req)
			if mode == "valid" {
				if err != nil {
					t.Fatal("valid control rejected", err)
				}
			} else if err == nil {
				t.Fatal("unsafe response accepted")
			}
			req.Revision = ""
			if _, err := client.ReaderRichDocument(t.Context(), req); err == nil || calls != 1 {
				t.Fatal("invalid request reached transport")
			}
		})
	}
}
