package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/render"
)

func composedTarget(t *testing.T, root, id, title, body string) {
	t.Helper()
	writeAppFixture(t, filepath.Join(root, "notes", id+".md"), "---\nschema_version: pinax.note.v1\nnote_id: "+id+"\ntitle: "+title+"\nstatus: active\n---\n\n"+body)
}
func TestReaderDocumentComposedDepthAndOrigins(t *testing.T) {
	svc, req := richFixture(t, "root\n\n![[note_b]]\n\n![[note_missing]]\n")
	composedTarget(t, req.VaultPath, "note_b", "B", "B text\n\n![[note_c]]\n\n![[note_rich]]\n")
	composedTarget(t, req.VaultPath, "note_c", "C", "```math\nx+y\n```\n\n![[note_d]]\n")
	composedTarget(t, req.VaultPath, "note_d", "D", "DO NOT EXPAND THIRD LEVEL")
	before := tagPlanTree(t, req.VaultPath)
	projection, err := svc.ReaderComposedDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(ReaderComposedDocumentPage)
	counts := map[string]int{}
	for _, e := range page.Embeds {
		counts[e.Status]++
		if e.Status == "expanded" && e.Target == nil {
			t.Fatal("missing provenance")
		}
		if e.Status == "depth_limit" && e.Target != nil {
			t.Fatal("third level identity leaked")
		}
	}
	if counts["expanded"] != 2 || counts["cycle"] != 1 || counts["depth_limit"] != 1 || counts["unavailable"] != 1 {
		t.Fatalf("wrong expansion: %+v", page.Embeds)
	}
	text := ""
	nodes := 0
	var walk func([]render.ReaderDocumentNode)
	walk = func(ns []render.ReaderDocumentNode) {
		for _, n := range ns {
			nodes++
			text += n.Text
			walk(n.Children)
		}
	}
	walk(page.RichDocument.Document.Nodes)
	if !strings.Contains(text, "B text") || !strings.Contains(text, "x+y") || strings.Contains(text, "DO NOT EXPAND") || nodes != len(page.Origins) {
		t.Fatalf("composition/origins mismatch %d/%d", nodes, len(page.Origins))
	}
	if len(page.RichDocument.Annotations) != 1 {
		t.Fatal("embedded math lost")
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, req.VaultPath)) {
		t.Fatal("read wrote vault")
	}
}
func TestReaderDocumentComposedSnapshotCycleUsesRootRevision(t *testing.T) {
	svc, req := richFixture(t, "old root\n\n![[note_b]]\n")
	composedTarget(t, req.VaultPath, "note_b", "B", "![[note_rich]]\n")
	if _, err := svc.VersionSnapshot(t.Context(), SnapshotRequest{VaultPath: req.VaultPath, Message: "composed original"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(req.VaultPath, "notes/rich.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "old root", "new root", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := svc.ReaderComposedDocument(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := p.Data.(ReaderComposedDocumentPage)
	if page.RichDocument.Document.CurrentRevision == req.Revision {
		t.Fatal("snapshot positive control failed")
	}
	cycle := false
	for _, e := range page.Embeds {
		if e.Status == "cycle" {
			cycle = true
			if e.Target.Revision != req.Revision {
				t.Fatal("cycle used live root identity")
			}
		}
	}
	if !cycle {
		t.Fatal("cycle missing")
	}
}

func TestReaderDocumentComposedBoundaries(t *testing.T) {
	for _, mode := range []string{"budget", "hidden", "scope", "long-block", "duplicate-title"} {
		t.Run(mode, func(t *testing.T) {
			body := "![[note_b]]\n"
			if mode == "budget" {
				body = strings.Repeat("![[note_b]]\n\n", 32)
			}
			svc, req := richFixture(t, body)
			target := "target"
			if mode == "budget" {
				target = strings.Repeat("x", 300*1024)
			}
			if mode == "long-block" {
				target = strings.Repeat("中", 110000)
			}
			composedTarget(t, req.VaultPath, "note_b", "B", target)
			if mode == "hidden" {
				if err := os.Rename(filepath.Join(req.VaultPath, "notes/note_b.md"), filepath.Join(req.VaultPath, "notes/.note_b.md")); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "scope" {
				req.Session.Scopes = []string{ReaderScopeMetadata}
			}
			if mode == "duplicate-title" {
				svc, req = richFixture(t, "![[Same]]\n")
				composedTarget(t, req.VaultPath, "note_b", "Same", "one")
				composedTarget(t, req.VaultPath, "note_c", "Same", "two")
			}
			projection, err := svc.ReaderComposedDocument(t.Context(), req)
			if mode == "budget" || mode == "scope" {
				if err == nil {
					t.Fatal("boundary accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			page := projection.Data.(ReaderComposedDocumentPage)
			if mode == "hidden" || mode == "duplicate-title" {
				if len(page.Embeds) != 1 || page.Embeds[0].Status != "unavailable" || page.Embeds[0].Target != nil {
					t.Fatal("invisible identity disclosed")
				}
				return
			}
			text := ""
			pages := 0
			for {
				pages++
				for _, n := range page.RichDocument.Document.Nodes {
					for _, o := range page.Origins {
						if o.PageAnchor == n.Anchor && o.EmbedID != "" {
							text += n.Text
						}
					}
				}
				if page.RichDocument.Document.NextCursor == "" {
					break
				}
				if pages > 6 {
					t.Fatal("loop")
				}
				req.Cursor = page.RichDocument.Document.NextCursor
				projection, err = svc.ReaderComposedDocument(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				page = projection.Data.(ReaderComposedDocumentPage)
				if len(page.Embeds) != 1 {
					t.Fatal("continuation lost source")
				}
			}
			if text != target || pages < 2 {
				t.Fatal("fragment bytes lost")
			}
		})
	}
}

func TestReaderDocumentComposedFragmentsAndPaging(t *testing.T) {
	svc, req := richFixture(t, "![[note_b#Section]]\n\n![[note_b#^block]]\n\n![[note_b#absent]]\n")
	composedTarget(t, req.VaultPath, "note_b", "B", "# Before\n\nOMIT BEFORE\n\n# Section\n\n"+strings.Repeat("selected\n\n", 650)+"# After\n\nOMIT AFTER\n\nblock text ^block\n")
	pages := 0
	all := ""
	firstCursor := ""
	for {
		projection, err := svc.ReaderComposedDocument(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		page := projection.Data.(ReaderComposedDocumentPage)
		pages++
		for _, n := range page.RichDocument.Document.Nodes {
			all += n.Text
		}
		if pages > 10 {
			t.Fatal("paging loop")
		}
		next := page.RichDocument.Document.NextCursor
		if firstCursor == "" {
			firstCursor = next
		}
		if next == "" {
			break
		}
		req.Cursor = next
	}
	if pages < 2 || strings.Contains(all, "OMIT") || !strings.Contains(all, "block text") {
		t.Fatal("fragment scope or paging lost")
	}
	req.Cursor = firstCursor
	if _, err := svc.ReaderRichDocument(t.Context(), req); err == nil {
		t.Fatal("composed cursor crossed rich")
	}
	composedTarget(t, req.VaultPath, "note_b", "B", "changed")
	if _, err := svc.ReaderComposedDocument(t.Context(), req); err == nil {
		t.Fatal("changed target reused layout cursor")
	}
}
