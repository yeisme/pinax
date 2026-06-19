package promptasset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Repository struct {
	q   *query.Query
	now func() time.Time
}

type SearchRequest struct {
	Query     string
	Domain    string
	Tag       string
	Lifecycle string
	Limit     int
}

type Feedback struct {
	FeedbackID         string
	PromptAssetID      string
	VersionID          string
	PromptTemplateHash string
	ExternalRunRef     string
	Decision           string
	Reason             string
	ArtifactRefs       []string
}

type FeedbackImportResult struct {
	Imported bool
	Record   model.PromptUsageFeedbackRecord
}

type AssetDetails struct {
	Asset      model.PromptAssetRecord            `json:"asset"`
	Version    model.PromptAssetVersionRecord     `json:"version,omitempty"`
	SourceRefs []model.PromptAssetSourceRefRecord `json:"source_refs,omitempty"`
}

func OpenVaultRepository(root string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, ".pinax", "index.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return nil, err
	}
	return NewRepository(db), nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{q: query.Use(db), now: func() time.Time { return time.Now().UTC() }}
}

func (r *Repository) SetNow(now func() time.Time) {
	if now != nil {
		r.now = now
	}
}

func (r *Repository) Create(ctx context.Context, asset Asset) (model.PromptAssetRecord, error) {
	if err := Validate(asset); err != nil {
		return model.PromptAssetRecord{}, err
	}
	now := r.now().UTC().Format(time.RFC3339)
	lifecycle := strings.TrimSpace(asset.Lifecycle)
	if lifecycle == "" {
		lifecycle = "draft"
	}
	hash := contentHash(asset.PromptTemplate)
	versionID := asset.ID + "@" + hash[:12]
	variables, err := marshalJSONString(asset.Variables)
	if err != nil {
		return model.PromptAssetRecord{}, err
	}
	constraints, err := marshalJSONString(asset.Constraints)
	if err != nil {
		return model.PromptAssetRecord{}, err
	}
	tags, err := marshalJSONString(asset.Tags)
	if err != nil {
		return model.PromptAssetRecord{}, err
	}

	record := model.PromptAssetRecord{
		PromptAssetID:      asset.ID,
		SchemaVersion:      asset.SchemaVersion,
		Title:              asset.Title,
		Domain:             asset.Domain,
		Lifecycle:          lifecycle,
		Permission:         asset.Permission,
		OwnerProject:       asset.OwnerProject,
		CurrentVersionID:   versionID,
		PromptTemplateHash: hash,
		TagsJSON:           tags,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	version := model.PromptAssetVersionRecord{
		VersionID:          versionID,
		PromptAssetID:      asset.ID,
		PromptTemplate:     asset.PromptTemplate,
		PromptTemplateHash: hash,
		VariablesJSON:      variables,
		ConstraintsJSON:    constraints,
		ReviewGuidance:     asset.ReviewGuidance,
		CreatedAt:          now,
	}

	err = r.q.UnderlyingDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		qt := query.Use(tx)
		if err := qt.PromptAssetRecord.WithContext(ctx).Create(&record); err != nil {
			return err
		}
		if err := qt.PromptAssetVersionRecord.WithContext(ctx).Create(&version); err != nil {
			return err
		}
		for _, source := range asset.SourceRefs {
			ref := model.PromptAssetSourceRefRecord{PromptAssetID: asset.ID, VersionID: versionID, URI: source.URI, Label: source.Label, Evidence: source.Evidence}
			if err := qt.PromptAssetSourceRefRecord.WithContext(ctx).Create(&ref); err != nil {
				return err
			}
		}
		return nil
	})
	return record, err
}

func (r *Repository) UpdateLifecycle(ctx context.Context, assetID, lifecycle string) (model.PromptAssetRecord, error) {
	if _, ok := allowedLifecycles[lifecycle]; !ok {
		return model.PromptAssetRecord{}, fmt.Errorf("lifecycle must be one of draft, tested, accepted, promoted, retired")
	}
	p := r.q.PromptAssetRecord
	record, err := p.WithContext(ctx).Where(p.PromptAssetID.Eq(assetID)).First()
	if err != nil {
		return model.PromptAssetRecord{}, err
	}
	record.Lifecycle = lifecycle
	record.UpdatedAt = r.now().UTC().Format(time.RFC3339)
	return *record, p.WithContext(ctx).Save(record)
}

