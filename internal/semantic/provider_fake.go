package semantic

import "context"

type FakeProvider struct{ ModelName string }

func (p FakeProvider) Name() string { return "fake" }

func (p FakeProvider) Model() string {
	if p.ModelName != "" {
		return p.ModelName
	}
	return FakeProviderModel
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
