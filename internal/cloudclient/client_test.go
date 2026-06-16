package cloudclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/cloudclient/mlptest"
)

func TestClientSendsAuthDeviceAndRequestHeaders(t *testing.T) {
	var gotAuth, gotDevice, gotRequestID string
	var gotCommit CommitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vaults/vault_1/revisions" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
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

	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_1", DeviceID: "dev_laptop", Token: "secret-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	res, err := client.CommitRevision(context.Background(), CommitRequest{BaseRevision: "", RevisionID: "rev_b", ManifestBlobID: "blob_manifest", ObjectRefs: []ObjectRef{{PathHash: "sha256:path-a", BlobID: "blob_a", BlobHash: "sha256:blob-a", Size: 10, SizeBytes: 10}, {PathHash: "sha256:path-b", BlobID: "blob_b", BlobHash: "sha256:blob-b", Size: 20, SizeBytes: 20}}, DeviceID: "dev_laptop", IdempotencyKey: "req_123"})
	if err != nil {
		t.Fatalf("commit revision: %v", err)
	}
	if res.RevisionID != "rev_b" {
		t.Fatalf("revision response = %#v", res)
	}
	if gotAuth != "Bearer secret-token" || gotDevice != "dev_laptop" || gotRequestID != "req_123" {
		t.Fatalf("headers auth=%q device=%q request=%q", gotAuth, gotDevice, gotRequestID)
	}
	if gotCommit.RevisionID != "rev_b" || gotCommit.DeviceID != "dev_laptop" || len(gotCommit.ObjectRefs) != 2 || gotCommit.ObjectRefs[1].BlobID != "blob_b" {
		t.Fatalf("commit body = %#v", gotCommit)
	}
}

