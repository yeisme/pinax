package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func TestSyncCancelMarkerRoundTripAndIdempotency(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	path, err := writeSyncCancelMarker(root, "sync_cancel_1", "user requested")
	if err != nil {
		t.Fatalf("writeSyncCancelMarker: %v", err)
	}
	if want := ".pinax/sync-jobs/cancel/sync_cancel_1.json"; path != want {
		t.Fatalf("marker path = %q, want %q", path, want)
	}
	marker, requested, err := readSyncCancelMarker(root, "sync_cancel_1")
	if err != nil || !requested {
		t.Fatalf("readSyncCancelMarker: marker=%#v requested=%v err=%v", marker, requested, err)
	}
	if marker.SchemaVersion != syncCancelSchemaVersion || marker.RunID != "sync_cancel_1" || marker.Reason != "user requested" {
		t.Fatalf("marker = %#v", marker)
	}

	// 幂等：重复写只刷新 requested_at，不报错、不产生第二份标记。
	if _, err := writeSyncCancelMarker(root, "sync_cancel_1", "retry"); err != nil {
		t.Fatalf("rewrite cancel marker: %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(root, ".pinax", "sync-jobs", "cancel"))
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("cancel dir entries=%d err=%v, want exactly 1 marker", len(entries), readErr)
	}

	// 另一个 run 无标记 → not requested。
	_, requested, err = readSyncCancelMarker(root, "sync_other_1")
	if err != nil || requested {
		t.Fatalf("unrelated run requested=%v err=%v, want false nil", requested, err)
	}

	// 空 run id 拒绝。
	if _, err := writeSyncCancelMarker(root, "  ", ""); err == nil {
		t.Fatal("empty run id must be rejected")
	}
}

func TestSyncCancelServiceProjection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()

	projection, err := svc.SyncCancel(context.Background(), SyncCancelRequest{VaultPath: root, RunID: "sync_cancel_svc_1", Reason: "too slow"})
	if err != nil {
		t.Fatalf("SyncCancel: %v", err)
	}
	for key, want := range map[string]string{
		"run_id":           "sync_cancel_svc_1",
		"cancel_requested": "true",
		"receipt_policy":   "completed_items_preserved",
	} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("fact %s = %q, want %q; facts=%#v", key, got, want, projection.Facts)
		}
	}
	if _, requested, readErr := readSyncCancelMarker(root, "sync_cancel_svc_1"); readErr != nil || !requested {
		t.Fatalf("marker after service cancel: requested=%v err=%v", requested, readErr)
	}

	// 免 id 调用但无活跃 claim → 可解释 not-found 错误。
	_, err = svc.SyncCancel(context.Background(), SyncCancelRequest{VaultPath: root})
	if err == nil {
		t.Fatal("cancel without run id and without active claim must fail")
	}
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != "sync_run_not_found" {
		t.Fatalf("err = %#v, want sync_run_not_found CommandError", err)
	}

	// 免 id 调用：claim 指向活跃 run 时解析成功。
	if err := claimSyncScope(root, "capsa", "sync_active_1", "sync.push"); err != nil {
		t.Fatalf("claimSyncScope: %v", err)
	}
	defer releaseSyncScope(root, "capsa", "sync_active_1")
	projection, err = svc.SyncCancel(context.Background(), SyncCancelRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("SyncCancel(active): %v", err)
	}
	if projection.Facts["run_id"] != "sync_active_1" {
		t.Fatalf("resolved run_id = %q, want sync_active_1", projection.Facts["run_id"])
	}
}

