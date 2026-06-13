package cloudclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientSendsAuthDeviceAndRequestHeaders(t *testing.T) {
	var gotAuth, gotDevice, gotRequestID string
	var gotCommit CommitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws_123/revisions:commit" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotDevice = r.Header.Get("X-Pinax-Device-ID")
		gotRequestID = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotCommit); err != nil {
			t.Fatalf("decode commit: %v", err)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{"revision_id": "rev_b", "manifest_blob_id": "blob_manifest"})
	}))
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, WorkspaceID: "ws_123", DeviceID: "dev_laptop", Token: "secret-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	res, err := client.CommitRevision(context.Background(), CommitRequest{BaseRevision: "rev_a", RevisionID: "rev_b", ManifestBlobID: "blob_manifest", BlobIDs: []string{"blob_a", "blob_b"}, DeviceID: "dev_laptop", IdempotencyKey: "req_123"})
	if err != nil {
		t.Fatalf("commit revision: %v", err)
	}
	if res.RevisionID != "rev_b" {
		t.Fatalf("revision response = %#v", res)
	}
	if gotAuth != "Bearer secret-token" || gotDevice != "dev_laptop" || gotRequestID != "req_123" {
		t.Fatalf("headers auth=%q device=%q request=%q", gotAuth, gotDevice, gotRequestID)
	}
	if gotCommit.RevisionID != "rev_b" || gotCommit.DeviceID != "dev_laptop" || len(gotCommit.BlobIDs) != 2 || gotCommit.BlobIDs[1] != "blob_b" {
		t.Fatalf("commit body = %#v", gotCommit)
	}
}

func TestClientReadsRevisionAndTransfersEncryptedBlobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workspaces/ws_123/revision":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision_id": "rev_a", "manifest_blob_id": "blob_manifest"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workspaces/ws_123/blobs:batchCheck":
			var req blobCheckRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch check: %v", err)
			}
			if len(req.BlobIDs) != 2 || req.BlobIDs[0] != "blob_a" || req.BlobIDs[1] != "blob_b" {
				t.Fatalf("batch check ids = %#v", req.BlobIDs)
			}
			writeJSON(t, w, http.StatusOK, map[string]any{"missing_blob_ids": []string{"blob_b"}})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/workspaces/ws_123/blobs/blob_b":
			var req BlobEnvelope
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode upload: %v", err)
			}
			if req.Ciphertext != "encrypted-note" || req.PlainSHA256 != "plain-sha" {
				t.Fatalf("uploaded envelope = %#v", req)
			}
			writeJSON(t, w, http.StatusCreated, map[string]any{"blob_id": "blob_b"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workspaces/ws_123/blobs/blob_b":
			writeJSON(t, w, http.StatusOK, BlobEnvelope{SchemaVersion: "pinax.cloud.envelope.v1", Alg: "AES-256-GCM", KeyID: "key_1", Ciphertext: "encrypted-note", PlainSHA256: "plain-sha"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, WorkspaceID: "ws_123", DeviceID: "dev_laptop", Token: "secret-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	revision, err := client.CurrentRevision(context.Background())
	if err != nil {
		t.Fatalf("current revision: %v", err)
	}
	if revision.RevisionID != "rev_a" || revision.ManifestBlobID != "blob_manifest" {
		t.Fatalf("revision = %#v", revision)
	}
	missing, err := client.BatchCheckBlobs(context.Background(), []string{"blob_a", "blob_b"})
	if err != nil {
		t.Fatalf("batch check: %v", err)
	}
	if len(missing.MissingBlobIDs) != 1 || missing.MissingBlobIDs[0] != "blob_b" {
		t.Fatalf("missing = %#v", missing)
	}
	envelope := BlobEnvelope{SchemaVersion: "pinax.cloud.envelope.v1", Alg: "AES-256-GCM", KeyID: "key_1", Ciphertext: "encrypted-note", PlainSHA256: "plain-sha"}
	if err := client.UploadBlob(context.Background(), "blob_b", envelope); err != nil {
		t.Fatalf("upload blob: %v", err)
	}
	down, err := client.DownloadBlob(context.Background(), "blob_b")
	if err != nil {
		t.Fatalf("download blob: %v", err)
	}
	if down.Ciphertext != "encrypted-note" || down.PlainSHA256 != "plain-sha" {
		t.Fatalf("downloaded envelope = %#v", down)
	}
}

func TestClientReturnsStableRedactedCloudErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusConflict, errorResponse{Error: ErrorBody{Code: "revision_conflict", Message: "base revision is stale", Retryable: true}})
	}))
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, WorkspaceID: "ws_123", DeviceID: "dev_laptop", Token: "secret-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.CommitRevision(context.Background(), CommitRequest{BaseRevision: "rev_a", ManifestBlobID: "blob_manifest", IdempotencyKey: "req_123"})
	if err == nil {
		t.Fatalf("commit unexpectedly succeeded")
	}
	cloudErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if cloudErr.Code != "revision_conflict" || cloudErr.StatusCode != http.StatusConflict || !cloudErr.Retryable {
		t.Fatalf("cloud error = %#v", cloudErr)
	}
	text := err.Error()
	for _, secret := range []string{"secret-token", "Authorization", "Bearer"} {
		if strings.Contains(text, secret) {
			t.Fatalf("error leaked %q: %s", secret, text)
		}
	}
}

func TestContractFixtureCoversMinimumCloudAPI(t *testing.T) {
	payload, err := os.ReadFile("testdata/cloud_contract.json")
	if err != nil {
		t.Fatalf("read contract fixture: %v", err)
	}
	var fixture struct {
		Operations []struct {
			Name   string `json:"name"`
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"operations"`
		ErrorCodes []string `json:"error_codes"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode contract fixture: %v", err)
	}
	wantOps := map[string]bool{"health": false, "current_revision": false, "blob_batch_check": false, "blob_upload": false, "blob_download": false, "revision_commit": false}
	for _, op := range fixture.Operations {
		if op.Method == "" || op.Path == "" {
			t.Fatalf("operation missing method/path: %#v", op)
		}
		if _, ok := wantOps[op.Name]; ok {
			wantOps[op.Name] = true
		}
	}
	for name, seen := range wantOps {
		if !seen {
			t.Fatalf("contract fixture missing operation %s", name)
		}
	}
	wantErrors := map[string]bool{"revision_conflict": false, "unauthorized": false, "backend_unavailable": false}
	for _, code := range fixture.ErrorCodes {
		if _, ok := wantErrors[code]; ok {
			wantErrors[code] = true
		}
	}
	for code, seen := range wantErrors {
		if !seen {
			t.Fatalf("contract fixture missing error code %s", code)
		}
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
