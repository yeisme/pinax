package memory

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	TypeFact     = "fact"
	TypeDecision = "decision"
	TypeEvent    = "event"
	TypeTask     = "task"

	StatusDraft      = "draft"
	StatusConfirmed  = "confirmed"
	StatusSuperseded = "superseded"
	StatusExpired    = "expired"
	StatusRejected   = "rejected"
)

type Record struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	Type         string     `gorm:"index" json:"type"`
	Subject      string     `gorm:"index" json:"subject,omitempty"`
	Predicate    string     `gorm:"index" json:"predicate,omitempty"`
	Object       string     `json:"object,omitempty"`
	Body         string     `json:"body,omitempty"`
	Status       string     `gorm:"index" json:"status"`
	Confidence   string     `gorm:"index" json:"confidence,omitempty"`
	SourceURI    string     `json:"source_uri,omitempty"`
	SourceSpan   string     `json:"source_span,omitempty"`
	SupersedesID string     `json:"supersedes_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func (Record) TableName() string { return "memory_records" }

type Entity struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Kind         string    `gorm:"index" json:"kind"`
	Name         string    `json:"name"`
	CanonicalKey string    `gorm:"uniqueIndex" json:"canonical_key"`
	CreatedAt    time.Time `json:"created_at"`
}

func (Entity) TableName() string { return "memory_entities" }

type RecordEntity struct {
	RecordID  string    `gorm:"primaryKey" json:"record_id"`
	EntityID  string    `gorm:"primaryKey" json:"entity_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (RecordEntity) TableName() string { return "memory_record_entities" }

type Source struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	RecordID  string    `gorm:"index" json:"record_id"`
	URI       string    `json:"uri"`
	Kind      string    `json:"kind"`
	Span      string    `json:"span,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (Source) TableName() string { return "memory_sources" }

type CaptureRequest struct {
	Type       string
	Subject    string
	Predicate  string
	Object     string
	Body       string
	Status     string
	Confidence string
	SourceURI  string
	SourceSpan string
	Entities   []string
	DryRun     bool
}

type ListFilter struct {
	Type              string
	Entity            string
	IncludeDraft      bool
	IncludeSuperseded bool
	IncludeExpired    bool
	IncludeRejected   bool
	Limit             int
}

type RecallFilter struct {
	Query  string
	Entity string
	Type   string
	Limit  int
}

type RecallHit struct {
	Record       Record          `json:"record"`
	RecallReason string          `json:"recall_reason"`
	Score        int             `json:"score"`
	Signals      SignalBreakdown `json:"signals,omitempty"`
}

type Stats struct {
	Records   int64 `json:"records"`
	Confirmed int64 `json:"confirmed"`
	Drafts    int64 `json:"drafts"`
}

type Store struct {
	db *gorm.DB
}

func Open(root string) (*Store, error) {
	dir := filepath.Join(root, ".pinax", "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "ledger.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, memoryStoreError(err)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&Record{}, &Entity{}, &RecordEntity{}, &Source{}); err != nil {
		return memoryStoreError(err)
	}
	// FTS5 is a SQLite projection that GORM cannot model. Keep raw SQL centralized here.
	if err := s.db.WithContext(ctx).Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(record_id UNINDEXED, subject, predicate, object, body, entities)`).Error; err != nil {
		return memoryStoreError(err)
	}
	return nil
}

