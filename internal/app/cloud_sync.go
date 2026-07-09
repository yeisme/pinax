package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/cloudclient"
	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/domain"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

func cloudStateForSync(root string, req SyncRequest) (pinaxcloud.State, error) {
	if strings.TrimSpace(req.Endpoint) == "" {
		return pinaxcloud.Load(root)
	}
	return pinaxcloud.State{
		Config: pinaxcloud.Config{
			SchemaVersion: pinaxcloud.ConfigSchemaVersion,
			Endpoint:      strings.TrimRight(strings.TrimSpace(req.Endpoint), "/"),
			WorkspaceID:   strings.TrimSpace(req.WorkspaceID),
			DeviceID:      strings.TrimSpace(req.DeviceID),
			SecretRef:     strings.TrimSpace(req.SecretRef),
		},
		Session: pinaxcloud.DeviceSession{
			SchemaVersion: pinaxcloud.SessionSchemaVersion,
			DeviceID:      strings.TrimSpace(req.DeviceID),
			Status:        "profile",
		},
	}, nil
}

func buildCloudSyncProjection(ctx context.Context, command, root string, req SyncRequest, direction syncplan.Direction) (domain.Projection, error) {
	started := time.Now()
	outputTarget := syncOutputTarget(req.Target)
	pathPolicy := syncops.NormalizePathPolicy(req.PathPolicy)
	state, err := cloudStateForSync(root, req)
	if err != nil {
		return cloudStateErrorProjection(command, root, err)
	}
	receipt := syncRunStart(command, direction, state, pathPolicy, outputTarget)
	localManifest, err := pinaxcloud.BuildManifest(root)
	if err != nil {
		projection := errorProjection(command, err)
		return projection, err
	}
	baseManifest, cachedRevision, cacheErr := readCachedCloudManifest(root, state)
	if cacheErr != nil {
		projection := errorProjection(command, cacheErr)
		return projection, cacheErr
	}
	baseRevision := strings.TrimSpace(req.BaseRevision)
	if baseRevision == "" {
		baseRevision = cachedRevision
	}
	if cachedRevision != baseRevision {
		baseManifest = pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}
	}
	remoteSnapshot := cloudRemoteSnapshot{}
	if isExecutableCloudState(state) && direction == syncplan.DirectionPull && req.Yes && !req.DryRun {
		snapshot, snapshotErr := loadCloudRemoteSnapshot(ctx, state)
		if snapshotErr != nil {
			if direction == syncplan.DirectionPull && req.Yes && !req.DryRun {
				projection := errorProjection(command, snapshotErr)
				return projection, snapshotErr
			}
		} else {
			remoteSnapshot = snapshot
		}
	}
	remoteRevision := strings.TrimSpace(req.RemoteRevision)
	if remoteRevision == "" {
		remoteRevision = strings.TrimSpace(remoteSnapshot.RevisionID)
	}
	if remoteRevision == "" {
		remoteRevision = baseRevision
	}
	plan, planErr := syncplan.BuildPlan(syncplan.Request{Direction: direction, Target: outputTarget, LocalManifest: localManifest, BaseManifest: baseManifest, RemoteManifest: remoteSnapshot.Manifest, BaseRevision: baseRevision, RemoteRevision: remoteRevision, DryRun: req.DryRun, Yes: req.Yes})
	localDiffPlan, localDiffErr := syncplan.BuildPlan(syncplan.Request{Direction: syncplan.DirectionDiff, Target: outputTarget, LocalManifest: localManifest, BaseManifest: baseManifest, RemoteManifest: remoteSnapshot.Manifest, BaseRevision: baseRevision, RemoteRevision: remoteRevision, DryRun: true, Yes: true})
	if localDiffErr != nil && planErr == nil {
		planErr = localDiffErr
	}
	if errors.Is(planErr, syncplan.ErrRevisionConflict) {
		commandErr := &domain.CommandError{Code: "REVISION_CONFLICT", Message: "cloud revision conflict", Hint: "Review the conflict queue and resolve manually, then retry sync"}
		projection := domain.NewErrorProjection(command, commandErr)
		projection.Actions = append(syncConflictActions(root, nil), domain.Action{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))})
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
		if receiptErr == nil {
			_ = writeCurrentSyncState(root, state, receipt, "")
			projection.Facts["run_id"] = receipt.RunID
			projection.Evidence = []string{receiptPath}
		}
		addCloudSyncFacts(&projection, state, plan)
		addCapsaBridgeFacts(&projection, req.Target)
		addCloudContentFacts(&projection, localManifest)
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
		return projection, commandErr
	}
	if planErr != nil {
		projection := errorProjection(command, planErr)
		return projection, planErr
	}
	if direction == syncplan.DirectionPush && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		rebaseResult, execErr := runCloudPushRebase(cloudRebasePlan{
			commit: func(base string) (cloudsync.CommitResult, error) {
				return executeCloudPush(ctx, root, state, localManifest, base)
			},
			pull:          func() (cloudRemoteSnapshot, error) { return loadCloudRemoteSnapshot(ctx, state) },
			localManifest: localManifest,
			baseManifest:  baseManifest,
			baseRevision:  req.BaseRevision,
			yes:           req.Yes,
		})
		if len(rebaseResult.Conflicts) > 0 {
			// Auto-rebase pulled the remote head and found a content conflict that
			// cannot be auto-pushed. Surface a conflict_required projection.
			plan.RemoteWrite = false
			conflicts := cloudRebaseConflictEntries(rebaseResult.Conflicts)
			commandErr := &domain.CommandError{Code: "conflict_required", Message: "sync push hit a content conflict after auto-rebase", Hint: fmt.Sprintf("Resolve conflicts with pinax sync conflicts list --vault %s, then rerun pinax sync push --yes", shellQuote(root))}
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = syncConflictActions(root, conflicts)
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Facts["conflicts"] = fmt.Sprint(len(conflicts))
				projection.Evidence = []string{receiptPath}
			}
			addCloudSyncFacts(&projection, state, plan)
			addCapsaBridgeFacts(&projection, req.Target)
			addCloudContentFacts(&projection, localManifest)
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "conflicts": conflicts, "receipt": receipt}
			return projection, commandErr
		}
		if execErr != nil {
			plan.RemoteWrite = false
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax %s doctor --vault %s --json", syncConfigCommand(req.Target), shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		commit := rebaseResult.Commit
		plan.RemoteWrite = commit.RemoteWrite
		receipt.RemoteWrite = commit.RemoteWrite
		receipt.RevisionID = commit.RevisionID
		receipt.ManifestBlobID = commit.ManifestBlobID
		receipt.Counts["blobs"] = len(localManifest.Entries) + manifestTrashBackupCount(localManifest)
		receipt.Counts["delete_markers"] = len(localManifest.Deletes)
		receipt.Counts["trash_backup_blobs"] = manifestTrashBackupCount(localManifest)
		projection := domain.NewProjection(command, "Capsa sync push completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		if _, err := writeCloudManifestCache(root, commit.RevisionID, localManifest); err != nil {
			return errorProjection(command, err), err
		}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, commit.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
		addCapsaBridgeFacts(&projection, req.Target)
		addCloudContentFacts(&projection, localManifest)
		projection.Facts["run_id"] = receipt.RunID
		projection.Facts["revision_id"] = commit.RevisionID
		projection.Evidence = []string{receiptPath}
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "remote_write": commit.RemoteWrite, "revision_id": commit.RevisionID, "manifest_blob_id": commit.ManifestBlobID, "receipt": receipt}
		return projection, nil
	}
	if direction == syncplan.DirectionPull && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		if command == "sync.pull" && len(localUnpushedCloudOps(localDiffPlan)) > 0 {
			commandErr := &domain.CommandError{Code: "LOCAL_UNPUSHED_CHANGES", Message: "local changes have not been pushed", Hint: fmt.Sprintf("Run pinax sync --target %s --yes to merge and push local moves before pulling again", outputTarget)}
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "sync", Command: fmt.Sprintf("pinax sync --target %s --vault %s --yes", outputTarget, shellQuote(root))}, {Name: "diff", Command: fmt.Sprintf("pinax sync diff --target %s --vault %s --json", outputTarget, shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, localDiffPlan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			addCloudSyncFacts(&projection, state, localDiffPlan)
			addCapsaBridgeFacts(&projection, req.Target)
			projection.Facts["local_unpushed_changes"] = fmt.Sprint(len(localUnpushedCloudOps(localDiffPlan)))
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(localDiffPlan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		pullResult, execErr := executeCloudPull(ctx, root, state, plan, remoteSnapshot)
		if execErr != nil {
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax %s doctor --vault %s --json", syncConfigCommand(req.Target), shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		receipt.LocalWrite = pullResult.FilesApplied > 0 || pullResult.DeletesApplied > 0
		receipt.RevisionID = pullResult.RevisionID
		receipt.ManifestBlobID = pullResult.ManifestBlobID
		receipt.Counts["files_applied"] = pullResult.FilesApplied
		receipt.Counts["delete_markers_applied"] = pullResult.DeletesApplied
		receipt.Counts["conflicts"] = len(pullResult.Conflicts)
		projection := domain.NewProjection(command, "Capsa sync pull completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		if len(pullResult.Conflicts) > 0 {
			projection.Actions = append(projection.Actions, syncConflictActions(root, pullResult.Conflicts)...)
		}
		if _, err := writeCloudManifestCache(root, pullResult.RevisionID, pullResult.Manifest); err != nil {
			return errorProjection(command, err), err
		}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, pullResult.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
		addCapsaBridgeFacts(&projection, req.Target)
		projection.Facts["run_id"] = receipt.RunID
		projection.Facts["files_applied"] = fmt.Sprint(pullResult.FilesApplied)
		projection.Facts["delete_markers_applied"] = fmt.Sprint(pullResult.DeletesApplied)
		projection.Facts["revision_id"] = pullResult.RevisionID
		projection.Facts["conflicts"] = fmt.Sprint(len(pullResult.Conflicts))
		addSyncConflictFacts(&projection, pullResult.Conflicts)
		projection.Evidence = []string{receiptPath}
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "remote_write": false, "files_applied": pullResult.FilesApplied, "delete_markers_applied": pullResult.DeletesApplied, "revision_id": pullResult.RevisionID, "manifest_blob_id": pullResult.ManifestBlobID, "conflicts": pullResult.Conflicts, "receipt": receipt}
		return projection, nil
	}
	projection := domain.NewProjection(command, "Capsa sync plan generated; real remote writes are not wired yet.")
	status := "success"
	if plan.RequiresApproval {
		status = "approval_required"
		projection.Status = "failed"
	}
	if direction == syncplan.DirectionPush && req.Yes && !req.DryRun {
		plan.RemoteWrite = false
		status = "partial"
		projection.Status = "partial"
		projection.Facts["blocked_by"] = "cloud_api_unimplemented"
		projection.Actions = []domain.Action{{Name: "handoff", Command: fmt.Sprintf("pinax sync diff --target %s --vault %s --json", outputTarget, shellQuote(root))}}
	}
	if len(projection.Actions) == 0 {
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs list --vault %s --json", shellQuote(root))}}
	}
	var commandErr *domain.CommandError
	if status == "approval_required" {
		commandErr = &domain.CommandError{Code: "approval_required", Message: "sync requires approval", Hint: "Rerun with --yes or --dry-run"}
	}
	receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, status, commandErr, projection.Actions, pathPolicy, started)
	if receiptErr != nil {
		return errorProjection(command, receiptErr), receiptErr
	}
	_ = writeCurrentSyncState(root, state, receipt, "")
	addCloudSyncFacts(&projection, state, plan)
	addCapsaBridgeFacts(&projection, req.Target)
	addCloudContentFacts(&projection, localManifest)
	projection.Facts["run_id"] = receipt.RunID
	projection.Evidence = []string{receiptPath}
	projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "blocked_by": projection.Facts["blocked_by"], "receipt": receipt}
	return projection, nil
}

