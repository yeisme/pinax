package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

type legacyNoteRecord struct {
	Path   string `gorm:"primaryKey"`
	NoteID string `gorm:"index"`
	Title  string
}

func (legacyNoteRecord) TableName() string { return "note_records" }

type legacyTagRecord struct {
	ID       uint `gorm:"primaryKey"`
	NotePath string
	Tag      string
}

func (legacyTagRecord) TableName() string { return "tag_records" }

type legacyPropertyValueRecord struct {
	ID       uint `gorm:"primaryKey"`
	NotePath string
	Name     string
	Value    string
}

func (legacyPropertyValueRecord) TableName() string { return "property_value_records" }

func TestSchemaV2BackfillsObjectIdentityFromV1Projection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&NoteRecord{}, &TagRecord{}, &PropertyValueRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyNoteRecord{}, &legacyTagRecord{}, &legacyPropertyValueRecord{}); err != nil {
		t.Fatal(err)
	}
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	if err := db.Create(&legacyNoteRecord{Path: "notes/a.md", NoteID: objectID, Title: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyTagRecord{NotePath: "notes/a.md", Tag: "alpha"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyPropertyValueRecord{NotePath: "notes/a.md", Name: "priority", Value: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	var note NoteRecord
	if err := db.First(&note, "path = ?", "notes/a.md").Error; err != nil {
		t.Fatal(err)
	}
	var tag TagRecord
	if err := db.First(&tag).Error; err != nil {
		t.Fatal(err)
	}
	var property PropertyValueRecord
	if err := db.First(&property).Error; err != nil {
		t.Fatal(err)
	}
	if note.ObjectID != objectID || tag.ObjectID != objectID || property.ObjectID != objectID {
		t.Fatalf("backfill note=%#v tag=%#v property=%#v", note, tag, property)
	}
}

func TestObjectIdentityConsistencyMarksIndexStale(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	notes := []domain.Note{{ID: objectID, Title: "A", Path: "notes/a.md", Tags: []string{"alpha"}}}
	if _, err := Rebuild(root, notes); err != nil {
		t.Fatal(err)
	}
	db, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&TagRecord{}).Where("note_path = ?", "notes/a.md").Update("object_id", "wrong").Error; err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(root, notes)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "stale" || !containsString(status.Evidence, "identity_consistency=failed") {
		t.Fatalf("status = %#v", status)
	}
}
