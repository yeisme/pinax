package operation

import "errors"

type ErrorCode string

const (
	CodeIdentityRequired    ErrorCode = "operation_identity_required"
	CodeIdempotencyConflict ErrorCode = "idempotency_conflict"
	CodeStoreUnavailable    ErrorCode = "operation_store_unavailable"
	CodeNotFound            ErrorCode = "operation_not_found"
	CodeIllegalTransition   ErrorCode = "operation_transition_invalid"
	CodePayloadInvalid      ErrorCode = "operation_payload_invalid"
	CodePayloadTooLarge     ErrorCode = "operation_payload_too_large"
	CodeReconcileRequired   ErrorCode = "reconcile_required"
	CodeEvidenceConflict    ErrorCode = "reconcile_evidence_conflict"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func IsCode(err error, code ErrorCode) bool {
	var operationErr *Error
	return errors.As(err, &operationErr) && operationErr.Code == code
}

func operationError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
