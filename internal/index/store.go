package index

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/glebarez/sqlite"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const SchemaVersion = "pinax.index.v2"
const PropertySchemaVersion = "pinax.properties.v1"

// 以下类型别名指向 internal/index/model 中的 GORM 模型，保持本包公开 API 稳定。
// 普通业务读写必须通过 internal/index/query 生成的类型化 DAO，模型定义集中在 model 包。
type (
	IndexMetaRecord          = model.IndexMetaRecord
	NoteRecord               = model.NoteRecord
	NoteTextRecord           = model.NoteTextRecord
	TagRecord                = model.TagRecord
	LinkRecord               = model.LinkRecord
	SearchTokenRecord        = model.SearchTokenRecord
	AttachmentRecord         = model.AttachmentRecord
	AssetRecord              = model.AssetRecord
	AssetLinkRecord          = model.AssetLinkRecord
	VaultFileRecord          = model.VaultFileRecord
	FolderRecord             = model.FolderRecord
	DimensionCountRecord     = model.DimensionCountRecord
	PropertyDefinitionRecord = model.PropertyDefinitionRecord
	PropertyValueRecord      = model.PropertyValueRecord
)

// globalUpdate 开启 GORM 全局更新/删除会话，仅用于重建/增量投影清空整表等受控场景。
func globalUpdate() *gorm.Session {
	return &gorm.Session{AllowGlobalUpdate: true}
}

// clearAllProjections 清空索引除 index_meta 之外的全部 projection 表。
// 重建路径在事务内先清空再重写，保证 Markdown 真源与投影一致。
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
	}
	for _, clear := range clearers {
		if err := clear(); err != nil {
			return err
		}
	}
	return nil
}

type Counts struct {
	Notes       int
	Tags        int
	Links       int
	Tokens      int
	Attachments int
	Dimensions  int
	Folders     int
}

type NoteUpdate struct {
	OldPath      string
	Note         domain.Note
	ModifiedUnix int64
	Size         int64
}

type NoteDelete struct {
	Path string
}

type IncrementalResult struct {
	NotePath string
	Skipped  bool
	Parsed   int
	Indexed  int
}

type SyncResult struct {
	Created int `json:"created"`
	Changed int `json:"changed"`
	Moved   int `json:"moved"`
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type RefreshOptions struct {
	BatchSize int `json:"batch_size,omitempty"`
}

type RefreshResult struct {
	Scanned        int      `json:"scanned"`
	Changed        int      `json:"changed"`
	Skipped        int      `json:"skipped"`
	Indexed        int      `json:"indexed"`
	Created        int      `json:"created"`
	Moved          int      `json:"moved"`
	Deleted        int      `json:"deleted"`
	Failed         int      `json:"failed"`
	Batches        int      `json:"batches"`
	DurationMillis int64    `json:"duration_ms"`
	IndexStatus    string   `json:"index_status"`
	FailedPaths    []string `json:"failed_paths,omitempty"`
}

type Issue struct {
	Code     string   `json:"issue_code"`
	Severity string   `json:"severity"`
	Path     string   `json:"path,omitempty"`
	NoteID   string   `json:"note_id,omitempty"`
	Message  string   `json:"message"`
	Evidence []string `json:"evidence,omitempty"`
}

type DoctorReport struct {
	Status Status         `json:"status"`
	Issues []Issue        `json:"issues"`
	Counts map[string]int `json:"counts"`
}

type RepairOperation struct {
	Kind   string `json:"kind"`
	Mode   string `json:"mode"`
	Risk   string `json:"risk"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type RepairPlan struct {
	Kind       string            `json:"kind"`
	DryRun     bool              `json:"dry_run"`
	Writes     bool              `json:"writes"`
	Operations []RepairOperation `json:"operations"`
}

type RepairResult struct {
	Plan        RepairPlan `json:"plan"`
	IndexStatus string     `json:"index_status"`
	BackupPath  string     `json:"backup_path,omitempty"`
	Counts      Counts     `json:"counts,omitempty"`
}

type Status struct {
	Status        string   `json:"status"`
	Path          string   `json:"path"`
	SchemaVersion string   `json:"schema_version,omitempty"`
	Notes         int      `json:"notes,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
}

type SearchRequest struct {
	Query         string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedAfter  string
	LinkTarget    string
	HasAttachment bool
	Limit         int
	Sort          string
}

type SearchResult struct {
	Engine      string       `json:"engine"`
	IndexStatus string       `json:"index_status"`
	Total       int          `json:"total"`
	Returned    int          `json:"returned"`
	Results     []ResultItem `json:"results"`
}

type ResultItem struct {
	Note            domain.Note `json:"note"`
	Score           int         `json:"score"`
	MatchedFields   []string    `json:"matched_fields"`
	Snippet         string      `json:"snippet"`
	LinkCount       int         `json:"link_count"`
	AttachmentCount int         `json:"attachment_count"`
}

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
var inlineTagPattern = regexp.MustCompile(`(^|\s)#([\pL\pN_/-]+)`)
var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

func Init(root string) (Status, error) {
	db, err := open(root)
	if err != nil {
		return Status{}, err
	}
	if err := migrate(db); err != nil {
		return Status{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := upsertMeta(db, "schema_version", SchemaVersion, now); err != nil {
		return Status{}, err
	}
	status := Status{Status: "fresh", Path: indexRelPath(), SchemaVersion: SchemaVersion}
	return status, nil
}

func Inspect(root string, notes []domain.Note) (Status, error) {
	report, err := Diagnose(root, notes)
	if err != nil {
		return Status{}, err
	}
	return report.Status, nil
}

func Diagnose(root string, notes []domain.Note) (DoctorReport, error) {
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			status := Status{Status: "missing", Path: indexRelPath()}
			return doctorReport(status, []Issue{indexIssue("index_missing", "warning", status.Path, "本地索引缺失", []string{"index_status=missing"})}), nil
		}
		status := Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}
		return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
	}
	db, err := open(root)
	if err != nil {
		status := Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}
		return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
	}
	if schemaIssues := indexStorageSchemaIssues(db); len(schemaIssues) > 0 {
		if err := indexSchemaReadError(db); err != nil {
			status := Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}
			return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
		}
		status := Status{Status: "stale", Path: indexRelPath(), Notes: len(notes), Evidence: issueEvidence(schemaIssues)}
		if !schemaIssuesContainEvidence(schemaIssues, "missing_table=index_meta_records") {
			status.SchemaVersion = metaValue(db, "schema_version")
		}
		return doctorReport(status, schemaIssues), nil
	}
	if err := migrate(db); err != nil {
		status := Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}
		return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
	}
	schema := metaValue(db, "schema_version")
	if schema == "" {
		status := Status{Status: "stale", Path: indexRelPath(), Evidence: []string{"schema_version=missing"}}
		return doctorReport(status, []Issue{indexIssue("index_schema_mismatch", "warning", status.Path, "索引 schema 缺失", status.Evidence)}), nil
	}
	if schema != SchemaVersion {
		status := Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{"schema_version=" + schema}}
		return doctorReport(status, []Issue{indexIssue("index_schema_mismatch", "warning", status.Path, "索引 schema 版本不匹配", status.Evidence)}), nil
	}
	propertySchema := metaValue(db, "property_schema_version")
	if propertySchema != PropertySchemaVersion {
		if propertySchema == "" {
			propertySchema = "missing"
		}
		status := Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Notes: len(notes), Evidence: []string{"property_schema_version=" + propertySchema}}
		return doctorReport(status, []Issue{indexIssue("index_schema_mismatch", "warning", status.Path, "属性索引 schema 版本不匹配", status.Evidence)}), nil
	}
	q := query.Use(db)
	records, err := q.NoteRecord.WithContext(context.Background()).Find()
	if err != nil {
		status := Status{Status: "unreadable", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{err.Error()}}
		return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
	}
	byPath := map[string]*model.NoteRecord{}
	for _, record := range records {
		byPath[record.Path] = record
	}
	notePaths := map[string]bool{}
	issues := []Issue{}
	for _, note := range notes {
		if isSystemJournalNotePath(note.Path) || isSystemIndexNotePath(note.Path) {
			continue
		}
		notePaths[note.Path] = true
		record, ok := byPath[note.Path]
		if !ok {
			issues = append(issues, indexIssue("index_stale", "warning", indexRelPath(), "索引缺少 vault note projection", []string{"missing_note=" + note.Path}))
			continue
		}
		if record.SourceHash != noteSourceHash(note) {
			issues = append(issues, indexIssue("index_stale", "warning", indexRelPath(), "索引 note projection 已过期", []string{"changed_note=" + note.Path}))
		}
	}
	for _, record := range records {
		if isSystemJournalNotePath(record.Path) || isSystemIndexNotePath(record.Path) {
			continue
		}
		if !notePaths[record.Path] {
			issues = append(issues, indexIssue("index_stale", "warning", indexRelPath(), "索引包含 vault 中不存在 of note projection", []string{"extra_note=" + record.Path}))
		}
		textRows, countErr := q.NoteTextRecord.WithContext(context.Background()).Where(q.NoteTextRecord.NotePath.Eq(record.Path)).Count()
		if countErr != nil {
			status := Status{Status: "unreadable", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{countErr.Error()}}
			return doctorReport(status, []Issue{indexIssue("index_unreadable", "error", status.Path, "本地索引不可读", status.Evidence)}), nil
		}
		if textRows == 0 {
			issues = append(issues, indexIssue("index_row_consistency", "warning", indexRelPath(), "索引 note/text projection 不一致", []string{"missing_note_text=" + record.Path}))
		}
	}
	statusName := "fresh"
	if len(issues) > 0 {
		statusName = "partial"

		for _, issue := range issues {
			if issue.Code == "index_stale" || issue.Code == "index_schema_mismatch" {
				statusName = "stale"
				break
			}
		}
	}
	status := Status{Status: statusName, Path: indexRelPath(), SchemaVersion: schema, Notes: len(records), Evidence: issueEvidence(issues)}
	return doctorReport(status, issues), nil
}

