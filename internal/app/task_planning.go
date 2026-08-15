package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/templateengine"
)

const dailyTaskReviewBlockName = "daily-task-review"

type dailyTaskReviewSummary struct {
	Date    string             `json:"date"`
	Today   []domain.BoardItem `json:"today"`
	Overdue []domain.BoardItem `json:"overdue"`
	Blocked []domain.BoardItem `json:"blocked"`
	Review  []domain.BoardItem `json:"review"`
}

func (s *Service) planDailyTaskReview(_ context.Context, root string, capturedAt time.Time, snapshot domain.PlanningSnapshot, decision domain.PlanningDecision, yes bool) (domain.Projection, error) {
	date := capturedAt.UTC().Format("2006-01-02")
	targetRel := filepath.ToSlash(filepath.Join("daily", date+".md"))
	summary, err := buildDailyTaskReviewSummary(root, date)
	if err != nil {
		return errorProjection("plan.daily", err), err
	}
	body := renderDailyTaskReviewMarkdown(summary, capturedAt)
	projection := domain.NewProjection("plan.daily", "Daily task review plan generated.")
	projection.Facts["period"] = "daily"
	projection.Facts["managed_block"] = dailyTaskReviewBlockName
	projection.Facts["target_note"] = targetRel
	projection.Facts["writes"] = "false"
	projection.Facts["today"] = fmt.Sprint(len(summary.Today))
	projection.Facts["overdue"] = fmt.Sprint(len(summary.Overdue))
	projection.Facts["blocked"] = fmt.Sprint(len(summary.Blocked))
	projection.Facts["review"] = fmt.Sprint(len(summary.Review))
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["decision_id"] = decision.DecisionID
	projection.Data = map[string]any{"snapshot": snapshot, "decision": decision, "task_review": summary, "body": body}
	projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax plan daily --task-review --vault %s --yes --json", shellQuote(root))}}
	path, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("plan.daily", err), err
	}
	contentBytes, err := osReadFile(path)
	if err != nil {
		missing := &domain.CommandError{Code: "managed_block_missing", Message: "daily-task-review managed block is missing", Hint: fmt.Sprintf("pinax journal daily show --date %s --template journal.daily --vault %s --json", date, shellQuote(root))}
		projection := domain.NewErrorProjection("plan.daily", missing)
		projection.Facts["period"] = "daily"
		projection.Facts["managed_block"] = dailyTaskReviewBlockName
		projection.Facts["target_note"] = targetRel
		projection.Facts["writes"] = "false"
		projection.Data = map[string]any{"snapshot": snapshot, "decision": decision, "task_review": summary, "body": body}
		projection.Actions = []domain.Action{{Name: "create_daily", Command: missing.Hint}}
		return projection, missing
	}
	content := string(contentBytes)
	updated, err := templateengine.ReplaceManagedBlock(content, dailyTaskReviewBlockName, body)
	if err != nil {
		if templateengine.ErrorCode(err) == "managed_block_missing" {
			missing := &domain.CommandError{Code: "managed_block_missing", Message: "daily-task-review managed block is missing", Hint: fmt.Sprintf("Add <!-- pinax:managed name=%s --> to %s or recreate it with pinax journal daily show --date %s --template journal.daily --vault %s --json", dailyTaskReviewBlockName, targetRel, date, shellQuote(root))}
			projection := domain.NewErrorProjection("plan.daily", missing)
			projection.Facts["period"] = "daily"
			projection.Facts["managed_block"] = dailyTaskReviewBlockName
			projection.Facts["target_note"] = targetRel
			projection.Facts["writes"] = "false"
			projection.Facts["today"] = fmt.Sprint(len(summary.Today))
			projection.Facts["overdue"] = fmt.Sprint(len(summary.Overdue))
			projection.Facts["blocked"] = fmt.Sprint(len(summary.Blocked))
			projection.Facts["review"] = fmt.Sprint(len(summary.Review))
			projection.Data = map[string]any{"snapshot": snapshot, "decision": decision, "task_review": summary, "body": body}
			projection.Actions = []domain.Action{{Name: "add_marker", Command: missing.Hint}}
			return projection, missing
		}
		return errorProjection("plan.daily", planningBlockConflict()), planningBlockConflict()
	}
	if !yes {
		return projection, nil
	}
	if err := osWriteFile(path, []byte(updated), 0o644); err != nil {
		return errorProjection("plan.daily", err), err
	}
	appendEventWarned(root, "plan.daily", "success", map[string]string{"managed_block": dailyTaskReviewBlockName, "target_note": targetRel})
	projection.Summary = "Daily task review updated."
	projection.Facts["writes"] = "true"
	projection.Evidence = []string{targetRel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Actions = []domain.Action{{Name: "open", Command: fmt.Sprintf("pinax journal daily open --date %s --vault %s", date, shellQuote(root))}}
	return projection, nil
}

