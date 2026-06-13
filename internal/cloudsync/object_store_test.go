package cloudsync

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/remote"
)

func TestObjectStoreTransportCommitsHeadWithCAS(t *testing.T) {
	ctx := context.Background()
	store, err := remote.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("file backend: %v", err)
	}
	transport := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	envelope := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := transport.PutManifest(ctx, "manifest_a", envelope); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	commit, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_1"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !commit.RemoteWrite || commit.RevisionID != "rev_a" {
		t.Fatalf("commit = %#v", commit)
	}
	head, err := transport.CurrentHead(ctx, "vault_abc")
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.CurrentRevision != "rev_a" || head.ManifestBlobID != "manifest_a" {
		t.Fatalf("head = %#v", head)
	}
	_, err = transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "phone", RequestID: "req_2"})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale commit error = %v", err)
	}
}
func TestObjectStoreTransportRejectsConcurrentFirstCommits(t *testing.T) {
	ctx := context.Background()
	store, err := remote.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("file backend: %v", err)
	}
	first := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	second := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	envelope := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	for _, transport := range []*ObjectStoreTransport{first, second} {
		if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
			t.Fatalf("put blob: %v", err)
		}
		if err := transport.PutManifest(ctx, "manifest_a", envelope); err != nil {
			t.Fatalf("put manifest: %v", err)
		}
	}
	if _, err := first.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop"}); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if _, err := second.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "phone"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("second first commit error = %v", err)
	}
}

func TestObjectStoreTransportRejectsDuplicateRevisionID(t *testing.T) {
	ctx := context.Background()
	store, err := remote.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("file backend: %v", err)
	}
	transport := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	envelope := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := transport.PutManifest(ctx, "manifest_a", envelope); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	if _, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_same", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop"}); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if _, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "rev_same", RevisionID: "rev_same", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "phone"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("duplicate revision commit error = %v", err)
	}
}

func TestObjectStoreTransportLockFallbackRejectsConcurrentFirstHeadCreation(t *testing.T) {
	ctx := context.Background()
	store := newNonConditionalStore(t)
	first := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	second := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, first, "blob_a", "manifest_a")
	seedTransport(t, ctx, second, "blob_a", "manifest_a")

	errs := commitBoth(ctx, first, second, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_a"}, CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "phone", RequestID: "req_b"})
	assertOneSuccessOneConflict(t, errs)
}

func TestObjectStoreTransportLockFallbackRejectsSameBaseConcurrentUpdate(t *testing.T) {
	ctx := context.Background()
	store := newNonConditionalStore(t)
	base := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, base, "blob_a", "manifest_a")
	if _, err := base.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_a"}); err != nil {
		t.Fatalf("base commit: %v", err)
	}
	first := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	second := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, first, "blob_b", "manifest_b")
	seedTransport(t, ctx, second, "blob_c", "manifest_c")

	errs := commitBoth(ctx, first, second, CommitRequest{BaseRevision: "rev_a", RevisionID: "rev_b", ManifestBlobID: "manifest_b", BlobIDs: []string{"blob_b"}, DeviceID: "laptop", RequestID: "req_b"}, CommitRequest{BaseRevision: "rev_a", RevisionID: "rev_c", ManifestBlobID: "manifest_c", BlobIDs: []string{"blob_c"}, DeviceID: "phone", RequestID: "req_c"})
	assertOneSuccessOneConflict(t, errs)
}

func TestObjectStoreTransportLockFallbackRecoversExpiredLock(t *testing.T) {
	ctx := context.Background()
	store := newNonConditionalStore(t)
	transport := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, transport, "blob_a", "manifest_a")
	stale := Lock{DeviceID: "dead", RequestID: "old", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := transport.putJSON(ctx, transport.layout.LockKey(), stale, ""); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	commit, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_a"})
	if err != nil {
		t.Fatalf("commit after stale lock: %v", err)
	}
	if !commit.RemoteWrite {
		t.Fatalf("commit did not report durable write: %#v", commit)
	}
}

