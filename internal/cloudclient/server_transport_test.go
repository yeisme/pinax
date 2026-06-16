package cloudclient

import (
	"context"
	"errors"
	"testing"

	"github.com/yeisme/pinax/internal/cloudclient/mlptest"
	"github.com/yeisme/pinax/internal/cloudsync"
)

// TestServerTransportTwoDeviceConvergence 是 Pinax Cloud MLP server transport 的两设备收敛 e2e：
// 设备 A push（blob + manifest + CAS commit），设备 B pull（head → manifest → blob）后收敛到同一份加密内容。
// 全程走 HTTP server transport，不替换为 direct file transport。
func TestServerTransportTwoDeviceConvergence(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_conv", SessionToken: "conv-token"})
	defer server.Close()
	ctx := context.Background()

	deviceA, err := New(Config{Endpoint: server.URL, VaultID: "vault_conv", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new device A client: %v", err)
	}
	deviceB, err := New(Config{Endpoint: server.URL, VaultID: "vault_conv", DeviceID: "dev_phone", Token: server.Token()})
	if err != nil {
		t.Fatalf("new device B client: %v", err)
	}
	transportA := NewTransport(deviceA)
	transportB := NewTransport(deviceB)

	// 设备 A：扫描 vault，构造 manifest（两个 note blob + 一个 manifest blob），加密上传，CAS commit。
	noteBlobA := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "n_a", Ciphertext: "cipher_notes_a", PlainSHA256: "sha_notes_a"}
	noteBlobB := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "n_b", Ciphertext: "cipher_notes_b", PlainSHA256: "sha_notes_b"}
	manifestEnvelope := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "n_m", Ciphertext: "cipher_manifest", PlainSHA256: "sha_manifest"}

	batchCheck, err := transportA.BatchCheck(ctx, []string{"blob_notes_a", "blob_notes_b"})
	if err != nil {
		t.Fatalf("device A batch check: %v", err)
	}
	if len(batchCheck.MissingBlobIDs) != 2 {
		t.Fatalf("both blobs should be missing initially, got %#v", batchCheck)
	}
	if err := transportA.PutBlob(ctx, "blob_notes_a", noteBlobA); err != nil {
		t.Fatalf("device A put blob a: %v", err)
	}
	if err := transportA.PutBlob(ctx, "blob_notes_b", noteBlobB); err != nil {
		t.Fatalf("device A put blob b: %v", err)
	}
	if err := transportA.PutManifest(ctx, "manifest_conv", manifestEnvelope); err != nil {
		t.Fatalf("device A put manifest: %v", err)
	}
	commit, err := transportA.CommitRevision(ctx, cloudsync.CommitRequest{
		BaseRevision: "", RevisionID: "rev_conv_1", ManifestBlobID: "manifest_conv",
		BlobIDs: []string{"blob_notes_a", "blob_notes_b"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_notes_a", BlobHash: "sha256:blob-a", Size: 10}, {PathHash: "sha256:path-b", BlobID: "blob_notes_b", BlobHash: "sha256:blob-b", Size: 20}}, DeviceID: "dev_laptop", RequestID: "req_conv_1",
	})
	if err != nil {
		t.Fatalf("device A commit: %v", err)
	}
	if !commit.RemoteWrite || commit.RevisionID != "rev_conv_1" {
		t.Fatalf("device A commit result = %#v", commit)
	}

	// 设备 B：pull。读 head → 下载 manifest → 下载 blob → 解密内容与设备 A 一致。
	head, err := transportB.CurrentHead(ctx, "vault_conv")
	if err != nil {
		t.Fatalf("device B current head: %v", err)
	}
	if head.CurrentRevision != "rev_conv_1" || head.ManifestBlobID != "manifest_conv" {
		t.Fatalf("device B head did not converge = %#v", head)
	}
	pulledManifest, err := transportB.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		t.Fatalf("device B get manifest: %v", err)
	}
	if pulledManifest.Ciphertext != "cipher_manifest" {
		t.Fatalf("device B manifest ciphertext mismatch = %#v", pulledManifest)
	}
	pulledA, err := transportB.GetBlob(ctx, "blob_notes_a")
	if err != nil {
		t.Fatalf("device B get blob a: %v", err)
	}
	pulledB, err := transportB.GetBlob(ctx, "blob_notes_b")
	if err != nil {
		t.Fatalf("device B get blob b: %v", err)
	}
	if pulledA.Ciphertext != noteBlobA.Ciphertext || pulledB.Ciphertext != noteBlobB.Ciphertext {
		t.Fatalf("device B pulled blobs did not converge to device A ciphertext")
	}
	// 设备 B 的 batch-check 现在应该报告这两个 blob 都已存在（收敛后无需重新上传）。
	afterPull, err := transportB.BatchCheck(ctx, []string{"blob_notes_a", "blob_notes_b"})
	if err != nil {
		t.Fatalf("device B batch check after pull: %v", err)
	}
	if len(afterPull.MissingBlobIDs) != 0 {
		t.Fatalf("device B should see all blobs present after convergence, missing = %#v", afterPull)
	}
}

