package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNoteTagUnsafeValuesRejectedCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	unsafeNew, err := runCLIExpectError("note", "new", "Unsafe", "--tags", "bad]", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeNew, `"code":"invalid_tag"`) {
		t.Fatalf("note new unsafe tag should fail with invalid_tag: err=%v out=%s", err, unsafeNew)
	}
	if fileExists(filepath.Join(root, "Unsafe.md")) || fileExists(filepath.Join(root, ".pinax", "index.sqlite")) || fileExists(filepath.Join(root, ".pinax", "records", "ledger.jsonl")) {
		t.Fatalf("unsafe note new wrote vault assets")
	}

	safeOut := runCLI(t, "note", "new", "Safe", "--tags", "safe", "--vault", root, "--json")
	var safeEnvelope map[string]any
	if err := json.Unmarshal([]byte(safeOut), &safeEnvelope); err != nil {
		t.Fatalf("safe note json invalid: %v\n%s", err, safeOut)
	}
	path := filepath.Join(root, safeEnvelope["facts"].(map[string]any)["path"].(string))
	before := readCLIFile(t, path)
	unsafeTag, err := runCLIExpectError("note", "tag", "add", "Safe", "bad]", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeTag, `"code":"invalid_tag"`) {
		t.Fatalf("note tag unsafe value should fail with invalid_tag: err=%v out=%s", err, unsafeTag)
	}
	if got := readCLIFile(t, path); got != before {
		t.Fatalf("unsafe note tag changed frontmatter:\n%s", got)
	}
}

func TestNoteTagRecordFactsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Taggable", "--tags", "safe", "--vault", root, "--json")
	var createdEnvelope map[string]any
	if err := json.Unmarshal([]byte(created), &createdEnvelope); err != nil {
		t.Fatalf("created note json invalid: %v\n%s", err, created)
	}
	path := createdEnvelope["facts"].(map[string]any)["path"].(string)

	stdout, stderr, err := runCLISeparate("note", "tag", "add", path, "research", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("note tag json err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("note tag json invalid: %v\n%s", err, stdout)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{"record_event": "note.metadata_updated", "ledger_seq": "2", "record_version": "2", "index_updated": "true"} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; envelope=%#v", key, facts[key], want, envelope)
		}
	}

	agentOut := runCLI(t, "note", "tag", "add", path, "cli", "--vault", root, "--agent")
	for _, want := range []string{"command=note.tag", "fact.record_event=note.metadata_updated", "fact.ledger_seq=3", "fact.index_updated=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestNoteAddCommandRegistersOnlyPinaxNotes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw Markdown\n\nraw-only marker\n")

	noteOut := runCLI(t, "note", "add", "Managed", "--body", "managed marker", "--vault", root, "--json")
	var noteEnvelope map[string]any
	if err := json.Unmarshal([]byte(noteOut), &noteEnvelope); err != nil {
		t.Fatalf("note add json invalid: %v\n%s", err, noteOut)
	}
	if noteEnvelope["command"] != "note.new" || noteEnvelope["status"] != "success" {
		t.Fatalf("note add envelope = %#v", noteEnvelope)
	}

	listOut := runCLI(t, "note", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=note.list", "fact.total=1", "fact.returned=1"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("note list output missing %q:\n%s", want, listOut)
		}
	}
	if strings.Contains(listOut, "raw.md") || strings.Contains(listOut, "Raw Markdown") {
		t.Fatalf("raw markdown leaked into note list:\n%s", listOut)
	}

	searchOut := runCLI(t, "search", "raw-only", "--vault", root, "--agent")
	if !strings.Contains(searchOut, "fact.matches=0") {
		t.Fatalf("raw markdown leaked into search:\n%s", searchOut)
	}
	assertMachineOutputClean(t, listOut)
	assertMachineOutputClean(t, searchOut)
}

func TestNoteShowRenderedAndRefreshCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "B", "--body", "priority:: 2\n", "--status", "done", "--vault", root, "--json")
	dashboardBody := strings.Join([]string{
		"Dashboard intro",
		"```pinax-sql active",
		"SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"```",
		"<!-- pinax:render active start -->",
		"stale",
		"<!-- pinax:render active end -->",
	}, "\n")
	dashboardOut := runCLI(t, "note", "new", "Dashboard", "--body", dashboardBody, "--status", "active", "--vault", root, "--json")
	var dashboardEnvelope map[string]any
	if err := json.Unmarshal([]byte(dashboardOut), &dashboardEnvelope); err != nil {
		t.Fatalf("dashboard create json invalid: %v\n%s", err, dashboardOut)
	}
	path := dashboardEnvelope["facts"].(map[string]any)["path"].(string)

	sourceJSON := runCLI(t, "note", "show", "Dashboard", "--view", "source", "--vault", root, "--json")
	if !strings.Contains(sourceJSON, "```pinax-sql active") || !strings.Contains(sourceJSON, `"query_count":"0"`) {
		t.Fatalf("source json = %s", sourceJSON)
	}
	renderedJSON := runCLI(t, "note", "show", "Dashboard", "--view", "rendered", "--vault", root, "--json")
	if !strings.Contains(renderedJSON, "| A | active |") || strings.Contains(renderedJSON, "| B | done |") || !strings.Contains(renderedJSON, `"view":"rendered"`) {
		t.Fatalf("rendered json = %s", renderedJSON)
	}
	refreshFail, err := runCLIExpectError("note", "refresh", "Dashboard", "--rendered", "--vault", root, "--json")
	if err == nil || !strings.Contains(refreshFail, "approval_required") {
		t.Fatalf("refresh without yes err=%v out=%s", err, refreshFail)
	}
	refreshOut := runCLI(t, "note", "refresh", "Dashboard", "--rendered", "--yes", "--vault", root, "--json")
	if !strings.Contains(refreshOut, `"changed_blocks":"1"`) || !strings.Contains(refreshOut, `"query_count":"1"`) {
		t.Fatalf("refresh out = %s", refreshOut)
	}
	content := readCLIFile(t, filepath.Join(root, path))
	if !strings.Contains(content, "```pinax-sql active") || !strings.Contains(content, "| A | active |") || strings.Contains(content, "stale") {
		t.Fatalf("refreshed content = %s", content)
	}
}

func TestDataviewManagedBlockPreviewAndRefreshCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "priority:: 2", "--status", "active", "--tags", "pinax", "--slug", "active", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "dashboard.md"), strings.Join([]string{
		"---",
		"schema_version: pinax.note.v1",
		"note_id: note_dashboard",
		"title: Dashboard",
		"status: active",
		"---",
		"",
		"# Dashboard",
		"",
		"```pinax-dataview active",
		"TABLE title, status FROM #pinax LIMIT 5",
		"```",
		"",
		"<!-- pinax:managed name=active -->",
		"stale content",
		"<!-- /pinax:managed -->",
		"",
		"user text",
	}, "\n"))
	preview := runCLI(t, "note", "preview", "Dashboard", "--vault", root, "--json")
	if !strings.Contains(preview, "| Active | active |") || strings.Contains(readCLIFile(t, filepath.Join(root, "dashboard.md")), "| Active | active |") {
		t.Fatalf("preview should render read-only:\n%s", preview)
	}
	refresh := runCLI(t, "note", "refresh", "Dashboard", "--rendered", "--yes", "--vault", root, "--json")
	if !strings.Contains(refresh, "note.refresh") || !strings.Contains(refresh, `"changed_blocks":"1"`) {
		t.Fatalf("refresh output = %s", refresh)
	}
	updated := readCLIFile(t, filepath.Join(root, "dashboard.md"))
	if !strings.Contains(updated, "| Active | active |") || strings.Contains(updated, "stale content") || !strings.Contains(updated, "user text") {
		t.Fatalf("managed refresh body =\n%s", updated)
	}
}

func TestNoteRenderedDatabaseViewTabsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "due:: 2026-06-21", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "Done", "--body", "due:: 2026-06-22", "--status", "done", "--vault", root, "--json")
	runCLI(t, "database", "view", "save", "active-list", "--display", "list", "--query", "SELECT title, status FROM notes WHERE status = \"active\" LIMIT 10", "--vault", root, "--json")
	runCLI(t, "database", "view", "save", "calendar-due", "--display", "calendar", "--query", "SELECT title, due FROM notes LIMIT 10", "--calendar-field", "due", "--vault", root, "--json")
	dashboardPath := filepath.Join(root, "dashboard.md")
	writeCLIFixture(t, dashboardPath, strings.Join([]string{
		"---",
		"schema_version: pinax.note.v1",
		"note_id: note_dashboard",
		"title: Dashboard",
		"status: active",
		"---",
		"",
		"# Dashboard",
		"",
		"```pinax-database-view active-list",
		"```",
		"",
		"```pinax-database-view calendar-due",
		"```",
		"",
		"user text",
	}, "\n"))

	rendered := runCLI(t, "note", "show", "Dashboard", "--view", "rendered", "--vault", root, "--json")
	for _, want := range []string{"Active", "2026-06-21", `"database_tabs":`, `"name":"active-list"`, `"name":"calendar-due"`, `"query_count":"2"`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered database tabs missing %q:\n%s", want, rendered)
		}
	}
	if got := readCLIFile(t, dashboardPath); !strings.Contains(got, "```pinax-database-view active-list") || strings.Contains(got, "2026-06-21") {
		t.Fatalf("note show rendered should not write dashboard:\n%s", got)
	}

	writeCLIFixture(t, filepath.Join(root, "missing-dashboard.md"), strings.Join([]string{
		"---",
		"schema_version: pinax.note.v1",
		"note_id: note_missing_dashboard",
		"title: Missing Dashboard",
		"status: active",
		"---",
		"",
		"# Missing Dashboard",
		"",
		"```pinax-database-view missing-view",
		"```",
	}, "\n"))
	failed, err := runCLIExpectError("note", "show", "Missing Dashboard", "--view", "rendered", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "database_tab_view_not_found") || strings.Contains(readCLIFile(t, filepath.Join(root, "missing-dashboard.md")), "Database view") {
		t.Fatalf("missing database tab err=%v output=%s", err, failed)
	}
}

