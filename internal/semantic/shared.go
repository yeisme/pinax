package semantic

import (
	"os"
	"strings"

	"github.com/yeisme/pinax/internal/sharedcredentials"
)

// openAISharedRef is the default shared credential ref used for the OpenAI
// embedding provider fallback.
const openAISharedRef = "yeisme-credential://openai/personal-default"

// sharedResolver is an optional credentialctl fallback for the OpenAI embedding
// provider. It is attached via EnableSharedResolver from application startup.
// Tests may also set it directly (and reset to nil via t.Cleanup).
var sharedResolver sharedcredentials.Resolver

// EnableSharedResolver attaches the credentialctl shared credential resolver so
// the OpenAI embedding provider falls back to the shared credential when
// OPENAI_API_KEY is unset. Passing nil disables the fallback.
func EnableSharedResolver(r sharedcredentials.Resolver) {
	sharedResolver = r
}

// resolveOpenAIAPIKey returns the OpenAI API key and its source name. The native
// OPENAI_API_KEY env var wins; the shared credentialctl fallback is tried only
// when env is empty and a resolver is attached. A non-empty key whose source is
// "env:OPENAI_API_KEY" or "shared:<ref>" is returned; an empty key reports the
// env source name so doctor can guide configuration.
func resolveOpenAIAPIKey() (string, string) {
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		return v, "env:OPENAI_API_KEY"
	}
	if sharedResolver != nil {
		if ref, err := sharedcredentials.ParseRef(openAISharedRef); err == nil {
			if res, err := sharedResolver.Resolve("pinax", "embedding", ref); err == nil && len(res.Secret) > 0 {
				return string(res.Secret), "shared:" + openAISharedRef
			}
		}
	}
	return "", "env:OPENAI_API_KEY"
}
