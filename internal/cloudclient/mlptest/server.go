// Package mlptest 提供 Pinax Cloud Sync MLP REST 合同的内存 HTTP 测试服务端。
//
// 它忠实实现 pinax-cloud-sync-mlp 公开路由（vault_id 术语、bootstrap、vault create/link、
// changes cursor、blob batch-check/sign-upload/upload/download、revision CAS commit），
// 让 CLI 侧的 server transport push/pull、两设备收敛与冲突测试通过真实 HTTP 跑通，
// 而不依赖尚未实现的 Pinax Cloud 后端。
//
// 测试服务端不保存明文 note body：blob 只存加密 envelope。
package mlptest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Server 是内存 MLP 测试服务端。
type Server struct {
	*httptest.Server

	mu        sync.Mutex
	bootstrap string // configured bootstrap token
	token     string // session token issued after bootstrap
	accountID string
	deviceID  string
	vaults    map[string]*vaultState
}

type vaultState struct {
	cryptoMode     string
	blobs          map[string]map[string]any // blobID -> envelope
	blobMetadata   map[string]blobMetadata
	manifestBlobs  map[string]map[string]any
	revisions      []revisionRecord
	headRevision   string
	manifestBlobID string
	linked         bool
}

type blobMetadata struct {
	BlobHash  string
	Size      int64
	ExpiresAt time.Time
	Uploaded  bool
}

type revisionRecord struct {
	RevisionID     string
	ParentRevision string
	ManifestBlobID string
	BlobIDs        []string
	DeviceID       string
	CreatedAt      string
}

// Config 配置 MLP 测试服务端。
type Config struct {
	BootstrapToken string
	SessionToken   string
	AccountID      string
	VaultID        string
	CryptoMode     string
}

