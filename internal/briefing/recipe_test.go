package briefing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBriefingRecipeLifecycle(t *testing.T) {
	root := t.TempDir()
	recipe, err := InitRecipe(root, InitRecipeRequest{})
	if err != nil {
		t.Fatalf("init recipe: %v", err)
	}
	if recipe.SchemaVersion != RecipeSchemaVersion || recipe.Topic == "" || recipe.Limit == 0 || len(recipe.Sources) == 0 {
		t.Fatalf("recipe defaults = %#v", recipe)
	}
	if recipe.Weights.Relevance <= 0 || recipe.Weights.Novelty <= 0 || recipe.Output.Format == "" {
		t.Fatalf("recipe missing weights/output = %#v", recipe)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "briefing", "recipe.json")); err != nil {
		t.Fatalf("recipe asset missing: %v", err)
	}
	updated, err := SetRecipe(root, RecipePatch{Topic: "AI tooling", Limit: 7, AddSource: "fake:ai"})
	if err != nil {
		t.Fatalf("set recipe: %v", err)
	}
	if updated.Topic != "AI tooling" || updated.Limit != 7 || updated.Sources[len(updated.Sources)-1].ID != "fake:ai" {
		t.Fatalf("updated recipe = %#v", updated)
	}
	loaded, err := LoadRecipe(root)
	if err != nil {
		t.Fatalf("load recipe: %v", err)
	}
	if loaded.Topic != updated.Topic || loaded.Limit != updated.Limit {
		t.Fatalf("loaded = %#v want %#v", loaded, updated)
	}
}

func TestBriefingRecipeValidation(t *testing.T) {
	root := t.TempDir()
	if _, err := SetRecipe(root, RecipePatch{Limit: -1}); err == nil {
		t.Fatalf("negative limit accepted")
	}
	if _, err := InitRecipe(root, InitRecipeRequest{Limit: 101}); err == nil {
		t.Fatalf("oversized limit accepted")
	}
}
