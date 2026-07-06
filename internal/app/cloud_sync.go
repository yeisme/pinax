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
	pathPolicy := syncops.NormalizePathPolicy(req.PathPolicy)
	state, err := cloudStateForSync(root, req)
	if err != nil {
		return cloudStateErrorProjection(command, root, err)
	}
	receipt := syncRunStart(command, direction, state, pathPolicy)
	manifest := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}
	if direction != syncplan.DirectionPull {
		manifest, err = pinaxcloud.BuildManifest(root)
		if err != nil {
			projection := errorProjection(command, err)
			return projection, err
		}
	}
	baseRevision := req.BaseRevision
	remoteRevision := req.RemoteRevision
	if remoteRevision == "" {
		remoteRevision = baseRevision
	}
	plan, planErr := syncplan.BuildPlan(syncplan.Request{Direction: direction, Target: "cloud", LocalManifest: manifest, BaseRevision: baseRevision, RemoteRevision: remoteRevision, DryRun: req.DryRun, Yes: req.Yes})
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
		addCloudContentFacts(&projection, manifest)
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
		return projection, commandErr
	}
	if planErr != nil {
		projection := errorProjection(command, planErr)
		return projection, planErr
	}
	if direction == syncplan.DirectionPush && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		commit, execErr := executeCloudPush(ctx, root, state, manifest, req.BaseRevision)
		if execErr != nil {
			plan.RemoteWrite = false
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		plan.RemoteWrite = commit.RemoteWrite
		receipt.RemoteWrite = commit.RemoteWrite
		receipt.RevisionID = commit.RevisionID
		receipt.ManifestBlobID = commit.ManifestBlobID
		receipt.Counts["blobs"] = len(manifest.Entries) + manifestTrashBackupCount(manifest)
		receipt.Counts["delete_markers"] = len(manifest.Deletes)
		receipt.Counts["trash_backup_blobs"] = manifestTrashBackupCount(manifest)
		projection := domain.NewProjection(command, "Cloud sync push completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, commit.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
		addCloudContentFacts(&projection, manifest)
		projection.Facts["run_id"] = receipt.RunID
		projection.Facts["revision_id"] = commit.RevisionID
		projection.Evidence = []string{receiptPath}
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "remote_write": commit.RemoteWrite, "revision_id": commit.RevisionID, "manifest_blob_id": commit.ManifestBlobID, "receipt": receipt}
		return projection, nil
	}
	if direction == syncplan.DirectionPull && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		pullResult, execErr := executeCloudPull(ctx, root, state)
		if execErr != nil {
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
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
		projection := domain.NewProjection(command, "Cloud sync pull completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		if len(pullResult.Conflicts) > 0 {
			projection.Actions = append(projection.Actions, syncConflictActions(root, pullResult.Conflicts)...)
		}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, pullResult.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
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
	projection := domain.NewProjection(command, "Cloud sync plan generated; real remote writes are not wired yet.")
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
		projection.Actions = []domain.Action{{Name: "handoff", Command: fmt.Sprintf("pinax sync diff --target cloud --vault %s --json", shellQuote(root))}}
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
	addCloudContentFacts(&projection, manifest)
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
		return &domain.CommandError{Code: "lock_held", Message: message, Hint: "Retry after the current cloud sync finishes"}
	case strings.Contains(rawMessage, "transport_unavailable"), isRcloneCommandFailure(rawMessage):
		return &domain.CommandError{Code: "transport_unavailable", Message: message, Hint: "Check the configured cloud transport before retrying"}
	default:
		return &domain.CommandError{Code: "cloud_sync_failed", Message: message, Hint: "Run pinax cloud doctor --vault <vault> --json"}
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
	Conflicts      []domain.SyncConflictEntry
}

