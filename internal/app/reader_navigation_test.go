package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func navigationNote(t *testing.T, root, path, id, title, kind, tags, body string) string {
	t.Helper()
	content := fmt.Sprintf("---\nschema_version: pinax.note.v1\nnote_id: %s\ntitle: %q\nkind: %s\ntags: [%s]\nstatus: active\nupdated_at: 2026-10-05T00:00:00Z\n---\n%s", id, title, kind, tags, body)
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(path)), content)
	return content
}

func navigationContract(t *testing.T, schema string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if err := catalogschema.ValidatePayload(catalogschema.Default(), schema, raw); err != nil {
		t.Fatal(err)
	}
}

func TestReaderNavigationTagsCountsPaginationAndScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	navigationNote(t, root, "notes/a.md", "note_a", "Alpha", "reference", "research, research/go, research/go, research/go/testing, 中文/知识", "alpha")
	navigationNote(t, root, "notes/b.md", "note_b", "Beta", "reference", "research/go, research/python", "beta")
	navigationNote(t, root, ".private/hidden.md", "hidden", "Hidden", "reference", "secret, research/go", "secret body")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	req := ReaderTagsRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), Limit: 2, CursorSecret: readerCursorSecret()}
	svc := NewService()
	all := []ReaderTag{}
	first := ""
	watermark := ""
	for pageNumber := 0; pageNumber < 8; pageNumber++ {
		page, err := svc.ReaderTags(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		navigationContract(t, catalogschema.ReaderTagsV1Alpha1, page)
		if !page.Coverage.CorpusComplete || len(page.Items) > 2 {
			t.Fatal("unexpected coverage")
		}
		if watermark != "" && page.Watermark != watermark {
			t.Fatal("watermark changed while paging")
		}
		watermark = page.Watermark
		all = append(all, page.Items...)
		if first == "" {
			first = page.NextCursor
		}
		if page.NextCursor == "" {
			break
		}
		req.Cursor = page.NextCursor
	}
	if len(all) != 6 {
		t.Fatalf("tags=%d", len(all))
	}
	byPath := map[string]ReaderTag{}
	for _, tag := range all {
		byPath[tag.Path] = tag
	}
	if byPath["research"].DirectCount != 1 || byPath["research"].DescendantCount != 2 || byPath["research/go"].DirectCount != 2 || byPath["research/go"].DescendantCount != 1 {
		t.Fatal("counts must deduplicate objects independently")
	}
	if !byPath["中文"].Virtual || byPath["中文"].DirectCount != 0 || byPath["中文"].DescendantCount != 1 || byPath["中文/知识"].ParentID != byPath["中文"].TagID {
		t.Fatal("virtual ancestors or parent identity drifted")
	}
	if _, exists := byPath["secret"]; exists {
		t.Fatal("hidden taxonomy leaked")
	}
	req.Cursor = first
	req.Session.BindingID = "other"
	_, err := svc.ReaderTags(context.Background(), req)
	requireEditorSourceError(t, err, "reader_cursor_invalid")
	req.Session = readerTestSession(ReaderScopeBody)
	_, err = svc.ReaderTags(context.Background(), req)
	requireEditorSourceError(t, err, "insufficient_scope")
	req.Session = readerTestSession(ReaderScopeMetadata)
	navigationNote(t, root, "notes/b.md", "note_b", "Beta", "reference", "research/rust", "edited")
	_, err = svc.ReaderTags(context.Background(), req)
	requireEditorSourceError(t, err, "reader_cursor_expired")
	req.Cursor = ""
	page, err := svc.ReaderTags(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].TagID != byPath[page.Items[0].Path].TagID {
		t.Fatal("unchanged semantic tag identity changed")
	}
}

