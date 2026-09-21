package judgment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// HTTPDoer abstracts the HTTP client so callers inject authorization and
// instrumentation. The SDK itself never stores credentials.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

const (
	capabilitiesPath = "/v1/judgments/capabilities"
	evaluatePath     = "/v1/judgments/evaluate"
	defaultMaxOutput = 1 << 20
)

// HTTPTransportOptions configures the HTTP transport.
type HTTPTransportOptions struct {
	// BaseURL of the adapter, for example "https://adapter.example".
	BaseURL string
	// Doer performs requests. When nil a redirect-refusing client is used.
	Doer HTTPDoer
	// Authorize mutates each request (for example adding Authorization).
	// Returning an error aborts the request before dispatch. The SDK never
	// reads credentials itself.
	Authorize func(*http.Request) error
	// MaxOutputBytes caps response bodies (default 1 MiB).
	MaxOutputBytes int64
	// UserAgent defaults to "yeisme-judgment-sdk-go".
	UserAgent string
}

// HTTPTransport speaks the judgment HTTP contract:
// GET /v1/judgments/capabilities and POST /v1/judgments/evaluate. Redirects
// are never followed, so Authorization can never be replayed elsewhere, and
// error bodies are never surfaced — only status-derived classifications and
// a redacted digest reference.
type HTTPTransport struct {
	baseURL        *url.URL
	doer           HTTPDoer
	authorize      func(*http.Request) error
	maxOutputBytes int64
	userAgent      string
}

// NewHTTPTransport validates options and builds the transport.
func NewHTTPTransport(opts HTTPTransportOptions) (*HTTPTransport, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("judgment: invalid base url")
	}
	doer := opts.Doer
	if doer == nil {
		doer = &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	maxOutput := opts.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultMaxOutput
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = "yeisme-judgment-sdk-go"
	}
	return &HTTPTransport{
		baseURL:        parsed,
		doer:           doer,
		authorize:      opts.Authorize,
		maxOutputBytes: maxOutput,
		userAgent:      ua,
	}, nil
}

// Capabilities fetches GET /v1/judgments/capabilities.
func (t *HTTPTransport) Capabilities(ctx context.Context) (*Capabilities, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url(capabilitiesPath), nil)
	if err != nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "request_build_failed")
	}
	req.Header.Set("Accept", "application/json")
	if aerr := t.decorate(req); aerr != nil {
		return nil, aerr
	}
	body, status, ref, terr := t.roundTrip(req)
	if terr != nil {
		return nil, terr
	}
	if status != http.StatusOK {
		return nil, NewError(CodeInvalidResponse, SubmissionNotSubmitted, RetrySafeBeforeSubmit, ref, "capabilities_status_"+strconv.Itoa(status))
	}

	caps, verr := ParseCapabilities(body)
	if verr != nil {
		return nil, NewError(CodeInvalidResponse, SubmissionNotSubmitted, RetrySafeBeforeSubmit, ref, verr.Reason)
	}
	return caps, nil
}

// Evaluate posts exactly one evaluate request; it never retries. Results are
// parsed with the default precision policy here; the client re-validates
// with its configured policy.
func (t *HTTPTransport) Evaluate(ctx context.Context, jr *Request) (*Result, error) {
	wire, err := jr.WireJSON()
	if err != nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "request_wire_unavailable")
	}
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, t.url(evaluatePath), bytes.NewReader(wire))
	if reqErr != nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "request_build_failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if aerr := t.decorate(req); aerr != nil {
		return nil, aerr
	}
	body, status, ref, terr := t.roundTrip(req)
	if terr != nil {
		return nil, terr
	}
	if status != http.StatusOK {
		return nil, classifyHTTPStatus(status, ref)
	}
	result, verr := ParseResult(body, jr, DefaultPrecisionPolicy())
	if verr != nil {
		return nil, verr.AsResponseError()
	}
	return result, nil
}

func (t *HTTPTransport) url(path string) string {
	joined := *t.baseURL
	joined.Path = strings.TrimRight(joined.Path, "/") + path
	return joined.String()
}

func (t *HTTPTransport) decorate(req *http.Request) *Error {
	req.Header.Set("User-Agent", t.userAgent)
	if t.authorize == nil {
		return nil
	}
	if err := t.authorize(req); err != nil {
		return NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "authorize_failed")
	}
	return nil
}

// bodyRef derives a redacted diagnostic reference from withheld content.
func bodyRef(status int, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("http:%d:sha256:%s", status, hex.EncodeToString(sum[:6]))
}

// roundTrip performs the HTTP exchange and returns the bounded body, the
// status, a redacted diagnostic reference, and an error that is either a
// classified *Error or a raw infrastructure error (which the client maps to
// outcome_unknown).
func (t *HTTPTransport) roundTrip(req *http.Request) ([]byte, int, string, error) {
	resp, err := t.doer.Do(req)
	if err != nil {
		if isPreConnectError(err) {
			// Dial failures provably never delivered the request.
			return nil, 0, "transport:dial_failed",
				NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "transport:dial_failed", "dial_failed")
		}
		// Timeouts, resets, TLS failures after dispatch: the outcome cannot
		// be proven; propagate raw so the client fail-closes to
		// outcome_unknown without leaking anything.
		return nil, 0, "transport:dispatch_unknown", err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, t.maxOutputBytes+1))
	if readErr != nil {
		return nil, resp.StatusCode, "transport:body_read_failed",
			NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, "transport:body_read_failed", "body_read_failed")
	}
	if int64(len(body)) > t.maxOutputBytes {
		return nil, resp.StatusCode, bodyRef(resp.StatusCode, nil),
			NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, bodyRef(resp.StatusCode, nil), "response_oversize")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Redirect refused: the request was not executed and Authorization
		// was never replayed to the redirect target.
		return nil, resp.StatusCode, "http:redirect_refused",
			NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "http:redirect_refused", "redirect_refused")
	}
	if resp.StatusCode != http.StatusOK {
		// Error body content is withheld; only the classification and a
		// digest reference survive.
		return nil, resp.StatusCode, bodyRef(resp.StatusCode, body), nil
	}
	return body, resp.StatusCode, "http:200", nil
}

func isPreConnectError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	return false
}

func classifyHTTPStatus(status int, diagnosticRef string) *Error {
	switch {
	case status == http.StatusRequestTimeout:
		return NewError(CodeOutcomeUnknown, SubmissionUnknown, RetryReconcileFirst, diagnosticRef, "http_408")
	case status == http.StatusTooManyRequests:
		return NewError(CodeRateLimited, SubmissionNotSubmitted, RetrySafeBeforeSubmit, diagnosticRef, "http_429")
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return NewError(CodeUnauthorized, SubmissionNotSubmitted, RetrySafeBeforeSubmit, diagnosticRef, "http_"+strconv.Itoa(status))
	case status >= 500:
		// A 5xx (including 503) cannot prove the request was not executed.
		return NewError(CodeUnavailable, SubmissionUnknown, RetryReconcileFirst, diagnosticRef, "http_5xx")
	case status >= 400:
		return NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, diagnosticRef, "http_4xx")
	default:
		return NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, diagnosticRef, "http_"+strconv.Itoa(status))
	}
}
