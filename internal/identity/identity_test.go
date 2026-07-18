package identity

import (
	"errors"
	"testing"
)

func TestIdentityNewObjectIDReturnsCanonicalUUIDv7(t *testing.T) {
	id, err := NewObjectID()
	if err != nil {
		t.Fatalf("NewObjectID() error = %v", err)
	}
	if Classify(id.String()) != IDClassCanonical {
		t.Fatalf("Classify(%q) = %q, want %q", id, Classify(id.String()), IDClassCanonical)
	}
	parsed, err := ParseObjectID(id.String())
	if err != nil {
		t.Fatalf("ParseObjectID(%q) error = %v", id, err)
	}
	if parsed != id {
		t.Fatalf("ParseObjectID(%q) = %q", id, parsed)
	}
}

func TestIdentityClassifyRecognizesLegacyNoteIDs(t *testing.T) {
	for _, raw := range []string{"note_alpha", "note_0123456789ab", "note_existing-id"} {
		if got := Classify(raw); got != IDClassLegacy {
			t.Fatalf("Classify(%q) = %q, want %q", raw, got, IDClassLegacy)
		}
	}
	for _, raw := range []string{"", "notes/alpha.md", "not a note id", "550e8400-e29b-41d4-a716-446655440000"} {
		if got := Classify(raw); got != IDClassInvalid {
			t.Fatalf("Classify(%q) = %q, want %q", raw, got, IDClassInvalid)
		}
	}
}

func TestIdentityObjectKindsCoverDurableVaultObjects(t *testing.T) {
	for _, kind := range []ObjectKind{KindVault, KindNote, KindAsset, KindProject, KindSubproject, KindFolder, KindView, KindTemplate} {
		if !kind.Valid() {
			t.Fatalf("ObjectKind(%q).Valid() = false", kind)
		}
	}
	if ObjectKind("temporary").Valid() {
		t.Fatal("temporary object kind unexpectedly valid")
	}
}

func TestIdentityAllocatorReusesIDForSameIdempotencyKey(t *testing.T) {
	ids := []ObjectID{
		MustParseObjectID("018f22e2-7b6d-7a3a-8db8-1f7ddf0c0001"),
		MustParseObjectID("018f22e2-7b6d-7a3a-8db8-1f7ddf0c0002"),
	}
	calls := 0
	allocator := NewAllocatorWithGenerator(func() (ObjectID, error) {
		if calls >= len(ids) {
			return "", errors.New("generator exhausted")
		}
		id := ids[calls]
		calls++
		return id, nil
	})

	first, err := allocator.Allocate("note.create:/vault/notes/a.md")
	if err != nil {
		t.Fatalf("Allocate(first) error = %v", err)
	}
	second, err := allocator.Allocate("note.create:/vault/notes/a.md")
	if err != nil {
		t.Fatalf("Allocate(second) error = %v", err)
	}
	third, err := allocator.Allocate("note.create:/vault/notes/b.md")
	if err != nil {
		t.Fatalf("Allocate(third) error = %v", err)
	}
	if first != second {
		t.Fatalf("same idempotency key returned %q then %q", first, second)
	}
	if third == first {
		t.Fatalf("different idempotency keys returned the same id %q", third)
	}
	if calls != 2 {
		t.Fatalf("generator calls = %d, want 2", calls)
	}
}

func TestIdentityAllocatorRejectsEmptyIdempotencyKey(t *testing.T) {
	_, err := NewAllocator().Allocate(" ")
	if !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("Allocate(empty) error = %v, want %v", err, ErrIdempotencyKeyRequired)
	}
}