func commandErrorFromError(err error) *domain.CommandError {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr
	}
	rawMessage := err.Error()
	message := syncops.SanitizeString(rawMessage)
	switch {
	case strings.Contains(rawMessage, "lock_held"):
		return &domain.CommandError{Code: "lock_held", Message: message, Hint: "Retry after the current Capsa sync finishes"}
	case strings.Contains(rawMessage, "transport_unavailable"), isRcloneCommandFailure(rawMessage):
		return &domain.CommandError{Code: "transport_unavailable", Message: message, Hint: "Check the configured Capsa transport before retrying"}
	case isCloudRevisionConflict(err):
		return &domain.CommandError{Code: "REVISION_CONFLICT", Message: message, Hint: "Another device pushed a newer revision; rerun pinax sync push --yes to auto-rebase, or pull first"}
	default:
		return &domain.CommandError{Code: "cloud_sync_failed", Message: message, Hint: "Run pinax capsa doctor --vault <vault> --json"}
	}
}

func isRcloneCommandFailure(message string) bool {
	return strings.Contains(message, "rclone ") && strings.Contains(message, " failed")
}

func directBackendKind(state pinaxcloud.State) string {
	if strings.TrimSpace(state.Config.BackendKind) != "" {
		return state.Config.BackendKind
	}
	if strings.HasPrefix(state.Config.Endpoint, "file://") {
		return "embedded"
	}
	if strings.HasPrefix(state.Config.Endpoint, "s3://") {
		return "s3-direct"
	}
	if strings.HasPrefix(state.Config.Endpoint, "rclone://") {
		return "rclone-direct"
	}
	return "direct"
}
func isExecutableCloudState(state pinaxcloud.State) bool {
	endpoint := strings.TrimSpace(state.Config.Endpoint)
	return strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") || strings.HasPrefix(endpoint, "file://") || strings.HasPrefix(endpoint, "s3://") || strings.HasPrefix(endpoint, "rclone://") || state.Config.BackendKind == "server" || state.Config.BackendKind == "s3-direct" || state.Config.BackendKind == "rclone-direct"
}

