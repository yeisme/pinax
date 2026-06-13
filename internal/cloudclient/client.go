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

type Config struct {
	Endpoint    string
	WorkspaceID string
	DeviceID    string
	Token       string
	HTTPClient  *http.Client
}

type Client struct {
	endpoint    string
	workspaceID string
	deviceID    string
	token       string
	httpClient  *http.Client
}

type Health struct {
	Status          string `json:"status"`
	ContractVersion string `json:"contract_version,omitempty"`
}

type Revision struct {
	RevisionID     string `json:"revision_id"`
	ManifestBlobID string `json:"manifest_blob_id"`
}

type BlobEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Alg           string `json:"alg"`
	KeyID         string `json:"key_id"`
	Nonce         string `json:"nonce,omitempty"`
	Ciphertext    string `json:"ciphertext"`
	PlainSHA256   string `json:"plain_sha256"`
}

type BlobCheckResult struct {
	MissingBlobIDs []string `json:"missing_blob_ids"`
}

type CommitRequest struct {
	BaseRevision   string   `json:"base_revision"`
	RevisionID     string   `json:"revision_id,omitempty"`
	ManifestBlobID string   `json:"manifest_blob_id"`
	BlobIDs        []string `json:"blob_ids,omitempty"`
	DeviceID       string   `json:"device_id,omitempty"`
	IdempotencyKey string   `json:"-"`
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

func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("cloud endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid cloud endpoint")
	}
	workspaceID := strings.TrimSpace(cfg.WorkspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("cloud workspace id is required")
	}
	deviceID := strings.TrimSpace(cfg.DeviceID)
	if deviceID == "" {
		return nil, fmt.Errorf("cloud device id is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{endpoint: endpoint, workspaceID: workspaceID, deviceID: deviceID, token: strings.TrimSpace(cfg.Token), httpClient: httpClient}, nil
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.doJSON(ctx, http.MethodGet, "/v1/health", "", nil, &out)
	return out, err
}

func (c *Client) CurrentRevision(ctx context.Context) (Revision, error) {
	var out Revision
	err := c.doJSON(ctx, http.MethodGet, c.workspacePath("revision"), "", nil, &out)
	return out, err
}

func (c *Client) BatchCheckBlobs(ctx context.Context, blobIDs []string) (BlobCheckResult, error) {
	var out BlobCheckResult
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath("blobs:batchCheck"), "", blobCheckRequest{BlobIDs: blobIDs}, &out)
	return out, err
}

func (c *Client) UploadBlob(ctx context.Context, blobID string, envelope BlobEnvelope) error {
	return c.doJSON(ctx, http.MethodPut, c.workspacePath("blobs", blobID), "", envelope, nil)
}

func (c *Client) DownloadBlob(ctx context.Context, blobID string) (BlobEnvelope, error) {
	var out BlobEnvelope
	err := c.doJSON(ctx, http.MethodGet, c.workspacePath("blobs", blobID), "", nil, &out)
	return out, err
}

func (c *Client) CommitRevision(ctx context.Context, req CommitRequest) (CommitResponse, error) {
	var out CommitResponse
	err := c.doJSON(ctx, http.MethodPost, c.workspacePath("revisions:commit"), strings.TrimSpace(req.IdempotencyKey), req, &out)
	return out, err
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
	request.Header.Set("X-Pinax-Device-ID", c.deviceID)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return &Error{Code: "transport_error", Message: "cloud transport failed", Retryable: true, StatusCode: 0}
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
		return &Error{Code: "cloud_http_error", Message: "cloud request failed", Retryable: response.StatusCode >= 500, StatusCode: response.StatusCode}
	}
	return &Error{Code: payload.Error.Code, Message: payload.Error.Message, Retryable: payload.Error.Retryable, StatusCode: response.StatusCode}
}

func (c *Client) workspacePath(parts ...string) string {
	var builder strings.Builder
	builder.WriteString("/v1/workspaces/")
	builder.WriteString(url.PathEscape(c.workspaceID))
	for _, part := range parts {
		builder.WriteByte('/')
		builder.WriteString(url.PathEscape(part))
	}
	return builder.String()
}
