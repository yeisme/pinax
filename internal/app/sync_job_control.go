package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/domain"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

// sync/delivery 长任务的本地执行控制（pinax-local-async-substrate-v1 §2.2/§2.3）：
// cancel 标记与同作用域单执行器 claim 都是 CLI-authored 结构化资产，落在
// `.pinax/sync-jobs/` 下。本地优先：不引入 daemon、租约心跳或分布式锁。
const (
	syncCancelSchemaVersion = "pinax.sync_cancel.v1"
	syncClaimSchemaVersion  = "pinax.sync_claim.v1"

	// syncClaimStaleAfter 是无心跳模型下的 claim 过期阈值：持有者超过该时长
	// 未释放即视为残留（崩溃残留或 SIGKILL），允许幂等抢占。
	syncClaimStaleAfter = time.Hour

	// SyncCancelledErrorCode 是执行器在项边界发现 cancel 标记后返回的稳定
	// 错误码；receipt/projection/事件流用它区分用户取消与执行失败。
	SyncCancelledErrorCode = "sync_cancelled"
)

// syncCancelMarker 是 `.pinax/sync-jobs/cancel/<run_id>.json` 的内容。
// 由 `pinax sync cancel` 写入（幂等：重复写只刷新 requested_at/reason），
// 执行器在项边界轮询读取；run 终态后由 status 投影继续展示。
type syncCancelMarker struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Reason        string `json:"reason,omitempty"`
	RequestedAt   string `json:"requested_at"`
	RequestedBy   string `json:"requested_by,omitempty"`
}

// syncScopeClaim 是 `.pinax/sync-jobs/claims/<scope>.json` 的内容：同一
// sync 作用域（vault + target）当前唯一执行器的自描述持有记录。
type syncScopeClaim struct {
	SchemaVersion string `json:"schema_version"`
	Scope         string `json:"scope"`
	RunID         string `json:"run_id"`
	Command       string `json:"command"`
	PID           int    `json:"pid"`
	Host          string `json:"host"`
	StartedAt     string `json:"started_at"`
}

// SyncCancelRequest 是 `pinax sync cancel` 的入参。RunID 为空时解析当前
// 作用域 claim 指向的活跃 run。
type SyncCancelRequest struct {
	VaultPath string
	RunID     string
	Reason    string
}

func syncCancelMarkerPath(root, runID string) string {
	return filepath.Join(root, ".pinax", "sync-jobs", "cancel", syncops.SanitizeString(runID)+".json")
}

func syncScopeClaimPath(root, scope string) string {
	return filepath.Join(root, ".pinax", "sync-jobs", "claims", syncops.SanitizeString(scope)+".json")
}

// writeSyncCancelMarker 幂等写入 cancel 标记并返回相对路径。标记是
// CLI-authored 结构化资产：只有 CLI（app service）创建，MCP/tool 不写。
func writeSyncCancelMarker(root, runID, reason string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", &domain.CommandError{Code: "sync_run_required", Message: "sync cancel requires a run id", Hint: "Run pinax sync cancel <run-id>, or omit the id to cancel the active run"}
	}
	marker := syncCancelMarker{
		SchemaVersion: syncCancelSchemaVersion,
		RunID:         syncops.SanitizeString(runID),
		Reason:        syncops.SanitizeString(reason),
		RequestedAt:   time.Now().UTC().Format(time.RFC3339),
		RequestedBy:   "pinax-cli",
	}
	path := syncCancelMarkerPath(root, runID)
	if err := writeJSONAsset(path, marker); err != nil {
		return "", err
	}
	return filepath.ToSlash(strings.TrimPrefix(path, root+string(os.PathSeparator))), nil
}

// readSyncCancelMarker 读取一个 run 的 cancel 标记；不存在时返回 ok=false。
func readSyncCancelMarker(root, runID string) (syncCancelMarker, bool, error) {
	b, err := os.ReadFile(syncCancelMarkerPath(root, runID))
	if err != nil {
		if os.IsNotExist(err) {
			return syncCancelMarker{}, false, nil
		}
		return syncCancelMarker{}, false, err
	}
	var marker syncCancelMarker
	if err := json.Unmarshal(b, &marker); err != nil {
		return syncCancelMarker{}, false, err
	}
	if marker.SchemaVersion != syncCancelSchemaVersion {
		return syncCancelMarker{}, false, fmt.Errorf("cancel marker schema version %q is not %q", marker.SchemaVersion, syncCancelSchemaVersion)
	}
	return marker, true, nil
}

