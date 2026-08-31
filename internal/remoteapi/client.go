package remoteapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

const defaultTimeout = 15 * time.Second

// Config is the legacy internal facade configuration. New callers should use
// pinaxclient.Config directly; these fields remain additive and source
// compatible while existing Pinax integrations migrate.
type Config struct {
	BaseURL    string
	Token      string
	TokenFile  string
	HTTPClient *http.Client
}

type RPCRequest = pinaxclient.RPCRequest

type MutationIdentity = pinaxclient.MutationIdentity

// Client is a compatibility facade over pkg/pinaxclient. It intentionally
// keeps the old constructor and domain.Projection surface while delegating all
// transport, redirect, size, timeout and credential policy to the public SDK.
type Client struct {
	sdk     *pinaxclient.Client
	initErr error
	http    *http.Client
}

func NewClient(config Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	sdk, err := pinaxclient.New(pinaxclient.Config{
		BaseURL:    config.BaseURL,
		Token:      config.Token,
		TokenFile:  config.TokenFile,
		HTTPClient: httpClient,
	})
	return &Client{sdk: sdk, initErr: err, http: httpClient}
}

func (c *Client) Ping(ctx context.Context) (domain.Projection, error) {
	if c == nil || c.initErr != nil {
		return Adapt(pinaxclient.Projection{}, clientInitError(c))
	}
	projection, err := c.sdk.Ping(ctx)
	return Adapt(projection, err)
}

func (c *Client) Capabilities(ctx context.Context) (domain.Projection, error) {
	if c == nil || c.initErr != nil {
		return Adapt(pinaxclient.Projection{}, clientInitError(c))
	}
	projection, err := c.sdk.Capabilities(ctx)
	return Adapt(projection, err)
}

func (c *Client) Call(ctx context.Context, request RPCRequest) (domain.Projection, error) {
	if c == nil || c.initErr != nil {
		return Adapt(pinaxclient.Projection{}, clientInitError(c))
	}
	projection, err := c.sdk.CallRPC(ctx, request)
	return Adapt(projection, err)
}

// CallMutation retains the compatibility facade while exposing the public
// SDK's no-blind-retry operation recovery behavior to legacy callers.
func (c *Client) CallMutation(ctx context.Context, request RPCRequest, identity MutationIdentity) (domain.Projection, error) {
	if c == nil || c.initErr != nil {
		return Adapt(pinaxclient.Projection{}, clientInitError(c))
	}
	projection, err := c.sdk.CallMutationRPC(ctx, request, identity)
	return Adapt(projection, err)
}

// Adapt converts the public wire projection and typed client error into the
// legacy domain projection/error contract used by existing CLI renderers.
func Adapt(projection pinaxclient.Projection, callErr error) (domain.Projection, error) {
	adapted := domain.Projection{
		SpecVersion: projection.SpecVersion,
		Mode:        projection.Mode,
		Command:     projection.Command,
		Status:      projection.Status,
		Summary:     projection.Summary,
		Facts:       projection.Facts,
		Evidence:    append([]string(nil), projection.Evidence...),
	}
	for _, action := range projection.Actions {
		adapted.Actions = append(adapted.Actions, domain.Action{Name: action.Name, Command: action.Command})
	}
	for _, warning := range projection.Warnings {
		adapted.Warnings = append(adapted.Warnings, domain.ProjectionWarning{Code: warning.Code, Message: warning.Message, Hint: warning.Hint})
	}
	if len(projection.Data) > 0 && string(projection.Data) != "null" {
		var data any
		if err := json.Unmarshal(projection.Data, &data); err == nil {
			adapted.Data = data
		}
	}
	if projection.Error != nil {
		adapted.Error = &domain.CommandError{Code: projection.Error.Code, Message: projection.Error.Message, Hint: projection.Error.Hint}
		if adapted.Status == "" {
			adapted.Status = "failed"
		}
		if adapted.Command == "" {
			adapted.Command = "remote.api"
		}
		if adapted.SpecVersion == "" {
			adapted.SpecVersion = pinaxclient.ProjectionSpecVersion
		}
		return adapted, adapted.Error
	}
	if callErr == nil {
		return adapted, nil
	}
	commandErr := legacyClientError(callErr)
	failed := domain.NewErrorProjection("remote.api", commandErr)
	var clientErr *pinaxclient.Error
	if errors.As(callErr, &clientErr) {
		if clientErr.OperationRef != "" {
			failed.Facts["operation_id"] = clientErr.OperationRef
		}
		if clientErr.ReconcileRequired {
			failed.Facts["reconcile_required"] = "true"
		}
	}
	return failed, commandErr
}

func clientInitError(client *Client) error {
	if client == nil {
		return &pinaxclient.Error{Code: pinaxclient.CodeInvalidConfig, Message: "Pinax client is not configured"}
	}
	return client.initErr
}

func legacyClientError(err error) *domain.CommandError {
	var clientErr *pinaxclient.Error
	if !errors.As(err, &clientErr) {
		return &domain.CommandError{Code: "remote_api_request_failed", Message: "Remote API request failed", Hint: "Check that pinax api serve is running and reachable"}
	}
	if clientErr.ReconcileRequired {
		hint := clientErr.RequiredAction
		if hint == "" {
			hint = "Inspect and reconcile the retained operation; do not repeat the mutation"
		}
		return &domain.CommandError{Code: clientErr.Code, Message: clientErr.Message, Hint: hint}
	}
	switch clientErr.Code {
	case pinaxclient.CodeInvalidConfig:
		return &domain.CommandError{Code: "remote_api_invalid_config", Message: "Remote API URL or credential configuration is invalid", Hint: "Use an HTTPS URL or loopback HTTP URL and one credential source"}
	case pinaxclient.CodeRequestInvalid:
		return &domain.CommandError{Code: "remote_api_request_invalid", Message: "Remote API request could not be encoded", Hint: "Check the command arguments"}
	case pinaxclient.CodeRequestFailed:
		return &domain.CommandError{Code: "remote_api_request_failed", Message: "Remote API request failed", Hint: "Check that pinax api serve is running and reachable"}
	case pinaxclient.CodeTokenFileInvalid, pinaxclient.CodeTokenFileTooLarge, pinaxclient.CodeTokenFileUnsafe, pinaxclient.CodeTokenFileUnreadable:
		return &domain.CommandError{Code: "remote_api_token_unreadable", Message: "Remote API token file could not be read securely", Hint: "Use an absolute owner-only token file"}
	default:
		return &domain.CommandError{Code: "remote_api_invalid_response", Message: "Remote API returned an invalid response", Hint: "Retry the command or check server logs"}
	}
}
