package cloudsync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestObjectKeysNeverContainPlaintextPath(t *testing.T) {
	layout := Layout{Prefix: "pinax-sync/", WorkspaceID: "personal", VaultID: "vault_abc"}
	blobKey := layout.BlobKey("blob_7af3deadbeef")
	manifestKey := layout.ManifestKey("manifest_429cdeadbeef")
	for _, key := range []string{blobKey, manifestKey, layout.HeadKey(), layout.LockKey()} {
		if !strings.HasPrefix(key, "pinax-sync/workspaces/personal/vaults/vault_abc/") && key != "pinax-sync/protocol.json" {
			t.Fatalf("key missing namespace: %s", key)
		}
		for _, leaked := range []string{"notes/", "alpha.md", "Authorization", "token"} {
			if strings.Contains(key, leaked) {
				t.Fatalf("key leaked %q: %s", leaked, key)
			}
		}
	}
	if got := layout.ProtocolKey(); got != "pinax-sync/protocol.json" {
		t.Fatalf("protocol key = %s", got)
	}
}

func TestEnvelopeValidationRejectsPlaintextAndMissingCiphertext(t *testing.T) {
	valid := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	invalid := valid
	invalid.Ciphertext = ""
	if err := invalid.Validate(); err == nil {
		t.Fatalf("missing ciphertext accepted")
	}
	invalid = valid
	invalid.Metadata = map[string]string{"path": "notes/alpha.md"}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("plaintext path metadata accepted")
	}
}
func TestManifestConflictDomainTypesRejectPlaintextMetadata(t *testing.T) {
	manifest := Manifest{SchemaVersion: ManifestSchemaVersion, Entries: []ManifestEntry{{Path: "notes/alpha.md", BlobID: "blob_abcd", PlainSHA256: "sha", Size: 12, UpdatedAt: "2026-06-12T00:00:00Z"}}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if got := manifest.BlobIDs(); len(got) != 1 || got[0] != "blob_abcd" {
		t.Fatalf("manifest blob ids = %#v", got)
	}
	invalid := manifest
	invalid.Entries[0].BlobID = "notes/alpha.md"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("manifest accepted plaintext path as blob id")
	}
	conflict := Conflict{SchemaVersion: ConflictSchemaVersion, PathHash: "pathhash_abc", LocalBlobID: "blob_local", RemoteBlobID: "blob_remote", BaseRevisionID: "rev_base", RemoteRevisionID: "rev_remote"}
	if err := conflict.Validate(); err != nil {
		t.Fatalf("valid conflict rejected: %v", err)
	}
	conflict.PathHash = "notes/alpha.md"
	if err := conflict.Validate(); err == nil {
		t.Fatalf("conflict accepted plaintext path hash")
	}
}

func TestMemoryTransportCommitRevisionUsesCAS(t *testing.T) {
	ctx := context.Background()
	transport := NewMemoryTransport(Layout{WorkspaceID: "personal", VaultID: "vault_abc"})
	envelope := Envelope{SchemaVersion: EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := transport.PutManifest(ctx, "manifest_a", envelope); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	commit, err := transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_1"})
	if err != nil {
		t.Fatalf("commit rev_a: %v", err)
	}
	if !commit.RemoteWrite || commit.RevisionID != "rev_a" {
		t.Fatalf("commit result = %#v", commit)
	}
	_, err = transport.CommitRevision(ctx, CommitRequest{BaseRevision: "", RevisionID: "rev_stale", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "phone", RequestID: "req_2"})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale commit error = %v", err)
	}
	head, err := transport.CurrentHead(ctx, "vault_abc")
	if err != nil {
		t.Fatalf("current head: %v", err)
	}
	if head.CurrentRevision != "rev_a" || head.ManifestBlobID != "manifest_a" {
		t.Fatalf("head = %#v", head)
	}
}

func TestCommitLockExpiresAndCanBeReacquired(t *testing.T) {
	ctx := context.Background()
	transport := NewMemoryTransport(Layout{WorkspaceID: "personal", VaultID: "vault_abc"})
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	if err := transport.AcquireLock(ctx, Lock{DeviceID: "laptop", RequestID: "req_1", ExpiresAt: now.Add(time.Minute)}, now); err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if err := transport.AcquireLock(ctx, Lock{DeviceID: "phone", RequestID: "req_2", ExpiresAt: now.Add(time.Minute)}, now.Add(10*time.Second)); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected lock held, got %v", err)
	}
	if err := transport.AcquireLock(ctx, Lock{DeviceID: "phone", RequestID: "req_2", ExpiresAt: now.Add(3 * time.Minute)}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("reacquire expired lock: %v", err)
	}
}
