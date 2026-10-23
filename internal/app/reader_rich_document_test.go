package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/render"
)

func richFixture(t *testing.T, body string) (*Service, ReaderDocumentRequest) {
	t.Helper()
	root := t.TempDir()
	raw := "---\nschema_version: pinax.note.v1\nnote_id: note_rich\ntitle: Rich\nstatus: active\n---\n\n" + body
	writeAppFixture(t, filepath.Join(root, "notes/rich.md"), raw)
	return NewService(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_rich", Revision: collaborationRevision(raw), CursorSecret: readerCursorSecret()}
}

func TestReaderDocumentRichServiceFixedPages(t *testing.T) {
	body := "```mermaid\ngraph TD\n A-->B\n```\n\n" + strings.Repeat("ordinary paragraph\n\n", 650) + "> [!note]+ Read me\n> Text\n\n```math\nx+y\n```\n\n```dataview\nTABLE private\n```\n"
	svc, req := richFixture(t, body)
	before := tagPlanTree(t, req.VaultPath)
	seen := map[string]string{}
	pages := 0
	for {
		projection, err := svc.ReaderRichDocument(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		page := projection.Data.(ReaderRichDocumentPage)
		pages++
		if pages > 10 || page.Document.Revision != req.Revision {
			t.Fatal("pagination identity drift")
		}
		var walk func([]render.ReaderDocumentNode)
		visible := map[string]bool{}
		walk = func(nodes []render.ReaderDocumentNode) {
			for _, n := range nodes {
				visible[n.Anchor] = true
				if n.BlockAnchor != "" {
					visible[n.BlockAnchor] = true
				}
				if n.Type == "fence" || n.Type == "fence_failed" {
					t.Fatal("rich service ran fence")
				}
				walk(n.Children)
			}
		}
		walk(page.Document.Nodes)
		for _, annotation := range page.Annotations {
			if !visible[annotation.Anchor] {
				t.Fatal("annotation belongs to another page")
			}
			if old, exists := seen[annotation.Anchor]; exists && old != annotation.Kind {
				t.Fatal("anchor annotation changed")
			}
			seen[annotation.Anchor] = annotation.Kind
		}
		raw, err := json.Marshal(projection)
		if err != nil || len(raw) > 1024*1024 {
			t.Fatal("wire budget ignored")
		}
		if page.Document.NextCursor == "" {
			if !page.Document.Complete {
				t.Fatal("final page incomplete")
			}
			break
		}
		req.Cursor = page.Document.NextCursor
	}
	if pages < 2 || len(seen) != 3 {
		t.Fatalf("rich pagination coverage pages=%d annotations=%v", pages, seen)
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.VaultPath)) {
		t.Fatal("rich read wrote vault state")
	}
}

func TestReaderDocumentRichServiceCursorIsolation(t *testing.T) {
	svc, req := richFixture(t, strings.Repeat("paragraph\n\n", 700))
	projection, err := svc.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	cursor := projection.Data.(ReaderRichDocumentPage).Document.NextCursor
	if cursor == "" {
		t.Fatal("missing cursor positive control")
	}
	req.Cursor = cursor
	if _, err := svc.ReaderRichDocument(t.Context(), req); err != nil {
		t.Fatal("same cursor rejected", err)
	}
	if _, err := svc.ReaderDocument(t.Context(), req); err == nil {
		t.Fatal("rich cursor crossed legacy protocol")
	}
	for _, mode := range []string{"session", "binding", "object", "scope", "revision", "anchor", "key"} {
		t.Run(mode, func(t *testing.T) {
			changed := req
			switch mode {
			case "session":
				changed.Session.SessionID = "other"
			case "binding":
				changed.Session.BindingID = "other"
			case "object":
				changed.ObjectID = "note_missing"
			case "scope":
				changed.Session.Scopes = []string{ReaderScopeMetadata}
			case "revision":
				changed.Revision = "sha256:" + strings.Repeat("f", 64)
			case "anchor":
				changed.Anchor = "block-1"
			case "key":
				changed.CursorSecret = nil
			}
			if _, err := svc.ReaderRichDocument(t.Context(), changed); err == nil {
				t.Fatal("invalid rich request accepted")
			}
		})
	}
	req.Cursor = ""
	legacy, err := svc.ReaderDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Cursor = legacy.Data.(map[string]any)["document"].(ReaderDocumentPage).NextCursor
	if _, err := svc.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("legacy cursor crossed rich protocol")
	}
}

