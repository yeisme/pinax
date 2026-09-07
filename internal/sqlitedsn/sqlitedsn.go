// Package sqlitedsn 提供 Pinax 所有 SQLite 打开点的统一 DSN 与连接池界。
//
// 规则（pinax-local-async-substrate-v1）：operation、promptasset、agentmemory、
// memory ledger 等所有 SQLite 打开点必须经本包打开，禁止裸 `sqlite.Open(path)`
// 直连（守卫测试 internal/sqlitedsn/guard_test.go 强制）。模式对齐 sonora
// internal/store 已验证行为：WAL（N 读者 + 1 写者），busy_timeout 经 DSN
// pragma 逐连接下发（池化下 db.Exec 只会打到池中一个连接），写经 busy_timeout
// 串行，保持单写者语义，不引入跨进程锁。
//
// 进程退出契约：本包登记所有打开的连接池；CLI 进程退出前必须调用 CloseAll，
// 让 SQLite checkpoint WAL 并移除 -wal/-shm sidecar，保持 vault 树在命令间
// 不残留运行时产物。
package sqlitedsn

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	// busyTimeoutMillis 是写方在 SQLITE_BUSY 上的有界等待时长（毫秒）。
	busyTimeoutMillis = 5000
	// MaxOpenConns / MaxIdleConns 是统一读池界：读者并行、写者仍单写者。
	MaxOpenConns = 4
	MaxIdleConns = 4
)

// DSN 返回统一写路径 DSN：WAL journal + busy_timeout + foreign_keys。
func DSN(path string) string {
	return path + "?" +
		"_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMillis) + ")" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
}

// ReadOnlyDSN 返回只读 DSN（mode=ro + busy_timeout + foreign_keys）。
// 不设置 journal_mode：只读连接无法切换 journal mode，WAL 持久化属性由
// 写路径连接负责。
func ReadOnlyDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?" +
		"mode=ro" +
		"&_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMillis) + ")" +
		"&_pragma=foreign_keys(1)"
}

// Open 以统一 DSN 与池界打开 GORM SQLite 连接（写路径）。
func Open(path string) (*gorm.DB, error) {
	return open(DSN(path))
}

// OpenReadOnly 以只读 DSN 与池界打开 GORM SQLite 连接；数据库文件不存在时
// 返回错误（不创建），调用方负责先做存在性判断。
func OpenReadOnly(path string) (*gorm.DB, error) {
	return open(ReadOnlyDSN(path))
}

func open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite via unified dsn: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(MaxOpenConns)
	sqlDB.SetMaxIdleConns(MaxIdleConns)
	registerForExitClose(sqlDB)
	return db, nil
}

// openDBs 登记进程内经本包打开的全部连接池。Pinax CLI 是短生命周期进程：
// 命令结束时必须关闭全部连接——SQLite 在最后一个连接关闭时把 WAL 内容
// checkpoint 回主库文件并删除 -wal/-shm sidecar。否则每个 vault 会留下
// 运行时 sidecar（内容滞留 -wal、主文件近空），并让损坏检测读到旧 WAL
// 而非真实主文件。长生命周期进程（如 `pinax mcp`）在退出时同样经
// CloseAll 收口；被 SIGKILL 的残留 sidecar 由 SQLite 下次打开时自动恢复。
var (
	exitCloseMu sync.Mutex
	openDBs     []*sql.DB
)

func registerForExitClose(db *sql.DB) {
	exitCloseMu.Lock()
	defer exitCloseMu.Unlock()
	openDBs = append(openDBs, db)
}

// CloseAll 关闭本进程内经本包打开的全部 SQLite 连接，应在 CLI 进程退出
// （main 与进程内命令测试收尾）时调用。幂等；调用后既有句柄不可再用。
func CloseAll() {
	exitCloseMu.Lock()
	dbs := openDBs
	openDBs = nil
	exitCloseMu.Unlock()
	for _, db := range dbs {
		_ = db.Close()
	}
}
