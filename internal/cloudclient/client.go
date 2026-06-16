// Package cloudclient 是 Pinax CLI 访问 Pinax Cloud Sync MLP 服务端 REST 合同的客户端。
//
// MLP 合同（pinax-cloud-sync-mlp）使用 vault_id 术语，公开路由：
//
//   - POST /v1/auth/bootstrap          账户与设备会话 bootstrap
//   - GET  /v1/auth/principal          当前登录主体（login state）
//   - POST /v1/vaults                  创建 vault
//   - POST /v1/vaults/{vault_id}/link  设备绑定 vault
//   - GET  /v1/vaults/{vault_id}/changes?since=<revision_id>
//   - POST /v1/vaults/{vault_id}/blobs:batch-check
//   - POST /v1/vaults/{vault_id}/blobs:sign-upload
//   - PUT  /v1/vaults/{vault_id}/blobs/{blob_id}
//   - GET  /v1/vaults/{vault_id}/blobs/{blob_id}
//   - GET  /v1/vaults/{vault_id}/head   当前 head revision/manifest
//   - POST /v1/vaults/{vault_id}/revisions  CAS commit
//
// 所有写操作携带稳定 error code、idempotency、device 与脱敏要求；Cloud 不读取明文。
package cloudclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// MLP 稳定 error code（大写，与服务端 pinax-cloud-sync-mlp 合同一致）。
const (
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeDeviceRevoked      = "DEVICE_REVOKED"
	CodeForbiddenScope     = "FORBIDDEN_SCOPE"
	CodeRevisionConflict   = "REVISION_CONFLICT"
	CodeValidationFailed   = "VALIDATION_FAILED"
	CodeBlobMissing        = "BLOB_MISSING"
	CodeBackendUnavailable = "BACKEND_UNAVAILABLE"
	CodeTransportError     = "TRANSPORT_ERROR"
	CodeCloudHTTPError     = "CLOUD_HTTP_ERROR"
)

type Config struct {
	Endpoint   string
	VaultID    string
	DeviceID   string
	Token      string
	HTTPClient *http.Client
}

type Client struct {
	endpoint   string
	vaultID    string
	deviceID   string
	token      string
	httpClient *http.Client
}

type Health struct {
	Status          string `json:"status"`
	ContractVersion string `json:"contract_version,omitempty"`
}

