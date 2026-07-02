package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMonitorRecordsSearchStepsAndActivitySource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\nAlpha body secret-token raw prompt system prompt\n")

	if _, err := svc.SearchProjection(ctx, SearchRequest{VaultPath: root, Query: "Alpha secret-token", Engine: "native", Limit: 5}); err != nil {
		t.Fatalf("search projection: %v", err)
	}

	list, err := svc.MonitorList(ctx, MonitorRequest{VaultPath: root, Command: "note.search", Limit: 10})
	if err != nil {
		t.Fatalf("monitor list: %v", err)
	}
	runs := list.Data.(map[string]any)["runs"].([]MonitorRun)
	if len(runs) != 1 {
		t.Fatalf("runs = %#v", runs)
	}
	run := runs[0]
	if run.Command != "note.search" || run.Status != "success" || len(run.Steps) < 4 {
		t.Fatalf("run = %#v", run)
	}
	if run.Metrics.HeapAllocBytes == 0 {
		t.Fatalf("metrics missing process data: %#v", run.Metrics)
	}
	encoded, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	for _, forbidden := range []string{"Alpha secret-token", "raw prompt", "system prompt"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("monitor run leaked forbidden payload %q: %s", forbidden, encoded)
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(run.Evidence[0]))); err != nil {
		t.Fatalf("monitor run evidence missing: %v", err)
	}

	activity, err := svc.ActivityList(ctx, ActivityRequest{VaultPath: root, Source: "monitor_runs", Query: "note.search", Limit: 10})
	if err != nil {
		t.Fatalf("activity monitor source: %v", err)
	}
	entries := activity.Data.(map[string]any)["entries"].([]ActivityEntry)
	if len(entries) != 1 || entries[0].Source != "monitor_runs" || entries[0].RunID != run.RunID {
		t.Fatalf("activity entries = %#v", entries)
	}
}

func TestMonitorShowSummaryAndFailedRun(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: "not valid sql", LazyIndex: true}); err == nil {
		t.Fatalf("expected query parse error")
	}

	list, err := svc.MonitorList(ctx, MonitorRequest{VaultPath: root, Status: "failed", Limit: 10})
	if err != nil {
		t.Fatalf("monitor list failed: %v", err)
	}
	runs := list.Data.(map[string]any)["runs"].([]MonitorRun)
	if len(runs) != 1 || runs[0].Command != "query.run" || runs[0].ErrorCode == "" {
		t.Fatalf("failed runs = %#v", runs)
	}
	show, err := svc.MonitorShow(ctx, MonitorRequest{VaultPath: root, RunID: runs[0].RunID})
	if err != nil || show.Facts["run_id"] != runs[0].RunID {
		t.Fatalf("monitor show projection=%#v err=%v", show, err)
	}
	summary, err := svc.MonitorSummary(ctx, MonitorRequest{VaultPath: root, Limit: 10})
	if err != nil || summary.Facts["runs"] != "1" {
		t.Fatalf("monitor summary projection=%#v err=%v", summary, err)
	}
}

func TestMonitorProjectionIncludesFilterFacts(t *testing.T) {
	result := monitorQueryResult{
		Runs:    []MonitorRun{{RunID: "run_1", Command: "note.search", Status: "success"}},
		Filters: map[string]string{"command": "note.search", "limit": "5", "status": "success", "query": "alpha"},
	}

	projection := monitorProjection("monitor.runs", "Monitor runs listed.", result)

	for key, want := range map[string]string{
		"filter.command": "note.search",
		"filter.limit":   "5",
		"filter.status":  "success",
		"filter.query":   "alpha",
	} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("fact %s = %q, want %q; facts=%#v", key, got, want, projection.Facts)
		}
	}
}
