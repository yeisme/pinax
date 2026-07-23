// Package projectsecrets is the experimental public Go API for versioned
// repository-scoped project secrets. A domain owner (for example Pinax)
// constructs a Resolver bound to a repository envelope and an unlock source,
// then calls Resolve with a Grant to obtain a short-lived Snapshot of decrypted
// entry bytes.
//
// The API surface is additive and experimental (schema yeisme.project_secrets.
// v0.1). It never returns or accepts domain semantics: entries are opaque bytes
// and the caller parses its own payload (Pinax parses s3_credentials.v1).
//
// All authentication failures surface as ErrUnlockFailed so a brute-force
// attacker cannot distinguish wrong-passphrase from tamper or cross-repo copy.
package projectsecrets

import "errors"

// Stable error codes (see spec Output And Error Contract). Each typed error
// carries one of these codes; callers may switch on Code() but must not branch
// on the message.
const (
	CodeAssetMissing        = "project_secret_asset_missing"
	CodeAssetInvalid        = "project_secret_asset_invalid"
	CodeUnlockRequired      = "project_secret_unlock_required"
	CodeUnlockFailed        = "project_secret_unlock_failed"
	CodeProviderUnsupported = "project_secret_provider_unsupported"
	CodeGrantDenied         = "project_secret_grant_denied"
	CodeEntryMissing        = "project_secret_entry_missing"
	CodePathUnsafe          = "project_secret_path_unsafe"
	CodeKeychainUnavailable = "project_secret_keychain_unavailable"
)

// Error is the unified error type. Code is the stable machine-readable token;
// Err is the sentinel for errors.Is. The message never contains a passphrase,
// ciphertext value, wrapped key, absolute private path or child env value.
type Error struct {
	Code string
	Err  error
	msg  string
}

func (e *Error) Error() string {
	if e.msg == "" {
		return e.Code
	}
	return e.msg
}

// Unwrap returns the sentinel so errors.Is(err, ErrUnlockFailed) works.
func (e *Error) Unwrap() error { return e.Err }

func newError(code string, sentinel error, msg string) *Error {
	return &Error{Code: code, Err: sentinel, msg: msg}
}

// Sentinel errors.
var (
	ErrAssetMissing        = errors.New("project secret asset missing")
	ErrAssetInvalid        = errors.New("project secret asset invalid")
	ErrUnlockRequired      = errors.New("project secret unlock required")
	ErrUnlockFailed        = errors.New("project secret unlock failed")
	ErrProviderUnsupported = errors.New("project secret provider unsupported")
	ErrGrantDenied         = errors.New("project secret grant denied")
	ErrEntryMissing        = errors.New("project secret entry missing")
	ErrPathUnsafe          = errors.New("project secret path unsafe")
	ErrKeychainUnavailable = errors.New("project secret keychain unavailable")
)

// AssetMissingError wraps a missing envelope.
func AssetMissingError(msg string) *Error { return newError(CodeAssetMissing, ErrAssetMissing, msg) }

// AssetInvalidError wraps a structurally invalid envelope.
func AssetInvalidError(msg string) *Error { return newError(CodeAssetInvalid, ErrAssetInvalid, msg) }

// UnlockRequiredError means no interactive unlock is available.
func UnlockRequiredError(msg string) *Error {
	return newError(CodeUnlockRequired, ErrUnlockRequired, msg)
}

// UnlockFailedError is the unified fail-closed authentication error.
func UnlockFailedError(msg string) *Error { return newError(CodeUnlockFailed, ErrUnlockFailed, msg) }

// ProviderUnsupportedError means the envelope provider is not in the supported range.
func ProviderUnsupportedError(msg string) *Error {
	return newError(CodeProviderUnsupported, ErrProviderUnsupported, msg)
}

// GrantDeniedError means the grant's consumer/capability/entry/digest was rejected.
func GrantDeniedError(msg string) *Error { return newError(CodeGrantDenied, ErrGrantDenied, msg) }

// EntryMissingError means the grant requested an entry not present.
func EntryMissingError(msg string) *Error { return newError(CodeEntryMissing, ErrEntryMissing, msg) }

// PathUnsafeError wraps a path-containment violation.
func PathUnsafeError(msg string) *Error { return newError(CodePathUnsafe, ErrPathUnsafe, msg) }

// KeychainUnavailableError wraps a Keychain backend that is locked/missing/denied.
func KeychainUnavailableError(msg string) *Error {
	return newError(CodeKeychainUnavailable, ErrKeychainUnavailable, msg)
}
