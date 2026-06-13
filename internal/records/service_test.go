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
