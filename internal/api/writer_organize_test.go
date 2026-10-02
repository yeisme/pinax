package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
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

// TestWriterOrganizeWholeVaultConsumption pins the whole-vault plan
// consumption contract: a legacy (non-scoped) plan applies once, its
// consumed status persists to the plan file, and replaying the same plan id
// fails explicitly with organize_plan_not_planned — the same explicit
// consumption the scoped machinery enforces.
func TestWriterOrganizeWholeVaultConsumption(t *testing.T) {
	t.Parallel()
	root := writerOrganizeVault(t)
	server, organizerBearer := newOrganizerTestServer(t, root)

	// Whole-vault plan (no note_refs).
	status, wholeEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/plan", `{}`)
	if status != http.StatusOK {
		t.Fatalf("whole-vault plan status = %d, envelope = %v", status, wholeEnvelope)
	}
	planID, _ := writerData(t, wholeEnvelope)["plan_id"].(string)
	if planID == "" {
		t.Fatal("whole-vault plan returned no plan_id")
	}

	// Apply once: succeeds (no operations is a valid whole-vault outcome)
	// and persists consumption.
	status, applyEnvelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
	if status != http.StatusOK {
		t.Fatalf("whole-vault apply status = %d, envelope = %v", status, applyEnvelope)
	}
	receipt := writerData(t, applyEnvelope)
	if got, _ := receipt["status"].(string); got != "succeeded" && got != "failed" {
		t.Fatalf("whole-vault receipt status = %v", receipt["status"])
	}

	// The plan file records the consumed status.
	consumedStatus := readPlanStatus(t, root, planID)
	if consumedStatus != "applied" && consumedStatus != "failed" && consumedStatus != "partial" {
		t.Fatalf("plan file status after apply = %q, want a consumed state", consumedStatus)
	}

	// Replay: explicit organize_plan_not_planned, never a silent re-plan.
	status, replay := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
	if status == http.StatusOK {
		t.Fatalf("consumed whole-vault plan replayed: %v", replay)
	}
	assertWriterErrorInTable(t, replay, "writer.organize.apply")
	if code, _ := replay["error"].(map[string]any)["code"].(string); code != "organize_plan_not_planned" {
		t.Fatalf("consumed plan code = %q, want organize_plan_not_planned", code)
	}
}

// TestWriterOrganizeCorruptedPlan pins the frozen-table guard on raw load
// errors: corrupted plan JSON surfaces as the frozen writer_unavailable
// umbrella, never an undeclared code on the wire.
func TestWriterOrganizeCorruptedPlan(t *testing.T) {
	t.Parallel()
	root := writerOrganizeVault(t)
	server, organizerBearer := newOrganizerTestServer(t, root)

	plansDir := filepath.Join(root, ".pinax", "organize-plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "plan-corrupt-1.json"), []byte(`{"schema_version": `), 0o644); err != nil {
		t.Fatal(err)
	}
	status, envelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "plan-corrupt-1"}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("corrupted plan status = %d, envelope = %v", status, envelope)
	}
	if code, _ := envelope["error"].(map[string]any)["code"].(string); code != "writer_unavailable" {
		t.Fatalf("corrupted plan code = %q, want writer_unavailable", code)
	}
	assertWriterErrorInTable(t, envelope, "writer.organize.apply")
}

func readPlanStatus(t *testing.T, root, planID string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".pinax", "organize-plans", planID+".json"))
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	var plan struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode plan file: %v", err)
	}
	return plan.Status
}

