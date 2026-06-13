package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/remote"
)

type ObjectStoreTransport struct {
	store  remote.BlobStore
	layout Layout
}

func NewObjectStoreTransport(store remote.BlobStore, layout Layout) *ObjectStoreTransport {
	return &ObjectStoreTransport{store: store, layout: layout}
}

func (t *ObjectStoreTransport) CurrentHead(ctx context.Context, vaultID string) (Head, error) {
	data, _, err := t.store.Get(ctx, t.layout.HeadKey())
	if errors.Is(err, remote.ErrObjectNotFound) {
		if vaultID == "" {
			vaultID = t.layout.VaultID
		}
		return Head{SchemaVersion: HeadSchemaVersion, VaultID: vaultID}, nil
	}
	if err != nil {
		return Head{}, err
	}
	var head Head
	if err := json.Unmarshal(data, &head); err != nil {
		return Head{}, err
	}
	return head, nil
}

func (t *ObjectStoreTransport) BatchCheck(ctx context.Context, blobIDs []string) (BatchCheckResult, error) {
	missing := make([]string, 0)
	for _, blobID := range blobIDs {
		if _, err := t.store.Stat(ctx, t.layout.BlobKey(blobID)); errors.Is(err, remote.ErrObjectNotFound) {
			missing = append(missing, blobID)
		} else if err != nil {
			return BatchCheckResult{}, err
		}
	}
	return BatchCheckResult{MissingBlobIDs: missing}, nil
}

func (t *ObjectStoreTransport) PutBlob(ctx context.Context, blobID string, envelope Envelope) error {
	return t.putEnvelope(ctx, t.layout.BlobKey(blobID), envelope)
}

func (t *ObjectStoreTransport) GetBlob(ctx context.Context, blobID string) (Envelope, error) {
	return t.getEnvelope(ctx, t.layout.BlobKey(blobID))
}

func (t *ObjectStoreTransport) PutManifest(ctx context.Context, blobID string, envelope Envelope) error {
	return t.putEnvelope(ctx, t.layout.ManifestKey(blobID), envelope)
}

func (t *ObjectStoreTransport) GetManifest(ctx context.Context, blobID string) (Envelope, error) {
	return t.getEnvelope(ctx, t.layout.ManifestKey(blobID))
}

func (t *ObjectStoreTransport) CommitRevision(ctx context.Context, req CommitRequest) (CommitResult, error) {
	if !t.supportsConditionalWrites() {
		return t.commitRevisionWithLock(ctx, req)
	}
	return t.commitRevisionCAS(ctx, req)
}

