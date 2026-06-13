package cloudclient

import (
	"context"

	"github.com/yeisme/pinax/internal/cloudsync"
)

type Transport struct {
	client  *Client
	vaultID string
}

func NewTransport(client *Client, vaultID string) *Transport {
	return &Transport{client: client, vaultID: vaultID}
}

func (t *Transport) CurrentHead(ctx context.Context, vaultID string) (cloudsync.Head, error) {
	revision, err := t.client.CurrentRevision(ctx)
	if err != nil {
		return cloudsync.Head{}, err
	}
	if vaultID == "" {
		vaultID = t.vaultID
	}
	return cloudsync.Head{SchemaVersion: cloudsync.HeadSchemaVersion, VaultID: vaultID, CurrentRevision: revision.RevisionID, ManifestBlobID: revision.ManifestBlobID}, nil
}

func (t *Transport) BatchCheck(ctx context.Context, blobIDs []string) (cloudsync.BatchCheckResult, error) {
	result, err := t.client.BatchCheckBlobs(ctx, blobIDs)
	if err != nil {
		return cloudsync.BatchCheckResult{}, err
	}
	return cloudsync.BatchCheckResult{MissingBlobIDs: result.MissingBlobIDs}, nil
}

func (t *Transport) PutBlob(ctx context.Context, blobID string, envelope cloudsync.Envelope) error {
	return t.client.UploadBlob(ctx, blobID, toBlobEnvelope(envelope))
}

func (t *Transport) GetBlob(ctx context.Context, blobID string) (cloudsync.Envelope, error) {
	envelope, err := t.client.DownloadBlob(ctx, blobID)
	if err != nil {
		return cloudsync.Envelope{}, err
	}
	return fromBlobEnvelope(envelope), nil
}

func (t *Transport) PutManifest(ctx context.Context, blobID string, envelope cloudsync.Envelope) error {
	return t.PutBlob(ctx, blobID, envelope)
}

func (t *Transport) GetManifest(ctx context.Context, blobID string) (cloudsync.Envelope, error) {
	return t.GetBlob(ctx, blobID)
}

func (t *Transport) CommitRevision(ctx context.Context, req cloudsync.CommitRequest) (cloudsync.CommitResult, error) {
	result, err := t.client.CommitRevision(ctx, CommitRequest{BaseRevision: req.BaseRevision, RevisionID: req.RevisionID, ManifestBlobID: req.ManifestBlobID, BlobIDs: req.BlobIDs, DeviceID: req.DeviceID, IdempotencyKey: req.RequestID})
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	return cloudsync.CommitResult{RevisionID: result.RevisionID, ManifestBlobID: result.ManifestBlobID, RemoteWrite: true}, nil
}

func toBlobEnvelope(envelope cloudsync.Envelope) BlobEnvelope {
	return BlobEnvelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func fromBlobEnvelope(envelope BlobEnvelope) cloudsync.Envelope {
	return cloudsync.Envelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}
