package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type GeminiProvider struct {
	APIKey     string
	ModelName  string
	HTTPClient *http.Client
}

func (p GeminiProvider) Name() string { return "gemini" }

func (p GeminiProvider) Model() string {
	if strings.TrimSpace(p.ModelName) != "" {
		return p.ModelName
	}
	return DefaultModel
}

func (p GeminiProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, providerNotConfigured(p.Name(), "env:GEMINI_API_KEY")
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	body := map[string]any{"content": map[string]any{"parts": []map[string]string{{"text": text}}}}
	payload, _ := json.Marshal(body)
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + p.Model() + ":embedContent?key=" + p.APIKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
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
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Embedding.Values) == 0 {
		return nil, providerEmptyEmbedding(p.Name())
	}
	return decoded.Embedding.Values, nil
}
