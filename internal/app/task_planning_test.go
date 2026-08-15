package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanDailyTaskReviewRequiresManagedBlockAndYes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService().WithNowFunc(func() time.Time { return time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC) })
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
