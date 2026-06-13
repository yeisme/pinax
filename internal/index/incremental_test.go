package index

import (
	"fmt"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestHashSkip(t *testing.T) {
	root := t.TempDir()
	note := domain.Note{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n", Tags: []string{"work"}}
	if _, err := Rebuild(root, []domain.Note{note}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	result, err := UpdateNote(root, NoteUpdate{Note: note})
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if !result.Skipped || result.Parsed != 0 || result.Indexed != 0 {
		t.Fatalf("hash skip result = %#v", result)
	}
}

func TestIncrementalNoteChangedUpdatesOnlyThatProjection(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\nold body\n", Tags: []string{"old"}},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\nkeep body\n", Tags: []string{"keep"}},
	}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	changed := domain.Note{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\nchanged body links [[B]]\n", Tags: []string{"new"}}
	result, err := UpdateNote(root, NoteUpdate{Note: changed, ModifiedUnix: 10, Size: 42})
	if err != nil {
		t.Fatalf("update changed note: %v", err)
	}
	if result.Skipped || result.Parsed != 1 || result.Indexed != 1 || result.NotePath != "notes/a.md" {
		t.Fatalf("changed result = %#v", result)
	}

	search, err := Search(root, SearchRequest{Query: "changed"})
	if err != nil {
		t.Fatalf("search changed: %v", err)
	}
	if search.Total != 1 || search.Results[0].Note.Path != "notes/a.md" || search.Results[0].LinkCount != 1 {
		t.Fatalf("search changed result = %#v", search)
	}
	keep, err := Search(root, SearchRequest{Query: "keep"})
	if err != nil {
		t.Fatalf("search keep: %v", err)
	}
	if keep.Total != 1 || keep.Results[0].Note.Path != "notes/b.md" {
		t.Fatalf("unrelated note changed = %#v", keep)
	}
}

func TestNoUnrelatedScan(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatalf("init: %v", err)
	}

	result, err := UpdateNote(root, NoteUpdate{Note: domain.Note{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "only input note"}})
	if err != nil {
		t.Fatalf("update note without vault scan: %v", err)
	}
	if result.Parsed != 1 || result.Indexed != 1 {
		t.Fatalf("update without scan result = %#v", result)
	}
}

func TestAffectedLinkEdges(t *testing.T) {
	root := t.TempDir()
	if _, err := Rebuild(root, []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n[[B]]\n"},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\n"},
	}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	if _, err := UpdateNote(root, NoteUpdate{Note: domain.Note{ID: "note_b", Title: "C", Path: "notes/b.md", Body: "# C\n"}, ModifiedUnix: 1, Size: 5}); err != nil {
		t.Fatalf("retitle target: %v", err)
	}

	links := linksForNote(t, root, "notes/a.md")
	if len(links) != 1 || links[0].Status != "broken" || !links[0].Broken || links[0].TargetPath != "" {
		t.Fatalf("affected source link not reclassified: %#v", links)
	}
}

func TestMovedNoteIncremental(t *testing.T) {
	root := t.TempDir()
	if _, err := Rebuild(root, []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n[[B]]\n"},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\n"},
	}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	if _, err := UpdateNote(root, NoteUpdate{OldPath: "notes/b.md", Note: domain.Note{ID: "note_b", Title: "B", Path: "notes/archive/b.md", Body: "# B\n"}, ModifiedUnix: 2, Size: 5}); err != nil {
		t.Fatalf("move target: %v", err)
	}

	if records := noteRecords(t, root, "notes/b.md"); len(records) != 0 {
		t.Fatalf("old note projection remained: %#v", records)
	}
	links := linksForNote(t, root, "notes/a.md")
	if len(links) != 1 || links[0].Status != "resolved" || links[0].TargetPath != "notes/archive/b.md" {
		t.Fatalf("moved backlink not retargeted: %#v", links)
	}
}

func TestDeletedNoteBacklinks(t *testing.T) {
	root := t.TempDir()
	if _, err := Rebuild(root, []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n[[B]]\n"},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\n"},
	}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	result, err := DeleteNote(root, NoteDelete{Path: "notes/b.md"})
	if err != nil {
		t.Fatalf("delete target: %v", err)
	}
	if result.Indexed != 1 || result.NotePath != "notes/b.md" {
		t.Fatalf("delete result = %#v", result)
	}
	links := linksForNote(t, root, "notes/a.md")
	if len(links) != 1 || links[0].Status != "broken" || !links[0].Broken || links[0].TargetPath != "" {
		t.Fatalf("deleted backlink not broken: %#v", links)
	}
	if outgoing := linksForNote(t, root, "notes/b.md"); len(outgoing) != 0 {
		t.Fatalf("deleted note outgoing links remained: %#v", outgoing)
	}
}

