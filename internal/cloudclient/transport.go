package cloudclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/yeisme/pinax/internal/cloudsync"
)

// Transport 把 cloudclient.Client（MLP REST）适配为 cloudsync.Transport，
// 让 Pinax 同步引擎统一通过 server transport 跑 push/pull。
// remote_write 只在服务端 CAS commit 成功后才为 true（见 CommitRevision）。
type Transport struct {
	client *Client
}

func NewTransport(client *Client) *Transport {
	return &Transport{client: client}
}

func (t *Transport) CurrentHead(ctx context.Context, vaultID string) (cloudsync.Head, error) {
	revision, err := t.client.CurrentRevision(ctx)
	if err != nil {
		return cloudsync.Head{}, err
	}
	if vaultID == "" {
		vaultID = t.client.VaultID()
	}
	return cloudsync.Head{SchemaVersion: cloudsync.HeadSchemaVersion, VaultID: vaultID, CurrentRevision: revision.RevisionID, ManifestBlobID: revision.ManifestBlobID}, nil
}

func (t *Transport) BatchCheck(ctx context.Context, blobIDs []string) (cloudsync.BatchCheckResult, error) {
	result, err := t.client.BatchCheckBlobs(ctx, blobIDs)
	if err != nil {
		return cloudsync.BatchCheckResult{}, err
	}
	present := make([]cloudsync.BlobFact, 0, len(result.Present))
	for _, fact := range result.Present {
		present = append(present, cloudsync.BlobFact{BlobID: fact.BlobID, BlobHash: fact.BlobHash, Size: fact.Size})
	}
	return cloudsync.BatchCheckResult{MissingBlobIDs: result.MissingBlobIDs, Present: present}, nil
}

func (t *Transport) PutBlob(ctx context.Context, blobID string, envelope cloudsync.Envelope) error {
	return t.client.UploadBlob(ctx, blobID, toBlobEnvelope(envelope))
}

func (t *Transport) RegisterBlobMetadata(ctx context.Context, blobID, blobHash string, sizeBytes int64) error {
	_, err := t.client.SignUpload(ctx, blobID, blobHash, sizeBytes, "application/vnd.pinax.encrypted-envelope+json")
	return err
}

func (t *Transport) PutBlobWithMetadata(ctx context.Context, blobID, blobHash string, sizeBytes int64, envelope cloudsync.Envelope) error {
	if err := t.RegisterBlobMetadata(ctx, blobID, blobHash, sizeBytes); err != nil {
		return err
	}
	return t.client.UploadBlob(ctx, blobID, toBlobEnvelope(envelope))
}

func (t *Transport) PutBlobWithEnvelopeMetadata(ctx context.Context, blobID string, envelope cloudsync.Envelope) (string, int64, error) {
	blobHash, sizeBytes, err := envelopeHashAndSize(envelope)
	if err != nil {
		return "", 0, err
	}
	if err := t.RegisterBlobMetadata(ctx, blobID, blobHash, sizeBytes); err != nil {
		return "", 0, err
	}
	if err := t.client.UploadBlob(ctx, blobID, toBlobEnvelope(envelope)); err != nil {
		return "", 0, err
	}
	return blobHash, sizeBytes, nil
}

func (t *Transport) GetBlob(ctx context.Context, blobID string) (cloudsync.Envelope, error) {
	envelope, err := t.client.DownloadBlob(ctx, blobID)
	if err != nil {
		return cloudsync.Envelope{}, err
	}
	return fromBlobEnvelope(envelope), nil
}

func (t *Transport) PutManifest(ctx context.Context, blobID string, envelope cloudsync.Envelope) error {
	_, _, err := t.PutBlobWithEnvelopeMetadata(ctx, blobID, envelope)
	return err
}

func (t *Transport) GetManifest(ctx context.Context, blobID string) (cloudsync.Envelope, error) {
	return t.GetBlob(ctx, blobID)
}

// CommitRevision 只在服务端 CAS commit 成功后返回 RemoteWrite=true。
// 服务端返回 REVISION_CONFLICT 时透传错误，RemoteWrite 保持 false。
func (t *Transport) CommitRevision(ctx context.Context, req cloudsync.CommitRequest) (cloudsync.CommitResult, error) {
	objectRefs := make([]ObjectRef, 0, len(req.ObjectRefs))
	for _, ref := range req.ObjectRefs {
		objectRefs = append(objectRefs, ObjectRef{PathHash: ref.PathHash, BlobID: ref.BlobID, BlobHash: ref.BlobHash, Size: ref.Size, SizeBytes: ref.Size, Deleted: ref.Deleted})
	}
	result, err := t.client.CommitRevision(ctx, CommitRequest{BaseRevision: req.BaseRevision, RevisionID: req.RevisionID, ManifestBlobID: req.ManifestBlobID, ObjectRefs: objectRefs, DeviceID: req.DeviceID, IdempotencyKey: req.RequestID})
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	return cloudsync.CommitResult{RevisionID: result.RevisionID, ManifestBlobID: result.ManifestBlobID, RemoteWrite: true}, nil
}

// IsRevisionConflict 判断错误是否为服务端 CAS 冲突（REVISION_CONFLICT）。
func IsRevisionConflict(err error) bool {
	var cloudErr *Error
	if errors.As(err, &cloudErr) {
		return cloudErr.Code == CodeRevisionConflict
	}
	return false
}

func toBlobEnvelope(envelope cloudsync.Envelope) BlobEnvelope {
	return BlobEnvelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func fromBlobEnvelope(envelope BlobEnvelope) cloudsync.Envelope {
	return cloudsync.Envelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func envelopeHashAndSize(envelope cloudsync.Envelope) (string, int64, error) {
	raw, err := json.Marshal(toBlobEnvelope(envelope))
	if err != nil {
		return "", 0, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(compact.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:]), int64(compact.Len()), nil
}

func compactBlobEnvelopeHashAndSize(envelope BlobEnvelope) (string, int64, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", 0, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(compact.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:]), int64(compact.Len()), nil
}

func IsCode(err error, code string) bool {
	var cloudErr *Error
	if errors.As(err, &cloudErr) {
		return cloudErr.Code == code
	}
	return false
}