func TestNoteDimensionPrimaryPaths(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Dim", "--tags", "research", "--kind", "reference", "--folder", "work", "--vault", root, "--json")
	humanTags := runCLI(t, "note", "tags", "--vault", root)
	for _, want := range []string{"Tag", "Count", "Share", "Heat", "research", "##########"} {
		if !strings.Contains(humanTags, want) {
			t.Fatalf("note tags summary missing %q:\n%s", want, humanTags)
		}
	}
	for _, tc := range []struct {
		legacy  []string
		primary []string
		command string
	}{
		{legacy: []string{"tag", "list"}, primary: []string{"note", "tags"}, command: "tag.list"},
		{legacy: []string{"kind", "list"}, primary: []string{"note", "kinds"}, command: "kind.list"},
		{legacy: []string{"group", "list"}, primary: []string{"note", "groups"}, command: "group.list"},
	} {
		legacyArgs := append(append([]string{}, tc.legacy...), "--vault", root, "--json")
		primaryArgs := append(append([]string{}, tc.primary...), "--vault", root, "--json")
		legacyOut := runCLI(t, legacyArgs...)
		primaryOut := runCLI(t, primaryArgs...)
		assertSameCommandAndFacts(t, legacyOut, primaryOut, tc.command)
	}
}

func TestDailyInboxWorkflowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	editorLog := filepath.Join(root, "daily-editor.log")
	editor := writeFakeEditor(t, root, editorLog)
	date := time.Now().UTC().Format("2006-01-02")
	dailyRel := filepath.ToSlash(filepath.Join("daily", date+".md"))

	openOut := runCLI(t, "daily", "open", "--editor", editor, "--vault", root, "--json")
	var openEnvelope map[string]any
	if err := json.Unmarshal([]byte(openOut), &openEnvelope); err != nil {
		t.Fatalf("daily open json invalid: %v\n%s", err, openOut)
	}
	openFacts := openEnvelope["facts"].(map[string]any)
	if openEnvelope["command"] != "daily.open" || openFacts["path"] != dailyRel || openFacts["date"] != date || openFacts["editor_executable"] != editor {
		t.Fatalf("daily open envelope = %#v", openEnvelope)
	}
	if !strings.Contains(readCLIFile(t, editorLog), dailyRel) {
		t.Fatalf("daily open did not invoke editor with daily path")
	}

	showOut := runCLI(t, "daily", "show", "--vault", root, "--json")
	if !strings.Contains(showOut, "daily.show") || !strings.Contains(showOut, dailyRel) {
		t.Fatalf("daily show output invalid:\n%s", showOut)
	}
	appendOut := runCLI(t, "daily", "append", "--body", "复盘", "--vault", root, "--json")
	if !strings.Contains(appendOut, "daily.append") || !strings.Contains(appendOut, "index_updated") {
		t.Fatalf("daily append output invalid:\n%s", appendOut)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, filepath.FromSlash(dailyRel))), "复盘") {
		t.Fatalf("daily append did not write body")
	}

	journalDate := "2026-06-06"
	dailyDatedRel := filepath.ToSlash(filepath.Join("daily", journalDate+".md"))
	dailyDatedOut := runCLI(t, "daily", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(dailyDatedOut, dailyDatedRel) || !strings.Contains(dailyDatedOut, `"period":"daily"`) {
		t.Fatalf("daily show --date output invalid:\n%s", dailyDatedOut)
	}
	dailyNextOut := runCLI(t, "daily", "show", "--date", journalDate, "--next", "--vault", root, "--json")
	if !strings.Contains(dailyNextOut, "daily/2026-06-07.md") || !strings.Contains(dailyNextOut, `"date":"2026-06-07"`) {
		t.Fatalf("daily show --next output invalid:\n%s", dailyNextOut)
	}

	weeklyOut := runCLI(t, "weekly", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(weeklyOut, "weekly.show") || !strings.Contains(weeklyOut, "weekly/2026-W23.md") || !strings.Contains(weeklyOut, `"period":"weekly"`) {
		t.Fatalf("weekly show output invalid:\n%s", weeklyOut)
	}
	weeklyKeyOut := runCLI(t, "weekly", "show", "--date", "2026-W23", "--vault", root, "--json")
	if !strings.Contains(weeklyKeyOut, "weekly/2026-W23.md") || !strings.Contains(weeklyKeyOut, `"date":"2026-W23"`) {
		t.Fatalf("weekly show should accept completion key:\n%s", weeklyKeyOut)
	}
	monthlyOut := runCLI(t, "monthly", "show", "--date", journalDate, "--vault", root)
	for _, want := range []string{"Highlights", "monthly/2026-06.md", "Period", "monthly", "Monthly 2026-06"} {
		if !strings.Contains(monthlyOut, want) {
			t.Fatalf("monthly show human output missing %q:\n%s", want, monthlyOut)
		}
	}
	monthlyKeyOut := runCLI(t, "monthly", "show", "--date", "2026-06", "--vault", root, "--json")
	if !strings.Contains(monthlyKeyOut, "monthly/2026-06.md") || !strings.Contains(monthlyKeyOut, `"date":"2026-06"`) {
		t.Fatalf("monthly show should accept completion key:\n%s", monthlyKeyOut)
	}

	captureOut := runCLI(t, "inbox", "capture", "Inbox Idea", "--body", "body", "--tags", "idea", "--vault", root, "--json")
	var captureEnvelope map[string]any
	if err := json.Unmarshal([]byte(captureOut), &captureEnvelope); err != nil {
		t.Fatalf("inbox capture json invalid: %v\n%s", err, captureOut)
	}
	captureFacts := captureEnvelope["facts"].(map[string]any)
	inboxPath := captureFacts["path"].(string)
	if captureEnvelope["command"] != "inbox.capture" || !strings.HasPrefix(inboxPath, "inbox/") || captureFacts["kind"] != "inbox" || captureFacts["status"] != "inbox" {
		t.Fatalf("inbox capture envelope = %#v", captureEnvelope)
	}
	inboxContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(inboxPath)))
	for _, want := range []string{"kind: inbox", "status: inbox", "body"} {
		if !strings.Contains(inboxContent, want) {
			t.Fatalf("inbox content missing %q:\n%s", want, inboxContent)
		}
	}

	listOut := runCLI(t, "inbox", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("inbox list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "inbox.list" || listEnvelope["facts"].(map[string]any)["returned"] != "1" {
		t.Fatalf("inbox list envelope = %#v", listEnvelope)
	}

	triageOut := runCLI(t, "inbox", "triage", inboxPath, "--group", "work", "--folder", "ideas", "--kind", "reference", "--status", "active", "--vault", root, "--json")
	var triageEnvelope map[string]any
	if err := json.Unmarshal([]byte(triageOut), &triageEnvelope); err != nil {
		t.Fatalf("inbox triage json invalid: %v\n%s", err, triageOut)
	}
	triageFacts := triageEnvelope["facts"].(map[string]any)
	triagedPath := "notes/work/ideas/inbox-idea.md"
	if triageEnvelope["command"] != "inbox.triage" || triageFacts["path"] != triagedPath || triageFacts["status"] != "active" || triageFacts["kind"] != "reference" {
		t.Fatalf("inbox triage envelope = %#v", triageEnvelope)
	}
	triagedContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(triagedPath)))
	for _, want := range []string{"project: work", "folder: ideas", "kind: reference", "status: active"} {
		if !strings.Contains(triagedContent, want) {
			t.Fatalf("triaged note missing %q:\n%s", want, triagedContent)
		}
	}
}

func TestDailyTaskReviewManagedBlockCLI(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "research", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Due today", "--column", "next", "--due-at", "2026-06-21", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Overdue", "--column", "next", "--due-at", "2026-06-20", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Blocked task", "--column", "blocked", "--blocked-by", "api", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Review task", "--column", "review", "--vault", root, "--json")

	dailyPath := filepath.Join(root, "daily", "2026-06-21.md")
	writeCLIFixture(t, dailyPath, "# 2026-06-21\n\nmanual note\n")
	missingBefore := readCLIFile(t, dailyPath)
	missing, err := runCLIExpectError("plan", "daily", "--task-review", "--yes", "--vault", root, "--json")
	if err == nil || !strings.Contains(missing, `"code":"managed_block_missing"`) {
		t.Fatalf("missing marker should fail with managed_block_missing err=%v out=%s", err, missing)
	}
	if got := readCLIFile(t, dailyPath); got != missingBefore {
		t.Fatalf("missing marker changed daily note:\n%s", got)
	}

	withBlock := strings.Join([]string{
		"# 2026-06-21",
		"",
		"manual note",
		"",
		"<!-- pinax:managed name=daily-task-review -->",
		"old review",
		"<!-- /pinax:managed -->",
		"",
	}, "\n")
	writeCLIFixture(t, dailyPath, withBlock)
	preview := runCLI(t, "plan", "daily", "--task-review", "--vault", root, "--json")
	if !strings.Contains(preview, `"managed_block":"daily-task-review"`) || !strings.Contains(preview, `"writes":"false"`) {
		t.Fatalf("preview output invalid:\n%s", preview)
	}
	if got := readCLIFile(t, dailyPath); got != withBlock {
		t.Fatalf("preview changed daily note:\n%s", got)
	}

	applyOut := runCLI(t, "plan", "daily", "--task-review", "--yes", "--vault", root, "--json")
	for _, want := range []string{`"writes":"true"`, `"today":"1"`, `"overdue":"1"`, `"blocked":"1"`, `"review":"1"`} {
		if !strings.Contains(applyOut, want) {
			t.Fatalf("apply output missing %s:\n%s", want, applyOut)
		}
	}
	daily := readCLIFile(t, dailyPath)
	for _, want := range []string{"manual note", "## Daily Task Review", "Due today", "Overdue", "Blocked task", "Review task"} {
		if !strings.Contains(daily, want) {
			t.Fatalf("daily task review missing %q:\n%s", want, daily)
		}
	}
	if strings.Contains(daily, "old review") {
		t.Fatalf("daily task review did not replace managed content:\n%s", daily)
	}
}

func TestNoteListPropertyOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 2", "--status", "active", "--vault", root, "--json")
	out := runCLI(t, "note", "list", "--property", "priority", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note list property json invalid: %v\n%s", err, out)
	}
	if envelope["facts"].(map[string]any)["properties"] != "priority" || !strings.Contains(out, "priority") {
		t.Fatalf("note list property envelope = %#v", envelope)
	}
	failed, err := runCLIExpectError("note", "list", "--property", "missing", "--strict-properties", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "property_not_found") {
		t.Fatalf("strict property err=%v out=%s", err, failed)
	}
}