// New 启动一台内存 MLP 测试服务端。调用方用完应 Close。
func New(cfg Config) *Server {
	bootstrap := strings.TrimSpace(cfg.BootstrapToken)
	if bootstrap == "" {
		bootstrap = "bootstrap-secret"
	}
	token := strings.TrimSpace(cfg.SessionToken)
	if token == "" {
		token = "session-token"
	}
	accountID := strings.TrimSpace(cfg.AccountID)
	if accountID == "" {
		accountID = "acc_test"
	}
	cryptoMode := strings.TrimSpace(cfg.CryptoMode)
	if cryptoMode == "" {
		cryptoMode = "client_encrypted_v1"
	}
	s := &Server{
		bootstrap: bootstrap,
		token:     token,
		accountID: accountID,
		vaults:    map[string]*vaultState{},
	}
	if vid := strings.TrimSpace(cfg.VaultID); vid != "" {
		s.vaults[vid] = newVaultState(cryptoMode)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func newVaultState(cryptoMode string) *vaultState {
	return &vaultState{
		cryptoMode:    cryptoMode,
		blobs:         map[string]map[string]any{},
		blobMetadata:  map[string]blobMetadata{},
		manifestBlobs: map[string]map[string]any{},
	}
}

// Token 返回 bootstrap 后颁发的会话 token（测试用于构造 cloudclient.Config.Token）。
func (s *Server) Token() string { return s.token }

// Endpoint 返回服务端地址。
func (s *Server) Endpoint() string { return s.URL }

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/health":
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "contract_version": "pinax.cloud.contract.mlp.v1"})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/bootstrap":
		s.handleBootstrap(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/principal":
		s.handlePrincipal(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/vaults":
		s.handleCreateVault(w, r)
	default:
		vaultID, rest, ok := splitVaultPath(r.URL.Path)
		if !ok {
			writeError(w, http.StatusNotFound, "VALIDATION_FAILED", "unknown route")
			return
		}
		s.handleVault(w, r, vaultID, rest)
	}
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BootstrapToken string `json:"bootstrap_token"`
		DeviceLabel    string `json:"device_label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid body")
		return
	}
	if req.BootstrapToken != s.bootstrap {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "bootstrap token mismatch")
		return
	}
	s.mu.Lock()
	s.deviceID = req.DeviceLabel
	vaultID := "vault_" + req.DeviceLabel
	if _, ok := s.vaults[vaultID]; !ok {
		s.vaults[vaultID] = newVaultState("client_encrypted_v1")
	}
	dev := req.DeviceLabel
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"account_id": s.accountID,
		"device_id":  dev,
		"vault_id":   vaultID,
		"token_ref":  "profile://cloud",
		"scope":      "sync",
	})
}

func (s *Server) handlePrincipal(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing or invalid token")
		return
	}
	deviceID := r.Header.Get("X-Pinax-Device-ID")
	writeJSON(w, http.StatusOK, map[string]any{
		"account_id": s.accountID,
		"device_id":  deviceID,
		"vault_id":   "vault_" + deviceID,
		"token_ref":  "profile://cloud",
		"scope":      "sync",
	})
}

func (s *Server) handleCreateVault(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing or invalid token")
		return
	}
	var req struct {
		CryptoMode string `json:"crypto_mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.CryptoMode == "" {
		req.CryptoMode = "client_encrypted_v1"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultID := "vault_" + r.Header.Get("X-Pinax-Device-ID")
	s.vaults[vaultID] = newVaultState(req.CryptoMode)
	writeJSON(w, http.StatusOK, map[string]any{"vault_id": vaultID, "crypto_mode": req.CryptoMode})
}

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request, vaultID, rest string) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing or invalid token")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, ok := s.vaults[vaultID]
	if !ok {
		writeError(w, http.StatusNotFound, "VALIDATION_FAILED", "vault not found")
		return
	}
	switch {
	case rest == "link" && r.Method == http.MethodPost:
		vault.linked = true
		writeJSON(w, http.StatusOK, map[string]any{"vault_id": vaultID, "crypto_mode": vault.cryptoMode, "current_revision": vault.headRevision})
	case rest == "head" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"revision_id": vault.headRevision, "manifest_blob_id": vault.manifestBlobID})
	case rest == "changes" && r.Method == http.MethodGet:
		s.handleChanges(w, r, vault)
	case rest == "blobs:batch-check" && r.Method == http.MethodPost:
		s.handleBatchCheck(w, r, vault)
	case rest == "blobs:sign-upload" && r.Method == http.MethodPost:
		s.handleSignUpload(w, r, vaultID, vault)
	case strings.HasPrefix(rest, "blobs/") && r.Method == http.MethodPut:
		s.handlePutBlob(w, r, vault, strings.TrimPrefix(rest, "blobs/"))
	case strings.HasPrefix(rest, "blobs/") && r.Method == http.MethodGet:
		s.handleGetBlob(w, vault, strings.TrimPrefix(rest, "blobs/"))
	case rest == "revisions" && r.Method == http.MethodPost:
		s.handleCommit(w, r, vault, r.Header.Get("Idempotency-Key"))
	default:
		writeError(w, http.StatusNotFound, "VALIDATION_FAILED", "unknown vault route")
	}
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request, vault *vaultState) {
	since := r.URL.Query().Get("since")
	revisions := []map[string]any{}
	start := 0
	for i, rev := range vault.revisions {
		if rev.RevisionID == since {
			start = i + 1
			break
		}
	}
	for _, rev := range vault.revisions[start:] {
		revisions = append(revisions, map[string]any{"revision_id": rev.RevisionID, "manifest_blob_id": rev.ManifestBlobID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions, "objects": []any{}, "has_more": false})
}

func (s *Server) handleBatchCheck(w http.ResponseWriter, r *http.Request, vault *vaultState) {
	var req struct {
		BlobIDs []string `json:"blob_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid body")
		return
	}
	missing := []string{}
	present := []map[string]any{}
	for _, id := range req.BlobIDs {
		metadata, ok := vault.blobMetadata[id]
		if !ok || !metadata.Uploaded {
			missing = append(missing, id)
			continue
		}
		present = append(present, map[string]any{"blob_id": id, "blob_hash": metadata.BlobHash, "size": metadata.Size, "retention_eligible": true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"present": present, "missing_blob_ids": missing})
}

func (s *Server) handleSignUpload(w http.ResponseWriter, r *http.Request, vaultID string, vault *vaultState) {
	var req struct {
		BlobID      string `json:"blob_id"`
		BlobHash    string `json:"blob_hash"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid body")
		return
	}
	if !validMLPID(req.BlobID) || strings.TrimSpace(req.BlobHash) == "" || req.Size < 0 || req.ContentType != "application/vnd.pinax.encrypted-envelope+json" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "blob_id, blob_hash, and size are required")
		return
	}
	expiresAt := time.Now().Add(15 * time.Minute).UTC()
	metadata := vault.blobMetadata[req.BlobID]
	if metadata.Uploaded {
		if metadata.BlobHash != req.BlobHash {
			writeError(w, http.StatusBadRequest, "BLOB_HASH_MISMATCH", "planned blob hash does not match uploaded blob")
			return
		}
		if metadata.Size != req.Size {
			writeError(w, http.StatusBadRequest, "BLOB_SIZE_MISMATCH", "planned blob size does not match uploaded blob")
			return
		}
		expiresAt = metadata.ExpiresAt
	} else {
		metadata = blobMetadata{BlobHash: req.BlobHash, Size: req.Size, ExpiresAt: expiresAt}
	}
	vault.blobMetadata[req.BlobID] = metadata
	writeJSON(w, http.StatusOK, map[string]any{
		"blob_id":    req.BlobID,
		"object_key": serverOwnedObjectKey(vaultID, req.BlobID, req.BlobHash),
		"method":     "PUT",
		"mode":       "local-proxy",
		"url":        "/v1/vaults/" + vaultID + "/blobs/" + req.BlobID,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handlePutBlob(w http.ResponseWriter, r *http.Request, vault *vaultState, blobID string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid envelope")
		return
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid envelope")
		return
	}
	var envelope map[string]any
	if err := json.Unmarshal(compact.Bytes(), &envelope); err != nil || !validEncryptedEnvelope(envelope) {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid envelope")
		return
	}
	metadata, ok := vault.blobMetadata[blobID]
	if !ok {
		writeError(w, http.StatusNotFound, "BLOB_MISSING", "upload plan required")
		return
	}
	if time.Now().After(metadata.ExpiresAt) {
		writeError(w, http.StatusGone, "UPLOAD_PLAN_EXPIRED", "blob upload plan has expired")
		return
	}
	blobHash := compactHash(compact.Bytes())
	if metadata.BlobHash != blobHash {
		writeError(w, http.StatusBadRequest, "BLOB_HASH_MISMATCH", "uploaded blob hash does not match planned hash")
		return
	}
	if metadata.Size != int64(compact.Len()) {
		writeError(w, http.StatusBadRequest, "BLOB_SIZE_MISMATCH", "uploaded blob size does not match planned size")
		return
	}
	metadata.Uploaded = true
	vault.blobMetadata[blobID] = metadata
	// manifest blobs 用 manifest_ 前缀约定与 object-store transport 保持一致。
	if strings.HasPrefix(blobID, "manifest_") {
		vault.manifestBlobs[blobID] = envelope
	} else {
		vault.blobs[blobID] = envelope
	}
	writeJSON(w, http.StatusCreated, map[string]any{"blob_id": blobID})
}

func (s *Server) handleGetBlob(w http.ResponseWriter, vault *vaultState, blobID string) {
	if env, ok := vault.manifestBlobs[blobID]; ok {
		writeJSON(w, http.StatusOK, env)
		return
	}
	if env, ok := vault.blobs[blobID]; ok {
		writeJSON(w, http.StatusOK, env)
		return
	}
	writeError(w, http.StatusNotFound, "BLOB_MISSING", "blob not found")
}

func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request, vault *vaultState, idempotencyKey string) {
	var req struct {
		BaseRevision   string `json:"base_revision"`
		RevisionID     string `json:"revision_id"`
		ManifestBlobID string `json:"manifest_blob_id"`
		ObjectRefs     []struct {
			PathHash  string `json:"path_hash"`
			BlobID    string `json:"blob_id"`
			BlobHash  string `json:"blob_hash"`
			SizeBytes int64  `json:"size_bytes"`
			Deleted   bool   `json:"deleted"`
		} `json:"object_refs"`
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid body")
		return
	}
	// CAS: base revision 必须匹配当前 head。
	if req.BaseRevision != vault.headRevision {
		writeError(w, http.StatusConflict, "REVISION_CONFLICT", "base revision is stale")
		return
	}
	// 引用的 manifest 和 blob 必须已有 sign-upload 计划且完成上传。
	manifestMetadata, ok := vault.blobMetadata[req.ManifestBlobID]
	if !ok || !manifestMetadata.Uploaded {
		writeError(w, http.StatusBadRequest, "BLOB_MISSING", "manifest blob missing")
		return
	}
	if _, ok := vault.manifestBlobs[req.ManifestBlobID]; !ok {
		writeError(w, http.StatusBadRequest, "BLOB_MISSING", "manifest blob missing")
		return
	}
	for _, ref := range req.ObjectRefs {
		if ref.Deleted {
			continue
		}
		if !validPathHash(ref.PathHash) || ref.BlobID == "" || ref.BlobHash == "" || ref.SizeBytes < 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "object_refs are required")
			return
		}
		if _, ok := vault.blobs[ref.BlobID]; !ok {
			if _, ok := vault.manifestBlobs[ref.BlobID]; !ok {
				writeError(w, http.StatusBadRequest, "BLOB_MISSING", "referenced blob missing: "+ref.BlobID)
				return
			}
		}
		metadata, ok := vault.blobMetadata[ref.BlobID]
		if !ok || !metadata.Uploaded || metadata.BlobHash != ref.BlobHash || metadata.Size != ref.SizeBytes {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "object_ref metadata does not match blob metadata")
			return
		}
	}
	revisionID := strings.TrimSpace(req.RevisionID)
	if revisionID == "" {
		revisionID = "rev_" + time.Now().UTC().Format("20060102150405.000000")
	}
	vault.revisions = append(vault.revisions, revisionRecord{
		RevisionID:     revisionID,
		ParentRevision: req.BaseRevision,
		ManifestBlobID: req.ManifestBlobID,
		BlobIDs:        objectRefBlobIDs(req.ObjectRefs),
		DeviceID:       req.DeviceID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	})
	vault.headRevision = revisionID
	vault.manifestBlobID = req.ManifestBlobID
	writeJSON(w, http.StatusOK, map[string]any{"revision_id": revisionID, "manifest_blob_id": req.ManifestBlobID})
}

