// Package store implements the credential backends and registry persistence
// behind the pkg/credentials contract. Secret bytes live only inside Backend
// implementations and are never logged, printed or echoed by this package.
package store

import "github.com/yeisme/credentialctl/pkg/credentials"

// Backend persists secret bytes for one credential. Implementations must never
// log or echo the secret.
type Backend interface {
	// Name is the backend identifier recorded in the registry.
	Name() string
	// Available reports whether the backend can be used on this host.
	Available() bool
	// Get returns the secret bytes for key, or ErrNotFound.
	Get(key string) ([]byte, error)
	// Put atomically stores the secret for key.
	Put(key string, secret []byte) error
	// Delete removes the secret for key. A missing key is not an error.
	Delete(key string) error
}

// SelectBackend returns the first available backend from candidates (preferred
// first). Keychain is preferred over the file fallback per the design. Returns
// ErrBackendUnavailable if none are available.
func SelectBackend(candidates ...Backend) (Backend, error) {
	for _, b := range candidates {
		if b != nil && b.Available() {
			return b, nil
		}
	}
	return nil, credentials.ErrBackendUnavailable
}
