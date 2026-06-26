package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	APIKey     string
	ModelName  string
	BaseURL    string
	HTTPClient *http.Client
}

func (p OpenAIProvider) Name() string { return "openai" }

func (p OpenAIProvider) Model() string {
	if strings.TrimSpace(p.ModelName) != "" {
		return p.ModelName
	}
	return OpenAIDefaultModel
}

func (p OpenAIProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	vectors, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, providerEmptyEmbedding(p.Name())
	}
	return vectors[0], nil
}

func (p OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, providerNotConfigured(p.Name(), "env:OPENAI_API_KEY")
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	body := map[string]any{"model": p.Model(), "input": texts}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(defaultString(p.BaseURL, "https://api.openai.com"), "/")+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providerRequestFailed(p.Name(), resp.StatusCode)
	}
	var decoded struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) != len(texts) {
		return nil, providerEmptyEmbedding(p.Name())
	}
	vectors := make([][]float64, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if len(item.Embedding) == 0 {
			return nil, providerEmptyEmbedding(p.Name())
		}
		vectors = append(vectors, item.Embedding)
	}
	return vectors, nil
}
