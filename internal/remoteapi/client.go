package remoteapi

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

	"github.com/yeisme/pinax/internal/domain"
)

const (
	maxResponseBytes int64 = 4 << 20
	defaultTimeout         = 15 * time.Second
)

type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type RPCRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(config Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"), token: config.Token, http: httpClient}
}

func (c *Client) Ping(ctx context.Context) (domain.Projection, error) {
	return c.get(ctx, "/")
}

func (c *Client) Capabilities(ctx context.Context) (domain.Projection, error) {
	return c.get(ctx, "/v1/capabilities")
}

func (c *Client) get(ctx context.Context, path string) (domain.Projection, error) {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return remoteError("remote_api_invalid_config", "Remote API URL must be http or https", "Use --api-url http://127.0.0.1:<port>"), err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return remoteError("remote_api_invalid_config", "Remote API URL is invalid", "Use --api-url http://127.0.0.1:<port>"), err
	}
	req.Header.Set("Accept", "application/json")
	return c.do(req)
}

func (c *Client) Call(ctx context.Context, rpc RPCRequest) (domain.Projection, error) {
	endpoint, err := c.endpoint("/v1/rpc")
	if err != nil {
		return remoteError("remote_api_invalid_config", "Remote API URL must be http or https", "Use --api-url http://127.0.0.1:<port>"), err
	}
	payload, err := json.Marshal(rpc)
	if err != nil {
		commandErr := &domain.CommandError{Code: "remote_api_request_invalid", Message: "Remote API request could not be encoded"}
		return domain.NewErrorProjection("remote.api", commandErr), commandErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return remoteError("remote_api_invalid_config", "Remote API URL is invalid", "Use --api-url http://127.0.0.1:<port>"), err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return c.do(req)
}

func (c *Client) do(req *http.Request) (domain.Projection, error) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return remoteError("remote_api_request_failed", "Remote API request failed", "Check that pinax api serve is running and reachable"), err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || int64(len(body)) > maxResponseBytes {
		commandErr := &domain.CommandError{Code: "remote_api_invalid_response", Message: "Remote API response could not be read", Hint: "Retry the command or check server logs"}
		return domain.NewErrorProjection("remote.api", commandErr), commandErr
	}
	var projection domain.Projection
	if err := json.Unmarshal(body, &projection); err != nil || projection.Command == "" || projection.Status == "" {
		commandErr := &domain.CommandError{Code: "remote_api_invalid_response", Message: "Remote API returned an invalid Projection envelope", Hint: "Check that --api-url points to a Pinax API server"}
		return domain.NewErrorProjection("remote.api", commandErr), commandErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if projection.Error != nil {
			return projection, projection.Error
		}
		commandErr := &domain.CommandError{Code: "remote_api_request_failed", Message: fmt.Sprintf("Remote API returned HTTP %d", resp.StatusCode)}
		projection.Error = commandErr
		projection.Status = "failed"
		return projection, commandErr
	}
	if projection.Error != nil {
		return projection, projection.Error
	}
	return projection, nil
}

func (c *Client) endpoint(path string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("missing remote API URL")
	}
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported remote API scheme")
	}
	return c.baseURL + path, nil
}

func remoteError(code, message, hint string) domain.Projection {
	return domain.NewErrorProjection("remote.api", &domain.CommandError{Code: code, Message: message, Hint: hint})
}
