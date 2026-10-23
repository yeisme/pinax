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
	schema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func composedClientFixture() ReaderComposedDocumentPage {
	rich := richClientFixture()
	rich.Document.Nodes[0].Anchor = "embed-1-code-1"
	rich.Annotations[0].Anchor = "embed-1-code-1"
	rich.Annotations[0].PageAnchors = []string{"embed-1-code-1"}
	return ReaderComposedDocumentPage{SchemaVersion: schema.ReaderComposedDocumentV1Alpha1, RichDocument: rich, Embeds: []ReaderEmbed{{EmbedID: "embed-1", SourceAnchor: "block-1", PageAnchors: []string{"embed-1-code-1"}, Depth: 1, Status: "expanded", Target: &ReaderEmbedTarget{ObjectID: "note_child", Revision: rich.Document.Revision}}}, Origins: []ReaderNodeOrigin{{PageAnchor: "embed-1-code-1", EmbedID: "embed-1", SourceAnchor: "code-1"}}}
}
func TestReaderDocumentComposedClientRejectsProvenance(t *testing.T) {
	for _, mode := range []string{"valid", "missing-origin", "duplicate-origin", "unknown-origin", "missing-parent", "depth", "cycle-lie", "expanded-cycle", "foreign-page", "unclaimed-node", "duplicate-embed", "root-identity", "fence", "extra", "command", "wire", "target-revision", "depth-leak", "ancestor-page-omitted", "cycle-revision"} {
		t.Run(mode, func(t *testing.T) {
			page := composedClientFixture()
			command := "reader.composed_document"
			switch mode {
			case "missing-origin":
				page.Origins = []ReaderNodeOrigin{}
			case "duplicate-origin":
				page.Origins = append(page.Origins, page.Origins[0])
			case "unknown-origin":
				page.Origins[0].EmbedID = "other"
			case "missing-parent":
				page.Embeds[0].ParentEmbedID = "absent"
				page.Embeds[0].Depth = 2
			case "depth":
				page.Embeds[0].Depth = 2
			case "cycle-lie":
				page.Embeds[0].Status = "cycle"
			case "expanded-cycle":
				page.Embeds[0].Target.ObjectID = "note_rich"
			case "foreign-page":
				page.Embeds[0].PageAnchors = []string{"absent"}
			case "unclaimed-node":
				page.RichDocument.Document.Nodes = append(page.RichDocument.Document.Nodes, ReaderDocumentNode{Type: "paragraph", Anchor: "root-other", Text: "root"})
				page.Origins = append(page.Origins, ReaderNodeOrigin{PageAnchor: "root-other", SourceAnchor: "block-other"})
				page.Embeds[0].PageAnchors = []string{"root-other"}
			case "duplicate-embed":
				page.Embeds = append(page.Embeds, page.Embeds[0])
			case "root-identity":
				page.RichDocument.Document.ObjectID = "other"
			case "fence":
				page.RichDocument.Document.Nodes[0].Type = "fence"
			case "command":
				command = "reader.rich_document"
			case "target-revision":
				page.Embeds[0].Target.Revision = "invalid"
			case "depth-leak":
				page.Embeds[0].Status = "depth_limit"
				page.Embeds[0].Depth = 3
			case "ancestor-page-omitted":
				page.RichDocument.Document.Nodes = append(page.RichDocument.Document.Nodes, ReaderDocumentNode{Type: "paragraph", Anchor: "child-block", Text: "child"})
				page.Origins = append(page.Origins, ReaderNodeOrigin{PageAnchor: "child-block", EmbedID: "embed-2", SourceAnchor: "block-2"})
				page.Embeds = append(page.Embeds, ReaderEmbed{EmbedID: "embed-2", ParentEmbedID: "embed-1", SourceAnchor: "block-3", PageAnchors: []string{"child-block"}, Depth: 2, Status: "expanded", Target: &ReaderEmbedTarget{ObjectID: "note_leaf", Revision: page.RichDocument.Document.Revision}})
			case "cycle-revision":
				page.Embeds[0].Status = "cycle"
				page.Embeds[0].Target.ObjectID = "note_rich"
				page.Embeds[0].Target.Revision = "sha256:" + strings.Repeat("b", 64)
				page.Origins[0].EmbedID = ""
				page.Origins[0].SourceAnchor = page.Embeds[0].SourceAnchor
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
			_, err = client.ReaderComposedDocument(t.Context(), req)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			req.Revision = ""
			if _, err := client.ReaderComposedDocument(t.Context(), req); err == nil || calls != 1 {
				t.Fatal("invalid request reached network")
			}
		})
	}
}

func TestReaderDocumentComposedClientRealPages(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(id, body string) string {
		t.Helper()
		raw := "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: " + id + "\n---\n\n" + body
		if err := os.WriteFile(filepath.Join(root, "notes", id+".md"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(raw))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	revision := write("note_rich", "![[note_child]]\n")
	write("note_child", strings.Repeat("child\n\n", 650)+"![[note_leaf]]\n\n![[note_rich]]\n")
	write("note_leaf", "```math\nx+y\n```\n\n![[note_four]]\n")
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(api.ReaderSession{SessionID: "composed-sdk", BindingID: "binding", Scopes: []string{app.ReaderScopeBody}, ExpiresAt: time.Now().Add(time.Hour)}, token); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client := writerClient(t, server.URL, token)
	req := ReaderDocumentRequest{ObjectID: "note_rich", Revision: revision}
	first, err := client.ReaderComposedDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.RichDocument.Document.NextCursor == "" || len(first.Embeds) != 1 {
		t.Fatal("first page absent")
	}
	req.Cursor = first.RichDocument.Document.NextCursor
	second, err := client.ReaderComposedDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !second.RichDocument.Document.Complete || len(second.Embeds) != 4 || len(second.RichDocument.Annotations) != 1 {
		t.Fatal("second page incomplete")
	}
	if _, err := client.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("cursor domain crossed")
	}
	write("note_child", "changed")
	if _, err := client.ReaderComposedDocument(t.Context(), req); err == nil {
		t.Fatal("stale target accepted")
	}
	sessions.Revoke("composed-sdk")
	req.Cursor = ""
	if _, err := client.ReaderComposedDocument(t.Context(), req); err == nil {
		t.Fatal("revoked read accepted")
	}
}