func isSystemJournalNotePath(path string) bool {
	p := filepath.ToSlash(path)
	return strings.HasPrefix(p, "daily/") || strings.HasPrefix(p, "notes/daily/") ||
		strings.HasPrefix(p, "weekly/") || strings.HasPrefix(p, "notes/weekly/") ||
		strings.HasPrefix(p, "monthly/") || strings.HasPrefix(p, "notes/monthly/")
}

func isSystemIndexNotePath(path string) bool {
	p := filepath.ToSlash(path)
	return strings.HasPrefix(p, "index/") || strings.HasPrefix(p, "notes/index/")
}

func indexIssue(code, severity, path, message string, evidence []string) Issue {
	return Issue{Code: code, Severity: severity, Path: path, Message: message, Evidence: evidence}
}

func doctorReport(status Status, issues []Issue) DoctorReport {
	return DoctorReport{Status: status, Issues: issues, Counts: issueSeverityCounts(issues)}
}

func issueSeverityCounts(issues []Issue) map[string]int {
	counts := map[string]int{"warning": 0, "error": 0}
	for _, issue := range issues {
		if issue.Severity == "" {
			continue
		}
		counts[issue.Severity]++
	}
	return counts
}

func issueEvidence(issues []Issue) []string {
	evidence := []string{}
	seen := map[string]bool{}
	for _, issue := range issues {
		for _, item := range issue.Evidence {
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			evidence = append(evidence, item)
		}
	}
	return evidence
}

func indexStorageSchemaIssues(db *gorm.DB) []Issue {
	requirements := []struct {
		model   any
		table   string
		columns []string
	}{
		{model: &IndexMetaRecord{}, table: "index_meta_records", columns: []string{"key", "value", "updated_at"}},
		{model: &NoteRecord{}, table: "note_records", columns: []string{"path", "note_id", "filename", "stem", "object_kind", "managed_status", "source_hash"}},
		{model: &AssetRecord{}, table: "asset_records", columns: []string{"path", "asset_id", "filename", "stem", "media_type", "managed_status", "sha256"}},
		{model: &AssetLinkRecord{}, table: "asset_link_records", columns: []string{"asset_path", "source_path", "raw_reference", "link_style", "link_kind", "status", "media_type"}},
		{model: &VaultFileRecord{}, table: "vault_file_records", columns: []string{"path", "filename", "stem", "object_kind", "managed_status"}},
		{model: &FolderRecord{}, table: "folder_records", columns: []string{"path", "purpose", "managed_status", "note_count", "asset_count"}},
		{model: &PropertyDefinitionRecord{}, table: "property_definition_records", columns: []string{"name", "type", "source"}},
		{model: &PropertyValueRecord{}, table: "property_value_records", columns: []string{"note_path", "name", "type", "value", "source"}},
	}
	issues := []Issue{}
	for _, requirement := range requirements {
		if !db.Migrator().HasTable(requirement.model) {
			issues = append(issues, indexIssue("index_schema_mismatch", "warning", indexRelPath(), "索引 projection 表缺失", []string{"missing_table=" + requirement.table}))
			continue
		}
		for _, column := range requirement.columns {
			if !db.Migrator().HasColumn(requirement.model, column) {
				issues = append(issues, indexIssue("index_schema_mismatch", "warning", indexRelPath(), "索引 projection 字段缺失", []string{"missing_column=" + requirement.table + "." + column}))
			}
		}
	}
	return issues
}

func schemaIssuesContainEvidence(issues []Issue, evidence string) bool {
	for _, issue := range issues {
		for _, item := range issue.Evidence {
			if item == evidence {
				return true
			}
		}
	}
	return false
}

