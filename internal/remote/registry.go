package remote

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// StoreFactory is a function signature for building a BlobStore.
type StoreFactory func(ctx context.Context, endpoint string) (BlobStore, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]StoreFactory)
)

// Register registers a new BlobStore factory for a URI scheme.
func Register(scheme string, factory StoreFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[strings.ToLower(scheme)] = factory
}

// NewStore instantiates a BlobStore based on the endpoint URI scheme.
func NewStore(ctx context.Context, endpoint string) (BlobStore, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid remote URI: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return nil, fmt.Errorf("endpoint URI must specify a scheme: %s", endpoint)
	}

	registryMu.RLock()
	factory, exists := registry[scheme]
	registryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported remote scheme: %s", scheme)
	}

	return factory(ctx, endpoint)
}

// IsSupportedScheme returns true if the scheme has a registered factory.
func IsSupportedScheme(scheme string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, exists := registry[strings.ToLower(scheme)]
	return exists
}

func unsupportedTransportFactory(scheme string) StoreFactory {
	return func(ctx context.Context, endpoint string) (BlobStore, error) {
		return nil, fmt.Errorf("transport_unavailable: %s transport is not wired", scheme)
	}
}

func init() {
	Register("http", unsupportedTransportFactory("http"))
	Register("https", unsupportedTransportFactory("https"))
}
