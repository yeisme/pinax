package semantic

import (
	"testing"

	"github.com/yeisme/pinax/internal/sharedcredentials"
)

type fakeResolver struct {
	secret []byte
}

func (f fakeResolver) Resolve(consumer, capability string, ref sharedcredentials.Ref) (sharedcredentials.Resolution, error) {
	return sharedcredentials.Resolution{Secret: f.secret, Backend: "fake", Ref: ref}, nil
}

func TestResolveOpenAIAPIKeyPrecedence(t *testing.T) {
	t.Cleanup(func() { sharedResolver = nil })

	// env wins over shared
	t.Setenv("OPENAI_API_KEY", "sk-env-value")
	EnableSharedResolver(fakeResolver{secret: []byte("sk-shared-embedding")})
	key, source := resolveOpenAIAPIKey()
	if key != "sk-env-value" || source != "env:OPENAI_API_KEY" {
		t.Fatalf("env precedence: key=%q source=%q", key, source)
	}

	// shared fallback fires when env unset
	t.Setenv("OPENAI_API_KEY", "")
	key, source = resolveOpenAIAPIKey()
	if key != "sk-shared-embedding" || source != "shared:"+openAISharedRef {
		t.Fatalf("shared fallback: key=%q source=%q", key, source)
	}

	// nil resolver disables the fallback
	t.Setenv("OPENAI_API_KEY", "")
	EnableSharedResolver(nil)
	key, source = resolveOpenAIAPIKey()
	if key != "" || source != "env:OPENAI_API_KEY" {
		t.Fatalf("nil resolver should disable shared: key=%q source=%q", key, source)
	}
}

func TestOpenAIProviderInfoReflectsSharedSource(t *testing.T) {
	t.Cleanup(func() { sharedResolver = nil })
	t.Setenv("OPENAI_API_KEY", "")
	EnableSharedResolver(fakeResolver{secret: []byte("sk-shared-info")})

	info, err := ProviderInfoFor("openai")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Configured || info.CredentialSource != "shared:"+openAISharedRef {
		t.Fatalf("provider info should reflect shared source: %+v", info)
	}

	provider, err := NewProvider("openai", "")
	if err != nil {
		t.Fatal(err)
	}
	oa, ok := provider.(OpenAIProvider)
	if !ok || oa.APIKey != "sk-shared-info" {
		t.Fatalf("factory did not use shared key: %#v", provider)
	}
}
