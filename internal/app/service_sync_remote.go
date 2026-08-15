package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

type SyncInitRequest struct {
	VaultPath   string
	Endpoint    string
	WorkspaceID string
	DeviceID    string
	SecretRef   string
}

type SyncStatusRequest struct {
	VaultPath string
}

func (s *Service) SyncInit(ctx context.Context, req SyncInitRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.init", err), err
	}
	if strings.TrimSpace(req.Endpoint) == "" {
		state, loadErr := remote.Load(root)
		if loadErr == nil {
			projection := domain.NewProjection("sync.init", "Existing Capsa sync configuration reused.")
			projection.Facts["target"] = syncTargetCapsa
			addCapsaBridgeFacts(&projection, syncTargetCapsa)
			projection.Facts["backend_kind"] = state.Config.BackendKind
			projection.Facts["endpoint"] = state.Config.Endpoint
			projection.Facts["workspace"] = state.Config.WorkspaceID
			projection.Facts["device"] = state.Config.DeviceID
			projection.Data = map[string]any{"backend_kind": state.Config.BackendKind, "endpoint": state.Config.Endpoint, "workspace": state.Config.WorkspaceID, "device": state.Config.DeviceID, "s3": state.Config.S3}
			return projection, nil
		}
		if remote.IsNotConfigured(loadErr) {
			err := &domain.CommandError{Code: "cloud_not_configured", Message: "Capsa sync is not configured", Hint: "Run pinax capsa backend set s3 or pinax capsa login first"}
			return domain.NewErrorProjection("sync.init", err), err
		}
		return errorProjection("sync.init", loadErr), loadErr
	}
	if _, err := remote.Login(root, remote.LoginRequest{
		Endpoint:    req.Endpoint,
		WorkspaceID: req.WorkspaceID,
		DeviceID:    req.DeviceID,
		SecretRef:   req.SecretRef,
	}); err != nil {
		return errorProjection("sync.init", err), err
	}
	projection := domain.NewProjection("sync.init", "Capsa sync configuration initialized.")
	projection.Facts["target"] = syncTargetCapsa
	addCapsaBridgeFacts(&projection, syncTargetCapsa)
	projection.Data = map[string]any{"target": syncTargetCapsa, "endpoint": req.Endpoint, "workspace": req.WorkspaceID, "device": req.DeviceID}
	return projection, nil
}

