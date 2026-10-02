package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestWriterOrganize covers the W04 supervised writer organize lane: plan
// generates and saves a plan under the writer.organizer scope (scoped by
// note_refs or whole-vault), apply consumes the plan id as-is under the
// approval_and_snapshot gate, and a consumed plan never replays silently.
// Fixture vault only: no real vault, no model, no sync.

func writerOrganizeVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "Writer organize fixture"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	fixtures := map[string]string{
		"Alpha.md":      "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000aa\ntitle: Alpha\nkind: reference\n---\n\nAlpha links [[Beta]]. #topic\n",
		"notes/beta.md": "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-0000000000bb\ntitle: Beta\nkind: reference\n---\n\nBeta body.\n",
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

func newOrganizerTestServer(t *testing.T, root string) (*httptest.Server, string) {
	t.Helper()
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	organizerBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-organizer",
		BindingID: "binding-organizer",
		Scopes:    []string{app.ReaderScopeBody, app.WriterScopeOrganizer},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, organizerBearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, organizerBearer
}

func TestWriterOrganizePlanAndApply(t *testing.T) {
	t.Parallel()
	root := writerOrganizeVault(t)
	server, organizerBearer := newOrganizerTestServer(t, root)

	// Scoped plan: exactly the selected note may change; the plan is saved
	// under its plan_id for host review.
	status, envelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{"note_refs": ["01982d84-2b48-7000-8000-0000000000aa", "Alpha.md"]}`)
	if status != http.StatusOK {
		t.Fatalf("scoped plan status = %d, envelope = %v", status, envelope)
	}
	data := writerData(t, envelope)
	planID, _ := data["plan_id"].(string)
	if planID == "" {
		t.Fatalf("scoped plan returned no plan_id: %v", data)
	}
	if saved := writerFact(t, envelope, "saved_path"); saved == "" {
		t.Fatalf("scoped plan was not saved: %v", envelope)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "organize-plans", planID+".json")); err != nil {
		t.Fatalf("saved plan file missing: %v", err)
	}
	// The served plan must conform to the frozen closed schema.
	assertWriterDataConformance(t, envelope, catalogschema.WriterOrganizePlanV1Alpha1)

	// Apply the plan id AS-IS: approval is the writer.organizer scope, the
	// snapshot runs first, and the receipt carries the recovery identity.
	status, applyEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
	if status != http.StatusOK {
		t.Fatalf("apply status = %d, envelope = %v", status, applyEnvelope)
	}
	receipt := writerData(t, applyEnvelope)
	if got, _ := receipt["plan_id"].(string); got != planID {
		t.Fatalf("receipt plan_id = %q, want the applied plan id", got)
	}
	if got, _ := receipt["operation_id"].(string); got != planID {
		t.Fatalf("receipt operation_id = %v, want the plan id as recovery identity", receipt["operation_id"])
	}
	if got, _ := receipt["status"].(string); got != "succeeded" {
		t.Fatalf("receipt status = %q, want succeeded", got)
	}
	if snapshot, _ := receipt["snapshot_id"].(string); snapshot == "" {
		t.Fatalf("receipt missing snapshot_id: %v", receipt)
	}
	if snapshot := writerFact(t, applyEnvelope, "snapshot_id"); snapshot == "" {
		t.Fatal("apply facts missing snapshot_id — snapshot gate skipped")
	}
	// The served organize receipt must conform to the frozen closed schema.
	assertWriterDataConformance(t, applyEnvelope, catalogschema.WriterOrganizeApplyReceiptV1Alpha1)

	// An EXPLICIT selection with blank entries never widens to the whole
	// vault: blank refs are rejected with organize_selection_invalid.
	status, blank := writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{"note_refs": [" "]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("blank note_refs status = %d, envelope = %v", status, blank)
	}
	assertWriterErrorInTable(t, blank, "writer.organize.plan")

	// Re-applying the consumed plan fails explicitly; no silent replay.
	status, replay := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
	if status == http.StatusOK {
		t.Fatalf("consumed plan replayed: %v", replay)
	}
	assertWriterErrorInTable(t, replay, "writer.organize.apply")

	// Unknown plan id: plan_not_found, fail closed.
	status, missingEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "plan-missing-1"}`)
	if status != http.StatusNotFound {
		t.Fatalf("unknown plan status = %d, want 404", status)
	}
	assertWriterErrorInTable(t, missingEnvelope, "writer.organize.apply")

	// Whole-vault plan (no note_refs) still generates and saves.
	status, wholeEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{}`)
	if status != http.StatusOK {
		t.Fatalf("whole-vault plan status = %d, envelope = %v", status, wholeEnvelope)
	}
	if got, _ := writerData(t, wholeEnvelope)["plan_id"].(string); got == "" {
		t.Fatal("whole-vault plan returned no plan_id")
	}

	// Invalid selection: unknown note ref fails closed with 400.
	status, selectionEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{"note_refs": ["01982d84-2b48-7000-8000-00000000f7"]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid selection status = %d, want 400", status)
	}
	assertWriterErrorInTable(t, selectionEnvelope, "writer.organize.plan")
}

func TestWriterOrganizeScopeGate(t *testing.T) {
	t.Parallel()
	root := writerOrganizeVault(t)
	server, organizerBearer := newOrganizerTestServer(t, root)

	// The editor scope does NOT grant organize: independent writer scopes.
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	editorBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-editor-x", BindingID: "binding-editor-x",
		Scopes:    []string{app.WriterScopeEditor},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, editorBearer); err != nil {
		t.Fatal(err)
	}
	editorServer := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer editorServer.Close()
	status, _ := writerPost(t, editorServer, editorBearer, "/v1/writer/organize/plan", `{}`)
	if status != http.StatusForbidden {
		t.Fatalf("editor-scope organize status = %d, want 403", status)
	}

	// Reader-only session on the organizer server: insufficient_scope.
	// Reuse the organizer server with a bogus bearer for auth failure.
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/writer/organize/plan", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+organizerBearer+"x")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad bearer status = %d, want 401", response.StatusCode)
	}

	// Closed payload: unknown field violates the frozen contract.
	status, _ = writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{"note_refs": ["01982d84-2b48-7000-8000-0000000000aa"], "yes": true}`)
	if status != http.StatusBadRequest {
		t.Fatalf("closed organize payload status = %d, want 400", status)
	}
}
