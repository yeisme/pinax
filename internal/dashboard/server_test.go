package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestReadonlyDashboardServesStatsDoctorAndRedacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\ntags: [pinax]\n---\n\n# Active\n")
	writeDashboardFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"type":"provider","token":"secret-token","authorization":"Bearer secret"}`+"\n")

	server := NewServer(svc, root)
	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("overview status = %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(strings.ToLower(body), "secret-token") || strings.Contains(strings.ToLower(body), "bearer secret") {
		t.Fatalf("dashboard leaked secret material:\n%s", body)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("overview json invalid: %v\n%s", err, body)
	}
	if payload["spec_version"] != "1.0" || payload["command"] != "dashboard.overview" {
		t.Fatalf("overview payload = %#v", payload)
	}

	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/overview", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write-like method status = %d", res.Code)
	}
}

func TestReadonlyDashboardServesRepairPlans(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "No Tags.md"), "# No Tags\n\nbody\n")
	if _, err := svc.PlanRepair(ctx, app.RepairPlanRequest{VaultPath: root, Save: true}); err != nil {
		t.Fatalf("save repair plan: %v", err)
	}

	server := NewServer(svc, root)
	indexRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(indexRes, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRes.Code != http.StatusOK || !strings.Contains(indexRes.Body.String(), "Repair plans") || !strings.Contains(indexRes.Body.String(), "pinax repair apply") {
		t.Fatalf("repair index missing plan summary: status=%d body=%s", indexRes.Code, indexRes.Body.String())
	}
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/repair-plans", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("repair plans status = %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(strings.ToLower(body), "authorization") || strings.Contains(strings.ToLower(body), "secret-token") || strings.Contains(strings.ToLower(body), "bearer ") {
		t.Fatalf("repair plans leaked sensitive material:\n%s", body)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("repair plans json invalid: %v\n%s", err, body)
	}
	if payload["command"] != "dashboard.repair_plans" || payload["status"] != "success" {
		t.Fatalf("repair plans payload = %#v", payload)
	}
	data := payload["data"].(map[string]any)
	if len(data["plans"].([]any)) == 0 || data["apply_command"] == "" {
		t.Fatalf("repair plans data missing summary: %#v", data)
	}

	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/repair-plans", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("repair plans write-like method status = %d", res.Code)
	}
}

func writeDashboardFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
