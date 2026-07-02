// Package index tests full rebuild and incremental index consistency.
package index

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestIncrementalMatchesFullRebuild(t *testing.T) {
	initial := []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n[[B]] [[C]]\n", Tags: []string{"start"}},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\nold body\n", Tags: []string{"keep"}},
		{ID: "note_c", Title: "C", Path: "notes/c.md", Body: "# C\nremove me\n", Tags: []string{"drop"}},
	}
	final := []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n[[B]] [[C]]\nchanged body\n", Tags: []string{"done"}},
		{ID: "note_b", Title: "B", Path: "notes/archive/b.md", Body: "# B\nold body\n", Tags: []string{"keep"}},
	}

	fullRoot := t.TempDir()
	if _, err := Rebuild(fullRoot, final); err != nil {
		t.Fatalf("full rebuild: %v", err)
	}
	incrementalRoot := t.TempDir()
	if _, err := Rebuild(incrementalRoot, initial); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	if _, err := UpdateNote(incrementalRoot, NoteUpdate{Note: final[0], ModifiedUnix: 10, Size: int64(len(final[0].Body))}); err != nil {
		t.Fatalf("incremental update a: %v", err)
	}
	if _, err := UpdateNote(incrementalRoot, NoteUpdate{OldPath: "notes/b.md", Note: final[1], ModifiedUnix: 11, Size: int64(len(final[1].Body))}); err != nil {
		t.Fatalf("incremental move b: %v", err)
	}
	if _, err := DeleteNote(incrementalRoot, NoteDelete{Path: "notes/c.md"}); err != nil {
		t.Fatalf("incremental delete c: %v", err)
	}

	if got, want := indexSnapshot(t, incrementalRoot), indexSnapshot(t, fullRoot); !reflect.DeepEqual(got, want) {
		t.Fatalf("incremental snapshot != full rebuild\ngot:  %#v\nwant: %#v", got, want)
	}
	for _, query := range []string{"changed", "old", "remove"} {
		if got, want := searchPaths(t, incrementalRoot, query), searchPaths(t, fullRoot, query); !reflect.DeepEqual(got, want) {
			t.Fatalf("search %q mismatch got=%v want=%v", query, got, want)
		}
	}
}

func TestRebuildProjectsEnhancedWikiLinkMetadata(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{
		{ID: "note_source", Title: "Source", Path: "notes/source.md", Body: "[[Alpha|Short]] [[Alpha#Details]] ![[diagram.png]] [[Meeting]]\n"},
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Body: "# Alpha\n"},
		{ID: "note_meeting_a", Title: "Meeting", Path: "notes/work/meeting-a.md", Body: "# Meeting\n"},
		{ID: "note_meeting_b", Title: "Meeting", Path: "notes/home/meeting-b.md", Body: "# Meeting\n"},
	}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	links := linksForNote(t, root, "notes/source.md")
	if len(links) != 3 {
		t.Fatalf("expected 3 note links, got %d: %#v", len(links), links)
	}
	byRaw := map[string]LinkRecord{}
	for _, link := range links {
		byRaw[link.TargetRaw] = link
	}
	alias := byRaw["Alpha|Short"]
	if alias.Status != string(domain.LinkStatusResolved) || alias.TargetAlias != "Short" || alias.TargetHeading != "" || alias.TargetPath != "notes/alpha.md" || alias.TargetNoteID != "note_alpha" {
		t.Fatalf("alias link not enhanced/resolved: %#v", alias)
	}
	heading := byRaw["Alpha#Details"]
	if heading.Status != string(domain.LinkStatusResolved) || heading.TargetHeading != "Details" || heading.TargetAlias != "" || heading.TargetPath != "notes/alpha.md" {
		t.Fatalf("heading link not enhanced/resolved: %#v", heading)
	}
	ambiguous := byRaw["Meeting"]
	if ambiguous.Status != string(domain.LinkStatusAmbiguous) || ambiguous.Broken || ambiguous.TargetPath != "" || !strings.Contains(ambiguous.Evidence, "ambiguous") {
		t.Fatalf("ambiguous link not preserved for review: %#v", ambiguous)
	}
	if _, ok := byRaw["diagram.png"]; ok {
		t.Fatalf("media embed should not be projected as note link: %#v", links)
	}
}

