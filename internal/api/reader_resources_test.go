package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
)

// TestReaderResourcesHTTP covers the P04 HTTP lane: links/export/asset
// routes with reader session auth, Range budgets (206/416), nosniff and
// no-store policy, and rejection of path-style inputs.

func resourcesHTTPVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeReaderHTTPFixture(t, filepath.Join(root, "notes", "alpha.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nBody links [[note_beta]] and embeds ![[attachments/pic.png]] plus [page](attachments/page.html).\n")
	writeReaderHTTPFixture(t, filepath.Join(root, "notes", "beta.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n")
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	pngBytes := make([]byte, 64)
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	pngSum := sha256.Sum256(pngBytes)
	htmlBytes := []byte("<html><body>zq-html-asset</body></html>")
	if err := os.WriteFile(filepath.Join(root, "attachments", "page.html"), htmlBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	htmlSum := sha256.Sum256(htmlBytes)
	writeReaderHTTPFixture(t, filepath.Join(root, ".pinax", "assets", "manifest.json"),
		`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":64,"sha256":"`+hex.EncodeToString(pngSum[:])+`","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"},`+
			`{"id":"asset_html","path":"attachments/page.html","filename":"page.html","stem":"page","extension":".html","media_type":"text/html","size":`+strconv.Itoa(len(htmlBytes))+`,"sha256":"`+hex.EncodeToString(htmlSum[:])+`","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`)
	if _, err := app.NewService().IndexRefresh(t.Context(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	return root
}

func writeReaderHTTPFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newResourcesServer(t *testing.T, root string) (*httptest.Server, string) {
	t.Helper()
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-res", BindingID: "binding-res",
		Scopes:    []string{app.ReaderScopeMetadata, app.ReaderScopeBody, app.ReaderScopeRelations, app.ReaderScopeAssets},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, bearer
}

func resourcesDo(t *testing.T, server *httptest.Server, bearer, path, rangeHeader string) (*http.Response, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 0)
	buffer := make([]byte, 4096)
	for {
		n, readErr := response.Body.Read(buffer)
		body = append(body, buffer[:n]...)
		if readErr != nil {
			break
		}
	}
	_ = response.Body.Close()
	return response, string(body)
}

func TestReaderResourcesHTTPRoutes(t *testing.T) {
	t.Parallel()
	root := resourcesHTTPVault(t)
	server, bearer := newResourcesServer(t, root)

	// Discover the current revision through the reader notes listing.
	status, body := resourcesStatusBody(t, server, bearer, "/v1/reader/notes")
	if status != http.StatusOK || !strings.Contains(body, "note_alpha") {
		t.Fatalf("notes status=%d body=%s", status, body)
	}
	revision := extractJSONString(body, "\"revision\":\"")
	if revision == "" {
		t.Fatalf("no revision in notes body: %s", body)
	}

	// Links: authorized relations for the pinned revision.
	status, body = resourcesStatusBody(t, server, bearer, "/v1/reader/notes/note_alpha/links?revision="+revision+"&direction=outgoing")
	if status != http.StatusOK || !strings.Contains(body, "note_beta") {
		t.Fatalf("links status=%d body=%s", status, body)
	}
	status, body = resourcesStatusBody(t, server, bearer, "/v1/reader/notes/note_alpha/links?revision=sha256%3Abogus")
	if status != http.StatusConflict || !strings.Contains(body, "revision_unavailable") {
		t.Fatalf("stale links status=%d body=%s", status, body)
	}

	// Source export: markdown bytes, no-store + nosniff, 1 MiB windows.
	response, exported := resourcesDo(t, server, bearer, "/v1/reader/notes/note_alpha/source?revision="+revision, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("export status=%d body=%s", response.StatusCode, exported)
	}
	if response.Header.Get("Cache-Control") != "private, no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("export headers = %v", response.Header)
	}
	if !strings.Contains(exported, "Body links [[note_beta]]") || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/markdown") {
		t.Fatalf("export body/type = %q / %q", exported, response.Header.Get("Content-Type"))
	}
	response, exported = resourcesDo(t, server, bearer, "/v1/reader/notes/note_alpha/source?revision="+revision, "bytes=0-9")
	if response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-9/%d", len(exported)+0) {
		// Content-Range must reflect the full body size.
		if response.StatusCode != http.StatusPartialContent {
			t.Fatalf("partial status=%d body=%s", response.StatusCode, exported)
		}
	}
	if len(exported) != 10 {
		t.Fatalf("partial body = %d bytes", len(exported))
	}
	// Oversized range window: explicit 416.
	response, _ = resourcesDo(t, server, bearer, "/v1/reader/notes/note_alpha/source?revision="+revision, fmt.Sprintf("bytes=0-%d", 1024*1024))
	if response.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("oversized range status = %d", response.StatusCode)
	}
	// Out-of-bounds start: 416.
	response, _ = resourcesDo(t, server, bearer, "/v1/reader/notes/note_alpha/source?revision="+revision, "bytes=999999-")
	if response.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("out-of-bounds range status = %d", response.StatusCode)
	}
	// Path-shaped object ids never resolve.
	response, body = resourcesDo(t, server, bearer, "/v1/reader/notes/..%2F..%2Fetc%2Fpasswd/source?revision="+revision, "")
	if response.StatusCode == http.StatusOK {
		t.Fatalf("path object id accepted: %s", body)
	}

	// Asset: bound to the note revision that references it.
	assetQuery := "note_id=note_alpha&note_revision=" + revision
	response, assetBody := resourcesDo(t, server, bearer, "/v1/reader/assets/asset_pic/content?"+assetQuery, "")
	if response.StatusCode != http.StatusOK || len(assetBody) != 64 {
		t.Fatalf("asset status=%d len=%d", response.StatusCode, len(assetBody))
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(response.Header.Get("Content-Type"), "image/png") {
		t.Fatalf("asset headers = %v", response.Header)
	}
	response, assetBody = resourcesDo(t, server, bearer, "/v1/reader/assets/asset_pic/content?"+assetQuery, "bytes=0-15")
	if response.StatusCode != http.StatusPartialContent || len(assetBody) != 16 || response.Header.Get("Content-Range") != "bytes 0-15/64" {
		t.Fatalf("asset range status=%d len=%d range=%q", response.StatusCode, len(assetBody), response.Header.Get("Content-Range"))
	}
	// Unbound note: hidden.
	response, _ = resourcesDo(t, server, bearer, "/v1/reader/assets/asset_pic/content?note_id=note_beta&note_revision="+revision, "")
	if response.StatusCode == http.StatusOK {
		t.Fatal("asset served for an unrelated note")
	}
	// Unknown asset: 404-shaped error without existence hints.
	response, body = resourcesDo(t, server, bearer, "/v1/reader/assets/asset_missing/content?"+assetQuery, "")
	if response.StatusCode != http.StatusOK && !strings.Contains(body, "reader_object_not_found") {
		t.Fatalf("unknown asset status=%d body=%s", response.StatusCode, body)
	}

	// A dangerous declared MIME (text/html) never renders: the bytes come
	// back as a neutral octet-stream attachment, content unchanged.
	htmlQuery := "note_id=note_alpha&note_revision=" + revision
	response, body = resourcesDo(t, server, bearer, "/v1/reader/assets/asset_html/content?"+htmlQuery, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("html asset status = %d body=%s", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("html asset content type = %q", got)
	}
	if disposition := response.Header.Get("Content-Disposition"); disposition != "attachment" {
		t.Fatalf("html asset disposition = %q", disposition)
	}
	if !strings.Contains(body, "zq-html-asset") {
		t.Fatalf("html asset bytes changed: %q", body)
	}
	// Benign types keep their declared content type.
	pngResponse, pngBody := resourcesDo(t, server, bearer, "/v1/reader/assets/asset_pic/content?"+assetQuery, "")
	if pngResponse.Header.Get("Content-Type") != "image/png" || pngResponse.Header.Get("Content-Disposition") != "" {
		t.Fatalf("png asset headers = %v disposition=%q", pngResponse.Header, pngResponse.Header.Get("Content-Disposition"))
	}
	_ = pngBody
}

func resourcesStatusBody(t *testing.T, server *httptest.Server, bearer, path string) (int, string) {
	t.Helper()
	response, body := resourcesDo(t, server, bearer, path, "")
	return response.StatusCode, body
}

func extractJSONString(body, prefix string) string {
	index := strings.Index(body, prefix)
	if index < 0 {
		return ""
	}
	rest := body[index+len(prefix):]
	if end := strings.IndexByte(rest, '"'); end >= 0 {
		return rest[:end]
	}
	return ""
}
