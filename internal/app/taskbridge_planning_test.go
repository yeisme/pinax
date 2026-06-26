package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestTaskBridgePlanDailyDryRunDoesNotWrite(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridge(t, fakeTaskBridgeTodayPayload())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, DryRun: true})
	if err != nil {
		t.Fatalf("plan daily taskbridge dry-run: %v", err)
	}

	if projection.Facts["source"] != "taskbridge" || projection.Facts["captured_at"] != "2026-06-21T15:30:00Z" {
		t.Fatalf("taskbridge facts = %#v", projection.Facts)
	}
	if projection.Facts["target_note"] != "daily/2026-06-21.md" || projection.Facts["selected_commitments"] != "3" {
		t.Fatalf("daily plan facts = %#v", projection.Facts)
	}
	if fileExistsApp(filepath.Join(root, "daily", "2026-06-21.md")) {
		t.Fatalf("dry-run created daily note")
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "planning")) {
		t.Fatalf("dry-run created planning assets")
	}
	data := projection.Data.(map[string]any)
	snapshot := data["snapshot"].(domain.PlanningSnapshot)
	if snapshot.Source != "taskbridge" || snapshot.TaskBridge == nil || len(snapshot.TaskBridge.Tasks) != 4 {
		t.Fatalf("snapshot taskbridge data = %#v", snapshot)
	}
}

func TestTaskBridgePlanDailyYesWritesPlanningDailyBlock(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridge(t, fakeTaskBridgeTodayPayload())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, Yes: true, Save: true})
	if err != nil {
		t.Fatalf("plan daily taskbridge apply: %v", err)
	}

	if projection.Facts["managed_block"] != "planning-daily" || projection.Facts["saved_path"] == "" {
		t.Fatalf("apply facts = %#v", projection.Facts)
	}
	body := readFile(t, filepath.Join(root, "daily", "2026-06-21.md"))
	for _, want := range []string{"<!-- pinax:managed name=planning-daily -->", "## TaskBridge Daily Todo", "Captured at: 2026-06-21T15:30:00Z", "- [ ] Finish control-plane slice", "source: local", "task_1", "<!-- pinax:managed name=daily-captures -->"} {
		if !strings.Contains(body, want) {
			t.Fatalf("daily body missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(root, projection.Facts["saved_path"])), `"source": "taskbridge"`) {
		t.Fatalf("saved snapshot does not record taskbridge source")
	}
}

func TestTaskBridgePlanDailyAppendsBlockToExistingDailyNote(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridge(t, fakeTaskBridgeTodayPayload())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "daily", "2026-06-21.md"), "# 2026-06-21\n\nuser notes\n")

	if _, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, Yes: true}); err != nil {
		t.Fatalf("plan daily taskbridge append: %v", err)
	}

	body := readFile(t, filepath.Join(root, "daily", "2026-06-21.md"))
	if !strings.Contains(body, "user notes") || !strings.Contains(body, "<!-- pinax:managed name=planning-daily -->") {
		t.Fatalf("existing daily note not preserved/appended:\n%s", body)
	}
}

func TestTaskBridgePlanDailyDuplicatePlanningBlockFailsClosed(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridge(t, fakeTaskBridgeTodayPayload())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	bad := strings.Join([]string{
		"# 2026-06-21",
		"<!-- pinax:managed name=planning-daily -->",
		"old",
		"<!-- /pinax:managed -->",
		"<!-- pinax:managed name=planning-daily -->",
		"duplicate",
		"<!-- /pinax:managed -->",
	}, "\n")
	writeAppFixture(t, filepath.Join(root, "daily", "2026-06-21.md"), bad)

	_, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, Yes: true})
	if !hasCommandCode(err, "PLANNING_BLOCK_CONFLICT") {
		t.Fatalf("expected PLANNING_BLOCK_CONFLICT, got %v", err)
	}
	if got := readFile(t, filepath.Join(root, "daily", "2026-06-21.md")); got != bad {
		t.Fatalf("conflict changed daily note:\n%s", got)
	}
}

func TestTaskBridgePlanDailyUnavailableIsReadOnly(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	t.Setenv("PATH", t.TempDir())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	_, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, WithTaskBridge: true, Yes: true})
	if !hasCommandCode(err, "TASKBRIDGE_UNAVAILABLE") {
		t.Fatalf("expected TASKBRIDGE_UNAVAILABLE, got %v", err)
	}
	if fileExistsApp(filepath.Join(root, "daily", "2026-06-21.md")) || fileExistsApp(filepath.Join(root, ".pinax", "planning")) {
		t.Fatalf("unavailable taskbridge wrote files")
	}
}

