package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/transportcatalog"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestWriterCapture covers the W02 supervised writer capture lane: the
// writer.capture scope is the host gate, operation identity is REQUIRED
// (approval + idempotency), receipts replay under the same identity
// without a second write, legacy tokens never reach the writer surface,
// and reader-only sessions are refused with insufficient_scope. Fixture
// vault only: no real vault, no model, no sync.

func newWriterTestServer(t *testing.T, root string) (*httptest.Server, string, string) {
	t.Helper()
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	writerBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	readerBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-writer",
		BindingID: "binding-writer",
		Scopes:    []string{app.ReaderScopeBody, app.WriterScopeCapture},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, writerBearer); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-reader",
		BindingID: "binding-reader",
		Scopes:    []string{app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, readerBearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, writerBearer, readerBearer
}

func writerCaptureDo(t *testing.T, server *httptest.Server, bearer, body, operationID, idempotencyKey string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/writer/capture", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if operationID != "" {
		request.Header.Set("X-Pinax-Operation-ID", operationID)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	envelope := map[string]any{}
	if decodeErr := json.Unmarshal(raw, &envelope); decodeErr != nil {
		t.Fatalf("capture response is not JSON: %v (%s)", decodeErr, raw)
	}
	return response.StatusCode, envelope
}

func writerFact(t *testing.T, envelope map[string]any, key string) string {
	t.Helper()
	facts, _ := envelope["facts"].(map[string]any)
	value, _ := facts[key].(string)
	return value
}

// assertWriterDataConformance proves the SERVED data matches the frozen
// response schema exactly (closed objects): the wire format can never
// drift from the contract without failing the lane.
func assertWriterDataConformance(t *testing.T, envelope map[string]any, schemaID string) {
	t.Helper()
	data, _ := envelope["data"].(map[string]any)
	if data == nil {
		t.Fatalf("writer response carries no data object: %v", envelope)
	}
	if err := catalogschema.ValidatePayload(catalogschema.Default(), schemaID, data); err != nil {
		t.Fatalf("served data violates frozen schema %s: %v (data=%v)", schemaID, err, data)
	}
}

// assertWriterErrorInTable proves the SERVED error code belongs to the
// frozen capability error table: no writer path can surface an
// off-contract error code.
func assertWriterErrorInTable(t *testing.T, envelope map[string]any, capabilityID string) {
	t.Helper()
	errObject, _ := envelope["error"].(map[string]any)
	code, _ := errObject["code"].(string)
	if code == "" {
		t.Fatalf("writer error envelope carries no error code: %v", envelope)
	}
	for _, definition := range transportcatalog.WriterCapabilityDefinitions() {
		if definition.ID != capabilityID {
			continue
		}
		for _, declared := range definition.Errors {
			if declared == code {
				return
			}
		}
		t.Fatalf("served error code %q is outside the frozen %s error table", code, capabilityID)
	}
	t.Fatalf("unknown writer capability %q", capabilityID)
}

