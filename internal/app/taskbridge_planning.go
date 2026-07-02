package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/templateengine"
)

const planningDailyBlockName = "planning-daily"
const dailyTaskReviewBlockName = "daily-task-review"

type dailyTaskReviewSummary struct {
	Date    string             `json:"date"`
	Today   []domain.BoardItem `json:"today"`
	Overdue []domain.BoardItem `json:"overdue"`
	Blocked []domain.BoardItem `json:"blocked"`
	Review  []domain.BoardItem `json:"review"`
}

type taskBridgeAgentToday struct {
	Schema string                `json:"schema"`
	Status string                `json:"status"`
	Result taskBridgeTodayResult `json:"result"`
}

type taskBridgeTodayResult struct {
	Schema           string                      `json:"schema"`
	Date             string                      `json:"date"`
	Status           string                      `json:"status"`
	Summary          map[string]json.RawMessage  `json:"summary"`
	Sections         []taskBridgeTodaySection    `json:"sections"`
	SuggestedActions []taskBridgeSuggestedAction `json:"suggested_actions"`
	Warnings         []string                    `json:"warnings"`
}

type taskBridgeTodaySection struct {
	ID    string                `json:"id"`
	Title string                `json:"title"`
	Tasks []taskBridgeTodayTask `json:"tasks"`
}

type taskBridgeTodayTask struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Status   string          `json:"status"`
	Source   string          `json:"source"`
	Priority json.RawMessage `json:"priority"`
	Reason   string          `json:"reason"`
}

