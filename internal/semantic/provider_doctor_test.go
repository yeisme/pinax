package semantic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDoctorProviderForBackendRunsOllamaEmbedCanary(t *testing.T) {
	var tagsCalls, embedCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tagsCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[{"name":"pinax-qwen3-embedding:lowmem"}]}`))
		case "/api/embed":
			embedCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"embeddings":[[1,2,3],[4,5,6]]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	result, err := DoctorProviderForBackend(context.Background(), DefaultBackend, "ollama", "pinax-qwen3-embedding:lowmem")
	if err != nil {
		t.Fatalf("doctor should pass real embed canary: %v", err)
	}
	if result["available"] != true || result["embed_ready"] != true || result["embedding_dim"] != 3 {
		t.Fatalf("doctor result = %#v", result)
	}
	if tagsCalls != 1 || embedCalls != 1 {
		t.Fatalf("readiness calls = tags=%d embed=%d, want one each", tagsCalls, embedCalls)
	}
}

func TestDoctorProviderForBackendReportsEmbedFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		if r.URL.Path == "/api/embed" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"model missing"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	result, err := DoctorProviderForBackend(context.Background(), DefaultBackend, "ollama", "missing-model")
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "provider_request_failed" {
		t.Fatalf("error = %#v, want provider_request_failed", err)
	}
	if result["available"] != false || result["embed_ready"] != false {
		t.Fatalf("failed doctor result = %#v", result)
	}
}

func TestDoctorProviderForBackendReportsDaemonDown(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	result, err := DoctorProviderForBackend(context.Background(), DefaultBackend, "ollama", "offline-model")
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "provider_unavailable" {
		t.Fatalf("error = %#v, want provider_unavailable", err)
	}
	if result["available"] != false || result["embed_ready"] != false {
		t.Fatalf("daemon-down doctor result = %#v", result)
	}
}

func TestDoctorProviderForBackendRejectsEmptyEmbedVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		if r.URL.Path == "/api/embed" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"embeddings":[[],[]]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	result, err := DoctorProviderForBackend(context.Background(), DefaultBackend, "ollama", "empty-model")
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "embedding_dimension_invalid" {
		t.Fatalf("error = %#v, want embedding_dimension_invalid", err)
	}
	if result["available"] != false || result["embed_ready"] != false {
		t.Fatalf("empty vector doctor result = %#v", result)
	}
}

func TestDoctorProviderForBackendRejectsMixedEmbedDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		if r.URL.Path == "/api/embed" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"embeddings":[[1,2],[3]]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	result, err := DoctorProviderForBackend(context.Background(), DefaultBackend, "ollama", "mixed-model")
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "embedding_dimension_mismatch" {
		t.Fatalf("error = %#v, want embedding_dimension_mismatch", err)
	}
	if result["available"] != false || result["embed_ready"] != false {
		t.Fatalf("mixed dimension doctor result = %#v", result)
	}
}

func TestInspectProviderIdentitySeparatesBaseDerivedAndProfileDigests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.20.5"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"pinax-qwen3-embedding:lowmem","digest":"sha256:derived"},{"name":"qwen3-embedding:0.6b","digest":"sha256:base"}]}`))
		case "/api/show":
			_, _ = w.Write([]byte(`{"parameters":"num_ctx 512\nnum_thread 8","details":{"parent_model":"qwen3-embedding:0.6b"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	identity, err := InspectProviderIdentity(context.Background(), "ollama", "pinax-qwen3-embedding:lowmem")
	if err != nil {
		t.Fatalf("inspect identity: %v", err)
	}
	if identity.DaemonVersion != "0.20.5" || identity.ModelManifestDigest != "sha256:derived" || identity.BaseModelDigest != "sha256:base" {
		t.Fatalf("identity = %#v", identity)
	}
	if identity.ProfileHash == "" || identity.ProfileHash == identity.ModelManifestDigest {
		t.Fatalf("profile hash = %#v", identity)
	}
}

func TestInspectProviderIdentityCanonicalizesOllamaParameterOrder(t *testing.T) {
	parameters := "num_thread 8\nnum_ctx 512"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.20.5"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"pinax-qwen3-embedding:lowmem","digest":"sha256:derived"},{"name":"qwen3-embedding:0.6b","digest":"sha256:base"}]}`))
		case "/api/show":
			_, _ = w.Write([]byte(`{"parameters":` + strconv.Quote(parameters) + `,"details":{"parent_model":"qwen3-embedding:0.6b"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)

	first, err := InspectProviderIdentity(context.Background(), "ollama", "pinax-qwen3-embedding:lowmem")
	if err != nil {
		t.Fatalf("inspect first identity: %v", err)
	}
	parameters = "num_ctx 512\nnum_thread 8"
	second, err := InspectProviderIdentity(context.Background(), "ollama", "pinax-qwen3-embedding:lowmem")
	if err != nil {
		t.Fatalf("inspect second identity: %v", err)
	}
	if first.ProfileHash != second.ProfileHash {
		t.Fatalf("parameter order changed profile identity: first=%s second=%s", first.ProfileHash, second.ProfileHash)
	}
}
