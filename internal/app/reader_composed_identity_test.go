package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderDocumentComposedDottedIdentity(t *testing.T) {
	for _, mode := range []string{"title", "alias", "ambiguous", "missing"} {
		t.Run(mode, func(t *testing.T) {
			svc, req := richFixture(t, "![[Version 1.2]]\n")
			if mode == "title" || mode == "ambiguous" {
				composedTarget(t, req.VaultPath, "note_version", "Version 1.2", "resolved body")
			}
			if mode == "alias" {
				writeAppFixture(t, filepath.Join(req.VaultPath, "notes/version.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_version\ntitle: Version\naliases: [Version 1.2]\nstatus: active\n---\n\nresolved body")
			}
			if mode == "ambiguous" {
				composedTarget(t, req.VaultPath, "note_other", "Version 1.2", "must not choose")
			}
			projection, err := svc.ReaderComposedDocument(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			page := projection.Data.(ReaderComposedDocumentPage)
			if len(page.Embeds) != 1 {
				t.Fatalf("suffix bypassed owner resolver: %+v", page.Embeds)
			}
			edge := page.Embeds[0]
			if mode == "ambiguous" || mode == "missing" {
				if edge.Status != "unavailable" || edge.Target != nil {
					t.Fatal("unresolved target exposed")
				}
				return
			}
			if edge.Status != "expanded" || edge.Target == nil || edge.Target.ObjectID != "note_version" {
				t.Fatalf("wrong owner identity: %+v", edge)
			}
			found := false
			for _, n := range page.RichDocument.Document.Nodes {
				found = found || strings.Contains(n.Text, "resolved body")
			}
			if !found {
				t.Fatal("target body absent")
			}
		})
	}
}
