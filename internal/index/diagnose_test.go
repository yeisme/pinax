package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestIndexDiagnoseClassifiesStatusAndIssues(t *testing.T) {
	notes := []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n"}}

	missing, err := Diagnose(t.TempDir(), notes)
	if err != nil {
		t.Fatalf("diagnose missing: %v", err)
	}
	assertDiagnosis(t, missing, "missing", "index_missing")

	freshRoot := t.TempDir()
	if _, err := Rebuild(freshRoot, notes); err != nil {
		t.Fatalf("rebuild fresh: %v", err)
	}
	fresh, err := Diagnose(freshRoot, notes)
	if err != nil {
		t.Fatalf("diagnose fresh: %v", err)
	}
	if fresh.Status.Status != "fresh" || len(fresh.Issues) != 0 || fresh.Counts["warning"] != 0 || fresh.Counts["error"] != 0 {
		t.Fatalf("fresh diagnosis = %#v", fresh)
	}

	schemaRoot := t.TempDir()
	if _, err := Init(schemaRoot); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	schemaReport, err := Diagnose(schemaRoot, notes)
	if err != nil {
		t.Fatalf("diagnose schema: %v", err)
	}
	assertDiagnosis(t, schemaReport, "stale", "index_schema_mismatch")
	if !hasEvidence(schemaReport, "property_schema_version=missing") {
		t.Fatalf("schema diagnosis missing evidence: %#v", schemaReport)
	}
	projectionSchemaRoot := t.TempDir()
	if _, err := Rebuild(projectionSchemaRoot, notes); err != nil {
		t.Fatalf("rebuild projection schema: %v", err)
	}
	projectionDB, err := open(projectionSchemaRoot)
	if err != nil {
		t.Fatalf("open projection schema index: %v", err)
	}
	if err := projectionDB.Migrator().DropTable(&AssetLinkRecord{}); err != nil {
		t.Fatalf("drop asset link projection table: %v", err)
	}
	projectionSchemaReport, err := Diagnose(projectionSchemaRoot, notes)
	if err != nil {
		t.Fatalf("diagnose projection schema: %v", err)
	}
	assertDiagnosis(t, projectionSchemaReport, "stale", "index_schema_mismatch")
	if !hasEvidence(projectionSchemaReport, "missing_table=asset_link_records") {
		t.Fatalf("projection schema diagnosis missing evidence: %#v", projectionSchemaReport)
	}

	staleRoot := t.TempDir()
	if _, err := Rebuild(staleRoot, notes); err != nil {
		t.Fatalf("rebuild stale: %v", err)
	}
	changed := []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\nchanged\n"}}
	stale, err := Diagnose(staleRoot, changed)
	if err != nil {
		t.Fatalf("diagnose stale: %v", err)
	}
	assertDiagnosis(t, stale, "stale", "index_stale")
	if !hasEvidence(stale, "changed_note=notes/a.md") {
		t.Fatalf("stale diagnosis missing changed note evidence: %#v", stale)
	}

	partialRoot := t.TempDir()
	if _, err := Rebuild(partialRoot, notes); err != nil {
		t.Fatalf("rebuild partial: %v", err)
	}
	db, err := open(partialRoot)
	if err != nil {
		t.Fatalf("open partial index: %v", err)
	}
	if err := db.Where("note_path = ?", "notes/a.md").Delete(&NoteTextRecord{}).Error; err != nil {
		t.Fatalf("delete note text projection: %v", err)
	}
	partial, err := Diagnose(partialRoot, notes)
	if err != nil {
		t.Fatalf("diagnose partial: %v", err)
	}
	assertDiagnosis(t, partial, "partial", "index_row_consistency")
	if !hasEvidence(partial, "missing_note_text=notes/a.md") {
		t.Fatalf("partial diagnosis missing row evidence: %#v", partial)
	}

	unreadableRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unreadableRoot, ".pinax"), 0o755); err != nil {
		t.Fatalf("mkdir unreadable root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unreadableRoot, ".pinax", "index.sqlite"), []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}
	unreadable, err := Diagnose(unreadableRoot, notes)
	if err != nil {
		t.Fatalf("diagnose unreadable: %v", err)
	}
	assertDiagnosis(t, unreadable, "unreadable", "index_unreadable")
}

func assertDiagnosis(t *testing.T, report DoctorReport, status, code string) {
	t.Helper()
	if report.Status.Status != status {
		t.Fatalf("status=%q want %q report=%#v", report.Status.Status, status, report)
	}
	for _, issue := range report.Issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue %q in %#v", code, report.Issues)
}

func hasEvidence(report DoctorReport, evidence string) bool {
	for _, issue := range report.Issues {
		for _, got := range issue.Evidence {
			if got == evidence {
				return true
			}
		}
	}
	return false
}
