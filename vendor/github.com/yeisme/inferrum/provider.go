// Package inferrum is the shared vector + RAG platform for the Yeisme CLI family
// (eikona, pinax, auctra). It lifts the embedding-provider registry, the vector
// store, the domain-multiplexed sidecar client, and the retrieval pipeline out
// of per-CLI internals into a single workspace-level Go module.
//
// Consumers import github.com/yeisme/inferrum (no subprocess, single install, zero
// config) and register only their domain adapter (~50-80 lines). The module is
// otherwise domain agnostic and never interprets record metadata.
//
// Strategic positioning and architecture:
// docs/architecture/inferrum-vector-rag-platform.md
package inferrum

import (
	"context"
	"crypto/sha256"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Provider is the embedding-provider interface. A provider turns text into a
// fixed-dimension vector and reports its identity and configuration state. All
// four built-in providers (gemini, openai, ollama, fake) are lifted from pinax
// internal/semantic and carry zero domain coupling.
type Provider interface {
	Name() string
	Model() string
	Embed(ctx context.Context, text string) ([]float64, error)
	Info() ProviderInfo
}

// EmbeddingProvider is the architecture-contract surface that the retrieval
// pipeline consumes. Every registered Provider satisfies it.
type EmbeddingProvider = Provider

// BatchProvider is optionally implemented by providers that can embed multiple
// texts in a single round trip (openai, ollama, fake).
type BatchProvider interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

// BatchEmbedder aliases BatchProvider for the architecture-contract surface.
type BatchEmbedder = BatchProvider

// ProviderInfo describes a provider for listing and doctor output.
type ProviderInfo struct {
	Name               string `json:"name"`
	DefaultModel       string `json:"default_model"`
	Configured         bool   `json:"configured"`
	CredentialSource   string `json:"credential_source,omitempty"`
	LocalOnly          bool   `json:"local_only"`
	RequiresCredential bool   `json:"requires_credential"`
}

const (
	DefaultProvider      = "gemini"
	DefaultModel         = "text-embedding-004"
	OpenAIDefaultModel   = "text-embedding-3-small"
	OllamaDefaultModel   = "nomic-embed-text"
	FakeProviderModel    = "fake-hash-v1"
	defaultOllamaBaseURL = "http://127.0.0.1:11434"
)

// CommandError is the Inferrum-local structured error type. It replaces Pinax's
// internal/domain.CommandError so the module stays free of any internal/
// dependency while preserving the stable {code, message, hint} shape that
// callers and CLIs parse.
type CommandError struct {
	Code    string
	Message string
	Hint    string
}

func (e *CommandError) Error() string {
	s := strings.TrimSpace(e.Message)
	if s == "" {
		s = e.Code
	} else if e.Code != "" {
		s = e.Code + ": " + s
	}
	if strings.TrimSpace(e.Hint) != "" {
		s += "; " + e.Hint
	}
	return s
}

type providerRegistration struct {
	info    func() ProviderInfo
	factory func(model string) (Provider, error)
}

var providerRegistry = map[string]providerRegistration{
	"gemini": {
		info: func() ProviderInfo {
			return GeminiProvider{APIKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))}.Info()
		},
		factory: func(model string) (Provider, error) {
			return GeminiProvider{APIKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), ModelName: model}, nil
		},
	},
	"openai": {
		info: func() ProviderInfo {
			return OpenAIProvider{APIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")), BaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))}.Info()
		},
		factory: func(model string) (Provider, error) {
			return OpenAIProvider{APIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")), ModelName: model, BaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))}, nil
		},
	},
	"ollama": {
		info: func() ProviderInfo {
			return OllamaProvider{BaseURL: strings.TrimSpace(os.Getenv("OLLAMA_HOST"))}.Info()
		},
		factory: func(model string) (Provider, error) {
			baseURL := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
			if baseURL == "" {
				baseURL = defaultOllamaBaseURL
			}
			return OllamaProvider{ModelName: model, BaseURL: baseURL}, nil
		},
	},
	"fake": {
		info:    func() ProviderInfo { return FakeProvider{}.Info() },
		factory: func(model string) (Provider, error) { return FakeProvider{ModelName: model}, nil },
	},
}

// ListProviders returns info for every registered provider in stable display
// order (gemini, openai, ollama, fake).
func ListProviders() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(providerRegistry))
	for _, registration := range providerRegistry {
		infos = append(infos, registration.info())
	}
	sort.SliceStable(infos, func(i, j int) bool { return providerOrder(infos[i].Name) < providerOrder(infos[j].Name) })
	return infos
}

