package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func jobEvent(eventType, status, ts, changeCode string) syncJobStatusEvent {
	return syncJobStatusEvent{Type: eventType, Status: status, TS: ts, ChangeCode: changeCode}
}

// TestReplaySyncJobStatusPhasesAndIdempotency 覆盖 accepted/progress/terminal
// 三相、崩溃前缀流、重复终态以最后为准，以及纯函数幂等性。
func TestReplaySyncJobStatusPhasesAndIdempotency(t *testing.T) {
	t.Parallel()
	fullStream := []syncJobStatusEvent{
		jobEvent("sync.run", "accepted", "2026-09-07T10:00:00Z", ""),
		jobEvent("sync.file", "success", "2026-09-07T10:00:01Z", "A"),
		jobEvent("sync.file", "success", "2026-09-07T10:00:02Z", "M"),
		jobEvent("sync.file", "success", "2026-09-07T10:00:03Z", "D"),
	}
	terminalStream := append(append([]syncJobStatusEvent{}, fullStream...),
		jobEvent("sync.run", "success", "2026-09-07T10:00:04Z", ""))

	cases := []struct {
		name    string
		events  []syncJobStatusEvent
		phase   string
		status  string
		compl   int
		total   int
		changes map[string]int
	}{
		{name: "empty", events: nil, phase: "", compl: 0, total: 0},
		{
			name:   "accepted: run-level event before any item",
			events: []syncJobStatusEvent{jobEvent("sync.run", "accepted", "2026-09-07T10:00:00Z", "")},
			phase:  SyncJobPhaseAccepted, status: "", compl: 0, total: 0,
		},
		{
			name:    "progress: items without terminal (crash prefix)",
			events:  fullStream[1:],
			phase:   SyncJobPhaseProgress,
			status:  "",
			compl:   3,
			total:   0,
			changes: map[string]int{"A": 1, "M": 1, "D": 1},
		},
		{
			name:    "terminal: items plus terminal event",
			events:  terminalStream,
			phase:   SyncJobPhaseTerminal,
			status:  "success",
			compl:   3,
			total:   3,
			changes: map[string]int{"A": 1, "M": 1, "D": 1},
		},
		{
			name: "terminal duplicate: last terminal wins",
			events: append(append([]syncJobStatusEvent{}, terminalStream...),
				jobEvent("sync.run", "failed", "2026-09-07T10:00:05Z", "")),
			phase: SyncJobPhaseTerminal, status: "failed", compl: 3, total: 3,
			changes: map[string]int{"A": 1, "M": 1, "D": 1},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			first := ReplaySyncJobStatus("run_1", tc.events)
			if first.Phase != tc.phase {
				t.Fatalf("phase = %q, want %q; status=%#v", first.Phase, tc.phase, first)
			}
			if first.Status != tc.status {
				t.Fatalf("status = %q, want %q", first.Status, tc.status)
			}
			if first.Completed != tc.compl || first.Total != tc.total {
				t.Fatalf("completed/total = %d/%d, want %d/%d", first.Completed, first.Total, tc.compl, tc.total)
			}
			if tc.changes != nil && !reflect.DeepEqual(first.ChangeCounts, tc.changes) {
				t.Fatalf("change counts = %#v, want %#v", first.ChangeCounts, tc.changes)
			}
			if tc.events != nil && first.SchemaVersion != syncJobStatusSchemaVersion {
				t.Fatalf("schema version = %q", first.SchemaVersion)
			}
			// 幂等：同一事件流重放结论一致。
			second := ReplaySyncJobStatus("run_1", tc.events)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("replay is not idempotent:\nfirst=%#v\nsecond=%#v", first, second)
			}
		})
	}
}