func cloudTransportForState(ctx context.Context, state pinaxcloud.State) (cloudsync.Transport, error) {
	endpoint := strings.TrimSpace(state.Config.Endpoint)
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") || state.Config.BackendKind == "server" {
		token, err := pinaxprofile.ResolveSecretRef(state.Config.SecretRef)
		if err != nil {
			return nil, &domain.CommandError{Code: "cloud_secret_unavailable", Message: "cloud credential is unavailable", Hint: "Check the configured cloud secret reference before retrying"}
		}
		client, err := cloudclient.New(cloudclient.Config{Endpoint: endpoint, VaultID: state.Config.WorkspaceID, DeviceID: state.Config.DeviceID, Token: token})
		if err != nil {
			return nil, err
		}
		return cloudclient.NewTransport(client), nil
	}
	store, err := state.GetStore(ctx)
	if err != nil {
		return nil, err
	}
	return cloudsync.NewObjectStoreTransport(store, cloudsync.Layout{WorkspaceID: state.Config.WorkspaceID, VaultID: state.Config.WorkspaceID}), nil
}

type directPullResult struct {
	FilesApplied   int
	DeletesApplied int
	RevisionID     string
	ManifestBlobID string
	Manifest       pinaxcloud.Manifest
	Conflicts      []domain.SyncConflictEntry
}