func (t *ObjectStoreTransport) commitRevisionCAS(ctx context.Context, req CommitRequest) (CommitResult, error) {
	head, headObjectRev, err := t.readHeadWithObjectRev(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	if head.CurrentRevision != req.BaseRevision {
		return CommitResult{}, ErrRevisionConflict
	}
	if err := t.ensureCommitObjects(ctx, req); err != nil {
		return CommitResult{}, err
	}
	revisionID := revisionIDForRequest(req)
	revision := Revision{SchemaVersion: RevisionSchemaVersion, RevisionID: revisionID, ParentRevisionID: req.BaseRevision, ManifestBlobID: req.ManifestBlobID, BlobIDs: append([]string(nil), req.BlobIDs...), CreatedAt: time.Now().UTC().Format(time.RFC3339), CreatedByDevice: req.DeviceID}
	if err := t.putJSON(ctx, t.layout.RevisionKey(revisionID), revision, remote.CreateIfAbsentRevision); err != nil {
		if errors.Is(err, remote.ErrConflict) {
			return CommitResult{}, ErrRevisionConflict
		}
		return CommitResult{}, err
	}
	newHead := Head{SchemaVersion: HeadSchemaVersion, VaultID: t.layout.VaultID, CurrentRevision: revisionID, ManifestBlobID: req.ManifestBlobID, UpdatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedByDevice: req.DeviceID}
	headBaseRev := headObjectRev
	if headBaseRev == "" {
		headBaseRev = remote.CreateIfAbsentRevision
	}
	if err := t.putJSON(ctx, t.layout.HeadKey(), newHead, headBaseRev); err != nil {
		if errors.Is(err, remote.ErrConflict) {
			return CommitResult{}, ErrRevisionConflict
		}
		return CommitResult{}, err
	}
	return CommitResult{RevisionID: revisionID, ManifestBlobID: req.ManifestBlobID, RemoteWrite: true}, nil
}

func (t *ObjectStoreTransport) commitRevisionWithLock(ctx context.Context, req CommitRequest) (CommitResult, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = "pinax-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	lock := Lock{DeviceID: req.DeviceID, RequestID: requestID, ExpiresAt: time.Now().UTC().Add(2 * time.Minute)}
	if err := t.acquireObjectLock(ctx, lock); err != nil {
		return CommitResult{}, err
	}
	defer t.releaseObjectLock(ctx, lock)

	head, _, err := t.readHeadWithObjectRev(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	if head.CurrentRevision != req.BaseRevision {
		return CommitResult{}, ErrRevisionConflict
	}
	if err := t.ensureCommitObjects(ctx, req); err != nil {
		return CommitResult{}, err
	}
	revisionID := revisionIDForRequest(req)
	if _, err := t.store.Stat(ctx, t.layout.RevisionKey(revisionID)); err == nil {
		return CommitResult{}, ErrRevisionConflict
	} else if err != nil && !errors.Is(err, remote.ErrObjectNotFound) {
		return CommitResult{}, err
	}
	revision := Revision{SchemaVersion: RevisionSchemaVersion, RevisionID: revisionID, ParentRevisionID: req.BaseRevision, ManifestBlobID: req.ManifestBlobID, BlobIDs: append([]string(nil), req.BlobIDs...), CreatedAt: time.Now().UTC().Format(time.RFC3339), CreatedByDevice: req.DeviceID}
	if err := t.putJSON(ctx, t.layout.RevisionKey(revisionID), revision, ""); err != nil {
		return CommitResult{}, err
	}
	if err := t.verifyObjectLock(ctx, lock); err != nil {
		return CommitResult{}, err
	}
	head, _, err = t.readHeadWithObjectRev(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	if head.CurrentRevision != req.BaseRevision {
		return CommitResult{}, ErrRevisionConflict
	}
	newHead := Head{SchemaVersion: HeadSchemaVersion, VaultID: t.layout.VaultID, CurrentRevision: revisionID, ManifestBlobID: req.ManifestBlobID, UpdatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedByDevice: req.DeviceID}
	if err := t.putJSON(ctx, t.layout.HeadKey(), newHead, ""); err != nil {
		return CommitResult{}, err
	}
	return CommitResult{RevisionID: revisionID, ManifestBlobID: req.ManifestBlobID, RemoteWrite: true}, nil
}

func (t *ObjectStoreTransport) supportsConditionalWrites() bool {
	capability, ok := t.store.(remote.ConditionalWriteCapability)
	return ok && capability.SupportsConditionalWrites()
}

func revisionIDForRequest(req CommitRequest) string {
	if strings.TrimSpace(req.RevisionID) != "" {
		return strings.TrimSpace(req.RevisionID)
	}
	return "rev_" + time.Now().UTC().Format("20060102150405.000000000")
}

func (t *ObjectStoreTransport) ensureCommitObjects(ctx context.Context, req CommitRequest) error {
	if _, err := t.GetManifest(ctx, req.ManifestBlobID); err != nil {
		return err
	}
	for _, blobID := range req.BlobIDs {
		if _, err := t.GetBlob(ctx, blobID); err != nil {
			return err
		}
	}
	return nil
}

func (t *ObjectStoreTransport) acquireObjectLock(ctx context.Context, lock Lock) error {
	now := time.Now().UTC()
	existing, err := t.readObjectLock(ctx)
	if err == nil && now.Before(existing.ExpiresAt) && existing.RequestID != lock.RequestID {
		return ErrLockHeld
	}
	if err != nil && !errors.Is(err, remote.ErrObjectNotFound) {
		return err
	}
	if err == nil && !now.Before(existing.ExpiresAt) {
		_ = t.store.Delete(ctx, t.layout.LockKey())
	}
	if err := t.putJSON(ctx, t.layout.LockKey(), lock, ""); err != nil {
		return err
	}
	return t.verifyObjectLock(ctx, lock)
}

func (t *ObjectStoreTransport) verifyObjectLock(ctx context.Context, lock Lock) error {
	existing, err := t.readObjectLock(ctx)
	if err != nil {
		return err
	}
	if existing.RequestID != lock.RequestID || existing.DeviceID != lock.DeviceID {
		return ErrLockHeld
	}
	if time.Now().UTC().After(existing.ExpiresAt) {
		return ErrLockHeld
	}
	return nil
}

func (t *ObjectStoreTransport) readObjectLock(ctx context.Context) (Lock, error) {
	data, _, err := t.store.Get(ctx, t.layout.LockKey())
	if err != nil {
		return Lock{}, err
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func (t *ObjectStoreTransport) releaseObjectLock(ctx context.Context, lock Lock) {
	if err := t.verifyObjectLock(ctx, lock); err != nil {
		return
	}
	_ = t.store.Delete(ctx, t.layout.LockKey())
}

func (t *ObjectStoreTransport) readHeadWithObjectRev(ctx context.Context) (Head, string, error) {
	data, rev, err := t.store.Get(ctx, t.layout.HeadKey())
	if errors.Is(err, remote.ErrObjectNotFound) {
		return Head{SchemaVersion: HeadSchemaVersion, VaultID: t.layout.VaultID}, "", nil
	}
	if err != nil {
		return Head{}, "", err
	}
	var head Head
	if err := json.Unmarshal(data, &head); err != nil {
		return Head{}, "", err
	}
	return head, rev, nil
}

func (t *ObjectStoreTransport) putEnvelope(ctx context.Context, key string, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	return t.putJSON(ctx, key, envelope, "")
}

func (t *ObjectStoreTransport) getEnvelope(ctx context.Context, key string) (Envelope, error) {
	data, _, err := t.store.Get(ctx, key)
	if errors.Is(err, remote.ErrObjectNotFound) {
		return Envelope{}, ErrObjectNotFound
	}
	if err != nil {
		return Envelope{}, err
	}
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, err
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (t *ObjectStoreTransport) putJSON(ctx context.Context, key string, value any, baseRev string) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if _, err := t.store.Put(ctx, key, append(data, '\n'), baseRev); err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}