func TestReaderNavigationGraphClosedPagesAndHops(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	alpha := navigationNote(t, root, "notes/a.md", "a", "Alpha", "reference", "graph", "[[Beta]] [[Gamma]] [[Beta|alias]] [[Secret]] [web](https://example.invalid)")
	navigationNote(t, root, "notes/b.md", "b", "Beta", "reference", "graph", "[[Gamma]]")
	navigationNote(t, root, "notes/c.md", "c", "Gamma", "reference", "graph", "[[Delta]]")
	navigationNote(t, root, "notes/d.md", "d", "Delta", "reference", "graph", "")
	navigationNote(t, root, "notes/e.md", "e", "Isolated", "reference", "graph", "")
	navigationNote(t, root, ".hidden/secret.md", "hidden", "Secret", "reference", "hidden", "")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	req := ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", Limit: 2, CursorSecret: readerCursorSecret()}
	svc := NewService()
	nodes := map[string]bool{}
	edges := map[ReaderGraphEdge]bool{}
	firstCursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := svc.ReaderGraph(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		navigationContract(t, catalogschema.ReaderGraphV1Alpha1, page)
		if len(page.Nodes) > 2 || page.TotalNodes != 5 || page.TotalEdges != 4 || !page.Coverage.CorpusComplete {
			t.Fatal("invalid graph coverage")
		}
		local := map[string]bool{}
		for _, node := range page.Nodes {
			local[node.ObjectID] = true
			nodes[node.ObjectID] = true
		}
		for _, edge := range page.Edges {
			if !local[edge.SourceID] || !local[edge.TargetID] || edges[edge] {
				t.Fatal("dangling or repeated edge")
			}
			edges[edge] = true
		}
		if firstCursor == "" {
			firstCursor = page.NextCursor
		}
		if page.NextCursor == "" {
			break
		}
		req.Cursor = page.NextCursor
	}
	if len(nodes) != 5 || len(edges) != 4 || !nodes["e"] || nodes["hidden"] {
		t.Fatal("pagination lost edges, isolated nodes, or visibility")
	}
	req.Cursor = ""
	req.Mode = "local"
	req.ObjectID = "a"
	req.Revision = collaborationRevision(alpha)
	req.Limit = 200
	page, err := svc.ReaderGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalNodes != 3 || page.Hops != 1 {
		t.Fatal("one-hop neighborhood drift")
	}
	req.Hops = 2
	page, err = svc.ReaderGraph(context.Background(), req)
	if err != nil || page.TotalNodes != 4 {
		t.Fatal("two-hop neighborhood drift", err)
	}
	req.Cursor = firstCursor
	_, err = svc.ReaderGraph(context.Background(), req)
	requireEditorSourceError(t, err, "reader_cursor_invalid")
	req.Cursor = ""
	req.Revision = collaborationRevision("stale")
	_, err = svc.ReaderGraph(context.Background(), req)
	requireEditorSourceError(t, err, "revision_unavailable")
	req.Mode = "source"
	req.ObjectID = ""
	req.Revision = ""
	req.Hops = 0
	req.Limit = 2
	req.Cursor = firstCursor
	if err := os.Rename(filepath.Join(root, "notes/d.md"), filepath.Join(root, "notes/renamed.md")); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReaderGraph(context.Background(), req)
	requireEditorSourceError(t, err, "reader_cursor_expired")
}

func TestReaderNavigationGraphBudgetsAndDuplicateIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	links := []string{}
	for i := 0; i < 205; i++ {
		name := fmt.Sprintf("Node%03d", i)
		links = append(links, "[["+name+"]]")
		navigationNote(t, root, "notes/"+name+".md", name, name, "reference", "many", "")
	}
	navigationNote(t, root, "notes/hub.md", "hub", "Hub", "reference", "many", strings.Join(links, " "))
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	req := ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()}
	page, err := NewService().ReaderGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 200 || len(page.Edges) != 199 || page.NextCursor == "" || page.Coverage.PageComplete {
		t.Fatal("node page budget not enforced")
	}
	req.Cursor = page.NextCursor
	next, err := NewService().ReaderGraph(context.Background(), req)
	if err != nil || len(next.Nodes) != 7 || len(next.Edges) != 6 || next.NextCursor != "" {
		t.Fatal("star graph continuation lost edges", err)
	}
	navigationNote(t, root, "notes/duplicate.md", "hub", "Duplicate", "reference", "many", "")
	req.Cursor = ""
	_, err = NewService().ReaderGraph(context.Background(), req)
	requireEditorSourceError(t, err, "reader_unsupported")
}

