package inferrum

import "context"

// FakeProvider produces deterministic SHA256-hash pseudo-embeddings. It is the
// zero-dependency local provider used for validation, tests, and offline flows.
type FakeProvider struct{ ModelName string }

func (p FakeProvider) Name() string { return "fake" }

func (p FakeProvider) Model() string {
	return defaultString(p.ModelName, FakeProviderModel)
}

func (p FakeProvider) Info() ProviderInfo {
	return ProviderInfo{Name: "fake", DefaultModel: FakeProviderModel, Configured: true, LocalOnly: true}
}

func (p FakeProvider) Embed(_ context.Context, text string) ([]float64, error) {
	return hashEmbedding(text, 32), nil
}

func (p FakeProvider) EmbedBatch(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, text := range texts {
		out = append(out, hashEmbedding(text, 32))
	}
	return out, nil
}
