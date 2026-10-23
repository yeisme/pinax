package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/sqlitedsn"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderNavigationDenseGraphEdgeBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 24; i++ {
		links := make([]string, 0, 24)
		for j := 0; j < 24; j++ {
			links = append(links, fmt.Sprintf("[[Dense%02d]]", j))
		}
		name := fmt.Sprintf("Dense%02d", i)
		navigationNote(t, root, "notes/"+name+".md", name, name, "reference", "dense", strings.Join(links, " "))
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	req := ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()}
	first, err := NewService().ReaderGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Edges) != 400 || first.NextCursor == "" || first.TotalEdges != 576 {
		t.Fatal("dense edge budget or total drifted")
	}
	req.Cursor = first.NextCursor
	second, err := NewService().ReaderGraph(context.Background(), req)
	if err != nil || len(second.Edges) != 176 || second.NextCursor != "" {
		t.Fatal("dense graph continuation lost edges", err)
	}
	seen := map[ReaderGraphEdge]bool{}
	for _, page := range []ReaderGraphPage{first, second} {
		navigationContract(t, catalogschema.ReaderGraphV1Alpha1, page)
		for _, edge := range page.Edges {
			if seen[edge] {
				t.Fatal("edge repeated between pages")
			}
			seen[edge] = true
		}
	}
}

func TestReaderNavigationOversizedSourcesDeclareIncompleteCoverage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Keep the index below its independent snapshot budget, then grow the
	// source as an external editor would. A readable stale index is allowed;
	// incomplete source coverage must still be explicit.
	navigationNote(t, root, "notes/large.md", "large", "Large", "reference", "large", "Small indexed fixture")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	navigationNote(t, root, "notes/large.md", "large", "Large", "reference", "large", strings.Repeat("x", catalogschema.ReaderDocumentSourceMaxBytes+1))
	svc := NewService()
	graph, err := svc.ReaderGraph(context.Background(), ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	if graph.Coverage.CorpusComplete || !graph.Coverage.PageComplete || graph.TotalNodes != 1 {
		t.Fatal("oversized source was reported as complete graph")
	}
	tags, err := svc.ReaderTags(context.Background(), ReaderTagsRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), CursorSecret: readerCursorSecret()})
	if err != nil || tags.Coverage.CorpusComplete || len(tags.Items) != 1 {
		t.Fatal("oversized metadata coverage was not explicit", err)
	}
}

func TestReaderNavigationIndexSnapshotBudgetRefusesSourceWithoutWriting(t *testing.T) {
	root := t.TempDir()
	navigationNote(t, root, "notes/small.md", "small", "Small", "reference", "small", "Small indexed source")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".pinax", "index.sqlite")
	// Sparse disposable fixture: exceed the snapshot budget deterministically
	// without depending on SQLite's text/FTS amplification or WAL timing.
	if err := os.Truncate(path, sqlitedsn.ReadSnapshotLimit+1); err != nil {
		t.Fatal(err)
	}
	type observation struct {
		size     int64
		modified int64
	}
	observe := func() map[string]observation {
		result := map[string]observation{}
		for _, name := range []string{"index.sqlite", "index.sqlite-wal", "index.sqlite-shm"} {
			info, err := os.Stat(filepath.Join(root, ".pinax", name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			result[name] = observation{info.Size(), info.ModTime().UnixNano()}
		}
		return result
	}
	before := observe()
	_, err := NewService().ReaderGraph(context.Background(), ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()})
	requireEditorSourceError(t, err, "index_unavailable")
	_, err = NewService().ReaderTags(context.Background(), ReaderTagsRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), CursorSecret: readerCursorSecret()})
	requireEditorSourceError(t, err, "index_unavailable")
	if !reflect.DeepEqual(before, observe()) {
		t.Fatal("budget refusal mutated index files")
	}
}

func TestReaderNavigationInvisibleCountsAndUnchangedOwnerState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	navigationNote(t, root, "notes/visible.md", "visible", "Visible", "reference", "visible", "[[Secret]]")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".pinax/index.sqlite")
	before, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	bad := navigationNote(t, root, "notes/bad.md", "bad", "Secret", "reference", "secret", "body")
	bad = strings.Replace(bad, "status: active", "status: active\nstale_after: invalid-private-value", 1)
	writeAppFixture(t, filepath.Join(root, "notes/bad.md"), bad)
	navigationNote(t, root, ".hidden/private.md", "hidden", "Hidden", "reference", "hidden", "body")
	outside := t.TempDir()
	navigationNote(t, outside, "secret.md", "outside", "Outside", "reference", "secret", "outside-body")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "notes/link.md")); err != nil {
		t.Skip("symlink permission unavailable")
	}
	svc := NewService()
	tags, err := svc.ReaderTags(context.Background(), ReaderTagsRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), CursorSecret: readerCursorSecret()})
	if err != nil || len(tags.Items) != 1 || tags.Items[0].Path != "visible" || tags.Items[0].DirectCount != 1 {
		t.Fatal("invisible tags affected counts", err)
	}
	graph, err := svc.ReaderGraph(context.Background(), ReaderGraphRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeRelations), Mode: "source", CursorSecret: readerCursorSecret()})
	if err != nil || len(graph.Nodes) != 1 || len(graph.Edges) != 0 {
		t.Fatal("invisible graph objects leaked", err)
	}
	after, err := os.Stat(indexPath)
	if err != nil || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatal("navigation modified the index")
	}
}

func TestReaderNavigationDailyTemplateAndNoteSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	body := navigationNote(t, outside, "2026-10-05.md", "outside-day", "Outside Day", "daily", "daily", "secret")
	if err := os.MkdirAll(filepath.Join(root, "daily"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "2026-10-05.md"), filepath.Join(root, "daily/2026-10-05.md")); err != nil {
		t.Skip("symlink permission unavailable")
	}
	req := ReaderDailyRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), Date: "2026-10-05", Timezone: "UTC"}
	page, err := NewService().ReaderDaily(context.Background(), req)
	if err != nil || page.Status != "missing" || page.Note != nil {
		t.Fatal("symlinked daily became visible", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".pinax/templates"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "2026-10-05.md"), filepath.Join(root, ".pinax/templates/journal.daily.md")); err != nil {
		t.Fatal(err)
	}
	_, err = NewService().ReaderDaily(context.Background(), req)
	requireEditorSourceError(t, err, "reader_unsupported")
	unchanged, err := os.ReadFile(filepath.Join(outside, "2026-10-05.md"))
	if err != nil || string(unchanged) != body {
		t.Fatal("readonly lookup changed external file")
	}
}