func noteRecords(t *testing.T, root, path string) []NoteRecord {
	t.Helper()
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	records := []NoteRecord{}
	if err := db.Where("path = ?", path).Find(&records).Error; err != nil {
		t.Fatalf("query note records: %v", err)
	}
	return records
}

func linksForNote(t *testing.T, root, path string) []LinkRecord {
	t.Helper()
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	links := []LinkRecord{}
	if err := db.Where("note_path = ?", path).Order("id asc").Find(&links).Error; err != nil {
		t.Fatalf("query links: %v", err)
	}
	return links
}

func TestIndexSyncClassifiesCreatedChangedMovedDeletedAndSkipped(t *testing.T) {
	root := t.TempDir()
	initial := []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n"},
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\n"},
		{ID: "note_c", Title: "C", Path: "notes/c.md", Body: "# C\n"},
	}
	if _, err := Rebuild(root, initial); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	final := []domain.Note{
		initial[0],
		{ID: "note_b", Title: "B", Path: "notes/b.md", Body: "# B\nchanged\n"},
		{ID: "note_c", Title: "C", Path: "notes/archive/c.md", Body: "# C\n"},
		{ID: "note_d", Title: "D", Path: "notes/d.md", Body: "# D\n"},
	}

	result, err := Sync(root, final)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Created != 1 || result.Changed != 1 || result.Moved != 1 || result.Deleted != 0 || result.Skipped != 1 {
		t.Fatalf("sync result = %#v", result)
	}
	if records := noteRecords(t, root, "notes/c.md"); len(records) != 0 {
		t.Fatalf("old moved record remained: %#v", records)
	}
	if records := noteRecords(t, root, "notes/archive/c.md"); len(records) != 1 {
		t.Fatalf("moved record missing: %#v", records)
	}
}

func TestIndexSyncDeletesMissingRows(t *testing.T) {
	root := t.TempDir()
	if _, err := Rebuild(root, []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n"}}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	result, err := Sync(root, nil)
	if err != nil {
		t.Fatalf("sync delete: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("sync delete result = %#v", result)
	}
	if records := noteRecords(t, root, "notes/a.md"); len(records) != 0 {
		t.Fatalf("deleted record remained: %#v", records)
	}
}

func TestIndexRefreshCreatesMissingAndSkipsUnchanged(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n"}}

	created, err := Refresh(root, notes, RefreshOptions{})
	if err != nil {
		t.Fatalf("refresh missing: %v", err)
	}
	if created.Scanned != 1 || created.Indexed != 1 || created.Created != 1 || created.Skipped != 0 || created.Failed != 0 || created.IndexStatus != "fresh" || created.Batches != 1 {
		t.Fatalf("refresh missing result = %#v", created)
	}

	fresh, err := Refresh(root, notes, RefreshOptions{})
	if err != nil {
		t.Fatalf("refresh fresh: %v", err)
	}
	if fresh.Scanned != 1 || fresh.Indexed != 0 || fresh.Created != 0 || fresh.Skipped != 1 || fresh.Failed != 0 || fresh.IndexStatus != "fresh" {
		t.Fatalf("refresh fresh result = %#v", fresh)
	}
}

func BenchmarkIndexRefreshSkipsUnchanged(b *testing.B) {
	root := b.TempDir()
	notes := make([]domain.Note, 0, 200)
	for i := 0; i < 200; i++ {
		notes = append(notes, domain.Note{ID: fmt.Sprintf("note_%03d", i), Title: fmt.Sprintf("Note %03d", i), Path: fmt.Sprintf("notes/%03d.md", i), Body: "# Body\n"})
	}
	if _, err := Refresh(root, notes, RefreshOptions{}); err != nil {
		b.Fatalf("initial refresh: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := Refresh(root, notes, RefreshOptions{BatchSize: 50})
		if err != nil {
			b.Fatalf("refresh: %v", err)
		}
		if result.Skipped != len(notes) || result.Indexed != 0 || result.Batches != 4 {
			b.Fatalf("refresh result = %#v", result)
		}
	}
}
