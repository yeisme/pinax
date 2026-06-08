package index

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const SchemaVersion = "pinax.index.v2"

type IndexMetaRecord struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt string
}

type NoteRecord struct {
	Path         string `gorm:"primaryKey"`
	NoteID       string `gorm:"index"`
	Title        string
	Project      string
	Group        string `gorm:"index"`
	Folder       string `gorm:"index"`
	Kind         string `gorm:"index"`
	Status       string `gorm:"index"`
	CreatedAt    string
	UpdatedAt    string
	SourceHash   string
	ModifiedUnix int64
	Size         int64
	IsSystem     bool `gorm:"index"`
}

type NoteTextRecord struct {
	NotePath  string `gorm:"primaryKey"`
	TitleText string
	BodyText  string
	Excerpt   string
	WordCount int
}

type TagRecord struct {
	ID       uint `gorm:"primaryKey"`
	NotePath string
	Tag      string `gorm:"index"`
}

type LinkRecord struct {
	ID            uint `gorm:"primaryKey"`
	NotePath      string
	Target        string `gorm:"index"`
	TargetPath    string `gorm:"index"`
	Kind          string
	Broken        bool `gorm:"index"`
	SourceNoteID  string
	TargetNoteID  string
	TargetTitle   string
	TargetRaw     string
	TargetAlias   string
	TargetHeading string
	Status        string `gorm:"index"` // resolved|broken|ambiguous|external|ignored
	Line          int
	Evidence      string
}

type SearchTokenRecord struct {
	ID       uint   `gorm:"primaryKey"`
	Token    string `gorm:"index"`
	NotePath string `gorm:"index"`
	Field    string
	Count    int
	Weight   int
}

type AttachmentRecord struct {
	ID            uint `gorm:"primaryKey"`
	NotePath      string
	ReferenceText string
	TargetPath    string `gorm:"index"`
	MediaType     string
	Exists        bool `gorm:"index"`
}

type DimensionCountRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Dimension string `gorm:"index"`
	Value     string `gorm:"index"`
	Count     int
}