func (r *Repository) Resolve(ctx context.Context, ref string) (model.PromptAssetRecord, error) {
	assetID := strings.TrimPrefix(strings.TrimSpace(ref), "pinax://prompt/")
	if assetID == "" {
		return model.PromptAssetRecord{}, errors.New("prompt asset id is required")
	}
	p := r.q.PromptAssetRecord
	record, err := p.WithContext(ctx).Where(p.PromptAssetID.Eq(assetID)).First()
	if err != nil {
		return model.PromptAssetRecord{}, err
	}
	return *record, nil
}

func (r *Repository) Details(ctx context.Context, ref string) (AssetDetails, error) {
	record, err := r.Resolve(ctx, ref)
	if err != nil {
		return AssetDetails{}, err
	}
	details := AssetDetails{Asset: record}
	if record.CurrentVersionID != "" {
		v := r.q.PromptAssetVersionRecord
		version, err := v.WithContext(ctx).Where(v.VersionID.Eq(record.CurrentVersionID)).First()
		if err == nil {
			details.Version = *version
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return AssetDetails{}, err
		}
	}
	s := r.q.PromptAssetSourceRefRecord
	sources, err := s.WithContext(ctx).Where(s.PromptAssetID.Eq(record.PromptAssetID)).Find()
	if err != nil {
		return AssetDetails{}, err
	}
	for _, source := range sources {
		details.SourceRefs = append(details.SourceRefs, *source)
	}
	return details, nil
}

func (r *Repository) Search(ctx context.Context, req SearchRequest) ([]model.PromptAssetRecord, error) {
	p := r.q.PromptAssetRecord
	records, err := p.WithContext(ctx).Find()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(req.Query))
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	results := make([]model.PromptAssetRecord, 0, len(records))
	for _, record := range records {
		if req.Domain != "" && record.Domain != req.Domain {
			continue
		}
		if req.Lifecycle != "" && record.Lifecycle != req.Lifecycle {
			continue
		}
		if tag != "" && !containsJSONText(record.TagsJSON, tag) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(record.PromptAssetID+" "+record.Title+" "+record.Domain+" "+record.OwnerProject), needle) {
			continue
		}
		results = append(results, *record)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].PromptAssetID < results[j].PromptAssetID })
	if req.Limit > 0 && len(results) > req.Limit {
		results = results[:req.Limit]
	}
	return results, nil
}

func (r *Repository) ImportFeedback(ctx context.Context, feedback Feedback) (FeedbackImportResult, error) {
	if strings.TrimSpace(feedback.FeedbackID) == "" {
		return FeedbackImportResult{}, errors.New("feedback_id is required")
	}
	if strings.TrimSpace(feedback.PromptAssetID) == "" {
		return FeedbackImportResult{}, errors.New("prompt_asset_id is required")
	}
	f := r.q.PromptUsageFeedbackRecord
	existing, err := f.WithContext(ctx).Where(f.FeedbackID.Eq(feedback.FeedbackID)).First()
	if err == nil {
		return FeedbackImportResult{Imported: false, Record: *existing}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return FeedbackImportResult{}, err
	}
	artifacts, err := marshalJSONString(feedback.ArtifactRefs)
	if err != nil {
		return FeedbackImportResult{}, err
	}
	record := model.PromptUsageFeedbackRecord{
		FeedbackID:         feedback.FeedbackID,
		PromptAssetID:      feedback.PromptAssetID,
		VersionID:          feedback.VersionID,
		PromptTemplateHash: feedback.PromptTemplateHash,
		ExternalRunRef:     feedback.ExternalRunRef,
		Decision:           feedback.Decision,
		Reason:             feedback.Reason,
		ArtifactRefsJSON:   artifacts,
		ImportedAt:         r.now().UTC().Format(time.RFC3339),
	}
	if err := f.WithContext(ctx).Create(&record); err != nil {
		return FeedbackImportResult{}, err
	}
	return FeedbackImportResult{Imported: true, Record: record}, nil
}

func marshalJSONString(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func contentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func containsJSONText(raw, needle string) bool {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		for _, value := range values {
			if strings.ToLower(value) == needle {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(raw), needle)
}