func TestNoteListPeriodFiltersCLI(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLINoteListPeriodFixture(t, root, "notes/recent.md", "Recent Five Hour", "2026-06-21T12:30:00Z")
	writeCLINoteListPeriodFixture(t, root, "notes/today.md", "Today Morning", "2026-06-21T08:00:00Z")
	writeCLINoteListPeriodFixture(t, root, "notes/week.md", "This Week", "2026-06-18T10:00:00Z")
	writeCLINoteListPeriodFixture(t, root, "notes/month.md", "This Month", "2026-06-02T10:00:00Z")
	writeCLINoteListPeriodFixture(t, root, "notes/old.md", "Older", "2026-05-31T23:59:59Z")

	fiveHour := runCLI(t, "note", "list", "--period", "5h", "--vault", root, "--json")
	assertNoteListTitles(t, fiveHour, []string{"Recent Five Hour"}, []string{"Today Morning", "This Week", "This Month", "Older"})
	assertNoteListFacts(t, fiveHour, map[string]string{"filter.period": "5h", "filter.updated_after": "2026-06-21T10:30:00Z", "returned": "1"})

	daily := runCLI(t, "note", "list", "--period", "daily", "--vault", root, "--json")
	assertNoteListTitles(t, daily, []string{"Recent Five Hour", "Today Morning"}, []string{"This Week", "This Month", "Older"})
	assertNoteListFacts(t, daily, map[string]string{"filter.period": "daily", "filter.updated_after": "2026-06-21T00:00:00Z", "returned": "2"})

	weekly := runCLI(t, "note", "list", "--period", "weekly", "--vault", root, "--json")
	assertNoteListTitles(t, weekly, []string{"Recent Five Hour", "Today Morning", "This Week"}, []string{"This Month", "Older"})
	assertNoteListFacts(t, weekly, map[string]string{"filter.period": "weekly", "filter.updated_after": "2026-06-15T00:00:00Z", "returned": "3"})

	monthly := runCLI(t, "note", "list", "--period", "monthly", "--vault", root, "--json")
	assertNoteListTitles(t, monthly, []string{"Recent Five Hour", "Today Morning", "This Week", "This Month"}, []string{"Older"})
	assertNoteListFacts(t, monthly, map[string]string{"filter.period": "monthly", "filter.updated_after": "2026-06-01T00:00:00Z", "returned": "4"})
}

func writeCLINoteListPeriodFixture(t *testing.T, root, rel, title, updatedAt string) {
	t.Helper()
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(rel)), "---\nschema_version: pinax.note.v1\nnote_id: "+strings.ReplaceAll(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)), "-", "_")+"\ntitle: "+title+"\ntags: []\nstatus: active\ncreated_at: "+updatedAt+"\nupdated_at: "+updatedAt+"\n---\n\n# "+title+"\n")
}

func assertNoteListTitles(t *testing.T, out string, wants []string, forbidden []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("note list missing %q:\n%s", want, out)
		}
	}
	for _, item := range forbidden {
		if strings.Contains(out, item) {
			t.Fatalf("note list should not include %q:\n%s", item, out)
		}
	}
}

func assertNoteListFacts(t *testing.T, out string, wants map[string]string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note list json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range wants {
		if facts[key] != want {
			t.Fatalf("note list fact %s = %#v, want %q; facts=%#v\n%s", key, facts[key], want, facts, out)
		}
	}
}

func TestJournalDateCompletionCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-04.md"), "# Daily 2026-06-04\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "# Daily 2026-06-06\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-05.md"), "# Daily 2026-06-05\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "weekly", "2026-W22.md"), "# Weekly 2026-W22\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "weekly", "2026-W23.md"), "# Weekly 2026-W23\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "monthly", "2026-06.md"), "# Monthly 2026-06\n")

	daily := runCLI(t, "__complete", "daily", "show", "--vault", root, "--date", "")
	for _, want := range []string{"\t", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(daily, want) {
			t.Fatalf("daily date completion missing %q:\n%s", want, daily)
		}
	}
	dailyLines := completionValueLines(daily)
	if strings.Join(dailyLines, ",") != "2026-06-06,2026-06-05,2026-06-04" {
		t.Fatalf("daily completion should be newest first:\n%s", daily)
	}
	if strings.Contains(daily, "2026-06-03") {
		t.Fatalf("daily completion should not include missing journals:\n%s", daily)
	}

	weekly := runCLI(t, "__complete", "weekly", "show", "--vault", root, "--date", "")
	for _, want := range []string{"week", "(", "--", ")"} {
		if !strings.Contains(weekly, want) {
			t.Fatalf("weekly date completion missing %q:\n%s", want, weekly)
		}
	}
	weeklyLines := completionValueLines(weekly)
	if strings.Join(weeklyLines, ",") != "2026-W23,2026-W22" {
		t.Fatalf("weekly completion should be newest first:\n%s", weekly)
	}
	if strings.Contains(weekly, "2026-W21") {
		t.Fatalf("weekly completion should not include missing journals:\n%s", weekly)
	}

	monthly := runCLI(t, "__complete", "monthly", "show", "--vault", root, "--date", "")
	if !strings.Contains(monthly, "--") || !strings.Contains(monthly, "\t") {
		t.Fatalf("monthly date completion should include date range descriptions:\n%s", monthly)
	}
	monthlyLines := completionValueLines(monthly)
	if strings.Join(monthlyLines, ",") != "2026-06" || strings.Contains(monthly, "2026-05") {
		t.Fatalf("monthly completion should only include existing journals:\n%s", monthly)
	}
}

func TestNotebookOrganizationViewsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "架构笔记", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth,design", "--body", "正文", "--slug", "arch-note", "--vault", root, "--json")
	runCLI(t, "note", "new", "草稿笔记", "--folder", "drafts", "--kind", "fleeting", "--status", "draft", "--tags", "draft", "--body", "草稿", "--slug", "draft-note", "--vault", root, "--json")

	listOut := runCLI(t, "note", "list", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--created-after", "2000-01-01", "--updated-before", "2999-01-01", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("org note list json invalid: %v\n%s", err, listOut)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	for _, key := range []string{"filter.group", "filter.folder", "filter.kind", "filter.status", "filter.created_after", "filter.updated_before"} {
		if listFacts[key] == "" {
			t.Fatalf("note list facts missing %s: %#v", key, listFacts)
		}
	}
	if listFacts["returned"] != "1" || !strings.Contains(listOut, "架构笔记") || strings.Contains(listOut, "草稿笔记") {
		t.Fatalf("org note list output invalid facts=%#v out=%s", listFacts, listOut)
	}

	for _, tc := range []struct {
		args []string
		cmd  string
		want string
	}{
		{args: []string{"tag", "list", "--vault", root, "--json"}, cmd: "tag.list", want: "auth"},
		{args: []string{"kind", "list", "--vault", root, "--json"}, cmd: "kind.list", want: "reference"},
		{args: []string{"group", "list", "--vault", root, "--json"}, cmd: "group.list", want: "work"},
	} {
		out := runCLI(t, tc.args...)
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("%s json invalid: %v\n%s", tc.cmd, err, out)
		}
		facts := envelope["facts"].(map[string]any)
		if envelope["command"] != tc.cmd || facts["dimensions"] == "" || facts["notes"] == "" || !strings.Contains(out, tc.want) {
			t.Fatalf("%s output invalid facts=%#v out=%s", tc.cmd, facts, out)
		}
	}
	folderOut := runCLI(t, "folder", "list", "--vault", root, "--json")
	var folderEnvelope map[string]any
	if err := json.Unmarshal([]byte(folderOut), &folderEnvelope); err != nil {
		t.Fatalf("folder list json invalid: %v\n%s", err, folderOut)
	}
	folderFacts := folderEnvelope["facts"].(map[string]any)
	if folderEnvelope["command"] != "folder.list" || folderFacts["folders"] == "" || !strings.Contains(folderOut, "architecture") {
		t.Fatalf("folder list output invalid facts=%#v out=%s", folderFacts, folderOut)
	}
}

func TestNoteLinkGraphCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nSee [[Beta]] and [[Missing Target]].\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nkind: reference\n---\n\n# Beta\n\nLinked by Alpha.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "gamma.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_gamma\ntitle: Gamma\nkind: reference\n---\n\n# Gamma\n\nNo graph edges.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily_index\ntitle: Daily Index\nkind: index\n---\n\n# Daily Index\n\nSystem index note.\n")

	linksOut := runCLI(t, "note", "links", "Alpha", "--vault", root, "--json")
	var linksEnvelope map[string]any
	if err := json.Unmarshal([]byte(linksOut), &linksEnvelope); err != nil {
		t.Fatalf("note links json invalid: %v\n%s", err, linksOut)
	}
	linksFacts := linksEnvelope["facts"].(map[string]any)
	if linksEnvelope["command"] != "note.links" || linksFacts["links"] != "2" || linksFacts["resolved"] != "1" || linksFacts["broken"] != "1" || !strings.Contains(linksOut, "notes/beta.md") || !strings.Contains(linksOut, "Missing Target") {
		t.Fatalf("note links envelope = %#v out=%s", linksEnvelope, linksOut)
	}

	backlinksOut := runCLI(t, "note", "backlinks", "Beta", "--vault", root, "--json")
	var backlinksEnvelope map[string]any
	if err := json.Unmarshal([]byte(backlinksOut), &backlinksEnvelope); err != nil {
		t.Fatalf("note backlinks json invalid: %v\n%s", err, backlinksOut)
	}
	backlinksFacts := backlinksEnvelope["facts"].(map[string]any)
	if backlinksEnvelope["command"] != "note.backlinks" || backlinksFacts["backlinks"] != "1" || backlinksFacts["unresolved"] != "0" || !strings.Contains(backlinksOut, "notes/alpha.md") {
		t.Fatalf("note backlinks envelope = %#v out=%s", backlinksEnvelope, backlinksOut)
	}

	orphansOut := runCLI(t, "note", "orphans", "--vault", root, "--json")
	var orphansEnvelope map[string]any
	if err := json.Unmarshal([]byte(orphansOut), &orphansEnvelope); err != nil {
		t.Fatalf("note orphans json invalid: %v\n%s", err, orphansOut)
	}
	orphansFacts := orphansEnvelope["facts"].(map[string]any)
	if orphansEnvelope["command"] != "note.orphans" || orphansFacts["orphans"] != "1" || !strings.Contains(orphansOut, "notes/gamma.md") || strings.Contains(orphansOut, "Daily Index") {
		t.Fatalf("note orphans envelope = %#v out=%s", orphansEnvelope, orphansOut)
	}
	orphansHuman := runCLI(t, "note", "orphans", "--vault", root)
	for _, want := range []string{"Highlights", "Orphan notes listed.", "Path", "Title", "Kind", "notes/gamma.md", "Gamma", "Reference"} {
		if !strings.Contains(orphansHuman, want) {
			t.Fatalf("note orphans human output missing %q:\n%s", want, orphansHuman)
		}
	}
	for _, old := range []string{"状态:", "重点:", "事实:"} {
		if strings.Contains(orphansHuman, old) {
			t.Fatalf("note orphans human output still uses old prose %q:\n%s", old, orphansHuman)
		}
	}
}