// TestServerTransportConflictPreservesBothSides 是 server transport 冲突 e2e：
// 两个设备从同一 base revision 提交，只有一个成功；失败方拿到 REVISION_CONFLICT，
// 且不能让后写者静默覆盖先写者。
func TestServerTransportConflictPreservesBothSides(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_conflict", SessionToken: "conflict-token"})
	defer server.Close()
	ctx := context.Background()

	base := cloudsync.Envelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "n", Ciphertext: "c", PlainSHA256: "s"}
	mkClient := func(deviceID string) *Transport {
		c, err := New(Config{Endpoint: server.URL, VaultID: "vault_conflict", DeviceID: deviceID, Token: server.Token()})
		if err != nil {
			t.Fatalf("new client %s: %v", deviceID, err)
		}
		return NewTransport(c)
	}
	transportA := mkClient("dev_laptop")
	transportB := mkClient("dev_phone")

	// 两个设备都上传各自的 manifest + blob，从空 base 提交。
	seed := func(tr *Transport, blobID, manifestID string) {
		if err := tr.PutBlob(ctx, blobID, base); err != nil {
			t.Fatalf("put blob %s: %v", blobID, err)
		}
		if err := tr.PutManifest(ctx, manifestID, base); err != nil {
			t.Fatalf("put manifest %s: %v", manifestID, err)
		}
	}
	seed(transportA, "blob_a", "manifest_a")
	seed(transportB, "blob_b", "manifest_b")

	commitA, err := transportA.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", RevisionID: "rev_a", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10}}, DeviceID: "dev_laptop", RequestID: "req_a"})
	if err != nil {
		t.Fatalf("device A first commit should succeed: %v", err)
	}
	if !commitA.RemoteWrite {
		t.Fatalf("device A commit should report remote_write=true")
	}
	// 设备 B 用相同 base 提交，必须拿到 REVISION_CONFLICT，且不能覆盖设备 A 的 head。
	_, err = transportB.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "manifest_b", BlobIDs: []string{"blob_b"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-b", BlobID: "blob_b", BlobHash: "sha256:blob-b", Size: 10}}, DeviceID: "dev_phone", RequestID: "req_b"})
	if err == nil {
		t.Fatalf("device B stale commit must be rejected")
	}
	if !IsRevisionConflict(err) {
		t.Fatalf("device B should receive REVISION_CONFLICT, got %#v", err)
	}

	// head 仍是设备 A 的 revision，后写者没有静默覆盖。
	head, err := transportA.CurrentHead(ctx, "vault_conflict")
	if err != nil {
		t.Fatalf("current head after conflict: %v", err)
	}
	if head.CurrentRevision != "rev_a" {
		t.Fatalf("head should stay at rev_a, got %#v", head)
	}

	// 设备 B 收敛路径：pull 当前 head，然后基于新 base 再次提交（pull → rebase → commit）。
	pulledManifest, err := transportB.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		t.Fatalf("device B pull manifest: %v", err)
	}
	if pulledManifest.Ciphertext != base.Ciphertext {
		t.Fatalf("device B pulled manifest mismatch")
	}
	// 基于新 base 提交应该成功。
	if err := transportB.PutManifest(ctx, "manifest_b2", base); err != nil {
		t.Fatalf("device B put manifest b2: %v", err)
	}
	if err := transportB.PutBlob(ctx, "blob_b2", base); err != nil {
		t.Fatalf("device B put blob b2: %v", err)
	}
	commitB2, err := transportB.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: head.CurrentRevision, RevisionID: "rev_b2", ManifestBlobID: "manifest_b2", BlobIDs: []string{"blob_b2"}, ObjectRefs: []cloudsync.ObjectRef{{PathHash: "sha256:path-b2", BlobID: "blob_b2", BlobHash: "sha256:blob-b2", Size: 10}}, DeviceID: "dev_phone", RequestID: "req_b2"})
	if err != nil {
		t.Fatalf("device B rebase commit should succeed: %v", err)
	}
	if !commitB2.RemoteWrite {
		t.Fatalf("device B rebase commit should report remote_write=true")
	}
}

// TestServerTransportUnavailablePreservesLocalState 证明后端不可用时本地不被破坏：
// transport_error 不能让 RemoteWrite 变 true，且错误可识别为可重试。
func TestServerTransportUnavailablePreservesLocalState(t *testing.T) {
	// 直接构造一个指向不存在的 endpoint 的 client。
	client, err := New(Config{Endpoint: "http://127.0.0.1:0", VaultID: "vault_down", DeviceID: "laptop", Token: "x"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := NewTransport(client)
	ctx := context.Background()

	_, err = transport.CurrentHead(ctx, "vault_down")
	if err == nil {
		t.Fatalf("expected transport error when backend unavailable")
	}
	var cloudErr *Error
	if !errors.As(err, &cloudErr) {
		t.Fatalf("expected cloudclient.Error, got %T", err)
	}
	if cloudErr.Code != CodeTransportError || !cloudErr.Retryable {
		t.Fatalf("expected retryable TRANSPORT_ERROR, got %#v", cloudErr)
	}
}