// ProviderInfoFor returns the info for the named provider (defaulting to the
// default provider when name is empty).
func ProviderInfoFor(name string) (ProviderInfo, error) {
	key := normalizeProvider(name)
	registration, ok := providerRegistry[key]
	if !ok {
		return ProviderInfo{}, invalidProviderError()
	}
	return registration.info(), nil
}

// NewProvider constructs a provider by name (defaulting when empty) and model.
func NewProvider(name, model string) (Provider, error) {
	key := normalizeProvider(name)
	registration, ok := providerRegistry[key]
	if !ok {
		return nil, invalidProviderError()
	}
	return registration.factory(model)
}

// DoctorProvider runs a provider health check, returning a structured result
// map suitable for JSON doctor output.
func DoctorProvider(ctx context.Context, name, model string) (map[string]any, error) {
	provider, err := NewProvider(name, model)
	if err != nil {
		return nil, err
	}
	info, err := ProviderInfoFor(provider.Name())
	if err != nil {
		return nil, err
	}
	result := map[string]any{"provider": provider.Name(), "model": provider.Model(), "configured": info.Configured, "credential_source": info.CredentialSource, "local_only": info.LocalOnly, "available": true}
	if !info.Configured {
		return result, providerNotConfigured(provider.Name(), info.CredentialSource)
	}
	if doctor, ok := provider.(interface{ Doctor(context.Context) error }); ok {
		if err := doctor.Doctor(ctx); err != nil {
			result["available"] = false
			return result, err
		}
	}
	return result, nil
}

func normalizeProvider(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return DefaultProvider
	}
	return key
}

func invalidProviderError() *CommandError {
	return &CommandError{Code: "provider_invalid", Message: "Embedding provider is not supported", Hint: "Use --provider gemini, openai, ollama, or fake"}
}

func providerNotConfigured(name, source string) *CommandError {
	hint := "Configure provider credentials or use --provider fake for local validation"
	if strings.TrimSpace(source) != "" {
		hint = "Configure " + source + " or use --provider fake for local validation"
	}
	return &CommandError{Code: "provider_not_configured", Message: name + " embedding provider is not configured", Hint: hint}
}

func providerRequestFailed(name string, status int) *CommandError {
	return &CommandError{Code: "provider_request_failed", Message: name + " embedding request failed", Hint: "Provider returned HTTP status " + strconv.Itoa(status) + "; inspect provider configuration and retry"}
}

func providerEmptyEmbedding(name string) *CommandError {
	return &CommandError{Code: "provider_response_invalid", Message: name + " embedding response did not include vectors", Hint: "Inspect provider model support and retry"}
}

func providerOrder(name string) int {
	switch name {
	case "gemini":
		return 0
	case "openai":
		return 1
	case "ollama":
		return 2
	case "fake":
		return 3
	default:
		return 99
	}
}

// defaultString returns value when non-empty, otherwise fallback.
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// hashEmbedding produces a deterministic pseudo-embedding by hashing the tokens
// of text into a fixed-dimension vector. It is the basis of the fake provider
// and is never used for real retrieval.
func hashEmbedding(text string, dim int) []float64 {
	vec := make([]float64, dim)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if token == "" {
			continue
		}
		sum := sha256.Sum256([]byte(token))
		idx := int(sum[0]) % dim
		vec[idx] += 1 + float64(sum[1])/255
	}
	return vec
}
