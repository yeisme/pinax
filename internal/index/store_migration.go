package index

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
	"github.com/yeisme/pinax/internal/sqlitedsn"
	"gorm.io/gorm"
)

func globalUpdate() *gorm.Session {
	return &gorm.Session{AllowGlobalUpdate: true}
}

func clearAllProjections(q *query.Query, ctx context.Context) error {
	clearers := []func() error{
		func() error { _, err := q.NoteRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error { _, err := q.NoteTextRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error { _, err := q.TagRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error { _, err := q.LinkRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error {
			_, err := q.SearchTokenRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error {
			_, err := q.AttachmentRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error {
			_, err := q.AssetLinkRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error {
			_, err := q.VaultFileRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error { _, err := q.AssetRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error { _, err := q.FolderRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
		func() error {
			_, err := q.DimensionCountRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error {
			_, err := q.PropertyDefinitionRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error {
			_, err := q.PropertyValueRecord.WithContext(ctx).Session(globalUpdate()).Delete()
			return err
		},
		func() error { _, err := q.TaskRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
	}
	for _, clear := range clearers {
		if err := clear(); err != nil {
			return err
		}
	}
	return nil
}

func open(root string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		return nil, err
	}
	return sqlitedsn.Open(filepath.Join(root, ".pinax", "index.sqlite"))
}

func migrate(db *gorm.DB) error {
	repaired, err := repairPrimaryKeyDrift(db)
	if err != nil {
		return err
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return err
	}
	if err := backfillObjectIdentity(db); err != nil {
		return err
	}
	if len(repaired) > 0 {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := recordSchemaPrimaryKeyRepair(db, repaired, now); err != nil {
			return err
		}
	}
	return nil
}

// repairPrimaryKeyDrift 修复旧版本索引库的主键漂移：AutoMigrate 只补列，
// 不会重建既有表的主键，而增量 upsert(Save) 依赖 ON CONFLICT(<model primary key>)。
// 主键不一致时在事务内 rename → 按模型重建表 → 拷贝共有列 → drop 旧表，
// 缺失的列由后续 AutoMigrate/backfill 补齐。返回实际重建的表名列表。
func repairPrimaryKeyDrift(db *gorm.DB) ([]string, error) {
	repaired := []string{}
	for _, m := range model.AllModels() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(m); err != nil {
			return nil, err
		}
		table := stmt.Schema.Table
		if !db.Migrator().HasTable(table) {
			continue
		}
		expected := make([]string, 0, len(stmt.Schema.PrimaryFields))
		for _, field := range stmt.Schema.PrimaryFields {
			expected = append(expected, field.DBName)
		}
		actual, err := sqlitePrimaryKeyColumns(db, table)
		if err != nil {
			return nil, err
		}
		if equalStringSet(expected, actual) {
			continue
		}
		if err := rebuildTablePrimaryKey(db, table, m); err != nil {
			return nil, fmt.Errorf("rebuild %s primary key: %w", table, err)
		}
		repaired = append(repaired, table)
	}
	return repaired, nil
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := append([]string{}, a...)
	sortedB := append([]string{}, b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if sortedA[i] != sortedB[i] {
			return false
		}
	}
	return true
}

// schemaPrimaryKeyRepairMetaKey 在 index_meta_records 中记录主键漂移修复日志，
// 让原本静默的自愈对 doctor/status 可见、可审计。
const schemaPrimaryKeyRepairMetaKey = "schema_pk_repairs"

const schemaPrimaryKeyRepairLogLimit = 10

type schemaPrimaryKeyRepairEntry struct {
	Tables []string `json:"tables"`
	At     string   `json:"at"`
}

func recordSchemaPrimaryKeyRepair(db *gorm.DB, tables []string, now string) error {
	entries := schemaPrimaryKeyRepairLog(db)
	entries = append(entries, schemaPrimaryKeyRepairEntry{Tables: tables, At: now})
	if len(entries) > schemaPrimaryKeyRepairLogLimit {
		entries = entries[len(entries)-schemaPrimaryKeyRepairLogLimit:]
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return upsertMeta(db, schemaPrimaryKeyRepairMetaKey, string(raw), now)
}

func schemaPrimaryKeyRepairLog(db *gorm.DB) []schemaPrimaryKeyRepairEntry {
	raw := metaValue(db, schemaPrimaryKeyRepairMetaKey)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	entries := []schemaPrimaryKeyRepairEntry{}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}

func latestSchemaPrimaryKeyRepair(db *gorm.DB) (schemaPrimaryKeyRepairEntry, bool) {
	entries := schemaPrimaryKeyRepairLog(db)
	if len(entries) == 0 {
		return schemaPrimaryKeyRepairEntry{}, false
	}
	return entries[len(entries)-1], true
}

func backfillObjectIdentity(db *gorm.DB) error {
	ctx := context.Background()
	q := query.Use(db)
	notes, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	byPath := map[string]string{}
	for _, note := range notes {
		objectID := strings.TrimSpace(note.ObjectID)
		if objectID == "" {
			objectID = strings.TrimSpace(note.NoteID)
			if objectID != "" {
				if _, err := q.NoteRecord.WithContext(ctx).Where(q.NoteRecord.Path.Eq(note.Path)).Update(q.NoteRecord.ObjectID, objectID); err != nil {
					return err
				}
			}
		}
		if objectID != "" {
			byPath[note.Path] = objectID
		}
	}
	texts, err := q.NoteTextRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range texts {
		if row.ObjectID == "" && byPath[row.NotePath] != "" {
			if _, err := q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(row.NotePath)).Update(q.NoteTextRecord.ObjectID, byPath[row.NotePath]); err != nil {
				return err
			}
		}
	}
	tags, err := q.TagRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range tags {
		if row.ObjectID == "" && byPath[row.NotePath] != "" {
			if _, err := q.TagRecord.WithContext(ctx).Where(q.TagRecord.ID.Eq(row.ID)).Update(q.TagRecord.ObjectID, byPath[row.NotePath]); err != nil {
				return err
			}
		}
	}
	links, err := q.LinkRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range links {
		updates := map[string]any{}
		if row.SourceObjectID == "" && byPath[row.NotePath] != "" {
			updates["source_object_id"] = byPath[row.NotePath]
		}
		if row.TargetObjectID == "" && strings.TrimSpace(row.TargetNoteID) != "" {
			updates["target_object_id"] = row.TargetNoteID
		}
		if len(updates) > 0 {
			if _, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.ID.Eq(row.ID)).UpdateColumns(updates); err != nil {
				return err
			}
		}
	}
	tokens, err := q.SearchTokenRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range tokens {
		if row.ObjectID == "" && byPath[row.NotePath] != "" {
			if _, err := q.SearchTokenRecord.WithContext(ctx).Where(q.SearchTokenRecord.ID.Eq(row.ID)).Update(q.SearchTokenRecord.ObjectID, byPath[row.NotePath]); err != nil {
				return err
			}
		}
	}
	attachments, err := q.AttachmentRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range attachments {
		if row.ObjectID == "" && byPath[row.NotePath] != "" {
			if _, err := q.AttachmentRecord.WithContext(ctx).Where(q.AttachmentRecord.ID.Eq(row.ID)).Update(q.AttachmentRecord.ObjectID, byPath[row.NotePath]); err != nil {
				return err
			}
		}
	}
	properties, err := q.PropertyValueRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range properties {
		if row.ObjectID == "" && byPath[row.NotePath] != "" {
			if _, err := q.PropertyValueRecord.WithContext(ctx).Where(q.PropertyValueRecord.ID.Eq(row.ID)).Update(q.PropertyValueRecord.ObjectID, byPath[row.NotePath]); err != nil {
				return err
			}
		}
	}
	tasks, err := q.TaskRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range tasks {
		if row.ObjectID == "" {
			objectID := firstNonEmptyIndexValue(row.NoteID, byPath[row.NotePath])
			if objectID != "" {
				if _, err := q.TaskRecord.WithContext(ctx).Where(q.TaskRecord.ID.Eq(row.ID)).Update(q.TaskRecord.ObjectID, objectID); err != nil {
					return err
				}
			}
		}
	}
	assets, err := q.AssetRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	assetByPath := map[string]string{}
	for _, row := range assets {
		objectID := firstNonEmptyIndexValue(row.ObjectID, row.AssetID)
		if row.ObjectID == "" && objectID != "" {
			if _, err := q.AssetRecord.WithContext(ctx).Where(q.AssetRecord.Path.Eq(row.Path)).Update(q.AssetRecord.ObjectID, objectID); err != nil {
				return err
			}
		}
		if objectID != "" {
			assetByPath[row.Path] = objectID
		}
	}
	assetLinks, err := q.AssetLinkRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range assetLinks {
		updates := map[string]any{}
		if row.SourceObjectID == "" {
			updates["source_object_id"] = firstNonEmptyIndexValue(row.SourceNoteID, byPath[row.SourcePath])
		}
		if row.AssetObjectID == "" && assetByPath[row.AssetPath] != "" {
			updates["asset_object_id"] = assetByPath[row.AssetPath]
		}
		if len(updates) > 0 {
			if _, err := q.AssetLinkRecord.WithContext(ctx).Where(q.AssetLinkRecord.ID.Eq(row.ID)).UpdateColumns(updates); err != nil {
				return err
			}
		}
	}
	files, err := q.VaultFileRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	for _, row := range files {
		if row.ObjectID == "" {
			objectID := firstNonEmptyIndexValue(byPath[row.Path], assetByPath[row.Path])
			if objectID != "" {
				if _, err := q.VaultFileRecord.WithContext(ctx).Where(q.VaultFileRecord.Path.Eq(row.Path)).Update(q.VaultFileRecord.ObjectID, objectID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func firstNonEmptyIndexValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func upsertMeta(db *gorm.DB, key, value, now string) error {
	q := query.Use(db)
	return q.IndexMetaRecord.WithContext(context.Background()).Save(&model.IndexMetaRecord{Key: key, Value: value, UpdatedAt: now})
}

func metaValue(db *gorm.DB, key string) string {
	q := query.Use(db)
	record, err := q.IndexMetaRecord.WithContext(context.Background()).Where(q.IndexMetaRecord.Key.Eq(key)).First()
	if err != nil {
		return ""
	}
	return record.Value
}
