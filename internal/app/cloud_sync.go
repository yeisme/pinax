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

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
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

func commandErrorFromError(err error) *domain.CommandError {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr
	}
	rawMessage := err.Error()
	message := syncops.SanitizeString(rawMessage)
	switch {
	case strings.Contains(strings.ToLower(rawMessage), "key id mismatch"):
		return &domain.CommandError{Code: "encryption_key_mismatch", Message: "remote data was encrypted with a different sync key", Hint: "Restore the previous encryption secret; do not push or rotate keys until the remote state is verified"}
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

// cloudTransportForStateWithCredential extends cloudTransportForState for the
// repository-encrypted credential mode. When the runtime S3 config declares
// repository-encrypted mode and an unlock source is supplied, the typed S3/COS
// bundle is resolved via SyncCredentialResolver and injected as an explicit AWS
// SDK credentials provider into the object store — the sync run never consults
// the device-local shared profile chain. On any other mode, or when no source
// is supplied, it falls back to cloudTransportForState. The returned snapshot
// (if any) MUST be closed by the caller after the sync run so plaintext is
// wiped; when no resolution happens, the returned snapshot is nil.
func cloudTransportForStateWithCredential(ctx context.Context, state pinaxcloud.State, repoRoot string, source projectsecrets.UnlockSource) (cloudsync.Transport, *projectsecrets.Snapshot, error) {
	mode := ""
	if state.Config.S3 != nil {
		mode = state.Config.S3.CredentialMode
	}
	// Fail closed (pinax-passphrase-s3-bootstrap task 6.7): repository-encrypted
	// mode MUST NOT fall back to the device-local shared AWS profile, default
	// credential chain or unrelated process AWS environment variables. An
	// explicit unlock source is required to unlock the typed bundle before any
	// remote access, so a missing source is an error rather than a silent
	// fallback to another local account.
	if mode == pinaxcloud.CredentialModeRepositoryEncrypted && source == nil {
		return nil, nil, &domain.CommandError{
			Code:    "sync_repo_unlock_required",
			Message: "repository-encrypted credential mode requires an explicit unlock source",
			Hint:    "Use --unlock keychain/file/env, --passphrase-file, or --env-var to supply the repository passphrase.",
		}
	}
	if mode != pinaxcloud.CredentialModeRepositoryEncrypted {
		t, err := cloudTransportForState(ctx, state)
		return t, nil, err
	}
	credEntry := state.Config.SecretRef
	if credEntry == "" {
		credEntry = "default"
	}
	resolver := NewSyncCredentialResolver("pinax", state.Config.WorkspaceID, credEntry)
	provider, snap, err := resolver.Resolve(ctx, repoRoot, source)
	if err != nil {
		return nil, nil, err
	}
	store, err := state.GetStoreWithCredentialProvider(ctx, provider)
	if err != nil {
		if snap != nil {
			_ = snap.Close()
		}
		return nil, nil, err
	}
	transport := cloudsync.NewObjectStoreTransport(store, cloudsync.Layout{WorkspaceID: state.Config.WorkspaceID, VaultID: state.Config.WorkspaceID})
	return transport, snap, nil
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
	Keys           pinaxcloud.CryptoKeys
	Manifest       pinaxcloud.Manifest
	RevisionID     string
	ManifestBlobID string
	// ManifestKeyID is the raw KeyID of the remote manifest envelope before
	// decryption; `pinax sync keys` classifies the remote derivation from it.
	ManifestKeyID string
}

// syncKeychain derives the vault's decryption keychain, provisioning and
// persisting the per-vault v2 salt on first use. Legacy envelopes stay
// readable through the legacy fallback key.
func syncKeychain(root string, state pinaxcloud.State) (pinaxcloud.CryptoKeys, error) {
	return pinaxcloud.DeriveKeychain(pinaxcloud.EncryptionSecretRef(state.Config))
}

func loadCloudRemoteSnapshot(ctx context.Context, root string, state pinaxcloud.State) (cloudRemoteSnapshot, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	return loadCloudRemoteSnapshotViaTransport(ctx, root, state, transport)
}

// loadCloudRemoteSnapshotWithCredential loads the remote snapshot using a
// credential-injecting transport when a project unlock source is supplied
// (repository-encrypted mode). The resolved AWS SDK StaticCredentialsProvider
// is self-contained, so the projectsecrets.Snapshot can be closed as soon as
// the transport is built — the apply path reuses snapshot.Transport for blob
// fetches, so the credential flows through the whole pull. When source is nil
// or the mode is not repository-encrypted, it falls back to loadCloudRemoteSnapshot.
func loadCloudRemoteSnapshotWithCredential(ctx context.Context, state pinaxcloud.State, repoRoot string, source projectsecrets.UnlockSource) (cloudRemoteSnapshot, error) {
	// Always route through the unified credential-aware transport so a
	// repository-encrypted vault with no unlock source fails closed instead of
	// silently reading via the device-local shared profile chain (task 6.7).
	transport, snap, err := cloudTransportForStateWithCredential(ctx, state, repoRoot, source)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	if snap != nil {
		// The provider holds its own credential copies; the projectsecrets
		// snapshot's plaintext buffers can be wiped now.
		_ = snap.Close()
	}
	return loadCloudRemoteSnapshotViaTransport(ctx, repoRoot, state, transport)
}

func loadCloudRemoteSnapshotViaTransport(ctx context.Context, root string, state pinaxcloud.State, transport cloudsync.Transport) (cloudRemoteSnapshot, error) {
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	if strings.TrimSpace(head.CurrentRevision) == "" || strings.TrimSpace(head.ManifestBlobID) == "" {
		return cloudRemoteSnapshot{}, nil
	}
	keys, err := syncKeychain(root, state)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	manifestEnvelope, err := transport.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	manifest, err := pinaxcloud.DecryptManifest(keys, remoteEnvelope(manifestEnvelope))
	if err != nil {
		return cloudRemoteSnapshot{}, err
	}
	return cloudRemoteSnapshot{Transport: transport, Keys: keys, Manifest: manifest, RevisionID: head.CurrentRevision, ManifestBlobID: head.ManifestBlobID, ManifestKeyID: manifestEnvelope.KeyID}, nil
}

func executeCloudPull(ctx context.Context, root string, state pinaxcloud.State, plan syncplan.Plan, snapshot cloudRemoteSnapshot) (directPullResult, error) {
	if strings.TrimSpace(snapshot.RevisionID) == "" || strings.TrimSpace(snapshot.ManifestBlobID) == "" {
		return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "Capsa backend has no committed revision", Hint: "Run pinax sync push --target capsa --yes from a device with notes first"}
	}
	transport := snapshot.Transport
	keys := snapshot.Keys
	manifest := snapshot.Manifest
	if transport == nil {
		loaded, err := loadCloudRemoteSnapshot(ctx, root, state)
		if err != nil {
			return directPullResult{}, err
		}
		if strings.TrimSpace(loaded.RevisionID) == "" || strings.TrimSpace(loaded.ManifestBlobID) == "" {
			return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "Capsa backend has no committed revision", Hint: "Run pinax sync push --target capsa --yes from a device with notes first"}
		}
		transport = loaded.Transport
		keys = loaded.Keys
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
	entriesByObjectID := manifestEntriesByObjectID(manifest)
	for _, op := range plan.Operations {
		switch op.Kind {
		case "download_blob":
			entry, ok := entries[op.Path]
			if op.ObjectID != "" {
				entry, ok = entriesByObjectID[op.ObjectID]
			}
			if !ok {
				continue
			}
			// The planner marks downloads whose local copy provably matches
			// the common base; only those may overwrite local without a
			// conflict copy. A local file that drifted from the last synced
			// blob since the plan was built (TOCTOU) still preserves a copy.
			preserveConflict := !op.FastForward || localFileDriftedFromBlob(root, entry.Path, op.LocalBlobID)
			applied, newConflicts, err := applyRemoteManifestEntryWithPolicy(ctx, root, transport, keys, entry, preserveConflict)
			if err != nil {
				return directPullResult{}, err
			}
			if applied {
				filesApplied++
				if err := recordRemoteManifestEntry(ctx, root, entry, ""); err != nil {
					return directPullResult{}, err
				}
			}
			conflicts = append(conflicts, newConflicts...)
		case "delete_local":
			applied, err := deleteLocalManifestObject(root, op.ObjectID, op.Path)
			if err != nil {
				return directPullResult{}, err
			}
			if applied {
				filesApplied++
			}
		case "move":
			entry, ok := entriesByObjectID[op.ObjectID]
			if !ok {
				continue
			}
			moved, moveErr := moveLocalManifestObject(root, op.ObjectID, entry.Path)
			if moveErr != nil {
				return directPullResult{}, moveErr
			}
			if moved {
				filesApplied++
			}
			preserveConflict := op.BaseRevision == "" || op.LocalRevision != op.BaseRevision
			applied, newConflicts, applyErr := applyRemoteManifestEntryWithPolicy(ctx, root, transport, keys, entry, preserveConflict)
			if applyErr != nil {
				return directPullResult{}, applyErr
			}
			if applied {
				filesApplied++
			}
			if moved || applied {
				if err := recordRemoteManifestEntry(ctx, root, entry, op.FromPath); err != nil {
					return directPullResult{}, err
				}
			}
			conflicts = append(conflicts, newConflicts...)
		case "conflict", "revision_conflict":
			entry, ok := entries[op.Path]
			if op.ObjectID != "" {
				entry, ok = entriesByObjectID[op.ObjectID]
			}
			if ok {
				applied, newConflicts, err := applyRemoteManifestEntry(ctx, root, transport, keys, entry)
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

func manifestEntriesByObjectID(manifest pinaxcloud.Manifest) map[string]pinaxcloud.ManifestEntry {
	entries := make(map[string]pinaxcloud.ManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if strings.TrimSpace(entry.ObjectID) != "" {
			entries[entry.ObjectID] = entry
		}
	}
	return entries
}

func localManifestObjectPath(root, objectID, fallback string) string {
	if strings.TrimSpace(objectID) != "" {
		notes, err := scanNotes(root)
		if err == nil {
			for _, note := range notes {
				if note.ID == objectID {
					return note.Path
				}
			}
		}
	}
	return fallback
}

func moveLocalManifestObject(root, objectID, targetRel string) (bool, error) {
	currentRel := localManifestObjectPath(root, objectID, "")
	if currentRel == "" || currentRel == targetRel {
		return false, nil
	}
	current, err := safeCloudSyncPath(root, currentRel)
	if err != nil {
		return false, err
	}
	target, err := safeCloudSyncPath(root, targetRel)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(current); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := os.Stat(target); err == nil {
		return false, &domain.CommandError{Code: "sync_path_collision", Message: "remote object path is occupied by another local file", Hint: "Review sync conflicts before retrying"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	return true, os.Rename(current, target)
}

func deleteLocalManifestObject(root, objectID, fallback string) (bool, error) {
	return deleteLocalManifestPath(root, localManifestObjectPath(root, objectID, fallback))
}

func applyRemoteManifestEntry(ctx context.Context, root string, transport cloudsync.Transport, keys pinaxcloud.CryptoKeys, entry pinaxcloud.ManifestEntry) (bool, []domain.SyncConflictEntry, error) {
	return applyRemoteManifestEntryWithPolicy(ctx, root, transport, keys, entry, true)
}

// localFileDriftedFromBlob reports whether the vault file at rel still hashes
// to lastSyncedBlobID. Unknown states (missing file, missing blob id, read
// error) read as drifted so apply stays conservative and preserves a copy.
func localFileDriftedFromBlob(root, rel, lastSyncedBlobID string) bool {
	if strings.TrimSpace(lastSyncedBlobID) == "" {
		return true
	}
	path, err := safeCloudSyncPath(root, rel)
	if err != nil {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return pinaxcloud.BlobID(raw) != lastSyncedBlobID
}

func applyRemoteManifestEntryWithPolicy(ctx context.Context, root string, transport cloudsync.Transport, keys pinaxcloud.CryptoKeys, entry pinaxcloud.ManifestEntry, preserveConflict bool) (bool, []domain.SyncConflictEntry, error) {
	blobEnvelope, err := transport.GetBlob(ctx, entry.BlobID)
	if err != nil {
		return false, nil, err
	}
	content, err := pinaxcloud.DecryptBlob(keys, remoteEnvelope(blobEnvelope), []byte(entry.BlobID))
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
		if preserveConflict {
			conflict, err := writeConflictCopy(root, path, existing, fileMode, time.Now().UTC())
			if err != nil {
				return false, nil, err
			}
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, nil, err
	}
	if err := atomicWriteFile(path, content, fileMode); err != nil {
		return false, nil, err
	}
	return true, conflicts, nil
}

func recordRemoteManifestEntry(ctx context.Context, root string, entry pinaxcloud.ManifestEntry, oldPath string) error {
	if entry.ObjectKind != "note" || strings.TrimSpace(entry.ObjectID) == "" {
		return nil
	}
	path, err := safeCloudSyncPath(root, entry.Path)
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	note := parseNote(entry.Path, string(payload))
	if note.ID == "" {
		note.ID = entry.ObjectID
	}
	kind := domain.RecordEventNoteMetadataUpdated
	idempotency := "sync.pull.update:" + note.ID + ":" + entry.RevisionID
	if strings.TrimSpace(oldPath) != "" && filepath.ToSlash(oldPath) != entry.Path {
		kind = domain.RecordEventNoteMoved
		idempotency = "sync.pull.move:" + note.ID + ":" + filepath.ToSlash(oldPath) + ":" + entry.Path + ":" + entry.RevisionID
	}
	_, err = appendNoteRecordEvent(ctx, root, kind, idempotency, note, filepath.ToSlash(oldPath))
	return err
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
	if err := atomicWriteFile(conflictPath, content, mode); err != nil {
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

func writeCloudManifestCache(root, revision string, manifest pinaxcloud.Manifest) error {
	if strings.TrimSpace(revision) == "" {
		return nil
	}
	rel := syncManifestCacheRel(revision)
	return writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), manifest)
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

// executeCloudPushWithCredential commits a push, optionally injecting the
// unlock source: in repository-encrypted mode it injects the resolved AWS SDK
// credentials provider into the push transport. The snapshot is closed after
// the transport is built (StaticCredentialsProvider is self-contained).
// cloudUploadStats records uploads performed by the push commit loop,
// including the key-rotation re-encryption branch that the plan-based receipt
// counters do not see.
type cloudUploadStats struct {
	Blobs int64
	Bytes int64
}

func executeCloudPushWithCredential(ctx context.Context, root string, state pinaxcloud.State, manifest pinaxcloud.Manifest, baseRevision string, source projectsecrets.UnlockSource, stats *cloudUploadStats) (cloudsync.CommitResult, error) {
	if stats == nil {
		stats = &cloudUploadStats{}
	}
	// Resolve the transport through the unified credential-aware path so a
	// repository-encrypted vault with no unlock source fails closed instead of
	// falling back to the device-local shared profile chain (task 6.7).
	transport, snap, err := cloudTransportForStateWithCredential(ctx, state, root, source)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	if snap != nil {
		_ = snap.Close()
	}
	if strings.TrimSpace(baseRevision) == "" {
		baseRevision = localCloudBaseRevision(root, state)
	}
	keys, err := syncKeychain(root, state)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	key := keys.Active
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
		stats.Blobs++
		stats.Bytes += int64(len(content))
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
	result, err := transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: baseRevision, RevisionID: "rev_" + time.Now().UTC().Format("20060102150405.000000000"), ManifestBlobID: manifestBlobID, BlobIDs: blobIDs, ObjectRefs: objectRefs, DeviceID: state.Config.DeviceID, RequestID: "pinax-" + time.Now().UTC().Format("20060102150405.000000000")})
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	// Durable commit read-back (pinax-passphrase-s3-bootstrap task 6.8): a backup
	// is durable only once the committed revision is observable on the remote.
	// If the head or manifest cannot be read back, report remote_write=false so
	// the caller treats the backup as incomplete rather than trusting the
	// CommitRevision call alone.
	if !verifyDurableCommit(ctx, transport, state.Config.WorkspaceID, result) {
		return cloudsync.CommitResult{RevisionID: result.RevisionID, ManifestBlobID: result.ManifestBlobID, RemoteWrite: false}, nil
	}
	return result, nil
}

// remoteReadBack is the minimal transport surface required to verify a durable
// commit. cloudsync.Transport satisfies it; tests may supply a focused fake.
type remoteReadBack interface {
	CurrentHead(ctx context.Context, vaultID string) (cloudsync.Head, error)
	GetManifest(ctx context.Context, blobID string) (cloudsync.Envelope, error)
}

// verifyDurableCommit confirms the committed revision is observable on the
// remote by reading back the head and manifest. A commit whose revision is not
// observable (head missing, head mismatch, or manifest unreadable) is not
// durable and MUST be reported as remote_write=false.
func verifyDurableCommit(ctx context.Context, transport remoteReadBack, workspaceID string, result cloudsync.CommitResult) bool {
	head, err := transport.CurrentHead(ctx, workspaceID)
	if err != nil || strings.TrimSpace(head.CurrentRevision) == "" || head.CurrentRevision != result.RevisionID {
		return false
	}
	if _, err := transport.GetManifest(ctx, result.ManifestBlobID); err != nil {
		return false
	}
	return true
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
	data := map[string]any{"target": outputTarget, "remote_write": false, "plan": map[string]any{"target": outputTarget, "status": "backend_required"}}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(syncplan.Plan{Direction: syncplan.DirectionDiff, Target: outputTarget}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, syncOutputViewOptions{Scope: "cached", Result: "partial"}))
	projection.Data = data
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

// cloudManifestContentEqual reports whether two manifests carry the same sync
// content identity — entries matched by PathHash -> (BlobID, Mode, ObjectKind)
// and delete markers matched by PathHash -> (ObjectKind, TombstoneID,
// TrashBlobID). Volatile per-build metadata (GeneratedAt, UpdatedAt, device/
// revision bookkeeping) is ignored so a rebuilt-but-unchanged manifest compares
// equal. This is the up-to-date signal for push (task 6.8): when the local
// manifest content already equals the remote, there is nothing to push.
func cloudManifestContentEqual(a, b pinaxcloud.Manifest) bool {
	if len(a.Entries) != len(b.Entries) || len(a.Deletes) != len(b.Deletes) {
		return false
	}
	wantEntries := make(map[string]string, len(a.Entries))
	for _, e := range a.Entries {
		wantEntries[e.PathHash] = e.BlobID + "\x00" + e.ObjectKind + "\x00" + fmt.Sprint(e.Mode)
	}
	for _, e := range b.Entries {
		if wantEntries[e.PathHash] != e.BlobID+"\x00"+e.ObjectKind+"\x00"+fmt.Sprint(e.Mode) {
			return false
		}
		delete(wantEntries, e.PathHash)
	}
	if len(wantEntries) != 0 {
		return false
	}
	wantDeletes := make(map[string]string, len(a.Deletes))
	for _, d := range a.Deletes {
		wantDeletes[d.PathHash] = d.ObjectKind + "\x00" + d.TombstoneID + "\x00" + d.TrashBlobID
	}
	for _, d := range b.Deletes {
		if wantDeletes[d.PathHash] != d.ObjectKind+"\x00"+d.TombstoneID+"\x00"+d.TrashBlobID {
			return false
		}
		delete(wantDeletes, d.PathHash)
	}
	return len(wantDeletes) == 0
}

// setRemoteCheckedFacts records whether the projection reflects a real
// remote-aware comparison (the remote head + manifest were actually read) or a
// cached, local-only comparison (pinax-passphrase-s3-bootstrap task 6.8). A
// cached diff/dry-run MUST NOT pass as a pre-backup check.
func setRemoteCheckedFacts(projection *domain.Projection, remoteLoaded bool) {
	if remoteLoaded {
		projection.Facts["remote_checked"] = "true"
		projection.Facts["diff_scope"] = "remote-aware"
	} else {
		projection.Facts["remote_checked"] = "false"
		projection.Facts["diff_scope"] = "cached"
	}
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

// remoteSnapshotFullyUnderKey reports whether every object in the remote
// snapshot — the manifest envelope and every blob referenced by it — carries
// the given key id. The up-to-date push fast path uses it so a key-derivation
// rotation still re-encrypts unchanged content instead of skipping it.
func remoteSnapshotFullyUnderKey(ctx context.Context, snapshot cloudRemoteSnapshot, keyID string) bool {
	if snapshot.ManifestKeyID != keyID {
		return false
	}
	for _, entry := range snapshot.Manifest.Entries {
		matches, err := remoteBlobMatchesKey(ctx, snapshot.Transport, entry.BlobID, keyID)
		if err != nil || !matches {
			return false
		}
	}
	for _, deleteMarker := range snapshot.Manifest.Deletes {
		if !strings.HasPrefix(deleteMarker.TrashBlobID, "blob_") {
			continue
		}
		matches, err := remoteBlobMatchesKey(ctx, snapshot.Transport, deleteMarker.TrashBlobID, keyID)
		if err != nil || !matches {
			return false
		}
	}
	return true
}