// readSyncScopeClaim 读取一个作用域的当前 claim；不存在时返回 ok=false。
func readSyncScopeClaim(root, scope string) (syncScopeClaim, bool, error) {
	b, err := os.ReadFile(syncScopeClaimPath(root, scope))
	if err != nil {
		if os.IsNotExist(err) {
			return syncScopeClaim{}, false, nil
		}
		return syncScopeClaim{}, false, err
	}
	var claim syncScopeClaim
	if err := json.Unmarshal(b, &claim); err != nil {
		return syncScopeClaim{}, false, err
	}
	if claim.SchemaVersion != syncClaimSchemaVersion {
		return syncScopeClaim{}, false, fmt.Errorf("sync claim schema version %q is not %q", claim.SchemaVersion, syncClaimSchemaVersion)
	}
	return claim, true, nil
}

// activeSyncScopeRun 解析一个作用域 claim 指向的 run id（供 cancel 免 id
// 调用与状态展示使用）。
func activeSyncScopeRun(root, scope string) (string, bool) {
	claim, ok, err := readSyncScopeClaim(root, scope)
	if err != nil || !ok {
		return "", false
	}
	return claim.RunID, strings.TrimSpace(claim.RunID) != ""
}

// syncClaimHost 返回 claim 记录的主机标识；跨主机复制 vault 时持有者 pid
// 无法判定存活性，只按终态/时长判定。
func syncClaimHost() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return syncops.SanitizeString(host)
}

// syncClaimHolderAlive 判断 claim 持有者是否仍在执行：持有 run 已有终态
// receipt、或同主机持有进程已死、或持有时长超过过期阈值时视为可抢占。
func syncClaimHolderAlive(root string, claim syncScopeClaim) (bool, string) {
	if strings.TrimSpace(claim.RunID) != "" {
		if _, err := findSyncRunReceipt(root, claim.RunID); err == nil {
			return false, "holder_run_terminal"
		}
	}
	if claim.PID > 0 {
		if claim.Host == syncClaimHost() {
			if !processAlive(claim.PID) {
				return false, "holder_process_dead"
			}
		}
	}
	if started, err := time.Parse(time.RFC3339, claim.StartedAt); err == nil {
		if age := time.Since(started); age > syncClaimStaleAfter {
			return false, "holder_claim_stale"
		}
	}
	return true, ""
}

// claimSyncScope 以原子 O_EXCL 创建获取作用域执行权；已存在时按幂等抢占
// 规则判定（同 run 重入放行；持有者终态/进程死亡/超时视为残留可抢占），
// 其余情况返回携带持有者事实的可解释占用错误。
func claimSyncScope(root, scope, runID, command string) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil // 无作用域（未配置 target）时不启用互斥。
	}
	claim := syncScopeClaim{
		SchemaVersion: syncClaimSchemaVersion,
		Scope:         syncops.SanitizeString(scope),
		RunID:         syncops.SanitizeString(runID),
		Command:       syncops.SanitizeString(command),
		PID:           os.Getpid(),
		Host:          syncClaimHost(),
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	path := syncScopeClaimPath(root, scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		b, marshalErr := json.Marshal(claim)
		if marshalErr == nil {
			_, marshalErr = file.Write(append(b, '\n'))
		}
		if syncErr := file.Sync(); syncErr != nil {
			marshalErr = syncErr
		}
		if closeErr := file.Close(); closeErr != nil {
			marshalErr = closeErr
		}
		if marshalErr != nil {
			_ = os.Remove(path)
			return marshalErr
		}
		return nil
	}
	if !os.IsExist(err) {
		return err
	}
	holder, ok, readErr := readSyncScopeClaim(root, scope)
	if readErr != nil {
		return readErr
	}
	if !ok {
		// 竞态窗口内持有者刚好释放：重试一次原子创建。
		return claimSyncScope(root, scope, runID, command)
	}
	if holder.RunID == claim.RunID && strings.TrimSpace(holder.RunID) != "" {
		return nil // 幂等：同一 run 重入（重试/恢复）放行。
	}
	if alive, _ := syncClaimHolderAlive(root, holder); alive {
		return syncScopeBusyError(scope, holder)
	}
	// 残留 claim：原子替换后回读确认持有者仍是本 run，避免并发抢占双跑。
	if err := writeJSONAsset(path, claim); err != nil {
		return err
	}
	confirmed, ok, readErr := readSyncScopeClaim(root, scope)
	if readErr != nil {
		return readErr
	}
	if !ok || confirmed.RunID != claim.RunID {
		return syncScopeBusyError(scope, confirmed)
	}
	return nil
}