func (s *Store) Capture(ctx context.Context, req CaptureRequest) (Record, error) {
	record, err := BuildRecord(req)
	if err != nil {
		return Record{}, err
	}
	if req.DryRun {
		return record, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		for _, name := range normalizeEntities(req.Entities, req.Subject) {
			entity := Entity{ID: entityID("project", name), Kind: "project", Name: name, CanonicalKey: canonicalEntityKey("project", name), CreatedAt: record.CreatedAt}
			if err := tx.Where("canonical_key = ?", entity.CanonicalKey).FirstOrCreate(&entity).Error; err != nil {
				return err
			}
			if err := tx.FirstOrCreate(&RecordEntity{RecordID: record.ID, EntityID: entity.ID, CreatedAt: record.CreatedAt}, RecordEntity{RecordID: record.ID, EntityID: entity.ID}).Error; err != nil {
				return err
			}
		}
		if strings.TrimSpace(record.SourceURI) != "" {
			source := Source{ID: record.ID + ":source", RecordID: record.ID, URI: record.SourceURI, Kind: sourceKind(record.SourceURI), Span: record.SourceSpan, CreatedAt: record.CreatedAt}
			if err := tx.Create(&source).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Record{}, memoryStoreError(err)
	}
	if err := s.indexRecord(ctx, record, normalizeEntities(req.Entities, req.Subject)); err != nil {
		return Record{}, err
	}
	return record, nil
}

func BuildRecord(req CaptureRequest) (Record, error) {
	typeName := strings.ToLower(strings.TrimSpace(req.Type))
	if !validType(typeName) {
		return Record{}, &domain.CommandError{Code: "memory_record_invalid", Message: "Memory type is not supported", Hint: "Use --type fact, decision, event, or task"}
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = StatusConfirmed
	}
	if !validStatus(status) {
		return Record{}, &domain.CommandError{Code: "memory_record_invalid", Message: "Memory status is not supported", Hint: "Use confirmed, draft, superseded, expired, or rejected"}
	}
	if strings.TrimSpace(req.Subject) == "" && strings.TrimSpace(req.Object) == "" && strings.TrimSpace(req.Body) == "" {
		return Record{}, &domain.CommandError{Code: "memory_record_invalid", Message: "Memory record needs content", Hint: "Provide --subject and --object, or --body"}
	}
	now := time.Now().UTC()
	record := Record{Type: typeName, Subject: strings.TrimSpace(req.Subject), Predicate: strings.TrimSpace(req.Predicate), Object: strings.TrimSpace(req.Object), Body: boundedBody(req.Body), Status: status, Confidence: defaultString(strings.TrimSpace(req.Confidence), "confirmed"), SourceURI: filepath.ToSlash(strings.TrimSpace(req.SourceURI)), SourceSpan: strings.TrimSpace(req.SourceSpan), CreatedAt: now, UpdatedAt: now}
	record.ID = recordID(record)
	if status == StatusExpired {
		record.ExpiresAt = &now
	}
	return record, nil
}

func (s *Store) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	q := s.db.WithContext(ctx).Model(&Record{})
	q = applyStatusFilter(q, filter)
	if strings.TrimSpace(filter.Type) != "" {
		q = q.Where("type = ?", strings.ToLower(strings.TrimSpace(filter.Type)))
	}
	if strings.TrimSpace(filter.Entity) != "" {
		ids, err := s.recordIDsForEntity(ctx, filter.Entity)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []Record{}, nil
		}
		q = q.Where("id IN ?", ids)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var records []Record
	if err := q.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, memoryStoreError(err)
	}
	return records, nil
}

func (s *Store) Recall(ctx context.Context, filter RecallFilter) ([]RecallHit, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 8
	}
	listFilter := ListFilter{Type: filter.Type, Entity: filter.Entity, Limit: 200}
	records, err := s.List(ctx, listFilter)
	if err != nil {
		return nil, err
	}
	ftsRank := map[string]int{}
	if strings.TrimSpace(filter.Query) != "" {
		ids, err := s.ftsRecordIDs(ctx, filter.Query, limit*4)
		if err == nil {
			for idx, id := range ids {
				ftsRank[id] = len(ids) - idx
			}
		}
	}
	candidates := make([]Candidate, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, Candidate{Record: record, FTSRank: ftsRank[record.ID]})
	}
	return Scorer{}.Score(filter, candidates, ftsRank, limit), nil
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := s.db.WithContext(ctx).Model(&Record{}).Count(&stats.Records).Error; err != nil {
		return Stats{}, memoryStoreError(err)
	}
	if err := s.db.WithContext(ctx).Model(&Record{}).Where("status = ?", StatusConfirmed).Count(&stats.Confirmed).Error; err != nil {
		return Stats{}, memoryStoreError(err)
	}
	if err := s.db.WithContext(ctx).Model(&Record{}).Where("status = ?", StatusDraft).Count(&stats.Drafts).Error; err != nil {
		return Stats{}, memoryStoreError(err)
	}
	return stats, nil
}