func TestSearchUsesTokenIndexForBodyMatches(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Body: "searchable token lives in the body", Tags: []string{"work"}},
		{ID: "note_beta", Title: "Beta", Path: "notes/beta.md", Body: "other content", Tags: []string{"work"}},
	}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := db.Model(&NoteTextRecord{}).Where("note_path = ?", "notes/alpha.md").Update("body_text", "").Error; err != nil {
		t.Fatalf("clear body text projection: %v", err)
	}

	result, err := Search(root, SearchRequest{Query: "searchable"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 || result.Results[0].Note.Path != "notes/alpha.md" {
		t.Fatalf("search should use token index candidates, got %#v", result)
	}
	if !containsString(result.Results[0].MatchedFields, "body") {
		t.Fatalf("body token match should be reported, got %#v", result.Results[0].MatchedFields)
	}
}

func TestSearchUsesTokenIndexForUnicodeSubstringMatches(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{
		{ID: "note_title", Title: "认证方案", Path: "notes/title.md", Body: "OAuth login", Tags: []string{"auth"}},
		{ID: "note_body", Title: "Auth A", Path: "notes/body.md", Body: "认证 checklist", Tags: []string{"auth"}},
	}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	result, err := Search(root, SearchRequest{Query: "认证"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got, want := resultPaths(result), []string{"notes/body.md", "notes/title.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unicode substring search paths got=%v want=%v result=%#v", got, want, result)
	}
}

func TestExtractLinkRowsUsesEnhancedWikiParser(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_source", Title: "Source", Path: "notes/source.md", Body: "[[Alias Target]] [[Alpha|Short]]\n"},
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Frontmatter: map[string]string{"alias": "Alias Target"}},
	}
	rows := ExtractLinkRows(notes)
	if len(rows) != 2 {
		t.Fatalf("expected 2 link rows, got %d: %#v", len(rows), rows)
	}
	for _, row := range rows {
		if row.Values["status"].Raw != string(domain.LinkStatusResolved) || row.Values["target_path"].Raw != "notes/alpha.md" || row.Values["target_note_id"].Raw != "note_alpha" {
			t.Fatalf("row should be resolved with shared parser: %#v", row.Values)
		}
	}
	aliasRow := rows[1]
	if aliasRow.Values["target_alias"].Raw != "Short" {
		t.Fatalf("row should preserve target alias: %#v", aliasRow.Values)
	}
}

func BenchmarkIndexRebuild(b *testing.B) {
	notes := syntheticNotes(1000)
	for i := 0; i < b.N; i++ {
		root := b.TempDir()
		if _, err := Rebuild(root, notes); err != nil {
			b.Fatalf("rebuild: %v", err)
		}
	}
}

func BenchmarkIncrementalNoteUpdate(b *testing.B) {
	notes := syntheticNotes(1000)
	root := b.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		b.Fatalf("rebuild: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		note := notes[i%len(notes)]
		note.Body = note.Body + fmt.Sprintf("\nupdate %d", i)
		if _, err := UpdateNote(root, NoteUpdate{Note: note, ModifiedUnix: int64(i + 1), Size: int64(len(note.Body))}); err != nil {
			b.Fatalf("update: %v", err)
		}
	}
}

func BenchmarkBacklinks(b *testing.B) {
	notes := syntheticNotes(1000)
	root := b.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		b.Fatalf("rebuild: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		linksForBenchmark(b, root, fmt.Sprintf("notes/note-%04d.md", i%len(notes)))
	}
}

