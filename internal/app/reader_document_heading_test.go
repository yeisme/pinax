package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/render"
)

func TestReaderDocumentHeadingTargetBudget(t *testing.T) {
	resolver := readerHeadingRefs{}
	for i := range 32 {
		note := domain.Note{ID: fmt.Sprintf("note_%d", i), Body: "# Visible\n\n```text\n# Code\n```\n"}
		ref, found, err := resolver.uri(note, "Visible")
		if err != nil || !found || !strings.Contains(ref, "anchor=visible") {
			t.Fatal("valid target refused", err)
		}
		before := resolver.sourceBytes
		for range 3 {
			if _, found, err = resolver.uri(note, "Code"); err != nil || found {
				t.Fatal("code literal became heading", err)
			}
		}
		if resolver.sourceBytes != before {
			t.Fatal("same target parsed repeatedly")
		}
	}
	if _, _, err := resolver.uri(domain.Note{ID: "overflow", Body: "# Visible"}, "Visible"); err == nil {
		t.Fatal("target count budget ignored")
	}
	resolver = readerHeadingRefs{sourceBytes: render.ReaderSourceMaxBytes - 1}
	if _, _, err := resolver.uri(domain.Note{ID: "bytes", Body: "# Visible"}, "Visible"); err == nil {
		t.Fatal("aggregate bytes budget ignored")
	}
	resolver = readerHeadingRefs{nodes: render.ReaderTotalMaxNodes - 1}
	if _, _, err := resolver.uri(domain.Note{ID: "nodes", Body: "# Visible"}, "Visible"); err == nil {
		t.Fatal("aggregate node budget ignored")
	}
	resolver = readerHeadingRefs{}
	note := domain.Note{ID: "ambiguous", Body: "# Repeated\n\n# Repeated\n\n# 章节 一\n"}
	for _, heading := range []string{"Repeated", "missing", "^block", "%zz"} {
		if _, found, err := resolver.uri(note, heading); err != nil || found {
			t.Fatal("unsupported heading resolved", heading, err)
		}
	}
	for heading, anchor := range map[string]string{"repeated-2": "repeated-2", "%E7%AB%A0%E8%8A%82%20%E4%B8%80": "章节-一"} {
		ref, found, err := resolver.uri(note, heading)
		if err != nil || !found {
			t.Fatal("explicit anchor failed", err)
		}
		parsed, _ := url.Parse(ref)
		if parsed.Query().Get("anchor") != anchor {
			t.Fatal("encoded anchor mismatch")
		}
	}
}

func TestReaderDocumentHeadingReferences(t *testing.T) {
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "source.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_source\ntitle: Source\nstatus: active\n---\n\n[[note_target#章节 一|Wiki]] [Markdown](target.md#section-two) [[note_target#missing|Missing]] [[note_target#Repeated|Ambiguous]] `[[note_target#章节 一|Code]]`\n")
	writeAppFixture(t, filepath.Join(root, "notes", "target.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\nstatus: active\n---\n\n# 章节 一\n\nFirst section.\n\n# Section Two\n\nSecond section.\n\n# Repeated\n\nOne.\n\n# Repeated\n\nTwo.\n")
	svc := NewService()
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_source", CursorSecret: readerCursorSecret()})
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
	for label, want := range map[string]string{"Wiki": "章节-一", "Markdown": "section-two"} {
		ref, err := url.Parse(refs[label])
		if err != nil {
			t.Fatal(err)
		}
		if ref.Scheme != "pinax" || ref.Path != "/note_target" || ref.Query().Get("anchor") != want || ref.Query().Get("v") == "" {
			t.Fatalf("heading was lost: %s -> %s", label, refs[label])
		}
		target, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_target", Revision: ref.Query().Get("v"), Anchor: ref.Query().Get("anchor"), CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatal(err)
		}
		result := target.Data.(map[string]any)["document"].(ReaderDocumentPage)
		if len(result.Nodes) == 0 || result.Nodes[0].Anchor != want {
			t.Fatal("link did not land on original heading")
		}
	}
	for _, label := range []string{"Missing", "Ambiguous", "Code"} {
		if strings.HasPrefix(refs[label], "pinax://") {
			t.Fatal("unresolved heading became a live note link", label)
		}
	}
}