func TestWriterCaptureHTTP(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, writerBearer, readerBearer := newWriterTestServer(t, root)

	body := `{"title": "阅读捕获", "body": "受监督写入的捕获正文。", "tags": ["reading"], "slug": "writer-capture", "dir": "inbox"}`

	// Happy path: writer scope + operation identity + idempotency key.
	status, envelope := writerCaptureDo(t, server, writerBearer, body, "wop-capture-1", "wkey-capture-1")
	if status != http.StatusOK {
		t.Fatalf("capture status = %d, envelope = %v", status, envelope)
	}
	if got := writerFact(t, envelope, "operation_id"); got != "wop-capture-1" {
		t.Fatalf("operation_id fact = %q", got)
	}
	if got := writerFact(t, envelope, "operation_status"); got != "succeeded" {
		t.Fatalf("operation_status fact = %q, want succeeded", got)
	}
	if writerFact(t, envelope, "idempotent_replay") == "true" {
		t.Fatal("first capture must not be a replay")
	}
	// The served receipt must conform to the frozen closed schema.
	assertWriterDataConformance(t, envelope, catalogschema.WriterCaptureReceiptV1Alpha1)
	noteID := writerFact(t, envelope, "note_id")
	if noteID == "" {
		// note_id lives in the projection data for inbox captures; the
		// operation facts carry the resource ref instead.
		if writerFact(t, envelope, "resource_ref") == "" {
			t.Fatalf("capture receipt missing note identity: %v", envelope)
		}
	}

	// Idempotent replay: the SAME identity returns the same receipt and
	// never performs a second write. This is the recoverable-receipt path
	// a host uses after a timeout.
	before := countVaultNotes(t, root)
	status, replay := writerCaptureDo(t, server, writerBearer, body, "wop-capture-1", "wkey-capture-1")
	if status != http.StatusOK {
		t.Fatalf("replay status = %d", status)
	}
	if got := writerFact(t, replay, "idempotent_replay"); got != "true" {
		t.Fatalf("replay fact = %q, want true", got)
	}
	if got := writerFact(t, replay, "operation_id"); got != "wop-capture-1" {
		t.Fatalf("replay operation_id = %q", got)
	}
	if after := countVaultNotes(t, root); after != before {
		t.Fatalf("replay wrote again: %d -> %d notes", before, after)
	}

	// Same operation identity with a DIFFERENT payload is a conflict, not
	// a silent second write under a borrowed identity.
	status, conflictEnvelope := writerCaptureDo(t, server, writerBearer, `{"title": "不同标题"}`, "wop-capture-1", "wkey-capture-1")
	if status != http.StatusConflict {
		t.Fatalf("same identity different payload status = %d, want 409", status)
	}
	assertWriterErrorInTable(t, conflictEnvelope, "writer.capture")

	// Operation identity is required: no header pair, no write.
	status, identityEnvelope := writerCaptureDo(t, server, writerBearer, body, "", "")
	if status != http.StatusBadRequest {
		t.Fatalf("missing identity status = %d, want 400", status)
	}
	assertWriterErrorInTable(t, identityEnvelope, "writer.capture")
	status, _ = writerCaptureDo(t, server, writerBearer, body, "wop-capture-2", "")
	if status != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status = %d, want 400", status)
	}

	// Reader-only sessions never gain the write capability.
	status, scopeEnvelope := writerCaptureDo(t, server, readerBearer, body, "wop-capture-3", "wkey-capture-3")
	if status != http.StatusForbidden {
		t.Fatalf("reader-only capture status = %d, want 403", status)
	}
	assertWriterErrorInTable(t, scopeEnvelope, "writer.capture")

	// No bearer: 401, never an anonymous write.
	status, _ = writerCaptureDo(t, server, "", body, "wop-capture-4", "wkey-capture-4")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous capture status = %d, want 401", status)
	}

	// Closed payload: unknown fields and empty titles violate the frozen
	// contract and fail with writer_request_invalid.
	status, _ = writerCaptureDo(t, server, writerBearer, `{"title": "t", "yes": true}`, "wop-capture-5", "wkey-capture-5")
	if status != http.StatusBadRequest {
		t.Fatalf("closed body status = %d, want 400", status)
	}
	status, _ = writerCaptureDo(t, server, writerBearer, `{"body": "no title"}`, "wop-capture-6", "wkey-capture-6")
	if status != http.StatusBadRequest {
		t.Fatalf("missing title status = %d, want 400", status)
	}

	// Writer routes are POST-only.
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/writer/capture", nil)
	request.Header.Set("Authorization", "Bearer "+writerBearer)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET capture status = %d, want 405", response.StatusCode)
	}

	// Cross-session identity theft fails closed: a DIFFERENT session
	// replaying the same operation identity is a conflict, never a
	// replay of someone else's receipt. Durable cross-session recovery
	// stays with the owner-local operation.show/reconcile contract.
	sessions2, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	freshBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions2.Create(ReaderSession{
		SessionID: "sess-writer-3", BindingID: "binding-writer-3",
		Scopes:    []string{app.WriterScopeCapture},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, freshBearer); err != nil {
		t.Fatal(err)
	}
	server2 := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions2}).Handler())
	defer server2.Close()
	status, _ = writerCaptureDo(t, server2, freshBearer, body, "wop-capture-1", "wkey-capture-1")
	if status != http.StatusConflict {
		t.Fatalf("cross-session same identity status = %d, want 409 fail closed", status)
	}

	// Legacy token surfaces never expose the writer group: a temp-token
	// server with the same session store still refuses writer routes.
	legacyServer := NewServerWithOptions(app.NewService(), root, ServerOptions{AuthMode: AuthModeTemp, ReaderSessions: func() *ReaderSessionStore { return nil }()})
	legacy := httptest.NewServer(legacyServer.Handler())
	defer legacy.Close()
	request, _ = http.NewRequest(http.MethodPost, legacy.URL+"/v1/writer/capture", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+legacyServer.tempSecret)
	request.Header.Set("X-Pinax-Operation-ID", "wop-capture-7")
	request.Header.Set("Idempotency-Key", "wkey-capture-7")
	response, err = legacy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("legacy temp token reached writer surface: %d", response.StatusCode)
	}
}

func countVaultNotes(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// TestWriterCaptureNoStorePolicy pins the cache policy: writer responses —
// including auth failures — never enter any shared cache.
func TestWriterCaptureNoStorePolicy(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, writerBearer, _ := newWriterTestServer(t, root)

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/writer/capture", strings.NewReader(`{"title": "缓存策略"}`))
	request.Header.Set("Authorization", "Bearer "+writerBearer)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if cache := response.Header.Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("capture Cache-Control = %q, want private, no-store", cache)
	}

	// The supervised session with a writer scope is not readonly: the
	// bootstrap frame correlation is covered by the session tests; here we
	// pin that a writer scope never rides a readonly frame.
	frame := SessionBootstrapFrame{
		SchemaVersion: "pinax.local_session.bootstrap.v1alpha1",
		SessionNonce:  "nonce-0123456789abcdef",
		Bearer:        "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		BindingID:     "binding-w", VaultRoot: root,
		Scopes: []string{app.WriterScopeCapture}, Readonly: true,
	}
	if _, err := ReadSessionBootstrap(strings.NewReader(mustJSONString(frame))); err == nil {
		t.Fatal("writer scope with readonly=true must be rejected by the bootstrap reader")
	}
	frame.Readonly = false
	if _, err := ReadSessionBootstrap(strings.NewReader(mustJSONString(frame))); err != nil {
		t.Fatalf("writer frame with readonly=false rejected: %v", err)
	}
}

func mustJSONString(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
