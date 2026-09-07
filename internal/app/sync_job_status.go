package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yeisme/pinax/internal/domain"
)

// pinax.sync_job_status.v1：从 vault event JSONL 幂等重放出的 sync job status
// 投影（pinax-local-async-substrate-v1 §2.1）。phase 词汇对齐 `--events`
// NDJSON 事件模型的 accepted/progress/terminal。
const syncJobStatusSchemaVersion = "pinax.sync_job_status.v1"

const (
	SyncJobPhaseAccepted = "accepted"
	SyncJobPhaseProgress = "progress"
	SyncJobPhaseTerminal = "terminal"
)

// SyncJobStatus 是 sync run 的可重放状态汇总。它是 event JSONL 的纯函数：
// 同一事件流（含崩溃后不完整的前缀流）重放任意次得到同一结论。
type SyncJobStatus struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id"`
	Phase         string         `json:"phase"`
	Status        string         `json:"status,omitempty"`
	ErrorCode     string         `json:"error_code,omitempty"`
	Completed     int            `json:"completed"`
	Total         int            `json:"total,omitempty"`
	ChangeCounts  map[string]int `json:"change_counts,omitempty"`
	EventCount    int            `json:"event_count"`
	FirstEventAt  string         `json:"first_event_at,omitempty"`
	LastEventAt   string         `json:"last_event_at,omitempty"`
}

// syncJobStatusEvent 是 event JSONL 中属于一个 sync run 的最小事件视图。
type syncJobStatusEvent struct {
	Type       string
	Status     string
	TS         string
	ChangeCode string
	ErrorCode  string
}

// nonTerminalRunStatuses 是 run 级（sync.run）事件中的受理/进行标记：
// 它们只证明 job 已被接受并开始，不构成终态结论。
var nonTerminalRunStatuses = map[string]bool{
	"accepted": true,
	"running":  true,
}

// ReplaySyncJobStatus 按序重放属于同一 run 的事件并汇总状态：
//   - 只有受理标记、尚无逐项事件 → accepted；
//   - 已有逐项事件但无终态事件（崩溃/中断前缀流）→ progress；
//   - 出现终态 run 级事件 → terminal，status/error 取最后一个终态事件
//     （append-only 日志下重复终态以最后一条为准）；
//   - 逐项事件各计一次 Completed，terminal 时 Total == Completed。
//
// 幂等性：本函数是事件切片的纯函数，无时钟、随机或进程内状态。
func ReplaySyncJobStatus(runID string, events []syncJobStatusEvent) SyncJobStatus {
	status := SyncJobStatus{
		SchemaVersion: syncJobStatusSchemaVersion,
		RunID:         runID,
		ChangeCounts:  map[string]int{},
	}
	for _, event := range events {
		status.EventCount++
		if status.FirstEventAt == "" && event.TS != "" {
			status.FirstEventAt = event.TS
		}
		if event.TS != "" {
			status.LastEventAt = event.TS
		}
		switch event.Type {
		case "sync.file":
			status.Completed++
			if event.ChangeCode != "" {
				status.ChangeCounts[event.ChangeCode]++
			}
		case "sync.run":
			if nonTerminalRunStatuses[event.Status] {
				continue // 受理标记，不是终态。
			}
			// 终态 run 级事件；重复终态（重试追加）以最后一条为准。
			status.Phase = SyncJobPhaseTerminal
			status.Status = event.Status
			status.ErrorCode = event.ErrorCode
		}
	}
	if status.Phase != SyncJobPhaseTerminal {
		if status.Completed > 0 {
			status.Phase = SyncJobPhaseProgress
		} else if status.EventCount > 0 {
			status.Phase = SyncJobPhaseAccepted
		}
	} else {
		status.Total = status.Completed
	}
	if len(status.ChangeCounts) == 0 {
		status.ChangeCounts = nil
	}
	return status
}

// readSyncJobEvents 读取 vault event JSONL 中属于 runID 的事件（按写入序）。
func readSyncJobEvents(root, runID string) ([]syncJobStatusEvent, error) {
	path := filepath.Join(root, ".pinax", "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var events []syncJobStatusEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var raw struct {
			Type   string            `json:"type"`
			Status string            `json:"status"`
			TS     string            `json:"ts"`
			Facts  map[string]string `json:"facts"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		if raw.Type != "sync.run" && raw.Type != "sync.file" {
			continue
		}
		if raw.Facts["run_id"] != runID {
			continue
		}
		events = append(events, syncJobStatusEvent{
			Type:       raw.Type,
			Status:     raw.Status,
			TS:         raw.TS,
			ChangeCode: raw.Facts["change_code"],
			ErrorCode:  raw.Facts["error_code"],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// SyncLogsStatus 从 vault event JSONL 幂等重放一个 sync run 的 job status
// 投影。与 receipt（sync logs show）互补：本投影只依据事件流，因此对
// 崩溃中断（无 receipt/事件前缀）也能给出 accepted/progress 结论。
func (s *Service) SyncLogsStatus(_ context.Context, req SyncLogsRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.logs.status", err), err
	}
	events, err := readSyncJobEvents(root, req.RunID)
	if err != nil {
		return errorProjection("sync.logs.status", err), err
	}
	if len(events) == 0 {
		notFound := &domain.CommandError{Code: "sync_run_not_found", Message: "sync run events were not found", Hint: "Run pinax sync logs list --json"}
		return errorProjection("sync.logs.status", notFound), notFound
	}
	status := ReplaySyncJobStatus(req.RunID, events)
	projection := domain.NewProjection("sync.logs.status", "Sync job status replayed from the vault event JSONL.")
	projection.Facts["run_id"] = status.RunID
	projection.Facts["phase"] = status.Phase
	if status.Status != "" {
		projection.Facts["status"] = status.Status
	}
	projection.Facts["completed"] = fmt.Sprint(status.Completed)
	projection.Facts["total"] = fmt.Sprint(status.Total)
	projection.Facts["events"] = fmt.Sprint(status.EventCount)
	projection.Facts["replayable"] = "true"
	projection.Data = map[string]any{"status": status, "events": status.EventCount}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	return projection, nil
}
