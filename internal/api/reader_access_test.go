package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestReaderAccessHTTP covers the P02 reader HTTP lane: dedicated reader
// session authorization (legacy tokens never upgrade), scope enforcement,
// no-store cache policy, query/snippet redaction in logs, and binding
// isolation between two sessions sharing one vault. Fixture vault only.

func readerHTTPVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	fixtures := map[string]string{
		"notes/alpha.md": "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nAlpha contains zq-secret-marker body text.\n",
		"notes/beta.md":  "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: inbox\nupdated_at: 2026-09-11T10:00:00+00:00\n---\n\nBeta body zq-secret-marker with more text.\n",
	}
	for path, content := range fixtures {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir fixture %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	svc := app.NewService()
	if _, err := svc.IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	return root
}

func newReaderTestServer(t *testing.T, root string) (*httptest.Server, *ReaderSessionStore, string, string) {
	t.Helper()
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	metadataBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	fullBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-meta",
		BindingID: "binding-a",
		Scopes:    []string{app.ReaderScopeMetadata},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, metadataBearer); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-full",
		BindingID: "binding-b",
		Scopes:    []string{app.ReaderScopeMetadata, app.ReaderScopeBody, app.ReaderScopeRelations, app.ReaderScopeAssets},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, fullBearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, sessions, metadataBearer, fullBearer
}

func readerDo(t *testing.T, server *httptest.Server, bearer, path string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body)
}

func TestReaderAccessHTTPAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, sessions, metadataBearer, fullBearer := newReaderTestServer(t, root)

	// No bearer: 401, never an anonymous read.
	if status, _ := readerDo(t, server, "", "/v1/reader/source"); status != http.StatusUnauthorized {
		t.Fatalf("anonymous source status = %d", status)
	}
	// Legacy temp token must not upgrade into reader scopes.
	legacyServer := NewServerWithOptions(app.NewService(), root, ServerOptions{AuthMode: AuthModeTemp, ReaderSessions: sessions})
	legacy := httptest.NewServer(legacyServer.Handler())
	defer legacy.Close()
	request, _ := http.NewRequest(http.MethodGet, legacy.URL+"/v1/manifest", nil)
	response, err := legacy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var manifestEnvelope struct {
		Data struct {
			Manifest struct {
				Capabilities []struct {
					ID                string   `json:"id"`
					AvailableSurfaces []string `json:"available_surfaces"`
					Bindings          []struct {
						ID           string `json:"id"`
						Availability string `json:"availability"`
					} `json:"bindings"`
				} `json:"capabilities"`
			} `json:"manifest"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&manifestEnvelope); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	_ = response.Body.Close()
	tempSecret := legacyServer.tempSecret
	if tempSecret == "" {
		t.Fatal("temp secret missing")
	}
	if status, _ := readerDo(t, legacy, tempSecret, "/v1/reader/source"); status != http.StatusUnauthorized {
		t.Fatalf("legacy temp token reached reader surface: %d", status)
	}
	for _, capability := range manifestEnvelope.Data.Manifest.Capabilities {
		if !strings.HasPrefix(capability.ID, "reader.") {
			continue
		}
		// Since P06 the reader bindings are backed by real handlers: REST is
		// advertised exactly and truthfully in the manifest.
		if len(capability.AvailableSurfaces) != 1 || capability.AvailableSurfaces[0] != "rest" {
			t.Fatalf("reader capability %q available surfaces = %#v", capability.ID, capability.AvailableSurfaces)
		}
		for _, binding := range capability.Bindings {
			if binding.Availability != "available" {
				t.Fatalf("reader binding %q availability = %q", binding.ID, binding.Availability)
			}
		}
	}

	// Metadata-only session: source OK, search forbidden (body scope).
	if status, body := readerDo(t, server, metadataBearer, "/v1/reader/source"); status != http.StatusOK || !strings.Contains(body, "pinax.reader.source.v1alpha1") {
		t.Fatalf("metadata source status=%d body=%s", status, body)
	}
	// Unknown project filter is a 400 reader_request_invalid over HTTP —
	// never a silent empty success.
	if status, body := readerDo(t, server, fullBearer, "/v1/reader/notes?project_id=no-such-project"); status != http.StatusBadRequest || !strings.Contains(body, "reader_request_invalid") {
		t.Fatalf("unknown project status=%d body=%s", status, body)
	}
	if status, body := readerDo(t, server, metadataBearer, "/v1/reader/search?q=alpha"); status != http.StatusForbidden || !strings.Contains(body, "insufficient_scope") {
		t.Fatalf("metadata search status=%d body=%s", status, body)
	}

	// Full session: search succeeds; expired sessions fail closed.
	if status, body := readerDo(t, server, fullBearer, "/v1/reader/search?q=zq-secret-marker"); status != http.StatusOK || !strings.Contains(body, "note_alpha") {
		t.Fatalf("full search status=%d body=%s", status, body)
	}
	expiredBearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(ReaderSession{SessionID: "sess-expired", BindingID: "binding-c", Scopes: []string{app.ReaderScopeMetadata}, ExpiresAt: time.Now().UTC().Add(-time.Minute)}, expiredBearer); err != nil {
		t.Fatal(err)
	}
	if status, _ := readerDo(t, server, expiredBearer, "/v1/reader/source"); status != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d", status)
	}

	// Reader disabled when the store is absent.
	bare := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{}).Handler())
	defer bare.Close()
	if status, body := readerDo(t, bare, fullBearer, "/v1/reader/source"); status != http.StatusForbidden || !strings.Contains(body, "reader_disabled") {
		t.Fatalf("bare server reader status=%d body=%s", status, body)
	}

	// Method discipline: reader routes are GET-only.
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/reader/notes", nil)
	request.Header.Set("Authorization", "Bearer "+fullBearer)
	postResponse, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = postResponse.Body.Close()
	if postResponse.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST reader notes status = %d", postResponse.StatusCode)
	}
}

func TestReaderAccessHTTPNoStoreAndRedaction(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)

	logBuffer := &zapTestBuffer{}
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(logBuffer), zapcore.InfoLevel))
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(ReaderSession{SessionID: "sess-log", BindingID: "binding-a", Scopes: []string{app.ReaderScopeMetadata, app.ReaderScopeBody}, ExpiresAt: time.Now().UTC().Add(time.Hour)}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions, Logger: logger}).Handler())
	defer server.Close()

	status, body := readerDo(t, server, bearer, "/v1/reader/search?q=zq-secret-marker")
	if status != http.StatusOK {
		t.Fatalf("search status = %d body=%s", status, body)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/reader/notes?limit=1", nil)
	request.Header.Set("Authorization", "Bearer "+bearer)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("reader cache control = %q", response.Header.Get("Cache-Control"))
	}
	if !strings.Contains(string(rawBody), "next_cursor") {
		t.Fatalf("expected paginated cursor in body: %s", rawBody)
	}
	// Snippets and queries must never reach logs; bearer neither.
	logs := logBuffer.String()
	for _, leak := range []string{"zq-secret-marker", "Alpha contains", bearer, "sess-log"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("access log leaked %q: %s", leak, logs)
		}
	}
}

type zapTestBuffer struct {
	content strings.Builder
}

func (b *zapTestBuffer) Write(p []byte) (int, error) {
	return b.content.Write(p)
}

func (b *zapTestBuffer) String() string {
	return b.content.String()
}

func TestReaderAccessHTTPBindingIsolation(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, _, _, fullBearer := newReaderTestServer(t, root)

	// Grab a cursor as binding-b full session.
	status, body := readerDo(t, server, fullBearer, "/v1/reader/notes?limit=1")
	if status != http.StatusOK {
		t.Fatalf("notes status = %d body=%s", status, body)
	}
	var envelope struct {
		Data struct {
			Notes struct {
				NextCursor string `json:"next_cursor"`
			} `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode notes: %v", err)
	}
	cursor := envelope.Data.Notes.NextCursor
	if cursor == "" {
		t.Fatal("expected cursor")
	}

	// The same vault served by another server process (different cursor
	// secret) must reject the foreign cursor instead of silently paging.
	other, otherSessions, _, otherBearer := newReaderTestServer(t, root)
	_ = otherSessions
	status, body = readerDo(t, other, otherBearer, "/v1/reader/notes?limit=1&cursor="+cursor)
	if status != http.StatusBadRequest || !strings.Contains(body, "reader_cursor_invalid") {
		t.Fatalf("cross-server cursor status=%d body=%s", status, body)
	}
}