func TestLinkOutputContractModes(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "links", "Alpha", "--vault", root, "--json")
	assertMachineOutputClean(t, out)
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") {
		t.Fatalf("link json leaked note body or secret:\n%s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("link json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "note.links" || envelope["mode"] != "json" || facts["links"] != "2" || facts["broken"] != "1" || facts["engine"] == "" {
		t.Fatalf("link json envelope = %#v", envelope)
	}
}

func TestBacklinkOutputContractAgent(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "backlinks", "Beta", "--vault", root, "--agent")
	assertMachineOutputClean(t, out)
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=note.backlinks", "status=success", "fact.backlinks=1", "fact.engine="} {
		if !strings.Contains(out, want) {
			t.Fatalf("backlink agent missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") {
		t.Fatalf("backlink agent leaked note body or secret:\n%s", out)
	}
}

func TestOrphanOutputContractEvents(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "orphans", "--vault", root, "--events")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("orphan events lines = %q", out)
	}
	for i, line := range lines {
		assertMachineOutputClean(t, line)
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("orphan event %d invalid: %v\n%s", i, err, out)
		}
		if event["command"] != "note.orphans" || event["mode"] != "events" {
			t.Fatalf("orphan event %d payload = %#v", i, event)
		}
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) || !strings.Contains(lines[1], `"orphans":"1"`) {
		t.Fatalf("orphan events missing start/end facts:\n%s", out)
	}
}

func TestGraphExplainOutputContract(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "links", "Alpha", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("graph explain missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") || strings.Contains(out, "system prompt") {
		t.Fatalf("graph explain leaked sensitive body text:\n%s", out)
	}
}

func TestStdoutStderrLinkOutputContract(t *testing.T) {
	root := linkOutputFixture(t)
	stdout, stderr, err := runCLISeparate("note", "links", "Alpha", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("link json stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("link stdout json invalid: %v\n%s", err, stdout)
	}
	stdout, stderr, err = runCLISeparate("note", "links", "Missing", "--vault", root, "--json")
	if err == nil || stderr != "" {
		t.Fatalf("link error stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("link error stdout json invalid: %v\n%s", err, stdout)
	}
	errorObject := envelope["error"].(map[string]any)
	if envelope["status"] != "failed" || errorObject["code"] != "note_not_found" {
		t.Fatalf("link error envelope = %#v", envelope)
	}
}

func TestNoteAttachPlacementLinkStyleAndModesCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nBody.\n")
	sourceDir := t.TempDir()
	diagram := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, diagram, "png-bytes")

	attachOut := runCLI(t, "note", "attach", "Alpha", diagram, "--placement", "note-folder", "--embed", "--rename", "renamed.png", "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("note attach placement json invalid: %v\n%s", err, attachOut)
	}
	facts := attachEnvelope["facts"].(map[string]any)
	if attachEnvelope["command"] != "note.attach" || facts["attachment_path"] != "notes/assets/renamed.png" || facts["placement"] != "note-folder" || facts["link_style"] != "markdown" || facts["mode"] != "copy" || facts["reference"] != "![renamed.png](assets/renamed.png)" {
		t.Fatalf("placement attach envelope = %#v", attachEnvelope)
	}
	if !fileExists(filepath.Join(root, "notes", "assets", "renamed.png")) || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "alpha.md")), "![renamed.png](assets/renamed.png)") {
		t.Fatalf("note-folder attach did not write expected file/reference")
	}

	before := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	moveSource := filepath.Join(sourceDir, "move.pdf")
	writeCLIFixture(t, moveSource, "move-pdf")
	failed, err := runCLIExpectError("note", "attach", "Alpha", moveSource, "--mode", "move", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("move without approval err=%v out=%s", err, failed)
	}
	if !fileExists(moveSource) || readCLIFile(t, filepath.Join(root, "notes", "alpha.md")) != before {
		t.Fatalf("move without approval modified source or note")
	}

	registeredPath := filepath.Join(root, "notes", "assets", "spec.pdf")
	writeCLIFixture(t, registeredPath, "pdf-bytes")
	registerOut := runCLI(t, "note", "attach", "Alpha", registeredPath, "--mode", "register", "--link-style", "wiki", "--vault", root, "--json")
	var registerEnvelope map[string]any
	if err := json.Unmarshal([]byte(registerOut), &registerEnvelope); err != nil {
		t.Fatalf("note attach register json invalid: %v\n%s", err, registerOut)
	}
	registerFacts := registerEnvelope["facts"].(map[string]any)
	if registerFacts["attachment_path"] != "notes/assets/spec.pdf" || registerFacts["mode"] != "register" || registerFacts["link_style"] != "wiki" || registerFacts["reference"] != "[[notes/assets/spec.pdf]]" {
		t.Fatalf("register attach envelope = %#v", registerEnvelope)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "alpha.md")), "[[notes/assets/spec.pdf]]") {
		t.Fatalf("register attach missing wiki reference")
	}
}

func TestImportExportMarkdownCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "research", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: existing_alpha\ntitle: Existing Alpha\ntags: [imported]\nproject: research\n---\n\n# Existing Alpha\n")
	sourceDir := t.TempDir()
	writeCLIFixture(t, filepath.Join(sourceDir, "alpha.md"), "# Alpha\n\nImported alpha.\n")
	writeCLIFixture(t, filepath.Join(sourceDir, "beta.md"), "# Beta\n\nImported beta.\n")

	dryRunOut := runCLI(t, "import", "markdown", sourceDir, "--group", "research", "--tags", "imported", "--dry-run", "--vault", root, "--json")
	var dryRunEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRunOut), &dryRunEnvelope); err != nil {
		t.Fatalf("import dry-run json invalid: %v\n%s", err, dryRunOut)
	}
	dryFacts := dryRunEnvelope["facts"].(map[string]any)
	if dryRunEnvelope["command"] != "import.markdown" || dryFacts["dry_run"] != "true" || dryFacts["planned"] != "2" || dryFacts["written"] != "0" {
		t.Fatalf("import dry-run envelope = %#v", dryRunEnvelope)
	}
	if fileExists(filepath.Join(root, "notes", "research", "beta.md")) || fileExists(filepath.Join(root, ".pinax", "receipts")) {
		t.Fatalf("dry-run wrote vault assets")
	}

	importOut := runCLI(t, "import", "markdown", sourceDir, "--group", "research", "--tags", "imported", "--kind", "reference", "--status", "active", "--conflict", "rename", "--yes", "--vault", root, "--json")
	var importEnvelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &importEnvelope); err != nil {
		t.Fatalf("import json invalid: %v\n%s", err, importOut)
	}
	importFacts := importEnvelope["facts"].(map[string]any)
	if importEnvelope["command"] != "import.markdown" || importFacts["written"] != "2" || importFacts["renamed"] != "1" || importFacts["receipt_path"] == "" {
		t.Fatalf("import envelope = %#v", importEnvelope)
	}
	if !fileExists(filepath.Join(root, "notes", "research", "alpha-2.md")) || !fileExists(filepath.Join(root, "notes", "research", "beta.md")) {
		t.Fatalf("imported files missing")
	}
	imported := readCLIFile(t, filepath.Join(root, "notes", "research", "beta.md"))
	for _, want := range []string{"schema_version: pinax.note.v1", "project: research", "kind: reference", "status: active", "tags: [imported]"} {
		if !strings.Contains(imported, want) {
			t.Fatalf("imported note missing %q:\n%s", want, imported)
		}
	}
	writeCLIFixture(t, filepath.Join(sourceDir, "beta.md"), "# Beta\n\nOverwritten beta.\n")
	overwriteOut := runCLI(t, "import", "markdown", filepath.Join(sourceDir, "beta.md"), "--group", "research", "--tags", "imported", "--conflict", "overwrite", "--yes", "--vault", root, "--json")
	var overwriteEnvelope map[string]any
	if err := json.Unmarshal([]byte(overwriteOut), &overwriteEnvelope); err != nil {
		t.Fatalf("overwrite import json invalid: %v\n%s", err, overwriteOut)
	}
	overwriteFacts := overwriteEnvelope["facts"].(map[string]any)
	if overwriteEnvelope["command"] != "import.markdown" || overwriteFacts["written"] != "1" || overwriteFacts["overwritten"] != "1" || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "research", "beta.md")), "Overwritten beta") {
		t.Fatalf("overwrite import failed envelope=%#v", overwriteEnvelope)
	}

	asset := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, asset, "png")
	attachOut := runCLI(t, "note", "attach", "Beta", asset, "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("attach json invalid: %v\n%s", err, attachOut)
	}
	attachmentPath := attachEnvelope["facts"].(map[string]any)["attachment_path"].(string)
	outDir := filepath.Join(t.TempDir(), "bundle")
	exportOut := runCLI(t, "export", "markdown", outDir, "--tag", "imported", "--vault", root, "--json")
	var exportEnvelope map[string]any
	if err := json.Unmarshal([]byte(exportOut), &exportEnvelope); err != nil {
		t.Fatalf("export json invalid: %v\n%s", err, exportOut)
	}
	exportFacts := exportEnvelope["facts"].(map[string]any)
	if exportEnvelope["command"] != "export.markdown" || exportFacts["notes"] == "0" || exportFacts["receipt_path"] == "" {
		t.Fatalf("export envelope = %#v", exportEnvelope)
	}
	if !fileExists(filepath.Join(outDir, "notes", "research", "beta.md")) || !fileExists(filepath.Join(outDir, attachmentPath)) {
		t.Fatalf("export missing note or attachment: out=%s attachment=%s", exportOut, attachmentPath)
	}
}

