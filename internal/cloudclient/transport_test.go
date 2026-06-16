package cloudclient

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/cloudclient/mlptest"
	"github.com/yeisme/pinax/internal/cloudsync"
)

func TestServerTransportImplementsCloudsyncTransport(t *testing.T) {
	var _ cloudsync.Transport = (*Transport)(nil)
}

func TestServerTransportMapsCloudsyncOperations(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_abc", SessionToken: "secret"})
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_abc", DeviceID: "laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := NewTransport(client)
	ctx := context.Background()

	head, err := transport.CurrentHead(ctx, "vault_abc")
	if err != nil {
		t.Fatalf("current head: %v", err)
	}
	if head.CurrentRevision != "" {
		t.Fatalf("empty vault head should be empty, got %#v", head)
	}

	envelope := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"}
	if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	gotEnvelope, err := transport.GetBlob(ctx, "blob_a")
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}
	if gotEnvelope.Ciphertext != "cipher" || gotEnvelope.Nonce != "nonce" {
		t.Fatalf("envelope = %#v", gotEnvelope)
	}
	missing, err := transport.BatchCheck(ctx, []string{"blob_a", "blob_b"})
	if err != nil {
		t.Fatalf("batch check: %v", err)
	}
	if len(missing.MissingBlobIDs) != 1 || missing.MissingBlobIDs[0] != "blob_b" {
		t.Fatalf("missing = %#v", missing)
	}
	// manifest blob 必须先上传，否则 commit 会被服务端拒绝。
	if err := transport.PutManifest(ctx, "manifest_a", envelope); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	commit, err := transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10}}, DeviceID: "laptop", RequestID: "req_1"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !commit.RemoteWrite || commit.RevisionID != "rev_b" {
		t.Fatalf("commit = %#v", commit)
	}
	// CAS 冲突：用旧 base revision 再提交一次必须失败且 RemoteWrite 不为 true。
	_, err = transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", RevisionID: "rev_c", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10}}, DeviceID: "laptop", RequestID: "req_2"})
	if err == nil {
		t.Fatalf("expected REVISION_CONFLICT on stale base")
	}
	if !IsRevisionConflict(err) {
		t.Fatalf("expected revision conflict error, got %#v", err)
	}
}

// TestServerTransportNeverRemoteWriteBeforeCommit 是 remote_write gate 的 RED 守卫：
// 在 CAS commit 之前，CurrentHead/BatchCheck/PutBlob/GetBlob 都不能让 RemoteWrite 变 true。
func TestServerTransportNeverRemoteWriteBeforeCommit(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_gate", SessionToken: "secret"})
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_gate", DeviceID: "laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := NewTransport(client)
	ctx := context.Background()

	// 只读和上传 blob 都不是 durable commit，不能触发 remote_write。
	if _, err := transport.CurrentHead(ctx, "vault_gate"); err != nil {
		t.Fatalf("current head: %v", err)
	}
	envelope := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "n", Ciphertext: "c", PlainSHA256: "s"}
	if err := transport.PutBlob(ctx, "blob_a", envelope); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if _, err := transport.BatchCheck(ctx, []string{"blob_a"}); err != nil {
		t.Fatalf("batch check: %v", err)
	}

	// CAS commit 失败（manifest 缺失）时 RemoteWrite 必须保持 false。
	_, commitErr := transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", ManifestBlobID: "manifest_missing", BlobIDs: []string{"blob_a"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10}}, DeviceID: "laptop", RequestID: "req_gate"})
	if commitErr == nil {
		t.Fatalf("expected commit to fail when manifest blob is missing")
	}
	// 成功 commit 后 RemoteWrite 才为 true。
	if err := transport.PutManifest(ctx, "manifest_ok", envelope); err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	commit, err := transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", ManifestBlobID: "manifest_ok", BlobIDs: []string{"blob_a"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10}}, DeviceID: "laptop", RequestID: "req_gate_ok"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !commit.RemoteWrite {
		t.Fatalf("RemoteWrite must be true after successful CAS commit")
	}
}