func TestReaderNavigationMissingIndexAndScopeAreNotEmpty(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, false)
	svc := NewService()
	_, err := svc.ReaderTags(context.Background(), ReaderTagsRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), CursorSecret: readerCursorSecret()})
	requireEditorSourceError(t, err, "index_unavailable")
	_, err = svc.ReaderGraph(context.Background(), ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()})
	requireEditorSourceError(t, err, "insufficient_scope")
	if exists, _ := statIndexFile(root); exists {
		t.Fatal("navigation created an index")
	}
}

func TestReaderNavigationDailyIsReadOnlyAndTimezoneAware(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()
	req := ReaderDailyRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), Date: "2026-03-08", Timezone: "America/New_York"}
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := svc.ReaderDaily(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	navigationContract(t, catalogschema.ReaderDailyV1Alpha1, missing)
	if missing.Status != "missing" || missing.Note != nil || missing.Date != req.Date || missing.Timezone != req.Timezone {
		t.Fatal("invalid missing day projection")
	}
	after, err := os.ReadDir(root)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("missing daily lookup wrote files")
	}
	content := navigationNote(t, root, "daily/2026-03-08.md", "daily-note", "DST Day", "daily", "daily", "body")
	found, err := svc.ReaderDaily(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	navigationContract(t, catalogschema.ReaderDailyV1Alpha1, found)
	if found.Note == nil || found.Note.ObjectID != "daily-note" || found.Note.Revision != collaborationRevision(content) {
		t.Fatal("daily identity mismatch")
	}
	midnightGap := req
	midnightGap.Date, midnightGap.Timezone = "2018-11-04", "America/Sao_Paulo"
	if page, err := svc.ReaderDaily(context.Background(), midnightGap); err != nil || page.Date != midnightGap.Date {
		t.Fatal("valid calendar date rejected because midnight was skipped", err)
	}
	for _, invalid := range []ReaderDailyRequest{
		{Date: "2026-02-30", Timezone: "UTC"}, {Date: "2026-01-01", Timezone: "Local"}, {Date: "2026-01-01", Timezone: "/etc/localtime"}, {Date: "2026-01-01", Timezone: "Invalid/Zone"}, {Date: "2011-12-30", Timezone: "Pacific/Apia"},
	} {
		invalid.VaultPath = root
		invalid.Session = req.Session
		_, err := svc.ReaderDaily(context.Background(), invalid)
		requireEditorSourceError(t, err, "reader_request_invalid")
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatal("daily lookup created owner state")
	}
}

func TestReaderNavigationDailyCustomTemplateAndLegacyCollision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	template := "---\nschema_version: pinax.template.v2\nkind: journal_template\nname: journal.daily\nengine: go-template\noutput:\n  path_pattern: journal/{{ .Date }}.md\n---\n{{ .Date }}\n"
	writeAppFixture(t, filepath.Join(root, ".pinax/templates/journal.daily.md"), template)
	navigationNote(t, root, "journal/2026-10-05.md", "custom-day", "Custom Day", "daily", "daily", "not executed")
	req := ReaderDailyRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), Date: "2026-10-05", Timezone: "Asia/Shanghai"}
	page, err := NewService().ReaderDaily(context.Background(), req)
	if err != nil || page.Note == nil || page.Note.ObjectID != "custom-day" {
		t.Fatal("custom template lookup failed", err)
	}
	navigationNote(t, root, "notes/daily/2026-10-05.md", "legacy-day", "Legacy Day", "daily", "daily", "legacy")
	_, err = NewService().ReaderDaily(context.Background(), req)
	requireEditorSourceError(t, err, "reader_unsupported")
}