func TestSyncScopeClaimExclusiveAndExplainableBusy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := claimSyncScope(root, "capsa", "sync_hold_1", "sync.push"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	claim, ok, err := readSyncScopeClaim(root, "capsa")
	if err != nil || !ok {
		t.Fatalf("readSyncScopeClaim: ok=%v err=%v", ok, err)
	}
	if claim.SchemaVersion != syncClaimSchemaVersion || claim.RunID != "sync_hold_1" || claim.Command != "sync.push" || claim.PID != os.Getpid() || claim.Scope != "capsa" {
		t.Fatalf("claim = %#v", claim)
	}

	// 同 run 重入：幂等放行。
	if err := claimSyncScope(root, "capsa", "sync_hold_1", "sync.push"); err != nil {
		t.Fatalf("same-run reclaim must be idempotent: %v", err)
	}

	// 另一 run：可解释占用错误，携带持有者事实。
	err = claimSyncScope(root, "capsa", "sync_second_1", "sync.pull")
	if err == nil {
		t.Fatal("second executor must receive a busy error, not silent double-run")
	}
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != "sync_scope_busy" {
		t.Fatalf("err = %#v, want sync_scope_busy CommandError", err)
	}
	for _, want := range []string{"sync_hold_1", "sync.push"} {
		if !strings.Contains(commandErr.Message, want) && !strings.Contains(commandErr.Hint, want) {
			t.Fatalf("busy error must explain holder %s: %s / %s", want, commandErr.Message, commandErr.Hint)
		}
	}
	if !strings.Contains(commandErr.Hint, "pid=") {
		t.Fatalf("busy error must carry holder pid: %s", commandErr.Hint)
	}

	// 释放后可重新获取；释放者不能误删他人 claim。
	releaseSyncScope(root, "capsa", "sync_second_1") // 非 holder：无操作。
	if _, ok, _ := readSyncScopeClaim(root, "capsa"); !ok {
		t.Fatal("non-holder release must not remove the claim")
	}
	releaseSyncScope(root, "capsa", "sync_hold_1")
	if err := claimSyncScope(root, "capsa", "sync_second_1", "sync.pull"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}

	// 并发初始 claim：恰一个赢家，其余收到 busy。
	raceRoot := t.TempDir()
	var mu sync.Mutex
	winners, busy := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := claimSyncScope(raceRoot, "capsa", "sync_race_"+string(rune('a'+i)), "sync.push")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners++
			} else if strings.Contains(err.Error(), "sync_scope_busy") {
				busy++
			}
		}(i)
	}
	wg.Wait()
	if winners != 1 || busy != 7 {
		t.Fatalf("concurrent claims: winners=%d busy=%d, want exactly 1 winner and 7 busy", winners, busy)
	}
}

