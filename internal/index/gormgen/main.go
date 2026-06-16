// gormgen 为 internal/index 本地索引投影生成 GORM Gen 类型化 DAO。
//
// 运行方式（在本子项目根目录）：
//
//	go run ./internal/index/gormgen
//
// 生成产物写入 internal/index/query，引用 internal/index/model 中的模型。
// Markdown vault 仍是真源，这里生成的 DAO 只用于 SQLite 投影的类型化读写。
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/index/model"
	"gorm.io/gen"
	"gorm.io/gorm"
)

func main() {
	outPath := resolveOutPath()

	// 生成器需要一个真实 DB 来解析模型字段，使用临时 SQLite 文件即可，
	// 不依赖任何 vault 数据。
	dbPath := createTempDBPath()
	defer func() { _ = os.Remove(dbPath) }()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("open temp db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("open temp db handle: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		log.Fatalf("auto migrate: %v", err)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath: outPath,
		Mode:    gen.WithoutContext | gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(db)
	g.ApplyBasic(model.AllModels()...)
	g.Execute()

	log.Printf("gormgen: generated typed DAO into %s", outPath)
}

// createTempDBPath 创建本次生成专用的临时 SQLite 文件路径。
func createTempDBPath() string {
	tempFile, err := os.CreateTemp("", "pinax-gormgen-index-*.db")
	if err != nil {
		log.Fatalf("create temp db: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		log.Fatalf("close temp db: %v", err)
	}
	return tempFile.Name()
}

// resolveOutPath 计算生成输出目录，默认相对生成器源码的 ../query。
func resolveOutPath() string {
	if custom := os.Getenv("PINAX_GORMGEN_OUT"); custom != "" {
		return custom
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("gormgen: cannot resolve source path")
	}
	return filepath.Join(filepath.Dir(file), "..", "query")
}
