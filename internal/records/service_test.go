package records

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestLedgerAppendIsIdempotentAndMaterializesRegistry(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	event := domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:note_a", NoteID: "note_a", Path: "notes/a.md", Title: "A", ContentRevision: domain.ContentRevision{Hash: "h1", Size: 12}}
	first, err := svc.AppendEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := svc.AppendEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("append duplicate: %v", err)
	}
	if first.Seq != 1 || second.Seq != first.Seq {
		t.Fatalf("idempotent seq mismatch: first=%#v second=%#v", first, second)
	}

	state, err := svc.Replay(context.Background())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	record := state.Records["note_a"]
	if record.NoteID != "note_a" || record.Path != "notes/a.md" || record.Lifecycle != domain.NoteLifecycleActive || record.LedgerSeq != 1 || state.Version.LastSeq != 1 {
		t.Fatalf("state = %#v", state)
	}
	if countJSONLLines(t, filepath.Join(root, ".pinax", "records", "events.jsonl")) != 1 {
		t.Fatalf("duplicate event was appended")
	}
}

func TestLedgerRejectsIllegalLifecycleTransition(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create", NoteID: "note_a", Path: "notes/a.md", Title: "A"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteDeleted, IdempotencyKey: "delete", NoteID: "note_a", Path: "notes/a.md"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteArchived, IdempotencyKey: "archive-after-delete", NoteID: "note_a", Path: "notes/a.md"})
	if err == nil || domain.ErrorCode(err) != "record_lifecycle_invalid" {
		t.Fatalf("illegal transition err = %v", err)
	}
}

func TestLedgerTrashedNoteMaterializesTombstone(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create", NoteID: "note_a", Path: "notes/a.md", Title: "A", ContentRevision: domain.ContentRevision{Hash: "h1", Size: 12}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteTrashed, IdempotencyKey: "trash", NoteID: "note_a", Path: "notes/a.md", TrashPath: ".pinax/trash/20260708/notes/a.md"}); err != nil {
		t.Fatalf("trash: %v", err)
	}
	state, err := svc.Replay(context.Background())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	record := state.Records["note_a"]
	tombstone := state.Tombstones["note_a"]
	if record.Lifecycle != domain.NoteLifecycleTrashed || tombstone.ObjectKind != "note" || tombstone.ObjectID != "note_a" || tombstone.OldPath != "notes/a.md" || tombstone.TrashPath == "" {
		t.Fatalf("trashed state record=%#v tombstone=%#v", record, tombstone)
	}
}

func TestLedgerMetadataUpdateMaterializesExistingRecord(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create", NoteID: "note_a", Path: "notes/a.md", Title: "A", ContentRevision: domain.ContentRevision{Hash: "h1", Size: 12}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteMetadataUpdated, IdempotencyKey: "tag:add:note_a:research", NoteID: "note_a", Path: "notes/a.md", Title: "A", ContentRevision: domain.ContentRevision{Hash: "h2", Size: 16}, Evidence: []string{"operation=tag.add", "tags=research"}})
	if err != nil {
		t.Fatalf("metadata update: %v", err)
	}
	if updated.Kind != domain.RecordEventNoteMetadataUpdated || updated.Seq != 2 {
		t.Fatalf("updated event = %#v", updated)
	}

	state, err := svc.Replay(context.Background())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	record := state.Records["note_a"]
	if record.RecordVersion != 2 || record.LedgerSeq != 2 || record.ContentRevision.Hash != "h2" || record.Lifecycle != domain.NoteLifecycleActive {
		t.Fatalf("metadata update was not materialized: %#v", record)
	}
}

func TestLedgerConcurrentAppendsUseSingleSequence(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			noteID := "note_" + string(rune('a'+i))
			if _, err := svc.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:" + noteID, NoteID: noteID, Path: "notes/" + noteID + ".md", Title: noteID}); err != nil {
				t.Errorf("append %s: %v", noteID, err)
			}
		}()
	}
	wg.Wait()
	state, err := svc.Replay(context.Background())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(state.Records) != 16 || state.Version.LastSeq != 16 {
		t.Fatalf("state = %#v", state)
	}
}

func TestLedgerReplayRejectsCorruptJSONL(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	path := filepath.Join(root, ".pinax", "records", "events.jsonl")
	if err := os.WriteFile(path, []byte("{bad json\n"), 0o644); err != nil {
		t.Fatalf("write corrupt jsonl: %v", err)
	}
	_, err := svc.Replay(context.Background())
	if err == nil || domain.ErrorCode(err) != "record_event_log_corrupt" {
		t.Fatalf("corrupt replay err = %v", err)
	}
}

