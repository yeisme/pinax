package judgment

import "fmt"

// Stable error codes of the structured judgment contract.
const (
	CodeUnsupportedCapability = "unsupported_capability"
	CodeInvalidRequest        = "invalid_request"
	CodeUnauthorized          = "unauthorized"
	CodeRateLimited           = "rate_limited"
	CodeUnavailable           = "unavailable"
	CodeDeadlineExceeded      = "deadline_exceeded"
	CodeInvalidResponse       = "invalid_response"
	CodeOutcomeUnknown        = "outcome_unknown"
)

// SubmissionState tells whether the attempt reached the executor.
const (
	SubmissionNotSubmitted = "not_submitted"
	SubmissionSubmitted    = "submitted"
	SubmissionUnknown      = "unknown"
)

// RetryClass is the SDK's advisory classification; the SDK never retries.
const (
	RetrySafeBeforeSubmit = "safe_before_submit"
	RetryReconcileFirst   = "reconcile_first"
	RetryNever            = "never"
)

// Error is the structured, redacted error envelope of the SDK. DiagnosticRef
// may carry an opaque reference; Detail carries only bounded reason tokens,
// never provider bodies or raw payloads.
type Error struct {
	Code            string
	SubmissionState string
	RetryClass      string
	DiagnosticRef   string
	Detail          string
}

func (e *Error) Error() string {
	return fmt.Sprintf("judgment: %s (submission_state=%s retry_class=%s): %s",
		e.Code, e.SubmissionState, e.RetryClass, e.Detail)
}

// NewError builds a contract-valid Error; unknown enum values are rejected
// so no implementation can invent new classes by accident.
func NewError(code, submissionState, retryClass, diagnosticRef, detail string) *Error {
	if !validCode(code) || !validSubmissionState(submissionState) || !validRetryClass(retryClass) {
		return &Error{
			Code:            CodeInvalidResponse,
			SubmissionState: SubmissionUnknown,
			RetryClass:      RetryReconcileFirst,
			DiagnosticRef:   diagnosticRef,
			Detail:          "invalid_error_envelope",
		}
	}
	return &Error{
		Code:            code,
		SubmissionState: submissionState,
		RetryClass:      retryClass,
		DiagnosticRef:   diagnosticRef,
		Detail:          detail,
	}
}

func validCode(c string) bool {
	switch c {
	case CodeUnsupportedCapability, CodeInvalidRequest, CodeUnauthorized,
		CodeRateLimited, CodeUnavailable, CodeDeadlineExceeded,
		CodeInvalidResponse, CodeOutcomeUnknown:
		return true
	}
	return false
}

func validSubmissionState(s string) bool {
	return s == SubmissionNotSubmitted || s == SubmissionSubmitted || s == SubmissionUnknown
}

func validRetryClass(r string) bool {
	return r == RetrySafeBeforeSubmit || r == RetryReconcileFirst || r == RetryNever
}

// ValidationError describes a local contract violation with a stable reason
// token. Reason tokens are shared with the TypeScript implementation and the
// conformance vectors.
type ValidationError struct {
	Reason string
	Detail string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("judgment: validation failed: %s: %s", e.Reason, e.Detail)
}

func validationErr(reason, detail string) *ValidationError {
	return &ValidationError{Reason: reason, Detail: detail}
}

// AsRequestError converts a ValidationError into the structured error the
// client surfaces for a rejected request (always pre-submission).
func (e *ValidationError) AsRequestError() *Error {
	return NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", e.Reason)
}

// AsResponseError converts a ValidationError into the structured error the
// client surfaces for an unusable response (the attempt did reach the
// executor, so callers must reconcile before any new attempt).
func (e *ValidationError) AsResponseError() *Error {
	return NewError(CodeInvalidResponse, SubmissionSubmitted, RetryReconcileFirst, "", e.Reason)
}

// AsCapabilityError converts a ValidationError produced by the capability
// gate into an unsupported_capability error.
func (e *ValidationError) AsCapabilityError() *Error {
	return NewError(CodeUnsupportedCapability, SubmissionNotSubmitted, RetryNever, "", e.Reason)
}
