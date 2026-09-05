package promptrepo

import "fmt"

const (
	CodeInvalidRequest             = "INVALID_REQUEST"
	CodeNotFound                   = "NOT_FOUND"
	CodeAlreadyExists              = "ALREADY_EXISTS"
	CodeUnsupportedSourceScheme    = "UNSUPPORTED_SOURCE_SCHEME"
	CodeAuthRequired               = "AUTH_REQUIRED"
	CodeAuthorizationFailed        = "AUTHORIZATION_FAILED"
	CodeSourceFetchFailed          = "SOURCE_FETCH_FAILED"
	CodeDigestMismatch             = "DIGEST_MISMATCH"
	CodeStateLocked                = "STATE_LOCKED"
	CodeStateSchemaTooNew          = "STATE_SCHEMA_TOO_NEW"
	CodeIncompatible               = "INCOMPATIBLE"
	CodeRightsBlocked              = "RIGHTS_BLOCKED"
	CodeAddressMismatch            = "ADDRESS_MISMATCH"
	CodeInputRequired              = "INPUT_REQUIRED"
	CodeInputUnknown               = "INPUT_UNKNOWN"
	CodeInputType                  = "INPUT_TYPE"
	CodeInputEnum                  = "INPUT_ENUM"
	CodeInputConstraint            = "INPUT_CONSTRAINT"
	CodeTemplatePlaceholder        = "TEMPLATE_PLACEHOLDER_UNDECLARED"
	CodeTemplateSyntax             = "TEMPLATE_SYNTAX_INVALID"
	CodeSelectorUnsupported        = "SELECTOR_UNSUPPORTED"
	CodeSelectorNotFound           = "SELECTOR_NOT_FOUND"
	CodeSelectorAmbiguous          = "SELECTOR_AMBIGUOUS"
	CodeDocumentDescriptorMissing  = "DOCUMENT_DESCRIPTOR_MISSING"
	CodeDocumentDescriptorInvalid  = "DOCUMENT_DESCRIPTOR_INVALID"
	CodeDocumentFormatMismatch     = "DOCUMENT_FORMAT_MISMATCH"
	CodeDocumentDuplicateKey       = "DOCUMENT_DUPLICATE_KEY"
	CodeDocumentParseFailed        = "DOCUMENT_PARSE_FAILED"
	CodeDocumentSchemaInvalid      = "DOCUMENT_SCHEMA_INVALID"
	CodeDocumentCanonicalizeFailed = "DOCUMENT_CANONICALIZE_FAILED"
	CodeDocumentTooLarge           = "DOCUMENT_TOO_LARGE"
	CodeJSONLRecordDuplicate       = "JSONL_RECORD_DUPLICATE"
	CodeJSONLRecordTooLarge        = "JSONL_RECORD_TOO_LARGE"
	CodeJSONLRecordIDInvalid       = "JSONL_RECORD_ID_INVALID"
	CodeUITemplateAddressInvalid   = "UI_TEMPLATE_ADDRESS_INVALID"
	CodeUITemplateLimitExceeded    = "UI_TEMPLATE_LIMIT_EXCEEDED"
	CodeUITemplateHTMLForbidden    = "UI_TEMPLATE_HTML_FORBIDDEN"
	CodeUITemplateCSSForbidden     = "UI_TEMPLATE_CSS_FORBIDDEN"
	CodeUITemplateSlotInvalid      = "UI_TEMPLATE_SLOT_INVALID"
	CodeUITemplateDigestMismatch   = "UI_TEMPLATE_DIGEST_MISMATCH"
	CodeUITemplateSnapshotMismatch = "UI_TEMPLATE_SNAPSHOT_MISMATCH"
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
