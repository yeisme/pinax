package judgment

import (
	"context"
	"sync"
)

// FixtureTransport is an in-memory Transport for offline consumers and
// conformance runs: no network, no credentials, deterministic scripting.
// Counters prove call counts (for example: zero provider calls on replay).
type FixtureTransport struct {
	mu sync.Mutex

	caps Capabilities

	// EvaluateFn scripts the single evaluate call the client is allowed to
	// make. Returning an *Error surfaces it verbatim; returning a result
	// that fails validation surfaces as invalid_response.
	EvaluateFn func(ctx context.Context, req *Request) (*Result, error)

	// CapabilitiesFn optionally scripts capability discovery.
	CapabilitiesFn func(ctx context.Context) (*Capabilities, error)

	CapabilitiesCalls int
	EvaluateCalls     int
}

// NewFixtureTransport builds a fixture transport with static capabilities.
func NewFixtureTransport(caps Capabilities, fn func(ctx context.Context, req *Request) (*Result, error)) *FixtureTransport {
	return &FixtureTransport{caps: caps, EvaluateFn: fn}
}

// Capabilities records the call and returns static capabilities.
func (f *FixtureTransport) Capabilities(ctx context.Context) (*Capabilities, error) {
	f.mu.Lock()
	f.CapabilitiesCalls++
	fn := f.CapabilitiesFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	caps := f.caps
	return &caps, nil
}

// Evaluate records the call and delegates to the scripted behavior.
func (f *FixtureTransport) Evaluate(ctx context.Context, req *Request) (*Result, error) {
	f.mu.Lock()
	f.EvaluateCalls++
	fn := f.EvaluateFn
	f.mu.Unlock()
	if fn == nil {
		return nil, NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "fixture_not_scripted")
	}
	return fn(ctx, req)
}