type taskBridgeSuggestedAction struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	TaskID               string `json:"task_id"`
	Reason               string `json:"reason"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

func loadTaskBridgeDaily(ctx context.Context, capturedAt time.Time) (*domain.TaskBridgePlan, error) {
	cmd := exec.CommandContext(ctx, "taskbridge", "agent", "today")
	stdout, err := cmd.Output()
	if err != nil {
		return nil, &domain.CommandError{Code: "TASKBRIDGE_UNAVAILABLE", Message: "TaskBridge daily facts are unavailable", Hint: "Run taskbridge agent today to verify TaskBridge is installed and configured"}
	}
	var envelope taskBridgeAgentToday
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		return nil, &domain.CommandError{Code: "TASKBRIDGE_UNAVAILABLE", Message: "TaskBridge returned invalid JSON", Hint: "Run taskbridge agent today and inspect stdout"}
	}
	if envelope.Schema != "taskbridge.agent-result.v1" || envelope.Status != "ok" || envelope.Result.Schema != "taskbridge.today.v1" || envelope.Result.Status != "ok" {
		return nil, &domain.CommandError{Code: "TASKBRIDGE_CONTRACT_UNSUPPORTED", Message: "TaskBridge today schema is unsupported", Hint: "Upgrade TaskBridge or run taskbridge agent schemas"}
	}
	plan := &domain.TaskBridgePlan{
		SchemaVersion: "taskbridge.today.v1",
		CapturedAt:    capturedAt.UTC().Format(time.RFC3339),
		Date:          strings.TrimSpace(envelope.Result.Date),
		Status:        envelope.Result.Status,
		Summary:       normalizeTaskBridgeSummary(envelope.Result.Summary),
		Tasks:         normalizeTaskBridgeTasks(envelope.Result.Sections),
		Actions:       normalizeTaskBridgeActions(envelope.Result.SuggestedActions),
		Warnings:      cleanStringList(envelope.Result.Warnings),
	}
	return plan, nil
}

func applyTaskBridgePlanning(snapshot *domain.PlanningSnapshot, decision *domain.PlanningDecision, taskBridge *domain.TaskBridgePlan, maxCommitments int) {
	snapshot.Source = "taskbridge"
	snapshot.CapturedAt = taskBridge.CapturedAt
	snapshot.TaskBridge = taskBridge
	snapshot.Facts["source"] = "taskbridge"
	snapshot.Facts["captured_at"] = taskBridge.CapturedAt
	snapshot.Facts["taskbridge_tasks"] = fmt.Sprint(len(taskBridge.Tasks))
	selected := selectTaskBridgeCommitments(taskBridge.Tasks, maxCommitments)
	decision.Selected = selected
	decision.Reasons = append(decision.Reasons, domain.PlanningReason{Kind: "taskbridge", Summary: fmt.Sprintf("selected %d TaskBridge commitments for today's plan", len(selected))})
	for _, action := range taskBridge.Actions {
		if action.Type == "defer_task" && strings.TrimSpace(action.TaskID) != "" {
			decision.Deferred = append(decision.Deferred, action.TaskID)
		}
	}
	decision.NextActions = append(decision.NextActions, domain.Action{Name: "actions", Command: "pinax plan actions --from daily --save --vault <vault>"})
}

func selectTaskBridgeCommitments(tasks []domain.TaskBridgePlanTask, limit int) []string {
	if limit <= 0 {
		return []string{}
	}
	bySection := map[string][]domain.TaskBridgePlanTask{}
	for _, task := range tasks {
		bySection[task.SectionID] = append(bySection[task.SectionID], task)
	}
	selected := []string{}
	seen := map[string]bool{}
	for _, section := range []string{"must_do", "next", "at_risk"} {
		for _, task := range bySection[section] {
			id := strings.TrimSpace(task.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			selected = append(selected, id)
			if len(selected) >= limit {
				return selected
			}
		}
	}
	return selected
}

func renderTaskBridgeDailyMarkdown(snapshot domain.PlanningSnapshot, decision domain.PlanningDecision) string {
	if snapshot.TaskBridge == nil {
		return "## TaskBridge Daily Todo\n\nCaptured at: " + snapshot.CapturedAt + "\n\nNo TaskBridge tasks captured."
	}
	selected := map[string]bool{}
	for _, id := range decision.Selected {
		selected[id] = true
	}
	var b strings.Builder
	b.WriteString("## TaskBridge Daily Todo\n\n")
	b.WriteString("Captured at: ")
	b.WriteString(snapshot.TaskBridge.CapturedAt)
	b.WriteString("\n\n")
	if len(selected) == 0 {
		b.WriteString("No TaskBridge commitments selected.\n")
		return strings.TrimSpace(b.String())
	}
	for _, task := range snapshot.TaskBridge.Tasks {
		if !selected[task.ID] {
			continue
		}
		b.WriteString("- [ ] ")
		b.WriteString(markdownInline(task.Title))
		facts := []string{}
		if task.Source != "" {
			facts = append(facts, "source: "+markdownInline(task.Source))
		}
		if task.ID != "" {
			facts = append(facts, "id: "+markdownInline(task.ID))
		}
		if task.Priority != "" {
			facts = append(facts, "priority: "+markdownInline(task.Priority))
		}
		if len(facts) > 0 {
			b.WriteString(" _(")
			b.WriteString(strings.Join(facts, ", "))
			b.WriteString(")_")
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nNext action: `pinax plan actions --from daily --save --vault <vault>`")
	return strings.TrimSpace(b.String())
}

func writeDailyPlanningBlock(root string, capturedAt time.Time, body string) (string, error) {
	date := capturedAt.UTC().Format("2006-01-02")
	root, rel, _, err := ensureJournalNote(root, "daily", DailyRequest{Date: date})
	if err != nil {
		return "", err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	contentBytes, err := osReadFile(path)
	if err != nil {
		return rel, err
	}
	content := string(contentBytes)
	blocks, err := templateengine.InspectManagedBlocks(content)
	if err != nil {
		return rel, planningBlockConflict(err)
	}
	found := false
	for _, block := range blocks {
		if block.Name == planningDailyBlockName {
			found = true
			break
		}
	}
	var updated string
	if found {
		updated, err = templateengine.ReplaceManagedBlock(content, planningDailyBlockName, body)
		if err != nil {
			return rel, planningBlockConflict(err)
		}
	} else {
		updated = strings.TrimRight(content, "\n") + "\n\n" + managedBlock(planningDailyBlockName, body) + "\n"
	}
	return rel, osWriteFile(path, []byte(updated), 0o644)
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
		return errorProjection("plan.daily", planningBlockConflict(err)), planningBlockConflict(err)
	}
	if !yes {
		return projection, nil
	}
	if err := osWriteFile(path, []byte(updated), 0o644); err != nil {
		return errorProjection("plan.daily", err), err
	}
	_ = appendEvent(root, "plan.daily", "success", map[string]string{"managed_block": dailyTaskReviewBlockName, "target_note": targetRel})
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

func planningBlockConflict(err error) error {
	return &domain.CommandError{Code: "PLANNING_BLOCK_CONFLICT", Message: "Daily planning managed block is invalid", Hint: "Open the daily note and keep exactly one closed planning-daily managed block"}
}

func managedBlock(name, body string) string {
	return "<!-- pinax:managed name=" + name + " -->\n" + strings.TrimSpace(body) + "\n<!-- /pinax:managed -->"
}

func normalizeTaskBridgeSummary(raw map[string]json.RawMessage) map[string]int {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := map[string]int{}
	for _, key := range keys {
		var n int
		if err := json.Unmarshal(raw[key], &n); err == nil {
			out[key] = n
		}
	}
	return out
}

func normalizeTaskBridgeTasks(sections []taskBridgeTodaySection) []domain.TaskBridgePlanTask {
	tasks := []domain.TaskBridgePlanTask{}
	seen := map[string]bool{}
	for _, section := range sections {
		for _, task := range section.Tasks {
			id := cleanOneLine(task.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			tasks = append(tasks, domain.TaskBridgePlanTask{ID: id, Title: cleanOneLine(task.Title), Status: cleanOneLine(task.Status), Source: cleanOneLine(task.Source), Priority: rawScalarString(task.Priority), Reason: cleanOneLine(task.Reason), SectionID: cleanOneLine(section.ID), SectionTitle: cleanOneLine(section.Title)})
		}
	}
	return tasks
}

func normalizeTaskBridgeActions(actions []taskBridgeSuggestedAction) []domain.TaskBridgePlanAction {
	out := []domain.TaskBridgePlanAction{}
	for _, action := range actions {
		if strings.TrimSpace(action.ID) == "" && strings.TrimSpace(action.TaskID) == "" {
			continue
		}
		out = append(out, domain.TaskBridgePlanAction{ID: cleanOneLine(action.ID), Type: cleanOneLine(action.Type), TaskID: cleanOneLine(action.TaskID), Reason: cleanOneLine(action.Reason), RequiresConfirmation: action.RequiresConfirmation})
	}
	return out
}

func cleanStringList(values []string) []string {
	out := []string{}
	for _, value := range values {
		if cleaned := cleanOneLine(value); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func rawScalarString(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return cleanOneLine(s)
	}
	return cleanOneLine(string(trimmed))
}

func cleanOneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func markdownInline(value string) string {
	value = cleanOneLine(value)
	value = strings.ReplaceAll(value, "`", "'")
	return value
}
