package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestIndexRebuildPropertyProjectionAndStatus(t *testing.T) {
	root := t.TempDir()
	notes := []domain.Note{{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Tags: []string{"pinax"}, Status: "active", Body: "priority:: 2\n"}}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defs := []PropertyDefinitionRecord{}
	if err := db.Find(&defs).Error; err != nil {
		t.Fatalf("find definitions: %v", err)
	}
	values := []PropertyValueRecord{}
	if err := db.Find(&values).Error; err != nil {
		t.Fatalf("find values: %v", err)
	}
	if len(defs) == 0 || len(values) == 0 {
		t.Fatalf("missing property projection defs=%#v values=%#v", defs, values)
	}
	status, err := Inspect(root, notes)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if status.Status != "fresh" {
		t.Fatalf("status = %#v", status)
	}
	if err := upsertMeta(db, "property_schema_version", "old", "now"); err != nil {
		t.Fatalf("mutate property schema version: %v", err)
	}
	stale, err := Inspect(root, notes)
	if err != nil {
		t.Fatalf("inspect stale: %v", err)
	}
	if stale.Status != "stale" || stale.SchemaVersion == "" {
		t.Fatalf("stale status = %#v", stale)
	}
}
