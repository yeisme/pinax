package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCacheMiddleware_GETReturnsCacheControl(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s := &Server{}
	wrapped := s.cacheMiddleware(handler)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	wrapped.ServeHTTP(res, req)

	cc := res.Header().Get("Cache-Control")
	if cc == "" {
		t.Fatal("expected Cache-Control header on GET")
	}
	if !strings.Contains(cc, "max-age=") {
		t.Fatalf("expected max-age in Cache-Control, got: %s", cc)
	}
}

func TestCacheMiddleware_GETReturnsETag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s := &Server{}
	wrapped := s.cacheMiddleware(handler)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	wrapped.ServeHTTP(res, req)

	etag := res.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header on JSON GET response")
	}
	if !strings.HasPrefix(etag, `"`) {
		t.Fatalf("ETag should be quoted, got: %s", etag)
	}
}

func TestCacheMiddleware_ConditionalRequestReturns304(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s := &Server{}
	wrapped := s.cacheMiddleware(handler)

	// First request to get ETag
	res1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	wrapped.ServeHTTP(res1, req1)
	etag := res1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag from first request")
	}

	// Second request with If-None-Match
	res2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req2.Header.Set("If-None-Match", etag)
	wrapped.ServeHTTP(res2, req2)

	if res2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", res2.Code)
	}
}

func TestCacheMiddleware_POSTNotCached(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s := &Server{}
	wrapped := s.cacheMiddleware(handler)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/capabilities", nil)
	wrapped.ServeHTTP(res, req)

	cc := res.Header().Get("Cache-Control")
	if cc != "" {
		t.Fatalf("POST should not have Cache-Control, got: %s", cc)
	}
	etag := res.Header().Get("ETag")
	if etag != "" {
		t.Fatalf("POST should not have ETag, got: %s", etag)
	}
}

func TestCacheMiddleware_NonJSONNotCached(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})

	s := &Server{}
	wrapped := s.cacheMiddleware(handler)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	wrapped.ServeHTTP(res, req)

	etag := res.Header().Get("ETag")
	if etag != "" {
		t.Fatalf("non-JSON should not have ETag, got: %s", etag)
	}
}

func TestLookupCachePolicy(t *testing.T) {
	tests := []struct {
		path    string
		wantAge int
	}{
		{"/v1/capabilities", 300},
		{"/v1/folders", 60},
		{"/v1/notes/note-001", 30},
		{"/v1/inbox", 10},
		{"/v1/unknown", 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			p := lookupCachePolicy(tt.path)
			if tt.wantAge == 0 {
				if p != nil {
					t.Fatalf("expected nil policy for %s", tt.path)
				}
				return
			}
			if p == nil {
				t.Fatalf("expected policy for %s", tt.path)
			}
			if p.MaxAge != tt.wantAge {
				t.Fatalf("expected max-age %d, got %d", tt.wantAge, p.MaxAge)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{60, "60"},
		{300, "300"},
	}
	for _, tt := range tests {
		got := itoa(tt.n)
		if got != tt.want {
			t.Fatalf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
