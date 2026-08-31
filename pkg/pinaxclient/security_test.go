package pinaxclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBaseURLSecurityPolicy(t *testing.T) {
	t.Parallel()

	accepted := []string{
		"http://127.0.0.1:8080",
		"http://127.99.1.2",
		"http://localhost:8080/",
		"http://localhost.:8080/pinax",
		"http://[::1]:8080",
		"https://example.com",
		"https://example.com/pinax",
	}
	for _, raw := range accepted {
		t.Run("accept "+raw, func(t *testing.T) {
			if _, err := New(Config{BaseURL: raw}); err != nil {
				t.Fatalf("New(%q) error = %v", raw, err)
			}
		})
	}

	rejected := []string{
		"",
		"/relative",
		"file:///tmp/pinax.sock",
		"http://example.com",
		"http://192.168.1.10:8080",
		"http://user:pass@127.0.0.1:8080",
		"https://example.com?token=secret",
		"https://example.com/#fragment",
		"https://example.com/a/../b",
		"https://example.com/a/%2e%2e/b",
	}
	for _, raw := range rejected {
		t.Run("reject "+raw, func(t *testing.T) {
			_, err := New(Config{BaseURL: raw, Token: "never-send"})
			var clientErr *Error
			if !errors.As(err, &clientErr) || clientErr.Code != CodeInvalidConfig {
				t.Fatalf("New(%q) error = %#v", raw, err)
			}
		})
	}
}

func TestTimeoutConfigurationIsBounded(t *testing.T) {
	t.Parallel()

	for _, timeout := range []time.Duration{-time.Second, maxClientTimeout + time.Second} {
		_, err := New(Config{BaseURL: "https://example.com", Timeout: timeout})
		var clientErr *Error
		if !errors.As(err, &clientErr) || clientErr.Code != CodeInvalidConfig {
			t.Fatalf("timeout %s error = %#v", timeout, err)
		}
	}
	_, err := New(Config{BaseURL: "https://example.com", HTTPClient: &http.Client{Timeout: maxClientTimeout + time.Second}})
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeInvalidConfig {
		t.Fatalf("custom client timeout error = %#v", err)
	}
}

func TestRedirectIsRejectedBeforeCredentialCanReachTarget(t *testing.T) {
	t.Parallel()

	const secret = "redirect-secret"
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("redirect target received Authorization header")
		}
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("source authorization = %q", r.Header.Get("Authorization"))
		}
		http.Redirect(w, r, target.URL+"/v1/manifest", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := New(Config{BaseURL: source.URL, Token: secret})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Manifest(context.Background())
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeRedirectRejected || targetCalls.Load() != 0 {
		t.Fatalf("error=%#v target_calls=%d", err, targetCalls.Load())
	}
}

func TestResponseLimitAndTimeoutAreBounded(t *testing.T) {
	t.Parallel()

	t.Run("response limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", 512)))
		}))
		defer server.Close()
		client, err := New(Config{BaseURL: server.URL, MaxResponseBytes: 128})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Manifest(context.Background())
		var clientErr *Error
		if !errors.As(err, &clientErr) || clientErr.Code != CodeResponseTooLarge {
			t.Fatalf("error = %#v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writeTestProjection(t, w, "api.manifest", map[string]any{})
		}))
		defer server.Close()
		client, err := New(Config{BaseURL: server.URL, Timeout: 10 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Manifest(context.Background())
		var clientErr *Error
		if !errors.As(err, &clientErr) || clientErr.Code != CodeRequestFailed || !clientErr.Retryable {
			t.Fatalf("error = %#v", err)
		}
	})
}

func TestMalformed2xxAndRequestLimitsFailClosed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"token":"do-not-copy"}}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, MaxRequestBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Manifest(context.Background())
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeUpstreamInvalidResponse || strings.Contains(clientErr.Error(), "do-not-copy") {
		t.Fatalf("malformed response error = %#v", err)
	}
	_, err = client.CallRPC(context.Background(), RPCRequest{Method: "Pinax.Test", Params: map[string]any{"large": strings.Repeat("x", 128)}})
	if !errors.As(err, &clientErr) || clientErr.Code != CodeRequestInvalid {
		t.Fatalf("oversized request error = %#v", err)
	}
}