// TestReaderAccessServedPayloadsMatchFrozenSchemas validates the real HTTP
// payloads (not just catalog registrations) against the frozen machine
// schemas, so generated consumer types never drift from the wire format.
func TestReaderAccessServedPayloadsMatchFrozenSchemas(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, _, _, fullBearer := newReaderTestServer(t, root)
	ctx := context.Background()
	client := &http.Client{}
	do := func(path string) map[string]any {
		request, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+fullBearer)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.StatusCode, body)
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		return envelope.Data
	}

	registry := catalogschema.Default()
	validate := func(path, key, schemaID string, data map[string]any) {
		t.Helper()
		if err := catalogschema.ValidatePayload(registry, schemaID, data[key]); err != nil {
			t.Fatalf("%s payload violates %s: %v", path, schemaID, err)
		}
	}

	source := do("/v1/reader/source")
	validate("/v1/reader/source", "source", catalogschema.ReaderSourceV1Alpha1, source)

	notes := do("/v1/reader/notes?limit=2")
	validate("/v1/reader/notes", "notes", catalogschema.ReaderNoteListV1Alpha1, notes)

	search := do("/v1/reader/search?q=alpha")
	validate("/v1/reader/search", "search", catalogschema.ReaderSearchResultsV1Alpha1, search)

	// Pin the first card's object id and revision from the structured note
	// list so the document and link requests describe the same note.
	listPayload, ok := notes["notes"].(map[string]any)
	if !ok {
		t.Fatalf("note list payload shape: %#v", notes)
	}
	items, _ := listPayload["items"].([]any)
	if len(items) == 0 {
		t.Fatal("note list is empty")
	}
	firstCard, _ := items[0].(map[string]any)
	objectID, _ := firstCard["object_id"].(string)
	revision, _ := firstCard["revision"].(string)
	if objectID == "" || revision == "" {
		t.Fatalf("first card = %#v", firstCard)
	}

	document := do("/v1/reader/notes/" + objectID + "/document?revision=" + revision)
	validate("/v1/reader/document", "document", catalogschema.ReaderDocumentV1Alpha1, document)

	links := do("/v1/reader/notes/" + objectID + "/links?revision=" + revision + "&direction=outgoing")
	validate("/v1/reader/links", "links", catalogschema.ReaderLinkPageV1Alpha1, links)

	_ = ctx
}

func TestReaderAccessReplacedVaultRootFailsSourceChanged(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-root", BindingID: "binding-root", Scopes: []string{app.ReaderScopeMetadata},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()

	// Before the swap the session reads normally.
	if status, _ := readerDo(t, server, bearer, "/v1/reader/source"); status != http.StatusOK {
		t.Fatalf("pre-swap status = %d", status)
	}

	// Replace the vault directory: rename the authorized root away and
	// create a different directory at the same path. The pinned identity no
	// longer matches; the old bearer must fail with 409 source_changed
	// instead of reading the new content.
	replacement := filepath.Join(t.TempDir(), "replacement")
	if err := os.MkdirAll(replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, root); err != nil {
		t.Fatal(err)
	}
	status, body := readerDo(t, server, bearer, "/v1/reader/source")
	if status != http.StatusConflict || !strings.Contains(body, "source_changed") {
		t.Fatalf("replaced root status=%d body=%s", status, body)
	}
	// Notes listing fails the same way; nothing from the new directory is
	// served under the old authorization.
	if status, body := readerDo(t, server, bearer, "/v1/reader/notes"); status != http.StatusConflict || !strings.Contains(body, "source_changed") {
		t.Fatalf("replaced root notes status=%d body=%s", status, body)
	}
}

func TestReaderAccessAuthFailuresCarryNoStore(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, _, _, bearer := newReaderTestServer(t, root)
	// Missing token: 401 with the reader cache policy on the response.
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/reader/source", nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", response.StatusCode)
	}
	if got := response.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("auth failure cache control = %q", got)
	}
	// Invalid token: same policy.
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/v1/reader/source", nil)
	request.Header.Set("Authorization", "Bearer bogus-token")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d", response.StatusCode)
	}
	if got := response.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("invalid token cache control = %q", got)
	}
	_ = bearer
}

func TestReaderAccessExplicitLimitZeroRejected(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, _, _, fullBearer := newReaderTestServer(t, root)
	for _, tc := range []struct{ name, path string }{
		{"notes", "/v1/reader/notes?limit=0"},
		{"search", "/v1/reader/search?q=alpha&limit=0"},
	} {
		if status, body := readerDo(t, server, fullBearer, tc.path); status != http.StatusBadRequest || !strings.Contains(body, "reader_request_invalid") {
			t.Fatalf("%s limit=0 status=%d body=%s", tc.name, status, body)
		}
	}
	// Links endpoint: revision required first; limit=0 must still 400.
	revision := func() string {
		request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/reader/notes?limit=1", nil)
		request.Header.Set("Authorization", "Bearer "+fullBearer)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		return extractJSONString(string(body), `"revision":"`)
	}()
	if status, body := readerDo(t, server, fullBearer, "/v1/reader/notes/note_alpha/links?revision="+revision+"&limit=0"); status != http.StatusBadRequest || !strings.Contains(body, "reader_request_invalid") {
		t.Fatalf("links limit=0 status=%d body=%s", status, body)
	}
	// Negative limits stay rejected too.
	if status, _ := readerDo(t, server, fullBearer, "/v1/reader/notes?limit=-1"); status != http.StatusBadRequest {
		t.Fatalf("limit=-1 status = %d", status)
	}
}