func Rebuild(root string, notes []domain.Note) (Counts, error) {
	db, err := open(root)
	if err != nil {
		return Counts{}, err
	}
	if err := migrate(db); err != nil {
		return Counts{}, err
	}
	counts := Counts{}
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		q := query.Use(tx)
		if err := clearAllProjections(q, ctx); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := upsertMeta(tx, "schema_version", SchemaVersion, now); err != nil {
			return err
		}
		if err := upsertMeta(tx, "property_schema_version", PropertySchemaVersion, now); err != nil {
			return err
		}
		if err := upsertMeta(tx, "rebuilt_at", now, now); err != nil {
			return err
		}
		pathByTitle := notePathByTitle(notes)
		for _, note := range notes {
			record := noteRecordFromDomain(note, 0, 0)
			if err := q.NoteRecord.WithContext(ctx).Create(&record); err != nil {
				return err
			}
			counts.Notes++
			if err := q.NoteTextRecord.WithContext(ctx).Create(&NoteTextRecord{NotePath: note.Path, TitleText: note.Title, BodyText: note.Body, Excerpt: excerpt(note.Body), WordCount: len(tokens(note.Body))}); err != nil {
				return err
			}
			for _, tag := range uniqueTags(note) {
				if err := q.TagRecord.WithContext(ctx).Create(&TagRecord{NotePath: note.Path, Tag: tag}); err != nil {
					return err
				}
				counts.Tags++
			}
			for _, token := range noteTokens(note) {
				if err := q.SearchTokenRecord.WithContext(ctx).Create(&SearchTokenRecord{NotePath: note.Path, Token: token.Token, Field: token.Field, Count: token.Count, Weight: token.Weight}); err != nil {
					return err
				}
				counts.Tokens++
			}
			for _, link := range noteLinks(note, pathByTitle) {
				if err := q.LinkRecord.WithContext(ctx).Create(&link); err != nil {
					return err
				}
				counts.Links++
			}
			for _, attachment := range noteAttachments(root, note) {
				if err := q.AttachmentRecord.WithContext(ctx).Create(&attachment); err != nil {
					return err
				}
				counts.Attachments++
			}
			for _, assetLink := range noteAssetLinks(root, note) {
				if err := q.AssetLinkRecord.WithContext(ctx).Create(&assetLink); err != nil {
					return err
				}
			}
		}
		for _, dimension := range noteDimensionCounts(notes) {
			if err := q.DimensionCountRecord.WithContext(ctx).Create(&dimension); err != nil {
				return err
			}
			counts.Dimensions++
		}
		if err := rebuildVaultObjectProjection(tx, root, notes); err != nil {
			return err
		}
		folders, err := rebuildFolderProjection(tx, root, notes)
		if err != nil {
			return err
		}
		counts.Folders = folders
		if err := rebuildPropertyProjection(tx, notes); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Counts{}, err
	}
	return counts, nil
}

func Refresh(root string, notes []domain.Note, opts RefreshOptions) (RefreshResult, error) {
	started := time.Now()
	syncResult, err := Sync(root, notes)
	result := RefreshResult{
		Scanned:        len(notes),
		Changed:        syncResult.Changed,
		Skipped:        syncResult.Skipped,
		Indexed:        syncResult.Created + syncResult.Changed + syncResult.Moved,
		Created:        syncResult.Created,
		Moved:          syncResult.Moved,
		Deleted:        syncResult.Deleted,
		Failed:         syncResult.Failed,
		Batches:        refreshBatchCount(len(notes), opts.BatchSize),
		DurationMillis: time.Since(started).Milliseconds(),
		IndexStatus:    "fresh",
	}
	if result.Failed > 0 {
		result.IndexStatus = "partial"
	}
	if err == nil {
		if db, openErr := open(root); openErr != nil {
			return result, openErr
		} else if migrateErr := migrate(db); migrateErr != nil {
			return result, migrateErr
		} else if rebuildErr := db.Transaction(func(tx *gorm.DB) error {
			ctx := context.Background()
			q := query.Use(tx)
			now := time.Now().UTC().Format(time.RFC3339)
			if err := upsertMeta(tx, "schema_version", SchemaVersion, now); err != nil {
				return err
			}
			if err := upsertMeta(tx, "property_schema_version", PropertySchemaVersion, now); err != nil {
				return err
			}
			for _, clear := range []func() error{
				func() error {
					_, err := q.PropertyDefinitionRecord.WithContext(ctx).Session(globalUpdate()).Delete()
					return err
				},
				func() error {
					_, err := q.PropertyValueRecord.WithContext(ctx).Session(globalUpdate()).Delete()
					return err
				},
				func() error { _, err := q.FolderRecord.WithContext(ctx).Session(globalUpdate()).Delete(); return err },
			} {
				if err := clear(); err != nil {
					return err
				}
			}
			if err := rebuildVaultObjectProjection(tx, root, notes); err != nil {
				return err
			}
			if _, err := rebuildFolderProjection(tx, root, notes); err != nil {
				return err
			}
			return rebuildPropertyProjection(tx, notes)
		}); rebuildErr != nil {
			return result, rebuildErr
		}
	}
	return result, err
}

func RefreshChanged(root string, notes []domain.Note, changed []domain.ChangedPath, opts RefreshOptions) (RefreshResult, error) {
	started := time.Now()
	result := RefreshResult{Scanned: len(changed), Batches: refreshBatchCount(len(changed), opts.BatchSize), IndexStatus: "fresh"}
	db, err := open(root)
	if err != nil {
		return result, err
	}
	if err := migrate(db); err != nil {
		return result, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := upsertMeta(db, "schema_version", SchemaVersion, now); err != nil {
		return result, err
	}
	if err := upsertMeta(db, "property_schema_version", PropertySchemaVersion, now); err != nil {
		return result, err
	}
	notesByPath := map[string]domain.Note{}
	for _, note := range notes {
		notesByPath[filepath.ToSlash(filepath.Clean(note.Path))] = note
	}
	for _, candidate := range changed {
		path := filepath.ToSlash(filepath.Clean(candidate.Path))
		note, ok := notesByPath[path]
		if !ok {
			continue
		}
		update, err := UpdateNote(root, NoteUpdate{Note: note, ModifiedUnix: candidate.ModifiedUnix, Size: candidate.SizeBytes})
		if err != nil {
			result.Failed++
			result.FailedPaths = append(result.FailedPaths, path)
			result.IndexStatus = "partial"
			return result, err
		}
		if update.Skipped {
			result.Skipped++
			continue
		}
		result.Changed++
		result.Indexed++
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return rebuildVaultObjectProjection(tx, root, notes) }); err != nil {
		return result, err
	}
	result.DurationMillis = time.Since(started).Milliseconds()
	return result, nil
}

func refreshBatchCount(total, batchSize int) int {
	if total == 0 {
		return 0
	}
	if batchSize <= 0 {
		return 1
	}
	return (total + batchSize - 1) / batchSize
}

func rebuildPropertyProjection(tx *gorm.DB, notes []domain.Note) error {
	ctx := context.Background()
	q := query.Use(tx)
	rows := ExtractPropertyRows(notes)
	for _, def := range InferPropertyDefinitions(rows) {
		record := PropertyDefinitionRecord{Name: def.Name, Type: string(def.Type), Source: def.Source, Count: def.Count, Samples: strings.Join(def.Samples, "\n")}
		if err := q.PropertyDefinitionRecord.WithContext(ctx).Create(&record); err != nil {
			return err
		}
	}
	for _, row := range rows {
		for _, value := range row.Values {
			record := PropertyValueRecord{NotePath: row.Note.Path, Name: value.Name, Type: string(value.Type), Raw: value.Raw, Value: value.String(), Source: value.Source}
			if err := q.PropertyValueRecord.WithContext(ctx).Create(&record); err != nil {
				return err
			}
		}
	}
	return nil
}

func Sync(root string, notes []domain.Note) (SyncResult, error) {
	if _, err := Init(root); err != nil {
		return SyncResult{}, err
	}
	db, err := open(root)
	if err != nil {
		return SyncResult{}, err
	}
	if err := migrate(db); err != nil {
		return SyncResult{}, err
	}
	q := query.Use(db)
	records, err := q.NoteRecord.WithContext(context.Background()).Find()
	if err != nil {
		return SyncResult{}, err
	}
	byPath := map[string]*model.NoteRecord{}
	byID := map[string]*model.NoteRecord{}
	for _, record := range records {
		byPath[record.Path] = record
		if strings.TrimSpace(record.NoteID) != "" {
			byID[record.NoteID] = record
		}
	}
	seen := map[string]bool{}
	result := SyncResult{}
	for _, note := range notes {
		seen[note.Path] = true
		hash := noteSourceHash(note)
		if record, ok := byPath[note.Path]; ok {
			if record.SourceHash == hash {
				result.Skipped++
				continue
			}
			if _, err := UpdateNote(root, NoteUpdate{Note: note}); err != nil {
				result.Failed++
				return result, err
			}
			result.Changed++
			continue
		}
		if record, ok := byID[note.ID]; ok && strings.TrimSpace(note.ID) != "" {
			if _, err := UpdateNote(root, NoteUpdate{OldPath: record.Path, Note: note}); err != nil {
				result.Failed++
				return result, err
			}
			seen[record.Path] = true
			result.Moved++
			continue
		}
		if _, err := UpdateNote(root, NoteUpdate{Note: note}); err != nil {
			result.Failed++
			return result, err
		}
		result.Created++
	}
	for _, record := range records {
		if seen[record.Path] {
			continue
		}
		if _, err := DeleteNote(root, NoteDelete{Path: record.Path}); err != nil {
			result.Failed++
			return result, err
		}
		result.Deleted++
	}
	return result, nil
}