func TestNoteCreateBuildsNotebookInformationArchitecture(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")

	out := runCLI(t, "note", "new", "工具笔记", "--group", "work", "--folder", "inbox", "--kind", "reference", "--tags", "pinax,cli", "--body", "正文 #daily", "--slug", "tool-note", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["project"] != "work" || facts["group"] != "work" || facts["folder"] != "inbox" || facts["kind"] != "reference" || facts["index_updated"] != "true" || facts["daily_index"] == "" {
		t.Fatalf("note facts missing notebook IA: %#v", facts)
	}
	path := filepath.Join(root, "notes", "work", "inbox", "tool-note.md")
	content := readCLIFile(t, path)
	for _, want := range []string{"project: work", "folder: inbox", "kind: reference", "tags: [pinax, cli]", "正文 #daily"} {
		if !strings.Contains(content, want) {
			t.Fatalf("note content missing %q:\n%s", want, content)
		}
	}
	dailyIndexPath := filepath.Join(root, facts["daily_index"].(string))
	dailyIndex := readCLIFile(t, dailyIndexPath)
	for _, want := range []string{"notes/work/inbox/tool-note.md", "#pinax", "group=work", "folder=inbox", "kind=reference"} {
		if !strings.Contains(dailyIndex, want) {
			t.Fatalf("daily index missing %q:\n%s", want, dailyIndex)
		}
	}
	statsOut := runCLI(t, "stats", "--vault", root, "--json")
	var stats map[string]any
	if err := json.Unmarshal([]byte(statsOut), &stats); err != nil {
		t.Fatalf("stats json invalid: %v\n%s", err, statsOut)
	}
	statsFacts := stats["facts"].(map[string]any)
	if statsFacts["notes"] != "1" || statsFacts["index_status"] != "fresh" {
		t.Fatalf("index not fresh after note create: %#v", statsFacts)
	}
}

func TestNoteShowStemAndMetadataPlanQueryResolverContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "stem-target.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_stem\ntitle: Different Title\ntags: []\n---\n\n# Different Title\n")
	showOut := runCLI(t, "note", "show", "stem-target", "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("note show stem json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "note.show" || showFacts["path"] != "notes/stem-target.md" || showFacts["resolver.match_field"] != "stem" || showFacts["resolver.candidates"] != "1" {
		t.Fatalf("note show stem envelope = %#v", showEnvelope)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "adopt-target.md"), "# Adopt Target\n\nraw markdown\n")
	metadataOut := runCLI(t, "metadata", "plan", "adopt-target", "--vault", root, "--json")
	var metadataEnvelope map[string]any
	if err := json.Unmarshal([]byte(metadataOut), &metadataEnvelope); err != nil {
		t.Fatalf("metadata plan query json invalid: %v\n%s", err, metadataOut)
	}
	metadataFacts := metadataEnvelope["facts"].(map[string]any)
	if metadataEnvelope["command"] != "metadata.plan" || metadataFacts["candidates"] != "1" || metadataFacts["planned_updates"] != "1" || metadataFacts["writes"] != "false" || !strings.Contains(metadataOut, "record adopt adopt-target --plan") {
		t.Fatalf("metadata plan query envelope = %#v\n%s", metadataEnvelope, metadataOut)
	}
	if fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("metadata plan query wrote ledger")
	}

	ambiguousRoot := t.TempDir()
	runCLI(t, "init", ambiguousRoot, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "target-a.md"), "# Target A\n")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "target-b.md"), "# Target B\n")
	ambiguous, err := runCLIExpectError("metadata", "plan", "target", "--vault", ambiguousRoot, "--json")
	if err == nil {
		t.Fatalf("metadata plan ambiguous succeeded: %s", ambiguous)
	}
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguous), &ambiguousEnvelope); err != nil {
		t.Fatalf("metadata plan ambiguous json invalid: %v\n%s", err, ambiguous)
	}
	if ambiguousEnvelope["command"] != "metadata.plan" || ambiguousEnvelope["status"] != "failed" || ambiguousEnvelope["error"].(map[string]any)["code"] != "vault_object_ref_ambiguous" || !strings.Contains(ambiguous, "notes/target-a.md") || !strings.Contains(ambiguous, "notes/target-b.md") {
		t.Fatalf("metadata ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguous)
	}
}

func TestRecordAdoptQueryPlanContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "yeisme.md"), "# Yeisme\n\nunmanaged markdown\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "other.md"), "# Other\n\nunmanaged markdown\n")

	planOut := runCLI(t, "record", "adopt", "yeisme", "--plan", "--vault", root, "--json")
	var planEnvelope map[string]any
	if err := json.Unmarshal([]byte(planOut), &planEnvelope); err != nil {
		t.Fatalf("record adopt query plan json invalid: %v\n%s", err, planOut)
	}
	planFacts := planEnvelope["facts"].(map[string]any)
	if planEnvelope["command"] != "record.adopt" || planFacts["writes"] != "false" || planFacts["candidates"] != "1" || planFacts["operations"] != "1" || planFacts["adopted"] != "0" {
		t.Fatalf("record adopt query plan envelope = %#v", planEnvelope)
	}
	if !strings.Contains(planOut, "notes/yeisme.md") || !strings.Contains(planOut, "pinax record adopt yeisme --vault") {
		t.Fatalf("record adopt query plan missing operation/action:\n%s", planOut)
	}
	if fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("record adopt --plan wrote ledger")
	}

	fullPlan := runCLI(t, "record", "adopt", "--plan", "--vault", root, "--json")
	var fullEnvelope map[string]any
	if err := json.Unmarshal([]byte(fullPlan), &fullEnvelope); err != nil {
		t.Fatalf("record adopt full plan json invalid: %v\n%s", err, fullPlan)
	}
	fullFacts := fullEnvelope["facts"].(map[string]any)
	if fullFacts["operations"] != "2" || fullFacts["writes"] != "false" || fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("record adopt full plan envelope = %#v", fullEnvelope)
	}

	ambiguousRoot := t.TempDir()
	runCLI(t, "init", ambiguousRoot, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "yeisme-a.md"), "# Yeisme A\n")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "yeisme-b.md"), "# Yeisme B\n")
	ambiguous, err := runCLIExpectError("record", "adopt", "yeisme", "--plan", "--vault", ambiguousRoot, "--json")
	if err == nil {
		t.Fatalf("ambiguous record adopt succeeded: %s", ambiguous)
	}
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguous), &ambiguousEnvelope); err != nil {
		t.Fatalf("record adopt ambiguous json invalid: %v\n%s", err, ambiguous)
	}
	if ambiguousEnvelope["command"] != "record.adopt" || ambiguousEnvelope["status"] != "failed" || ambiguousEnvelope["error"].(map[string]any)["code"] != "vault_object_ref_ambiguous" || !strings.Contains(ambiguous, "notes/yeisme-a.md") || !strings.Contains(ambiguous, "notes/yeisme-b.md") {
		t.Fatalf("record adopt ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguous)
	}
	if fileExists(filepath.Join(ambiguousRoot, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("ambiguous record adopt wrote ledger")
	}
}

func TestRecordHistoryUsesResolverInputCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "History Note", "--body", "history body", "--slug", "history-note", "--vault", root, "--json")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	runCLI(t, "record", "adopt", "--vault", root, "--json")

	out := runCLI(t, "record", "history", "history-note", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("record history json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "record.history" || facts["note_id"] == "" || facts["path"] != "history-note.md" || facts["candidates"] != "1" || facts["match_field"] != "stem" {
		t.Fatalf("record history resolver envelope = %#v\n%s", envelope, out)
	}
}

func TestNoteCommandUXCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	inlineOut := runCLI(t, "note", "new", "Inline Note", "--body", "正文", "--tags", "research", "--status", "active", "--dir", "work", "--slug", "inline", "--vault", root, "--json")
	var inlineEnvelope map[string]any
	if err := json.Unmarshal([]byte(inlineOut), &inlineEnvelope); err != nil {
		t.Fatalf("inline note json invalid: %v\n%s", err, inlineOut)
	}
	if inlineEnvelope["command"] != "note.new" || inlineEnvelope["status"] != "success" {
		t.Fatalf("inline envelope = %#v", inlineEnvelope)
	}
	inlineContent := readCLIFile(t, filepath.Join(root, "notes", "work", "inline.md"))
	for _, want := range []string{"title: Inline Note", "tags: [research]", "status: active", "正文"} {
		if !strings.Contains(inlineContent, want) {
			t.Fatalf("inline note missing %q:\n%s", want, inlineContent)
		}
	}

	dryRun := runCLI(t, "note", "create", "Draft", "--dry-run", "--dir", "work", "--vault", root, "--json")
	if !strings.Contains(dryRun, "planned_path") || fileExists(filepath.Join(root, "notes", "work", "draft.md")) {
		t.Fatalf("dry-run did not return plan or wrote file:\n%s", dryRun)
	}

	failed, err := runCLIExpectError("note", "new", "Conflict", "--body", "a", "--from", filepath.Join(root, "source.md"), "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "note_source_conflict") {
		t.Fatalf("source conflict err=%v out=%s", err, failed)
	}
	missingSource, err := runCLIExpectError("note", "new", "Missing Source", "--from", filepath.Join(root, "missing.md"), "--vault", root, "--json")
	if err == nil || !strings.Contains(missingSource, "internal_error") || fileExists(filepath.Join(root, "notes", "missing-source.md")) {
		t.Fatalf("missing source err=%v out=%s", err, missingSource)
	}

	stdinOut := runCLIWithInput(t, "stdin body", "note", "create", "Stdin Note", "--stdin", "--vault", root, "--json")
	if !strings.Contains(stdinOut, "note.new") || !strings.Contains(readCLIFile(t, filepath.Join(root, "stdin-note.md")), "stdin body") {
		t.Fatalf("stdin create failed:\n%s", stdinOut)
	}

	listOut := runCLI(t, "note", "list", "--tag", "research", "--status", "active", "--recent", "--limit", "1", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "note.list" || listEnvelope["status"] != "success" {
		t.Fatalf("list envelope = %#v", listEnvelope)
	}
	facts := listEnvelope["facts"].(map[string]any)
	if facts["total"] != "1" || facts["returned"] != "1" || facts["filter.tag"] != "research" {
		t.Fatalf("list facts = %#v", facts)
	}
	humanList := runCLI(t, "note", "list", "--tag", "research", "--vault", root)
	for _, want := range []string{"Highlights", "Path", "Title", "Inline Note", "notes/work/inline.md"} {
		if !strings.Contains(humanList, want) {
			t.Fatalf("human list missing %q:\n%s", want, humanList)
		}
	}

	showByTitle := runCLI(t, "note", "read", "Inline Note", "--vault", root, "--json")
	if !strings.Contains(showByTitle, "notes/work/inline.md") {
		t.Fatalf("show by title failed:\n%s", showByTitle)
	}
	writeCLIFixture(t, filepath.Join(root, "notes", "dupe-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dupe_a\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "dupe-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dupe_b\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	ambiguous, err := runCLIExpectError("note", "show", "Meeting", "--vault", root, "--json")
	if err == nil || !strings.Contains(ambiguous, "note_ref_ambiguous") || !strings.Contains(ambiguous, "dupe-a.md") {
		t.Fatalf("ambiguous ref err=%v out=%s", err, ambiguous)
	}
	ambiguousAgent, err := runCLIExpectError("note", "show", "Meeting", "--vault", root, "--agent")
	if err == nil || !strings.Contains(ambiguousAgent, "error.code=note_ref_ambiguous") || !strings.Contains(ambiguousAgent, "candidate.1.path=notes/dupe-a.md") || !strings.Contains(ambiguousAgent, "candidate.2.note_id=note_dupe_b") {
		t.Fatalf("ambiguous agent err=%v out=%s", err, ambiguousAgent)
	}

	editorLog := filepath.Join(root, "editor.log")
	editor := writeFakeEditor(t, root, editorLog)
	editOut := runCLI(t, "note", "edit", "Inline Note", "--editor", editor, "--vault", root, "--json")
	if !strings.Contains(editOut, "note.edit") || !strings.Contains(readCLIFile(t, editorLog), "notes/work/inline.md") {
		t.Fatalf("edit output/log invalid:\n%s\nlog=%s", editOut, readCLIFile(t, editorLog))
	}

	oldEditor, hadEditor := os.LookupEnv("EDITOR")
	_ = os.Unsetenv("EDITOR")
	t.Cleanup(func() {
		if hadEditor {
			_ = os.Setenv("EDITOR", oldEditor)
		}
	})
	missingEditor, err := runCLIExpectError("note", "edit", "Inline Note", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingEditor, "editor_not_configured") {
		t.Fatalf("missing editor err=%v out=%s", err, missingEditor)
	}

	renameOut := runCLI(t, "note", "rename", "Inline Note", "Renamed Note", "--vault", root, "--json")
	if !strings.Contains(renameOut, "note.rename") || !fileExists(filepath.Join(root, "notes", "work", "renamed-note.md")) {
		t.Fatalf("rename failed:\n%s", renameOut)
	}
	moveOut := runCLI(t, "note", "move", "Renamed Note", "archive", "--vault", root, "--json")
	if !strings.Contains(moveOut, "note.move") || !fileExists(filepath.Join(root, "notes", "archive", "renamed-note.md")) {
		t.Fatalf("move failed:\n%s", moveOut)
	}
	archiveOut := runCLI(t, "note", "archive", "Renamed Note", "--vault", root, "--json")
	archivedContent := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if !strings.Contains(archiveOut, "note.archive") || !strings.Contains(archivedContent, "status: archived") {
		t.Fatalf("archive failed:\n%s\n%s", archiveOut, archivedContent)
	}
	propertyOut := runCLI(t, "note", "property", "set", "Renamed Note", "priority", "2", "--vault", root, "--json")
	if !strings.Contains(propertyOut, "note.property") || !strings.Contains(propertyOut, `"property":"priority"`) || !strings.Contains(propertyOut, `"index_updated":"true"`) {
		t.Fatalf("property set output invalid:\n%s", propertyOut)
	}
	withProperty := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if !strings.Contains(withProperty, "priority: 2") {
		t.Fatalf("property set did not update frontmatter:\n%s", withProperty)
	}
	propertyList := runCLI(t, "note", "list", "--property", "priority", "--strict-properties", "--vault", root, "--json")
	if !strings.Contains(propertyList, "priority") || !strings.Contains(propertyList, "Renamed Note") {
		t.Fatalf("property set did not update property projection:\n%s", propertyList)
	}
	propertyRemove := runCLI(t, "note", "property", "remove", "Renamed Note", "priority", "--vault", root, "--agent")
	for _, want := range []string{"command=note.property", "fact.operation=remove", "fact.property=priority", "fact.index_updated=true"} {
		if !strings.Contains(propertyRemove, want) {
			t.Fatalf("property remove agent missing %q:\n%s", want, propertyRemove)
		}
	}
	withoutProperty := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if strings.Contains(withoutProperty, "priority:") {
		t.Fatalf("property remove left priority frontmatter:\n%s", withoutProperty)
	}
	tagOut := runCLI(t, "note", "tag", "add", "Renamed Note", "important", "--vault", root, "--json")
	if !strings.Contains(tagOut, "note.tag") || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md")), "important") {
		t.Fatalf("tag add failed:\n%s", tagOut)
	}
	deleteWithoutApproval, err := runCLIExpectError("note", "delete", "Renamed Note", "--hard", "--vault", root, "--json")
	if err == nil || !strings.Contains(deleteWithoutApproval, "approval_required") {
		t.Fatalf("hard delete without approval err=%v out=%s", err, deleteWithoutApproval)
	}
	deleteOut := runCLI(t, "note", "delete", "Renamed Note", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "note.delete") || !strings.Contains(deleteOut, ".pinax/trash/") || fileExists(filepath.Join(root, "notes", "archive", "renamed-note.md")) {
		t.Fatalf("trash delete failed:\n%s", deleteOut)
	}

	help := runCLI(t, "note", "--help")
	for _, want := range []string{"create", "read", "open", "edit", "rename", "move", "archive", "delete", "tag", "property"} {
		if !strings.Contains(help, want) {
			t.Fatalf("note help missing %q:\n%s", want, help)
		}
	}
}

func TestNotePropertyPreservesObsidianPluginFrontmatterCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "notes", "obsidian.md")
	writeCLIFixture(t, notePath, strings.Join([]string{
		"---",
		"schema_version: pinax.note.v1",
		"note_id: note_obsidian",
		"title: Obsidian Note",
		"tags: [research]",
		"cssclasses:",
		"  - wide",
		"obsidian_plugin_state: keep-me",
		"kanban-plugin: board-a",
		"---",
		"",
		"# Obsidian Note",
		"",
		"User edited body.",
	}, "\n"))

	setOut := runCLI(t, "note", "property", "set", "Obsidian Note", "priority", "2", "--vault", root, "--json")
	if !strings.Contains(setOut, "note.property") || !strings.Contains(setOut, `"property":"priority"`) {
		t.Fatalf("property set output invalid:\n%s", setOut)
	}
	withProperty := readCLIFile(t, notePath)
	for _, want := range []string{"cssclasses:\n  - wide", "obsidian_plugin_state: keep-me", "kanban-plugin: board-a", "priority: 2", "User edited body."} {
		if !strings.Contains(withProperty, want) {
			t.Fatalf("property set failed to preserve %q:\n%s", want, withProperty)
		}
	}

	removeOut := runCLI(t, "note", "property", "remove", "Obsidian Note", "priority", "--vault", root, "--json")
	if !strings.Contains(removeOut, "note.property") || !strings.Contains(removeOut, `"operation":"remove"`) {
		t.Fatalf("property remove output invalid:\n%s", removeOut)
	}
	withoutProperty := readCLIFile(t, notePath)
	if strings.Contains(withoutProperty, "priority:") {
		t.Fatalf("property remove left priority frontmatter:\n%s", withoutProperty)
	}
	for _, want := range []string{"cssclasses:\n  - wide", "obsidian_plugin_state: keep-me", "kanban-plugin: board-a", "User edited body."} {
		if !strings.Contains(withoutProperty, want) {
			t.Fatalf("property remove failed to preserve %q:\n%s", want, withoutProperty)
		}
	}
}

func TestNoteTagBulkManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Alpha", "--tags", "old,keep", "--vault", root, "--json")
	runCLI(t, "note", "new", "Beta", "--tags", "old", "--vault", root, "--json")

	dryRun := runCLI(t, "note", "tags", "rename", "old", "new", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRun, "tag.rename") || !strings.Contains(dryRun, `"dry_run":"true"`) || !strings.Contains(dryRun, `"matched":"2"`) {
		t.Fatalf("tag rename dry-run output invalid:\n%s", dryRun)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, "alpha.md")), "old") {
		t.Fatalf("tag rename dry-run unexpectedly changed note")
	}

	renameOut := runCLI(t, "note", "tags", "rename", "old", "new", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=tag.rename", "fact.old_tag=old", "fact.new_tag=new", "fact.changed=2", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("tag rename agent missing %q:\n%s", want, renameOut)
		}
	}
	for _, rel := range []string{"alpha.md", "beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if !strings.Contains(content, "new") || strings.Contains(content, "old") {
			t.Fatalf("tag rename did not update %s:\n%s", rel, content)
		}
	}

	deleteOut := runCLI(t, "note", "tags", "delete", "new", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "tag.delete") || !strings.Contains(deleteOut, `"changed":"2"`) || !strings.Contains(deleteOut, `"index_updated":"true"`) {
		t.Fatalf("tag delete output invalid:\n%s", deleteOut)
	}
	for _, rel := range []string{"alpha.md", "beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if strings.Contains(content, "new") {
			t.Fatalf("tag delete did not remove tag from %s:\n%s", rel, content)
		}
	}
}

func TestNoteFolderBulkManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Alpha", "--folder", "inbox", "--slug", "alpha", "--vault", root, "--json")
	runCLI(t, "note", "new", "Beta", "--folder", "inbox", "--slug", "beta", "--vault", root, "--json")

	dryRun := runCLI(t, "note", "folders", "rename", "inbox", "archive", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRun, "folder.rename") || !strings.Contains(dryRun, `"dry_run":"true"`) || !strings.Contains(dryRun, `"matched":"2"`) {
		t.Fatalf("folder rename dry-run output invalid:\n%s", dryRun)
	}
	if !fileExists(filepath.Join(root, "inbox", "alpha.md")) || fileExists(filepath.Join(root, "archive", "alpha.md")) {
		t.Fatalf("folder rename dry-run unexpectedly changed files")
	}

	withoutApproval, err := runCLIExpectError("note", "folders", "rename", "inbox", "archive", "--vault", root, "--json")
	if err == nil || !strings.Contains(withoutApproval, "approval_required") || fileExists(filepath.Join(root, "archive", "alpha.md")) {
		t.Fatalf("folder rename without approval err=%v out=%s", err, withoutApproval)
	}

	renameOut := runCLI(t, "note", "folders", "rename", "inbox", "archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.old_folder=inbox", "fact.new_folder=archive", "fact.changed=2", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename agent missing %q:\n%s", want, renameOut)
		}
	}
	for _, rel := range []string{"archive/alpha.md", "archive/beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if !strings.Contains(content, "folder: archive") || strings.Contains(content, "folder: inbox") {
			t.Fatalf("folder rename did not update %s:\n%s", rel, content)
		}
	}
	if fileExists(filepath.Join(root, "inbox", "alpha.md")) || fileExists(filepath.Join(root, "inbox", "beta.md")) {
		t.Fatalf("folder rename left old note files")
	}

	listOut := runCLI(t, "note", "list", "--folder", "archive", "--vault", root, "--json")
	if !strings.Contains(listOut, `"filter.folder":"archive"`) || !strings.Contains(listOut, `"total":"2"`) {
		t.Fatalf("folder rename did not refresh list projection:\n%s", listOut)
	}
}

func TestFolderCreateListShowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	dryRun := runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--dry-run", "--vault", root, "--json")
	var dryEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRun), &dryEnvelope); err != nil {
		t.Fatalf("folder create dry-run json invalid: %v\n%s", err, dryRun)
	}
	dryFacts := dryEnvelope["facts"].(map[string]any)
	if dryEnvelope["command"] != "folder.create" || dryFacts["folder_path"] != "spaces/research" || dryFacts["purpose"] != "notes" || dryFacts["dry_run"] != "true" || dryFacts["writes"] != "false" {
		t.Fatalf("folder create dry-run facts invalid: %#v\n%s", dryFacts, dryRun)
	}
	if fileExists(filepath.Join(root, "spaces", "research")) {
		t.Fatalf("folder create dry-run unexpectedly created directory")
	}

	createOut := runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(createOut), &createEnvelope); err != nil {
		t.Fatalf("folder create json invalid: %v\n%s", err, createOut)
	}
	createFacts := createEnvelope["facts"].(map[string]any)
	if createEnvelope["command"] != "folder.create" || createFacts["folder_path"] != "spaces/research" || createFacts["purpose"] != "notes" || createFacts["managed_status"] != "managed" || createFacts["index_updated"] != "true" {
		t.Fatalf("folder create facts invalid: %#v\n%s", createFacts, createOut)
	}
	if !fileExists(filepath.Join(root, "spaces", "research")) {
		t.Fatalf("folder create did not create directory")
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "folders.json"))
	if !strings.Contains(registry, "spaces/research") || !strings.Contains(registry, "pinax.folders.v1") {
		t.Fatalf("folder registry missing created folder:\n%s", registry)
	}

	showOut := runCLI(t, "folder", "show", "spaces/research", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.show", "fact.folder_path=spaces/research", "fact.purpose=notes", "fact.managed_status=managed"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("folder show agent missing %q:\n%s", want, showOut)
		}
	}

	listOut := runCLI(t, "folder", "list", "--purpose", "notes", "--include-empty", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("folder list json invalid: %v\n%s", err, listOut)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listEnvelope["command"] != "folder.list" || listFacts["folders"] == "" || listFacts["filter.purpose"] != "notes" || !strings.Contains(listOut, "spaces/research") {
		t.Fatalf("folder list output invalid facts=%#v out=%s", listFacts, listOut)
	}

	unsafeOut, err := runCLIExpectError("folder", "create", "../outside", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeOut, "unsafe_folder_path") {
		t.Fatalf("unsafe folder create err=%v out=%s", err, unsafeOut)
	}

	repairOut := runCLI(t, "folder", "repair", "--plan", "--vault", root, "--json")
	if !strings.Contains(repairOut, `"command":"folder.repair"`) || !strings.Contains(repairOut, `"writes":"false"`) {
		t.Fatalf("folder repair plan output invalid:\n%s", repairOut)
	}
}

func TestFolderListHumanOutputShowsDetailedRowsAndSubtreeFilterCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")
	runCLI(t, "folder", "create", "spaces/research/runs", "--purpose", "notes", "--vault", root, "--json")
	runCLI(t, "folder", "create", "assets/images", "--purpose", "assets", "--vault", root, "--json")
	runCLI(t, "note", "new", "Alpha", "--folder", "spaces/research", "--slug", "alpha", "--vault", root, "--json")

	human := runCLI(t, "folder", "list", "--include-empty", "--vault", root, "--color", "never")
	for _, want := range []string{"Folders", "Path", "Purpose", "Managed", "Exists", "Empty", "Notes", "Assets", "Depth", "spaces/research", "managed", "notes"} {
		if !strings.Contains(human, want) {
			t.Fatalf("folder list human output missing %q:\n%s", want, human)
		}
	}
	if strings.Contains(human, "\x1b[") {
		t.Fatalf("folder list human output should honor --color never:\n%s", human)
	}

	subtreeJSON := runCLI(t, "folder", "list", "--under", "spaces/research", "--depth", "1", "--include-empty", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(subtreeJSON), &envelope); err != nil {
		t.Fatalf("folder subtree json invalid: %v\n%s", err, subtreeJSON)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["filter.under"] != "spaces/research" || facts["depth"] != "1" {
		t.Fatalf("folder subtree facts invalid: %#v\n%s", facts, subtreeJSON)
	}
	if !strings.Contains(subtreeJSON, "spaces/research/runs") || strings.Contains(subtreeJSON, "assets/images") {
		t.Fatalf("folder subtree filter output invalid:\n%s", subtreeJSON)
	}
}

func TestFolderShowIncludesChildrenAndCountsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")
	runCLI(t, "folder", "create", "spaces/research/runs", "--purpose", "notes", "--vault", root, "--json")
	runCLI(t, "folder", "create", "spaces/research/outputs", "--purpose", "generic", "--vault", root, "--json")

	showOut := runCLI(t, "folder", "show", "spaces/research", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &envelope); err != nil {
		t.Fatalf("folder show json invalid: %v\n%s", err, showOut)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["child_folders"] != "2" || facts["descendant_folders"] != "2" {
		t.Fatalf("folder show child facts invalid: %#v\n%s", facts, showOut)
	}
	data := envelope["data"].(map[string]any)
	children := data["children"].([]any)
	if len(children) != 2 || !strings.Contains(showOut, "spaces/research/runs") || !strings.Contains(showOut, "spaces/research/outputs") {
		t.Fatalf("folder show children invalid: %#v\n%s", children, showOut)
	}

	human := runCLI(t, "folder", "show", "spaces/research", "--vault", root, "--color", "never")
	for _, want := range []string{"Folder details read.", "Folder path", "Child folders", "Children", "spaces/research/runs", "spaces/research/outputs"} {
		if !strings.Contains(human, want) {
			t.Fatalf("folder show human output missing %q:\n%s", want, human)
		}
	}
}

func TestFolderMutationManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")

	dryRename := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRename, "folder.rename") || !strings.Contains(dryRename, `"dry_run":"true"`) || !strings.Contains(dryRename, `"target_path":"spaces/archive"`) {
		t.Fatalf("folder rename dry-run output invalid:\n%s", dryRename)
	}
	if !fileExists(filepath.Join(root, "spaces", "research")) || fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename dry-run unexpectedly changed directories")
	}
	withoutApproval, err := runCLIExpectError("folder", "rename", "spaces/research", "spaces/archive", "--vault", root, "--json")
	if err == nil || !strings.Contains(withoutApproval, "approval_required") || fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename without approval err=%v out=%s", err, withoutApproval)
	}
	renameOut := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.folder_path=spaces/research", "fact.target_path=spaces/archive", "fact.renamed=true", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename agent missing %q:\n%s", want, renameOut)
		}
	}
	if fileExists(filepath.Join(root, "spaces", "research")) || !fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename did not move directory")
	}

	runCLI(t, "folder", "create", "containers", "--vault", root, "--json")
	moveOut := runCLI(t, "folder", "move", "spaces/archive", "containers", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.move", "fact.folder_path=spaces/archive", "fact.target_path=containers/archive", "fact.moved=true", "fact.index_updated=true"} {
		if !strings.Contains(moveOut, want) {
			t.Fatalf("folder move agent missing %q:\n%s", want, moveOut)
		}
	}
	if fileExists(filepath.Join(root, "spaces", "archive")) || !fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder move did not move directory")
	}

	manualPath := filepath.Join(root, "manual", "assets")
	if err := os.MkdirAll(manualPath, 0o755); err != nil {
		t.Fatalf("create fixture folder: %v", err)
	}
	writeCLIFixture(t, filepath.Join(manualPath, "diagram.txt"), "fixture")
	adoptOut := runCLI(t, "folder", "adopt", "manual/assets", "--purpose", "assets", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.adopt", "fact.folder_path=manual/assets", "fact.purpose=assets", "fact.managed_status=managed", "fact.index_updated=true"} {
		if !strings.Contains(adoptOut, want) {
			t.Fatalf("folder adopt agent missing %q:\n%s", want, adoptOut)
		}
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "folders.json"))
	if !strings.Contains(registry, "manual/assets") || !strings.Contains(registry, "containers/archive") {
		t.Fatalf("folder registry missing adopted or moved folder:\n%s", registry)
	}

	nonEmptyDelete, err := runCLIExpectError("folder", "delete", "manual/assets", "--empty-only", "--yes", "--vault", root, "--json")
	if err == nil || !strings.Contains(nonEmptyDelete, "folder_not_empty") || !fileExists(filepath.Join(manualPath, "diagram.txt")) {
		t.Fatalf("non-empty folder delete err=%v out=%s", err, nonEmptyDelete)
	}
	dryDelete := runCLI(t, "folder", "delete", "containers/archive", "--empty-only", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryDelete, "folder.delete") || !strings.Contains(dryDelete, `"dry_run":"true"`) || !fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder delete dry-run invalid:\n%s", dryDelete)
	}
	deleteOut := runCLI(t, "folder", "delete", "containers/archive", "--empty-only", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.delete", "fact.folder_path=containers/archive", "fact.deleted=true", "fact.index_updated=true"} {
		if !strings.Contains(deleteOut, want) {
			t.Fatalf("folder delete agent missing %q:\n%s", want, deleteOut)
		}
	}
	if fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder delete did not remove empty directory")
	}
}