func (s *Store) indexRecord(ctx context.Context, record Record, entities []string) error {
	// FTS writes are centralized in the store because SQLite virtual tables are outside GORM's model layer.
	if err := s.db.WithContext(ctx).Exec(`DELETE FROM memory_fts WHERE record_id = ?`, record.ID).Error; err != nil {
		return memoryStoreError(err)
	}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO memory_fts(record_id, subject, predicate, object, body, entities) VALUES (?, ?, ?, ?, ?, ?)`, record.ID, record.Subject, record.Predicate, record.Object, record.Body, strings.Join(entities, " ")).Error; err != nil {
		return memoryStoreError(err)
	}
	return nil
}

func (s *Store) ftsRecordIDs(ctx context.Context, query string, limit int) ([]string, error) {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	match := strings.Join(terms, " OR ")
	type row struct{ RecordID string }
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`SELECT record_id FROM memory_fts WHERE memory_fts MATCH ? LIMIT ?`, match, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RecordID)
	}
	return ids, nil
}

func (s *Store) recordIDsForEntity(ctx context.Context, entity string) ([]string, error) {
	key := canonicalEntityKey("project", entity)
	var found Entity
	if err := s.db.WithContext(ctx).Where("canonical_key = ?", key).First(&found).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, memoryStoreError(err)
	}
	var links []RecordEntity
	if err := s.db.WithContext(ctx).Where("entity_id = ?", found.ID).Find(&links).Error; err != nil {
		return nil, memoryStoreError(err)
	}
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.RecordID)
	}
	return ids, nil
}

func applyStatusFilter(q *gorm.DB, filter ListFilter) *gorm.DB {
	statuses := []string{StatusConfirmed}
	if filter.IncludeDraft {
		statuses = append(statuses, StatusDraft)
	}
	if filter.IncludeSuperseded {
		statuses = append(statuses, StatusSuperseded)
	}
	if filter.IncludeExpired {
		statuses = append(statuses, StatusExpired)
	}
	if filter.IncludeRejected {
		statuses = append(statuses, StatusRejected)
	}
	return q.Where("status IN ?", statuses)
}

func normalizeEntities(values []string, subject string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(values, subject) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func validType(value string) bool {
	switch value {
	case TypeFact, TypeDecision, TypeEvent, TypeTask:
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	switch value {
	case StatusDraft, StatusConfirmed, StatusSuperseded, StatusExpired, StatusRejected:
		return true
	default:
		return false
	}
}

func recordID(record Record) string {
	sum := sha1.Sum([]byte(strings.Join([]string{record.Type, record.Subject, record.Predicate, record.Object, record.Body, record.Status, record.CreatedAt.Format(time.RFC3339Nano)}, "\x00")))
	return "mem_" + hex.EncodeToString(sum[:])[:16]
}

func entityID(kind, name string) string {
	sum := sha1.Sum([]byte(canonicalEntityKey(kind, name)))
	return "ent_" + hex.EncodeToString(sum[:])[:16]
}

func canonicalEntityKey(kind, name string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + ":" + strings.ToLower(strings.TrimSpace(name))
}

func sourceKind(uri string) string {
	switch {
	case strings.Contains(uri, "openspec/"):
		return "openspec"
	case strings.Contains(uri, "docs/"):
		return "docs"
	case strings.HasPrefix(uri, "gh-run:"):
		return "github_actions"
	default:
		return "file"
	}
}

func boundedBody(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 2000 {
		return value
	}
	return value[:2000]
}

func tokenize(value string) []string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	terms := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) < 2 || seen[part] {
			continue
		}
		seen[part] = true
		terms = append(terms, part)
	}
	return terms
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func memoryStoreError(err error) error {
	if err == nil {
		return nil
	}
	return &domain.CommandError{Code: "memory_store_unavailable", Message: "Memory ledger store is unavailable", Hint: fmt.Sprintf("Check .pinax/memory permissions: %v", err)}
}
