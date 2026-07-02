package semantic

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	DefaultProvider      = "gemini"
	DefaultModel         = "text-embedding-004"
	OpenAIDefaultModel   = "text-embedding-3-small"
	OllamaDefaultModel   = "nomic-embed-text"
	FakeProviderModel    = "fake-hash-v1"
	defaultOllamaBaseURL = "http://127.0.0.1:11434"
)

type Provider interface {
	Name() string
	Model() string
	Embed(ctx context.Context, text string) ([]float64, error)
}

type BatchProvider interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

type ProviderInfo struct {
	Name               string `json:"name"`
	DefaultModel       string `json:"default_model"`
	Configured         bool   `json:"configured"`
	CredentialSource   string `json:"credential_source,omitempty"`
	LocalOnly          bool   `json:"local_only"`
	RequiresCredential bool   `json:"requires_credential"`
}

type providerRegistration struct {
	info    func() ProviderInfo
	factory func(model string) (Provider, error)
}

var providerRegistry = map[string]providerRegistration{
	"gemini": {
		info: func() ProviderInfo {
			return ProviderInfo{Name: "gemini", DefaultModel: DefaultModel, Configured: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "", CredentialSource: "env:GEMINI_API_KEY", RequiresCredential: true}
		},
		factory: func(model string) (Provider, error) {
			return GeminiProvider{APIKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), ModelName: model}, nil
		},
	},
	"openai": {
		info: func() ProviderInfo {
			return ProviderInfo{Name: "openai", DefaultModel: OpenAIDefaultModel, Configured: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "", CredentialSource: "env:OPENAI_API_KEY", RequiresCredential: true}
		},
		factory: func(model string) (Provider, error) {
			return OpenAIProvider{APIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")), ModelName: model, BaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))}, nil
		},
	},
	"ollama": {
		info: func() ProviderInfo {
			return ProviderInfo{Name: "ollama", DefaultModel: OllamaDefaultModel, Configured: true, CredentialSource: "local:http://127.0.0.1:11434", LocalOnly: true}
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
		info: func() ProviderInfo {
			return ProviderInfo{Name: "fake", DefaultModel: FakeProviderModel, Configured: true, LocalOnly: true}
		},
		factory: func(model string) (Provider, error) { return FakeProvider{ModelName: model}, nil },
	},
}

func ListProviders() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(providerRegistry))
	for _, registration := range providerRegistry {
		infos = append(infos, registration.info())
	}
	sort.SliceStable(infos, func(i, j int) bool { return providerOrder(infos[i].Name) < providerOrder(infos[j].Name) })
	return infos
}

func ProviderInfoFor(name string) (ProviderInfo, error) {
	key := normalizeProvider(name)
	registration, ok := providerRegistry[key]
	if !ok {
		return ProviderInfo{}, invalidProviderError()
	}
	return registration.info(), nil
}

func NewProvider(name, model string) (Provider, error) {
	key := normalizeProvider(name)
	registration, ok := providerRegistry[key]
	if !ok {
		return nil, invalidProviderError()
	}
	return registration.factory(model)
}

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

func invalidProviderError() *domain.CommandError {
	return &domain.CommandError{Code: "provider_invalid", Message: "Embedding provider is not supported", Hint: "Use --provider gemini, openai, ollama, or fake"}
}

func providerNotConfigured(name, source string) *domain.CommandError {
	hint := "Configure provider credentials or use --provider fake for local validation"
	if strings.TrimSpace(source) != "" {
		hint = "Configure " + source + " or use --provider fake for local validation"
	}
	return &domain.CommandError{Code: "provider_not_configured", Message: name + " embedding provider is not configured", Hint: hint}
}

func providerRequestFailed(name string, status int) *domain.CommandError {
	return &domain.CommandError{Code: "provider_request_failed", Message: name + " embedding request failed", Hint: "Provider returned HTTP status " + strconv.Itoa(status) + "; inspect provider configuration and retry"}
}

func providerEmptyEmbedding(name string) *domain.CommandError {
	return &domain.CommandError{Code: "provider_response_invalid", Message: name + " embedding response did not include vectors", Hint: "Inspect provider model support and retry"}
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
