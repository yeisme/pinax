package promptrepo

import "fmt"

const (
	CodeInvalidRequest          = "INVALID_REQUEST"
	CodeNotFound                = "NOT_FOUND"
	CodeAlreadyExists           = "ALREADY_EXISTS"
	CodeUnsupportedSourceScheme = "UNSUPPORTED_SOURCE_SCHEME"
	CodeAuthRequired            = "AUTH_REQUIRED"
	CodeAuthorizationFailed     = "AUTHORIZATION_FAILED"
	CodeSourceFetchFailed       = "SOURCE_FETCH_FAILED"
	CodeDigestMismatch          = "DIGEST_MISMATCH"
	CodeStateLocked             = "STATE_LOCKED"
	CodeStateSchemaTooNew       = "STATE_SCHEMA_TOO_NEW"
	CodeIncompatible            = "INCOMPATIBLE"
	CodeRightsBlocked           = "RIGHTS_BLOCKED"
	CodeAddressMismatch         = "ADDRESS_MISMATCH"
	CodeInputRequired           = "INPUT_REQUIRED"
	CodeInputUnknown            = "INPUT_UNKNOWN"
	CodeInputType               = "INPUT_TYPE"
	CodeInputEnum               = "INPUT_ENUM"
	CodeInputConstraint         = "INPUT_CONSTRAINT"
	CodeTemplatePlaceholder     = "TEMPLATE_PLACEHOLDER_UNDECLARED"
	CodeTemplateSyntax          = "TEMPLATE_SYNTAX_INVALID"
	CodeSelectorUnsupported     = "SELECTOR_UNSUPPORTED"
)

// Error is the stable public SDK error projection. Message is safe for logs;
// it must never contain credentials, Authorization headers, or prompt bodies.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	Cause     error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string, retryable bool, cause error) *Error {
	return &Error{Code: code, Message: message, Retryable: retryable, Cause: cause}
}

func ErrorCode(err error) string {
	if typed, ok := err.(*Error); ok {
		return typed.Code
	}
	return "INTERNAL"
}
