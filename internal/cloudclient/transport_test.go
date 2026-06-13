package cloudclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yeisme/pinax/internal/cloudsync"
)

func TestServerTransportImplementsCloudsyncTransport(t *testing.T) {
	var _ cloudsync.Transport = (*Transport)(nil)
}

func TestServerTransportMapsCloudsyncOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workspaces/ws_123/revision":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision_id": "rev_a", "manifest_blob_id": "manifest_a"})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/workspaces/ws_123/blobs/blob_a":
			writeJSON(t, w, http.StatusCreated, map[string]any{"blob_id": "blob_a"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workspaces/ws_123/blobs/blob_a":
			writeJSON(t, w, http.StatusOK, BlobEnvelope{SchemaVersion: cloudsync.EnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "cipher", PlainSHA256: "sha"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workspaces/ws_123/blobs:batchCheck":
			writeJSON(t, w, http.StatusOK, map[string]any{"missing_blob_ids": []string{"blob_b"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workspaces/ws_123/revisions:commit":
			if got := r.Header.Get("Idempotency-Key"); got != "req_1" {
				t.Fatalf("idempotency key = %q", got)
			}
			var req CommitRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode commit: %v", err)
			}
			if req.RevisionID != "rev_b" || req.DeviceID != "laptop" || len(req.BlobIDs) != 1 || req.BlobIDs[0] != "blob_a" {
				t.Fatalf("commit request = %#v", req)
			}
			writeJSON(t, w, http.StatusOK, map[string]any{"revision_id": "rev_b", "manifest_blob_id": "manifest_a"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, WorkspaceID: "ws_123", DeviceID: "laptop", Token: "secret"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := NewTransport(client, "vault_abc")
	ctx := context.Background()
	head, err := transport.CurrentHead(ctx, "vault_abc")
	if err != nil {
		t.Fatalf("current head: %v", err)
	}
	if head.CurrentRevision != "rev_a" || head.ManifestBlobID != "manifest_a" {
		t.Fatalf("head = %#v", head)
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
	commit, err := transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: "rev_a", RevisionID: "rev_b", ManifestBlobID: "manifest_a", BlobIDs: []string{"blob_a"}, DeviceID: "laptop", RequestID: "req_1"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !commit.RemoteWrite || commit.RevisionID != "rev_b" {
		t.Fatalf("commit = %#v", commit)
	}
}