func TestClientBootstrapPrincipalAndVaultLifecycle(t *testing.T) {
	server := mlptest.New(mlptest.Config{BootstrapToken: "boot-secret", SessionToken: "session-token"})
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	// bootstrap 成功且返回脱敏事实。
	boot, err := client.Bootstrap(context.Background(), "boot-secret", "dev_laptop")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if boot.AccountID == "" || boot.DeviceID != "dev_laptop" || boot.VaultID == "" || boot.TokenRef != "profile://cloud" || boot.Scope != "sync" {
		t.Fatalf("bootstrap result = %#v", boot)
	}
	// bootstrap token mismatch 返回 UNAUTHENTICATED。
	if _, err := client.Bootstrap(context.Background(), "wrong", "dev_laptop"); err == nil {
		t.Fatalf("expected bootstrap auth error")
	} else if cloudErr, ok := err.(*Error); !ok || cloudErr.Code != CodeUnauthenticated {
		t.Fatalf("bootstrap error = %#v", err)
	}

	principal, err := client.CurrentPrincipal(context.Background())
	if err != nil {
		t.Fatalf("principal: %v", err)
	}
	if principal.AccountID == "" || principal.DeviceID != "dev_laptop" || principal.VaultID != boot.VaultID || principal.TokenRef != "profile://cloud" || principal.Scope != "sync" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestClientChangesBatchCheckSignUploadAndBlobTransfer(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_1", SessionToken: "secret-token"})
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_1", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	head, err := client.CurrentRevision(context.Background())
	if err != nil {
		t.Fatalf("current head: %v", err)
	}
	if head.RevisionID != "" {
		t.Fatalf("empty vault head should have empty revision, got %#v", head)
	}

	changes, err := client.Changes(context.Background(), "")
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if changes.HasMore {
		t.Fatalf("changes has_more should be false on empty vault")
	}

	missing, err := client.BatchCheckBlobs(context.Background(), []string{"blob_a", "blob_b"})
	if err != nil {
		t.Fatalf("batch check: %v", err)
	}
	if len(missing.MissingBlobIDs) != 2 {
		t.Fatalf("missing = %#v", missing)
	}

	envelope := BlobEnvelope{SchemaVersion: "pinax.cloud.envelope.v1", Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce-b", Ciphertext: "encrypted-note", PlainSHA256: "plain-sha"}
	blobHash := compactBlobEnvelopeHash(t, envelope)
	blobSize := int64(compactBlobEnvelopeSize(t, envelope))
	plan, err := client.SignUpload(context.Background(), "blob_b", blobHash, blobSize, "application/vnd.pinax.encrypted-envelope+json")
	if err != nil {
		t.Fatalf("sign upload: %v", err)
	}
	if plan.BlobID != "blob_b" || plan.ObjectKey == "" || plan.URL == "" {
		t.Fatalf("upload plan = %#v", plan)
	}
	if strings.Contains(plan.ObjectKey, "notes/") || strings.Contains(plan.ObjectKey, ".md") {
		t.Fatalf("upload plan object key leaked plaintext path: %s", plan.ObjectKey)
	}

	if err := client.UploadBlob(context.Background(), "blob_b", envelope); err != nil {
		t.Fatalf("upload blob: %v", err)
	}
	// 上传后该 blob 不再 missing。
	missingAfter, err := client.BatchCheckBlobs(context.Background(), []string{"blob_b"})
	if err != nil {
		t.Fatalf("batch check after upload: %v", err)
	}
	if len(missingAfter.MissingBlobIDs) != 0 {
		t.Fatalf("blob should be present after upload, missing = %#v", missingAfter)
	}
	down, err := client.DownloadBlob(context.Background(), "blob_b")
	if err != nil {
		t.Fatalf("download blob: %v", err)
	}
	if down.Ciphertext != "encrypted-note" || down.PlainSHA256 != "plain-sha" {
		t.Fatalf("downloaded envelope = %#v", down)
	}
}

func TestClientUploadBlobRequiresPlannedHashSizeAndExpiry(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_plan", SessionToken: "secret-token"})
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_plan", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	envelope := BlobEnvelope{SchemaVersion: "pinax.cloud.envelope.v1", Alg: "AES-256-GCM", KeyID: "key_1", Nonce: "nonce", Ciphertext: "encrypted-note", PlainSHA256: "plain-sha"}
	if err := client.UploadBlob(context.Background(), "blob_unplanned", envelope); err == nil || !IsCode(err, "BLOB_MISSING") {
		t.Fatalf("unplanned upload err = %#v", err)
	}
	if _, err := client.SignUpload(context.Background(), "blob_hash_mismatch", "sha256:not-the-envelope", int64(compactBlobEnvelopeSize(t, envelope)), "application/vnd.pinax.encrypted-envelope+json"); err != nil {
		t.Fatalf("sign mismatched blob: %v", err)
	}
	if err := client.UploadBlob(context.Background(), "blob_hash_mismatch", envelope); err == nil || !IsCode(err, "BLOB_HASH_MISMATCH") {
		t.Fatalf("hash mismatch err = %#v", err)
	}
	if _, err := client.SignUpload(context.Background(), "blob_expired", compactBlobEnvelopeHash(t, envelope), int64(compactBlobEnvelopeSize(t, envelope)), "application/vnd.pinax.encrypted-envelope+json"); err != nil {
		t.Fatalf("sign expired blob: %v", err)
	}
	server.ExpireBlobPlanForTest("vault_plan", "blob_expired")
	if err := client.UploadBlob(context.Background(), "blob_expired", envelope); err == nil || !IsCode(err, "UPLOAD_PLAN_EXPIRED") {
		t.Fatalf("expired upload err = %#v", err)
	}
}

func compactBlobEnvelopeHash(t *testing.T, envelope BlobEnvelope) string {
	t.Helper()
	blobHash, _, err := compactBlobEnvelopeHashAndSize(envelope)
	if err != nil {
		t.Fatalf("hash envelope: %v", err)
	}
	return blobHash
}

func compactBlobEnvelopeSize(t *testing.T, envelope BlobEnvelope) int {
	t.Helper()
	_, sizeBytes, err := compactBlobEnvelopeHashAndSize(envelope)
	if err != nil {
		t.Fatalf("size envelope: %v", err)
	}
	return int(sizeBytes)
}

func TestClientSignUploadRejectsNegativeSize(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_1", SessionToken: "secret-token"})
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_1", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.SignUpload(context.Background(), "blob_bad", "sha256:blob-bad", -1, "application/vnd.pinax.encrypted-envelope+json")
	if err == nil {
		t.Fatalf("negative size sign-upload succeeded")
	}
	cloudErr, ok := err.(*Error)
	if !ok || cloudErr.Code != CodeValidationFailed || cloudErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative size error = %#v", err)
	}
}

func TestClientRevisionCASConflictReturnsStableError(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_1", SessionToken: "secret-token"})
	defer server.Close()

	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_1", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	// 用不匹配的 base revision 提交，必须拿到 REVISION_CONFLICT。
	_, err = client.CommitRevision(context.Background(), CommitRequest{BaseRevision: "rev_nonexistent", ManifestBlobID: "blob_manifest", IdempotencyKey: "req_123"})
	if err == nil {
		t.Fatalf("commit unexpectedly succeeded")
	}
	cloudErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if cloudErr.Code != CodeRevisionConflict || cloudErr.StatusCode != http.StatusConflict || !cloudErr.Retryable {
		t.Fatalf("cloud error = %#v", cloudErr)
	}
	if !IsRevisionConflict(err) {
		t.Fatalf("IsRevisionConflict should be true")
	}
	text := err.Error()
	for _, secret := range []string{"secret-token", "Authorization", "Bearer"} {
		if strings.Contains(text, secret) {
			t.Fatalf("error leaked %q: %s", secret, text)
		}
	}
}

func TestClientRejectsMissingAuth(t *testing.T) {
	server := mlptest.New(mlptest.Config{VaultID: "vault_1", SessionToken: "session-token"})
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, VaultID: "vault_1", DeviceID: "dev_laptop", Token: "wrong-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.CurrentPrincipal(context.Background())
	if err == nil {
		t.Fatalf("expected auth error")
	}
	cloudErr, ok := err.(*Error)
	if !ok || cloudErr.Code != CodeUnauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got %#v", err)
	}
}

