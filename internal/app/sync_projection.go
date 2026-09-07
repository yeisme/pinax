package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

// cloudSyncRun carries the state of one cloud sync command so the projection
// builder can be decomposed into load/plan/execute phases without threading a
// dozen locals through every branch.
type cloudSyncRun struct {
	ctx            context.Context
	command        string
	root           string
	req            SyncRequest
	direction      syncplan.Direction
	outputTarget   string
	pathPolicy     string
	state          pinaxcloud.State
	receipt        SyncRunReceipt
	started        time.Time
	localManifest  pinaxcloud.Manifest
	baseManifest   pinaxcloud.Manifest
	baseRevision   string
	remoteSnapshot cloudRemoteSnapshot
	remoteLoaded   bool
	plan           syncplan.Plan
	localDiffPlan  syncplan.Plan
	planErr        error
	ctrl           *syncJobControl
	claimed        bool
}

func buildCloudSyncProjection(ctx context.Context, command, root string, req SyncRequest, direction syncplan.Direction) (domain.Projection, error) {
	r, projection, err := newCloudSyncRun(ctx, command, root, req, direction)
	if err != nil {
		return projection, err
	}
	if errors.Is(r.planErr, syncplan.ErrRevisionConflict) {
		return r.projectRevisionConflict()
	}
	if r.planErr != nil {
		projection := errorProjection(r.command, r.planErr)
		return projection, r.planErr
	}
	if r.direction == syncplan.DirectionPush && r.req.Yes && !r.req.DryRun && isExecutableCloudState(r.state) {
		if err := r.claimExecutorScope(); err != nil {
			return r.projectScopeBusy(err)
		}
		defer r.releaseExecutorScope()
		return r.executeCloudPush()
	}
	if r.direction == syncplan.DirectionPull && r.req.Yes && !r.req.DryRun && isExecutableCloudState(r.state) {
		if err := r.claimExecutorScope(); err != nil {
			return r.projectScopeBusy(err)
		}
		defer r.releaseExecutorScope()
		return r.executeCloudPull()
	}
	return r.projectPlanOnly()
}

// claimExecutorScope 获取同作用域（vault + target）单执行器原子 claim
// （pinax-local-async-substrate-v1 §2.3）：另一执行器持有时返回携带持有者
// 事实的可解释占用错误，绝不静默双跑。
func (r *cloudSyncRun) claimExecutorScope() error {
	if err := claimSyncScope(r.root, r.outputTarget, r.receipt.RunID, r.command); err != nil {
		return err
	}
	r.claimed = true
	return nil
}

func (r *cloudSyncRun) releaseExecutorScope() {
	if !r.claimed {
		return
	}
	releaseSyncScope(r.root, r.outputTarget, r.receipt.RunID)
	r.claimed = false
}

// projectScopeBusy 把 scope 占用错误折叠为可解释投影 + failed receipt，
// 供 `sync logs` 追溯被拒 run；不执行任何项。
func (r *cloudSyncRun) projectScopeBusy(err error) (domain.Projection, error) {
	commandErr := commandErrorFromError(err)
	projection := domain.NewErrorProjection(r.command, commandErr)
	projection.Facts["run_id"] = r.receipt.RunID
	projection.Facts["remote_write"] = "false"
	projection.Facts["local_write"] = "false"
	projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs list --vault %s --json", shellQuote(r.root))}}
	receiptOut, receiptPath, receiptErr := finishSyncRunWithFileEvents(r.root, r.receipt, r.plan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started, true)
	r.receipt = receiptOut
	if receiptErr != nil {
		return errorProjection(r.command, receiptErr), receiptErr
	}
	projection.Evidence = []string{receiptPath}
	projection.Data = map[string]any{"receipt": r.receipt}
	return projection, commandErr
}

