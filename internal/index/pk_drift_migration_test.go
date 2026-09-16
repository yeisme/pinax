package index

import (
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// 旧版本索引库 note_records 以 path 为主键（v1 schema），AutoMigrate 不会重建主键。
// 增量 upsert(Save) 依赖 ON CONFLICT(object_id)，在这类库上必须自愈而不是报 SQL 错误。
func TestMigrateRebuildsLegacyPrimaryKeySoIncrementalUpdateSucceeds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyNoteRecord{}, &legacyTagRecord{}, &legacyPropertyValueRecord{}); err != nil {
		t.Fatal(err)
	}
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	if err := db.Create(&legacyNoteRecord{Path: "notes/a.md", NoteID: objectID, Title: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	note := domain.Note{ID: objectID, Title: "A v2", Path: "notes/a.md", Body: "updated body"}
	result, err := UpdateNote(root, NoteUpdate{Note: note, ModifiedUnix: 10, Size: 42})
	if err != nil {
		t.Fatalf("UpdateNote on legacy index: %v", err)
	}
	if result.Skipped {
		t.Fatalf("expected changed note to be reindexed, got skipped")
	}
	var stored NoteRecord
	if err := db.First(&stored, "path = ?", "notes/a.md").Error; err != nil {
		t.Fatal(err)
	}
	if stored.ObjectID != objectID || stored.Title != "A v2" {
		t.Fatalf("stored = %#v", stored)
	}
}

// 主键漂移自愈要留下可审计痕迹：meta 记录 + Diagnose evidence。
func TestSchemaPrimaryKeyRepairIsRecordedAndSurfaced(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyNoteRecord{}); err != nil {
		t.Fatal(err)
	}
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	if err := db.Create(&legacyNoteRecord{Path: "notes/a.md", NoteID: objectID, Title: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	repair, ok := latestSchemaPrimaryKeyRepair(db)
	if !ok {
		t.Fatal("expected schema primary key repair to be recorded")
	}
	if len(repair.Tables) != 1 || repair.Tables[0] != "note_records" || repair.At == "" {
		t.Fatalf("repair = %#v", repair)
	}
	// refresh/init 路径会在 migrate 后写入 schema 版本，这里对齐真实时序。
	now := "2026-09-16T00:00:00Z"
	if err := upsertMeta(db, "schema_version", SchemaVersion, now); err != nil {
		t.Fatal(err)
	}
	if err := upsertMeta(db, "property_schema_version", PropertySchemaVersion, now); err != nil {
		t.Fatal(err)
	}
	report, err := Diagnose(root, []domain.Note{{ID: objectID, Title: "A", Path: "notes/a.md"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, evidence := range report.Status.Evidence {
		if strings.HasPrefix(evidence, "schema_pk_repair=note_records@") {
			found = true
		}
	}
	if !found {
		t.Fatalf("evidence = %#v", report.Status.Evidence)
	}
}

// 健康索引不得误报修复痕迹。
func TestDiagnoseWithoutRepairHasNoSchemaRepairEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	notes := []domain.Note{{ID: objectID, Title: "A", Path: "notes/a.md"}}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatal(err)
	}
	report, err := Diagnose(root, notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range report.Status.Evidence {
		if strings.HasPrefix(evidence, "schema_pk_repair=") {
			t.Fatalf("unexpected repair evidence on healthy index: %#v", report.Status.Evidence)
		}
	}
}