type cloudRemoteSnapshot struct {
	Transport      cloudsync.Transport
	Key            pinaxcloud.CryptoKey
	Manifest       pinaxcloud.Manifest
	RevisionID     string
	ManifestBlobID string
}

func loadCloudRemoteSnapshot(ctx context.Context, state pinaxcloud.State) (cloudRemoteSnapshot, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	if strings.TrimSpace(head.CurrentRevision) == "" || strings.TrimSpace(head.ManifestBlobID) == "" {
		return cloudRemoteSnapshot{}, nil
	}
	key, err := pinaxcloud.DeriveKey(pinaxcloud.EncryptionSecretRef(state.Config))
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	manifestEnvelope, err := transport.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	manifest, err := pinaxcloud.DecryptManifest(key, remoteEnvelope(manifestEnvelope))
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	return cloudRemoteSnapshot{Transport: transport, Key: key, Manifest: manifest, RevisionID: head.CurrentRevision, ManifestBlobID: head.ManifestBlobID}, nil
}

func executeCloudPull(ctx context.Context, root string, state pinaxcloud.State, plan syncplan.Plan, snapshot cloudRemoteSnapshot) (directPullResult, error) {
	if strings.TrimSpace(snapshot.RevisionID) == "" || strings.TrimSpace(snapshot.ManifestBlobID) == "" {
		return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "Capsa backend has no committed revision", Hint: "Run pinax sync push --target capsa --yes from a device with notes first"}
	}
	transport := snapshot.Transport
	key := snapshot.Key
	manifest := snapshot.Manifest
	if transport == nil {
		loaded, err := loadCloudRemoteSnapshot(ctx, state)
		if err != nil {
			return directPullResult{}, err
		}
		if strings.TrimSpace(loaded.RevisionID) == "" || strings.TrimSpace(loaded.ManifestBlobID) == "" {
			return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "Capsa backend has no committed revision", Hint: "Run pinax sync push --target capsa --yes from a device with notes first"}
		}
		transport = loaded.Transport
		key = loaded.Key
		manifest = loaded.Manifest
		snapshot = loaded
	}
	filesApplied := 0
	deletesApplied := 0
	conflicts := []domain.SyncConflictEntry{}
	for _, deleteMarker := range manifest.Deletes {
		result, err := applyRemoteTrashDelete(root, remoteTrashDeleteMarker{ObjectKind: deleteMarker.ObjectKind, ObjectID: deleteMarker.ObjectID, TombstoneID: deleteMarker.TombstoneID, DeletedAt: deleteMarker.DeletedAt})
		if err != nil {
			return directPullResult{}, err
		}
		if result.Applied {
			deletesApplied++
		}
		if result.Conflict != nil {
			conflicts = append(conflicts, *result.Conflict)
		}
	}
	entries := manifestEntriesByPath(manifest)
	for _, op := range plan.Operations {
		switch op.Kind {
		case "download_blob":
			entry, ok := entries[op.Path]
			if !ok {
				continue
			}
			applied, newConflicts, err := applyRemoteManifestEntry(ctx, root, transport, key, entry)
			if err != nil {
				return directPullResult{}, err
			}
			if applied {
				filesApplied++
			}
			conflicts = append(conflicts, newConflicts...)
		case "delete_local":
			applied, err := deleteLocalManifestPath(root, op.Path)
			if err != nil {
				return directPullResult{}, err
			}
			if applied {
				filesApplied++
			}
		case "conflict":
			entry, ok := entries[op.Path]
			if ok {
				applied, newConflicts, err := applyRemoteManifestEntry(ctx, root, transport, key, entry)
				if err != nil {
					return directPullResult{}, err
				}
				if applied {
					filesApplied++
				}
				conflicts = append(conflicts, newConflicts...)
				continue
			}
			conflict, err := preserveLocalConflict(root, op.Path, time.Now().UTC())
			if err != nil {
				return directPullResult{}, err
			}
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			}
		}
	}
	result := directPullResult{FilesApplied: filesApplied, DeletesApplied: deletesApplied, RevisionID: snapshot.RevisionID, ManifestBlobID: snapshot.ManifestBlobID, Manifest: manifest, Conflicts: conflicts}

	return result, nil
}

func manifestEntriesByPath(manifest pinaxcloud.Manifest) map[string]pinaxcloud.ManifestEntry {
	entries := make(map[string]pinaxcloud.ManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		entries[entry.Path] = entry
	}
	return entries
}

