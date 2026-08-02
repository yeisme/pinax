package semantic

import (
	"context"

	inferrum "github.com/yeisme/inferrum"
	"github.com/yeisme/pinax/internal/domain"
)

type inferrumProvider struct {
	provider inferrum.Provider
}

// NewInferrumProvider constructs the shared provider registry implementation while
// preserving Pinax's narrow Provider interface and stable command errors.
func NewInferrumProvider(name, model string) (Provider, error) {
	provider, err := inferrum.NewProvider(name, model)
	if err != nil {
		return nil, translateInferrumProviderError(err)
	}
	return inferrumProvider{provider: provider}, nil
}

// NewProviderForBackend selects the shared Inferrum provider registry for the
// LanceDB backend and keeps the deterministic Pinax provider path for the
// built-in fake file backend.
func NewProviderForBackend(backend, name, model string) (Provider, error) {
	if normalizedBackend(backend) == DefaultBackend {
		return NewInferrumProvider(name, model)
	}
	return NewProvider(name, model)
}

func (p inferrumProvider) Name() string  { return p.provider.Name() }
func (p inferrumProvider) Model() string { return p.provider.Model() }
func (p inferrumProvider) Info() inferrum.ProviderInfo {
	return p.provider.Info()
}

// Doctor keeps the provider-specific daemon check available to Pinax without
// widening the Pinax Provider interface. The shared provider owns the actual
// endpoint and response semantics.
func (p inferrumProvider) Doctor(ctx context.Context) error {
	if doctor, ok := p.provider.(interface{ Doctor(context.Context) error }); ok {
		return translateInferrumProviderError(doctor.Doctor(ctx))
	}
	return nil
}

func (p inferrumProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	vector, err := p.provider.Embed(ctx, text)
	if err != nil {
		return nil, translateInferrumProviderError(err)
	}
	return vector, nil
}

func (p inferrumProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if batch, ok := p.provider.(inferrum.BatchProvider); ok {
		vectors, err := batch.EmbedBatch(ctx, texts)
		if err != nil {
			return nil, translateInferrumProviderError(err)
		}
		return vectors, nil
	}
	vectors := make([][]float64, 0, len(texts))
	for _, text := range texts {
		vector, err := p.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func translateInferrumProviderError(err error) error {
	if err == nil {
		return nil
	}
	if cmdErr, ok := err.(*inferrum.CommandError); ok {
		return &domain.CommandError{Code: cmdErr.Code, Message: cmdErr.Message, Hint: cmdErr.Hint}
	}
	return err
}

// DoctorProviderForBackend runs the provider readiness check through the
// shared Inferrum registry for the LanceDB backend. Ollama readiness is layered:
// daemon reachability is checked first, then a non-sensitive real embed
// canary proves that the exact model returns a non-empty fixed dimension.
// Cloud providers keep the existing credential/doctor behavior and are never
// contacted merely by listing or inspecting the registry.
func DoctorProviderForBackend(ctx context.Context, backend, name, model string) (map[string]any, error) {
	if normalizedBackend(backend) != DefaultBackend {
		return DoctorProvider(ctx, name, model)
	}
	provider, err := NewInferrumProvider(name, model)
	if err != nil {
		return nil, err
	}
	info := provider.(interface{ Info() inferrum.ProviderInfo }).Info()
	result := map[string]any{
		"provider":          provider.Name(),
		"model":             provider.Model(),
		"configured":        info.Configured,
		"credential_source": info.CredentialSource,
		"local_only":        info.LocalOnly,
		"available":         true,
		"embed_ready":       false,
	}
	if !info.Configured {
		result["available"] = false
		return result, providerNotConfigured(provider.Name(), info.CredentialSource)
	}
	if err := provider.(interface{ Doctor(context.Context) error }).Doctor(ctx); err != nil {
		result["available"] = false
		return result, err
	}
	if provider.Name() != "ollama" && provider.Name() != "fake" {
		return result, nil
	}
	var vectors [][]float64
	if batch, ok := provider.(BatchProvider); ok {
		vectors, err = batch.EmbedBatch(ctx, []string{"pinax provider readiness canary", "pinax provider readiness canary 2"})
	} else {
		var vector []float64
		vector, err = provider.Embed(ctx, "pinax provider readiness canary")
		vectors = [][]float64{vector}
	}
	if err != nil {
		result["available"] = false
		return result, err
	}
	if err := validateEmbeddingDimensions(vectors); err != nil {
		result["available"] = false
		return result, err
	}
	result["embed_ready"] = true
	result["embedding_dim"] = len(vectors[0])
	return result, nil
}

var _ Provider = inferrumProvider{}
var _ BatchProvider = inferrumProvider{}
