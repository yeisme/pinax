package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	syncplan "github.com/yeisme/pinax/internal/sync"
)

func TestSyncLogsListAndTailIncludeLimitFacts(t *testing.T) {
	root := t.TempDir()
	svc := NewService()

	list, err := svc.SyncLogsList(context.Background(), SyncLogsRequest{VaultPath: root, Limit: 7})
	if err != nil {
		t.Fatalf("SyncLogsList: %v", err)
	}
	if got := list.Facts["limit"]; got != "7" {
		t.Fatalf("list limit fact = %q, want 7; facts=%#v", got, list.Facts)
	}

	writeActivityFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"type":"sync.run","status":"success","ts":"2026-06-27T10:00:00Z","facts":{"run_id":"sync_1","direction":"push","backend_kind":"server"}}`+"\n")
	tail, err := svc.SyncLogsTail(context.Background(), SyncLogsRequest{VaultPath: root, Limit: 3})
	if err != nil {
		t.Fatalf("SyncLogsTail: %v", err)
	}
	if got := tail.Facts["limit"]; got != "3" {
		t.Fatalf("tail limit fact = %q, want 3; facts=%#v", got, tail.Facts)
	}
	if got := tail.Facts["run_id"]; got != "sync_1" {
		t.Fatalf("tail run_id fact = %q, want sync_1; facts=%#v", got, tail.Facts)
	}
}

func TestSyncRunTimelineIncludesSanitizedFileOperations(t *testing.T) {
	root := t.TempDir()
	receipt := SyncRunReceipt{
		RunID:       "sync_files_1",
		Command:     "sync.push",
		Direction:   "push",
		Status:      "success",
		BackendKind: "embedded",
		Counts:      map[string]int{"conflicts": 0},
		Operations: []syncplan.Operation{{
			Kind:   "upload_blob",
			Path:   "notes/research/alpha.md",
			Status: "planned",
		}},
	}
	if err := appendSyncRunEvent(root, receipt); err != nil {
		t.Fatalf("appendSyncRunEvent: %v", err)
	}

	events, err := tailSyncEvents(root, 10)
	if err != nil {
		t.Fatalf("tailSyncEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v, want file event plus run event", events)
	}
	fileEvent := events[0]
	for key, want := range map[string]any{
		"type":      "sync.file",
		"run_id":    "sync_files_1",
		"direction": "push",
		"kind":      "upload_blob",
		"path":      "notes/research/alpha.md",
		"status":    "success",
	} {
		if got := fileEvent[key]; got != want {
			t.Fatalf("file event %s = %#v, want %#v; event=%#v", key, got, want, fileEvent)
		}
	}
	if fileEvent["seq"] != 1 || events[1]["seq"] != 2 {
		t.Fatalf("event sequence is not stable: %#v", events)
	}
}

func TestSyncLogsFollowEmitsInitialAndAppendedEvents(t *testing.T) {
	root := t.TempDir()
	if err := appendEvent(root, "sync.run", "success", map[string]string{"run_id": "sync_1", "direction": "push"}); err != nil {
		t.Fatalf("append initial event: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan map[string]any, 4)
	done := make(chan error, 1)
	go func() {
		done <- NewService().SyncLogsFollow(ctx, SyncLogsRequest{VaultPath: root, Limit: 10, PollInterval: 5 * time.Millisecond}, func(event map[string]any) error {
			events <- event
			return nil
		})
	}()

	first := <-events
	if first["type"] != "sync.run" || first["seq"] != 1 {
		t.Fatalf("first follow event = %#v", first)
	}
	if err := appendEvent(root, "sync.file", "success", map[string]string{"run_id": "sync_1", "direction": "push", "kind": "upload_blob", "path": "notes/live.md"}); err != nil {
		t.Fatalf("append file event: %v", err)
	}
	select {
	case second := <-events:
		if second["type"] != "sync.file" || second["seq"] != 2 || second["path"] != "notes/live.md" {
			t.Fatalf("second follow event = %#v", second)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for appended sync event")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SyncLogsFollow: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SyncLogsFollow did not stop after cancellation")
	}
}
