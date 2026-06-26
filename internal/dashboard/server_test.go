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

func TestReadonlyDashboardServesLinkGraphSummary(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "Source.md"), "# Source\n\n[[Missing Target]]\n\n[[Shared]]\n")
	writeDashboardFixture(t, filepath.Join(root, "First.md"), "# Shared\n\nfirst\n")
	writeDashboardFixture(t, filepath.Join(root, "Second.md"), "# Shared\n\nsecond\n")
	writeDashboardFixture(t, filepath.Join(root, "Orphan.md"), "# Orphan\n\nsolo\n")

	server := NewServer(svc, root)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/graph-summary", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("graph summary status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("graph summary json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["command"] != "dashboard.graph_summary" || payload["status"] != "partial" {
		t.Fatalf("graph summary payload = %#v", payload)
	}
	facts := payload["facts"].(map[string]any)
	for _, key := range []string{"broken", "ambiguous", "orphans", "engine"} {
		if facts[key] == nil || facts[key] == "0" {
			t.Fatalf("graph summary fact %s missing: %#v", key, facts)
		}
	}
	data := payload["data"].(map[string]any)
	if data["broken"].(float64) < 1 || data["ambiguous"].(float64) < 1 || data["orphans"].(float64) < 1 {
		t.Fatalf("graph summary counts invalid: %#v", data)
	}
	if len(payload["actions"].([]any)) == 0 || !strings.Contains(res.Body.String(), "pinax ") {
		t.Fatalf("graph summary next action missing: %#v", payload)
	}

	overview := httptest.NewRecorder()
	server.Handler().ServeHTTP(overview, httptest.NewRequest(http.MethodGet, "/api/overview", nil))
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), "link_graph") || !strings.Contains(overview.Body.String(), "dashboard.graph_summary") {
		t.Fatalf("overview missing link graph summary: status=%d body=%s", overview.Code, overview.Body.String())
	}

	index := httptest.NewRecorder()
	server.Handler().ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, want := range []string{"关系", "断链", "歧义", "孤立", "pinax "} {
		if !strings.Contains(index.Body.String(), want) {
			t.Fatalf("dashboard index missing %q:\n%s", want, index.Body.String())
		}
	}

	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/graph-summary", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("graph summary write-like method status = %d", res.Code)
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

func TestReadonlyDashboardServesProjectBoard(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "research", "task.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_task\ntitle: Board Task\nproject: research\nkind: task\nstatus: active\n---\n\nsecret body should not leak as body field\n")

	server := NewServer(svc, root)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/project-board/research", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("board status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("board json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["command"] != "dashboard.project_board" || !strings.Contains(res.Body.String(), `"project":"research"`) || strings.Contains(res.Body.String(), `"body"`) {
		t.Fatalf("board payload = %s", res.Body.String())
	}
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/project-board/research", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("board write-like method status = %d", res.Code)
	}
}

func TestReadonlyDashboardServesBoundedNoteDisplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "notes", "secret.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_secret\ntitle: Secret\nkind: task\nstatus: active\n---\n\nsecret body should stay out of dashboard card\n")
	server := NewServer(svc, root)

	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/note-display/note_secret?display=detail", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("note display status = %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"command":"dashboard.note_display"`) || !strings.Contains(res.Body.String(), `"display":"detail"`) || strings.Contains(res.Body.String(), `"body"`) {
		t.Fatalf("note display leaked body or missed fields: %s", res.Body.String())
	}

	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/note-display/note_secret", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write-like note display status = %d", res.Code)
	}
}

func TestReadonlyDashboardServesDatabaseTabProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeDashboardFixture(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\nstatus: active\nkind: reference\n---\n\nbody\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if _, err := svc.SaveDatabaseView(ctx, app.ViewRequest{VaultPath: root, Name: "active-tab", Display: "list", Query: `SELECT title, status FROM notes WHERE status = "active" LIMIT 10`}); err != nil {
		t.Fatalf("save database view: %v", err)
	}
	server := NewServer(svc, root)

	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/database-tabs/active-tab", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("database tab status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("database tab json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["command"] != "dashboard.database_tab" || !strings.Contains(res.Body.String(), `"database_tab"`) || !strings.Contains(res.Body.String(), `"database_view"`) || !strings.Contains(res.Body.String(), `"database.display":"list"`) {
		t.Fatalf("database tab payload = %s", res.Body.String())
	}

	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/database-tabs/active-tab", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("database tab write-like method status = %d", res.Code)
	}
}

func writeDashboardFixture(t *testing.T, path, content string) {
	t.Helper()
	if strings.EqualFold(filepath.Ext(path), ".md") && !strings.HasPrefix(content, "---\n") {
		title := dashboardFixtureTitle(path, content)
		id := "note_" + strings.NewReplacer("/", "_", "\\", "_", ".", "_", " ", "_").Replace(strings.ToLower(filepath.Base(path)))
		content = "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: " + title + "\ntags: []\n---\n\n" + content
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func dashboardFixtureTitle(path, content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
