package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

// newTrustCenterTestServer creates a Server backed by a fresh initialized
// vault, matching the existing dashboard test setup pattern.
func newTrustCenterTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	return NewServer(svc, root)
}

func TestAgentContinuityRoute_GET(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodGet, "/api/agent-continuity", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("agent-continuity status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("agent-continuity json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["schema_version"] != "yeisme.agent_continuity.v1" {
		t.Fatalf("agent-continuity schema_version = %#v", payload["schema_version"])
	}
	if payload["experimental"] != true {
		t.Fatalf("agent-continuity missing experimental flag: %#v", payload)
	}
}

func TestAgentContinuityRoute_MethodNotAllowed(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodPost, "/api/agent-continuity", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("agent-continuity POST status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}

func TestMemoryInboxRoute_GET(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodGet, "/api/memory-inbox", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("memory-inbox status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("memory-inbox json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["schema_version"] != "yeisme.memory_inbox.v1" {
		t.Fatalf("memory-inbox schema_version = %#v", payload["schema_version"])
	}
	if payload["experimental"] != true {
		t.Fatalf("memory-inbox missing experimental flag: %#v", payload)
	}
	if _, ok := payload["items"]; !ok {
		t.Fatalf("memory-inbox missing items field: %#v", payload)
	}
}

func TestMemoryInboxRoute_MethodNotAllowed(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodPost, "/api/memory-inbox", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("memory-inbox POST status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}

func TestTrustMetricsRoute_GET(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodGet, "/api/trust-metrics", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("trust-metrics status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("trust-metrics json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["schema_version"] != "yeisme.trust_center.v1" {
		t.Fatalf("trust-metrics schema_version = %#v", payload["schema_version"])
	}
	if payload["experimental"] != true {
		t.Fatalf("trust-metrics missing experimental flag: %#v", payload)
	}
	metrics, ok := payload["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("trust-metrics missing metrics object: %#v", payload)
	}
	for _, key := range []string{"context_reuse", "source_resolvability", "proposal_acceptance", "handoff_continuation", "silent_promotion"} {
		if _, ok := metrics[key]; !ok {
			t.Fatalf("trust-metrics missing metric %q: %#v", key, metrics)
		}
	}
}

func TestTrustMetricsRoute_MethodNotAllowed(t *testing.T) {
	server := newTrustCenterTestServer(t)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, newLocalRequest(http.MethodDelete, "/api/trust-metrics", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("trust-metrics DELETE status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}

func TestOldRoutesUnchanged(t *testing.T) {
	server := newTrustCenterTestServer(t)

	overview := httptest.NewRecorder()
	server.Handler().ServeHTTP(overview, newLocalRequest(http.MethodGet, "/api/overview", nil))
	if overview.Code != http.StatusOK {
		t.Fatalf("overview status = %d body=%s", overview.Code, overview.Body.String())
	}
	var overviewPayload map[string]any
	if err := json.Unmarshal(overview.Body.Bytes(), &overviewPayload); err != nil {
		t.Fatalf("overview json invalid: %v", err)
	}
	if overviewPayload["command"] != "dashboard.overview" {
		t.Fatalf("overview command changed: %#v", overviewPayload["command"])
	}

	notes := httptest.NewRecorder()
	server.Handler().ServeHTTP(notes, newLocalRequest(http.MethodGet, "/api/notes", nil))
	if notes.Code != http.StatusOK {
		t.Fatalf("notes status = %d body=%s", notes.Code, notes.Body.String())
	}
	var notesPayload map[string]any
	if err := json.Unmarshal(notes.Body.Bytes(), &notesPayload); err != nil {
		t.Fatalf("notes json invalid: %v", err)
	}
	if notesPayload["command"] != "dashboard.notes" {
		t.Fatalf("notes command changed: %#v", notesPayload["command"])
	}
}

func TestTrustCenter_NoBodyLeak(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "notes", "secret.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_secret\ntitle: Secret\nkind: task\nstatus: active\n---\n\nSECRET_BODY_SENTINEL must not leak into trust center\n")
	writeDashboardFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"type":"provider","token":"secret-token-xyz","authorization":"Bearer secret"}`+"\n")

	server := NewServer(svc, root)
	for _, endpoint := range []string{"/api/agent-continuity", "/api/memory-inbox", "/api/trust-metrics"} {
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, newLocalRequest(http.MethodGet, endpoint, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", endpoint, res.Code, res.Body.String())
		}
		body := strings.ToLower(res.Body.String())
		for _, forbidden := range []string{"secret_body_sentinel", "secret-token-xyz", "bearer secret"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s leaked forbidden material %q:\n%s", endpoint, forbidden, res.Body.String())
			}
		}
	}
}
