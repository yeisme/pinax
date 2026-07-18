package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActivityListNormalizesSourcesAndFilters(t *testing.T) {
	root := t.TempDir()
	writeActivityFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"schema_version":"pinax.event.v1","type":"note.new","status":"success","ts":"2026-06-27T10:00:00Z","facts":{"path":"notes/alpha.md","token":"secret-token"}}`+"\n")
	writeActivityFixture(t, filepath.Join(root, ".pinax", "sync-daemon", "events.jsonl"), `{"schema_version":"pinax.sync_daemon.event.v1","type":"sync_completed","status":"success","target":"cloud","sync_run_id":"sync_1","created_at":"2026-06-27T10:05:00Z"}`+"\n")
	writeActivityFixture(t, filepath.Join(root, ".pinax", "events", "api-audit.jsonl"), `{"ts":"2026-06-27T10:02:00Z","token_id":"tok_123","method":"GET","path":"/v1/notes/alpha","scope":"read","group":"notes","status":200}`+"\n")
	writeActivityFixture(t, filepath.Join(root, ".pinax", "records", "events.jsonl"), `{"schema_version":"pinax.record_event.v1","event_id":"record_evt_1","seq":1,"kind":"note.created","note_id":"note_alpha","path":"notes/alpha.md","title":"Alpha","created_at":"2026-06-27T10:03:00Z"}`+"\n")
	writeActivityFixture(t, filepath.Join(root, ".pinax", "sync-runs", "2026", "06", "sync_1.json"), `{"schema_version":"pinax.sync_run.v1","run_id":"sync_1","command":"sync.push","target":"cloud","direction":"push","status":"success","remote_write":true,"backend_kind":"server","timings_ms":{"total":12},"created_at":"2026-06-27T10:04:00Z"}`+"\n")

	projection, err := NewService().ActivityList(context.Background(), ActivityRequest{VaultPath: root, Source: "all", Query: "alpha", Limit: 10})
	if err != nil {
		t.Fatalf("ActivityList: %v", err)
	}
	if projection.Command != "activity.list" || projection.Status != "success" || projection.Facts["entries"] != "3" {
		t.Fatalf("projection facts = %#v status=%s", projection.Facts, projection.Status)
	}
	data := projection.Data.(map[string]any)
	entries := data["entries"].([]ActivityEntry)
	if len(entries) != 3 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Source != "record_ledger" || entries[0].ObjectRef != "note_alpha" {
		t.Fatalf("newest filtered entry = %#v", entries[0])
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	if strings.Contains(string(encoded), "secret-token") || strings.Contains(string(encoded), "token") {
		t.Fatalf("activity entries leaked token fact: %s", encoded)
	}
}

func TestActivityPartialWarningsAndShow(t *testing.T) {
	root := t.TempDir()
	writeActivityFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"schema_version":"pinax.event.v1","type":"index.refresh","status":"success","ts":"2026-06-27T10:00:00Z","facts":{"path":"notes/index.md"}}`+"\nnot-json\n")

	svc := NewService()
	list, err := svc.ActivityList(context.Background(), ActivityRequest{VaultPath: root, Limit: 10})
	if err != nil {
		t.Fatalf("ActivityList: %v", err)
	}
	if list.Status != "partial" || list.Facts["warnings"] != "1" || list.Facts["entries"] != "1" {
		t.Fatalf("partial facts = %#v status=%s", list.Facts, list.Status)
	}
	entry := list.Data.(map[string]any)["entries"].([]ActivityEntry)[0]
	show, err := svc.ActivityShow(context.Background(), ActivityRequest{VaultPath: root, EventID: entry.EventID})
	if err != nil || show.Command != "activity.show" || show.Facts["event_id"] != entry.EventID {
		t.Fatalf("show projection=%#v err=%v", show, err)
	}
	missing, err := svc.ActivityShow(context.Background(), ActivityRequest{VaultPath: root, EventID: "missing"})
	if err == nil || missing.Error == nil || missing.Error.Code != "activity_event_not_found" {
		t.Fatalf("missing projection=%#v err=%v", missing, err)
	}
}

func TestActivityProjectionIncludesFilterFacts(t *testing.T) {
	result := activityQueryResult{
		Entries: []ActivityEntry{{EventID: "evt_1", Source: "vault_events", Kind: "note.created", Status: "success"}},
		Filters: map[string]string{"source": "vault_events", "limit": "3", "status": "success", "query": "alpha"},
	}

	projection := activityProjection("activity.list", "Activity entries listed.", result)

	for key, want := range map[string]string{
		"filter.source": "vault_events",
		"filter.limit":  "3",
		"filter.status": "success",
		"filter.query":  "alpha",
	} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("fact %s = %q, want %q; facts=%#v", key, got, want, projection.Facts)
		}
	}
}

func writeActivityFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
