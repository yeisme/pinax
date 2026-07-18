package cli

import (
	"sync"

	"github.com/yeisme/credentialctl/pkg/localstore"
	"github.com/yeisme/pinax/internal/semantic"
)

// enableSharedOnce ensures the credentialctl shared resolver is constructed at
// most once per process. A construction failure (no Keychain and no usable
// config dir) silently disables the shared fallback, so the OpenAI embedding
// provider fails closed to its existing env source.
var enableSharedOnce sync.Once

// enableSharedCredentials constructs the credentialctl shared resolver and
// attaches it to the semantic provider so OpenAI embedding can fall back to the
// shared credential when OPENAI_API_KEY is unset. Safe to call repeatedly.
func enableSharedCredentials() {
	enableSharedOnce.Do(func() {
		if r, err := localstore.NewResolver(); err == nil {
			semantic.EnableSharedResolver(r)
		}
	})
}
