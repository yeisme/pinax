package app

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/render"
)

func TestReaderDocumentSelfReferenceSnapshot(t *testing.T) {
	root := documentTestVault(t)
	path := filepath.Join(root, "notes/short.md")
	writeAppFixture(t, path, "---\nschema_version: pinax.note.v1\nnote_id: note_short\ntitle: Short\nstatus: active\n---\n\n[[#^old|Local]]\n\nOriginal block ^old\n")
	svc := NewService()
	revision := noteBodyRevision(t, root, "notes/short.md")
	if _, err := svc.VersionSnapshot(context.Background(), SnapshotRequest{VaultPath: root, Message: "before block change"}); err != nil {
		t.Fatal(err)
	}
	writeAppFixture(t, path, "---\nschema_version: pinax.note.v1\nnote_id: note_short\ntitle: Short\nstatus: active\n---\n\nNew content without old block\n")
	req := ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_short", Revision: revision, CursorSecret: readerCursorSecret()}
	projection, err := svc.ReaderDocument(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	ref := ""
	for _, node := range page.Nodes {
		for _, child := range node.Children {
			if child.Type == "link" && child.Text == "Local" {
				ref = child.Ref
			}
		}
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed == nil || parsed.Query().Get("v") != revision || parsed.Query().Get("anchor") == "" {
		t.Fatal("snapshot self link lost original identity")
	}
	req.Anchor = parsed.Query().Get("anchor")
	projection, err = svc.ReaderDocument(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	page = projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.CurrentRevision == revision || len(page.Nodes) == 0 || page.Nodes[0].Text != "Original block ^old" {
		t.Fatal("snapshot link followed current source")
	}
}

func TestReaderDocumentSelfReferencesUseServingRevision(t *testing.T) {
	source := domain.Note{ID: "note_self", Path: "notes/self.md", Title: "Self", Body: "# Original Heading\n\nOriginal block ^fixed\n\n[[#Original Heading|WikiHeading]] [[#^fixed|WikiBlock]] [MarkdownHeading](#original-heading) [MarkdownBlock](#%5Efixed) [[note_self#^fixed|ExplicitSelf]] [[#^missing|Missing]] [MissingMd](#missing) `[[#^fixed|Code]]`\n"}
	current := source
	current.Body = "# New Heading\n\nChanged block ^fixed\n"
	doc, err := render.BuildReaderDocument(source.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolveReaderNoteRefs(source, []domain.Note{current}, &doc); err != nil {
		t.Fatal(err)
	}
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
	walk(doc.Blocks)
	for _, label := range []string{"WikiHeading", "WikiBlock", "MarkdownHeading", "MarkdownBlock", "ExplicitSelf"} {
		ref, err := url.Parse(refs[label])
		if err != nil {
			t.Fatal(err)
		}
		if ref.Scheme != "pinax" || ref.Path != "/note_self" || ref.Query().Get("v") != readerNoteRevision(source) || ref.Query().Get("anchor") == "" {
			t.Fatalf("unbound self ref %s: %s", label, refs[label])
		}
		if render.LocateReaderAnchor(doc, ref.Query().Get("anchor")) < 0 {
			t.Fatal("anchor not in serving source")
		}
	}
	for _, label := range []string{"Missing", "MissingMd", "Code"} {
		if strings.HasPrefix(refs[label], "pinax:") || strings.HasPrefix(refs[label], "#") {
			t.Fatalf("unsafe self ref: %s", label)
		}
	}
}