// projectCancelled 收口一个项边界取消的 run（§2.2）：receipt 状态 cancelled、
// 已完成项保留、未执行项不产生部分状态。取消是用户请求的预期结果，
// projection 报 partial + cancelled=true 而非 failed，退出码为 0。
func (r *cloudSyncRun) projectCancelled(summary string, extraFacts map[string]string) (domain.Projection, error) {
	commandErr := &domain.CommandError{
		Code:    SyncCancelledErrorCode,
		Message: "sync run was cancelled at an item boundary",
		Hint:    "Completed items and their receipt are preserved; rerun the sync command to continue",
	}
	projection := domain.NewProjection(r.command, summary)
	projection.Status = "partial"
	projection.Facts["run_id"] = r.receipt.RunID
	projection.Facts["cancelled"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Facts["local_write"] = fmt.Sprint(r.receipt.LocalWrite)
	projection.Facts["completed_items"] = fmt.Sprint(r.ctrl.itemsCompleted)
	projection.Facts["total_items"] = fmt.Sprint(r.ctrl.itemsTotal)
	for key, value := range extraFacts {
		projection.Facts[key] = value
	}
	projection.Actions = []domain.Action{
		{Name: "status", Command: fmt.Sprintf("pinax sync logs status %s --vault %s --json", r.receipt.RunID, shellQuote(r.root))},
		{Name: "retry", Command: fmt.Sprintf("pinax sync %s --target %s --vault %s --yes", strings.ToLower(strings.TrimPrefix(r.command, "sync.")), r.outputTarget, shellQuote(r.root))},
	}
	receiptOut, receiptPath, receiptErr := finishSyncRunWithFileEvents(r.root, r.receipt, r.plan, "cancelled", commandErr, projection.Actions, r.pathPolicy, r.started, !r.ctrl.liveFileEvents)
	r.receipt = receiptOut
	if receiptErr != nil {
		return errorProjection(r.command, receiptErr), receiptErr
	}
	if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
		warnPersistFailure("sync state", err)
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "done", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "cancelled", RemoteWrite: false, LocalWrite: r.receipt.LocalWrite})
	projection.Evidence = []string{receiptPath}
	projection.Data = map[string]any{"receipt": r.receipt, "completed_items": r.ctrl.itemsCompleted, "total_items": r.ctrl.itemsTotal}
	return projection, nil
}

