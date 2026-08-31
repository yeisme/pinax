package pinaxclient

import "fmt"

const (
	CodeInvalidConfig           = "invalid_config"
	CodeRequestInvalid          = "request_invalid"
	CodeRequestFailed           = "request_failed"
	CodeRedirectRejected        = "redirect_rejected"
	CodeResponseTooLarge        = "response_too_large"
	CodeTokenFileInvalid        = "token_file_invalid"
	CodeTokenFileTooLarge       = "token_file_too_large"
	CodeTokenFileUnsafe         = "token_file_unsafe"
	CodeTokenFileUnreadable     = "token_file_unreadable"
	CodeUpstreamInvalidResponse = "upstream_invalid_response"
	CodeMutationOutcomeUnknown  = "mutation_outcome_unknown"
	CodeReconcileRequired       = "reconcile_required"
)

type Error struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	Retryable         bool   `json:"retryable"`
	RequiredAction    string `json:"required_action,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	OperationRef      string `json:"operation_ref,omitempty"`
	ReconcileRequired bool   `json:"reconcile_required"`
	cause             error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.HTTPStatus)
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newError(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}
