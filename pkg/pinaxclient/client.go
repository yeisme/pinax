package pinaxclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	defaultTimeout          = 15 * time.Second
	maxClientTimeout        = 2 * time.Minute
	defaultMaxRequestBytes  = int64(1 << 20)
	defaultMaxResponseBytes = int64(4 << 20)
	maxConfiguredBodyBytes  = int64(64 << 20)
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// NewMutationIdentity creates the stable pair a client must retain across
// retries of one logical mutation. The owner binds both values to the request
// digest and authenticated owner scope.
func NewMutationIdentity() (MutationIdentity, error) {
	operationValue, err := randomOpaqueValue("op_")
	if err != nil {
		return MutationIdentity{}, newError(CodeInvalidConfig, "Pinax operation identity could not be generated", err)
	}
	idempotencyValue, err := randomOpaqueValue("idem_")
	if err != nil {
		return MutationIdentity{}, newError(CodeInvalidConfig, "Pinax idempotency identity could not be generated", err)
	}
	return MutationIdentity{OperationID: operationValue, IdempotencyKey: idempotencyValue}, nil
}

func randomOpaqueValue(prefix string) (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value[:]), nil
}

type Config struct {
	BaseURL          string
	Token            string
	TokenFile        string
	HTTPClient       *http.Client
	Timeout          time.Duration
	MaxRequestBytes  int64
	MaxResponseBytes int64
}

type Client struct {
	baseURL          *url.URL
	token            string
	http             *http.Client
	maxRequestBytes  int64
	maxResponseBytes int64
}