func TestContractFixtureCoversMinimumCloudAPI(t *testing.T) {
	payload, err := os.ReadFile("testdata/cloud_contract.json")
	if err != nil {
		t.Fatalf("read contract fixture: %v", err)
	}
	var fixture struct {
		Operations []struct {
			Name        string         `json:"name"`
			Method      string         `json:"method"`
			Path        string         `json:"path"`
			SuccessBody map[string]any `json:"success_body"`
		} `json:"operations"`
		ErrorCodes  []string `json:"error_codes"`
		Terminology string   `json:"terminology"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode contract fixture: %v", err)
	}
	if fixture.Terminology != "vault_id" {
		t.Fatalf("contract fixture terminology = %q, want vault_id", fixture.Terminology)
	}
	wantOps := map[string]bool{
		"health": false, "bootstrap": false, "current_principal": false,
		"create_vault": false, "link_vault": false, "changes": false,
		"current_head": false, "blob_batch_check": false, "blob_sign_upload": false,
		"blob_upload": false, "blob_download": false, "revision_commit": false,
	}
	for _, op := range fixture.Operations {
		if op.Method == "" || op.Path == "" {
			t.Fatalf("operation missing method/path: %#v", op)
		}
		if strings.Contains(op.Path, "{workspace_id}") || strings.Contains(op.Path, "/workspaces/") {
			t.Fatalf("MLP contract must use vault_id terminology, got path %q", op.Path)
		}
		if op.Name == "bootstrap" || op.Name == "current_principal" {
			assertAuthFixtureFields(t, op.Name, op.SuccessBody)
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
	wantErrors := map[string]bool{
		"UNAUTHENTICATED": false, "DEVICE_REVOKED": false, "REVISION_CONFLICT": false,
		"FORBIDDEN_SCOPE": false, "VALIDATION_FAILED": false, "BLOB_MISSING": false,
		"BACKEND_UNAVAILABLE": false,
	}
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

func assertAuthFixtureFields(t *testing.T, operation string, body map[string]any) {
	t.Helper()
	for _, key := range []string{"account_id", "device_id", "vault_id", "token_ref", "scope"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("%s success_body missing %s: %#v", operation, key, body)
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