func TestReaderDocumentRichServiceStructuralContinuation(t *testing.T) {
	body := "> [!tip]+ Long quote\n>\n" + strings.Repeat("> paragraph\n>\n", 700)
	svc, req := richFixture(t, body)
	pages := 0
	anchor := ""
	for {
		projection, err := svc.ReaderRichDocument(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		page := projection.Data.(ReaderRichDocumentPage)
		pages++
		if len(page.Annotations) != 1 {
			t.Fatalf("quote context lost on page %d: %#v", pages, page.Annotations)
		}
		if anchor == "" {
			anchor = page.Annotations[0].Anchor
		}
		if page.Annotations[0].Anchor != anchor {
			t.Fatal("continued quote annotation changed identity")
		}
		visible := map[string]bool{}
		var walk func([]render.ReaderDocumentNode)
		walk = func(nodes []render.ReaderDocumentNode) {
			for _, node := range nodes {
				visible[node.Anchor] = true
				walk(node.Children)
			}
		}
		walk(page.Document.Nodes)
		if len(page.Annotations[0].PageAnchors) == 0 {
			t.Fatal("missing actual page targets")
		}
		for _, target := range page.Annotations[0].PageAnchors {
			if !visible[target] {
				t.Fatal("annotation points outside current page")
			}
		}
		if page.Document.NextCursor == "" {
			break
		}
		if pages > 10 {
			t.Fatal("nonterminating continuation")
		}
		req.Cursor = page.Document.NextCursor
	}
	if pages < 2 {
		t.Fatal("fixture did not exercise continuation")
	}
}

func TestReaderDocumentRichServiceTargetChangeInvalidatesCursor(t *testing.T) {
	svc, req := richFixture(t, strings.Repeat("paragraph\n\n", 700))
	target := filepath.Join(req.VaultPath, "notes/target.md")
	writeAppFixture(t, target, "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\n---\n\nold")
	projection, err := svc.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Cursor = projection.Data.(ReaderRichDocumentPage).Document.NextCursor
	if req.Cursor == "" {
		t.Fatal("no cursor")
	}
	writeAppFixture(t, target, "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\n---\n\nnew")
	if _, err := svc.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("changed corpus continued old layout")
	}
}

func TestReaderDocumentRichServiceSnapshotAndWireBudget(t *testing.T) {
	svc, req := richFixture(t, "```math\noriginal\n```\n")
	first, err := svc.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VersionSnapshot(t.Context(), SnapshotRequest{VaultPath: req.VaultPath, Message: "rich original"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(req.VaultPath, "notes/rich.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "original", "changed", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	req.Anchor = first.Data.(ReaderRichDocumentPage).Annotations[0].Anchor
	got, err := svc.ReaderRichDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := got.Data.(ReaderRichDocumentPage)
	if page.Document.CurrentRevision == req.Revision || len(page.Annotations) != 1 || page.Annotations[0].Source != "original\n" {
		t.Fatal("rich snapshot mixed live bytes")
	}
	svc, req = richFixture(t, strings.Repeat("```math\n"+strings.Repeat("<", 60000)+"\n```\n\n", 4))
	if _, err := svc.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("JSON escaping expansion exceeded 1MiB without error")
	}
	svc, req = richFixture(t, "plain")
	req.Revision = ""
	if _, err := svc.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("unfixed revision admitted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := svc.ReaderRichDocument(ctx, req); err == nil {
		t.Fatal("cancelled read accepted")
	}
}
