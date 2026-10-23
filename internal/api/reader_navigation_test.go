package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderNavigationHTTP(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, sessions, metadata, full := newReaderTestServer(t, root)
	for _, tc := range []struct {
		name, path, bearer, method, schema string
		status                             int
	}{
		{"tags", "/v1/reader/tags?limit=1", metadata, "GET", catalogschema.ReaderTagsV1Alpha1, 200},
		{"graph", "/v1/reader/graph?mode=source&limit=2", full, "GET", catalogschema.ReaderGraphV1Alpha1, 200},
		{"daily", "/v1/reader/journals/daily?date=2026-10-05&timezone=Asia%2FShanghai", metadata, "GET", catalogschema.ReaderDailyV1Alpha1, 200},
		{"graph scope", "/v1/reader/graph?mode=source", metadata, "GET", "", 403},
		{"anonymous", "/v1/reader/tags", "", "GET", "", 401},
		{"invalid credential", "/v1/reader/tags", "not-valid", "GET", "", 401},
		{"method", "/v1/reader/tags", metadata, "POST", "", 405},
		{"unknown field", "/v1/reader/tags?body=true", metadata, "GET", "", 400},
		{"duplicate field", "/v1/reader/tags?limit=1&limit=2", metadata, "GET", "", 400},
		{"zero", "/v1/reader/tags?limit=0", metadata, "GET", "", 400},
		{"oversized page", "/v1/reader/tags?limit=201", metadata, "GET", "", 400},
		{"bad cursor", "/v1/reader/tags?cursor=invalid", metadata, "GET", "", 400},
		{"one graph node", "/v1/reader/graph?mode=source&limit=1", full, "GET", "", 400},
		{"wide graph", "/v1/reader/graph?mode=source&limit=201", full, "GET", "", 400},
		{"source center", "/v1/reader/graph?mode=source&object_id=note_alpha", full, "GET", "", 400},
		{"missing center", "/v1/reader/graph?mode=local", full, "GET", "", 400},
		{"wide hops", "/v1/reader/graph?mode=local&hops=3", full, "GET", "", 400},
		{"bad date", "/v1/reader/journals/daily?date=2026-02-30&timezone=UTC", metadata, "GET", "", 400},
		{"local zone", "/v1/reader/journals/daily?date=2026-10-05&timezone=Local", metadata, "GET", "", 400},
		{"missing timezone", "/v1/reader/journals/daily?date=2026-10-05", metadata, "GET", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status {
				t.Fatalf("status=%d want=%d", response.StatusCode, tc.status)
			}
			if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("missing no-store")
			}
			if strings.Contains(string(body), root) || strings.Contains(string(body), "zq-secret-marker") || strings.Contains(string(body), full) || strings.Contains(string(body), metadata) {
				t.Fatal("navigation leaked body, path or credentials")
			}
			if tc.schema != "" {
				var envelope map[string]any
				if err := json.Unmarshal(body, &envelope); err != nil {
					t.Fatal(err)
				}
				if err := catalogschema.ValidatePayload(catalogschema.Default(), tc.schema, envelope["data"]); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	sessions.Revoke("sess-full")
	if status, _ := readerDo(t, server, full, "/v1/reader/graph?mode=source"); status != 401 {
		t.Fatal("revoked graph session survived")
	}
	if _, err := os.Stat(filepath.Join(root, "daily")); !os.IsNotExist(err) {
		t.Fatal("daily lookup created a directory")
	}
}
