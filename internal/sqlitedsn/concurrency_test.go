package sqlitedsn

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

type concurrencyProbeRow struct {
	ID      int   `gorm:"primaryKey"`
	Value   int64 `gorm:"not null"`
	Updated int64
}

func (concurrencyProbeRow) TableName() string { return "concurrency_probe_rows" }

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

func isBusyLike(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "busy")
}

// TestReaderNotBlockedByUncommittedWriter 验证 WAL 下读者与写者并行：
// 模拟 `pinax mcp` 的连接持有一个未提交写事务时，模拟 CLI 的连接读取
// 不被阻塞（快照读立即完成），而非串行等待写者释放。
func TestReaderNotBlockedByUncommittedWriter(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "probe.sqlite")
	mcpSide, err := Open(path)
	if err != nil {
		t.Fatalf("mcp side Open() error = %v", err)
	}
	defer closeDB(t, mcpSide)
	cliSide, err := Open(path)
	if err != nil {
		t.Fatalf("cli side Open() error = %v", err)
	}
	defer closeDB(t, cliSide)
	if err := mcpSide.AutoMigrate(&concurrencyProbeRow{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := mcpSide.Create(&concurrencyProbeRow{ID: 1, Value: 1, Updated: time.Now().Unix()}).Error; err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	writerDone := make(chan error, 1)
	go func() {
		writerDone <- mcpSide.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&concurrencyProbeRow{}).Where("id = ?", 1).
				Update("value", 2).Error; err != nil {
				return err
			}
			// 写锁持有窗口：读者必须在此期间完成读取。
			time.Sleep(400 * time.Millisecond)
			return nil
		})
	}()

	// 等写事务真正拿到写锁再读。
	time.Sleep(100 * time.Millisecond)
	readDone := make(chan error, 1)
	go func() {
		var count int64
		readDone <- cliSide.Model(&concurrencyProbeRow{}).Count(&count).Error
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("concurrent read failed: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("read blocked on uncommitted writer: WAL snapshot read must complete in parallel")
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("writer transaction error = %v", err)
	}
}

// TestWriterWaitsBoundedInsteadOfBusy 验证跨连接写竞争经 busy_timeout
// 有界等待吸收，不产生 SQLITE_BUSY 失败。
func TestWriterWaitsBoundedInsteadOfBusy(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "probe.sqlite")
	mcpSide, err := Open(path)
	if err != nil {
		t.Fatalf("mcp side Open() error = %v", err)
	}
	defer closeDB(t, mcpSide)
	cliSide, err := Open(path)
	if err != nil {
		t.Fatalf("cli side Open() error = %v", err)
	}
	defer closeDB(t, cliSide)
	if err := mcpSide.AutoMigrate(&concurrencyProbeRow{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := mcpSide.Create(&concurrencyProbeRow{ID: 1, Value: 1, Updated: time.Now().Unix()}).Error; err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	holderDone := make(chan error, 1)
	go func() {
		holderDone <- mcpSide.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&concurrencyProbeRow{}).Where("id = ?", 1).
				Update("value", 2).Error; err != nil {
				return err
			}
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	err = cliSide.Model(&concurrencyProbeRow{}).Where("id = ?", 1).
		Update("value", 3).Error
	elapsed := time.Since(start)
	if err != nil {
		if isBusyLike(err) {
			t.Fatalf("contended write surfaced BUSY instead of bounded wait: %v", err)
		}
		t.Fatalf("contended write failed (expected busy_timeout bounded wait): %v", err)
	}
	// 写锁被持有了 ~200ms（300ms 窗口减去 100ms 等待），写方必须等待而非失败；
	// 总耗时远小于 5s busy_timeout 才算「有界」。
	if elapsed < 100*time.Millisecond {
		t.Fatalf("contended write returned in %v; it should have waited for the write lock", elapsed)
	}
	if elapsed > busyTimeoutMillis*time.Millisecond {
		t.Fatalf("contended write waited %v, beyond busy_timeout bound", elapsed)
	}
	if err := <-holderDone; err != nil {
		t.Fatalf("holder transaction error = %v", err)
	}
}

// TestParallelReadersWithConcurrentWriter 模拟 MCP server + CLI 并发读、
// 单写者循环写同一 vault 索引：读者并行完成、写者全部成功、无 BUSY。
func TestParallelReadersWithConcurrentWriter(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "probe.sqlite")
	mcpSide, err := Open(path)
	if err != nil {
		t.Fatalf("mcp side Open() error = %v", err)
	}
	defer closeDB(t, mcpSide)
	cliSide, err := Open(path)
	if err != nil {
		t.Fatalf("cli side Open() error = %v", err)
	}
	defer closeDB(t, cliSide)
	if err := mcpSide.AutoMigrate(&concurrencyProbeRow{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	const readerCount = 4
	const writeCount = 50
	var readerErrs []error
	var readersMu sync.Mutex
	readersDone := make(chan struct{})
	var readersWG sync.WaitGroup
	for i := 0; i < readerCount; i++ {
		readersWG.Add(1)
		go func() {
			defer readersWG.Done()
			for {
				select {
				case <-readersDone:
					return
				default:
				}
				var count int64
				if err := mcpSide.Model(&concurrencyProbeRow{}).Count(&count).Error; err != nil {
					readersMu.Lock()
					readerErrs = append(readerErrs, err)
					readersMu.Unlock()
					return
				}
			}
		}()
	}

	// 读者计数覆盖两条路径：MCP 侧连接 + CLI 侧连接并行读。
	var cliReaderErrs []error
	cliReaderDone := make(chan struct{})
	go func() {
		defer close(cliReaderDone)
		for {
			select {
			case <-readersDone:
				return
			default:
			}
			var rows []concurrencyProbeRow
			if err := cliSide.Limit(10).Find(&rows).Error; err != nil {
				readersMu.Lock()
				cliReaderErrs = append(cliReaderErrs, err)
				readersMu.Unlock()
				return
			}
		}
	}()

	deadline := time.Now().Add(20 * time.Second)
	for i := 0; i < writeCount; i++ {
		if time.Now().After(deadline) {
			t.Fatal("writer stalled beyond deadline")
		}
		err := mcpSide.Create(&concurrencyProbeRow{ID: i + 1, Value: int64(i), Updated: time.Now().Unix()}).Error
		if err != nil {
			close(readersDone)
			t.Fatalf("write %d failed: %v", i, err)
		}
		if err := cliSide.Model(&concurrencyProbeRow{}).Where("id = ?", i+1).
			Update("value", int64(i)*2).Error; err != nil {
			close(readersDone)
			t.Fatalf("cross-connection update %d failed: %v", i, err)
		}
	}
	close(readersDone)
	readersWG.Wait()
	<-cliReaderDone

	readersMu.Lock()
	defer readersMu.Unlock()
	for _, err := range append(readerErrs, cliReaderErrs...) {
		if err != nil {
			t.Fatalf("parallel reader failed: %v", err)
		}
	}

	var count int64
	if err := mcpSide.Model(&concurrencyProbeRow{}).Count(&count).Error; err != nil {
		t.Fatalf("final count error = %v", err)
	}
	if count != writeCount {
		t.Fatalf("final row count = %d, want %d", count, writeCount)
	}
}