func buildDailyTaskReviewSummary(root, date string) (dailyTaskReviewSummary, error) {
	summary := dailyTaskReviewSummary{Date: date}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return summary, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return summary, err
	}
	ordinary := ordinaryNotes(notes)
	for _, project := range registry.Projects {
		columns, err := loadProjectBoardColumns(root, project.Slug, "")
		if err != nil {
			return summary, err
		}
		board := buildProjectBoard(root, project, nil, columns, ordinary, domain.NoteDisplayCard, "scan", "unknown", false)
		for _, item := range board.Items {
			appendDailyTaskReviewItem(&summary, item, date)
		}
	}
	sortDailyTaskReviewItems(&summary)
	return summary, nil
}

func appendDailyTaskReviewItem(summary *dailyTaskReviewSummary, item domain.BoardItem, date string) {
	dueDate := strings.TrimSpace(firstBoardNonEmpty(item.DueAt, item.Due))
	if dueDate != "" {
		day := dueDate
		if len(day) > len("2006-01-02") {
			day = day[:len("2006-01-02")]
		}
		if day == date {
			summary.Today = append(summary.Today, item)
		} else if day < date {
			summary.Overdue = append(summary.Overdue, item)
		}
	}
	if item.Column == "blocked" || len(item.BlockedBy) > 0 {
		summary.Blocked = append(summary.Blocked, item)
	}
	if item.Column == "review" {
		summary.Review = append(summary.Review, item)
	}
}

func sortDailyTaskReviewItems(summary *dailyTaskReviewSummary) {
	sortItems := func(items []domain.BoardItem) {
		sort.Slice(items, func(i, j int) bool {
			if items[i].Project != items[j].Project {
				return items[i].Project < items[j].Project
			}
			if items[i].Path != items[j].Path {
				return items[i].Path < items[j].Path
			}
			return items[i].Title < items[j].Title
		})
	}
	sortItems(summary.Today)
	sortItems(summary.Overdue)
	sortItems(summary.Blocked)
	sortItems(summary.Review)
}

func renderDailyTaskReviewMarkdown(summary dailyTaskReviewSummary, capturedAt time.Time) string {
	var b strings.Builder
	b.WriteString("## Daily Task Review\n\n")
	b.WriteString("Captured at: ")
	b.WriteString(capturedAt.UTC().Format(time.RFC3339))
	b.WriteString("\n\n")
	writeTaskReviewSection(&b, "Today", summary.Today)
	writeTaskReviewSection(&b, "Overdue", summary.Overdue)
	writeTaskReviewSection(&b, "Blocked", summary.Blocked)
	writeTaskReviewSection(&b, "Review", summary.Review)
	return strings.TrimSpace(b.String())
}

func writeTaskReviewSection(b *strings.Builder, title string, items []domain.BoardItem) {
	b.WriteString("### ")
	b.WriteString(title)
	b.WriteString("\n")
	if len(items) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for _, item := range items {
		b.WriteString("- [ ] ")
		b.WriteString(markdownInline(item.Title))
		facts := []string{}
		if item.Project != "" {
			facts = append(facts, "project: "+markdownInline(item.Project))
		}
		if item.Column != "" {
			facts = append(facts, "column: "+markdownInline(item.Column))
		}
		if due := firstBoardNonEmpty(item.DueAt, item.Due); due != "" {
			facts = append(facts, "due: "+markdownInline(due))
		}
		if len(item.BlockedBy) > 0 {
			facts = append(facts, "blocked_by: "+markdownInline(strings.Join(item.BlockedBy, ",")))
		}
		if item.ItemID != "" {
			facts = append(facts, "id: "+markdownInline(item.ItemID))
		}
		if len(facts) > 0 {
			b.WriteString(" _(")
			b.WriteString(strings.Join(facts, ", "))
			b.WriteString(")_")
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
}

var osReadFile = os.ReadFile
var osWriteFile = os.WriteFile

func cleanOneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func markdownInline(value string) string {
	value = cleanOneLine(value)
	return strings.ReplaceAll(value, "`", "'")
}

func planningBlockConflict() error {
	return &domain.CommandError{Code: "PLANNING_BLOCK_CONFLICT", Message: "Daily planning managed block is invalid", Hint: "Open the daily note and keep exactly one closed daily-task-review managed block"}
}
