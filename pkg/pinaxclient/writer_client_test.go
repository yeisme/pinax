package pinaxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestWriterClient covers the typed supervised writer surface end to end
// against a real Pinax server (SDK + service integration): typed capture,
// propose, apply, organize plan/apply; same-identity recovery after an
// ambiguous transport failure; identity discipline (no blind retries);
// schema-version safe failure; and permission failures. Fixture vault only.

func writerClientServer(t *testing.T, scopes ...string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "Writer client fixture"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	fixtures := map[string]string{
		"Alpha.md":      "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000c1\ntitle: Alpha\nkind: reference\n---\n\nAlpha body zq-writer-client.\n",
		"notes/beta.md": "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000c2\ntitle: Beta\nkind: reference\n---\n\nBeta body.\n",
	}
	for rel, content := range fixtures {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.IndexRefresh(context.Background(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-writer-client", BindingID: "binding-writer-client", Scopes: scopes,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, bearer
}

func writerClient(t *testing.T, serverURL, bearer string) *Client {
	t.Helper()
	client, err := New(Config{BaseURL: serverURL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestWriterClientTypedCallsAgainstRealServer(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.ReaderScopeBody, app.WriterScopeCapture, app.WriterScopeEditor, app.WriterScopeOrganizer)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()

	// Capture: durable identity, receipt in the frozen schema shape.
	identity, err := NewWriterOperationIdentity("wclient-op-1", "wclient-key-1")
	if err != nil {
		t.Fatal(err)
	}
	capture, err := client.WriterCapture(ctx, WriterCaptureRequest{
		Title: "阅读捕获", Body: "typed client 捕获正文。", Tags: []string{"reading"}, Slug: "typed-capture", Dir: "inbox",
	}, identity)
	if err != nil {
		t.Fatalf("WriterCapture: %v", err)
	}
	if capture.SchemaVersion != WriterCaptureReceiptSchemaV1Alpha1 {
		t.Fatalf("capture schema = %q", capture.SchemaVersion)
	}
	if capture.OperationID != "wclient-op-1" || capture.Status != "succeeded" {
		t.Fatalf("capture receipt = %#v", capture)
	}

	// Propose: exact preview plus digest.
	preview, err := client.WriterPropose(ctx, WriterChange{
		Action: "append", OperationID: "wclient-edit-1",
		NoteRef: "Alpha.md", Body: "\ntyped client 追加。",
	})
	if err != nil {
		t.Fatalf("WriterPropose: %v", err)
	}
	if preview.SchemaVersion != WriterProposalPreviewSchemaV1Alpha1 || preview.PreviewDigest == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if preview.Change.OperationID != "wclient-edit-1" || preview.Change.ExpectedRevision == "" {
		t.Fatalf("preview change not resolved: %#v", preview.Change)
	}

	// Apply: unchanged resolved change + caller assertion.
	apply, err := client.WriterApply(ctx, WriterApplyRequest{
		Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "reviewed_proposal",
	})
	if err != nil {
		t.Fatalf("WriterApply: %v", err)
	}
	if apply.SchemaVersion != WriterApplyReceiptSchemaV1Alpha1 || apply.Status != "succeeded" {
		t.Fatalf("apply receipt = %#v", apply)
	}
	if !apply.Terminal() {
		t.Fatalf("apply receipt not terminal: %#v", apply)
	}
	if apply.RevisionAfter == "" || apply.ReceiptRef == "" {
		t.Fatalf("apply receipt missing recovery anchors: %#v", apply)
	}

	// Organize plan + apply: plan id as-is, snapshot first.
	plan, err := client.WriterOrganizePlan(ctx, []string{"01982d84-2b48-7000-8000-0000000000c1"})
	if err != nil {
		t.Fatalf("WriterOrganizePlan: %v", err)
	}
	if plan.SchemaVersion != WriterOrganizePlanSchemaV1Alpha1 || plan.PlanID == "" {
		t.Fatalf("plan = %#v", plan)
	}
	organize, err := client.WriterOrganizeApply(ctx, WriterOrganizeApplyRequest{PlanID: plan.PlanID})
	if err != nil {
		t.Fatalf("WriterOrganizeApply: %v", err)
	}
	if organize.SchemaVersion != WriterOrganizeApplyReceiptSchemaV1Alpha1 || organize.PlanID != plan.PlanID {
		t.Fatalf("organize receipt = %#v", organize)
	}
	if organize.SnapshotID == "" {
		t.Fatalf("organize receipt missing snapshot: %#v", organize)
	}
}

// TestWriterClientSameIdentityRecovery proves the recovery discipline: an
// ambiguous failure (5xx before the handler runs) is recovered by re-issuing
// the SAME identity exactly once — the backend sees two identical requests,
// the client returns the (replayed) receipt, and no fresh identity appears.
func TestWriterClientSameIdentityRecovery(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.WriterScopeCapture, app.WriterScopeEditor)
	inner := server.Config.Handler
	var captureHits, applyHits int32
	failing := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/capture" {
			if atomic.AddInt32(&captureHits, 1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.capture","status":"failed","error":{"code":"operation_store_unavailable","message":"simulated transient failure"}}`))
				return
			}
		}
		if r.URL.Path == "/v1/writer/proposals:apply" {
			if atomic.AddInt32(&applyHits, 1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.apply","status":"failed","error":{"code":"operation_store_unavailable","message":"simulated transient failure"}}`))
				return
			}
		}
		inner.ServeHTTP(w, r)
	})
	front := httptest.NewServer(failing)
	t.Cleanup(front.Close)
	client := writerClient(t, front.URL, bearer)
	ctx := context.Background()

	identity, err := NewWriterOperationIdentity("wclient-recover-1", "wclient-recover-key-1")
	if err != nil {
		t.Fatal(err)
	}
	capture, err := client.WriterCapture(ctx, WriterCaptureRequest{Title: "恢复捕获"}, identity)
	if err != nil {
		t.Fatalf("WriterCapture recovery: %v", err)
	}
	if capture.OperationID != "wclient-recover-1" || capture.Status != "succeeded" {
		t.Fatalf("recovered capture = %#v", capture)
	}
	if hits := atomic.LoadInt32(&captureHits); hits != 2 {
		t.Fatalf("capture backend hits = %d, want exactly 2 (same identity re-issue)", hits)
	}

	preview, err := client.WriterPropose(ctx, WriterChange{Action: "append", OperationID: "wclient-recover-2", NoteRef: "Alpha.md", Body: "\n恢复应用。"})
	if err != nil {
		t.Fatalf("WriterPropose: %v", err)
	}
	apply, err := client.WriterApply(ctx, WriterApplyRequest{Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "explicit_instruction"})
	if err != nil {
		t.Fatalf("WriterApply recovery: %v", err)
	}
	if apply.OperationID != "wclient-recover-2" || apply.Status != "succeeded" {
		t.Fatalf("recovered apply = %#v", apply)
	}
	if hits := atomic.LoadInt32(&applyHits); hits != 2 {
		t.Fatalf("apply backend hits = %d, want exactly 2 (same identity re-issue)", hits)
	}
}

// TestWriterClientIdentityDiscipline pins the no-blind-retry rules locally:
// invalid or missing identities fail before any HTTP traffic, ambiguous
// failures beyond one recovery report outcome-unknown with the original
// identity, and organize ambiguity never auto-retries (plans are consumed).
func TestWriterClientIdentityDiscipline(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.WriterScopeCapture, app.WriterScopeEditor, app.WriterScopeOrganizer)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()

	if _, err := NewWriterOperationIdentity("", ""); err == nil {
		t.Fatal("empty identity accepted")
	}
	identity, _ := NewWriterOperationIdentity("wclient-disc-1", "wclient-disc-key-1")
	badIdentity := WriterOperationIdentity{OperationID: "not allowed!", IdempotencyKey: "also bad!"}
	if _, err := client.WriterCapture(ctx, WriterCaptureRequest{Title: "t"}, badIdentity); err == nil {
		t.Fatal("invalid identity accepted")
	}

	// Apply without an operation id fails locally.
	if _, err := client.WriterApply(ctx, WriterApplyRequest{Change: WriterChange{Action: "create", Title: "t"}, PreviewDigest: "sha256:" + "ab", Authorization: "explicit_instruction"}); err == nil {
		t.Fatal("identity-less apply accepted")
	}
	// Authorization must be an explicit caller assertion.
	preview, err := client.WriterPropose(ctx, WriterChange{Action: "create", OperationID: "wclient-disc-2", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriterApply(ctx, WriterApplyRequest{Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "host_approved"}); err == nil {
		t.Fatal("host-approval assertion accepted")
	}
	_ = identity

	// Persistent ambiguity: one recovery attempt, then outcome-unknown with
	// the ORIGINAL identity and a reconcile-required marker.
	var hits int32
	inner := server.Config.Handler
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/capture" {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.capture","status":"failed","error":{"code":"request_failed","message":"simulated persistent failure"}}`))
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(failing.Close)
	flaky := writerClient(t, failing.URL, bearer)
	_, err = flaky.WriterCapture(ctx, WriterCaptureRequest{Title: "持续失败"}, identity)
	if err == nil {
		t.Fatal("persistent ambiguity accepted")
	}
	if hits != 2 {
		t.Fatalf("backend hits = %d, want exactly 2 (original + one same-identity recovery)", hits)
	}
	writerErr, ok := err.(*Error)
	if !ok || writerErr.Code != CodeMutationOutcomeUnknown || writerErr.OperationRef != identity.OperationID || !writerErr.ReconcileRequired {
		t.Fatalf("outcome-unknown error = %#v", err)
	}

	// Organize ambiguity never auto-retries: the plan is consumed by apply.
	var organizeHits int32
	organizeFailing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/organize/apply" {
			atomic.AddInt32(&organizeHits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.organize.apply","status":"failed","error":{"code":"operation_store_unavailable","message":"simulated"}}`))
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(organizeFailing.Close)
	organizeClient := writerClient(t, organizeFailing.URL, bearer)
	_, err = organizeClient.WriterOrganizeApply(ctx, WriterOrganizeApplyRequest{PlanID: "plan-unknown-1"})
	if err == nil {
		t.Fatal("ambiguous organize apply accepted")
	}
	if organizeHits != 1 {
		t.Fatalf("organize backend hits = %d, want exactly 1 (no auto retry)", organizeHits)
	}
	if organizeErr, ok := err.(*Error); !ok || organizeErr.Code != CodeMutationOutcomeUnknown || organizeErr.OperationRef != "plan-unknown-1" {
		t.Fatalf("organize outcome-unknown = %#v", err)
	}
}