func applyRemoteManifestEntry(ctx context.Context, root string, transport cloudsync.Transport, key pinaxcloud.CryptoKey, entry pinaxcloud.ManifestEntry) (bool, []domain.SyncConflictEntry, error) {
	blobEnvelope, err := transport.GetBlob(ctx, entry.BlobID)
	if err != nil {
		return false, nil, err
	}
	content, err := pinaxcloud.DecryptBlob(key, remoteEnvelope(blobEnvelope), []byte(entry.BlobID))
	if err != nil {
		return false, nil, err
	}
	path, err := safeCloudSyncPath(root, entry.Path)
	if err != nil {
		return false, nil, err
	}
	fileMode := os.FileMode(entry.Mode & 0o777)
	if fileMode == 0 {
		fileMode = 0o600
	}
	conflicts := []domain.SyncConflictEntry{}
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() != fileMode {
				if err := os.Chmod(path, fileMode); err != nil {
					return false, nil, err
				}
				return true, nil, nil
			}
			return false, nil, nil
		}
		conflict, err := writeConflictCopy(root, path, existing, fileMode, time.Now().UTC())
		if err != nil {
			return false, nil, err
		}
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, nil, err
	}
	if err := os.WriteFile(path, content, fileMode); err != nil {
		return false, nil, err
	}
	return true, conflicts, nil
}

func deleteLocalManifestPath(root, rel string) (bool, error) {
	path, err := safeCloudSyncPath(root, rel)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func preserveLocalConflict(root, rel string, now time.Time) (*domain.SyncConflictEntry, error) {
	path, err := safeCloudSyncPath(root, rel)
	if err != nil {
		return nil, err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	info, statErr := os.Stat(path)
	mode := os.FileMode(0o600)
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	return writeConflictCopy(root, path, existing, mode, now)
}

func writeConflictCopy(root, path string, content []byte, mode os.FileMode, now time.Time) (*domain.SyncConflictEntry, error) {
	conflictPath := syncConflictCopyPath(path, now)
	if err := os.WriteFile(conflictPath, content, mode); err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, conflictPath)
	if err != nil {
		return nil, err
	}
	conflictRel := filepath.ToSlash(rel)
	mainRel, err := mainPathForSyncConflict(conflictRel)
	if err != nil {
		return nil, err
	}
	return &domain.SyncConflictEntry{File: conflictRel, MainPath: mainRel}, nil
}

func localUnpushedCloudOps(plan syncplan.Plan) []syncplan.Operation {
	ops := make([]syncplan.Operation, 0)
	for _, op := range plan.Operations {
		switch op.Kind {
		case "delete_remote":
			ops = append(ops, op)
		}
	}
	return ops
}

func syncConflictCopyPath(path string, now time.Time) string {
	base := path
	if filepath.Ext(path) == ".md" {
		base = strings.TrimSuffix(path, ".md")
	}
	return base + "." + now.Format("20060102150405") + ".conflict.md"
}

func safeCloudSyncPath(root, rel string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(rel) || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".pinax/") || strings.HasPrefix(clean, ".git/") {
		return "", &domain.CommandError{Code: "unsafe_cloud_path", Message: "cloud manifest path is outside the vault", Hint: "Inspect the remote manifest and retry after removing unsafe entries"}
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func localCloudBaseRevision(root string, cloudState pinaxcloud.State) string {
	state, err := readCurrentSyncState(root)
	if err != nil || !isCapsaSyncTarget(state.Target) {
		return ""
	}
	if state.BackendKind != directBackendKind(cloudState) || state.WorkspaceID != cloudState.Config.WorkspaceID || state.Endpoint != cloudState.Config.Endpoint {
		return ""
	}
	return strings.TrimSpace(state.LastSyncedRevision)
}

func readCachedCloudManifest(root string, cloudState pinaxcloud.State) (pinaxcloud.Manifest, string, error) {
	state, err := readCurrentSyncState(root)
	if err != nil {
		if os.IsNotExist(err) {
			return pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}, "", nil
		}
		return pinaxcloud.Manifest{}, "", err
	}
	if !isCapsaSyncTarget(state.Target) || state.BackendKind != directBackendKind(cloudState) || state.WorkspaceID != cloudState.Config.WorkspaceID || state.Endpoint != cloudState.Config.Endpoint {
		return pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}, "", nil
	}
	revision := strings.TrimSpace(state.LastSyncedRevision)
	if revision == "" {
		return pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}, "", nil
	}
	cacheRel := strings.TrimSpace(state.LastManifestCache)
	if cacheRel == "" {
		cacheRel = syncManifestCacheRel(revision)
	}
	cachePath, err := safeSyncManifestCachePath(root, cacheRel)
	if err != nil {
		return pinaxcloud.Manifest{}, "", err
	}
	b, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}, revision, nil
		}
		return pinaxcloud.Manifest{}, "", err
	}
	var manifest pinaxcloud.Manifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return pinaxcloud.Manifest{}, "", err
	}
	return manifest, revision, nil
}

func writeCloudManifestCache(root, revision string, manifest pinaxcloud.Manifest) (string, error) {
	if strings.TrimSpace(revision) == "" {
		return "", nil
	}
	rel := syncManifestCacheRel(revision)
	return rel, writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), manifest)
}