func countJSONLLines(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	count := 0
	for _, b := range body {
		if b == '\n' {
			count++
		}
	}
	return count
}

func TestLedgerMaterializesObjectIdentityAcrossRename(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	_, err := service.AppendEvent(context.Background(), domain.RecordEvent{
		Kind:           domain.RecordEventNoteCreated,
		IdempotencyKey: "create:" + objectID,
		ObjectID:       objectID,
		ObjectKind:     "note",
		CurrentPath:    "notes/original.md",
		Title:          "Original",
		LegacyAliases:  []string{"note_legacy"},
		ContentRevision: domain.ContentRevision{
			Hash: "h1",
			Size: 12,
		},
	})
	if err != nil {
		t.Fatalf("create object event: %v", err)
	}
	_, err = service.AppendEvent(context.Background(), domain.RecordEvent{
		Kind:           domain.RecordEventNoteRenamed,
		IdempotencyKey: "rename:" + objectID,
		ObjectID:       objectID,
		ObjectKind:     "note",
		OldPath:        "notes/original.md",
		CurrentPath:    "notes/renamed.md",
		Title:          "Renamed",
		ContentRevision: domain.ContentRevision{
			Hash: "h2",
			Size: 14,
		},
	})
	if err != nil {
		t.Fatalf("rename object event: %v", err)
	}

	state, err := service.Replay(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	record, ok := state.Records[objectID]
	if !ok {
		t.Fatalf("record missing for object %q", objectID)
	}
	if record.ObjectID != objectID || record.NoteID != objectID || record.ObjectKind != "note" {
		t.Fatalf("identity fields = %#v", record)
	}
	if record.CurrentPath != "notes/renamed.md" || record.Path != record.CurrentPath {
		t.Fatalf("locator fields = %#v", record)
	}
	if len(record.LegacyAliases) != 1 || record.LegacyAliases[0] != "note_legacy" {
		t.Fatalf("legacy aliases = %#v", record.LegacyAliases)
	}
	if record.RecordVersion != 2 || record.ContentRevision.Hash != "h2" {
		t.Fatalf("record version/revision = %#v", record)
	}
}

func TestLedgerRejectsDuplicateObjectCreate(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0102"
	if _, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:first", ObjectID: objectID, ObjectKind: "note", CurrentPath: "notes/a.md"}); err != nil {
		t.Fatal(err)
	}
	_, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:second", ObjectID: objectID, ObjectKind: "note", CurrentPath: "notes/a-copy.md"})
	if err == nil || domain.ErrorCode(err) != "record_object_id_duplicate" {
		t.Fatalf("duplicate create error = %v", err)
	}
}

func TestLedgerRejectsPathCollisionBetweenObjects(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	if _, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:a", ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0103", ObjectKind: "note", CurrentPath: "notes/shared.md"}); err != nil {
		t.Fatal(err)
	}
	_, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "create:b", ObjectID: "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0104", ObjectKind: "note", CurrentPath: "notes/shared.md"})
	if err == nil || domain.ErrorCode(err) != "record_path_collision" {
		t.Fatalf("path collision error = %v", err)
	}
}

func TestLedgerReplayReadOnlyDoesNotInitializeOrRewriteState(t *testing.T) {
	root := t.TempDir()
	state, err := NewService(root).ReplayReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 0 || state.Version.LastSeq != 0 {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatalf("read-only replay initialized ledger: %v", err)
	}
}

func TestLedgerIdentityMigrationRekeysLegacyRecord(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	legacyID := "note_legacy"
	objectID := "018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101"
	if _, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteCreated, IdempotencyKey: "legacy:create", ObjectID: legacyID, ObjectKind: "note", CurrentPath: "notes/a.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(context.Background(), domain.RecordEvent{Kind: domain.RecordEventNoteIdentityMigrated, IdempotencyKey: "legacy:migrate", ObjectID: objectID, ObjectKind: "note", CurrentPath: "notes/a.md", LegacyAliases: []string{legacyID}}); err != nil {
		t.Fatal(err)
	}
	state, err := service.ReplayReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := state.Records[legacyID]; exists {
		t.Fatalf("legacy record still exists: %#v", state.Records)
	}
	record := state.Records[objectID]
	if record.ObjectID != objectID || record.CurrentPath != "notes/a.md" || len(record.LegacyAliases) != 1 || record.LegacyAliases[0] != legacyID {
		t.Fatalf("record = %#v", record)
	}
}
