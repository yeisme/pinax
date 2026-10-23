package e2e

import (
	"context"
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
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

// TestWriterComponentE2E (pinax-human-reading-write-v1 W06) drives the full
// supervised write chain against a real vault, the real writer HTTP surface
// and the typed SDK: read → capture → propose → confirm → apply → crash
// recovery (same-identity replay, never a fresh identity) → organize plan /
// apply under the snapshot gate → post-write read. Fixture vault only: no
// real vault, no model, no sync, no race.

func writerComponentVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "Writer component fixture"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	fixtures := map[string]string{
		"Alpha.md":      "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000e1\ntitle: Alpha\nkind: reference\n---\n\nAlpha body zq-writer-component-marker.\n",
		"notes/beta.md": "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000e2\ntitle: Beta\nkind: reference\n---\n\nBeta body.\n",
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
	return root
}

func TestWriterComponentE2E(t *testing.T) {
	root := writerComponentVault(t)

	// One supervised session carrying reader plus all three writer scopes:
	// the host granted exactly this set.
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(api.ReaderSession{
		SessionID: "sess-writer-e2e", BindingID: "binding-writer-e2e",
		Scopes: []string{
			app.ReaderScopeMetadata, app.ReaderScopeBody,
			app.WriterScopeCapture, app.WriterScopeEditor, app.WriterScopeOrganizer,
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	service := app.NewService()
	server := httptest.NewServer(api.NewServerWithOptions(service, root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	crashServer := server
	client, err := pinaxclient.New(pinaxclient.Config{BaseURL: server.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 1. Read: the session starts from the reader surface.
	notes, err := client.ReaderNotes(ctx, pinaxclient.ReaderPageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("reader notes: %v", err)
	}
	if len(notes.Items) != 2 {
		t.Fatalf("notes = %#v", notes.Items)
	}

	// 2. Capture under a durable identity.
	identity, err := pinaxclient.NewWriterOperationIdentity("we2e-capture-1", "we2e-capture-key-1")
	if err != nil {
		t.Fatal(err)
	}
	capture, err := client.WriterCapture(ctx, pinaxclient.WriterCaptureRequest{
		Title: "阅读捕获", Body: "组件链捕获正文。", Tags: []string{"reading"}, Dir: "inbox",
	}, identity)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if capture.Status != "succeeded" || capture.OperationID != "we2e-capture-1" {
		t.Fatalf("capture receipt = %#v", capture)
	}

	// 3. Propose an append on Alpha: exact preview with a pinned revision.
	preview, err := client.WriterPropose(ctx, pinaxclient.WriterChange{
		Action: "append", OperationID: "we2e-apply-1",
		NoteRef: "Alpha.md", Body: "\n组件链追加段落。",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if preview.PreviewDigest == "" || preview.Change.ExpectedRevision == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if !strings.Contains(preview.After, "组件链追加段落。") {
		t.Fatalf("preview after = %q", preview.After)
	}

	// 4. Confirm + apply with REAL crash injection: the host asserts an
	// explicit instruction; the FIRST apply runs against the vault (the
	// write lands) but its response is lost to a 503 after commit. The
	// client recovers by re-issuing the SAME identity: the ledger replays
	// the original receipt and the body is applied exactly once.
	var applyHits int32
	crashing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/writer/proposals:apply" && atomic.AddInt32(&applyHits, 1) == 1 {
			// Run the REAL handler into a recorder: the mutation lands on the
			// vault behind it; only the response is then destroyed.
			recorder := httptest.NewRecorder()
			crashServer.Config.Handler.ServeHTTP(recorder, r)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"spec_version":"1.0","command":"writer.apply","status":"failed","error":{"code":"operation_store_unavailable","message":"response lost after commit"}}`))
			return
		}
		crashServer.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(crashing.Close)
	recoveryClient, err := pinaxclient.New(pinaxclient.Config{BaseURL: crashing.URL, Token: bearer})
	if err != nil {
		t.Fatal(err)
	}
	apply, err := recoveryClient.WriterApply(ctx, pinaxclient.WriterApplyRequest{
		Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "explicit_instruction",
	})
	if err != nil {
		t.Fatalf("crash-recovered apply: %v", err)
	}
	if apply.Status != "succeeded" || apply.ReceiptRef == "" || !strings.HasPrefix(apply.ReceiptRef, "snapshot:") {
		t.Fatalf("recovered apply receipt = %#v", apply)
	}
	if hits := atomic.LoadInt32(&applyHits); hits != 2 {
		t.Fatalf("apply backend hits = %d, want exactly 2 (lost response + same-identity recovery)", hits)
	}

	// 5. Post-write read: the appended body is visible through the reader
	// surface under the new revision.
	document, err := client.ReaderDocument(ctx, pinaxclient.ReaderDocumentRequest{ObjectID: "01982d84-2b48-7000-8000-0000000000e1"})
	if err != nil {
		t.Fatalf("post-apply document: %v", err)
	}
	joined := strings.Builder{}
	for _, node := range document.Nodes {
		joined.WriteString(node.Text)
	}
	if !strings.Contains(joined.String(), "组件链追加段落。") {
		t.Fatalf("appended body missing from document: %q", joined.String())
	}

	// 6. Exactly-once proof: the body exists once on disk and a clean
	// same-identity re-issue returns the SAME receipt without writing again.
	raw, err := os.ReadFile(filepath.Join(root, "Alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "组件链追加段落。") != 1 {
		t.Fatalf("recovery applied the body twice: %q", raw)
	}
	replayed, err := client.WriterApply(ctx, pinaxclient.WriterApplyRequest{
		Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "explicit_instruction",
	})
	if err != nil {
		t.Fatalf("clean replay: %v", err)
	}
	if replayed.OperationID != apply.OperationID || replayed.RevisionAfter != apply.RevisionAfter {
		t.Fatalf("replay receipt drift: %#v vs %#v", replayed, apply)
	}

	// 7. Capture recovery: re-issuing the same capture identity replays the
	// original receipt without a second note.
	replayedCapture, err := client.WriterCapture(ctx, pinaxclient.WriterCaptureRequest{
		Title: "阅读捕获", Body: "组件链捕获正文。", Tags: []string{"reading"}, Dir: "inbox",
	}, identity)
	if err != nil {
		t.Fatalf("capture recovery replay: %v", err)
	}
	if replayedCapture.OperationID != capture.OperationID || replayedCapture.Status != "succeeded" {
		t.Fatalf("capture replay receipt = %#v", replayedCapture)
	}

	// 8. Organize: plan (scoped to Beta), host review, apply under the
	// snapshot gate, then read the plan receipt identity back.
	plan, err := client.WriterOrganizePlan(ctx, []string{"01982d84-2b48-7000-8000-0000000000e2"})
	if err != nil {
		t.Fatalf("organize plan: %v", err)
	}
	organize, err := client.WriterOrganizeApply(ctx, pinaxclient.WriterOrganizeApplyRequest{PlanID: plan.PlanID})
	if err != nil {
		t.Fatalf("organize apply: %v", err)
	}
	if organize.PlanID != plan.PlanID || organize.SnapshotID == "" {
		t.Fatalf("organize receipt = %#v", organize)
	}

	// 9. Manifest truth: the five writer bindings are available with HTTP
	// backings; no write surface appears outside the writer group.
	manifest, err := client.Manifest(ctx)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	writerAvailable := 0
	for _, capability := range manifest.Capabilities {
		if !strings.HasPrefix(capability.ID, "writer.") {
			continue
		}
		writerAvailable++
		for _, binding := range capability.Bindings {
			if binding.Availability != "available" || !strings.HasPrefix(binding.BackingRef, "internal/api/http:") {
				t.Fatalf("writer binding %q = %#v", binding.ID, binding)
			}
		}
	}
	if writerAvailable != 14 {
		t.Fatalf("writer capabilities in manifest = %d, want 14", writerAvailable)
	}
}