func syncManifestCacheRel(revision string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "cloud", "manifest-cache", syncManifestCacheName(revision)+".json"))
}

func syncManifestCacheName(revision string) string {
	trimmed := strings.TrimSpace(revision)
	if trimmed == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	name := strings.Trim(b.String(), ".")
	if name == "" || strings.Contains(name, "..") {
		return "revision"
	}
	return name
}

func safeSyncManifestCachePath(root, rel string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if clean == "" || clean == "." || filepath.IsAbs(rel) || strings.HasPrefix(clean, "../") || !strings.HasPrefix(clean, ".pinax/cloud/manifest-cache/") || filepath.Ext(clean) != ".json" {
		return "", &domain.CommandError{Code: "sync_manifest_cache_invalid", Message: "local sync manifest cache path is invalid", Hint: "Run pinax sync diff --target capsa --json to rebuild sync state"}
	}
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(clean)))
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", &domain.CommandError{Code: "sync_manifest_cache_invalid", Message: "local sync manifest cache path is invalid", Hint: "Run pinax sync diff --target capsa --json to rebuild sync state"}
	}
	return path, nil
}

func executeCloudPush(ctx context.Context, root string, state pinaxcloud.State, manifest pinaxcloud.Manifest, baseRevision string) (cloudsync.CommitResult, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	if strings.TrimSpace(baseRevision) == "" {
		baseRevision = localCloudBaseRevision(root, state)
	}
	key, err := pinaxcloud.DeriveKey(pinaxcloud.EncryptionSecretRef(state.Config))
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	blobIDs := make([]string, 0, len(manifest.Entries)+len(manifest.Deletes))
	objectRefs := make([]cloudsync.ObjectRef, 0, len(manifest.Entries)+(len(manifest.Deletes)*2))
	for _, entry := range manifest.Entries {
		blobIDs = append(blobIDs, entry.BlobID)
		objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: entry.PathHash, BlobID: entry.BlobID, BlobHash: entry.SHA256, Size: entry.Size})
	}
	for _, deleteMarker := range manifest.Deletes {
		if strings.HasPrefix(deleteMarker.TrashBlobID, "blob_") {
			blobIDs = append(blobIDs, deleteMarker.TrashBlobID)
			objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: deleteMarker.PathHash, BlobID: deleteMarker.TrashBlobID})
		}
		objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: deleteMarker.PathHash, BlobID: deleteMarker.TrashBlobID, Deleted: true})
	}
	missing, err := transport.BatchCheck(ctx, blobIDs)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	missingSet := make(map[string]struct{}, len(missing.MissingBlobIDs))
	for _, blobID := range missing.MissingBlobIDs {
		missingSet[blobID] = struct{}{}
	}
	presentFacts := make(map[string]cloudsync.BlobFact, len(missing.Present))
	for _, fact := range missing.Present {
		presentFacts[fact.BlobID] = fact
	}
	for i, entry := range manifest.Entries {
		if _, ok := missingSet[entry.BlobID]; !ok {
			matchesKey, err := remoteBlobMatchesKey(ctx, transport, entry.BlobID, key.KeyID)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			if matchesKey {
				if fact, ok := presentFacts[entry.BlobID]; ok {
					objectRefs[i].BlobHash = fact.BlobHash
					objectRefs[i].Size = fact.Size
				}
				continue
			}
		}
		content, err := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", entry.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		envelope, err := pinaxcloud.EncryptBlob(key, content, []byte(entry.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		cloudBlob := cloudEnvelope(envelope)
		if metadataWriter, ok := transport.(interface {
			PutBlobWithEnvelopeMetadata(context.Context, string, cloudsync.Envelope) (string, int64, error)
		}); ok {
			blobHash, sizeBytes, err := metadataWriter.PutBlobWithEnvelopeMetadata(ctx, entry.BlobID, cloudBlob)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			objectRefs[i].BlobHash = blobHash
			objectRefs[i].Size = sizeBytes
			continue
		}
		if err := transport.PutBlob(ctx, entry.BlobID, cloudBlob); err != nil {
			return cloudsync.CommitResult{}, err
		}
	}
	for i := len(manifest.Entries); i < len(objectRefs); i++ {
		ref := objectRefs[i]
		if ref.Deleted || ref.BlobID == "" || !strings.HasPrefix(ref.BlobID, "blob_") {
			continue
		}
		if _, ok := missingSet[ref.BlobID]; !ok {
			matchesKey, err := remoteBlobMatchesKey(ctx, transport, ref.BlobID, key.KeyID)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			if matchesKey {
				if fact, ok := presentFacts[ref.BlobID]; ok {
					objectRefs[i].BlobHash = fact.BlobHash
					objectRefs[i].Size = fact.Size
				}
				continue
			}
		}
		content, err := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", ref.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		envelope, err := pinaxcloud.EncryptBlob(key, content, []byte(ref.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		cloudBlob := cloudEnvelope(envelope)
		if metadataWriter, ok := transport.(interface {
			PutBlobWithEnvelopeMetadata(context.Context, string, cloudsync.Envelope) (string, int64, error)
		}); ok {
			blobHash, sizeBytes, err := metadataWriter.PutBlobWithEnvelopeMetadata(ctx, ref.BlobID, cloudBlob)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			objectRefs[i].BlobHash = blobHash
			objectRefs[i].Size = sizeBytes
			continue
		}
		if err := transport.PutBlob(ctx, ref.BlobID, cloudBlob); err != nil {
			return cloudsync.CommitResult{}, err
		}
	}
	manifestEnvelope, err := pinaxcloud.EncryptManifest(key, manifest)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	manifestBlobID := "manifest_" + strings.TrimPrefix(pinaxcloud.BlobID(manifestBytes), "blob_")
	if err := transport.PutManifest(ctx, manifestBlobID, cloudEnvelope(manifestEnvelope)); err != nil {
		return cloudsync.CommitResult{}, err
	}
	return transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: baseRevision, RevisionID: "rev_" + time.Now().UTC().Format("20060102150405.000000000"), ManifestBlobID: manifestBlobID, BlobIDs: blobIDs, ObjectRefs: objectRefs, DeviceID: state.Config.DeviceID, RequestID: "pinax-" + time.Now().UTC().Format("20060102150405.000000000")})
}

