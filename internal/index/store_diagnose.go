package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
	"gorm.io/gorm"
)

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
	issues = append(issues, objectIdentityConsistencyIssues(q, records)...)
	statusName := "fresh"
	if len(issues) > 0 {
		statusName = "partial"

		for _, issue := range issues {
			if issue.Code == "index_stale" || issue.Code == "index_schema_mismatch" || issue.Code == "index_identity_consistency" {
				statusName = "stale"
				break
			}
		}
	}
	status := Status{Status: statusName, Path: indexRelPath(), SchemaVersion: schema, Notes: len(records), Evidence: issueEvidence(issues)}
	return doctorReport(status, issues), nil
}

func objectIdentityConsistencyIssues(q *query.Query, notes []*model.NoteRecord) []Issue {
	byPath := map[string]string{}
	for _, note := range notes {
		byPath[note.Path] = note.ObjectID
		if note.ObjectID == "" || note.ObjectID != note.NoteID {
			return []Issue{indexIssue("index_identity_consistency", "error", indexRelPath(), "索引 object/path projection 不一致", []string{"identity_consistency=failed", "note_path=" + note.Path})}
		}
	}
	ctx := context.Background()
	tags, err := q.TagRecord.WithContext(ctx).Find()
	if err != nil {
		return []Issue{indexIssue("index_identity_consistency", "error", indexRelPath(), "索引 identity consistency 不可读", []string{"identity_consistency=failed"})}
	}
	for _, row := range tags {
		if row.ObjectID != byPath[row.NotePath] {
			return []Issue{indexIssue("index_identity_consistency", "error", indexRelPath(), "Tag projection object identity 不一致", []string{"identity_consistency=failed", "note_path=" + row.NotePath})}
		}
	}
	properties, err := q.PropertyValueRecord.WithContext(ctx).Find()
	if err != nil {
		return []Issue{indexIssue("index_identity_consistency", "error", indexRelPath(), "索引 identity consistency 不可读", []string{"identity_consistency=failed"})}
	}
	for _, row := range properties {
		if row.ObjectID != byPath[row.NotePath] {
			return []Issue{indexIssue("index_identity_consistency", "error", indexRelPath(), "Property projection object identity 不一致", []string{"identity_consistency=failed", "note_path=" + row.NotePath})}
		}
	}
	return nil
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