func (s *Service) SyncStatus(ctx context.Context, req SyncStatusRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.status", err), err
	}
	state, err := remote.Load(root)
	if err != nil {
		return cloudStateErrorProjection("sync.status", root, err)
	}
	current, _ := readCurrentSyncState(root)
	records, _ := readSyncRunReceipts(root)
	var latest *SyncRunReceipt
	if len(records) > 0 {
		latest = &records[0].Receipt
	}
	status := "configured"
	remoteRevision := ""
	if latest != nil && latest.Status == "failed" {
		status = "last_failed"
	}
	if latest != nil && latest.Counts["conflicts"] > 0 {
		status = "conflicted"
	}
	if isExecutableCloudState(state) {
		transport, transportErr := cloudTransportForState(ctx, state)
		if transportErr != nil {
			status = "transport_unavailable"
		} else {
			head, headErr := transport.CurrentHead(ctx, state.Config.WorkspaceID)
			if headErr != nil {
				status = "transport_unavailable"
			} else {
				remoteRevision = strings.TrimSpace(head.CurrentRevision)
				if current.LastSyncedRevision != "" && remoteRevision != "" && current.LastSyncedRevision != remoteRevision && status == "configured" {
					status = "stale"
				}
			}
		}
	}
	projection := domain.NewProjection("sync.status", "Capsa sync status read.")
	projection.Facts["target"] = syncTargetCapsa
	addCapsaBridgeFacts(&projection, syncTargetCapsa)
	projection.Facts["configured"] = "true"
	projection.Facts["sync_status"] = status
	projection.Facts["backend_kind"] = directBackendKind(state)
	projection.Facts["workspace_id"] = state.Config.WorkspaceID
	projection.Facts["device_id"] = state.Config.DeviceID
	projection.Facts["last_synced_revision"] = current.LastSyncedRevision
	projection.Facts["last_status"] = current.LastStatus
	projection.Facts["last_direction"] = current.LastDirection
	projection.Facts["last_sync_run_id"] = current.LastSyncRunID
	if remoteRevision != "" {
		projection.Facts["remote_revision"] = remoteRevision
	}
	if latest != nil {
		projection.Facts["run_id"] = latest.RunID
		projection.Actions = append(projection.Actions, domain.Action{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", latest.RunID, shellQuote(root))})
	}
	switch status {
	case "conflicted":
		projection.Actions = append(projection.Actions, domain.Action{Name: "conflicts", Command: fmt.Sprintf("pinax sync conflicts list --vault %s --json", shellQuote(root))})
	case "stale":
		projection.Actions = append(projection.Actions, domain.Action{Name: "diff", Command: fmt.Sprintf("pinax sync diff --target capsa --vault %s --json", shellQuote(root))})
	case "transport_unavailable":
		projection.Actions = append(projection.Actions, domain.Action{Name: "doctor", Command: fmt.Sprintf("pinax capsa doctor --vault %s --json", shellQuote(root))})
	}
	projection.Data = map[string]any{"state": current, "latest_run": latest, "remote_revision": remoteRevision, "status": status}
	return projection, nil
}

func (s *Service) SyncAll(ctx context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.all", err), err
	}
	if !isCapsaSyncTarget(target) {
		return errorProjection("sync.all", fmt.Errorf("sync all currently only supports target=capsa")), nil
	}

	// Pull
	pullReq := req
	pullReq.Target = target
	pullReq.Yes = req.Yes
	pullReq.DryRun = req.DryRun
	pullProj, err := buildCloudSyncProjection(ctx, "sync.all.pull", root, pullReq, syncplan.DirectionPull)
	if err != nil {
		return pullProj, err
	}

	// Push
	pushReq := req
	pushReq.Target = target
	pushReq.Yes = req.Yes
	pushReq.DryRun = req.DryRun
	pushProj, err := buildCloudSyncProjection(ctx, "sync.all.push", root, pushReq, syncplan.DirectionPush)
	if err != nil {
		return pushProj, err
	}

	projection := domain.NewProjection("sync.all", "Bidirectional sync completed.")
	projection.Status = combinedSyncStatus(pullProj.Status, pushProj.Status)
	pullView, pullViewOK := syncViewFromProjection(pullProj)
	pushView, pushViewOK := syncViewFromProjection(pushProj)
	aggregateView := aggregateSyncOutputViews(pullView, pullViewOK, pushView, pushViewOK, projection.Status)
	pullReceipt, _ := syncReceiptFromProjection(pullProj)
	pushReceipt, _ := syncReceiptFromProjection(pushProj)
	state, stateErr := cloudStateForSync(root, req)
	if stateErr == nil {
		receipt := syncRunStart("sync.all", syncplan.Direction("all"), state, req.PathPolicy, target)
		receipt.Status = combinedSyncStatus(pullProj.Status, pushProj.Status)
		receipt.RemoteWrite = pushProj.Facts["remote_write"] == "true"
		receipt.LocalWrite = pullProj.Facts["files_applied"] != "" && pullProj.Facts["files_applied"] != "0"
		if rev := pushProj.Facts["revision_id"]; rev != "" {
			receipt.RevisionID = rev
		} else if rev := pullProj.Facts["revision_id"]; rev != "" {
			receipt.RevisionID = rev
		}
		receipt.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		aggregatePlan := syncplan.Plan{Direction: syncplan.Direction("all"), Target: syncOutputTarget(target), RemoteWrite: receipt.RemoteWrite}
		if pullReceipt != nil {
			aggregatePlan.Operations = append(aggregatePlan.Operations, pullReceipt.Operations...)
		}
		if pushReceipt != nil {
			aggregatePlan.Operations = append(aggregatePlan.Operations, pushReceipt.Operations...)
		}
		receipt, receiptPath, receiptErr := finishSyncRun(root, receipt, aggregatePlan, receipt.Status, nil, receipt.Actions, req.PathPolicy, time.Now())
		if receiptErr == nil {
			if err := writeCurrentSyncState(root, state, receipt, receipt.RevisionID); err != nil {
				warnPersistFailure("sync state", err)
			}
			projection.Facts["run_id"] = receipt.RunID
			projection.Facts["remote_write"] = fmt.Sprint(receipt.RemoteWrite)
			projection.Evidence = []string{receiptPath}
		}
	}
	projection.Facts["target"] = syncOutputTarget(target)
	addCapsaBridgeFacts(&projection, target)
	data := map[string]any{"pull": pullProj.Data, "push": pushProj.Data}
	attachSyncOutputView(projection.Facts, data, aggregateView)
	projection.Data = data
	return projection, nil
}

