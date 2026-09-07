package sqlitedsn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

type dsnProbeRow struct {
	ID    int    `gorm:"primaryKey"`
	Value string `gorm:"size:64"`
}

func (dsnProbeRow) TableName() string { return "dsn_probe_rows" }

func pragmaString(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var value string
	if err := db.Raw("PRAGMA " + name).Scan(&value).Error; err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return value
}

// TestUnifiedDSNShape 锁定统一 DSN 的确切形态（对齐 sonora 已验证模式）。
func TestUnifiedDSNShape(t *testing.T) {
	t.Parallel()
	got := DSN(filepath.Join("vault", ".pinax", "api", "operations.sqlite"))
	want := filepath.Join("vault", ".pinax", "api", "operations.sqlite") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	if got != want {
		t.Fatalf("DSN() = %q, want %q", got, want)
	}
	ro := ReadOnlyDSN(filepath.Join("vault", ".pinax", "index.sqlite"))
	if !strings.Contains(ro, "mode=ro") || !strings.Contains(ro, "_pragma=busy_timeout(5000)") ||
		!strings.Contains(ro, "_pragma=foreign_keys(1)") || strings.Contains(ro, "journal_mode") {
		t.Fatalf("ReadOnlyDSN() = %q, want mode=ro + busy_timeout + foreign_keys without journal_mode", ro)
	}
	if !strings.HasPrefix(ro, "file:") || strings.Contains(ro, "\\") {
		t.Fatalf("ReadOnlyDSN() = %q, want file: URI with slash path", ro)
	}
}

// TestOpenAppliesPragmasAndPoolBounds 验证打开后的连接确实带上
// WAL + busy_timeout + foreign_keys 与 4/4 池界。
func TestOpenAppliesPragmasAndPoolBounds(t *testing.T) {
	t.Parallel()
	db, err := Open(filepath.Join(t.TempDir(), "probe.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	if mode := pragmaString(t, db, "journal_mode"); mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	if timeout := pragmaString(t, db, "busy_timeout"); timeout != "5000" {
		t.Fatalf("busy_timeout = %q, want 5000", timeout)
	}
	if fk := pragmaString(t, db, "foreign_keys"); fk != "1" {
		t.Fatalf("foreign_keys = %q, want 1", fk)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != MaxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, MaxOpenConns)
	}
	// busy_timeout 必须经 DSN 逐连接生效：池内新连接也要带上。
	var pooled string
	if err := db.Raw("PRAGMA busy_timeout").Scan(&pooled).Error; err != nil {
		t.Fatalf("pooled PRAGMA busy_timeout: %v", err)
	}
	if pooled != "5000" {
		t.Fatalf("pooled busy_timeout = %q, want 5000", pooled)
	}
}

// TestOpenReadOnlyReadsExistingWithoutWriting 只读连接可读既有投影、
// 拒绝写入，且不会创建不存在的数据库文件。
func TestOpenReadOnlyReadsExistingWithoutWriting(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "probe.sqlite")

	writer, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := writer.AutoMigrate(&dsnProbeRow{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := writer.Create(&dsnProbeRow{ID: 1, Value: "seed"}).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sqlWriter, _ := writer.DB()
	if err := sqlWriter.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer func() {
		sqlDB, _ := reader.DB()
		_ = sqlDB.Close()
	}()
	var row dsnProbeRow
	if err := reader.First(&row, 1).Error; err != nil {
		t.Fatalf("read via read-only connection: %v", err)
	}
	if row.Value != "seed" {
		t.Fatalf("row.Value = %q, want seed", row.Value)
	}
	if err := reader.Create(&dsnProbeRow{ID: 2, Value: "must-fail"}).Error; err == nil {
		t.Fatal("read-only connection must reject writes")
	}

	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := OpenReadOnly(missing); err == nil {
		t.Fatal("OpenReadOnly() must fail for a missing database file")
	}
}

// TestOpenRejectsMissingParentDirectory 保持打开失败可解释（不静默造文件）。
func TestOpenRejectsMissingParentDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "no-such-dir", "probe.sqlite")
	if _, err := Open(path); err == nil {
		t.Fatal("Open() must fail when parent directory does not exist")
	}
}

// TestCloseAllCheckpointsAndRemovesSidecars 验证进程退出契约：CloseAll 关闭
// 全部连接后，SQLite 把 WAL 内容 checkpoint 回主库文件并移除 -wal/-shm
// sidecar，主文件包含全部数据（不留「内容滞留 -wal、主文件近空」状态）。
// 注意：本测试操作全局连接登记表，不能与包内其他并行测试重叠，
// 因此不调用 t.Parallel（顺序测试与并行测试不会并发执行）。
func TestCloseAllCheckpointsAndRemovesSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.sqlite")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.AutoMigrate(&dsnProbeRow{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := db.Create(&dsnProbeRow{ID: i, Value: fmt.Sprintf("row-%d", i)}).Error; err != nil {
			t.Fatalf("Create(%d) error = %v", i, err)
		}
	}
	CloseAll()

	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Fatalf("sidecar must be removed after CloseAll: %s", sidecar)
		}
	}

	// 重新打开验证全部数据已落主文件。
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after CloseAll error = %v", err)
	}
	defer closeDB(t, reopened)
	var count int64
	if err := reopened.Model(&dsnProbeRow{}).Count(&count).Error; err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if count != 5 {
		t.Fatalf("row count after reopen = %d, want 5 (WAL content must be checkpointed into the main file)", count)
	}
	CloseAll()
}
