package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type OllamaProvider struct {
	ModelName  string
	BaseURL    string
	HTTPClient *http.Client
}

func (p OllamaProvider) Name() string { return "ollama" }

func (p OllamaProvider) Model() string {
	if strings.TrimSpace(p.ModelName) != "" {
		return p.ModelName
	}
	return OllamaDefaultModel
}

func (p OllamaProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	vectors, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, providerEmptyEmbedding(p.Name())
	}
	return vectors[0], nil
}

func (p OllamaProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	body := map[string]any{"model": p.Model(), "input": texts}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(defaultString(p.BaseURL, defaultOllamaBaseURL), "/")+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, ollamaUnavailable()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providerRequestFailed(p.Name(), resp.StatusCode)
	}
	var decoded struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Embeddings) != len(texts) {
		return nil, providerEmptyEmbedding(p.Name())
	}
	return decoded.Embeddings, nil
}

func (p OllamaProvider) Doctor(ctx context.Context) error {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(defaultString(p.BaseURL, defaultOllamaBaseURL), "/")+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ollamaUnavailable()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerRequestFailed(p.Name(), resp.StatusCode)
	}
	return nil
}

func ollamaUnavailable() *domain.CommandError {
	return &domain.CommandError{Code: "provider_unavailable", Message: "ollama embedding provider is not reachable", Hint: "Start Ollama on http://127.0.0.1:11434 or set OLLAMA_HOST"}
}