func UpdateNote(root string, update NoteUpdate) (IncrementalResult, error) {
	db, err := open(root)
	if err != nil {
		return IncrementalResult{}, err
	}
	if err := migrate(db); err != nil {
		return IncrementalResult{}, err
	}
	note := update.Note
	hash := noteSourceHash(note)
	result := IncrementalResult{NotePath: note.Path}
	lookupPath := note.Path
	if strings.TrimSpace(update.OldPath) != "" {
		lookupPath = filepath.ToSlash(filepath.Clean(update.OldPath))
	}
	readQ := query.Use(db)
	existing, existingFound, lookupErr := firstNoteByPath(readQ, lookupPath)
	if lookupErr != nil {
		return IncrementalResult{}, lookupErr
	}
	if existingFound && existing.SourceHash == hash && existing.ModifiedUnix == update.ModifiedUnix && existing.Size == update.Size {
		result.Skipped = true
		return result, nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		oldKeys := map[string]bool{}
		if existingFound {
			oldKeys = linkTargetKeysFromRecord(*existing)
		}
		if update.OldPath != "" && update.OldPath != note.Path {
			if err := deleteNoteProjection(tx, update.OldPath, true); err != nil {
				return err
			}
		}
		record := noteRecordFromDomain(note, update.ModifiedUnix, update.Size)
		if err := query.Use(tx).NoteRecord.WithContext(context.Background()).Save(&record); err != nil {
			return err
		}
		if err := replaceNoteProjection(tx, root, note); err != nil {
			return err
		}
		keys := mergeLinkTargetKeys(oldKeys, linkTargetKeysFromRecord(record))
		if err := reclassifyAffectedLinkEdges(tx, keys, note.Path); err != nil {
			return err
		}
		return rebuildDimensionCountsFromIndex(tx)
	})
	if err != nil {
		return IncrementalResult{}, err
	}
	result.Parsed = 1
	result.Indexed = 1
	return result, nil
}

func DeleteNote(root string, del NoteDelete) (IncrementalResult, error) {
	db, err := open(root)
	if err != nil {
		return IncrementalResult{}, err
	}
	if err := migrate(db); err != nil {
		return IncrementalResult{}, err
	}
	path := filepath.ToSlash(filepath.Clean(del.Path))
	result := IncrementalResult{NotePath: path}
	err = db.Transaction(func(tx *gorm.DB) error {
		q := query.Use(tx)
		existing, found, lookupErr := firstNoteByPath(q, path)
		if lookupErr != nil {
			return lookupErr
		}
		if !found {
			return nil
		}
		keys := linkTargetKeysFromRecord(*existing)
		if err := deleteNoteProjection(tx, path, true); err != nil {
			return err
		}
		if err := reclassifyAffectedLinkEdges(tx, keys, ""); err != nil {
			return err
		}
		return rebuildDimensionCountsFromIndex(tx)
	})
	if err != nil {
		return IncrementalResult{}, err
	}
	result.Indexed = 1
	return result, nil
}

// firstNoteByPath 通过主键路径查找 note 投影。未找到时返回 (nil, false, nil)。
func firstNoteByPath(q *query.Query, path string) (*model.NoteRecord, bool, error) {
	record, err := q.NoteRecord.WithContext(context.Background()).Where(q.NoteRecord.Path.Eq(path)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func ReplaceAssetProjection(root string, assets []domain.Asset) error {
	db, err := open(root)
	if err != nil {
		return err
	}
	if err := migrate(db); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		q := query.Use(tx)
		if _, err := q.AssetRecord.WithContext(ctx).Session(globalUpdate()).Delete(); err != nil {
			return err
		}
		if err := upsertMeta(tx, "schema_version", SchemaVersion, now); err != nil {
			return err
		}
		if err := upsertMeta(tx, "property_schema_version", PropertySchemaVersion, now); err != nil {
			return err
		}
		for _, asset := range assets {
			if err := q.AssetRecord.WithContext(ctx).Create(assetRecordFromDomain(asset)); err != nil {
				return err
			}
		}
		return nil
	})
}

func ListAssets(root string) ([]domain.Asset, Status, error) {
	status, ok, err := assetProjectionReady(root)
	if err != nil || !ok {
		return nil, status, err
	}
	db, err := open(root)
	if err != nil {
		return nil, Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, err
	}
	q := query.Use(db)
	records, err := q.AssetRecord.WithContext(context.Background()).Order(q.AssetRecord.Path).Find()
	if err != nil {
		return nil, status, err
	}
	assets := make([]domain.Asset, 0, len(records))
	for _, record := range records {
		assets = append(assets, assetRecordToDomain(*record))
	}
	return assets, status, nil
}

func FindAsset(root, ref string) (domain.Asset, Status, error) {
	assets, status, err := ListAssets(root)
	if err != nil {
		return domain.Asset{}, status, err
	}
	ref = strings.TrimSpace(filepath.ToSlash(ref))
	for _, asset := range assets {
		if asset.ID == ref || asset.Path == ref || asset.Filename == ref || asset.Stem == ref {
			return asset, status, nil
		}
	}
	return domain.Asset{}, status, os.ErrNotExist
}
func assetProjectionReady(root string) (Status, bool, error) {
	if _, err := os.Stat(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		if os.IsNotExist(err) {
			return Status{Status: "missing", Path: indexRelPath()}, false, nil
		}
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, false, err
	}
	db, err := open(root)
	if err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, false, err
	}
	if err := migrate(db); err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, false, err
	}
	schema := metaValue(db, "schema_version")
	if schema != SchemaVersion {
		if schema == "" {
			schema = "missing"
		}
		return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{"schema_version=" + schema}}, false, nil
	}
	return Status{Status: "fresh", Path: indexRelPath(), SchemaVersion: schema}, true, nil
}

func assetRecordFromDomain(asset domain.Asset) *AssetRecord {
	return &AssetRecord{Path: asset.Path, AssetID: asset.ID, Filename: asset.Filename, Stem: asset.Stem, Extension: asset.Extension, MediaType: asset.MediaType, Size: asset.Size, ModifiedUnix: asset.ModifiedUnix, Width: asset.Width, Height: asset.Height, SHA256: asset.SHA256, ManagedStatus: asset.ManagedStatus, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt}
}