// Principal 描述当前登录主体的脱敏事实，用于 cloud login/doctor/sync status。
type Principal struct {
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
	VaultID   string `json:"vault_id,omitempty"`
	TokenRef  string `json:"token_ref,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// BootstrapResult 是 bootstrap/login 成功后返回的会话事实；不含 raw token。
type BootstrapResult struct {
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
	VaultID   string `json:"vault_id,omitempty"`
	TokenRef  string `json:"token_ref,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// VaultLinkFacts 是 vault create/link 返回的有界事实。
type VaultLinkFacts struct {
	VaultID         string `json:"vault_id"`
	CryptoMode      string `json:"crypto_mode"`
	CurrentRevision string `json:"current_revision,omitempty"`
}

type Revision struct {
	RevisionID     string `json:"revision_id"`
	ManifestBlobID string `json:"manifest_blob_id"`
}

// ObjectRef 是 changes cursor 返回的对象引用；使用 path_hash/blob_hash 而非明文路径。
type ObjectRef struct {
	PathHash  string `json:"path_hash"`
	BlobID    string `json:"blob_id,omitempty"`
	BlobHash  string `json:"blob_hash"`
	Size      int64  `json:"size"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Deleted   bool   `json:"deleted"`
}

// ChangesResult 是 GET /changes?since=<revision_id> 的返回。
type ChangesResult struct {
	Revisions []Revision  `json:"revisions"`
	Objects   []ObjectRef `json:"objects"`
	HasMore   bool        `json:"has_more"`
}

type BlobEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Alg           string `json:"alg"`
	KeyID         string `json:"key_id"`
	Nonce         string `json:"nonce,omitempty"`
	Ciphertext    string `json:"ciphertext"`
	PlainSHA256   string `json:"plain_sha256"`
}

type BlobFact struct {
	BlobID   string `json:"blob_id"`
	BlobHash string `json:"blob_hash"`
	Size     int64  `json:"size"`
}

type BlobCheckResult struct {
	MissingBlobIDs []string   `json:"missing_blob_ids"`
	Present        []BlobFact `json:"present"`
}

// UploadPlan 是 sign-upload 返回的服务端拥有对象上传计划。
type UploadPlan struct {
	BlobID    string            `json:"blob_id"`
	ObjectKey string            `json:"object_key"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt string            `json:"expires_at,omitempty"`
}

type CommitRequest struct {
	BaseRevision   string      `json:"base_revision"`
	RevisionID     string      `json:"revision_id,omitempty"`
	ManifestBlobID string      `json:"manifest_blob_id"`
	ObjectRefs     []ObjectRef `json:"object_refs,omitempty"`
	DeviceID       string      `json:"device_id,omitempty"`
	IdempotencyKey string      `json:"-"`
}

type CommitResponse struct {
	RevisionID     string `json:"revision_id"`
	ManifestBlobID string `json:"manifest_blob_id"`
}

type Error struct {
	Code       string
	Message    string
	Retryable  bool
	StatusCode int
}

func (e *Error) Error() string {
	if e == nil {
		return "cloud request failed"
	}
	if e.Message == "" {
		return fmt.Sprintf("cloud request failed: code=%s status=%d", e.Code, e.StatusCode)
	}
	return fmt.Sprintf("cloud request failed: code=%s status=%d message=%s", e.Code, e.StatusCode, e.Message)
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type errorResponse struct {
	Error ErrorBody `json:"error"`
}

type blobCheckRequest struct {
	BlobIDs []string `json:"blob_ids"`
}

type signUploadRequest struct {
	BlobID      string `json:"blob_id"`
	BlobHash    string `json:"blob_hash"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type bootstrapRequest struct {
	BootstrapToken string `json:"bootstrap_token"`
	DeviceLabel    string `json:"device_label,omitempty"`
}

type createVaultRequest struct {
	CryptoMode string `json:"crypto_mode"`
}

func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("cloud endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid cloud endpoint")
	}
	deviceID := strings.TrimSpace(cfg.DeviceID)
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{endpoint: endpoint, vaultID: strings.TrimSpace(cfg.VaultID), deviceID: deviceID, token: strings.TrimSpace(cfg.Token), httpClient: httpClient}, nil
}

// VaultID 返回客户端绑定的 vault id。
func (c *Client) VaultID() string { return c.vaultID }

func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.doJSON(ctx, http.MethodGet, "/v1/health", "", nil, &out)
	return out, err
}

// Bootstrap 调用 POST /v1/auth/bootstrap 完成单账户自托管 bootstrap。
// 返回的 BootstrapResult 只含脱敏事实；token 由调用方写入 secret ref，不进入日志。
func (c *Client) Bootstrap(ctx context.Context, bootstrapToken, deviceLabel string) (BootstrapResult, error) {
	var out BootstrapResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/bootstrap", "", bootstrapRequest{BootstrapToken: bootstrapToken, DeviceLabel: deviceLabel}, &out)
	return out, err
}

// CurrentPrincipal 调用 GET /v1/auth/principal 返回当前登录主体脱敏事实。
func (c *Client) CurrentPrincipal(ctx context.Context) (Principal, error) {
	var out Principal
	err := c.doJSON(ctx, http.MethodGet, "/v1/auth/principal", "", nil, &out)
	return out, err
}

// CreateVault 调用 POST /v1/vaults 创建 vault。
func (c *Client) CreateVault(ctx context.Context, cryptoMode string) (VaultLinkFacts, error) {
	var out VaultLinkFacts
	err := c.doJSON(ctx, http.MethodPost, "/v1/vaults", "", createVaultRequest{CryptoMode: cryptoMode}, &out)
	return out, err
}

// LinkVault 调用 POST /v1/vaults/{vault_id}/link 绑定当前设备到 vault。
func (c *Client) LinkVault(ctx context.Context, vaultID string) (VaultLinkFacts, error) {
	var out VaultLinkFacts
	err := c.doJSON(ctx, http.MethodPost, c.vaultPath(vaultID, "link"), "", nil, &out)
	return out, err
}

func (c *Client) CurrentRevision(ctx context.Context) (Revision, error) {
	vaultID := c.requireVaultID()
	var out Revision
	err := c.doJSON(ctx, http.MethodGet, c.vaultPath(vaultID, "head"), "", nil, &out)
	return out, err
}

// Changes 调用 GET /v1/vaults/{vault_id}/changes?since=<revision_id> 获取增量对象引用。
func (c *Client) Changes(ctx context.Context, since string) (ChangesResult, error) {
	vaultID := c.requireVaultID()
	path := c.vaultPath(vaultID, "changes")
	if strings.TrimSpace(since) != "" {
		path += "?since=" + url.QueryEscape(since)
	}
	var out ChangesResult
	err := c.doJSON(ctx, http.MethodGet, path, "", nil, &out)
	return out, err
}

func (c *Client) BatchCheckBlobs(ctx context.Context, blobIDs []string) (BlobCheckResult, error) {
	vaultID := c.requireVaultID()
	var out BlobCheckResult
	err := c.doJSON(ctx, http.MethodPost, c.vaultPath(vaultID, "blobs:batch-check"), "", blobCheckRequest{BlobIDs: blobIDs}, &out)
	return out, err
}

// SignUpload 调用 POST /v1/vaults/{vault_id}/blobs:sign-upload 获取服务端拥有的上传计划。
func (c *Client) SignUpload(ctx context.Context, blobID, blobHash string, size int64, contentType string) (UploadPlan, error) {
	vaultID := c.requireVaultID()
	var out UploadPlan
	err := c.doJSON(ctx, http.MethodPost, c.vaultPath(vaultID, "blobs:sign-upload"), "", signUploadRequest{BlobID: blobID, BlobHash: blobHash, Size: size, ContentType: contentType}, &out)
	return out, err
}

func (c *Client) UploadBlob(ctx context.Context, blobID string, envelope BlobEnvelope) error {
	vaultID := c.requireVaultID()
	return c.doJSON(ctx, http.MethodPut, c.vaultPath(vaultID, "blobs", blobID), "", envelope, nil)
}

func (c *Client) DownloadBlob(ctx context.Context, blobID string) (BlobEnvelope, error) {
	vaultID := c.requireVaultID()
	var out BlobEnvelope
	err := c.doJSON(ctx, http.MethodGet, c.vaultPath(vaultID, "blobs", blobID), "", nil, &out)
	return out, err
}

func (c *Client) CommitRevision(ctx context.Context, req CommitRequest) (CommitResponse, error) {
	vaultID := c.requireVaultID()
	var out CommitResponse
	err := c.doJSON(ctx, http.MethodPost, c.vaultPath(vaultID, "revisions"), strings.TrimSpace(req.IdempotencyKey), req, &out)
	return out, err
}

func (c *Client) requireVaultID() string {
	if c.vaultID == "" {
		panic("cloudclient: vault id is required for vault-scoped operations")
	}
	return c.vaultID
}

func (c *Client) doJSON(ctx context.Context, method, path, idempotencyKey string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.deviceID != "" {
		request.Header.Set("X-Pinax-Device-ID", c.deviceID)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return &Error{Code: CodeTransportError, Message: "cloud transport failed", Retryable: true, StatusCode: 0}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeError(response)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func decodeError(response *http.Response) error {
	var payload errorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Error.Code == "" {
		return &Error{Code: CodeCloudHTTPError, Message: "cloud request failed", Retryable: response.StatusCode >= 500, StatusCode: response.StatusCode}
	}
	return &Error{Code: payload.Error.Code, Message: payload.Error.Message, Retryable: payload.Error.Retryable, StatusCode: response.StatusCode}
}

// vaultPath 构造 /v1/vaults/{vault_id}/... 路径。
func (c *Client) vaultPath(vaultID string, parts ...string) string {
	var builder strings.Builder
	builder.WriteString("/v1/vaults/")
	builder.WriteString(url.PathEscape(vaultID))
	for _, part := range parts {
		builder.WriteByte('/')
		builder.WriteString(part)
	}
	return builder.String()
}