// TestWriterClientScopeFailures pins permission failures through the typed
// surface: missing scopes surface the frozen server error codes verbatim.
func TestWriterClientScopeFailures(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.ReaderScopeBody)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()

	identity, _ := NewWriterOperationIdentity("wclient-scope-1", "wclient-scope-key-1")
	_, err := client.WriterCapture(ctx, WriterCaptureRequest{Title: "越权"}, identity)
	if err == nil {
		t.Fatal("capture without scope accepted")
	}
	if writerErr, ok := err.(*Error); !ok || writerErr.Code != "insufficient_scope" {
		t.Fatalf("capture scope error = %#v", err)
	}

	_, err = client.WriterPropose(ctx, WriterChange{Action: "append", NoteRef: "Alpha.md", Body: "x"})
	if err == nil {
		t.Fatal("propose without scope accepted")
	}
	if writerErr, ok := err.(*Error); !ok || writerErr.Code != "insufficient_scope" {
		t.Fatalf("propose scope error = %#v", err)
	}
}

// TestWriterClientAwaitApply polls the SAME identity on persistent
// ambiguity until the budget is exhausted; it never issues a fresh one.
func TestWriterClientAwaitApply(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.WriterScopeEditor)
	var hits int32
	inner := server.Config.Handler
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/proposals:apply" {
			if atomic.AddInt32(&hits, 1) <= 2 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.apply","status":"failed","error":{"code":"request_failed","message":"simulated"}}`))
				return
			}
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(failing.Close)
	client := writerClient(t, failing.URL, bearer)
	ctx := context.Background()

	preview, err := client.WriterPropose(ctx, WriterChange{Action: "append", OperationID: "wclient-await-1", NoteRef: "Alpha.md", Body: "\n轮询恢复。"})
	if err != nil {
		t.Fatalf("WriterPropose: %v", err)
	}
	receipt, err := client.WriterAwaitApply(ctx, WriterApplyRequest{Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "explicit_instruction"}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("WriterAwaitApply: %v", err)
	}
	if receipt.OperationID != "wclient-await-1" || receipt.Status != "succeeded" {
		t.Fatalf("awaited receipt = %#v", receipt)
	}
	// Each poll re-issues the same apply once (plus its built-in single
	// recovery on ambiguity): the failing front drops the first two calls.
	if hits < 3 {
		t.Fatalf("await apply hits = %d, want at least 3 same-identity attempts", hits)
	}
}