func assetRecordToDomain(record AssetRecord) domain.Asset {
	return domain.Asset{ID: record.AssetID, Path: record.Path, Filename: record.Filename, Stem: record.Stem, Extension: record.Extension, MediaType: record.MediaType, Size: record.Size, ModifiedUnix: record.ModifiedUnix, Width: record.Width, Height: record.Height, SHA256: record.SHA256, ManagedStatus: domain.ManagedStatus(record.ManagedStatus), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func ListAssetLinks(root string) ([]AssetLinkRecord, Status, error) {
	status, ok, err := assetProjectionReady(root)
	if err != nil || !ok {
		return nil, status, err
	}
	db, err := open(root)
	if err != nil {
		return nil, Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, err
	}
	q := query.Use(db)
	records, err := q.AssetLinkRecord.WithContext(context.Background()).Order(q.AssetLinkRecord.SourcePath, q.AssetLinkRecord.Line, q.AssetLinkRecord.AssetPath).Find()
	if err != nil {
		return nil, status, err
	}
	out := make([]AssetLinkRecord, 0, len(records))
	for _, record := range records {
		out = append(out, *record)
	}
	return out, status, nil
}

func ListVaultFiles(root string) ([]VaultFileRecord, Status, error) {
	status, ok, err := assetProjectionReady(root)
	if err != nil || !ok {
		return nil, status, err
	}
	db, err := open(root)
	if err != nil {
		return nil, Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, err
	}
	q := query.Use(db)
	records, err := q.VaultFileRecord.WithContext(context.Background()).Order(q.VaultFileRecord.Path).Find()
	if err != nil {
		return nil, status, err
	}
	out := make([]VaultFileRecord, 0, len(records))
	for _, record := range records {
		out = append(out, *record)
	}
	return out, status, nil
}

func ListFolders(root string) ([]FolderRecord, Status, error) {
	status, ok, err := assetProjectionReady(root)
	if err != nil || !ok {
		return nil, status, err
	}
	db, err := open(root)
	if err != nil {
		return nil, Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, err
	}
	q := query.Use(db)
	records, err := q.FolderRecord.WithContext(context.Background()).Order(q.FolderRecord.Path).Find()
	if err != nil {
		return nil, status, err
	}
	out := make([]FolderRecord, 0, len(records))
	for _, record := range records {
		out = append(out, *record)
	}
	return out, status, nil
}

func rebuildVaultObjectProjection(tx *gorm.DB, root string, notes []domain.Note) error {
	ctx := context.Background()
	q := query.Use(tx)
	if _, err := q.VaultFileRecord.WithContext(ctx).Session(globalUpdate()).Delete(); err != nil {
		return err
	}
	if _, err := q.AssetRecord.WithContext(ctx).Session(globalUpdate()).Delete(); err != nil {
		return err
	}
	registeredPaths := registeredNotePaths(notes)
	files, err := scanVaultFiles(root, registeredPaths)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := q.VaultFileRecord.WithContext(ctx).Create(&file); err != nil {
			return err
		}
		if file.ObjectKind == string(domain.VaultObjectKindAsset) {
			asset := vaultFileAssetRecord(file)
			if err := q.AssetRecord.WithContext(ctx).Create(&asset); err != nil {
				return err
			}
		}
	}
	return nil
}

func rebuildFolderProjection(tx *gorm.DB, root string, notes []domain.Note) (int, error) {
	ctx := context.Background()
	q := query.Use(tx)
	if _, err := q.FolderRecord.WithContext(ctx).Session(globalUpdate()).Delete(); err != nil {
		return 0, err
	}
	folders, err := scanFolderRecords(root, notes)
	if err != nil {
		return 0, err
	}
	for _, folder := range folders {
		if err := q.FolderRecord.WithContext(ctx).Create(&folder); err != nil {
			return 0, err
		}
	}
	return len(folders), nil
}

func scanFolderRecords(root string, notes []domain.Note) ([]FolderRecord, error) {
	folders, err := loadRegisteredFolderRecords(root)
	if err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !entry.IsDir() {
			return nil
		}
		if rel == ".pinax" || rel == ".git" {
			return filepath.SkipDir
		}
		record := folders[rel]
		record.Path = rel
		record.Exists = true
		record.Empty = indexDirIsEmpty(path)
		record.Depth = indexFolderDepth(rel)
		if record.ManagedStatus == "" {
			record.ManagedStatus = string(domain.ManagedStatusAdoptable)
		}
		folders[rel] = record
		return nil
	}); err != nil {
		return nil, err
	}
	registeredPaths := registeredNotePaths(notes)
	files, err := scanVaultFiles(root, registeredPaths)
	if err != nil {
		return nil, err
	}
	for _, note := range notes {
		for path, record := range folders {
			if indexFolderContainsPath(path, note.Path) {
				record.NoteCount++
				folders[path] = record
			}
		}
	}
	for _, file := range files {
		if file.ObjectKind != string(domain.VaultObjectKindAsset) {
			continue
		}
		for path, record := range folders {
			if indexFolderContainsPath(path, file.Path) {
				record.AssetCount++
				folders[path] = record
			}
		}
	}
	records := make([]FolderRecord, 0, len(folders))
	for _, record := range folders {
		if record.Path == "" {
			continue
		}
		if record.ManagedStatus == "" {
			record.ManagedStatus = string(domain.ManagedStatusAdoptable)
		}
		if record.Purpose == "" {
			record.Purpose = inferIndexFolderPurpose(record)
		}
		record.Depth = indexFolderDepth(record.Path)
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func loadRegisteredFolderRecords(root string) (map[string]FolderRecord, error) {
	folders := map[string]FolderRecord{}
	b, err := os.ReadFile(filepath.Join(root, ".pinax", "folders.json"))
	if errors.Is(err, os.ErrNotExist) {
		return folders, nil
	}
	if err != nil {
		return folders, err
	}
	registry := domain.FolderRegistry{}
	if err := json.Unmarshal(b, &registry); err != nil {
		return folders, err
	}
	for _, folder := range registry.Folders {
		path := strings.Trim(filepath.ToSlash(folder.Path), "/")
		if path == "" || path == "." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
			continue
		}
		status := string(folder.ManagedStatus)
		if status == "" {
			status = string(domain.ManagedStatusManaged)
		}
		folders[path] = FolderRecord{Path: path, Purpose: string(folder.Purpose), ManagedStatus: status, CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt, Depth: indexFolderDepth(path)}
	}
	return folders, nil
}

func inferIndexFolderPurpose(record FolderRecord) string {
	if record.NoteCount > 0 || record.Path == "notes" || strings.HasPrefix(record.Path, "notes/") {
		return string(domain.FolderPurposeNotes)
	}
	if record.AssetCount > 0 || record.Path == "assets" || strings.HasPrefix(record.Path, "assets/") {
		return string(domain.FolderPurposeAssets)
	}
	return string(domain.FolderPurposeGeneric)
}

