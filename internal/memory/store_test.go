package memory

import (
	"context"
	"testing"
)

func TestMemoryStoreMigratesAndPersistsRecords(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	record, err := store.Capture(context.Background(), CaptureRequest{Type: TypeFact, Subject: "pinax", Predicate: "release_workflow", Object: "tag push triggers GitHub Actions", SourceURI: "docs/operations/release-packaging.md"})
	if err != nil {
		t.Fatalf("capture memory: %v", err)
	}
	if record.ID == "" || record.Status != StatusConfirmed {
		t.Fatalf("record = %#v", record)
	}
	records, err := store.List(context.Background(), ListFilter{Entity: "pinax"})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("records = %#v", records)
	}
}

func TestMemoryRecallUsesFTSAndFiltersStatus(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	ctx := context.Background()
	for _, status := range []string{StatusDraft, StatusConfirmed, StatusSuperseded, StatusExpired, StatusRejected} {
		if _, err := store.Capture(ctx, CaptureRequest{Type: TypeFact, Subject: "pinax", Predicate: "release_workflow", Object: status + " release memory", Status: status}); err != nil {
			t.Fatalf("capture %s: %v", status, err)
		}
	}
	hits, err := store.Recall(ctx, RecallFilter{Query: "release memory", Entity: "pinax"})
	if err != nil {
		t.Fatalf("recall memory: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %#v", hits)
	}
	if hits[0].Record.Status != StatusConfirmed || hits[0].Record.Object != "confirmed release memory" {
		t.Fatalf("hit = %#v", hits[0])
	}
	if hits[0].RecallReason == "" {
		t.Fatalf("missing recall reason: %#v", hits[0])
	}
}

func TestMemoryBuildRecordRejectsInvalidType(t *testing.T) {
	_, err := BuildRecord(CaptureRequest{Type: "unknown", Subject: "pinax", Object: "bad"})
	if err == nil {
		t.Fatalf("expected invalid memory type")
	}
	cmdErr, ok := err.(interface{ Error() string })
	if !ok || cmdErr.Error() == "" {
		t.Fatalf("invalid error = %#v", err)
	}
}
