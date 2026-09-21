package judgment

import (
	"context"
	"errors"
	"time"
)

// ClientOptions configures a Client.
type ClientOptions struct {
	// KnownExtensions lists extension names this consumer understands.
	KnownExtensions []string
	// PrecisionPolicy validates result distributions. Defaults to
	// DefaultPrecisionPolicy when nil.
	PrecisionPolicy *PrecisionPolicy
	// GateOptions apply caller-side capability policy before submit.
	GateOptions GateOptions
	// StaticCapabilities lets offline consumers skip transport discovery.
	StaticCapabilities *Capabilities
}

// Client is the SDK facade. It never retries: one Evaluate maps to exactly
// one transport call, and post-submit uncertainty is surfaced as
// outcome_unknown / reconcile_first for the caller to decide.
type Client struct {
	transport Transport
	options   ClientOptions
	caps      *Capabilities
}

// NewClient builds a Client over an injected transport.
func NewClient(transport Transport, options ClientOptions) *Client {
	return &Client{transport: transport, options: options}
}

// Transport exposes the injected transport for optional extensions.
func (c *Client) Transport() Transport { return c.transport }

// Capabilities returns the negotiated capabilities, discovering them once
// through the transport unless static capabilities were configured.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, *Error) {
	if c.caps != nil {
		return c.caps, nil
	}
	if c.options.StaticCapabilities != nil {
		c.caps = c.options.StaticCapabilities
		return c.caps, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, deadlineError(err)
	}
	caps, err := c.transport.Capabilities(ctx)
	if err != nil {
		return nil, classifyTransportError(err, "capabilities_discovery_failed")
	}
	if caps == nil {
		return nil, NewError(CodeInvalidResponse, SubmissionNotSubmitted, RetryNever, "", ReasonCapabilitiesInvalid)
	}
	c.caps = caps
	return c.caps, nil
}

// Evaluate performs one explicitly bounded attempt: local validation,
// capability gate, then exactly one transport call and strict result
// validation. It returns either a validated result or a structured error.
func (c *Client) Evaluate(ctx context.Context, req *Request) (*Result, *Error) {
	if req == nil || req.tree == nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "request_not_parsed")
	}
	if _, verr := parseRequestTree(req.tree, RequestOptions{KnownExtensions: c.options.KnownExtensions}); verr != nil {
		return nil, verr.AsRequestError()
	}
	caps, aerr := c.Capabilities(ctx)
	if aerr != nil {
		return nil, aerr
	}
	if verr := CheckCapabilities(caps, req, c.options.GateOptions); verr != nil {
		return nil, verr.AsCapabilityError()
	}
	callCtx := ctx
	var cancel context.CancelFunc = func() {}
	if req.Limits != nil && req.Limits.DeadlineMS != nil {
		callCtx, cancel = context.WithTimeout(ctx, durationMS(*req.Limits.DeadlineMS))
	}
	defer cancel()
	if err := callCtx.Err(); err != nil {
		return nil, deadlineError(err)
	}
	res, err := c.transport.Evaluate(callCtx, req)
	if err != nil {
		// After the request left the caller's hands the outcome cannot be
		// proven un-submitted; classify as unknown, never auto-resend.
		return nil, classifyTransportError(err, "evaluate_failed")
	}
	if res == nil {
		return nil, NewError(CodeInvalidResponse, SubmissionSubmitted, RetryReconcileFirst, "", "empty_result")
	}
	wire, merr := res.MarshalResult()
	if merr != nil {
		return nil, NewError(CodeInvalidResponse, SubmissionSubmitted, RetryReconcileFirst, "", "result_wire_unavailable")
	}
	policy := c.options.PrecisionPolicy
	if policy == nil {
		p := DefaultPrecisionPolicy()
		policy = &p
	}
	parsed, verr := ParseResult(wire, req, *policy)
	if verr != nil {
		return nil, verr.AsResponseError()
	}
	return parsed, nil
}

// Reconcile queries an existing attempt when, and only when, the capability
// is declared and the transport supports it. It never resubmits.
func (c *Client) Reconcile(ctx context.Context, attemptID string) (*Result, *Error) {
	caps, aerr := c.Capabilities(ctx)
	if aerr != nil {
		return nil, aerr
	}
	if !caps.SupportsReconcile {
		return nil, NewError(CodeUnsupportedCapability, SubmissionNotSubmitted, RetryNever, "", "reconcile_not_supported")
	}
	rt, ok := c.transport.(ReconcilingTransport)
	if !ok {
		return nil, NewError(CodeUnsupportedCapability, SubmissionNotSubmitted, RetryNever, "", "transport_cannot_reconcile")
	}
	res, err := rt.Reconcile(ctx, attemptID)
	if err != nil {
		return nil, classifyTransportError(err, "reconcile_failed")
	}
	return res, nil
}

// Cancel requests a declared remote cancel; local aborts are not remote
// cancels and never imply billing stopped.
func (c *Client) Cancel(ctx context.Context, attemptID string) *Error {
	caps, aerr := c.Capabilities(ctx)
	if aerr != nil {
		return aerr
	}
	if !caps.SupportsCancel {
		return NewError(CodeUnsupportedCapability, SubmissionNotSubmitted, RetryNever, "", "cancel_not_supported")
	}
	ct, ok := c.transport.(CancelingTransport)
	if !ok {
		return NewError(CodeUnsupportedCapability, SubmissionNotSubmitted, RetryNever, "", "transport_cannot_cancel")
	}
	if err := ct.Cancel(ctx, attemptID); err != nil {
		return classifyTransportError(err, "cancel_failed")
	}
	return nil
}

// classifyTransportError keeps already-classified *Error envelopes and maps
// raw infrastructure failures to a fail-closed outcome: the attempt is
// treated as possibly submitted and the caller must reconcile first.
func classifyTransportError(err error, detail string) *Error {
	var structured *Error
	if errors.As(err, &structured) {
		return structured
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(CodeOutcomeUnknown, SubmissionUnknown, RetryReconcileFirst, "", detail+":deadline")
	}
	if errors.Is(err, context.Canceled) {
		return NewError(CodeOutcomeUnknown, SubmissionUnknown, RetryReconcileFirst, "", detail+":canceled")
	}
	return NewError(CodeOutcomeUnknown, SubmissionUnknown, RetryReconcileFirst, "", detail)
}

func deadlineError(err error) *Error {
	if errors.Is(err, context.Canceled) {
		return NewError(CodeDeadlineExceeded, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "context_canceled_before_dispatch")
	}
	return NewError(CodeDeadlineExceeded, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "deadline_before_dispatch")
}

func durationMS(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
