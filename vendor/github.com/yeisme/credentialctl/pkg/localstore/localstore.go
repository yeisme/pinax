// Package localstore is the public entry point for consumers that need to
// resolve shared credentials through the credentialctl local store. It hides
// the backend selection and registry construction behind a Resolver, so
// consumers (eikona/pinax/aigora/...) depend only on this package and
// pkg/credentials.
package localstore

import (
	"github.com/yeisme/credentialctl/internal/store"
	"github.com/yeisme/credentialctl/pkg/credentials"
)

// NewResolver builds the default local Resolver using the same backend
// selection as the credentialctl CLI: system Keychain when available, 0700/0600
// file fallback otherwise. Consumers should call this once and keep the
// returned Resolver for the process lifetime.
//
// Returns an error if no backend is available (e.g. neither Keychain nor a
// usable config directory). A consumer that receives an error should fail
// closed to its next approved source rather than guessing.
func NewResolver() (credentials.Resolver, error) {
	return newService()
}

// NewGrantResolver builds the EXPERIMENTAL pre-v1 scoped-grant resolver using
// the same backend, registry, and policy implementation as NewResolver. It is
// additive: existing consumers can continue to use NewResolver and Resolve.
func NewGrantResolver() (credentials.GrantResolver, error) {
	return newService()
}

// NewReadinessResolver builds the EXPERIMENTAL pre-v1 secret-free readiness
// resolver over the same backend, registry, and policy as the CLI. Owners use
// it to bind an approval to the current credential revision before requesting
// one scoped secret resolution.
func NewReadinessResolver() (credentials.ReadinessResolver, error) {
	return newService()
}

func newService() (*store.Service, error) {
	base, err := store.DefaultBase()
	if err != nil {
		return nil, err
	}
	backend, err := store.SelectBackend(store.KeychainBackend{}, store.NewFileBackend(base))
	if err != nil {
		return nil, err
	}
	return store.NewService(backend, store.NewRegistryStore(base), credentials.Preset(credentials.DefaultPreset)), nil
}