func TestSyncScopeClaimPreemptsStaleHolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	writeClaim := func(claim syncScopeClaim) {
		t.Helper()
		if err := writeJSONAsset(syncScopeClaimPath(root, "capsa"), claim); err != nil {
			t.Fatalf("seed claim: %v", err)
		}
	}

	// 1) 持有 run 已终态（receipt 存在）→ 抢占。
	receipt := SyncRunReceipt{SchemaVersion: syncRunSchemaVersion, RunID: "sync_done_1", Command: "sync.push", Direction: "push", Status: "success", BackendKind: "embedded", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if _, err := writeSyncRunReceipt(root, receipt); err != nil {
		t.Fatalf("writeSyncRunReceipt: %v", err)
	}
	writeClaim(syncScopeClaim{SchemaVersion: syncClaimSchemaVersion, Scope: "capsa", RunID: "sync_done_1", Command: "sync.push", PID: os.Getpid(), Host: syncClaimHost(), StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if err := claimSyncScope(root, "capsa", "sync_new_1", "sync.push"); err != nil {
		t.Fatalf("terminal holder must be preemptable: %v", err)
	}
	confirmed, _, _ := readSyncScopeClaim(root, "capsa")
	if confirmed.RunID != "sync_new_1" {
		t.Fatalf("claim after preemption = %q, want sync_new_1", confirmed.RunID)
	}

	// 2) 同主机持有进程已死 → 抢占。
	deadCmd := exec.Command("sleep", "0")
	if err := deadCmd.Run(); err != nil {
		t.Fatalf("spawn short-lived process: %v", err)
	}
	writeClaim(syncScopeClaim{SchemaVersion: syncClaimSchemaVersion, Scope: "capsa", RunID: "sync_dead_1", Command: "sync.push", PID: deadCmd.Process.Pid, Host: syncClaimHost(), StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if err := claimSyncScope(root, "capsa", "sync_new_2", "sync.push"); err != nil {
		t.Fatalf("dead holder must be preemptable: %v", err)
	}

	// 3) 超时残留（无 receipt、pid 存活）→ 抢占。
	writeClaim(syncScopeClaim{SchemaVersion: syncClaimSchemaVersion, Scope: "capsa", RunID: "sync_stale_1", Command: "sync.push", PID: os.Getpid(), Host: syncClaimHost(), StartedAt: time.Now().UTC().Add(-2 * syncClaimStaleAfter).Format(time.RFC3339)})
	if err := claimSyncScope(root, "capsa", "sync_new_3", "sync.push"); err != nil {
		t.Fatalf("stale holder must be preemptable: %v", err)
	}

	// 4) 新鲜且存活的持有者 → 不允许抢占。
	writeClaim(syncScopeClaim{SchemaVersion: syncClaimSchemaVersion, Scope: "capsa", RunID: "sync_live_1", Command: "sync.push", PID: os.Getpid(), Host: syncClaimHost(), StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if err := claimSyncScope(root, "capsa", "sync_new_4", "sync.push"); err == nil {
		t.Fatal("live holder must not be preemptable")
	}
}

func TestSyncJobControlItemBoundaryAndLiveEvents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	receipt := SyncRunReceipt{RunID: "sync_ctrl_1", Command: "sync.pull", Direction: "pull", BackendKind: "embedded"}
	var seen []SyncEvent
	ctrl := newSyncJobControl(root, receipt, "default", func(event SyncEvent) { seen = append(seen, event) })
	ctrl.itemsTotal = 3

	if ctrl.cancelled() {
		t.Fatal("no marker yet: cancelled must be false")
	}
	ctrl.emitItem("download_blob", "notes/a.md", "", "A", "applied")
	ctrl.emitItem("delete_local", "notes/old.md", "", "D", "applied")
	if !ctrl.liveFileEvents || ctrl.itemsCompleted != 2 {
		t.Fatalf("liveFileEvents=%v itemsCompleted=%d, want true 2", ctrl.liveFileEvents, ctrl.itemsCompleted)
	}
	if len(seen) != 2 || seen[0].RunID != "sync_ctrl_1" || seen[0].ChangeCode != "A" || seen[1].Completed != 2 {
		t.Fatalf("sink events = %#v", seen)
	}

	// 置 cancel 标记后：项边界轮询为真，返回稳定错误码。
	if _, err := writeSyncCancelMarker(root, "sync_ctrl_1", ""); err != nil {
		t.Fatalf("writeSyncCancelMarker: %v", err)
	}
	if !ctrl.cancelled() {
		t.Fatal("cancelled must be true after marker")
	}
	if err := ctrl.cancelledError(); !isSyncCancelledError(err) {
		t.Fatalf("cancelledError = %#v, want %s", err, SyncCancelledErrorCode)
	}

	// live 事件已写入 vault event JSONL，可被 status 重放读取。
	events, err := readSyncJobEvents(root, "sync_ctrl_1")
	if err != nil {
		t.Fatalf("readSyncJobEvents: %v", err)
	}
	if len(events) != 2 || events[0].Type != "sync.file" || events[0].ChangeCode != "A" {
		t.Fatalf("live events in JSONL = %#v", events)
	}

	// path policy=omitted：事件不携带路径。
	hashCtrl := newSyncJobControl(root, receipt, "omitted", nil)
	hashCtrl.emitItem("download_blob", "notes/secret.md", "", "A", "applied")
	raw, _ := os.ReadFile(filepath.Join(root, ".pinax", "events.jsonl"))
	if strings.Contains(string(raw), "notes/secret.md") {
		t.Fatalf("omitted policy leaked path: %s", raw)
	}
}

func TestReplaySyncJobStatusCancelledTerminal(t *testing.T) {
	t.Parallel()
	events := []syncJobStatusEvent{
		jobEvent("sync.run", "running", "2026-09-07T10:00:00Z", ""),
		jobEvent("sync.file", "running", "2026-09-07T10:00:01Z", "A"),
		jobEvent("sync.file", "running", "2026-09-07T10:00:02Z", "M"),
		jobEvent("sync.run", "cancelled", "2026-09-07T10:00:03Z", ""),
	}
	status := ReplaySyncJobStatus("sync_c_1", events)
	if status.Phase != SyncJobPhaseTerminal || status.Status != "cancelled" {
		t.Fatalf("phase/status = %s/%s, want terminal/cancelled", status.Phase, status.Status)
	}
	if status.Completed != 2 || status.Total != 2 {
		t.Fatalf("completed/total = %d/%d, want 2/2 (unfinished items are not partial state)", status.Completed, status.Total)
	}
}

func TestSyncLogsStatusReportsCancelRequestedForRunningRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()

	// 运行中 run：受理 + 一项进度，无终态。
	if err := appendEvent(root, "sync.run", "running", map[string]string{"run_id": "sync_live_2", "command": "sync.pull", "direction": "pull", "backend_kind": "embedded"}); err != nil {
		t.Fatalf("appendEvent(accepted): %v", err)
	}
	if err := appendEvent(root, "sync.file", "running", map[string]string{"run_id": "sync_live_2", "kind": "download_blob", "change_code": "A", "change_state": "applied"}); err != nil {
		t.Fatalf("appendEvent(item): %v", err)
	}
	projection, err := svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_live_2"})
	if err != nil {
		t.Fatalf("SyncLogsStatus: %v", err)
	}
	if projection.Facts["phase"] != SyncJobPhaseProgress || projection.Facts["cancel_requested"] != "" {
		t.Fatalf("before cancel: phase=%s cancel_requested=%q, want progress and empty", projection.Facts["phase"], projection.Facts["cancel_requested"])
	}

	// 置 cancel 标记 → status 投影携带 cancel_requested 事实。
	if _, err := writeSyncCancelMarker(root, "sync_live_2", "user stop"); err != nil {
		t.Fatalf("writeSyncCancelMarker: %v", err)
	}
	projection, err = svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_live_2"})
	if err != nil {
		t.Fatalf("SyncLogsStatus(cancelled): %v", err)
	}
	if projection.Facts["cancel_requested"] != "true" {
		t.Fatalf("cancel_requested = %q, want true; facts=%#v", projection.Facts["cancel_requested"], projection.Facts)
	}
	if projection.Facts["cancel_requested_at"] == "" {
		t.Fatal("cancel_requested_at must be present")
	}

	// 终态后：cancel_requested 事实退场（结论已由 terminal 状态携带）。
	if err := appendEvent(root, "sync.run", "cancelled", map[string]string{"run_id": "sync_live_2", "command": "sync.pull", "direction": "pull", "backend_kind": "embedded", "error_code": SyncCancelledErrorCode}); err != nil {
		t.Fatalf("appendEvent(terminal): %v", err)
	}
	projection, err = svc.SyncLogsStatus(context.Background(), SyncLogsRequest{VaultPath: root, RunID: "sync_live_2"})
	if err != nil {
		t.Fatalf("SyncLogsStatus(terminal): %v", err)
	}
	if projection.Facts["status"] != "cancelled" || projection.Facts["cancel_requested"] != "" {
		t.Fatalf("terminal: status=%s cancel_requested=%q, want cancelled and empty", projection.Facts["status"], projection.Facts["cancel_requested"])
	}
}

// TestSyncPushScopeBusyBlocksSecondExecutor 验证双执行器互斥（spec 场景）：
// 持有 claim 的 push 正常执行，第二个并发作用域执行被拒并收到可解释
// 占用错误，被拒方写出 failed receipt（sync_scope_busy）。
func TestSyncPushScopeBusyBlocksSecondExecutor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := t.TempDir()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "busy.md"), "# Busy\n\ncontent\n")
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: root, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "dev", SecretRef: "test-secret", EncryptionSecretRef: "plain:test-secret"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// 手工持有同作用域 claim（模拟另一执行器在跑）。
	if err := claimSyncScope(root, "cloud", "sync_holder_run", "sync.push"); err != nil {
		t.Fatalf("claimSyncScope: %v", err)
	}
	defer releaseSyncScope(root, "cloud", "sync_holder_run")

	projection, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud", Yes: true})
	if err == nil {
		t.Fatal("second push must be rejected while the scope is claimed")
	}
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != "sync_scope_busy" {
		t.Fatalf("err = %#v, want sync_scope_busy CommandError", err)
	}
	if projection.Facts["remote_write"] != "false" || projection.Facts["run_id"] == "" {
		t.Fatalf("busy projection facts = %#v", projection.Facts)
	}
	// 被拒 run 留下 failed receipt 供追溯。
	found, findErr := findSyncRunReceipt(root, projection.Facts["run_id"])
	if findErr != nil || found.Receipt.Status != "failed" {
		t.Fatalf("rejected run receipt = %#v err=%v, want failed receipt", found.Receipt, findErr)
	}
}

// TestSyncPullCancelledAtItemBoundary 覆盖 spec「取消长同步」场景：vault B
// pull 期间外部（真实用户路径：从事件流发现 run_id）写 cancel 标记，执行器
// 在项边界停止——已完成项落盘并保留 receipt，未执行项不产生部分状态，
// 事件流以 terminal/cancelled 收口，status 重放与之一致。全程 file:// 本地
// 对象存储，零真实远端调用。
func TestSyncPullCancelledAtItemBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := t.TempDir()
	source := t.TempDir()
	target := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: source, Title: "Source"}); err != nil {
		t.Fatalf("init source: %v", err)
	}
	const noteCount = 120
	for i := 0; i < noteCount; i++ {
		writeFile(t, filepath.Join(source, "notes", fmt.Sprintf("note-%03d.md", i)), fmt.Sprintf("# Note %d\n\nbody %d\n", i, i))
	}
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: source, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "dev_a", SecretRef: "test-secret", EncryptionSecretRef: "plain:test-secret"}); err != nil {
		t.Fatalf("cloud login source: %v", err)
	}
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: source, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: target, Title: "Target"}); err != nil {
		t.Fatalf("init target: %v", err)
	}
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: target, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "dev_b", SecretRef: "test-secret", EncryptionSecretRef: "plain:test-secret"}); err != nil {
		t.Fatalf("cloud login target: %v", err)
	}

	type pullOutcome struct {
		projection domain.Projection
		err        error
	}
	done := make(chan pullOutcome, 1)
	go func() {
		projection, err := svc.SyncPull(ctx, SyncRequest{VaultPath: target, Target: "cloud", Yes: true})
		done <- pullOutcome{projection: projection, err: err}
	}()

	// 外部取消方：轮询事件流发现本 run 的受理事件，提取 run_id 后写标记。
	eventsPath := filepath.Join(target, ".pinax", "events.jsonl")
	runID := ""
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(eventsPath); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if !strings.Contains(line, `"sync.pull"`) || !strings.Contains(line, `"status":"running"`) {
					continue
				}
				var event struct {
					Type  string            `json:"type"`
					Facts map[string]string `json:"facts"`
				}
				if json.Unmarshal([]byte(line), &event) == nil && event.Facts["run_id"] != "" {
					runID = event.Facts["run_id"]
				}
			}
		}
		if runID != "" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if runID == "" {
		t.Fatal("accepted event with run_id was not observed in the event stream")
	}
	// 等第一个逐项事件出现后再取消，验证「当前项完成后停止」的项边界语义
	// （受理后立刻取消等价于 0 项完成，同样是合法边界）。
	sawItem := false
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(eventsPath); err == nil && strings.Contains(string(raw), runID) && strings.Count(string(raw), `"sync.file"`) > 0 {
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.Contains(line, runID) && strings.Contains(line, `"sync.file"`) {
					sawItem = true
					break
				}
			}
		}
		if sawItem {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !sawItem {
		t.Fatal("no item event was observed before cancel")
	}
	if _, err := svc.SyncCancel(ctx, SyncCancelRequest{VaultPath: target, RunID: runID, Reason: "test cancel"}); err != nil {
		t.Fatalf("SyncCancel: %v", err)
	}

	outcome := <-done
	if outcome.err != nil {
		t.Fatalf("cancelled pull returned error: %v (cancel is an expected outcome)", outcome.err)
	}
	projection := outcome.projection
	if projection.Facts["cancelled"] != "true" || projection.Facts["run_id"] != runID {
		t.Fatalf("cancelled pull facts = %#v", projection.Facts)
	}

	// 已完成项落盘保留；未执行项不出现。
	applied := 0
	entries, readErr := os.ReadDir(filepath.Join(target, "notes"))
	if readErr != nil {
		t.Fatalf("read target notes: %v", readErr)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			applied++
		}
	}
	if applied == 0 || applied >= noteCount {
		t.Fatalf("applied notes = %d, want strictly between 0 and %d (item-boundary stop)", applied, noteCount)
	}
	if got := projection.Facts["files_applied"]; got != fmt.Sprint(applied) {
		t.Fatalf("files_applied fact = %s, want %d", got, applied)
	}

	// receipt 保留且状态为 cancelled。
	record, err := findSyncRunReceipt(target, runID)
	if err != nil {
		t.Fatalf("cancelled run receipt missing: %v", err)
	}
	if record.Receipt.Status != "cancelled" || record.Receipt.Error == nil || record.Receipt.Error.Code != SyncCancelledErrorCode {
		t.Fatalf("receipt status/error = %s/%#v, want cancelled/%s", record.Receipt.Status, record.Receipt.Error, SyncCancelledErrorCode)
	}

	// status 重放：terminal/cancelled，且逐项事件计数与落盘一致。
	statusProjection, err := svc.SyncLogsStatus(ctx, SyncLogsRequest{VaultPath: target, RunID: runID})
	if err != nil {
		t.Fatalf("SyncLogsStatus: %v", err)
	}
	if statusProjection.Facts["phase"] != SyncJobPhaseTerminal || statusProjection.Facts["status"] != "cancelled" {
		t.Fatalf("status replay = %s/%s, want terminal/cancelled; facts=%#v", statusProjection.Facts["phase"], statusProjection.Facts["status"], statusProjection.Facts)
	}
	if got := statusProjection.Facts["completed"]; got != fmt.Sprint(applied) {
		t.Fatalf("status replay completed = %s, want %d (match applied files)", got, applied)
	}

	// claim 已释放：后续 sync 可立即执行。
	if _, ok, _ := readSyncScopeClaim(target, "cloud"); ok {
		t.Fatal("scope claim must be released after the run finished")
	}
}
