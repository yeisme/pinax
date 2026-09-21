package judgment

import "context"

// Transport is the injected execution boundary. Implementations own network
// access and credentials; the SDK itself holds none. Returned errors are
// either *Error (already classified) or raw infrastructure errors; the client
// maps raw post-dispatch failures to outcome_unknown instead of guessing.
type Transport interface {
	Capabilities(ctx context.Context) (*Capabilities, error)
	Evaluate(ctx context.Context, req *Request) (*Result, error)
}

// ReconcilingTransport is an optional transport extension: query an existing
// attempt without resubmitting it.
type ReconcilingTransport interface {
	Transport
	Reconcile(ctx context.Context, attemptID string) (*Result, error)
}

// CancelingTransport is an optional transport extension: request a declared
// remote cancel. A local Abort never implies the provider stopped or that
// billing ceased.
type CancelingTransport interface {
	Transport
	Cancel(ctx context.Context, attemptID string) error
}