func executeCloudPull(ctx context.Context, root string, state pinaxcloud.State) (directPullResult, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return directPullResult{}, err
	}
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		return directPullResult{}, err
	}
	if strings.TrimSpace(head.CurrentRevision) == "" || strings.TrimSpace(head.ManifestBlobID) == "" {
		return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "cloud backend has no committed revision", Hint: "Run pinax sync push --target cloud --yes from a device with notes first"}
	}
	key, err := pinaxcloud.DeriveKey(pinaxcloud.EncryptionSecretRef(state.Config))
	if err != nil {
		return directPullResult{}, err
	}
	manifestEnvelope, err := transport.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		return directPullResult{}, err
	}
	manifest, err := pinaxcloud.DecryptManifest(key, remoteEnvelope(manifestEnvelope))
	if err != nil {
		return directPullResult{}, err
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
	for _, entry := range manifest.Entries {
		blobEnvelope, err := transport.GetBlob(ctx, entry.BlobID)
		if err != nil {
			return directPullResult{}, err
		}
		content, err := pinaxcloud.DecryptBlob(key, remoteEnvelope(blobEnvelope), []byte(entry.BlobID))
		if err != nil {
			return directPullResult{}, err
		}
		path, err := safeCloudSyncPath(root, entry.Path)
		if err != nil {
			return directPullResult{}, err
		}
		fileMode := os.FileMode(entry.Mode & 0o777)
		if fileMode == 0 {
			fileMode = 0o600
		}
		if existing, err := os.ReadFile(path); err == nil {
			if bytes.Equal(existing, content) {
				if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() != fileMode {
					if err := os.Chmod(path, fileMode); err != nil {
						return directPullResult{}, err
					}
					filesApplied++
				}
				continue
			}
			conflictPath := syncConflictCopyPath(path, time.Now().UTC())
			if err := os.WriteFile(conflictPath, existing, fileMode); err != nil {
				return directPullResult{}, err
			}
			if rel, relErr := filepath.Rel(root, conflictPath); relErr == nil {
				conflictRel := filepath.ToSlash(rel)
				mainRel, mainErr := mainPathForSyncConflict(conflictRel)
				if mainErr != nil {
					return directPullResult{}, mainErr
				}
				conflicts = append(conflicts, domain.SyncConflictEntry{File: conflictRel, MainPath: mainRel})
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return directPullResult{}, err
		}
		if err := os.WriteFile(path, content, fileMode); err != nil {
			return directPullResult{}, err
		}
		filesApplied++
	}
	result := directPullResult{FilesApplied: filesApplied, DeletesApplied: deletesApplied, RevisionID: head.CurrentRevision, ManifestBlobID: head.ManifestBlobID, Conflicts: conflicts}

	return result, nil
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
	if err != nil || state.Target != "cloud" {
		return ""
	}
	if state.BackendKind != directBackendKind(cloudState) || state.WorkspaceID != cloudState.Config.WorkspaceID || state.Endpoint != cloudState.Config.Endpoint {
		return ""
	}
	return strings.TrimSpace(state.LastSyncedRevision)
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

func cloudSyncNotConfiguredProjection(root string) domain.Projection {
	projection := domain.NewProjection("sync.diff", "Cloud sync requires configuring a backend first.")
	projection.Status = "partial"
	projection.Facts["target"] = "cloud"
	projection.Facts["backend_required"] = "true"
	projection.Facts["configured"] = "false"
	projection.Facts["remote_write"] = "false"
	projection.Data = map[string]any{"target": "cloud", "remote_write": false, "plan": map[string]any{"target": "cloud", "status": "backend_required"}}
	projection.Actions = []domain.Action{{Name: "login", Command: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}}
	return projection
}

func isCommandErrorCode(err error, code string) bool {
	var commandErr *domain.CommandError
	return errors.As(err, &commandErr) && commandErr.Code == code
}

func addCloudSyncFacts(projection *domain.Projection, state pinaxcloud.State, plan syncplan.Plan) {
	projection.Facts["target"] = "cloud"
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