func TestObjectStoreTransportLockFallbackReturnsLockHeld(t *testing.T) {
	ctx := context.Background()
	store := newNonConditionalStore(t)
	transport := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, transport, "blob_a", "manifest_a")
	held := Lock{DeviceID: "other", RequestID: "other_req", ExpiresAt: time.Now().Add(time.Minute)}
	if err := transport.putJSON(ctx, transport.layout.LockKey(), held, ""); err != nil {
		t.Fatalf("write held lock: %v", err)
	}
	_, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_a"})
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("commit with held lock err = %v", err)
	}
}

func TestObjectStoreTransportUnknownConditionalCapabilityUsesLockFallback(t *testing.T) {
	ctx := context.Background()
	store := newUnknownConditionalStore(t)
	transport := NewObjectStoreTransport(store, Layout{Prefix: "pinax-sync", WorkspaceID: "personal", VaultID: "vault_abc"})
	seedTransport(t, ctx, transport, "blob_a", "manifest_a")
	held := Lock{DeviceID: "other", RequestID: "other_req", ExpiresAt: time.Now().Add(time.Minute)}
	if err := transport.putJSON(ctx, transport.layout.LockKey(), held, ""); err != nil {
		t.Fatalf("write held lock: %v", err)
	}
	_, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_a"})
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("unknown conditional capability should use lock fallback, err = %v", err)
	}
}

type unknownConditionalStore struct{ backend *remote.FileBackend }

func newUnknownConditionalStore(t *testing.T) *unknownConditionalStore {
	t.Helper()
	backend, err := remote.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("file backend: %v", err)
	}
	return &unknownConditionalStore{backend: backend}
}

func (s *unknownConditionalStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	return s.backend.Get(ctx, key)
}

func (s *unknownConditionalStore) Put(ctx context.Context, key string, data []byte, baseRev string) (string, error) {
	return s.backend.Put(ctx, key, data, baseRev)
}

func (s *unknownConditionalStore) Stat(ctx context.Context, key string) (string, error) {
	return s.backend.Stat(ctx, key)
}

func (s *unknownConditionalStore) Delete(ctx context.Context, key string) error {
	return s.backend.Delete(ctx, key)
}

type nonConditionalStore struct{ *remote.FileBackend }

func newNonConditionalStore(t *testing.T) *nonConditionalStore {
	t.Helper()
	backend, err := remote.NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("file backend: %v", err)
	}
	return &nonConditionalStore{FileBackend: backend}
}

func (s *nonConditionalStore) SupportsConditionalWrites() bool { return false }

func (s *nonConditionalStore) Put(ctx context.Context, key string, data []byte, _ string) (string, error) {
	return s.FileBackend.Put(ctx, key, data, "")
}

func seedTransport(t *testing.T, ctx context.Context, transport *ObjectStoreTransport, blobID, manifestID string) {
	t.Helper()
	envelope := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := transport.PutBlob(ctx, blobID, envelope); err != nil {
		t.Fatalf("put blob %s: %v", blobID, err)
	}
	if err := transport.PutManifest(ctx, manifestID, envelope); err != nil {
		t.Fatalf("put manifest %s: %v", manifestID, err)
	}
}

func commitBoth(ctx context.Context, first, second *ObjectStoreTransport, firstReq, secondReq CommitRequest) []error {
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, item := range []struct {
		transport *ObjectStoreTransport
		req       CommitRequest
	}{{first, firstReq}, {second, secondReq}} {
		wg.Add(1)
		go func(transport *ObjectStoreTransport, req CommitRequest) {
			defer wg.Done()
			result, err := transport.CommitRevision(ctx, req)
			if err == nil && !result.RemoteWrite {
				err = errors.New("commit returned remote_write=false")
			}
			errCh <- err
		}(item.transport, item.req)
	}
	wg.Wait()
	close(errCh)
	errs := make([]error, 0, 2)
	for err := range errCh {
		errs = append(errs, err)
	}
	return errs
}

func assertOneSuccessOneConflict(t *testing.T, errs []error) {
	t.Helper()
	successes := 0
	conflicts := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRevisionConflict) || errors.Is(err, ErrLockHeld):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success/conflict = %d/%d from %#v", successes, conflicts, errs)
	}
}