// releaseSyncScope 释放作用域执行权；只删除仍属于本 run 的 claim，绝不
// 误删其他执行者（含抢占者）的持有记录。
func releaseSyncScope(root, scope, runID string) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return
	}
	claim, ok, err := readSyncScopeClaim(root, scope)
	if err != nil || !ok {
		return
	}
	if claim.RunID != strings.TrimSpace(runID) {
		return
	}
	_ = os.Remove(syncScopeClaimPath(root, scope))
}

// syncScopeBusyError 把占用事实折叠进可解释错误：另一执行器能看到是谁、
// 哪个 run、哪个进程在占用，而不是静默双跑。
func syncScopeBusyError(scope string, holder syncScopeClaim) error {
	ageSeconds := 0
	if started, err := time.Parse(time.RFC3339, holder.StartedAt); err == nil {
		ageSeconds = int(time.Since(started).Seconds())
	}
	message := fmt.Sprintf("sync scope %q is already claimed by run %s", scope, holder.RunID)
	if holder.Command != "" {
		message = fmt.Sprintf("sync scope %q is already claimed by %s (run %s)", scope, holder.Command, holder.RunID)
	}
	commandErr := &domain.CommandError{
		Code:    "sync_scope_busy",
		Message: message,
		Hint:    "Wait for the active sync run to finish, or cancel it with pinax sync cancel " + holder.RunID,
	}
	commandErr.Hint += fmt.Sprintf(" [holder: pid=%d host=%s started_at=%s age_seconds=%d]", holder.PID, holder.Host, holder.StartedAt, ageSeconds)
	return commandErr
}

// SyncCancel 写入一个 sync run 的 cancel 标记。执行器在项边界发现标记后
// 停止后续项，已完成项的写入与 receipt 保留。标记幂等：对同一 run 重复
// cancel 只刷新 requested_at。
func (s *Service) SyncCancel(_ context.Context, req SyncCancelRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.cancel", err), err
	}
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		// 免 id 调用：解析当前作用域 claim 指向的活跃 run。
		if active, ok := activeSyncScopeRun(root, syncTargetCapsa); ok {
			runID = active
		} else {
			notFound := &domain.CommandError{Code: "sync_run_not_found", Message: "no active sync run was found to cancel", Hint: "Run pinax sync cancel <run-id>; list recent runs with pinax sync logs list --json"}
			return errorProjection("sync.cancel", notFound), notFound
		}
	}
	markerPath, err := writeSyncCancelMarker(root, runID, req.Reason)
	if err != nil {
		return errorProjection("sync.cancel", err), err
	}
	projection := domain.NewProjection("sync.cancel", "Sync run cancel marker written; the executor stops at the next item boundary.")
	projection.Status = "partial"
	projection.Facts["run_id"] = runID
	projection.Facts["cancel_requested"] = "true"
	projection.Facts["receipt_policy"] = "completed_items_preserved"
	projection.Actions = []domain.Action{
		{Name: "status", Command: fmt.Sprintf("pinax sync logs status %s --vault %s --json", runID, shellQuote(root))},
		{Name: "follow", Command: fmt.Sprintf("pinax sync logs tail --follow --vault %s", shellQuote(root))},
	}
	projection.Data = map[string]any{"run_id": runID, "cancel_requested": true, "marker_path": markerPath}
	projection.Evidence = []string{markerPath}
	return projection, nil
}