// isCloudRevisionConflict reports whether err is a CAS revision conflict from
// either the object-store transports (cloudsync.ErrRevisionConflict) or the
// Pinax Cloud server transport (REVISION_CONFLICT).
func isCloudRevisionConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, cloudsync.ErrRevisionConflict) {
		return true
	}
	return cloudclient.IsRevisionConflict(err)
}

// cloudRebaseOutcome reports whether a freshly pulled remote snapshot lets a
// push retry cleanly or exposes an unresolvable content conflict.
type cloudRebaseOutcome struct {
	conflict  bool
	conflicts []syncplan.Operation
}

// planCloudRebase diffs the local manifest against the freshly pulled remote
// snapshot (using the original base as the common ancestor) and reports whether
// the push can be retried cleanly or must surface a content conflict.
func planCloudRebase(localManifest, baseManifest, remoteManifest pinaxcloud.Manifest, remoteRevision string) cloudRebaseOutcome {
	diffPlan, _ := syncplan.BuildPlan(syncplan.Request{Direction: syncplan.DirectionDiff, Target: "cloud", LocalManifest: localManifest, BaseManifest: baseManifest, RemoteManifest: remoteManifest, BaseRevision: remoteRevision, RemoteRevision: remoteRevision, DryRun: true, Yes: true})
	var conflicts []syncplan.Operation
	for _, op := range diffPlan.Operations {
		if op.Kind == "conflict" {
			conflicts = append(conflicts, op)
		}
	}
	if len(conflicts) > 0 {
		return cloudRebaseOutcome{conflict: true, conflicts: conflicts}
	}
	return cloudRebaseOutcome{}
}

// cloudRebasePlan bundles the commit/pull hooks and plan inputs for a push that
// may auto-rebase once on a revision conflict. The commit and pull functions are
// injected so the rebase retry path is deterministic and unit-testable.
type cloudRebasePlan struct {
	commit        func(baseRevision string) (cloudsync.CommitResult, error)
	pull          func() (cloudRemoteSnapshot, error)
	localManifest pinaxcloud.Manifest
	baseManifest  pinaxcloud.Manifest
	baseRevision  string
	yes           bool
}

// cloudPushRebaseResult is the outcome of a push with optional auto-rebase.
type cloudPushRebaseResult struct {
	Commit    cloudsync.CommitResult
	Conflicts []syncplan.Operation // non-empty when auto-rebase found content conflicts
	Rebased   bool                 // true when the commit succeeded on the retry after rebase
}

// runCloudPushRebase commits a push and, when --yes is set and the commit fails
// with a revision conflict, pulls the remote head, rebuilds the push plan, and
// retries the commit exactly once:
//   - no content conflict -> retry the commit against the new base revision;
//   - content conflict    -> return the conflict operations (caller builds the
//     conflict projection), no error;
//   - retry exhausted     -> surface the ORIGINAL revision conflict error;
//   - pull failure        -> surface the original revision conflict error.
//
// There is no retry loop: at most one rebase attempt.
func runCloudPushRebase(plan cloudRebasePlan) (cloudPushRebaseResult, error) {
	commit, err := plan.commit(plan.baseRevision)
	if err == nil {
		return cloudPushRebaseResult{Commit: commit}, nil
	}
	if !plan.yes || !isCloudRevisionConflict(err) {
		return cloudPushRebaseResult{}, err
	}
	originalConflict := err
	snapshot, pullErr := plan.pull()
	if pullErr != nil {
		// Cannot rebase onto the remote head; surface the original conflict.
		return cloudPushRebaseResult{}, originalConflict
	}
	outcome := planCloudRebase(plan.localManifest, plan.baseManifest, snapshot.Manifest, snapshot.RevisionID)
	if outcome.conflict {
		return cloudPushRebaseResult{Conflicts: outcome.conflicts}, nil
	}
	retried, retryErr := plan.commit(snapshot.RevisionID)
	if retryErr != nil {
		// Retry exhausted: surface the original revision conflict (no loop).
		return cloudPushRebaseResult{}, originalConflict
	}
	return cloudPushRebaseResult{Commit: retried, Rebased: true}, nil
}