// TestWriterOrganizeFaultInjection covers the four fault windows of the
// whole-vault consumption contract with fixture injection: (1) a crash
// between the writes and the terminal state save, (2) a consumption-state
// save failure BEFORE any write (fail closed, nothing mutated), (3) a
// partial apply failure (recovery facts preserved, plan marked partial),
// and (4) concurrent consumption of the same plan (exactly one winner).
func TestWriterOrganizeFaultInjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	wholeVaultPlan := func(t *testing.T, root string) string {
		t.Helper()
		svc := app.NewService()
		p, err := svc.SuggestOrganize(ctx, app.OrganizeSuggestRequest{VaultPath: root, Save: true})
		if err != nil {
			t.Fatal(err)
		}
		return p.Data.(domain.OrganizePlan).PlanID
	}

	t.Run("crash after writes before terminal save", func(t *testing.T) {
		root := writerOrganizeVault(t)
		server, organizerBearer := newOrganizerTestServer(t, root)
		planID := wholeVaultPlan(t, root)
		if status, _ := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`); status != http.StatusOK {
			t.Fatalf("first apply failed")
		}
		// Simulate the crash window: the terminal save never landed, the
		// plan still says applying.
		rewritePlanStatus(t, root, planID, "applying")
		status, replay := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
		if status == http.StatusOK {
			t.Fatalf("crash-window plan replayed: %v", replay)
		}
		if code, _ := replay["error"].(map[string]any)["code"].(string); code != "organize_plan_not_planned" {
			t.Fatalf("crash-window replay code = %q", code)
		}
	})

	t.Run("consumption save failure before writes", func(t *testing.T) {
		root := writerOrganizeVault(t)
		server, organizerBearer := newOrganizerTestServer(t, root)
		planID := wholeVaultPlan(t, root)
		planFile := filepath.Join(root, ".pinax", "organize-plans", planID+".json")
		if err := os.Chmod(planFile, 0o444); err != nil {
			t.Fatal(err)
		}
		status, envelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
		if status != http.StatusInternalServerError {
			t.Fatalf("pre-write save failure status = %d, envelope = %v", status, envelope)
		}
		if code, _ := envelope["error"].(map[string]any)["code"].(string); code != "plan_save_failed" {
			t.Fatalf("pre-write save failure code = %q, want plan_save_failed", code)
		}
		// Nothing was written: Alpha stays at the vault root.
		if _, err := os.Stat(filepath.Join(root, "notes", "alpha.md")); !os.IsNotExist(err) {
			t.Fatal("pre-write save failure still moved notes")
		}
		// After the fault clears, the SAME plan id applies cleanly.
		if err := os.Chmod(planFile, 0o644); err != nil {
			t.Fatal(err)
		}
		if status, _ := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`); status != http.StatusOK {
			t.Fatalf("retry after cleared fault failed")
		}
	})

	t.Run("partial apply failure preserves recovery facts", func(t *testing.T) {
		root := writerOrganizeVault(t)
		server, organizerBearer := newOrganizerTestServer(t, root)
		planID := wholeVaultPlan(t, root)
		// Beta's status patch fails mid-apply AFTER Alpha's metadata
		// patches already landed: the response must carry the recovery
		// record and the plan must be marked partial, never failed.
		// Metadata patches write via atomic rename, so the blocker is a
		// read-only notes/ DIRECTORY, not a read-only file.
		notesDir := filepath.Join(root, "notes")
		if err := os.Chmod(notesDir, 0o555); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(notesDir, 0o755) }()
		status, envelope := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
		if status == http.StatusOK {
			t.Fatalf("partial apply reported success: %v", envelope)
		}
		facts, _ := envelope["facts"].(map[string]any)
		if facts["applied"] != "2" {
			t.Fatalf("partial apply facts = %#v, want applied=2 preserved", facts)
		}
		if _, hasChanged := facts["changed_paths_status"]; !hasChanged {
			t.Fatalf("partial apply lost the changed-paths marker: %#v", facts)
		}
		if readPlanStatus(t, root, planID) != "partial" {
			t.Fatalf("partial plan recorded as %q", readPlanStatus(t, root, planID))
		}
		// The already-applied metadata is on disk; replay is rejected.
		status, replay := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
		if status == http.StatusOK {
			t.Fatalf("partial plan replayed: %v", replay)
		}
	})

	t.Run("concurrent consumption has exactly one winner", func(t *testing.T) {
		root := writerOrganizeVault(t)
		server, organizerBearer := newOrganizerTestServer(t, root)
		planID := wholeVaultPlan(t, root)
		results := make(chan int, 2)
		for i := 0; i < 2; i++ {
			go func() {
				status, _ := writerPost(t, server, organizerBearer, "/v1/writer/organize/apply", `{"plan_id": "`+planID+`"}`)
				results <- status
			}()
		}
		first, second := <-results, <-results
		winners := 0
		for _, status := range []int{first, second} {
			if status == http.StatusOK {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("concurrent apply winners = %d (statuses %d, %d), want exactly 1", winners, first, second)
		}
	})
}

func rewritePlanStatus(t *testing.T, root, planID, status string) {
	t.Helper()
	path := filepath.Join(root, ".pinax", "organize-plans", planID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	plan["status"] = status
	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}
