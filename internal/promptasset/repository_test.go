package promptasset

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/index/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPromptAssetRepositoryCreateResolveSearchAndLifecycle(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()
	asset := loadFixture(t, "valid.yaml")

	created, err := repo.Create(ctx, asset)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.PromptAssetID != asset.ID || created.CurrentVersionID == "" || created.PromptTemplateHash == "" {
		t.Fatalf("created record incomplete: %#v", created)
	}

	resolved, err := repo.Resolve(ctx, "pinax://prompt/"+asset.ID)
	if err != nil {
		t.Fatalf("resolve uri: %v", err)
	}
	if resolved.PromptAssetID != asset.ID || resolved.Permission != "internal" {
		t.Fatalf("resolved record = %#v", resolved)
	}

	updated, err := repo.UpdateLifecycle(ctx, asset.ID, "tested")
	if err != nil {
		t.Fatalf("update lifecycle: %v", err)
	}
	if updated.Lifecycle != "tested" {
		t.Fatalf("lifecycle = %q, want tested", updated.Lifecycle)
	}

	results, err := repo.Search(ctx, SearchRequest{Query: "portrait", Domain: "visual_generation", Tag: "character", Lifecycle: "tested"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].PromptAssetID != asset.ID {
		t.Fatalf("search results = %#v", results)
	}
}

func TestPromptAssetRepositoryImportFeedbackIsIdempotent(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()
	asset := loadFixture(t, "valid.yaml")
	created, err := repo.Create(ctx, asset)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	feedback := Feedback{
		FeedbackID:         "feedback_eikona_001",
		PromptAssetID:      asset.ID,
		VersionID:          created.CurrentVersionID,
		PromptTemplateHash: created.PromptTemplateHash,
		ExternalRunRef:     "eikona://run/001",
		Decision:           "accepted",
		Reason:             "fixture render passed",
		ArtifactRefs:       []string{"eikona://artifact/001"},
	}

	first, err := repo.ImportFeedback(ctx, feedback)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if !first.Imported {
		t.Fatalf("first import marked duplicate: %#v", first)
	}
	second, err := repo.ImportFeedback(ctx, feedback)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Imported {
		t.Fatalf("second import created duplicate: %#v", second)
	}
	if second.Record.FeedbackID != feedback.FeedbackID {
		t.Fatalf("duplicate record = %#v", second.Record)
	}
}

func TestPromptAssetRepositoryRejectsInvalidLifecycle(t *testing.T) {
	repo := newTestRepository(t)
	_, err := repo.UpdateLifecycle(context.Background(), "missing", "production")
	if err == nil || !strings.Contains(err.Error(), "lifecycle") {
		t.Fatalf("update lifecycle error = %v, want invalid lifecycle", err)
	}
}

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/index.sqlite"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepository(db)
	repo.SetNow(func() time.Time { return time.Date(2026, 6, 18, 4, 0, 0, 0, time.UTC) })
	return repo
}
