package index

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/sqlitedsn"
)

func TestIndexDiagnoseClassifiesStatusAndIssues(t *testing.T) {
	t.Parallel()
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

// TestIndexDiagnoseDetectsCorruptMainFileWithHotWAL 验证统一 WAL DSN 下的
// 损坏检测：未 checkpoint 的热 -wal 可能包含全部数据页（含头页），主文件
// 被覆盖为垃圾时 SQLite 仍能读出"正常"数据；doctor 必须经主文件头校验
// 识别为不可读，而不是被热 WAL 掩盖（pinax-local-async-substrate-v1）。
func TestIndexDiagnoseDetectsCorruptMainFileWithHotWAL(t *testing.T) {
	t.Parallel()
	notes := []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n"}}
	root := t.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")

	// 头校验：主文件为垃圾字节（即使残留热 -wal/-shm）也必须判不可读。
	if err := os.WriteFile(indexPath, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}
	report, err := Diagnose(root, notes)
	if err != nil {
		t.Fatalf("diagnose corrupt: %v", err)
	}
	assertDiagnosis(t, report, "unreadable", "index_unreadable")

	// 健康索引不误伤：干净 root 重建后 fresh（头校验放行）由
	// TestIndexDiagnoseClassifiesStatusAndIssues 的 fresh 阶段覆盖。
}

func TestInspectReadOnlyNeverMigratesOrRepairs(t *testing.T) {
	t.Parallel()
	notes := []domain.Note{{ID: "note_a", Title: "A", Path: "notes/a.md", Body: "# A\n", Tags: []string{"t1"}}}
	root := t.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	// Bait the persistent repair: clear the tag projection's object
	// identity (the exact row the RW path backfills via Update), then
	// close the RW handle so the WAL is checkpointed into the main file.
	baitDB, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := baitDB.Exec(`UPDATE tag_records SET object_id = ''`).Error; err != nil {
		t.Fatal(err)
	}
	// Checkpoint so no RW sidecar remains; the read-only assertions below
	// then prove the RO inspection itself writes nothing anywhere.
	if err := baitDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		t.Fatal(err)
	}
	if sqlDB, dbErr := baitDB.DB(); dbErr == nil {
		_ = sqlDB.Close()
	}
	// The checkpointed RW session may leave empty sidecar files; remove
	// them so the assertions below prove the RO inspection creates none.
	for _, sidecar := range []string{"index.sqlite-wal", "index.sqlite-shm"} {
		_ = os.Remove(filepath.Join(root, ".pinax", sidecar))
	}
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	before := indexFileDigest(t, indexPath)
	status, err := InspectReadOnly(root, notes)
	if err != nil {
		t.Fatalf("inspect read-only: %v", err)
	}
	if status.Status != "stale" {
		t.Fatalf("baited index status = %q, want stale (identity gap detected, never repaired)", status.Status)
	}
	if after := indexFileDigest(t, indexPath); after != before {
		t.Fatal("InspectReadOnly modified the index file")
	}
	// Even empty WAL/SHM files are mutations to the source vault. SQLite
	// runtime files belong only in the disposable private diagnostic copy.
	for _, sidecar := range []string{"index.sqlite-wal", "index.sqlite-shm"} {
		if _, err := os.Lstat(filepath.Join(root, ".pinax", sidecar)); !os.IsNotExist(err) {
			t.Fatal("read-only inspection created a source sidecar")
		}
	}
	// Healthy index: fresh status, file untouched.
	healthy := t.TempDir()
	if _, err := Rebuild(healthy, notes); err != nil {
		t.Fatal(err)
	}
	healthyPath := filepath.Join(healthy, ".pinax", "index.sqlite")
	healthyBefore := indexFileDigest(t, healthyPath)
	healthyStatus, err := InspectReadOnly(healthy, notes)
	if err != nil || healthyStatus.Status != "fresh" {
		t.Fatalf("healthy status = %#v err = %v", healthyStatus, err)
	}
	if healthyAfter := indexFileDigest(t, healthyPath); healthyAfter != healthyBefore {
		t.Fatal("InspectReadOnly modified the healthy index file")
	}
}

func indexFileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestInspectReadOnlyReportsPartialAndReleasesHandles(t *testing.T) {
	// Sequential on purpose: the registry-count assertion needs an
	// exclusive measurement window (parallel tests open their own pools
	// into the same process-global registry).
	notes := []domain.Note{{ID: "note_p", Title: "P", Path: "notes/p.md", Body: "# P\n"}}
	root := t.TempDir()
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatal(err)
	}
	// Build the real partial shape: note record present, text projection
	// missing (index_row_consistency, no stale codes).
	if err := RemoveNoteTextForTest(root, "notes/p.md"); err != nil {
		t.Fatalf("remove text: %v", err)
	}
	status, err := InspectReadOnly(root, notes)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if status.Status != "partial" {
		t.Fatalf("partial shape status = %q, want partial; evidence = %#v", status.Status, status.Evidence)
	}
	// Repeated inspections must not accumulate handles on the index file:
	// count only /proc/self/fd entries pointing at *.sqlite so unrelated
	// parallel-test fds never skew the measurement.
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("/proc/self/fd unavailable")
	}
	target := filepath.Join(root, ".pinax", "index.sqlite")
	countIndexFds := func() int {
		entries, readErr := os.ReadDir("/proc/self/fd")
		if readErr != nil {
			t.Fatal(readErr)
		}
		count := 0
		for _, entry := range entries {
			link, linkErr := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
			if linkErr == nil && (link == target || strings.HasPrefix(link, target+"-")) {
				count++
			}
		}
		return count
	}
	// Warm up once, then measure across many calls.
	if _, err := InspectReadOnly(root, notes); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	before := countIndexFds()
	beforePools := sqlitedsn.OpenPoolCount()
	for i := 0; i < 50; i++ {
		if _, err := InspectReadOnly(root, notes); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	after := countIndexFds()
	if after > before {
		t.Fatalf("inspections leaked index file handles: %d -> %d", before, after)
	}
	// Closed pools must also leave the process exit registry: retained
	// pool objects would grow with every request in a long session.
	if afterPools := sqlitedsn.OpenPoolCount(); afterPools > beforePools {
		t.Fatalf("inspections accumulated registry entries: %d -> %d", beforePools, afterPools)
	}
}