func indexFolderDepth(path string) int {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func indexFolderContainsPath(folderPath, objectPath string) bool {
	folderPath = strings.Trim(filepath.ToSlash(folderPath), "/")
	objectPath = strings.Trim(filepath.ToSlash(objectPath), "/")
	return objectPath == folderPath || strings.HasPrefix(objectPath, folderPath+"/")
}

func indexDirIsEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func registeredNotePaths(notes []domain.Note) map[string]bool {
	paths := map[string]bool{}
	for _, note := range notes {
		path := strings.TrimSpace(filepath.ToSlash(note.Path))
		if path != "" {
			paths[path] = true
		}
	}
	return paths
}

func scanVaultFiles(root string, registeredPaths map[string]bool) ([]VaultFileRecord, error) {
	records := []VaultFileRecord{}
	if _, err := os.Stat(root); err != nil {
		return records, err
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".pinax" || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		records = append(records, vaultFileRecord(rel, info, registeredPaths))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func vaultFileRecord(rel string, info os.FileInfo, registeredPaths map[string]bool) VaultFileRecord {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")
	kind := string(domain.VaultObjectKindFile)
	if ext == "md" {
		kind = string(domain.VaultObjectKindNote)
	} else if ext != "" {
		kind = string(domain.VaultObjectKindAsset)
	}
	filename := filepath.Base(rel)
	managedStatus := string(domain.ManagedStatusUnmanaged)
	if registeredPaths[rel] {
		managedStatus = string(domain.ManagedStatusRegistered)
	}
	return VaultFileRecord{Path: rel, Filename: filename, Stem: strings.TrimSuffix(filename, filepath.Ext(filename)), Extension: ext, MediaType: mediaType(rel), Size: info.Size(), ModifiedUnix: info.ModTime().Unix(), ObjectKind: kind, ManagedStatus: managedStatus}
}

func vaultFileAssetRecord(file VaultFileRecord) AssetRecord {
	return AssetRecord{Path: file.Path, Filename: file.Filename, Stem: file.Stem, Extension: file.Extension, MediaType: file.MediaType, Size: file.Size, ModifiedUnix: file.ModifiedUnix, ManagedStatus: file.ManagedStatus}
}

func Search(root string, req SearchRequest) (SearchResult, error) {
	db, err := open(root)
	if err != nil {
		return SearchResult{}, err
	}
	if err := migrate(db); err != nil {
		return SearchResult{}, err
	}
	q := query.Use(db)
	ctx := context.Background()
	allRecords, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return SearchResult{}, err
	}
	records := make([]NoteRecord, 0, len(allRecords))
	for _, record := range allRecords {
		if !record.IsSystem {
			records = append(records, *record)
		}
	}
	tagRows, err := q.TagRecord.WithContext(ctx).Find()
	if err != nil {
		return SearchResult{}, err
	}
	textRows, err := q.NoteTextRecord.WithContext(ctx).Find()
	if err != nil {
		return SearchResult{}, err
	}
	linkRows, err := q.LinkRecord.WithContext(ctx).Find()
	if err != nil {
		return SearchResult{}, err
	}
	attachmentRows, err := q.AttachmentRecord.WithContext(ctx).Find()
	if err != nil {
		return SearchResult{}, err
	}
	tagsByPath := map[string][]string{}
	for _, tag := range tagRows {
		tagsByPath[tag.NotePath] = append(tagsByPath[tag.NotePath], tag.Tag)
	}
	textByPath := map[string]NoteTextRecord{}
	for _, text := range textRows {
		textByPath[text.NotePath] = *text
	}
	linksByPath := map[string][]LinkRecord{}
	for _, link := range linkRows {
		linksByPath[link.NotePath] = append(linksByPath[link.NotePath], *link)
	}
	attachmentsByPath := map[string][]AttachmentRecord{}
	for _, attachment := range attachmentRows {
		attachmentsByPath[attachment.NotePath] = append(attachmentsByPath[attachment.NotePath], *attachment)
	}
	items := make([]ResultItem, 0)
	queryText := strings.ToLower(strings.TrimSpace(req.Query))
	for _, record := range records {
		if !recordMatchesFilters(record, tagsByPath[record.Path], linksByPath[record.Path], attachmentsByPath[record.Path], req) {
			continue
		}
		text := textByPath[record.Path]
		score, fields := scoreRecord(record, text, tagsByPath[record.Path], queryText)
		if queryText != "" && score == 0 {
			continue
		}
		items = append(items, ResultItem{Note: domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Tags: tagsByPath[record.Path], Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, Score: score, MatchedFields: fields, Snippet: snippet(text, queryText), LinkCount: len(linksByPath[record.Path]), AttachmentCount: len(attachmentsByPath[record.Path])})
	}
	sortResults(items, req.Sort)
	total := len(items)
	limit := req.Limit
	if limit <= 0 || limit > total {
		limit = total
	}
	items = items[:limit]
	return SearchResult{Engine: "index", IndexStatus: "fresh", Total: total, Returned: len(items), Results: items}, nil
}

func sortResults(items []ResultItem, mode string) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "relevance"
	}
	sort.Slice(items, func(i, j int) bool {
		a := items[i].Note
		b := items[j].Note
		switch mode {
		case "title":
			if a.Title == b.Title {
				return a.Path < b.Path
			}
			return a.Title < b.Title
		case "path":
			return a.Path < b.Path
		case "created":
			return timestampDesc(a.CreatedAt, b.CreatedAt, a.Path, b.Path)
		case "updated":
			return timestampDesc(a.UpdatedAt, b.UpdatedAt, a.Path, b.Path)
		default:
			if items[i].Score == items[j].Score {
				return a.Path < b.Path
			}
			return items[i].Score > items[j].Score
		}
	})
}

func timestampDesc(a, b, pathA, pathB string) bool {
	at, aErr := parseDate(a)
	bt, bErr := parseDate(b)
	if aErr != nil || bErr != nil || at.Equal(bt) {
		return pathA < pathB
	}
	return at.After(bt)
}