// TestWriterClientAwaitApplyWaitsForTerminal pins the polling contract: a
// successful but NON-terminal receipt (accepted) keeps polling the SAME
// identity until terminal; exhausting the budget returns the in-flight
// receipt with an error instead of pretending the apply finished.
func TestWriterClientAwaitApplyWaitsForTerminal(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.WriterScopeEditor)
	inner := server.Config.Handler
	var calls int32
	inFlight := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/proposals:apply" {
			hit := atomic.AddInt32(&calls, 1)
			status := "applying"
			if hit >= 3 {
				status = "succeeded"
			}
			receipt := map[string]any{
				"schema_version": catalogschema.WriterApplyReceiptV1Alpha1,
				"operation_id":   "wclient-await-2", "status": status,
				"retryable": false, "replay_safe": true, "reconcile_required": false,
				"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:00Z",
				"accepted_at": "2026-10-02T12:00:00Z",
			}
			encoded, _ := json.Marshal(map[string]any{
				"spec_version": ProjectionSpecVersion, "command": "writer.apply", "status": "success",
				"summary": "in flight", "data": receipt,
			})
			_, _ = w.Write(encoded)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(inFlight.Close)
	client := writerClient(t, inFlight.URL, bearer)
	ctx := context.Background()

	request := WriterApplyRequest{
		Change:        WriterChange{Action: "append", OperationID: "wclient-await-2", NoteRef: "Alpha.md", Body: "\n等待终态。"},
		PreviewDigest: "sha256:" + strings.Repeat("ab", 32),
		Authorization: "explicit_instruction",
	}
	receipt, err := client.WriterAwaitApply(ctx, request, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("WriterAwaitApply terminal: %v", err)
	}
	if receipt.Status != "succeeded" || !receipt.Terminal() {
		t.Fatalf("awaited receipt = %#v, want terminal succeeded", receipt)
	}
	if calls != 3 {
		t.Fatalf("poll calls = %d, want 3 (two non-terminal then terminal)", calls)
	}

	// Budget exhausted while still in flight: the in-flight receipt comes
	// back with an error — never a fabricated terminal state.
	exhausted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/proposals:apply" {
			receipt := map[string]any{
				"schema_version": catalogschema.WriterApplyReceiptV1Alpha1,
				"operation_id":   "wclient-await-3", "status": "accepted",
				"retryable": false, "replay_safe": true, "reconcile_required": false,
				"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:00Z",
				"accepted_at": "2026-10-02T12:00:00Z",
			}
			encoded, _ := json.Marshal(map[string]any{
				"spec_version": ProjectionSpecVersion, "command": "writer.apply", "status": "success",
				"summary": "accepted", "data": receipt,
			})
			_, _ = w.Write(encoded)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(exhausted.Close)
	stuck := writerClient(t, exhausted.URL, bearer)
	stuckRequest := WriterApplyRequest{
		Change:        WriterChange{Action: "append", OperationID: "wclient-await-3", NoteRef: "Alpha.md", Body: "\n卡在途。"},
		PreviewDigest: "sha256:" + strings.Repeat("cd", 32),
		Authorization: "explicit_instruction",
	}
	inFlightReceipt, err := stuck.WriterAwaitApply(ctx, stuckRequest, 2, time.Millisecond)
	if err == nil {
		t.Fatal("exhausted await returned no error")
	}
	// The budget-exhausted result IS the in-flight receipt: non-terminal,
	// still accepted, and never a fabricated terminal state.
	if inFlightReceipt.Status != "accepted" || inFlightReceipt.Terminal() {
		t.Fatalf("exhausted receipt = %#v, want the non-terminal in-flight receipt", inFlightReceipt)
	}
}
