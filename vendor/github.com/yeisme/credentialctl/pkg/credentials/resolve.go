package credentials

import "errors"

// ErrNotAllowed is returned when a consumer/capability pair is not in the
// allowlist. The secret is never returned in this case.
var ErrNotAllowed = errors.New("consumer/capability not allowed for shared credential")

// ErrNotFound is returned when the referenced credential is not present in the
// local store.
var ErrNotFound = errors.New("shared credential not found")

// ErrBackendUnavailable is returned when no backend (Keychain or file) can
// serve the credential.
var ErrBackendUnavailable = errors.New("credential backend unavailable")

// ErrRegistryCorrupt is returned when the redacted local registry cannot be
// decoded. It is distinct from a missing credential or unavailable backend.
var ErrRegistryCorrupt = errors.New("credential registry corrupt")

// Resolution is the result of resolving a shared credential for a consumer.
// Secret is returned only to the calling owning process and must never be
// logged, persisted by the caller, or printed.
type Resolution struct {
	Ref      Ref
	Backend  string
	Revision string
	Digest   string // redacted digest
	Secret   []byte // sensitive — caller must not log or persist
}

// Resolver resolves a shared credential ref for a consumer+capability. The
// implementation must enforce the consumer/capability allowlist and must never
// return a secret for a disallowed (consumer, capability) pair.
type Resolver interface {
	// Resolve returns the credential for ref if (consumer, capability) is
	// allowed and the credential exists locally. The returned Secret must not be
	// logged or persisted by the caller.
	// Deprecated: new local owners should use an implementation's
	// ResolveWithGrant API so the policy claim is explicit. This method remains
	// available throughout the pre-v1 compatibility window.
	Resolve(consumer, capability string, ref Ref) (Resolution, error)
}

// GrantResolver is the EXPERIMENTAL pre-v1 resolver contract for local owner
// processes that can present one explicit, scoped credential-use grant.
// Implementations must reject broker claims and must never log or persist the
// returned Secret. Callers must clear Secret as soon as the owner operation no
// longer needs it.
type GrantResolver interface {
	ResolveWithGrant(grant CredentialUseGrant) (Resolution, error)
}