// newCloudSyncRun loads vault state, manifests, the remote head (when the
// direction needs it), and builds the sync plans. Error returns carry the
// already-built projection for the failure path.
func newCloudSyncRun(ctx context.Context, command, root string, req SyncRequest, direction syncplan.Direction) (*cloudSyncRun, domain.Projection, error) {
	r := &cloudSyncRun{ctx: ctx, command: command, root: root, req: req, direction: direction, started: time.Now()}
	r.outputTarget = syncOutputTarget(req.Target)
	r.pathPolicy = syncops.NormalizePathPolicy(req.PathPolicy)
	state, err := cloudStateForSync(root, req)
	if err != nil {
		projection, projErr := cloudStateErrorProjection(command, root, err)
		return nil, projection, projErr
	}
	r.state = state
	if gateErr := syncCapabilityGate(root); gateErr != nil {
		projection := errorProjection(command, gateErr)
		projection.Facts["remote_write"] = "false"
		return nil, projection, gateErr
	}
	r.receipt = syncRunStart(command, direction, r.state, r.pathPolicy, r.outputTarget)
	// 受理即入账（pinax-local-async-substrate-v1 §2.4）：run 级受理事件让
	// status 投影在执行期就能重放出 accepted 相，外部 cancel 方也能从
	// 事件流发现 run_id。
	r.ctrl = newSyncJobControl(root, r.receipt, r.pathPolicy, req.LiveEvents)
	r.ctrl.itemsTotal = 0 // 计划已知后在下方回填。
	r.ctrl.emitAccepted(false)
	emitSyncEvent(req.LiveEvents, SyncEvent{Type: "progress", Phase: "scan", Direction: string(direction), RunID: r.receipt.RunID, Status: "running", RemoteWrite: false})
	r.localManifest, err = buildLocalCloudManifest(root, r.state)
	if err != nil {
		projection := errorProjection(command, err)
		return nil, projection, err
	}
	baseManifest, cachedRevision, cacheErr := readCachedCloudManifest(root, r.state)
	if cacheErr != nil {
		projection := errorProjection(command, cacheErr)
		return nil, projection, cacheErr
	}
	r.baseManifest = baseManifest
	r.baseRevision = strings.TrimSpace(req.BaseRevision)
	if r.baseRevision == "" {
		r.baseRevision = cachedRevision
	}
	if cachedRevision != r.baseRevision {
		r.baseManifest = pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}
	}
	// diff and push dry-run MUST read the real remote head (pinax-passphrase-s3-
	// bootstrap task 6.7) so the result reflects remote-aware state. Only a real
	// pull hard-fails when the remote cannot be read; diff and push dry-run
	// degrade to a cached (local-only) comparison when the remote is unavailable
	// or the credential cannot be unlocked, and MUST mark remote_checked=false so
	// the cached result cannot pass as a pre-backup check (task 6.8).
	shouldLoadRemote := isExecutableCloudState(r.state) && (direction == syncplan.DirectionPull && req.Yes && !req.DryRun ||
		direction == syncplan.DirectionDiff ||
		direction == syncplan.DirectionPush && req.DryRun)
	if shouldLoadRemote {
		emitSyncEvent(req.LiveEvents, SyncEvent{Type: "progress", Phase: "remote_head", Direction: string(direction), RunID: r.receipt.RunID, Status: "running"})
		snapshot, snapshotErr := loadCloudRemoteSnapshotWithCredential(ctx, r.state, root, req.ProjectUnlockSource)
		if snapshotErr != nil {
			if direction == syncplan.DirectionPull && req.Yes && !req.DryRun {
				projection := errorProjection(command, snapshotErr)
				return nil, projection, snapshotErr
			}
		} else {
			r.remoteSnapshot = snapshot
			r.remoteLoaded = true
		}
	}
	remoteRevision := strings.TrimSpace(req.RemoteRevision)
	if remoteRevision == "" {
		remoteRevision = strings.TrimSpace(r.remoteSnapshot.RevisionID)
	}
	if remoteRevision == "" {
		remoteRevision = r.baseRevision
	}
	if err := negotiateSyncManifestCapability(r.localManifest, r.remoteSnapshot.Manifest); err != nil {
		projection := errorProjection(command, err)
		addCloudSyncFacts(&projection, r.state, syncplan.Plan{})
		projection.Facts["local_manifest_version"] = r.localManifest.SchemaVersion
		projection.Facts["remote_manifest_version"] = r.remoteSnapshot.Manifest.SchemaVersion
		return nil, projection, err
	}
	if r.localManifest.SchemaVersion != "" && r.baseManifest.SchemaVersion != r.localManifest.SchemaVersion {
		r.baseManifest = pinaxcloud.Manifest{SchemaVersion: r.localManifest.SchemaVersion}
	}
	r.plan, r.planErr = syncplan.BuildPlan(syncplan.Request{Direction: direction, Target: r.outputTarget, LocalManifest: r.localManifest, BaseManifest: r.baseManifest, RemoteManifest: r.remoteSnapshot.Manifest, BaseRevision: r.baseRevision, RemoteRevision: remoteRevision, DryRun: req.DryRun, Yes: req.Yes})
	r.ctrl.itemsTotal = len(r.plan.Operations)
	emitSyncEvent(req.LiveEvents, SyncEvent{Type: "progress", Phase: "plan", Direction: string(direction), RunID: r.receipt.RunID, Completed: 0, Total: len(r.plan.Operations), Status: "planned", Facts: map[string]string{"scope": syncOutputScope(r.remoteLoaded)}})
	localDiffPlan, localDiffErr := syncplan.BuildPlan(syncplan.Request{Direction: syncplan.DirectionDiff, Target: r.outputTarget, LocalManifest: r.localManifest, BaseManifest: r.baseManifest, RemoteManifest: r.remoteSnapshot.Manifest, BaseRevision: r.baseRevision, RemoteRevision: remoteRevision, DryRun: true, Yes: true})
	r.localDiffPlan = localDiffPlan
	if localDiffErr != nil && r.planErr == nil {
		r.planErr = localDiffErr
	}
	return r, domain.Projection{}, nil
}

// attachContentDiff adds metadata/content diff payloads when the request asked
// for them.
func (r *cloudSyncRun) attachContentDiff(data map[string]any, plan syncplan.Plan, remoteManifest pinaxcloud.Manifest) {
	view := buildSyncOutputView(plan, r.baseManifest, r.localManifest, remoteManifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: "planned", PathPolicy: r.pathPolicy})
	if strings.EqualFold(strings.TrimSpace(r.req.Preview), "diff") {
		data["metadata_diff"] = buildSyncMetadataDiff(view)
	}
	if r.req.ContentDiff {
		data["content_diff"] = buildSyncContentDiff(r.root, plan, r.baseManifest, r.localManifest, remoteManifest, r.pathPolicy)
	}
}

