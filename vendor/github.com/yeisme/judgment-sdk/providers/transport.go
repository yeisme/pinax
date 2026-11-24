package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	judgment "github.com/yeisme/judgment-sdk"
	or "github.com/yeisme/judgment-sdk/providers/openrouter-decisions"
	ts "github.com/yeisme/judgment-sdk/providers/typesafe-judgment"
)

type Options struct {
	Profile     ExecutionProfile
	Credentials Resolver
	// Authorize rechecks owner scope, source permissions and current policy.
	Authorize func(context.Context, *judgment.Request) error
	// RoundTripper permits instrumentation and local fake tests, not endpoint overrides.
	RoundTripper http.RoundTripper
}

type Transport struct{ opts Options }

func New(opts Options) (*Transport, error) {
	if err := opts.Profile.Validate(); err != nil {
		return nil, err
	}
	if opts.Authorize == nil {
		return nil, errors.New("judgment_authorizer_required")
	}
	return &Transport{opts: opts}, nil
}

func (t *Transport) Capabilities(context.Context) (*judgment.Capabilities, error) {
	switch t.opts.Profile.Provider {
	case "typesafe":
		return ts.Capabilities(), nil
	case "openrouter":
		return or.Capabilities(), nil
	default:
		caps := *ts.Capabilities()
		provider := "openai"
		caps.Adapter = judgment.Adapter{Name: "openai-decisions-sdk", Version: "0.2.0"}
		caps.Model = judgment.ModelIdentity{TransportProvider: provider, ModelProvider: &provider, RequestedModel: "gpt-6-luna", PinLevel: judgment.PinProviderExactModel}
		caps.Modalities = []string{"text", "image", "json"}
		caps.MaxQuestions = 256
		caps.MaxCandidates = 64
		caps.MaxSources = 256
		caps.MaxInlineTextBytes = 4 << 20
		caps.Extensions = []string{InputsExtension}
		return &caps, nil
	}
}

func reject(code, reason string) *judgment.Error {
	return judgment.NewError(code, judgment.SubmissionNotSubmitted, judgment.RetryNever, "", reason)
}
func invalid(reason string) *judgment.Error {
	return judgment.NewError(judgment.CodeInvalidResponse, judgment.SubmissionSubmitted, judgment.RetryReconcileFirst, "", reason)
}

