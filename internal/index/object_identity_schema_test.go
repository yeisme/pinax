package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestSchemaV2UsesObjectIDForNotesAndRelationshipProjections(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	note := domain.Note{ID: objectID, Title: "Alpha", Path: "notes/alpha.md", Tags: []string{"project/alpha"}, Body: "priority:: 2\n- [ ] Task\n[[Alpha]]"}
	if _, err := Rebuild(root, []domain.Note{note}); err != nil {
		t.Fatal(err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	for model, columns := range map[any][]string{
		&NoteRecord{}:          {"object_id", "path"},
		&NoteTextRecord{}:      {"object_id"},
		&TagRecord{}:           {"object_id"},
		&LinkRecord{}:          {"source_object_id", "target_object_id"},
		&SearchTokenRecord{}:   {"object_id"},
		&AttachmentRecord{}:    {"object_id"},
		&PropertyValueRecord{}: {"object_id"},
		&TaskRecord{}:          {"object_id"},
	} {
		for _, column := range columns {
			if !db.Migrator().HasColumn(model, column) {
				t.Fatalf("%T missing %s", model, column)
			}
		}
	}
	var noteRecord NoteRecord
	if err := db.First(&noteRecord, "object_id = ?", objectID).Error; err != nil {
		t.Fatal(err)
	}
	if noteRecord.ObjectID != objectID || noteRecord.Path != note.Path || noteRecord.NoteID != objectID {
		t.Fatalf("note record = %#v", noteRecord)
	}
	var tag TagRecord
	if err := db.First(&tag).Error; err != nil {
		t.Fatal(err)
	}
	if tag.ObjectID != objectID || tag.NotePath != note.Path {
		t.Fatalf("tag = %#v", tag)
	}
	var property PropertyValueRecord
	if err := db.First(&property).Error; err != nil {
		t.Fatal(err)
	}
	if property.ObjectID != objectID || property.NotePath != note.Path {
		t.Fatalf("property = %#v", property)
	}
}

func TestSchemaV2RejectsTwoActivePathsForOneObject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	_, err := Rebuild(root, []domain.Note{
		{ID: objectID, Title: "Alpha", Path: "notes/alpha.md"},
		{ID: objectID, Title: "Beta", Path: "notes/beta.md"},
	})
	if err == nil {
		t.Fatal("duplicate object id rebuild succeeded")
	}
}
