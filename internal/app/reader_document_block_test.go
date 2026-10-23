package app

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/render"
)

func TestReaderDocumentBlockReferenceRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes/source.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_source\ntitle: Source\nstatus: active\n---\n\n[[note_target#^exact|Wiki]] [Markdown](target.md#%5Eexact) [[note_target#^dup|Duplicate]] [[note_target#^missing|Missing]] [[note_target#^code|CodeTarget]] `[[note_target#^exact|Literal]]`\n")
	targetPath := filepath.Join(root, "notes/target.md")
	source := "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\nstatus: active\n---\n\n# Target\n\nExact target body ^exact\n\nFirst ^dup\n\nSecond ^dup\n\n```text\nnot a target ^code\n```\n"
	writeAppFixture(t, targetPath, source)
	svc := NewService()
	ctx := context.Background()
	projection, err := svc.ReaderDocument(ctx, ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_source", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	refs := map[string]string{}
	var walk func([]render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, n := range nodes {
			if n.Type == "link" {
				refs[n.Text] = n.Ref
			}
			walk(n.Children)
		}
	}
	walk(page.Nodes)
	for _, label := range []string{"Wiki", "Markdown"} {
		ref, err := url.Parse(refs[label])
		if err != nil {
			t.Fatal(err)
		}
		if ref.Scheme != "pinax" || ref.Query().Get("anchor") == "" || ref.Query().Get("v") == "" {
			t.Fatalf("no fixed block ref for %s: %s", label, refs[label])
		}
		result, err := svc.ReaderDocument(ctx, ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_target", Revision: ref.Query().Get("v"), Anchor: ref.Query().Get("anchor"), CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatal(err)
		}
		target := result.Data.(map[string]any)["document"].(ReaderDocumentPage)
		if len(target.Nodes) == 0 || target.Nodes[0].Text != "Exact target body ^exact" {
			t.Fatal("block link landed elsewhere")
		}
	}
	for _, label := range []string{"Duplicate", "Missing", "CodeTarget", "Literal"} {
		if strings.HasPrefix(refs[label], "pinax:") {
			t.Fatalf("unsafe link live: %s", label)
		}
	}
	ref, _ := url.Parse(refs["Wiki"])
	if err := os.WriteFile(targetPath, []byte(source+"\nExternal change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReaderDocument(ctx, ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_target", Revision: ref.Query().Get("v"), Anchor: ref.Query().Get("anchor"), CursorSecret: readerCursorSecret()}); err == nil {
		t.Fatal("stale block reference followed new head")
	}
}