// syncJobControl 是一个执行中 sync run 的本地控制句柄：项边界 cancel 轮询
// 与逐项 live 事件（vault event JSONL + --events NDJSON sink）。
type syncJobControl struct {
	root      string
	runID     string
	command   string
	direction string
	backend   string
	policy    string
	sink      SyncEventSink

	// liveFileEvents 记录是否已发出过逐项 live 事件；run 收口时据此跳过
	// appendSyncRunEvent 的整计划补发，避免重复计数。
	liveFileEvents bool
	itemsCompleted int
	itemsTotal     int
}

func newSyncJobControl(root string, receipt SyncRunReceipt, policy string, sink SyncEventSink) *syncJobControl {
	return &syncJobControl{
		root:      root,
		runID:     receipt.RunID,
		command:   receipt.Command,
		direction: receipt.Direction,
		backend:   receipt.BackendKind,
		policy:    syncops.NormalizePathPolicy(policy),
		sink:      sink,
	}
}

// nil-safe：无控制句柄（只读/测试路径）时永不取消、不发光。
func (c *syncJobControl) cancelled() bool {
	if c == nil {
		return false
	}
	_, requested, err := readSyncCancelMarker(c.root, c.runID)
	return err == nil && requested
}

// syncCancelledError 是项边界发现 cancel 标记后的停止信号。
func (c *syncJobControl) cancelledError() error {
	return &domain.CommandError{
		Code:    SyncCancelledErrorCode,
		Message: "sync run was cancelled at an item boundary",
		Hint:    "Completed items and their receipt are preserved; rerun the sync command to continue",
	}
}

// emitItem 在一个项完成后发出 live 事件：vault event JSONL 的 sync.file
// 事实事件 + 可选的 --events NDJSON progress 事件。路径先经 path policy
// 脱敏，与收口期 appendSyncRunEvent 的字段约定保持一致。
func (c *syncJobControl) emitItem(kind, path, pathHash, changeCode, changeState string) {
	if c == nil {
		return
	}
	c.itemsCompleted++
	c.liveFileEvents = true
	facts := map[string]string{
		"run_id":           c.runID,
		"command":          c.command,
		"direction":        c.direction,
		"backend_kind":     c.backend,
		"kind":             kind,
		"operation_status": "applied",
		"change_state":     changeState,
	}
	redacted := syncops.RedactPath(path, c.policy)
	if redacted != "" {
		facts["path"] = redacted
	}
	hash := syncops.RedactPath(pathHash, c.policy)
	if hash != "" {
		facts["path_hash"] = hash
	}
	if changeCode != "" {
		facts["change_code"] = changeCode
	}
	appendEventWarned(c.root, "sync.file", "running", facts)
	emitSyncEvent(c.sink, SyncEvent{
		Type:        "progress",
		Phase:       "item",
		Direction:   c.direction,
		RunID:       c.runID,
		Completed:   c.itemsCompleted,
		Total:       c.itemsTotal,
		Operation:   kind,
		ChangeCode:  changeCode,
		Path:        redacted,
		PathHash:    hash,
		Status:      "applied",
		RemoteWrite: c.direction == string(syncplan.DirectionPush),
		LocalWrite:  c.direction == string(syncplan.DirectionPull),
	})
}

// emitAccepted 在 run 受理后立即写入受理事件，使 status 投影在执行期就能
// 重放出 accepted 相，外部 cancel 方也能从事件流发现 run_id。
func (c *syncJobControl) emitAccepted(remoteWrite bool) {
	if c == nil {
		return
	}
	appendEventWarned(c.root, "sync.run", "running", map[string]string{
		"run_id":       c.runID,
		"command":      c.command,
		"direction":    c.direction,
		"backend_kind": c.backend,
		"remote_write": fmt.Sprint(remoteWrite),
	})
}

// isSyncCancelledError 判定 err 是否为项边界取消信号。
func isSyncCancelledError(err error) bool {
	var commandErr *domain.CommandError
	return errors.As(err, &commandErr) && commandErr.Code == SyncCancelledErrorCode
}