func open(root string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		return nil, err
	}
	return gorm.Open(sqlite.Open(filepath.Join(root, ".pinax", "index.sqlite")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(model.AllModels()...)
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

func replaceNoteProjection(tx *gorm.DB, root string, note domain.Note) error {
	ctx := context.Background()
	q := query.Use(tx)
	if err := deleteNotePathProjection(q, ctx, note.Path); err != nil {
		return err
	}
	if _, err := q.AssetLinkRecord.WithContext(ctx).Where(q.AssetLinkRecord.SourcePath.Eq(note.Path)).Delete(); err != nil {
		return err
	}
	if err := q.NoteTextRecord.WithContext(ctx).Create(&NoteTextRecord{NotePath: note.Path, TitleText: note.Title, BodyText: note.Body, Excerpt: excerpt(note.Body), WordCount: len(tokens(note.Body))}); err != nil {
		return err
	}
	for _, tag := range uniqueTags(note) {
		if err := q.TagRecord.WithContext(ctx).Create(&TagRecord{NotePath: note.Path, Tag: tag}); err != nil {
			return err
		}
	}
	for _, token := range noteTokens(note) {
		if err := q.SearchTokenRecord.WithContext(ctx).Create(&SearchTokenRecord{NotePath: note.Path, Token: token.Token, Field: token.Field, Count: token.Count, Weight: token.Weight}); err != nil {
			return err
		}
	}
	linkRecords, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	records := make([]NoteRecord, 0, len(linkRecords))
	for _, record := range linkRecords {
		records = append(records, *record)
	}
	for _, link := range noteLinks(note, notePathByTitleRecords(records)) {
		if err := q.LinkRecord.WithContext(ctx).Create(&link); err != nil {
			return err
		}
	}
	for _, attachment := range noteAttachments(root, note) {
		if err := q.AttachmentRecord.WithContext(ctx).Create(&attachment); err != nil {
			return err
		}
	}
	for _, assetLink := range noteAssetLinks(root, note) {
		if err := q.AssetLinkRecord.WithContext(ctx).Create(&assetLink); err != nil {
			return err
		}
	}
	return nil
}

// deleteNotePathProjection 删除按 note_path 关联 note 的 projection 行（不含 note 记录本身与 asset link）。
func deleteNotePathProjection(q *query.Query, ctx context.Context, notePath string) error {
	clearers := []func() error{
		func() error {
			_, err := q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.TagRecord.WithContext(ctx).Where(q.TagRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.SearchTokenRecord.WithContext(ctx).Where(q.SearchTokenRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
		func() error {
			_, err := q.AttachmentRecord.WithContext(ctx).Where(q.AttachmentRecord.NotePath.Eq(notePath)).Delete()
			return err
		},
	}
	for _, clear := range clearers {
		if err := clear(); err != nil {
			return err
		}
	}
	return nil
}

func deleteNoteProjection(tx *gorm.DB, path string, includeRecord bool) error {
	ctx := context.Background()
	q := query.Use(tx)
	path = filepath.ToSlash(filepath.Clean(path))
	if includeRecord {
		if _, err := q.NoteRecord.WithContext(ctx).Where(q.NoteRecord.Path.Eq(path)).Delete(); err != nil {
			return err
		}
	}
	if err := deleteNotePathProjection(q, ctx, path); err != nil {
		return err
	}
	if _, err := q.AssetLinkRecord.WithContext(ctx).Where(q.AssetLinkRecord.SourcePath.Eq(path)).Delete(); err != nil {
		return err
	}
	return nil
}
func reclassifyAffectedLinkEdges(tx *gorm.DB, targetKeys map[string]bool, changedPath string) error {
	if len(targetKeys) == 0 {
		return nil
	}
	ctx := context.Background()
	q := query.Use(tx)
	linkRows, err := q.LinkRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	affected := map[string]bool{}
	for _, link := range linkRows {
		if linkMatchesTargetKeys(*link, targetKeys) {
			affected[link.NotePath] = true
		}
	}
	if len(affected) == 0 {
		return nil
	}
	noteRows, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	records := make([]NoteRecord, 0, len(noteRows))
	for _, record := range noteRows {
		records = append(records, *record)
	}
	resolver := notePathByTitleRecords(records)
	paths := make([]string, 0, len(affected))
	for path := range affected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		// 增量目标变化只重算受影响 source note 的 link edges，避免重写正文、token 和其它 projection。
		note, ok, lookupErr := indexedNoteForLinkRebuild(tx, path)
		if lookupErr != nil {
			return lookupErr
		}
		if !ok {
			continue
		}
		if _, err := q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.NotePath.Eq(path)).Delete(); err != nil {
			return err
		}
		for _, link := range noteLinks(note, resolver) {
			if err := q.LinkRecord.WithContext(ctx).Create(&link); err != nil {
				return err
			}
		}
	}
	return nil
}

func indexedNoteForLinkRebuild(tx *gorm.DB, path string) (domain.Note, bool, error) {
	ctx := context.Background()
	q := query.Use(tx)
	record, err := q.NoteRecord.WithContext(ctx).Where(q.NoteRecord.Path.Eq(path)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Note{}, false, nil
		}
		return domain.Note{}, false, err
	}
	text, err := q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(path)).First()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Note{}, false, err
	}
	bodyText := ""
	if text != nil {
		bodyText = text.BodyText
	}
	return domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Body: bodyText, Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, true, nil
}

func linkMatchesTargetKeys(link LinkRecord, targetKeys map[string]bool) bool {
	for _, value := range []string{link.Target, link.TargetRaw, link.TargetPath, link.TargetNoteID, link.TargetTitle} {
		if targetKeys[normalizeLinkTargetKey(value)] {
			return true
		}
	}
	return false
}

func mergeLinkTargetKeys(groups ...map[string]bool) map[string]bool {
	merged := map[string]bool{}
	for _, group := range groups {
		for key := range group {
			if key != "" {
				merged[key] = true
			}
		}
	}
	return merged
}

func linkTargetKeysFromRecord(record NoteRecord) map[string]bool {
	keys := map[string]bool{}
	for _, value := range []string{record.Title, record.NoteID, record.Path, strings.TrimSuffix(filepath.Base(record.Path), filepath.Ext(record.Path))} {
		key := normalizeLinkTargetKey(value)
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func normalizeLinkTargetKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." {
		return ""
	}
	return strings.ToLower(value)
}

func rebuildDimensionCountsFromIndex(tx *gorm.DB) error {
	ctx := context.Background()
	q := query.Use(tx)
	if _, err := q.DimensionCountRecord.WithContext(ctx).Session(globalUpdate()).Delete(); err != nil {
		return err
	}
	records, err := q.NoteRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	tagRows, err := q.TagRecord.WithContext(ctx).Find()
	if err != nil {
		return err
	}
	counts := map[string]map[string]int{"tag": {}, "group": {}, "folder": {}, "kind": {}, "status": {}}
	for _, tag := range tagRows {
		counts["tag"][tag.Tag]++
	}
	for _, record := range records {
		counts["group"][record.Group]++
		counts["folder"][record.Folder]++
		counts["kind"][record.Kind]++
		counts["status"][record.Status]++
	}
	dimensions := make([]DimensionCountRecord, 0)
	for dimension, values := range counts {
		for value, count := range values {
			dimensions = append(dimensions, DimensionCountRecord{Dimension: dimension, Value: value, Count: count})
		}
	}
	sort.Slice(dimensions, func(i, j int) bool {
		if dimensions[i].Dimension == dimensions[j].Dimension {
			return dimensions[i].Value < dimensions[j].Value
		}
		return dimensions[i].Dimension < dimensions[j].Dimension
	})
	for _, dimension := range dimensions {
		if err := q.DimensionCountRecord.WithContext(ctx).Create(&dimension); err != nil {
			return err
		}
	}
	return nil
}

func indexRelPath() string {
	return filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
}

func uniqueTags(note domain.Note) []string {
	seen := map[string]bool{}
	for _, tag := range note.Tags {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag != "" {
			seen[tag] = true
		}
	}
	for _, match := range inlineTagPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) > 2 && match[2] != "" {
			seen[match[2]] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	return tags
}

func wikiLinks(body string) []string {
	seen := map[string]bool{}
	for _, match := range wikiLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			target := strings.TrimSpace(match[1])
			if target != "" {
				seen[target] = true
			}
		}
	}
	links := make([]string, 0, len(seen))
	for link := range seen {
		links = append(links, link)
	}
	return links
}

func noteLinks(note domain.Note, pathByTitle map[string]string) []LinkRecord {
	links := make([]LinkRecord, 0)
	for _, target := range wikiLinks(note.Body) {
		resolved := pathByTitle[strings.ToLower(target)]
		status := "broken"
		evidence := "target not found"
		if resolved != "" {
			status = "resolved"
			evidence = "resolved by title"
		}
		links = append(links, LinkRecord{NotePath: note.Path, SourceNoteID: note.ID, Target: target, TargetRaw: target, TargetPath: resolved, Kind: "wiki", Broken: resolved == "", Status: status, Evidence: evidence})
	}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		cleanPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(note.Path), target)))
		links = append(links, LinkRecord{NotePath: note.Path, SourceNoteID: note.ID, Target: target, TargetRaw: target, TargetPath: cleanPath, Kind: "markdown", Broken: false, Status: "resolved", Evidence: "resolved by relative path"})
	}
	return links
}

func noteAttachments(root string, note domain.Note) []AttachmentRecord {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	attachments := make([]AttachmentRecord, 0, len(links))
	for _, link := range links {
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(link.AssetPath)))
		attachments = append(attachments, AttachmentRecord{NotePath: note.Path, ReferenceText: link.RawReference, TargetPath: link.AssetPath, MediaType: mediaType(link.AssetPath), Exists: statErr == nil})
	}
	return attachments
}

