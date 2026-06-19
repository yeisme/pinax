package briefingops

import (
	"fmt"
	"path/filepath"

	"github.com/yeisme/pinax/internal/briefing"
	"github.com/yeisme/pinax/internal/domain"
)

func RecipeProjection(command, summary, root string, recipe briefing.Recipe) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["topic"] = recipe.Topic
	projection.Facts["limit"] = fmt.Sprint(recipe.Limit)
	projection.Facts["sources"] = fmt.Sprint(len(recipe.Sources))
	projection.Facts["output_format"] = recipe.Output.Format
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(root, ".pinax", "briefing", "recipe.json"))}
	projection.Data = map[string]any{"recipe": recipe}
	return projection
}
