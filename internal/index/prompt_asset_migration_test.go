package index

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/index/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPromptAssetModelsAutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/index.sqlite"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}

	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	migrator := db.Migrator()
	for _, table := range []any{
		&model.PromptAssetRecord{},
		&model.PromptAssetVersionRecord{},
		&model.PromptAssetSourceRefRecord{},
		&model.PromptUsageFeedbackRecord{},
	} {
		if !migrator.HasTable(table) {
			t.Fatalf("missing migrated table for %T", table)
		}
	}

	for _, column := range []string{"prompt_asset_id", "schema_version", "domain", "lifecycle", "permission", "current_version_id"} {
		if !migrator.HasColumn(&model.PromptAssetRecord{}, column) {
			t.Fatalf("prompt asset table missing column %s", column)
		}
	}
	if !migrator.HasColumn(&model.PromptUsageFeedbackRecord{}, "artifact_refs_json") {
		t.Fatalf("prompt usage feedback table missing artifact_refs_json column")
	}
}

func TestPromptAssetModelsAreRegisteredForGormGen(t *testing.T) {
	want := map[string]bool{
		"*model.PromptAssetRecord":          false,
		"*model.PromptAssetVersionRecord":   false,
		"*model.PromptAssetSourceRefRecord": false,
		"*model.PromptUsageFeedbackRecord":  false,
	}
	for _, m := range model.AllModels() {
		want[modelName(m)] = true
	}
	for name, found := range want {
		if !found {
			t.Fatalf("model.AllModels missing %s", name)
		}
	}
}

func modelName(m any) string {
	switch m.(type) {
	case *model.PromptAssetRecord:
		return "*model.PromptAssetRecord"
	case *model.PromptAssetVersionRecord:
		return "*model.PromptAssetVersionRecord"
	case *model.PromptAssetSourceRefRecord:
		return "*model.PromptAssetSourceRefRecord"
	case *model.PromptUsageFeedbackRecord:
		return "*model.PromptUsageFeedbackRecord"
	default:
		return ""
	}
}