func noteAssetLinks(root string, note domain.Note) []AssetLinkRecord {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	records := make([]AssetLinkRecord, 0, len(links))
	for _, link := range links {
		status := "resolved"
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(link.AssetPath))); err != nil {
			status = "missing"
		}
		records = append(records, AssetLinkRecord{AssetPath: link.AssetPath, SourceNoteID: link.SourceNoteID, SourcePath: link.SourcePath, RawReference: link.RawReference, LinkStyle: link.LinkStyle, LinkKind: link.LinkKind, Line: link.Line, Status: status, MediaType: mediaType(link.AssetPath)})
	}
	return records
}

func noteDimensionCounts(notes []domain.Note) []DimensionCountRecord {
	counts := map[string]map[string]int{
		"tag":    {},
		"group":  {},
		"folder": {},
		"kind":   {},
		"status": {},
	}
	for _, note := range notes {
		for _, tag := range uniqueTags(note) {
			counts["tag"][tag]++
		}
		counts["group"][noteProject(note)]++
		counts["folder"][strings.TrimSpace(note.Folder)]++
		counts["kind"][strings.TrimSpace(note.Kind)]++
		counts["status"][strings.TrimSpace(note.Status)]++
	}
	records := make([]DimensionCountRecord, 0)
	for dimension, values := range counts {
		for value, count := range values {
			records = append(records, DimensionCountRecord{Dimension: dimension, Value: value, Count: count})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Dimension == records[j].Dimension {
			return records[i].Value < records[j].Value
		}
		return records[i].Dimension < records[j].Dimension
	})
	return records
}

func mediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".pdf":
		return "document"
	default:
		return "file"
	}
}

func notePathByTitle(notes []domain.Note) map[string]string {
	paths := map[string]string{}
	for _, note := range notes {
		paths[strings.ToLower(note.Title)] = note.Path
		paths[strings.ToLower(note.ID)] = note.Path
		paths[strings.ToLower(note.Path)] = note.Path
	}
	return paths
}

func notePathByTitleRecords(records []NoteRecord) map[string]string {
	paths := map[string]string{}
	for _, record := range records {
		paths[strings.ToLower(record.Title)] = record.Path
		paths[strings.ToLower(record.NoteID)] = record.Path
		paths[strings.ToLower(record.Path)] = record.Path
	}
	return paths
}

func noteProject(note domain.Note) string {
	if strings.TrimSpace(note.Project) != "" {
		return note.Project
	}
	return projectFromPath(note.Path)
}

func isSystemIndexNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind == "index" {
		return strings.HasPrefix(path, "index/") || strings.HasPrefix(path, "notes/index/") || strings.HasPrefix(path, "notes/daily/")
	}
	if note.Kind == "daily" || note.Kind == "weekly" || note.Kind == "monthly" {
		return strings.HasPrefix(path, note.Kind+"/") || strings.HasPrefix(path, "notes/"+note.Kind+"/")
	}
	return false
}

func inferLifecycleStatus(status, kind string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "inbox", "draft", "active", "archived", "discarded":
		return s
	case "system":
		if kind == "index" {
			return "system"
		}
		return "active"
	default:
		return "active"
	}
}

func noteRecordFromDomain(note domain.Note, modifiedUnix, size int64) NoteRecord {
	filename := filepath.Base(note.Path)
	ext := filepath.Ext(filename)
	return NoteRecord{Path: note.Path, NoteID: note.ID, Title: note.Title, Filename: filename, Stem: strings.TrimSuffix(filename, ext), Project: noteProject(note), Group: noteProject(note), Folder: note.Folder, Kind: note.Kind, Status: note.Status, LifecycleStatus: inferLifecycleStatus(note.Status, note.Kind), CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt, SourceHash: noteSourceHash(note), ModifiedUnix: modifiedUnix, Size: size, IsSystem: isSystemIndexNote(note), ObjectKind: string(domain.VaultObjectKindNote), ManagedStatus: string(domain.ManagedStatusRegistered)}
}

func noteSourceHash(note domain.Note) string {
	parts := []string{note.ID, note.Title, note.Path, strings.Join(note.Tags, ","), note.Body, note.Project, note.Folder, note.Kind, note.Status, note.CreatedAt, note.UpdatedAt}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

type tokenRecord struct {
	Token  string
	Field  string
	Count  int
	Weight int
}

func noteTokens(note domain.Note) []tokenRecord {
	counts := map[string]tokenRecord{}
	add := func(field, text string, weight int) {
		for _, token := range tokens(text) {
			key := field + "\x00" + token
			record := counts[key]
			record.Token = token
			record.Field = field
			record.Weight = weight
			record.Count++
			counts[key] = record
		}
	}
	add("title", note.Title, 5)
	add("tag", strings.Join(note.Tags, " "), 4)
	add("path", note.Path, 2)
	add("body", note.Body, 1)
	records := make([]tokenRecord, 0, len(counts))
	for _, record := range counts {
		records = append(records, record)
	}
	return records
}

func tokens(text string) []string {
	tokens := make([]string, 0)
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, strings.ToLower(b.String()))
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func excerpt(body string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	if len(body) <= 120 {
		return body
	}
	return body[:120]
}

func snippet(text NoteTextRecord, query string) string {
	if query == "" {
		if text.Excerpt != "" {
			return text.Excerpt
		}
		return text.TitleText
	}
	haystack := text.BodyText
	idx := strings.Index(strings.ToLower(haystack), query)
	if idx < 0 {
		return text.TitleText
	}
	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 60
	if end > len(haystack) {
		end = len(haystack)
	}
	return strings.TrimSpace(haystack[start:end])
}

func scoreRecord(record NoteRecord, text NoteTextRecord, tags []string, query string) (int, []string) {
	if query == "" {
		return 1, []string{"filter"}
	}
	score := 0
	fields := make([]string, 0)
	if strings.Contains(strings.ToLower(record.Title), query) {
		score += 50
		fields = append(fields, "title")
	}
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 30
			fields = append(fields, "tag")
			break
		}
	}
	if strings.Contains(strings.ToLower(record.Path), query) {
		score += 10
		fields = append(fields, "path")
	}
	if strings.Contains(strings.ToLower(text.BodyText), query) {
		score += 5
		fields = append(fields, "body")
	}
	return score, fields
}

func recordMatchesFilters(record NoteRecord, tags []string, links []LinkRecord, attachments []AttachmentRecord, req SearchRequest) bool {
	if req.Group != "" && record.Group != req.Group && record.Project != req.Group {
		return false
	}
	if req.Folder != "" && record.Folder != req.Folder {
		return false
	}
	if req.Kind != "" && record.Kind != req.Kind {
		return false
	}
	if req.Status != "" && record.Status != req.Status {
		return false
	}
	if req.CreatedAfter != "" && !timestampAfterOrEqual(record.CreatedAt, req.CreatedAfter) {
		return false
	}
	if req.UpdatedAfter != "" && !timestampAfterOrEqual(record.UpdatedAt, req.UpdatedAfter) {
		return false
	}
	for _, want := range req.Tags {
		if !containsTag(tags, want) {
			return false
		}
	}
	if req.LinkTarget != "" {
		found := false
		for _, link := range links {
			if strings.Contains(strings.ToLower(link.Target), strings.ToLower(req.LinkTarget)) || strings.Contains(strings.ToLower(link.TargetPath), strings.ToLower(req.LinkTarget)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if req.HasAttachment && len(attachments) == 0 {
		return false
	}
	return true
}

func timestampAfterOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.After(boundaryTime)
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func containsTag(tags []string, want string) bool {
	want = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(want)), "#")
	for _, tag := range tags {
		if strings.ToLower(tag) == want {
			return true
		}
	}
	return false
}

func projectFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) >= 3 && parts[0] == "notes" {
		return parts[1]
	}
	return ""
}