func New(config Config) (*Client, error) {
	baseURL, err := validateBaseURL(config.BaseURL)
	if err != nil {
		return nil, newError(CodeInvalidConfig, "Pinax base URL is invalid", err)
	}
	if config.Token != "" && config.TokenFile != "" {
		return nil, newError(CodeInvalidConfig, "Pinax bearer token sources conflict", nil)
	}
	token := config.Token
	if config.TokenFile != "" {
		token, err = LoadTokenFile(config.TokenFile)
		if err != nil {
			return nil, err
		}
	}
	if strings.ContainsAny(token, "\r\n") {
		return nil, newError(CodeInvalidConfig, "Pinax bearer token contains invalid control characters", nil)
	}
	timeout := config.Timeout
	if timeout < 0 || timeout > maxClientTimeout {
		return nil, newError(CodeInvalidConfig, "Pinax client timeout is invalid", nil)
	}
	maxRequestBytes, err := boundedSize(config.MaxRequestBytes, defaultMaxRequestBytes)
	if err != nil {
		return nil, newError(CodeInvalidConfig, "Pinax request size limit is invalid", err)
	}
	maxResponseBytes, err := boundedSize(config.MaxResponseBytes, defaultMaxResponseBytes)
	if err != nil {
		return nil, newError(CodeInvalidConfig, "Pinax response size limit is invalid", err)
	}
	httpClient := &http.Client{}
	if config.HTTPClient != nil {
		*httpClient = *config.HTTPClient
	}
	if timeout > 0 {
		httpClient.Timeout = timeout
	} else if httpClient.Timeout <= 0 {
		httpClient.Timeout = defaultTimeout
	}
	if httpClient.Timeout > maxClientTimeout {
		return nil, newError(CodeInvalidConfig, "Pinax client timeout is invalid", nil)
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{
		baseURL:          baseURL,
		token:            token,
		http:             httpClient,
		maxRequestBytes:  maxRequestBytes,
		maxResponseBytes: maxResponseBytes,
	}, nil
}

// Ping reads the owner root projection.
func (c *Client) Ping(ctx context.Context) (Projection, error) {
	return c.getProjection(ctx, "/")
}

// Capabilities reads the legacy capabilities projection. The authoritative
// transport inventory remains Manifest; this method is retained for clients
// that still consume the additive legacy capability fields.
func (c *Client) Capabilities(ctx context.Context) (Projection, error) {
	return c.getProjection(ctx, "/v1/capabilities")
}

func (c *Client) Manifest(ctx context.Context) (Manifest, error) {
	projection, err := c.getProjection(ctx, "/v1/manifest")
	if err != nil {
		return Manifest{}, err
	}
	var payload struct {
		Manifest Manifest `json:"manifest"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return Manifest{}, err
	}
	if payload.Manifest.SchemaVersion != TransportManifestSchemaV1 || !validSHA256Digest(payload.Manifest.Digest) {
		return Manifest{}, newError(CodeUpstreamInvalidResponse, "Pinax manifest identity is invalid", nil)
	}
	return payload.Manifest, nil
}

func (c *Client) Readiness(ctx context.Context) (ConnectionReadiness, error) {
	projection, err := c.getProjection(ctx, "/v1/readiness")
	if err != nil {
		return ConnectionReadiness{}, err
	}
	var payload struct {
		Readiness ConnectionReadiness `json:"readiness"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ConnectionReadiness{}, err
	}
	if payload.Readiness.SchemaVersion != ConnectionReadinessSchemaV1 || !validReadiness(payload.Readiness.Overall) {
		return ConnectionReadiness{}, newError(CodeUpstreamInvalidResponse, "Pinax readiness identity is invalid", nil)
	}
	for _, layerName := range requiredReadinessLayers {
		if _, ok := payload.Readiness.Layers[layerName]; !ok {
			return ConnectionReadiness{}, newError(CodeUpstreamInvalidResponse, "Pinax readiness layers are incomplete", nil)
		}
	}
	for _, layer := range payload.Readiness.Layers {
		if !validReadiness(layer) {
			return ConnectionReadiness{}, newError(CodeUpstreamInvalidResponse, "Pinax readiness layer is invalid", nil)
		}
	}
	return payload.Readiness, nil
}

func (c *Client) Operation(ctx context.Context, operationID string) (Operation, error) {
	if !operationIDPattern.MatchString(operationID) {
		return Operation{}, newError(CodeRequestInvalid, "Pinax operation ID is invalid", nil)
	}
	projection, err := c.getProjection(ctx, "/v1/operations/"+url.PathEscape(operationID))
	if err != nil {
		return Operation{}, err
	}
	return decodeOperationProjection(projection, operationID)
}

func (c *Client) ReconcileOperation(ctx context.Context, operationID string) (Operation, error) {
	if !operationIDPattern.MatchString(operationID) {
		return Operation{}, newError(CodeRequestInvalid, "Pinax operation ID is invalid", nil)
	}
	projection, err := c.doProjection(ctx, http.MethodPost, "/v1/operations/"+url.PathEscape(operationID)+":reconcile", nil, "")
	if err != nil {
		return Operation{}, withOperationReconcile(err, operationID)
	}
	return decodeOperationProjection(projection, operationID)
}

func decodeOperationProjection(projection Projection, operationID string) (Operation, error) {
	var payload struct {
		Operation Operation `json:"operation"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return Operation{}, withOperationReconcile(err, operationID)
	}
	if payload.Operation.SchemaVersion != OperationSchemaV1 || payload.Operation.OperationID != operationID || payload.Operation.CapabilityID == "" || payload.Operation.BindingID == "" || !validOperationStatus(payload.Operation.Status) || payload.Operation.CreatedAt == "" || payload.Operation.UpdatedAt == "" || payload.Operation.AcceptedAt == "" {
		return Operation{}, &Error{Code: CodeUpstreamInvalidResponse, Message: "Pinax operation identity is invalid", OperationRef: operationID, ReconcileRequired: true}
	}
	if (payload.Operation.Status == "applying" || payload.Operation.Status == "reconcile_required") && !payload.Operation.ReconcileRequired {
		return Operation{}, &Error{Code: CodeUpstreamInvalidResponse, Message: "Pinax operation recovery facts are invalid", OperationRef: operationID, ReconcileRequired: true}
	}
	return payload.Operation, nil
}

func (c *Client) CallRPC(ctx context.Context, request RPCRequest) (Projection, error) {
	if strings.TrimSpace(request.Method) == "" {
		return Projection{}, newError(CodeRequestInvalid, "Pinax RPC method is required", nil)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax RPC request could not be encoded", err)
	}
	if int64(len(payload)) > c.maxRequestBytes {
		return Projection{}, newError(CodeRequestInvalid, "Pinax RPC request exceeds the configured size limit", nil)
	}
	return c.doProjection(ctx, http.MethodPost, "/v1/rpc", bytes.NewReader(payload), "application/json")
}

// CallMutationRPC executes one recovery-enabled RPC mutation. Ambiguous
// transport outcomes are resolved through the durable operation first; the
// original request is retried at most once and only when the owner reports a
// terminal failed operation that is both retryable and replay-safe.
func (c *Client) CallMutationRPC(ctx context.Context, request RPCRequest, identity MutationIdentity) (Projection, error) {
	if err := validateMutationIdentity(request, identity); err != nil {
		return Projection{}, err
	}
	return c.callMutationRPC(ctx, request, identity, true)
}

func (c *Client) callMutationRPC(ctx context.Context, request RPCRequest, identity MutationIdentity, allowReplaySafeRetry bool) (Projection, error) {
	projection, err := c.CallRPC(ctx, request)
	if err == nil || !ambiguousMutationError(err) {
		return projection, err
	}
	return c.recoverMutationRPC(ctx, request, identity, allowReplaySafeRetry, err)
}

func (c *Client) recoverMutationRPC(ctx context.Context, request RPCRequest, identity MutationIdentity, allowReplaySafeRetry bool, ambiguousErr error) (Projection, error) {
	operation, err := c.Operation(ctx, identity.OperationID)
	if err != nil {
		return Projection{}, mutationOutcomeUnknown(identity.OperationID, ambiguousErr, err)
	}
	if operation.Status == "accepted" || operation.Status == "applying" || operation.Status == "reconcile_required" {
		operation, err = c.ReconcileOperation(ctx, identity.OperationID)
		if err != nil {
			return Projection{}, mutationOutcomeUnknown(identity.OperationID, ambiguousErr, err)
		}
	}
	if operation.Status == "succeeded" {
		return mutationProjectionFromOperation(operation, true), nil
	}
	if operation.Status == "failed" && operation.Retryable && operation.ReplaySafe && allowReplaySafeRetry {
		return c.callMutationRPC(ctx, request, identity, false)
	}
	return mutationProjectionError(operation)
}

func validateMutationIdentity(request RPCRequest, identity MutationIdentity) error {
	if !operationIDPattern.MatchString(identity.OperationID) || !operationIDPattern.MatchString(identity.IdempotencyKey) {
		return newError(CodeRequestInvalid, "Pinax mutation identity is invalid", nil)
	}
	operationID, operationOK := request.Params["operation_id"].(string)
	idempotencyKey, idempotencyOK := request.Params["idempotency_key"].(string)
	if !operationOK || !idempotencyOK || operationID != identity.OperationID || idempotencyKey != identity.IdempotencyKey {
		return newError(CodeRequestInvalid, "Pinax mutation request identity does not match the retained binding", nil)
	}
	return nil
}

func ambiguousMutationError(err error) bool {
	var clientErr *Error
	if !errors.As(err, &clientErr) {
		return false
	}
	if clientErr.Code == CodeRequestFailed {
		return true
	}
	if clientErr.HTTPStatus >= http.StatusInternalServerError {
		return true
	}
	return clientErr.HTTPStatus >= http.StatusOK && clientErr.HTTPStatus < http.StatusMultipleChoices &&
		(clientErr.Code == CodeUpstreamInvalidResponse || clientErr.Code == CodeResponseTooLarge)
}

func mutationOutcomeUnknown(operationID string, ambiguousErr, recoveryErr error) error {
	return &Error{
		Code: CodeMutationOutcomeUnknown, Message: "Pinax mutation outcome could not be proven",
		RequiredAction: "Run pinax operation reconcile " + operationID + " --json; do not repeat the mutation",
		OperationRef:   operationID, ReconcileRequired: true,
		cause: errors.Join(ambiguousErr, recoveryErr),
	}
}

func mutationProjectionFromOperation(operation Operation, recovered bool) Projection {
	facts := map[string]string{
		"operation_id": operation.OperationID, "operation_status": operation.Status,
		"retryable": boolText(operation.Retryable), "replay_safe": boolText(operation.ReplaySafe),
		"reconcile_required": boolText(operation.ReconcileRequired),
		"idempotent_replay":  boolText(recovered), "recovered_from_operation": boolText(recovered),
	}
	for key, value := range map[string]string{
		"receipt_ref": operation.ReceiptRef, "resource_ref": operation.ResourceRef,
		"revision_before": operation.RevisionBefore, "revision_after": operation.RevisionAfter,
	} {
		if value != "" {
			facts[key] = value
		}
	}
	var result map[string]any
	if len(operation.Result) > 0 && json.Unmarshal(operation.Result, &result) == nil {
		for key, value := range result {
			if text, ok := value.(string); ok && text != "" {
				// 保留字合同字段（operation_status 等）来自已校验的 operation
				// 对象；owner 返回的 result 不得覆盖恢复语义。
				if _, reserved := facts[key]; !reserved {
					facts[key] = text
				}
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"operation": operation})
	return Projection{
		SpecVersion: ProjectionSpecVersion, Command: operation.CapabilityID, Status: "success",
		Summary: "Remote mutation was recovered from its durable operation.", Facts: facts, Data: data,
	}
}

func mutationProjectionError(operation Operation) (Projection, error) {
	projection := mutationProjectionFromOperation(operation, true)
	code := CodeReconcileRequired
	message := "Pinax mutation requires operation reconciliation"
	reconcileRequired := true
	if operation.Status == "failed" {
		code = "operation_failed"
		message = "Pinax mutation failed and is not safe to replay automatically"
		reconcileRequired = false
		if operation.Error != nil {
			if operation.Error.Code != "" {
				code = operation.Error.Code
			}
			if operation.Error.Message != "" {
				message = operation.Error.Message
			}
		}
	}
	requiredAction := "Run pinax operation reconcile " + operation.OperationID + " --json; do not repeat the mutation"
	projection.Status = "failed"
	projection.Error = &CommandError{Code: code, Message: message, Hint: requiredAction}
	return projection, &Error{
		Code: code, Message: message, Retryable: operation.Retryable,
		RequiredAction: requiredAction, OperationRef: operation.OperationID,
		ReconcileRequired: reconcileRequired,
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (c *Client) getProjection(ctx context.Context, endpointPath string) (Projection, error) {
	return c.doProjection(ctx, http.MethodGet, endpointPath, nil, "")
}

func (c *Client) doProjection(ctx context.Context, method, endpointPath string, body io.Reader, contentType string) (Projection, error) {
	endpoint, err := c.endpoint(endpointPath)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax request path is invalid", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax request could not be created", err)
	}
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		clientErr := newError(CodeRequestFailed, "Pinax owner request failed", err)
		clientErr.Retryable = true
		return Projection{}, clientErr
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return Projection{}, &Error{Code: CodeRedirectRejected, Message: "Pinax owner redirect was rejected", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return Projection{}, newError(CodeRequestFailed, "Pinax owner response could not be read", err)
	}
	if int64(len(encoded)) > c.maxResponseBytes {
		return Projection{}, &Error{Code: CodeResponseTooLarge, Message: "Pinax owner response exceeds the configured size limit", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	var projection Projection
	if err := json.Unmarshal(encoded, &projection); err != nil || projection.SpecVersion != ProjectionSpecVersion || projection.Command == "" || projection.Status == "" {
		return Projection{}, &Error{Code: CodeUpstreamInvalidResponse, Message: "Pinax owner returned an invalid projection", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || projection.Error != nil || projection.Status == "failed" {
		return projection, projectionClientError(projection, response)
	}
	return projection, nil
}

func (c *Client) endpoint(endpointPath string) (*url.URL, error) {
	if c == nil || c.baseURL == nil {
		return nil, fmt.Errorf("client is not configured")
	}
	if !strings.HasPrefix(endpointPath, "/") || strings.Contains(endpointPath, "?") || strings.Contains(endpointPath, "#") {
		return nil, fmt.Errorf("endpoint path must be absolute and query-free")
	}
	endpoint := *c.baseURL
	basePath := strings.TrimSuffix(endpoint.Path, "/")
	endpoint.Path = basePath + endpointPath
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return &endpoint, nil
}

func validateBaseURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("base URL must be absolute")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("base URL must not contain userinfo, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base URL scheme must be http or https")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return nil, fmt.Errorf("plain HTTP is allowed only for loopback hosts")
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || strings.Contains(decodedPath, "\x00") {
		return nil, fmt.Errorf("base URL path is invalid")
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == ".." || segment == "." {
			return nil, fmt.Errorf("base URL path traversal is not allowed")
		}
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(decodedPath, "/"))
	if cleaned == "/" {
		parsed.Path = ""
	} else {
		parsed.Path = strings.TrimSuffix(cleaned, "/")
	}
	parsed.RawPath = ""
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalized == "localhost" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func boundedSize(configured, fallback int64) (int64, error) {
	if configured == 0 {
		return fallback, nil
	}
	if configured < 0 || configured > maxConfiguredBodyBytes {
		return 0, fmt.Errorf("size must be between 1 and %d bytes", maxConfiguredBodyBytes)
	}
	return configured, nil
}

func decodeProjectionData(projection Projection, target any) error {
	if len(projection.Data) == 0 || bytes.Equal(bytes.TrimSpace(projection.Data), []byte("null")) {
		return newError(CodeUpstreamInvalidResponse, "Pinax owner projection has no typed data", nil)
	}
	if err := json.Unmarshal(projection.Data, target); err != nil {
		return newError(CodeUpstreamInvalidResponse, "Pinax owner projection data is invalid", err)
	}
	return nil
}

func projectionClientError(projection Projection, response *http.Response) *Error {
	clientErr := &Error{
		Code:       CodeRequestFailed,
		Message:    "Pinax owner request failed",
		HTTPStatus: response.StatusCode,
		Retryable:  retryableHTTPStatus(response.StatusCode),
		RequestID:  response.Header.Get("X-Request-ID"),
	}
	if projection.Error != nil {
		if projection.Error.Code != "" {
			clientErr.Code = projection.Error.Code
		}
		if projection.Error.Message != "" {
			clientErr.Message = projection.Error.Message
		}
		clientErr.RequiredAction = projection.Error.Hint
	}
	return clientErr
}

func retryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func withOperationReconcile(err error, operationID string) error {
	var clientErr *Error
	if errors.As(err, &clientErr) {
		copy := *clientErr
		copy.OperationRef = operationID
		copy.ReconcileRequired = true
		return &copy
	}
	return &Error{Code: CodeUpstreamInvalidResponse, Message: "Pinax operation response is invalid", OperationRef: operationID, ReconcileRequired: true, cause: err}
}

func validSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validReadiness(value Readiness) bool {
	if strings.TrimSpace(value.Status) == "" {
		return false
	}
	for _, maturity := range []string{"exploratory", "first-support", "mature"} {
		if value.Maturity == maturity {
			return true
		}
	}
	return false
}

func validOperationStatus(status string) bool {
	switch status {
	case "accepted", "applying", "succeeded", "failed", "reconcile_required":
		return true
	default:
		return false
	}
}