func syncViewFromProjection(projection domain.Projection) (syncOutputView, bool) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return syncOutputView{}, false
	}
	view, ok := data["sync_view"].(syncOutputView)
	return view, ok
}

func syncReceiptFromProjection(projection domain.Projection) (*SyncRunReceipt, bool) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return nil, false
	}
	switch receipt := data["receipt"].(type) {
	case SyncRunReceipt:
		copy := receipt
		return &copy, true
	case *SyncRunReceipt:
		return receipt, receipt != nil
	default:
		return nil, false
	}
}

func aggregateSyncOutputViews(pull syncOutputView, pullOK bool, push syncOutputView, pushOK bool, result string) syncOutputView {
	view := syncOutputView{SchemaVersion: syncOutputViewSchemaVersion, Direction: "all", Scope: "cached", Result: "planned"}
	if pullOK {
		view.Scope = pull.Scope
		view.Revisions = pull.Revisions
		view.Counts = pull.Counts
		view.Bytes = pull.Bytes
		view.Changes = append(view.Changes, pull.Changes...)
	}
	if pushOK {
		if push.Scope == "remote-aware" {
			view.Scope = push.Scope
		}
		if view.Revisions.Base == "" {
			view.Revisions.Base = push.Revisions.Base
		}
		if push.Revisions.RemoteBefore != "" {
			view.Revisions.RemoteBefore = push.Revisions.RemoteBefore
		}
		if push.Revisions.RemoteAfter != "" {
			view.Revisions.RemoteAfter = push.Revisions.RemoteAfter
		}
		if push.Revisions.LocalAfter != "" {
			view.Revisions.LocalAfter = push.Revisions.LocalAfter
		}
		view.Counts.Added += push.Counts.Added
		view.Counts.Modified += push.Counts.Modified
		view.Counts.Deleted += push.Counts.Deleted
		view.Counts.Renamed += push.Counts.Renamed
		view.Counts.Conflicts += push.Counts.Conflicts
		view.Counts.Unchanged += push.Counts.Unchanged
		view.Bytes.Uploaded += push.Bytes.Uploaded
		view.Bytes.Downloaded += push.Bytes.Downloaded
		view.Changes = append(view.Changes, push.Changes...)
	}
	view.Counts.Total = view.Counts.Added + view.Counts.Modified + view.Counts.Deleted + view.Counts.Renamed + view.Counts.Conflicts + view.Counts.Unchanged
	view.Total = len(view.Changes)
	view.Shown = view.Total
	view.Result = syncOutputResultFromStatus(result)
	sort.SliceStable(view.Changes, func(i, j int) bool {
		left := syncOutputFirstNonEmpty(view.Changes[i].Path, view.Changes[i].ToPath, view.Changes[i].FromPath)
		right := syncOutputFirstNonEmpty(view.Changes[j].Path, view.Changes[j].ToPath, view.Changes[j].FromPath)
		return left < right
	})
	return view
}

func syncOutputResultFromStatus(status string) string {
	switch status {
	case "failed":
		return "failed"
	case "partial":
		return "partial"
	case "success":
		return "applied"
	default:
		return "planned"
	}
}

func combinedSyncStatus(statuses ...string) string {
	combined := "success"
	for _, status := range statuses {
		switch status {
		case "failed":
			return "failed"
		case "partial":
			combined = "partial"
		}
	}
	return combined
}