func (t *Transport) Evaluate(ctx context.Context, original *judgment.Request) (*judgment.Result, error) {
	if !t.opts.Profile.Enabled {
		return nil, reject(judgment.CodeUnauthorized, "judgment_profile_disabled")
	}
	if original == nil {
		return nil, reject(judgment.CodeInvalidRequest, "request_missing")
	}
	wire, err := original.WireJSON()
	if err != nil {
		return nil, reject(judgment.CodeInvalidRequest, "request_invalid")
	}
	req, ve := judgment.ParseRequest(wire, judgment.RequestOptions{KnownExtensions: []string{InputsExtension}})
	if ve != nil {
		return nil, ve.AsRequestError()
	}
	caps, _ := t.Capabilities(ctx)
	if req.Model.TransportProvider != caps.Model.TransportProvider || req.Model.RequestedModel != caps.Model.RequestedModel || req.Model.PinLevel != caps.Model.PinLevel || req.Model.UnderlyingRevision != nil || req.Model.UnderlyingRevisionVerified || (req.Model.ModelProvider != nil && *req.Model.ModelProvider != *caps.Model.ModelProvider) {
		return nil, reject(judgment.CodeUnsupportedCapability, "model_identity_not_supported")
	}
	if ve := judgment.CheckCapabilities(caps, req, judgment.GateOptions{}); ve != nil {
		return nil, ve.AsCapabilityError()
	}
	if err := t.opts.Authorize(ctx, req); err != nil {
		return nil, reject(judgment.CodeUnauthorized, "owner_authorization_denied")
	}
	endpoint := "https://api.openai.com/v1/decisions"
	maxPairs, maxBytes := 400, 4<<20
	var payload map[string]any
	var normalize func(map[string]any) (map[string]any, *judgment.Error)
	switch t.opts.Profile.Provider {
	case "typesafe":
		maxPairs, maxBytes = ts.MaxPairs, ts.MaxPayloadBytes
		endpoint = ts.UpstreamEndpoint
		for _, s := range req.Sources {
			if s.InlineText == nil {
				return nil, reject(judgment.CodeUnsupportedCapability, "text_inline_source_required")
			}
		}
		for _, q := range req.Questions {
			if len(q.OrdinalLevels) > ts.MaxScoreLevels {
				return nil, reject(judgment.CodeUnsupportedCapability, "score_levels_over_bound")
			}
		}
		plan := ts.BuildPlan(req)
		payload = ts.BuildUpstreamPayload(req, plan)
		normalize = func(d map[string]any) (map[string]any, *judgment.Error) { return ts.NormalizeResponse(req, plan, d) }
	case "openrouter":
		maxPairs, maxBytes = or.MaxPairs, or.MaxPayloadBytes
		endpoint = or.UpstreamEndpoint
		for _, s := range req.Sources {
			if s.InlineText == nil {
				return nil, reject(judgment.CodeUnsupportedCapability, "text_inline_source_required")
			}
		}
		for _, q := range req.Questions {
			if len(q.OrdinalLevels) > or.MaxScoreLevels || len(q.ChoiceOptions) > or.MaxChoiceOption {
				return nil, reject(judgment.CodeUnsupportedCapability, "answer_domain_over_bound")
			}
		}
		plan := or.BuildPlan(req)
		for _, n := range or.InstructionRunes(req, plan) {
			if n > or.MaxInstructionRunes {
				return nil, reject(judgment.CodeInvalidRequest, "instruction_over_bound")
			}
		}
		payload = or.BuildUpstreamPayload(req, plan)
		normalize = func(d map[string]any) (map[string]any, *judgment.Error) { return or.NormalizeResponse(req, plan, d) }
	default:
		var e *judgment.Error
		payload, e = openAIPayload(req)
		if e != nil {
			return nil, e
		}
		normalize = func(d map[string]any) (map[string]any, *judgment.Error) { return normalizeOpenAI(req, d) }
	}
	if len(req.ExpectedPairs()) > maxPairs {
		return nil, reject(judgment.CodeInvalidRequest, "expanded_question_limit")
	}
	body, err := json.Marshal(payload)
	if err != nil || len(body) > maxBytes {
		return nil, reject(judgment.CodeInvalidRequest, "payload_over_bound")
	}
	deadline := 20 * time.Second
	if req.Limits != nil && req.Limits.DeadlineMS != nil {
		deadline = time.Duration(*req.Limits.DeadlineMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	if ctx.Err() != nil {
		return nil, reject(judgment.CodeDeadlineExceeded, "deadline_before_dispatch")
	}
	secret, err := t.opts.Credentials.Resolve(ctx, t.opts.Profile.Credential)
	if err != nil || strings.TrimSpace(secret) == "" || strings.ContainsAny(secret, "\r\n\t ") {
		return nil, reject(judgment.CodeUnauthorized, "credential_unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, reject(judgment.CodeInvalidRequest, "request_build_failed")
	}
	// Disable net/http replay of a body on transport errors. No upstream
	// idempotency header is invented for endpoints without that guarantee.
	request.GetBody = nil
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := http.Client{Transport: t.opts.RoundTripper, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	resp, err := client.Do(request)
	if err != nil {
		return nil, judgment.NewError(judgment.CodeOutcomeUnknown, judgment.SubmissionUnknown, judgment.RetryReconcileFirst, "", "provider_dispatch_unknown")
	}
	defer resp.Body.Close()
	limit := int64(1 << 20)
	if req.Limits != nil && req.Limits.MaxOutputBytes != nil && *req.Limits.MaxOutputBytes < limit {
		limit = *req.Limits.MaxOutputBytes
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, invalid("provider_body_unavailable")
	}
	if resp.StatusCode != 200 {
		code := judgment.CodeUnavailable
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			code = judgment.CodeUnauthorized
		}
		if resp.StatusCode == 429 {
			code = judgment.CodeRateLimited
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 408 {
			return nil, reject(code, "provider_rejected")
		}
		return nil, judgment.NewError(code, judgment.SubmissionUnknown, judgment.RetryReconcileFirst, "", "provider_outcome_unknown")
	}
	tree, err := judgment.ParseTree(raw)
	if err != nil {
		return nil, invalid("provider_json_invalid")
	}
	doc, ok := tree.(map[string]any)
	if !ok {
		return nil, invalid("provider_response_invalid")
	}
	out, e := normalize(doc)
	if e != nil {
		return nil, e
	}
	out["latency_ms"] = time.Since(start).Milliseconds()
	// Headers are not trusted diagnostics: only bounded identifier syntax survives.
	if id := resp.Header.Get("X-Request-ID"); safeRef.MatchString(id) {
		out["provider_request_id"] = id
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, invalid("result_encoding_failed")
	}
	result, ve := judgment.ParseResult(b, req, judgment.DefaultPrecisionPolicy())
	if ve != nil {
		return nil, invalid("result_conformance_failed")
	}
	return result, nil
}