func (s *Server) authorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return auth == "Bearer "+s.token
}

func compactHash(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s *Server) ExpireBlobPlanForTest(vaultID, blobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.vaults[vaultID]
	if vault == nil {
		return
	}
	metadata := vault.blobMetadata[blobID]
	metadata.ExpiresAt = time.Now().Add(-time.Minute)
	vault.blobMetadata[blobID] = metadata
}

func serverOwnedObjectKey(vaultID, blobID, blobHash string) string {
	sum := sha256.Sum256([]byte(vaultID + "\x00" + blobID + "\x00" + blobHash))
	digest := hex.EncodeToString(sum[:])
	return "objects/" + digest[:2] + "/" + digest[2:4] + "/" + digest + ".blob"
}

func validEncryptedEnvelope(envelope map[string]any) bool {
	allowed := map[string]struct{}{"schema_version": {}, "alg": {}, "key_id": {}, "nonce": {}, "ciphertext": {}, "plain_sha256": {}}
	for field := range envelope {
		lower := strings.ToLower(field)
		if _, ok := allowed[field]; !ok {
			return false
		}
		if field != "plain_sha256" && (strings.Contains(lower, "plain") || strings.Contains(lower, "body") || strings.Contains(lower, "content") || strings.Contains(lower, "path") || strings.Contains(lower, "note")) {
			return false
		}
	}
	return len(envelope) == len(allowed) && envelope["schema_version"] == "pinax.cloud.envelope.v1" && nonEmptyString(envelope["alg"]) && nonEmptyString(envelope["key_id"]) && nonEmptyString(envelope["nonce"]) && nonEmptyString(envelope["ciphertext"]) && nonEmptyString(envelope["plain_sha256"])
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func validPathHash(value string) bool {
	if value == "" || strings.ContainsAny(value, "/\\") {
		return false
	}
	if strings.HasPrefix(value, "sha256:") && len(value) > len("sha256:") {
		return true
	}
	if strings.HasPrefix(value, "path_") && len(value) > len("path_") {
		return true
	}
	if len(value) == 64 {
		for _, r := range value {
			if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
				continue
			}
			return false
		}
		return true
	}
	return false
}

func validMLPID(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func objectRefBlobIDs(refs []struct {
	PathHash  string `json:"path_hash"`
	BlobID    string `json:"blob_id"`
	BlobHash  string `json:"blob_hash"`
	SizeBytes int64  `json:"size_bytes"`
	Deleted   bool   `json:"deleted"`
}) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !ref.Deleted {
			ids = append(ids, ref.BlobID)
		}
	}
	return ids
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	// MLP 合同：REVISION_CONFLICT 可重试（客户端应 pull 后重试），其余 4xx 默认不可重试，5xx 可重试。
	retryable := status >= 500 || code == "REVISION_CONFLICT"
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": retryable}})
}

// splitVaultPath 从 /v1/vaults/{vault_id}/rest 解析 vaultID 和剩余路径。
func splitVaultPath(path string) (vaultID, rest string, ok bool) {
	const prefix = "/v1/vaults/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	remaining := strings.TrimPrefix(path, prefix)
	idx := strings.Index(remaining, "/")
	if idx < 0 {
		return remaining, "", true
	}
	return remaining[:idx], remaining[idx+1:], true
}