func TestFolderRenameUpdatesContainedNoteMetadataCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Moved Note", "--folder", "spaces/research", "--slug", "moved", "--vault", root, "--json")

	renameOut := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.renamed=true", "fact.updated_notes=1", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename note metadata output missing %q:\n%s", want, renameOut)
		}
	}
	content := readCLIFile(t, filepath.Join(root, "spaces", "archive", "moved.md"))
	if !strings.Contains(content, "folder: spaces/archive") || strings.Contains(content, "folder: spaces/research") {
		t.Fatalf("folder rename did not update note frontmatter:\n%s", content)
	}
	listOut := runCLI(t, "note", "list", "--folder", "spaces/archive", "--vault", root, "--json")
	if !strings.Contains(listOut, `"total":"1"`) || !strings.Contains(listOut, "Moved Note") {
		t.Fatalf("folder rename did not refresh note list folder projection:\n%s", listOut)
	}
}

func TestNoteDeletePromptsInHumanModeCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Prompt Delete", "--body", "body", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(created), &envelope); err != nil {
		t.Fatalf("created note json invalid: %v\n%s", err, created)
	}
	path := envelope["facts"].(map[string]any)["path"].(string)
	if !fileExists(filepath.Join(root, filepath.FromSlash(path))) {
		t.Fatalf("created note missing: %s", path)
	}

	out := runCLIWithInput(t, "y\n", "note", "delete", path, "--vault", root)
	if !strings.Contains(out, "Confirm move to trash") || !strings.Contains(out, "Note moved to trash") {
		t.Fatalf("interactive delete output missing prompt or success:\n%s", out)
	}
	if fileExists(filepath.Join(root, filepath.FromSlash(path))) || !strings.Contains(out, ".pinax/trash/") {
		t.Fatalf("interactive delete did not move note to trash:\n%s", out)
	}

	cancelCreated := runCLI(t, "note", "new", "Cancel Delete", "--body", "body", "--vault", root, "--json")
	var cancelEnvelope map[string]any
	if err := json.Unmarshal([]byte(cancelCreated), &cancelEnvelope); err != nil {
		t.Fatalf("cancel note json invalid: %v\n%s", err, cancelCreated)
	}
	cancelPath := cancelEnvelope["facts"].(map[string]any)["path"].(string)
	cancelOut := runCLIWithInput(t, "n\n", "note", "delete", cancelPath, "--vault", root)
	if !strings.Contains(cancelOut, "Confirm move to trash") || !strings.Contains(cancelOut, "Canceled") {
		t.Fatalf("interactive cancel output missing prompt or cancel message:\n%s", cancelOut)
	}
	if !fileExists(filepath.Join(root, filepath.FromSlash(cancelPath))) {
		t.Fatalf("interactive cancel changed note")
	}

	second := runCLI(t, "note", "new", "Machine Delete", "--body", "body", "--vault", root, "--json")
	var secondEnvelope map[string]any
	if err := json.Unmarshal([]byte(second), &secondEnvelope); err != nil {
		t.Fatalf("second note json invalid: %v\n%s", err, second)
	}
	secondPath := secondEnvelope["facts"].(map[string]any)["path"].(string)
	jsonOut, err := runCLIExpectError("note", "delete", secondPath, "--vault", root, "--json")
	if err == nil || strings.Contains(jsonOut, "确认移入回收站") || !strings.Contains(jsonOut, "approval_required") {
		t.Fatalf("machine delete should require --yes without prompt: err=%v out=%s", err, jsonOut)
	}
	if !fileExists(filepath.Join(root, filepath.FromSlash(secondPath))) {
		t.Fatalf("machine delete without --yes changed note")
	}
}

func TestNoteCommandHardeningEditorAndOpenCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Editable", "--body", "body", "--vault", root, "--json")

	editorLog := filepath.Join(root, "editor-args.log")
	editor := writeFakeEditor(t, root, editorLog)
	editOut := runCLI(t, "note", "edit", "Editable", "--editor", editor+" --wait", "--vault", root, "--json")
	var editEnvelope map[string]any
	if err := json.Unmarshal([]byte(editOut), &editEnvelope); err != nil {
		t.Fatalf("edit json invalid: %v\n%s", err, editOut)
	}
	if editEnvelope["command"] != "note.edit" || editEnvelope["status"] != "success" {
		t.Fatalf("edit envelope = %#v", editEnvelope)
	}
	facts := editEnvelope["facts"].(map[string]any)
	if facts["editor_executable"] != editor || !strings.Contains(fmt.Sprint(facts["editor_args"]), "--wait") {
		t.Fatalf("editor facts = %#v", facts)
	}
	log := readCLIFile(t, editorLog)
	if !strings.Contains(log, "--wait") || !strings.Contains(log, "editable.md") {
		t.Fatalf("fake editor log missing args/path:\n%s", log)
	}

	newOpenOut := runCLI(t, "note", "new", "Open After Create", "--body", "body", "--open", "--editor", editor+" --wait", "--vault", root, "--json")
	if strings.Count(strings.TrimSpace(newOpenOut), "\n") != 0 {
		t.Fatalf("note new --open should emit one JSON envelope, got:\n%s", newOpenOut)
	}
	var newEnvelope map[string]any
	if err := json.Unmarshal([]byte(newOpenOut), &newEnvelope); err != nil {
		t.Fatalf("new --open json invalid: %v\n%s", err, newOpenOut)
	}
	if newEnvelope["command"] != "note.new" || newEnvelope["status"] != "success" {
		t.Fatalf("new --open envelope = %#v", newEnvelope)
	}
	newFacts := newEnvelope["facts"].(map[string]any)
	if newFacts["opened"] != "true" || newFacts["editor_executable"] != editor || !strings.Contains(fmt.Sprint(newFacts["editor_args"]), "--wait") {
		t.Fatalf("new --open facts = %#v", newFacts)
	}
	log = readCLIFile(t, editorLog)
	if !strings.Contains(log, "open-after-create.md") {
		t.Fatalf("fake editor log missing created note path:\n%s", log)
	}

	agentOut := runCLI(t, "note", "list", "--recent", "--vault", root, "--agent")
	for _, want := range []string{"command=note.list", "fact.recent=true", "fact.sort=updated"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("recent agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestNotebookCoreOutputContractAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Contract Note", "--body", "body", "--tags", "contract", "--vault", root, "--json")

	rootHelp := runCLI(t, "--help")
	for _, want := range []string{"journal", "inbox", "view", "import", "export", "index", "organize"} {
		if !strings.Contains(rootHelp, want) {
			t.Fatalf("root help missing %q:\n%s", want, rootHelp)
		}
	}
	for _, hidden := range []string{"\n  daily ", "\n  weekly ", "\n  monthly "} {
		if strings.Contains(rootHelp, hidden) {
			t.Fatalf("root help should hide compatibility command %q:\n%s", strings.TrimSpace(hidden), rootHelp)
		}
	}
	noteHelp := runCLI(t, "note", "--help")
	for _, want := range []string{"links", "backlinks", "orphans", "attach", "attachments"} {
		if !strings.Contains(noteHelp, want) {
			t.Fatalf("note help missing %q:\n%s", want, noteHelp)
		}
	}
	searchHelp := runCLI(t, "search", "--help")
	for _, want := range []string{"--link-target", "--has-attachment", "--allow-stale"} {
		if !strings.Contains(searchHelp, want) {
			t.Fatalf("search help missing %q:\n%s", want, searchHelp)
		}
	}
	indexHelp := runCLI(t, "index", "--help")
	for _, want := range []string{"status", "refresh", "doctor", "rebuild", "sync", "repair", "low-cost maintenance", "full reset", "pinax index", "pinax index refresh --vault ./my-notes", "pinax index doctor --vault ./my-notes", "pinax index rebuild --vault ./my-notes"} {
		if !strings.Contains(indexHelp, want) {
			t.Fatalf("index help missing %q:\n%s", want, indexHelp)
		}
	}
	commandsStart := strings.Index(indexHelp, "Available Commands")
	commandsEnd := strings.Index(indexHelp, "Flags")
	if commandsStart < 0 || commandsEnd <= commandsStart {
		t.Fatalf("index help command section not found:\n%s", indexHelp)
	}
	commandSection := indexHelp[commandsStart:commandsEnd]
	last := -1
	for _, want := range []string{"status", "refresh", "doctor", "rebuild", "sync", "repair"} {
		pos := strings.Index(commandSection, want)
		if pos <= last {
			t.Fatalf("index help command %q is out of workflow order:\n%s", want, indexHelp)
		}
		last = pos
	}
	organizeHelp := runCLI(t, "organize", "--help")
	for _, want := range []string{"plan", "list", "apply"} {
		if !strings.Contains(organizeHelp, want) {
			t.Fatalf("organize help missing %q:\n%s", want, organizeHelp)
		}
	}

	humanOut := runCLI(t, "view", "list", "--vault", root)
	if !strings.Contains(humanOut, "Highlights") || strings.Contains(humanOut, "状态:") || strings.Contains(humanOut, "成功") || strings.HasPrefix(strings.TrimSpace(humanOut), "{") {
		t.Fatalf("view list human output invalid:\n%s", humanOut)
	}
	agentOut, err := runCLIExpectError("import", "markdown", filepath.Join(root, "missing"), "--dry-run", "--vault", root, "--agent")
	if err == nil {
		t.Fatalf("missing import source succeeded: %s", agentOut)
	}
	for _, want := range []string{"mode=agent", "command=import.markdown", "status=failed", "error.code=import_source_missing"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("import error agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, "状态:") || strings.Contains(agentOut, "重点:") || strings.Contains(agentOut, "raw") {
		t.Fatalf("agent output leaked prose/debug text:\n%s", agentOut)
	}
	jsonOut, err := runCLIExpectError("note", "attach", "Contract Note", filepath.Join(root, "missing.png"), "--vault", root, "--json")
	if err == nil {
		t.Fatalf("missing attachment succeeded: %s", jsonOut)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("attachment error json invalid: %v\n%s", err, jsonOut)
	}
	errorObject := envelope["error"].(map[string]any)
	if envelope["command"] != "note.attach" || envelope["status"] != "failed" || errorObject["code"] != "attachment_source_missing" {
		t.Fatalf("attachment error envelope = %#v", envelope)
	}
}