func TestPlanDailyTaskReviewRequiresManagedBlockAndYes(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "Due today", Column: "next", DueAt: "2026-06-21"}); err != nil {
		t.Fatalf("add due item: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "Overdue", Column: "next", DueAt: "2026-06-20"}); err != nil {
		t.Fatalf("add overdue item: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "Blocked task", Column: "blocked", BlockedBy: []string{"api"}}); err != nil {
		t.Fatalf("add blocked item: %v", err)
	}
	if _, err := svc.ProjectItemAdd(ctx, ProjectItemRequest{VaultPath: root, Project: "research", Title: "Review task", Column: "review"}); err != nil {
		t.Fatalf("add review item: %v", err)
	}
	dailyPath := filepath.Join(root, "daily", "2026-06-21.md")
	writeAppFixture(t, dailyPath, "# 2026-06-21\n\nUser notes stay.\n")
	missingBefore := readFile(t, dailyPath)

	missing, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, TaskReview: true, Yes: true})
	if !hasCommandCode(err, "managed_block_missing") {
		t.Fatalf("expected managed_block_missing, got %v", err)
	}
	if missing.Facts["managed_block"] != "daily-task-review" || len(missing.Actions) == 0 {
		t.Fatalf("missing block projection = %#v actions=%#v", missing.Facts, missing.Actions)
	}
	if got := readFile(t, dailyPath); got != missingBefore {
		t.Fatalf("missing block changed daily note:\n%s", got)
	}

	withBlock := strings.Join([]string{
		"# 2026-06-21",
		"",
		"User notes stay.",
		"",
		"<!-- pinax:managed name=daily-task-review -->",
		"old review",
		"<!-- /pinax:managed -->",
		"",
		"Manual footer stays.",
		"",
	}, "\n")
	writeAppFixture(t, dailyPath, withBlock)
	preview, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, TaskReview: true})
	if err != nil {
		t.Fatalf("task review preview: %v", err)
	}
	if preview.Facts["writes"] != "false" || preview.Facts["managed_block"] != "daily-task-review" {
		t.Fatalf("preview facts = %#v", preview.Facts)
	}
	if got := readFile(t, dailyPath); got != withBlock {
		t.Fatalf("preview changed daily note:\n%s", got)
	}

	applied, err := svc.PlanDaily(ctx, PlanningRequest{VaultPath: root, TaskReview: true, Yes: true})
	if err != nil {
		t.Fatalf("task review apply: %v", err)
	}
	if applied.Facts["writes"] != "true" || applied.Facts["today"] != "1" || applied.Facts["overdue"] != "1" || applied.Facts["blocked"] != "1" || applied.Facts["review"] != "1" {
		t.Fatalf("apply facts = %#v", applied.Facts)
	}
	body := readFile(t, dailyPath)
	for _, want := range []string{"User notes stay.", "Manual footer stays.", "## Daily Task Review", "Due today", "Overdue", "Blocked task", "Review task"} {
		if !strings.Contains(body, want) {
			t.Fatalf("daily body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "old review") {
		t.Fatalf("managed block was not replaced:\n%s", body)
	}
}

func TestTaskBridgePlanActionsSaveUsesDeferredCandidates(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridge(t, fakeTaskBridgeTodayPayload())
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanActions(ctx, PlanningRequest{VaultPath: root, FromPeriod: "daily", WithTaskBridge: true, Save: true})
	if err != nil {
		t.Fatalf("plan actions taskbridge save: %v", err)
	}

	if projection.Facts["source"] != "taskbridge" || projection.Facts["tasks"] != "1" || projection.Facts["saved_path"] == "" {
		t.Fatalf("taskbridge action facts = %#v", projection.Facts)
	}
	data := projection.Data.(map[string]any)
	draft := data["draft"].(domain.PlanningActionDraft)
	if !draft.RequiresConfirmation || len(draft.Tasks) != 1 || draft.Tasks[0].TaskID != "task_4" || draft.Tasks[0].Kind != "defer" {
		t.Fatalf("taskbridge action draft = %#v", draft)
	}
	if len(projection.Actions) != 1 || !strings.Contains(projection.Actions[0].Command, "--dry-run") || strings.Contains(projection.Actions[0].Command, "--confirm") {
		t.Fatalf("action next step must be dry-run only: %#v", projection.Actions)
	}
	if !strings.Contains(readFile(t, filepath.Join(root, projection.Facts["saved_path"])), `"task_id": "task_4"`) {
		t.Fatalf("saved action draft missing task_4")
	}
}

func installFakeTaskBridge(t *testing.T, payload string) {
	t.Helper()
	binDir := t.TempDir()
	path := filepath.Join(binDir, "taskbridge")
	script := "#!/bin/sh\nif [ \"$1 $2\" != \"agent today\" ]; then echo unexpected args: \"$@\" >&2; exit 2; fi\ncat <<'JSON'\n" + payload + "\nJSON\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake taskbridge: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func fakeTaskBridgeTodayPayload() string {
	return `{
  "schema": "taskbridge.agent-result.v1",
  "status": "ok",
  "request_id": "req_test",
  "dry_run": false,
  "requires_confirmation": false,
  "result": {
    "schema": "taskbridge.today.v1",
    "date": "2026-06-21",
    "status": "ok",
    "summary": {"must_do": 2, "at_risk": 1, "inbox": 0, "overdue": 1, "project_next": 0, "sync_warnings": 0},
    "sections": [
      {"id": "must_do", "title": "Must do today", "tasks": [
        {"id": "task_1", "title": "Finish control-plane slice", "status": "todo", "source": "local", "priority": "high", "reason": "Due today"},
        {"id": "task_2", "title": "Review overdue items", "status": "todo", "source": "todoist", "priority": 2, "reason": "Overdue"}
      ]},
      {"id": "next", "title": "Suggested next steps", "tasks": [
        {"id": "task_1", "title": "Finish control-plane slice", "status": "todo", "source": "local", "priority": "high", "reason": "Best task"},
        {"id": "task_3", "title": "Draft release note", "status": "todo", "source": "microsoft", "priority": "medium", "reason": "Best task"}
      ]},
      {"id": "at_risk", "title": "At risk", "tasks": [
        {"id": "task_4", "title": "Split large task", "status": "todo", "source": "local", "priority": "low", "reason": "Too large"}
      ]}
    ],
    "suggested_actions": [
      {"id": "act_1", "type": "defer_task", "task_id": "task_4", "reason": "Too large for today", "requires_confirmation": true}
    ],
    "warnings": []
  },
  "warnings": [],
  "errors": []
}`
}