func (r *cloudSyncRun) projectRevisionConflict() (domain.Projection, error) {
	commandErr := &domain.CommandError{Code: "REVISION_CONFLICT", Message: "cloud revision conflict", Hint: "Review the conflict queue and resolve manually, then retry sync"}
	projection := domain.NewErrorProjection(r.command, commandErr)
	projection.Actions = append(syncConflictActions(r.root, nil), domain.Action{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", r.receipt.RunID, shellQuote(r.root))})
	receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started)
	r.receipt = receiptOut
	if receiptErr == nil {
		if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
			warnPersistFailure("sync state", err)
		}
		projection.Facts["run_id"] = r.receipt.RunID
		projection.Evidence = []string{receiptPath}
	}
	addCloudSyncFacts(&projection, r.state, r.plan)
	addCapsaBridgeFacts(&projection, r.req.Target)
	addCloudContentFacts(&projection, r.localManifest)
	data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "receipt": r.receipt}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: "failed", PathPolicy: r.pathPolicy}))
	r.attachContentDiff(data, r.plan, r.remoteSnapshot.Manifest)
	projection.Data = data
	return projection, commandErr
}

func (r *cloudSyncRun) executeCloudPush() (domain.Projection, error) {
	if gateErr := syncManifestRemoteWriteGate(r.root); gateErr != nil {
		projection := errorProjection(r.command, gateErr)
		projection.Facts["remote_write"] = "false"
		projection.Facts["local_write"] = "false"
		projection.Actions = []domain.Action{{Name: "manifest_status", Command: fmt.Sprintf("pinax sync manifest audit --vault %s --json", shellQuote(r.root))}}
		return projection, gateErr
	}
	// Up-to-date fast path (pinax-passphrase-s3-bootstrap task 6.8): read the
	// real remote head and compare the local manifest content against it. When
	// the entries and delete markers already match the remote there is nothing
	// to push, so report up_to_date=true with a remote-aware read-back
	// (distinguishable from a blocked or failed push) instead of committing a
	// no-op revision. Any difference (content, mode, or delete marker) — or any
	// remote object still encrypted under a previous key derivation, which a
	// key-rotation push must rewrite even though the content is unchanged —
	// falls through to the normal rebase path so a needed push is never skipped.
	upToDateSnapshot, upToDateErr := loadCloudRemoteSnapshotWithCredential(r.ctx, r.state, r.root, r.req.ProjectUnlockSource)
	if upToDateErr == nil && cloudManifestContentEqual(r.localManifest, upToDateSnapshot.Manifest) && func() bool {
		activeKeys, activeErr := syncKeychain(r.root, r.state)
		return activeErr == nil && remoteSnapshotFullyUnderKey(r.ctx, upToDateSnapshot, activeKeys.Active.KeyID)
	}() {
		projection := domain.NewProjection(r.command, "Remote is already up to date; nothing to push.")
		projection.Actions = []domain.Action{{Name: "diff", Command: fmt.Sprintf("pinax sync diff --target %s --vault %s --json", r.outputTarget, shellQuote(r.root))}}
		receiptOut, _, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, "success", nil, projection.Actions, r.pathPolicy, r.started)
		r.receipt = receiptOut
		if receiptErr != nil {
			return errorProjection(r.command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(r.root, r.state, r.receipt, upToDateSnapshot.RevisionID); err != nil {
			return errorProjection(r.command, err), err
		}
		emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "done", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "up_to_date", RevisionID: upToDateSnapshot.RevisionID})
		addCloudSyncFacts(&projection, r.state, r.plan)
		addCapsaBridgeFacts(&projection, r.req.Target)
		addCloudContentFacts(&projection, r.localManifest)
		projection.Facts["up_to_date"] = "true"
		projection.Facts["remote_checked"] = "true"
		projection.Facts["remote_write"] = "false"
		projection.Facts["run_id"] = r.receipt.RunID
		data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "up_to_date": true, "receipt": r.receipt}
		attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, upToDateSnapshot.Manifest, syncOutputViewOptions{Scope: "remote-aware", Result: "up_to_date", RemoteAfter: upToDateSnapshot.RevisionID, LocalAfter: upToDateSnapshot.RevisionID, PathPolicy: r.pathPolicy}))
		r.attachContentDiff(data, r.plan, upToDateSnapshot.Manifest)
		projection.Data = data
		return projection, nil
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "transfer", Direction: string(r.direction), RunID: r.receipt.RunID, Total: len(r.plan.Operations), Status: "running", RemoteWrite: true})
	uploadStats := &cloudUploadStats{}
	rebaseResult, execErr := runCloudPushRebase(cloudRebasePlan{
		commit: func(base string) (cloudsync.CommitResult, error) {
			return executeCloudPushWithCredential(r.ctx, r.root, r.state, r.localManifest, r.baseManifest, base, r.req.ProjectUnlockSource, uploadStats, r.ctrl)
		},
		pull: func() (cloudRemoteSnapshot, error) {
			return loadCloudRemoteSnapshotWithCredential(r.ctx, r.state, r.root, r.req.ProjectUnlockSource)
		},
		localManifest: r.localManifest,
		baseManifest:  r.baseManifest,
		baseRevision:  r.baseRevision,
		yes:           r.req.Yes,
	})
	if isSyncCancelledError(execErr) {
		// 项边界取消（§2.2）：manifest 提交未发生，远端只可能多出内容寻址
		// blob（下次 push 直接复用）；已上传 blob 计入 receipt 保留。
		r.plan.RemoteWrite = false
		r.receipt.Counts["upload_blobs"] += int(uploadStats.Blobs)
		r.receipt.Counts["bytes_uploaded"] += int(uploadStats.Bytes)
		return r.projectCancelled("Sync push cancelled at an item boundary; uploaded blobs are recorded and reusable.", map[string]string{
			"upload_blobs":   fmt.Sprint(r.receipt.Counts["upload_blobs"]),
			"bytes_uploaded": fmt.Sprint(r.receipt.Counts["bytes_uploaded"]),
		})
	}
	if len(rebaseResult.Conflicts) > 0 {
		// Auto-rebase pulled the remote head and found a content conflict that
		// cannot be auto-pushed. Surface a conflict_required projection.
		r.plan.RemoteWrite = false
		conflicts := cloudRebaseConflictEntries(rebaseResult.Conflicts)
		commandErr := &domain.CommandError{Code: "conflict_required", Message: "sync push hit a content conflict after auto-rebase", Hint: fmt.Sprintf("Resolve conflicts with pinax sync conflicts list --vault %s, then rerun pinax sync push --yes", shellQuote(r.root))}
		projection := domain.NewErrorProjection(r.command, commandErr)
		projection.Actions = syncConflictActions(r.root, conflicts)
		receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started)
		r.receipt = receiptOut
		r.receipt = receiptOut
		if receiptErr == nil {
			if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
				warnPersistFailure("sync state", err)
			}
			projection.Facts["run_id"] = r.receipt.RunID
			projection.Facts["conflicts"] = fmt.Sprint(len(conflicts))
			projection.Evidence = []string{receiptPath}
		}
		addCloudSyncFacts(&projection, r.state, r.plan)
		addCapsaBridgeFacts(&projection, r.req.Target)
		addCloudContentFacts(&projection, r.localManifest)
		data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "conflicts": conflicts, "receipt": r.receipt}
		attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: "remote-aware", Result: "failed", PathPolicy: r.pathPolicy}))
		r.attachContentDiff(data, r.plan, r.remoteSnapshot.Manifest)
		projection.Data = data
		return projection, commandErr
	}
	if execErr != nil {
		r.plan.RemoteWrite = false
		commandErr := commandErrorFromError(execErr)
		projection := domain.NewErrorProjection(r.command, commandErr)
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax %s doctor --vault %s --json", syncConfigCommand(r.req.Target), shellQuote(r.root))}}
		receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started)
		r.receipt = receiptOut
		r.receipt = receiptOut
		if receiptErr == nil {
			if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
				warnPersistFailure("sync state", err)
			}
			projection.Facts["run_id"] = r.receipt.RunID
			projection.Evidence = []string{receiptPath}
		}
		data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "receipt": r.receipt}
		attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: "failed", PathPolicy: r.pathPolicy}))
		r.attachContentDiff(data, r.plan, r.remoteSnapshot.Manifest)
		projection.Data = data
		return projection, commandErr
	}
	commit := rebaseResult.Commit
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "commit", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "running", RevisionID: commit.RevisionID, RemoteWrite: commit.RemoteWrite})
	r.plan.RemoteWrite = commit.RemoteWrite
	r.receipt.RemoteWrite = commit.RemoteWrite
	r.receipt.RevisionID = commit.RevisionID
	r.receipt.ManifestBlobID = commit.ManifestBlobID
	if r.localManifest.SchemaVersion == pinaxcloud.ManifestSchemaVersionV2 && commit.RemoteWrite {
		if err := recordFirstV2RemoteRevision(r.root, commit.RevisionID); err != nil {
			return errorProjection(r.command, err), err
		}
	}
	r.receipt.Counts["blobs"] = len(r.localManifest.Entries) + manifestTrashBackupCount(r.localManifest)
	r.receipt.Counts["upload_blobs"] += int(uploadStats.Blobs)
	r.receipt.Counts["bytes_uploaded"] += int(uploadStats.Bytes)
	r.receipt.Counts["delete_markers"] = len(r.localManifest.Deletes)
	r.receipt.Counts["trash_backup_blobs"] = manifestTrashBackupCount(r.localManifest)
	projection := domain.NewProjection(r.command, "Capsa sync push completed through configured backend.")
	projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", r.receipt.RunID, shellQuote(r.root))}}
	if err := writeCloudManifestCache(r.root, commit.RevisionID, r.localManifest); err != nil {
		return errorProjection(r.command, err), err
	}
	receiptOut, receiptPath, receiptErr := finishSyncRunWithFileEvents(r.root, r.receipt, r.plan, "success", nil, projection.Actions, r.pathPolicy, r.started, !r.ctrl.liveFileEvents)
	r.receipt = receiptOut
	if receiptErr != nil {
		return errorProjection(r.command, receiptErr), receiptErr
	}
	if err := writeCurrentSyncState(r.root, r.state, r.receipt, commit.RevisionID); err != nil {
		return errorProjection(r.command, err), err
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "done", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "success", RevisionID: commit.RevisionID, RemoteWrite: r.receipt.RemoteWrite})
	addCloudSyncFacts(&projection, r.state, r.plan)
	addCapsaBridgeFacts(&projection, r.req.Target)
	addCloudContentFacts(&projection, r.localManifest)
	setRemoteCheckedFacts(&projection, true)
	if commit.RemoteWrite {
		projection.Facts["durable_commit"] = "true"
	}
	projection.Facts["run_id"] = r.receipt.RunID
	projection.Facts["revision_id"] = commit.RevisionID
	projection.Facts["local_write"] = "false"
	projection.Evidence = []string{receiptPath}
	data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "remote_write": commit.RemoteWrite, "revision_id": commit.RevisionID, "manifest_blob_id": commit.ManifestBlobID, "receipt": r.receipt}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.localManifest, syncOutputViewOptions{Scope: "remote-aware", Result: "applied", RemoteAfter: commit.RevisionID, LocalAfter: commit.RevisionID, PathPolicy: r.pathPolicy}))
	r.attachContentDiff(data, r.plan, r.localManifest)
	projection.Data = data
	return projection, nil
}

