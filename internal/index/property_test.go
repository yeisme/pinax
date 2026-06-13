package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestPropertyExtractorAndSchemaInfer(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Tags: []string{"pinax", "db"}, Status: "active", Kind: "reference", UpdatedAt: "2026-06-08T00:00:00Z", Body: "due:: 2026-06-09\npriority:: 2\npublished:: true\nrelated:: [[Beta]]\n"},
		{ID: "note_beta", Title: "Beta", Path: "notes/beta.md", Tags: []string{"db"}, Status: "done", Kind: "project", Body: "priority:: high\n"},
	}
	rows := ExtractPropertyRows(notes)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	alpha := rows[0].Values
	for _, key := range []string{"title", "path", "status", "kind", "tags", "due", "priority", "published", "related"} {
		if _, ok := alpha[key]; !ok {
			t.Fatalf("alpha missing property %q: %#v", key, alpha)
		}
	}
	if alpha["due"].Type != domain.PropertyTypeDate || alpha["priority"].Type != domain.PropertyTypeNumber || alpha["published"].Type != domain.PropertyTypeBoolean || alpha["related"].Type != domain.PropertyTypeLink {
		t.Fatalf("inline property types = %#v", alpha)
	}

	defs := InferPropertyDefinitions(rows)
	byName := map[string]domain.PropertyDefinition{}
	for _, def := range defs {
		byName[def.Name] = def
	}
	if byName["priority"].Type != domain.PropertyTypeMixed || byName["status"].Type != domain.PropertyTypeSelect || byName["tags"].Type != domain.PropertyTypeList {
		t.Fatalf("definitions = %#v", byName)
	}
}