type Counts struct {
	Notes       int
	Tags        int
	Links       int
	Tokens      int
	Attachments int
	Dimensions  int
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
	path := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Status{Status: "missing", Path: indexRelPath()}, nil
		}
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, nil
	}
	db, err := open(root)
	if err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, nil
	}
	if err := migrate(db); err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), Evidence: []string{err.Error()}}, nil
	}
	schema := metaValue(db, "schema_version")
	if schema == "" {
		return Status{Status: "stale", Path: indexRelPath(), Evidence: []string{"schema_version=missing"}}, nil
	}
	if schema != SchemaVersion {
		return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{"schema_version=" + schema}}, nil
	}
	records := []NoteRecord{}
	if err := db.Find(&records).Error; err != nil {
		return Status{Status: "unreadable", Path: indexRelPath(), SchemaVersion: schema, Evidence: []string{err.Error()}}, nil
	}
	byPath := map[string]NoteRecord{}
	for _, record := range records {
		byPath[record.Path] = record
	}
	for _, note := range notes {
		record, ok := byPath[note.Path]
		if !ok {
			return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Notes: len(records), Evidence: []string{"missing_note=" + note.Path}}, nil
		}
		if record.SourceHash != noteSourceHash(note) {
			return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Notes: len(records), Evidence: []string{"changed_note=" + note.Path}}, nil
		}
	}
	if len(records) != len(notes) {
		return Status{Status: "stale", Path: indexRelPath(), SchemaVersion: schema, Notes: len(records), Evidence: []string{fmt.Sprintf("indexed_notes=%d", len(records)), fmt.Sprintf("vault_notes=%d", len(notes))}}, nil
	}
	return Status{Status: "fresh", Path: indexRelPath(), SchemaVersion: schema, Notes: len(records)}, nil
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
		for _, model := range []any{&NoteRecord{}, &NoteTextRecord{}, &TagRecord{}, &LinkRecord{}, &SearchTokenRecord{}, &AttachmentRecord{}, &DimensionCountRecord{}} {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(model).Error; err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := upsertMeta(tx, "schema_version", SchemaVersion, now); err != nil {
			return err
		}
		if err := upsertMeta(tx, "rebuilt_at", now, now); err != nil {
			return err
		}
		pathByTitle := notePathByTitle(notes)
		for _, note := range notes {
			record := NoteRecord{Path: note.Path, NoteID: note.ID, Title: note.Title, Project: noteProject(note), Group: noteProject(note), Folder: note.Folder, Kind: note.Kind, Status: note.Status, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt, SourceHash: noteSourceHash(note), IsSystem: isSystemIndexNote(note)}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			counts.Notes++
			if err := tx.Create(&NoteTextRecord{NotePath: note.Path, TitleText: note.Title, BodyText: note.Body, Excerpt: excerpt(note.Body), WordCount: len(tokens(note.Body))}).Error; err != nil {
				return err
			}
			for _, tag := range uniqueTags(note) {
				if err := tx.Create(&TagRecord{NotePath: note.Path, Tag: tag}).Error; err != nil {
					return err
				}
				counts.Tags++
			}
			for _, token := range noteTokens(note) {
				if err := tx.Create(&SearchTokenRecord{NotePath: note.Path, Token: token.Token, Field: token.Field, Count: token.Count, Weight: token.Weight}).Error; err != nil {
					return err
				}
				counts.Tokens++
			}
			for _, link := range noteLinks(note, pathByTitle) {
				if err := tx.Create(&link).Error; err != nil {
					return err
				}
				counts.Links++
			}
			for _, attachment := range noteAttachments(note) {
				if err := tx.Create(&attachment).Error; err != nil {
					return err
				}
				counts.Attachments++
			}
		}
		for _, dimension := range noteDimensionCounts(notes) {
			if err := tx.Create(&dimension).Error; err != nil {
				return err
			}
			counts.Dimensions++
		}
		return nil
	})
	if err != nil {
		return Counts{}, err
	}
	return counts, nil
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
	var existing NoteRecord
	lookupPath := note.Path
	if strings.TrimSpace(update.OldPath) != "" {
		lookupPath = filepath.ToSlash(filepath.Clean(update.OldPath))
	}
	err = db.First(&existing, &NoteRecord{Path: lookupPath}).Error
	if err == nil && existing.SourceHash == hash && existing.ModifiedUnix == update.ModifiedUnix && existing.Size == update.Size {
		result.Skipped = true
		return result, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return IncrementalResult{}, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		oldKeys := linkTargetKeysFromRecord(existing)
		if update.OldPath != "" && update.OldPath != note.Path {
			if err := deleteNoteProjection(tx, update.OldPath, true); err != nil {
				return err
			}
		}
		record := NoteRecord{Path: note.Path, NoteID: note.ID, Title: note.Title, Project: noteProject(note), Group: noteProject(note), Folder: note.Folder, Kind: note.Kind, Status: note.Status, CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt, SourceHash: hash, ModifiedUnix: update.ModifiedUnix, Size: update.Size, IsSystem: isSystemIndexNote(note)}
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		if err := replaceNoteProjection(tx, note); err != nil {
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
		var existing NoteRecord
		if err := tx.First(&existing, &NoteRecord{Path: path}).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		keys := linkTargetKeysFromRecord(existing)
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

func Search(root string, req SearchRequest) (SearchResult, error) {
	db, err := open(root)
	if err != nil {
		return SearchResult{}, err
	}
	if err := migrate(db); err != nil {
		return SearchResult{}, err
	}
	allRecords := []NoteRecord{}
	if err := db.Find(&allRecords).Error; err != nil {
		return SearchResult{}, err
	}
	records := make([]NoteRecord, 0, len(allRecords))
	for _, record := range allRecords {
		if !record.IsSystem {
			records = append(records, record)
		}
	}
	tags := []TagRecord{}
	if err := db.Find(&tags).Error; err != nil {
		return SearchResult{}, err
	}
	texts := []NoteTextRecord{}
	if err := db.Find(&texts).Error; err != nil {
		return SearchResult{}, err
	}
	links := []LinkRecord{}
	if err := db.Find(&links).Error; err != nil {
		return SearchResult{}, err
	}
	attachments := []AttachmentRecord{}
	if err := db.Find(&attachments).Error; err != nil {
		return SearchResult{}, err
	}
	tagsByPath := map[string][]string{}
	for _, tag := range tags {
		tagsByPath[tag.NotePath] = append(tagsByPath[tag.NotePath], tag.Tag)
	}
	textByPath := map[string]NoteTextRecord{}
	for _, text := range texts {
		textByPath[text.NotePath] = text
	}
	linksByPath := map[string][]LinkRecord{}
	for _, link := range links {
		linksByPath[link.NotePath] = append(linksByPath[link.NotePath], link)
	}
	attachmentsByPath := map[string][]AttachmentRecord{}
	for _, attachment := range attachments {
		attachmentsByPath[attachment.NotePath] = append(attachmentsByPath[attachment.NotePath], attachment)
	}
	items := make([]ResultItem, 0)
	query := strings.ToLower(strings.TrimSpace(req.Query))
	for _, record := range records {
		if !recordMatchesFilters(record, tagsByPath[record.Path], linksByPath[record.Path], attachmentsByPath[record.Path], req) {
			continue
		}
		text := textByPath[record.Path]
		score, fields := scoreRecord(record, text, tagsByPath[record.Path], query)
		if query != "" && score == 0 {
			continue
		}
		items = append(items, ResultItem{Note: domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Tags: tagsByPath[record.Path], Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, Score: score, MatchedFields: fields, Snippet: snippet(text, query), LinkCount: len(linksByPath[record.Path]), AttachmentCount: len(attachmentsByPath[record.Path])})
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
	return gorm.Open(sqlite.Open(filepath.Join(root, ".pinax", "index.sqlite")), &gorm.Config{})
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(&IndexMetaRecord{}, &NoteRecord{}, &NoteTextRecord{}, &TagRecord{}, &LinkRecord{}, &SearchTokenRecord{}, &AttachmentRecord{}, &DimensionCountRecord{})
}

func upsertMeta(db *gorm.DB, key, value, now string) error {
	return db.Save(&IndexMetaRecord{Key: key, Value: value, UpdatedAt: now}).Error
}

func metaValue(db *gorm.DB, key string) string {
	var record IndexMetaRecord
	if err := db.First(&record, &IndexMetaRecord{Key: key}).Error; err != nil {
		return ""
	}
	return record.Value
}

func replaceNoteProjection(tx *gorm.DB, note domain.Note) error {
	for _, target := range []any{&NoteTextRecord{}, &TagRecord{}, &LinkRecord{}, &SearchTokenRecord{}, &AttachmentRecord{}} {
		if err := tx.Where("note_path = ?", note.Path).Delete(target).Error; err != nil {
			return err
		}
	}
	if err := tx.Create(&NoteTextRecord{NotePath: note.Path, TitleText: note.Title, BodyText: note.Body, Excerpt: excerpt(note.Body), WordCount: len(tokens(note.Body))}).Error; err != nil {
		return err
	}
	for _, tag := range uniqueTags(note) {
		if err := tx.Create(&TagRecord{NotePath: note.Path, Tag: tag}).Error; err != nil {
			return err
		}
	}
	for _, token := range noteTokens(note) {
		if err := tx.Create(&SearchTokenRecord{NotePath: note.Path, Token: token.Token, Field: token.Field, Count: token.Count, Weight: token.Weight}).Error; err != nil {
			return err
		}
	}
	records := []NoteRecord{}
	if err := tx.Find(&records).Error; err != nil {
		return err
	}
	for _, link := range noteLinks(note, notePathByTitleRecords(records)) {
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
	}
	for _, attachment := range noteAttachments(note) {
		if err := tx.Create(&attachment).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteNoteProjection(tx *gorm.DB, path string, includeRecord bool) error {
	path = filepath.ToSlash(filepath.Clean(path))
	if includeRecord {
		if err := tx.Where("path = ?", path).Delete(&NoteRecord{}).Error; err != nil {
			return err
		}
	}
	for _, target := range []any{&NoteTextRecord{}, &TagRecord{}, &LinkRecord{}, &SearchTokenRecord{}, &AttachmentRecord{}} {
		if err := tx.Where("note_path = ?", path).Delete(target).Error; err != nil {
			return err
		}
	}
	return nil
}

func reclassifyAffectedLinkEdges(tx *gorm.DB, targetKeys map[string]bool, changedPath string) error {
	if len(targetKeys) == 0 {
		return nil
	}
	links := []LinkRecord{}
	if err := tx.Find(&links).Error; err != nil {
		return err
	}
	affected := map[string]bool{}
	for _, link := range links {
		if linkMatchesTargetKeys(link, targetKeys) {
			affected[link.NotePath] = true
		}
	}
	if len(affected) == 0 {
		return nil
	}
	records := []NoteRecord{}
	if err := tx.Find(&records).Error; err != nil {
		return err
	}
	resolver := notePathByTitleRecords(records)
	paths := make([]string, 0, len(affected))
	for path := range affected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		// 增量目标变化只重算受影响 source note 的 link edges，避免重写正文、token 和其它 projection。
		note, ok, err := indexedNoteForLinkRebuild(tx, path)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := tx.Where("note_path = ?", path).Delete(&LinkRecord{}).Error; err != nil {
			return err
		}
		for _, link := range noteLinks(note, resolver) {
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func indexedNoteForLinkRebuild(tx *gorm.DB, path string) (domain.Note, bool, error) {
	var record NoteRecord
	if err := tx.First(&record, &NoteRecord{Path: path}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Note{}, false, nil
		}
		return domain.Note{}, false, err
	}
	var text NoteTextRecord
	if err := tx.First(&text, &NoteTextRecord{NotePath: path}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Note{}, false, err
	}
	return domain.Note{ID: record.NoteID, Title: record.Title, Path: record.Path, Body: text.BodyText, Project: record.Project, Folder: record.Folder, Kind: record.Kind, Status: record.Status, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, true, nil
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
	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&DimensionCountRecord{}).Error; err != nil {
		return err
	}
	records := []NoteRecord{}
	if err := tx.Find(&records).Error; err != nil {
		return err
	}
	tags := []TagRecord{}
	if err := tx.Find(&tags).Error; err != nil {
		return err
	}
	counts := map[string]map[string]int{"tag": {}, "group": {}, "folder": {}, "kind": {}, "status": {}}
	for _, tag := range tags {
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
		if err := tx.Create(&dimension).Error; err != nil {
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
		links = append(links, LinkRecord{
			NotePath:     note.Path,
			SourceNoteID: note.ID,
			Target:       target,
			TargetRaw:    target,
			TargetPath:   resolved,
			Kind:         "wiki",
			Broken:       resolved == "",
			Status:       status,
			Evidence:     evidence,
		})
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
		links = append(links, LinkRecord{
			NotePath:     note.Path,
			SourceNoteID: note.ID,
			Target:       target,
			TargetRaw:    target,
			TargetPath:   cleanPath,
			Kind:         "markdown",
			Broken:       false,
			Status:       "resolved",
			Evidence:     "resolved by relative path",
		})
	}
	return links
}

func noteAttachments(note domain.Note) []AttachmentRecord {
	attachments := make([]AttachmentRecord, 0)
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasSuffix(strings.ToLower(target), ".md") {
			continue
		}
		attachments = append(attachments, AttachmentRecord{NotePath: note.Path, ReferenceText: target, TargetPath: filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(note.Path), target))), MediaType: mediaType(target), Exists: true})
	}
	return attachments
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
	return note.Kind == "index" && strings.HasPrefix(filepath.ToSlash(note.Path), "notes/daily/")
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