// cloudRebaseConflictEntries converts auto-rebase conflict operations into sync
// conflict entries so the conflict projection's next actions point at real paths.
func cloudRebaseConflictEntries(ops []syncplan.Operation) []domain.SyncConflictEntry {
	entries := make([]domain.SyncConflictEntry, 0, len(ops))
	for _, op := range ops {
		entries = append(entries, domain.SyncConflictEntry{File: op.Path, MainPath: op.Path})
	}
	return entries
}

func remoteBlobMatchesKey(ctx context.Context, transport cloudsync.Transport, blobID, keyID string) (bool, error) {
	envelope, err := transport.GetBlob(ctx, blobID)
	if errors.Is(err, cloudsync.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return envelope.KeyID == keyID, nil
}

func remoteEnvelope(envelope cloudsync.Envelope) pinaxcloud.EncryptedEnvelope {
	return pinaxcloud.EncryptedEnvelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func cloudEnvelope(envelope pinaxcloud.EncryptedEnvelope) cloudsync.Envelope {
	return cloudsync.Envelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func cloudSyncNotConfiguredProjection(root, target string) domain.Projection {
	outputTarget := syncOutputTarget(target)
	projection := domain.NewProjection("sync.diff", "Capsa sync requires configuring a backend first.")
	projection.Status = "partial"
	projection.Facts["target"] = outputTarget
	projection.Facts["backend_required"] = "true"
	projection.Facts["configured"] = "false"
	projection.Facts["remote_write"] = "false"
	projection.Data = map[string]any{"target": outputTarget, "remote_write": false, "plan": map[string]any{"target": outputTarget, "status": "backend_required"}}
	projection.Actions = []domain.Action{{Name: "login", Command: fmt.Sprintf("pinax %s login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", syncConfigCommand(target), shellQuote(root))}}
	addCapsaBridgeFacts(&projection, target)
	return projection
}

func isCommandErrorCode(err error, code string) bool {
	var commandErr *domain.CommandError
	return errors.As(err, &commandErr) && commandErr.Code == code
}

func addCloudSyncFacts(projection *domain.Projection, state pinaxcloud.State, plan syncplan.Plan) {
	projection.Facts["target"] = syncOutputTarget(plan.Target)
	projection.Facts["workspace_id"] = state.Config.WorkspaceID
	projection.Facts["device_id"] = state.Config.DeviceID
	projection.Facts["backend_kind"] = directBackendKind(state)
	projection.Facts["dry_run"] = fmt.Sprint(plan.DryRun)
	projection.Facts["remote_write"] = fmt.Sprint(plan.RemoteWrite)
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["base_revision"] = plan.BaseRevision
	projection.Facts["remote_revision"] = plan.RemoteRevision
	projection.Facts["conflicts"] = fmt.Sprint(len(plan.ConflictQueue))
}

func addCloudContentFacts(projection *domain.Projection, manifest pinaxcloud.Manifest) {
	var bytes int64
	scripts := 0
	binaries := 0
	trashBackups := 0
	for _, entry := range manifest.Entries {
		bytes += entry.Size
		if isManifestScript(entry) {
			scripts++
			continue
		}
		if !isManifestText(entry.Path) {
			binaries++
		}
	}
	for _, deleteMarker := range manifest.Deletes {
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" {
			trashBackups++
		}
	}
	projection.Facts["content_files"] = fmt.Sprint(len(manifest.Entries))
	projection.Facts["content_bytes"] = fmt.Sprint(bytes)
	projection.Facts["script_files"] = fmt.Sprint(scripts)
	projection.Facts["binary_files"] = fmt.Sprint(binaries)
	projection.Facts["delete_markers"] = fmt.Sprint(len(manifest.Deletes))
	projection.Facts["trash_backup_blobs"] = fmt.Sprint(trashBackups)
}

func manifestTrashBackupCount(manifest pinaxcloud.Manifest) int {
	count := 0
	for _, deleteMarker := range manifest.Deletes {
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" {
			count++
		}
	}
	return count
}

func isManifestScript(entry pinaxcloud.ManifestEntry) bool {
	return strings.HasPrefix(entry.Path, "scripts/") || entry.Mode&0o111 != 0 || strings.HasSuffix(strings.ToLower(entry.Path), ".sh")
}

func isManifestText(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".yaml", ".yml", ".json", ".jsonl", ".toml", ".csv", ".gitignore", ".pinaxignore":
		return true
	case ".sh", ".bash", ".zsh", ".fish", ".py", ".js", ".ts":
		return true
	}
	base := filepath.Base(path)
	return base == ".gitignore" || base == ".pinaxignore"
}