func BenchmarkSearchLinkTarget(b *testing.B) {
	notes := syntheticNotes(1000)
	root := b.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		b.Fatalf("rebuild: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Search(root, SearchRequest{LinkTarget: fmt.Sprintf("Note %04d", i%len(notes)), Limit: 20}); err != nil {
			b.Fatalf("search link target: %v", err)
		}
	}
}

func BenchmarkIndexSearchTokenCandidates(b *testing.B) {
	notes := syntheticNotes(1000)
	root := b.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		b.Fatalf("rebuild: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Search(root, SearchRequest{Query: fmt.Sprintf("body %d", i%len(notes)), Limit: 20}); err != nil {
			b.Fatalf("search token candidates: %v", err)
		}
	}
}

type snapshotRow struct {
	Kind   string
	Path   string
	Fields []string
}

func indexSnapshot(t *testing.T, root string) []snapshotRow {
	t.Helper()
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	notes := []NoteRecord{}
	if err := db.Order("path asc").Find(&notes).Error; err != nil {
		t.Fatalf("query notes: %v", err)
	}
	links := []LinkRecord{}
	if err := db.Order("note_path asc, target asc, target_path asc").Find(&links).Error; err != nil {
		t.Fatalf("query links: %v", err)
	}
	rows := make([]snapshotRow, 0, len(notes)+len(links))
	for _, note := range notes {
		rows = append(rows, snapshotRow{Kind: "note", Path: note.Path, Fields: []string{note.NoteID, note.Title, note.SourceHash}})
	}
	for _, link := range links {
		rows = append(rows, snapshotRow{Kind: "link", Path: link.NotePath, Fields: []string{link.Target, link.TargetPath, link.Status, fmt.Sprint(link.Broken)}})
	}
	return rows
}

func searchPaths(t *testing.T, root, query string) []string {
	t.Helper()
	result, err := Search(root, SearchRequest{Query: query})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	paths := make([]string, 0, len(result.Results))
	for _, item := range result.Results {
		paths = append(paths, item.Note.Path)
	}
	sort.Strings(paths)
	return paths
}

func resultPaths(result SearchResult) []string {
	paths := make([]string, 0, len(result.Results))
	for _, item := range result.Results {
		paths = append(paths, item.Note.Path)
	}
	sort.Strings(paths)
	return paths
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func syntheticNotes(count int) []domain.Note {
	notes := make([]domain.Note, 0, count)
	for i := 0; i < count; i++ {
		target := (i + 1) % count
		notes = append(notes, domain.Note{
			ID:    fmt.Sprintf("note_%04d", i),
			Title: fmt.Sprintf("Note %04d", i),
			Path:  filepath.ToSlash(filepath.Join("notes", fmt.Sprintf("note-%04d.md", i))),
			Body:  fmt.Sprintf("# Note %04d\n[[Note %04d]]\nbody %d\n", i, target, i),
			Tags:  []string{"bench"},
		})
	}
	return notes
}

func linksForBenchmark(tb testing.TB, root, targetPath string) []LinkRecord {
	tb.Helper()
	db, err := open(root)
	if err != nil {
		tb.Fatalf("open index: %v", err)
	}
	links := []LinkRecord{}
	if err := db.Where("target_path = ?", targetPath).Find(&links).Error; err != nil {
		tb.Fatalf("query backlinks: %v", err)
	}
	return links
}

func BenchmarkIndexSyncUnchangedScan(b *testing.B) {
	root := b.TempDir()
	notes := make([]domain.Note, 0, 200)
	for i := 0; i < 200; i++ {
		notes = append(notes, domain.Note{ID: fmt.Sprintf("note_%03d", i), Title: fmt.Sprintf("Note %03d", i), Path: fmt.Sprintf("notes/%03d.md", i), Body: "# Note\nbody\n", Tags: []string{"bench"}})
	}
	if _, err := Rebuild(root, notes); err != nil {
		b.Fatalf("rebuild: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Sync(root, notes); err != nil {
			b.Fatalf("sync: %v", err)
		}
	}
}