func (r *cloudSyncRun) executeCloudPull() (domain.Projection, error) {
	if r.command == "sync.pull" && len(localUnpushedCloudOps(r.localDiffPlan)) > 0 {
		commandErr := &domain.CommandError{Code: "LOCAL_UNPUSHED_CHANGES", Message: "local changes have not been pushed", Hint: fmt.Sprintf("Run pinax sync --target %s --yes to merge and push local moves before pulling again", r.outputTarget)}
		projection := domain.NewErrorProjection(r.command, commandErr)
		projection.Actions = []domain.Action{{Name: "sync", Command: fmt.Sprintf("pinax sync --target %s --vault %s --yes", r.outputTarget, shellQuote(r.root))}, {Name: "diff", Command: fmt.Sprintf("pinax sync diff --target %s --vault %s --json", r.outputTarget, shellQuote(r.root))}}
		receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.localDiffPlan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started)
		r.receipt = receiptOut
		r.receipt = receiptOut
		if receiptErr == nil {
			if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
				warnPersistFailure("sync state", err)
			}
			projection.Facts["run_id"] = r.receipt.RunID
			projection.Evidence = []string{receiptPath}
		}
		addCloudSyncFacts(&projection, r.state, r.localDiffPlan)
		addCapsaBridgeFacts(&projection, r.req.Target)
		projection.Facts["local_unpushed_changes"] = fmt.Sprint(len(localUnpushedCloudOps(r.localDiffPlan)))
		data := map[string]any{"plan": syncops.SanitizePlan(r.localDiffPlan, r.pathPolicy), "receipt": r.receipt}
		attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.localDiffPlan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: "failed", PathPolicy: r.pathPolicy}))
		r.attachContentDiff(data, r.localDiffPlan, r.remoteSnapshot.Manifest)
		projection.Data = data
		return projection, commandErr
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "transfer", Direction: string(r.direction), RunID: r.receipt.RunID, Total: len(r.plan.Operations), Status: "running"})
	pullResult, execErr := executeCloudPull(r.ctx, r.root, r.state, r.plan, r.remoteSnapshot, r.ctrl)
	if isSyncCancelledError(execErr) {
		// 项边界取消（§2.2）：已落盘的文件/trash 删除保留，manifest 缓存
		// 不推进；receipt 记录已应用项计数。
		r.receipt.LocalWrite = pullResult.FilesApplied > 0 || pullResult.DeletesApplied > 0
		r.receipt.Counts["files_applied"] = pullResult.FilesApplied
		r.receipt.Counts["delete_markers_applied"] = pullResult.DeletesApplied
		return r.projectCancelled("Sync pull cancelled at an item boundary; applied files are preserved on disk.", map[string]string{
			"files_applied":          fmt.Sprint(pullResult.FilesApplied),
			"delete_markers_applied": fmt.Sprint(pullResult.DeletesApplied),
		})
	}
	if execErr != nil {
		commandErr := commandErrorFromError(execErr)
		projection := domain.NewErrorProjection(r.command, commandErr)
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax %s doctor --vault %s --json", syncConfigCommand(r.req.Target), shellQuote(r.root))}}
		receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, "failed", commandErr, projection.Actions, r.pathPolicy, r.started)
		r.receipt = receiptOut
		r.receipt = receiptOut
		if receiptErr == nil {
			if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
				warnPersistFailure("sync state", err)
			}
			projection.Facts["run_id"] = r.receipt.RunID
			projection.Evidence = []string{receiptPath}
		}
		data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "receipt": r.receipt}
		attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: "failed", PathPolicy: r.pathPolicy}))
		r.attachContentDiff(data, r.plan, r.remoteSnapshot.Manifest)
		projection.Data = data
		return projection, commandErr
	}
	r.receipt.LocalWrite = pullResult.FilesApplied > 0 || pullResult.DeletesApplied > 0
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "apply", Direction: string(r.direction), RunID: r.receipt.RunID, Completed: pullResult.FilesApplied + pullResult.DeletesApplied, Total: len(r.plan.Operations), Status: "running", LocalWrite: r.receipt.LocalWrite})
	r.receipt.RevisionID = pullResult.RevisionID
	r.receipt.ManifestBlobID = pullResult.ManifestBlobID
	r.receipt.Counts["files_applied"] = pullResult.FilesApplied
	r.receipt.Counts["delete_markers_applied"] = pullResult.DeletesApplied
	r.receipt.Counts["conflicts"] = len(pullResult.Conflicts)
	projection := domain.NewProjection(r.command, "Capsa sync pull completed through configured backend.")
	projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", r.receipt.RunID, shellQuote(r.root))}}
	if len(pullResult.Conflicts) > 0 {
		projection.Actions = append(projection.Actions, syncConflictActions(r.root, pullResult.Conflicts)...)
	}
	if err := writeCloudManifestCache(r.root, pullResult.RevisionID, pullResult.Manifest); err != nil {
		return errorProjection(r.command, err), err
	}
	receiptOut, receiptPath, receiptErr := finishSyncRunWithFileEvents(r.root, r.receipt, r.plan, "success", nil, projection.Actions, r.pathPolicy, r.started, !r.ctrl.liveFileEvents)
	r.receipt = receiptOut
	if receiptErr != nil {
		return errorProjection(r.command, receiptErr), receiptErr
	}
	if err := writeCurrentSyncState(r.root, r.state, r.receipt, pullResult.RevisionID); err != nil {
		return errorProjection(r.command, err), err
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "verify", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "success", RevisionID: pullResult.RevisionID, LocalWrite: r.receipt.LocalWrite})
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "done", Direction: string(r.direction), RunID: r.receipt.RunID, Status: "success", RevisionID: pullResult.RevisionID, LocalWrite: r.receipt.LocalWrite})
	addCloudSyncFacts(&projection, r.state, r.plan)
	addCapsaBridgeFacts(&projection, r.req.Target)
	setRemoteCheckedFacts(&projection, true)
	projection.Facts["run_id"] = r.receipt.RunID
	projection.Facts["files_applied"] = fmt.Sprint(pullResult.FilesApplied)
	projection.Facts["delete_markers_applied"] = fmt.Sprint(pullResult.DeletesApplied)
	projection.Facts["revision_id"] = pullResult.RevisionID
	projection.Facts["conflicts"] = fmt.Sprint(len(pullResult.Conflicts))
	projection.Facts["local_write"] = fmt.Sprint(r.receipt.LocalWrite)
	// A converged pull (nothing applied, no conflicts, manifest content equal)
	// reports up_to_date so the shared view matches the push fast path.
	upToDate := pullResult.FilesApplied == 0 && pullResult.DeletesApplied == 0 && len(pullResult.Conflicts) == 0 && cloudManifestContentEqual(r.localManifest, pullResult.Manifest)
	if upToDate {
		projection.Summary = "Remote is already up to date; nothing to pull."
		projection.Facts["up_to_date"] = "true"
	}
	addSyncConflictFacts(&projection, pullResult.Conflicts)
	projection.Evidence = []string{receiptPath}
	data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "remote_write": false, "files_applied": pullResult.FilesApplied, "delete_markers_applied": pullResult.DeletesApplied, "revision_id": pullResult.RevisionID, "manifest_blob_id": pullResult.ManifestBlobID, "conflicts": pullResult.Conflicts, "receipt": r.receipt}
	pullResultValue := "applied"
	if upToDate {
		pullResultValue = "up_to_date"
	}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, pullResult.Manifest, syncOutputViewOptions{Scope: "remote-aware", Result: pullResultValue, RemoteAfter: pullResult.RevisionID, LocalAfter: pullResult.RevisionID, PathPolicy: r.pathPolicy}))
	r.attachContentDiff(data, r.plan, pullResult.Manifest)
	projection.Data = data
	return projection, nil
}

