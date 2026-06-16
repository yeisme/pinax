package index

import "gorm.io/gorm"

// schema.go 是 internal/index 中唯一允许直接访问 SQLite schema 元数据的地方。
//
// GORM Gen 类型化 DAO 无法表达 PRAGMA 查询；当 doctor/diagnose 路径需要判断索引文件
// 是否可读（而不是 schema 版本是否匹配）时，只在这里集中执行 PRAGMA，普通业务文件
// 不得再引入 Raw/Exec/direct SQL。source guard test 会显式 allowlist 本文件。
//
// 优先使用 GORM migrator（HasTable/HasColumn）判断结构，只有可读性探测需要 schema_version。

// indexSchemaReadError 执行 PRAGMA schema_version 探测 SQLite 是否可读。
// 返回非 nil 表示索引文件损坏或不可读，调用方据此返回 index_unreadable。
func indexSchemaReadError(db *gorm.DB) error {
	var schemaVersion int
	return db.Raw("PRAGMA schema_version").Scan(&schemaVersion).Error
}
