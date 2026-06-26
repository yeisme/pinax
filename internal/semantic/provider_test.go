package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestProviderRegistryDefaultsAndInvalidProvider(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	provider, err := NewProvider("", "")
	if err != nil {
		t.Fatalf("NewProvider default failed: %v", err)
	}
	if provider.Name() != "gemini" || provider.Model() != DefaultModel {
		t.Fatalf("default provider = %s/%s", provider.Name(), provider.Model())
	}

	infos := ListProviders()
	seen := map[string]ProviderInfo{}
	for _, info := range infos {
		seen[info.Name] = info
	}
	for _, name := range []string{"gemini", "openai", "ollama", "fake"} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("provider registry missing %s: %#v", name, infos)
		}
	}
	if seen["openai"].DefaultModel != "text-embedding-3-small" || seen["openai"].CredentialSource != "env:OPENAI_API_KEY" || seen["openai"].Configured {
		t.Fatalf("openai info = %#v", seen["openai"])
	}
	if !seen["ollama"].LocalOnly || !seen["fake"].Configured {
		t.Fatalf("ollama/fake info = %#v %#v", seen["ollama"], seen["fake"])
	}

	_, err = NewProvider("gemni", "")
	var cmdErr *domain.CommandError
	if err == nil || !strings.Contains(err.Error(), "provider_invalid") || !strings.Contains(err.Error(), "Embedding provider") {
		t.Fatalf("unknown provider error = %v", err)
	}
	if !errors.As(err, &cmdErr) || cmdErr.Code != "provider_invalid" || !strings.Contains(cmdErr.Hint, "openai") || !strings.Contains(cmdErr.Hint, "ollama") {
		t.Fatalf("unknown provider command error = %#v err=%v", cmdErr, err)
	}
}

func TestOpenAIProviderEmbedsBatchAndRedactsErrors(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "text-embedding-3-small" || strings.Join(req.Input, ",") != "alpha,beta" {
			t.Fatalf("request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": []float64{1, 0}}, {"embedding": []float64{0, 1}}}})
	}))
	defer server.Close()

	provider := OpenAIProvider{APIKey: "sk-test-secret", ModelName: "text-embedding-3-small", BaseURL: server.URL, HTTPClient: server.Client()}
	vectors, err := provider.EmbedBatch(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if authHeader != "Bearer sk-test-secret" || len(vectors) != 2 || len(vectors[0]) != 2 {
		t.Fatalf("auth/vectors = %q %#v", authHeader, vectors)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authorization: Bearer sk-test-secret provider_payload raw_prompt"}}`))
	}))
	defer bad.Close()
	_, err = (OpenAIProvider{APIKey: "sk-test-secret", BaseURL: bad.URL, HTTPClient: bad.Client()}).Embed(context.Background(), "alpha")
	if err == nil {
		t.Fatalf("expected provider error")
	}
	for _, forbidden := range []string{"sk-test-secret", "Authorization", "Bearer", "provider_payload", "raw_prompt"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("provider error leaked %q: %v", forbidden, err)
		}
	}
}

func TestOllamaProviderEmbedsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "nomic-embed-text" || len(req.Input) != 2 {
			t.Fatalf("request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{{1, 1}, {2, 2}}})
	}))
	defer server.Close()

	provider := OllamaProvider{ModelName: "nomic-embed-text", BaseURL: server.URL, HTTPClient: server.Client()}
	vectors, err := provider.EmbedBatch(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(vectors) != 2 || vectors[1][0] != 2 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestBuildChunksUsesBatchProviderFallback(t *testing.T) {
	notes := []domain.Note{{ID: "note_a", Path: "notes/a.md", Title: "A", Body: "alpha\n\nbeta"}}
	batch := &countingBatchProvider{}
	chunks, err := BuildChunks(context.Background(), notes, batch, FakeBackend)
	if err != nil {
		t.Fatalf("BuildChunks batch failed: %v", err)
	}
	if batch.batchCalls != 1 || batch.singleCalls != 0 || len(chunks) != 2 || chunks[0].Provider != "batch" {
		t.Fatalf("batch calls=%d single=%d chunks=%#v", batch.batchCalls, batch.singleCalls, chunks)
	}

	single := &countingSingleProvider{}
	chunks, err = BuildChunks(context.Background(), notes, single, FakeBackend)
	if err != nil {
		t.Fatalf("BuildChunks single failed: %v", err)
	}
	if single.calls != 2 || len(chunks) != 2 || chunks[0].Provider != "single" {
		t.Fatalf("single calls=%d chunks=%#v", single.calls, chunks)
	}
}

type countingBatchProvider struct{ batchCalls, singleCalls int }

func (p *countingBatchProvider) Name() string  { return "batch" }
func (p *countingBatchProvider) Model() string { return "batch-v1" }
func (p *countingBatchProvider) Embed(context.Context, string) ([]float64, error) {
	p.singleCalls++
	return []float64{1}, nil
}
func (p *countingBatchProvider) EmbedBatch(_ context.Context, texts []string) ([][]float64, error) {
	p.batchCalls++
	out := make([][]float64, 0, len(texts))
	for range texts {
		out = append(out, []float64{1})
	}
	return out, nil
}

type countingSingleProvider struct{ calls int }

func (p *countingSingleProvider) Name() string  { return "single" }
func (p *countingSingleProvider) Model() string { return "single-v1" }
func (p *countingSingleProvider) Embed(context.Context, string) ([]float64, error) {
	p.calls++
	return []float64{1}, nil
}