func (r *cloudSyncRun) projectPlanOnly() (domain.Projection, error) {
	projection := domain.NewProjection(r.command, "Capsa sync plan generated; real remote writes are not wired yet.")
	status := "success"
	if r.plan.RequiresApproval {
		status = "approval_required"
		projection.Status = "failed"
	}
	if r.direction == syncplan.DirectionPush && r.req.Yes && !r.req.DryRun {
		r.plan.RemoteWrite = false
		status = "partial"
		projection.Status = "partial"
		projection.Facts["blocked_by"] = "cloud_api_unimplemented"
		projection.Actions = []domain.Action{{Name: "handoff", Command: fmt.Sprintf("pinax sync diff --target %s --vault %s --json", r.outputTarget, shellQuote(r.root))}}
	}
	if len(projection.Actions) == 0 {
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs list --vault %s --json", shellQuote(r.root))}}
	}
	var commandErr *domain.CommandError
	if status == "approval_required" {
		commandErr = &domain.CommandError{Code: "approval_required", Message: "sync requires approval", Hint: "Rerun with --yes or --dry-run"}
	}
	receiptOut, receiptPath, receiptErr := finishSyncRun(r.root, r.receipt, r.plan, status, commandErr, projection.Actions, r.pathPolicy, r.started)
	r.receipt = receiptOut
	if receiptErr != nil {
		return errorProjection(r.command, receiptErr), receiptErr
	}
	if err := writeCurrentSyncState(r.root, r.state, r.receipt, ""); err != nil {
		warnPersistFailure("sync state", err)
	}
	emitSyncEvent(r.req.LiveEvents, SyncEvent{Type: "progress", Phase: "done", Direction: string(r.direction), RunID: r.receipt.RunID, Status: status, RemoteWrite: r.receipt.RemoteWrite, LocalWrite: r.receipt.LocalWrite})
	addCloudSyncFacts(&projection, r.state, r.plan)
	addCapsaBridgeFacts(&projection, r.req.Target)
	addCloudContentFacts(&projection, r.localManifest)
	setRemoteCheckedFacts(&projection, r.remoteLoaded)
	projection.Facts["run_id"] = r.receipt.RunID
	projection.Evidence = []string{receiptPath}
	data := map[string]any{"plan": syncops.SanitizePlan(r.plan, r.pathPolicy), "blocked_by": projection.Facts["blocked_by"], "receipt": r.receipt}
	result := "planned"
	switch status {
	case "partial":
		result = "partial"
	case "failed":
		result = "failed"
	}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(r.plan, r.baseManifest, r.localManifest, r.remoteSnapshot.Manifest, syncOutputViewOptions{Scope: syncOutputScope(r.remoteLoaded), Result: result, PathPolicy: r.pathPolicy}))
	r.attachContentDiff(data, r.plan, r.remoteSnapshot.Manifest)
	projection.Data = data
	return projection, nil
}
