package briefing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RecipeSchemaVersion = "pinax.briefing.recipe.v1"

type Recipe struct {
	SchemaVersion string         `json:"schema_version"`
	Topic         string         `json:"topic"`
	Sources       []RecipeSource `json:"sources"`
	Weights       RecipeWeights  `json:"weights"`
	Output        RecipeOutput   `json:"output"`
	Limit         int            `json:"limit"`
	UpdatedAt     string         `json:"updated_at"`
}

type RecipeSource struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Capability string `json:"capability,omitempty"`
}

type RecipeWeights struct {
	Relevance float64 `json:"relevance"`
	Novelty   float64 `json:"novelty"`
	Trust     float64 `json:"trust"`
}

type RecipeOutput struct {
	Format string   `json:"format"`
	Tags   []string `json:"tags"`
}

type InitRecipeRequest struct {
	Topic string
	Limit int
}

type RecipePatch struct {
	Topic     string
	Limit     int
	AddSource string
}

func InitRecipe(root string, req InitRecipeRequest) (Recipe, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Recipe{}, err
	}
	recipe := DefaultRecipe()
	if strings.TrimSpace(req.Topic) != "" {
		recipe.Topic = strings.TrimSpace(req.Topic)
	}
	if req.Limit != 0 {
		recipe.Limit = req.Limit
	}
	if err := ValidateRecipe(recipe); err != nil {
		return Recipe{}, err
	}
	recipe.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return recipe, SaveRecipe(root, recipe)
}

func LoadRecipe(root string) (Recipe, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Recipe{}, err
	}
	b, err := os.ReadFile(recipePath(root))
	if err != nil {
		return Recipe{}, err
	}
	var recipe Recipe
	if err := json.Unmarshal(b, &recipe); err != nil {
		return Recipe{}, err
	}
	return recipe, ValidateRecipe(recipe)
}

func SetRecipe(root string, patch RecipePatch) (Recipe, error) {
	recipe, err := LoadRecipe(root)
	if err != nil {
		if os.IsNotExist(err) {
			recipe = DefaultRecipe()
		} else {
			return Recipe{}, err
		}
	}
	if strings.TrimSpace(patch.Topic) != "" {
		recipe.Topic = strings.TrimSpace(patch.Topic)
	}
	if patch.Limit != 0 {
		recipe.Limit = patch.Limit
	}
	if strings.TrimSpace(patch.AddSource) != "" {
		recipe.Sources = append(recipe.Sources, sourceFromID(strings.TrimSpace(patch.AddSource)))
	}
	if err := ValidateRecipe(recipe); err != nil {
		return Recipe{}, err
	}
	recipe.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return recipe, SaveRecipe(root, recipe)
}

func SaveRecipe(root string, recipe Recipe) error {
	if err := os.MkdirAll(filepath.Dir(recipePath(root)), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(recipe, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(recipePath(root), append(b, '\n'), 0o600)
}

func DefaultRecipe() Recipe {
	return Recipe{SchemaVersion: RecipeSchemaVersion, Topic: "AI research", Sources: []RecipeSource{{ID: "fake:default", Kind: "fake", Capability: "daily_hot_notes"}}, Weights: RecipeWeights{Relevance: 0.5, Novelty: 0.3, Trust: 0.2}, Output: RecipeOutput{Format: "briefing_candidate", Tags: []string{"briefing", "candidate"}}, Limit: 5}
}

func ValidateRecipe(recipe Recipe) error {
	if recipe.SchemaVersion != RecipeSchemaVersion {
		return fmt.Errorf("invalid recipe schema_version %q", recipe.SchemaVersion)
	}
	if strings.TrimSpace(recipe.Topic) == "" {
		return fmt.Errorf("recipe topic required")
	}
	if recipe.Limit <= 0 || recipe.Limit > 50 {
		return fmt.Errorf("recipe limit must be between 1 and 50")
	}
	if len(recipe.Sources) == 0 {
		return fmt.Errorf("recipe source required")
	}
	if recipe.Weights.Relevance <= 0 || recipe.Weights.Novelty <= 0 || recipe.Weights.Trust <= 0 {
		return fmt.Errorf("recipe weights must be positive")
	}
	if recipe.Output.Format == "" {
		return fmt.Errorf("recipe output format required")
	}
	return nil
}

func sourceFromID(id string) RecipeSource {
	kind, _, ok := strings.Cut(id, ":")
	if !ok || kind == "" {
		kind = "fake"
	}
	return RecipeSource{ID: id, Kind: kind, Capability: "daily_hot_notes"}
}

func recipePath(root string) string {
	return filepath.Join(root, ".pinax", "briefing", "recipe.json")
}
