package index

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// schema.go 是 internal/index 中唯一允许直接访问 SQLite schema 元数据的地方。
//
// GORM Gen 类型化 DAO 无法表达 PRAGMA 查询和 DDL 表重建；当 doctor/diagnose 路径需要
// 判断索引文件是否可读、或 migrate 路径需要修复旧版本遗留的主键漂移时，只在这里集中
// 执行 PRAGMA/DDL，普通业务文件不得再引入 Raw/Exec/direct SQL。source guard test 会
// 显式 allowlist 本文件。

// indexSchemaReadError 执行 PRAGMA schema_version 探测 SQLite 是否可读。
// 返回非 nil 表示索引文件损坏或不可读，调用方据此返回 index_unreadable。
func indexSchemaReadError(db *gorm.DB) error {
	var schemaVersion int
	return db.Raw("PRAGMA schema_version").Scan(&schemaVersion).Error
}

// sqlitePrimaryKeyColumns 返回表当前的主键列（按 PRAGMA table_info 的 pk 序）。
func sqlitePrimaryKeyColumns(db *gorm.DB, table string) ([]string, error) {
	type columnInfo struct {
		Name string
		PK   int
	}
	infos := []columnInfo{}
	if err := db.Raw(fmt.Sprintf("PRAGMA table_info(%q)", table)).Scan(&infos).Error; err != nil {
		return nil, err
	}
	columns := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.PK > 0 {
			columns = append(columns, info.Name)
		}
	}
	return columns, nil
}

// rebuildTablePrimaryKey 在事务内重建主键漂移的表：rename → 按模型 AutoMigrate
// 出新表 → 拷贝共有列 → drop 旧表。先 drop 旧表用户索引，避免新表索引命名冲突。
func rebuildTablePrimaryKey(db *gorm.DB, table string, dst any) error {
	legacy := table + "_pkfix_legacy"
	return db.Transaction(func(tx *gorm.DB) error {
		indexes := []string{}
		if err := tx.Raw("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name NOT LIKE 'sqlite_autoindex_%'", table).Scan(&indexes).Error; err != nil {
			return err
		}
		for _, index := range indexes {
			if err := tx.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %q", index)).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(fmt.Sprintf("ALTER TABLE %q RENAME TO %q", table, legacy)).Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(dst); err != nil {
			return err
		}
		columns, err := commonTableColumns(tx, table, legacy)
		if err != nil {
			return err
		}
		if len(columns) > 0 {
			quoted := make([]string, 0, len(columns))
			for _, column := range columns {
				quoted = append(quoted, fmt.Sprintf("%q", column))
			}
			list := strings.Join(quoted, ", ")
			if err := tx.Exec(fmt.Sprintf("INSERT INTO %q (%s) SELECT %s FROM %q", table, list, list, legacy)).Error; err != nil {
				return err
			}
		}
		return tx.Exec(fmt.Sprintf("DROP TABLE %q", legacy)).Error
	})
}

func commonTableColumns(tx *gorm.DB, dstTable, srcTable string) ([]string, error) {
	columnsOf := func(table string) (map[string]bool, []string, error) {
		type columnInfo struct {
			Name string
		}
		infos := []columnInfo{}
		if err := tx.Raw(fmt.Sprintf("PRAGMA table_info(%q)", table)).Scan(&infos).Error; err != nil {
			return nil, nil, err
		}
		set := map[string]bool{}
		ordered := make([]string, 0, len(infos))
		for _, info := range infos {
			set[info.Name] = true
			ordered = append(ordered, info.Name)
		}
		return set, ordered, nil
	}
	dstSet, dstOrdered, err := columnsOf(dstTable)
	if err != nil {
		return nil, err
	}
	srcSet, _, err := columnsOf(srcTable)
	if err != nil {
		return nil, err
	}
	common := make([]string, 0, len(dstOrdered))
	for _, column := range dstOrdered {
		if srcSet[column] && dstSet[column] {
			common = append(common, column)
		}
	}
	return common, nil
}
