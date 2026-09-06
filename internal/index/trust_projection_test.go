package index

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/query"
	"gorm.io/gorm"
)

type gormDB = gorm.DB

func trustTestNotes() []domain.Note {
	humanVerified := &domain.TrustSignals{Verified: []domain.TrustActorEvent{
		{By: "agent:pinax/0.9.0", At: "2026-09-01T08:00:00+00:00"},
		{By: "human:ye", At: "2026-09-06T10:30:00+00:00"},
	}, StaleAfter: "2026-12-01T00:00:00+00:00"}
	machineVerified := &domain.TrustSignals{Verified: []domain.TrustActorEvent{
		{By: "agent:pinax/0.9.0", At: "2026-09-02T08:00:00+00:00"},
	}}
	return []domain.Note{
		{ID: "note_human", Title: "Human Reviewed", Path: "notes/human.md", Body: "# human\n", Trust: humanVerified},
		{ID: "note_machine", Title: "Machine Confirmed", Path: "notes/machine.md", Body: "# machine\n", Trust: machineVerified},
		{ID: "note_legacy", Title: "Legacy", Path: "notes/legacy.md", Body: "# legacy\n"},
	}
}

func fetchNoteRecordByPath(t *testing.T, root, path string) NoteRecord {
	t.Helper()
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	q := queryUse(db)
	rows, err := q.NoteRecord.WithContext(context.Background()).Where(q.NoteRecord.Path.Eq(path)).Find()
	if err != nil {
		t.Fatalf("query note record: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("note record rows for %s = %d, want 1", path, len(rows))
	}
	return *rows[0]
}

func queryUse(db *gormDB) *query.Query {
	return query.Use(db)
}

func TestTrustProjectionColumnsRebuildAndRefresh(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	notes := trustTestNotes()
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	human := fetchNoteRecordByPath(t, root, "notes/human.md")
	if human.TrustTier != domain.TrustTierHuman {
		t.Fatalf("human record trust_tier = %q", human.TrustTier)
	}
	if human.StaleAfter != "2026-12-01T00:00:00+00:00" {
		t.Fatalf("human record stale_after = %q", human.StaleAfter)
	}
	if human.VerifiedAtLatest != "2026-09-06T10:30:00+00:00" {
		t.Fatalf("human record verified_at_latest = %q", human.VerifiedAtLatest)
	}
	machine := fetchNoteRecordByPath(t, root, "notes/machine.md")
	if machine.TrustTier != domain.TrustTierMachine || machine.StaleAfter != "" {
		t.Fatalf("machine record = %#v", machine)
	}
	legacy := fetchNoteRecordByPath(t, root, "notes/legacy.md")
	if legacy.TrustTier != domain.TrustTierUnverified || legacy.StaleAfter != "" || legacy.VerifiedAtLatest != "" {
		t.Fatalf("legacy record = %#v", legacy)
	}

	// rebuild 幂等：重复重建后派生列一致，且不改动 vault（这里只断言投影一致性）。
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild again: %v", err)
	}
	humanAgain := fetchNoteRecordByPath(t, root, "notes/human.md")
	if humanAgain != human {
		t.Fatalf("rebuild drifted: %#v vs %#v", humanAgain, human)
	}

	// refresh 路径同样维护派生列（把 legacy note 换成 human verified）。
	updated := notes[2]
	updated.Trust = notes[0].Trust
	if _, err := Refresh(root, []domain.Note{updated, notes[0], notes[1]}, RefreshOptions{}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	legacyAfter := fetchNoteRecordByPath(t, root, "notes/legacy.md")
	if legacyAfter.TrustTier != domain.TrustTierHuman || legacyAfter.VerifiedAtLatest != "2026-09-06T10:30:00+00:00" {
		t.Fatalf("refresh did not maintain trust columns: %#v", legacyAfter)
	}

	// UpdateNote 增量路径也维护派生列（清空信任字段回到 unverified）。
	cleared := updated
	cleared.Trust = nil
	if _, err := UpdateNote(root, NoteUpdate{OldPath: cleared.Path, Note: cleared}); err != nil {
		t.Fatalf("update note: %v", err)
	}
	clearedRow := fetchNoteRecordByPath(t, root, "notes/legacy.md")
	if clearedRow.TrustTier != domain.TrustTierUnverified || clearedRow.StaleAfter != "" || clearedRow.VerifiedAtLatest != "" {
		t.Fatalf("cleared trust columns = %#v", clearedRow)
	}
}