// TestSyncLogsStatusReplaysFromEventJSONL 用真实写入端（appendSyncRunEvent）
// 生成事件流，验证服务投影从 event JSONL 重放出与 receipt 一致的结论，
// 且崩溃前缀流（无终态事件）重放为 progress。
func TestSyncLogsStatusReplaysFromEventJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()

	receipt := SyncRunReceipt{
		RunID:       "sync_status_1",
		Command:     "sync.push",
		Direction:   "push",
		Status:      "success",
		BackendKind: "embedded",
		Operations: []syncplan.Operation{
			{Kind: "upload_blob", Path: "notes/a.md", Status: "planned"},
			{Kind: "upload_blob", Path: "notes/b.md", Status: "planned"},
			{Kind: "delete_local", Path: "notes/old.md", Status: "planned"},
		},
	}
	if err := appendSyncRunEvent(root, receipt); err != nil {
		t.Fatalf("appendSyncRunEvent: %v", err)
	}

	projection, err := svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_status_1"})
	if err != nil {
		t.Fatalf("SyncLogsStatus: %v", err)
	}
	for key, want := range map[string]string{
		"run_id":     "sync_status_1",
		"phase":      SyncJobPhaseTerminal,
		"status":     "success",
		"completed":  "3",
		"total":      "3",
		"replayable": "true",
	} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("fact %s = %q, want %q; facts=%#v", key, got, want, projection.Facts)
		}
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("projection.Data type = %T, want map[string]any", projection.Data)
	}
	status, ok := data["status"].(SyncJobStatus)
	if !ok {
		t.Fatalf("data.status type = %T, want SyncJobStatus", data["status"])
	}
	if !reflect.DeepEqual(status.ChangeCounts, map[string]int{"A": 2, "D": 1}) {
		t.Fatalf("change counts = %#v, want A:2 D:1", status.ChangeCounts)
	}

	// 幂等：再次投影结论一致。
	again, err := svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_status_1"})
	if err != nil {
		t.Fatalf("SyncLogsStatus(second): %v", err)
	}
	againData, ok := again.Data.(map[string]any)
	if !ok {
		t.Fatalf("again.Data type = %T, want map[string]any", again.Data)
	}
	if !reflect.DeepEqual(againData["status"], status) {
		t.Fatalf("status projection is not idempotent: %#v vs %#v", againData["status"], status)
	}

	// 崩溃前缀：另一个 run 只有逐项事件、没有终态事件 → progress。
	partial := SyncRunReceipt{RunID: "sync_crash_1", Command: "sync.push", Direction: "push", Status: "success", BackendKind: "embedded",
		Operations: []syncplan.Operation{{Kind: "upload_blob", Path: "notes/x.md", Status: "planned"}}}
	// 手写事件流前缀：只保留逐项事件（复用 appendEvent 事实格式）。
	if err := appendEvent(root, "sync.file", "success", map[string]string{
		"run_id": partial.RunID, "command": partial.Command, "direction": partial.Direction,
		"backend_kind": partial.BackendKind, "kind": "upload_blob", "operation_status": "planned",
		"change_code": "A", "change_state": "added",
	}); err != nil {
		t.Fatalf("appendEvent(prefix): %v", err)
	}
	partialProjection, err := svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: partial.RunID})
	if err != nil {
		t.Fatalf("SyncLogsStatus(partial): %v", err)
	}
	if got := partialProjection.Facts["phase"]; got != SyncJobPhaseProgress {
		t.Fatalf("partial run phase = %q, want %q; facts=%#v", got, SyncJobPhaseProgress, partialProjection.Facts)
	}
	if got := partialProjection.Facts["completed"]; got != "1" {
		t.Fatalf("partial run completed = %q, want 1", got)
	}

	// 无事件的 run → 可解释 not-found 错误。
	if _, err := svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_missing_1"}); err == nil {
		t.Fatal("SyncLogsStatus(missing) must fail")
	} else {
		var commandErr *domain.CommandError
		if !errors.As(err, &commandErr) {
			t.Fatalf("missing run error = %T, want *domain.CommandError", err)
		}
		if commandErr.Code != "sync_run_not_found" {
			t.Fatalf("missing run error code = %q, want sync_run_not_found", commandErr.Code)
		}
	}
}
