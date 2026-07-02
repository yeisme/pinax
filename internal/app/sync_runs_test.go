package app

import (
	"context"
	"path/filepath"
	"testing"
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
