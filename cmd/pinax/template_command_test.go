package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateAuthoringCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	designOut := runCLI(t, "template", "create", "视频学习", "--vault", root, "--json")
	var designEnvelope map[string]any
	if err := json.Unmarshal([]byte(designOut), &designEnvelope); err != nil {
		t.Fatalf("design create json invalid: %v\n%s", err, designOut)
	}
	if designEnvelope["command"] != "template.create" || designEnvelope["status"] != "success" {
		t.Fatalf("design create envelope = %#v", designEnvelope)
	}
	designContent := readCLIFile(t, filepath.Join(root, ".pinax", "templates", "视频学习.md"))
	for _, want := range []string{"schema_version: pinax.template_design.v1", "kind: template_design", "title: 视频学习"} {
		if !strings.Contains(designContent, want) {
			t.Fatalf("template design missing %q:\n%s", want, designContent)
		}
	}

	source := filepath.Join(root, "meeting-template.md")
	writeCLIFixture(t, source, "# {{title}}\n客户: {{client}}\n")

	createOut := runCLI(t, "template", "create", "meeting", "--from", source, "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(createOut), &createEnvelope); err != nil {
		t.Fatalf("create json invalid: %v\n%s", err, createOut)
	}
	if createEnvelope["command"] != "template.create" || createEnvelope["status"] != "success" {
		t.Fatalf("create envelope = %#v", createEnvelope)
	}

	renderOut := runCLI(t, "template", "render", "meeting", "--title", "客户会议", "--var", "client=Acme", "--vault", root, "--json")
	if !strings.Contains(renderOut, "客户: Acme") {
		t.Fatalf("render output missing custom var:\n%s", renderOut)
	}

	validateOut := runCLI(t, "template", "validate", "meeting", "--vault", root, "--agent")
	for _, want := range []string{"command=template.validate", "status=success", "fact.issues=0"} {
		if !strings.Contains(validateOut, want) {
			t.Fatalf("validate output missing %q:\n%s", want, validateOut)
		}
	}

	noteOut := runCLI(t, "note", "new", "客户会议", "--template", "meeting", "--var", "client=Acme", "--tags", "meeting,client", "--vault", root, "--json")
	if !strings.Contains(noteOut, "note.new") {
		t.Fatalf("note output = %s", noteOut)
	}
	content := readCLIFile(t, filepath.Join(root, "客户会议.md"))
	if !strings.Contains(content, "客户: Acme") {
		t.Fatalf("note content = %s", content)
	}

	failed, err := runCLIExpectError("template", "delete", "meeting", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("delete without approval err=%v out=%s", err, failed)
	}
	deleteOut := runCLI(t, "template", "delete", "meeting", "--vault", root, "--yes", "--json")
	if !strings.Contains(deleteOut, "template.delete") {
		t.Fatalf("delete output = %s", deleteOut)
	}
}

func TestTemplateInspectPreviewOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "go-meeting", "--engine", "go-template", "--body", "# {{ .Title | upper }}\n客户: {{ .Vars.client }}\n", "--vault", root, "--json")

	inspectJSON, inspectStderr, inspectErr := runCLISeparate("template", "inspect", "go-meeting", "--vault", root, "--json")
	if inspectErr != nil || inspectStderr != "" {
		t.Fatalf("inspect json failed: err=%v stderr=%q stdout=%s", inspectErr, inspectStderr, inspectJSON)
	}
	var inspectEnvelope map[string]any
	if err := json.Unmarshal([]byte(inspectJSON), &inspectEnvelope); err != nil {
		t.Fatalf("inspect json invalid: %v\n%s", err, inspectJSON)
	}
	if inspectEnvelope["command"] != "template.inspect" || inspectEnvelope["mode"] != "json" || inspectEnvelope["status"] != "success" {
		t.Fatalf("inspect envelope = %#v", inspectEnvelope)
	}
	inspectFacts := inspectEnvelope["facts"].(map[string]any)
	if inspectFacts["template"] != "go-meeting" || inspectFacts["engine"] != "go-template" || inspectFacts["schema_version"] != "pinax.template.v2" {
		t.Fatalf("inspect facts = %#v", inspectFacts)
	}

	previewOut := runCLI(t, "template", "preview", "go-meeting", "--title", "weekly", "--var", "client=Acme", "--vault", root, "--agent")
	for _, want := range []string{"command=template.preview", "mode=agent", "status=success", "fact.template=go-meeting", "fact.engine=go-template"} {
		if !strings.Contains(previewOut, want) {
			t.Fatalf("preview agent output missing %q:\n%s", want, previewOut)
		}
	}

	stdout, stderr, err := runCLISeparate("template", "preview", "go-meeting", "--title", "weekly", "--var", "client=Acme", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("preview json stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var previewEnvelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &previewEnvelope); err != nil {
		t.Fatalf("preview json invalid: %v\n%s", err, stdout)
	}
	data := previewEnvelope["data"].(map[string]any)
	if body, _ := data["body"].(string); !strings.Contains(body, "# WEEKLY") || !strings.Contains(body, "客户: Acme") {
		t.Fatalf("preview body = %#v", data["body"])
	}
}

func TestPreviewSummaryShowsBodyAndTags(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "brief", "--body", "# {{title}}\nPreview body", "--vault", root, "--json")

	templateSummary := runCLI(t, "template", "preview", "brief", "--title", "Weekly", "--tags", "research,client", "--vault", root)
	for _, want := range []string{"Template preview generated", "Tags", "research,client", "# Weekly", "Preview body"} {
		if !strings.Contains(templateSummary, want) {
			t.Fatalf("template preview summary missing %q:\n%s", want, templateSummary)
		}
	}

	runCLI(t, "note", "new", "Tagged Preview", "--tags", "research,client", "--body", "preview body", "--vault", root, "--json")
	noteSummary := runCLI(t, "note", "preview", "Tagged Preview", "--vault", root)
	for _, want := range []string{"Local note read", "Tags", "research,client", "Tagged Preview", "preview body"} {
		if !strings.Contains(noteSummary, want) {
			t.Fatalf("note preview summary missing %q:\n%s", want, noteSummary)
		}
	}
}

func TestTemplateQueryBackedCLIOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "B", "--body", "priority:: 2\n", "--status", "done", "--vault", root, "--json")

	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"kind: note",
		"queries:",
		"  active:",
		"    language: sql",
		"    sql: SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"    kind: table",
		"    max_rows: 5",
		"    required: true",
		"---",
		"# Active",
		"{{ table .Queries.active }}",
	}, "\n")
	runCLI(t, "template", "create", "active-report", "--body", body, "--vault", root, "--json")

	inspectJSON, inspectStderr, inspectErr := runCLISeparate("template", "inspect", "active-report", "--vault", root, "--json")
	if inspectErr != nil || inspectStderr != "" {
		t.Fatalf("inspect query json err=%v stderr=%q stdout=%s", inspectErr, inspectStderr, inspectJSON)
	}
	var inspectEnvelope map[string]any
	if err := json.Unmarshal([]byte(inspectJSON), &inspectEnvelope); err != nil {
		t.Fatalf("inspect query json invalid: %v\n%s", err, inspectJSON)
	}
	facts := inspectEnvelope["facts"].(map[string]any)
	if facts["queries"] != "1" || facts["engine"] != "go-template" || !strings.Contains(inspectJSON, "query_explain") {
		t.Fatalf("inspect query envelope = %#v", inspectEnvelope)
	}

	previewAgent := runCLI(t, "template", "preview", "active-report", "--title", "Active Report", "--vault", root, "--agent")
	for _, want := range []string{"command=template.preview", "mode=agent", "status=success", "fact.template=active-report", "fact.engine=go-template"} {
		if !strings.Contains(previewAgent, want) {
			t.Fatalf("preview query agent missing %q:\n%s", want, previewAgent)
		}
	}
	assertMachineOutputClean(t, previewAgent)

	previewJSON := runCLI(t, "template", "preview", "active-report", "--title", "Active Report", "--vault", root, "--json")
	var previewEnvelope map[string]any
	if err := json.Unmarshal([]byte(previewJSON), &previewEnvelope); err != nil {
		t.Fatalf("preview query json invalid: %v\n%s", err, previewJSON)
	}
	data := previewEnvelope["data"].(map[string]any)
	bodyOut, _ := data["body"].(string)
	if !strings.Contains(bodyOut, "| title | status |") || !strings.Contains(bodyOut, "| A | active |") || strings.Contains(bodyOut, "| B | done |") {
		t.Fatalf("preview query body = %q", bodyOut)
	}
}

func TestRenderRunSnapshotAndPruneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "study", "--engine", "go-template", "--body", "# {{ .Title }}\nURL: {{ .Vars.url }}\n", "--vault", root, "--json")

	saveOut := runCLI(t, "template", "render", "study", "--title", "Go", "--var", "url=https://go.dev", "--save-run", "go-run", "--vault", root, "--json")
	if !strings.Contains(saveOut, `"run_saved":"true"`) || !strings.Contains(saveOut, `"run_name":"go-run"`) {
		t.Fatalf("save run out = %s", saveOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "renders", "templates", "study", "index.json")); err != nil {
		t.Fatalf("template render index missing: %v", err)
	}

	reuseOut := runCLI(t, "template", "render", "study", "--run", "go-run", "--vault", root, "--json")
	if !strings.Contains(reuseOut, "URL: https://go.dev") || !strings.Contains(reuseOut, `"run":"go-run"`) {
		t.Fatalf("reuse run out = %s", reuseOut)
	}
	inspectRuns := runCLI(t, "template", "inspect", "study", "--runs", "--vault", root, "--json")
	if !strings.Contains(inspectRuns, "go-run") || !strings.Contains(inspectRuns, "render_runs") {
		t.Fatalf("inspect runs = %s", inspectRuns)
	}
	templateRunCompletion := runCLI(t, "__complete", "template", "render", "study", "--vault", root, "--run", "")
	if !strings.Contains(templateRunCompletion, "go-run	render-run") || !strings.Contains(templateRunCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template run completion = %s", templateRunCompletion)
	}

	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	dashboardBody := strings.Join([]string{
		"```pinax-sql active",
		"SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"```",
		"<!-- pinax:render active start -->",
		"empty",
		"<!-- pinax:render active end -->",
	}, "\n")
	runCLI(t, "note", "new", "Dashboard", "--body", dashboardBody, "--status", "active", "--vault", root, "--json")
	refreshOut := runCLI(t, "note", "refresh", "Dashboard", "--rendered", "--save-run", "dash", "--yes", "--vault", root, "--json")
	if !strings.Contains(refreshOut, `"run_saved":"true"`) || !strings.Contains(refreshOut, `"run_name":"dash"`) {
		t.Fatalf("refresh save run = %s", refreshOut)
	}
	snapshotOut := runCLI(t, "note", "show", "Dashboard", "--view", "rendered", "--snapshot", "latest", "--vault", root, "--json")
	if !strings.Contains(snapshotOut, "| A | active |") || !strings.Contains(snapshotOut, `"snapshot":"latest"`) {
		t.Fatalf("snapshot out = %s", snapshotOut)
	}
	snapshotCompletion := runCLI(t, "__complete", "note", "show", "Dashboard", "--view", "rendered", "--vault", root, "--snapshot", "")
	if !strings.Contains(snapshotCompletion, "dash	render-run") || !strings.Contains(snapshotCompletion, "latest	render-run") || !strings.Contains(snapshotCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("snapshot completion = %s", snapshotCompletion)
	}

	pruneDry := runCLI(t, "template", "runs", "prune", "study", "--keep", "0", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(pruneDry, `"dry_run":"true"`) || !strings.Contains(pruneDry, `"delete_candidates":"`) {
		t.Fatalf("prune dry = %s", pruneDry)
	}
	repairOut := runCLI(t, "template", "runs", "repair", "--vault", root, "--json")
	if !strings.Contains(repairOut, "template.runs.repair") {
		t.Fatalf("repair out = %s", repairOut)
	}
}

func TestTemplateAuthoringCLIRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("template", "create", "../bad", "--body", "x", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "invalid_template_name") {
		t.Fatalf("unsafe template create err=%v out=%s", err, out)
	}
	out, err = runCLIExpectError("template", "render", "note", "--var", "bad", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "template_variable_invalid") {
		t.Fatalf("bad var err=%v out=%s", err, out)
	}
}

func TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	starter := runCLI(t, "template", "list", "--pack", "starter", "--vault", root, "--json")
	if !strings.Contains(starter, "note.quick") || !strings.Contains(starter, `"filter.pack":"starter"`) {
		t.Fatalf("template list starter output missing starter facts:\n%s", starter)
	}
	meetings := runCLI(t, "template", "list", "--use-case", "meeting", "--vault", root, "--json")
	if !strings.Contains(meetings, "meeting.notes") || !strings.Contains(meetings, `"filter.use_case":"meeting"`) {
		t.Fatalf("template list use-case output missing meeting facts:\n%s", meetings)
	}
	recommended := runCLI(t, "template", "recommend", "--intent", "meeting", "--vault", root, "--json")
	if !strings.Contains(recommended, `"command":"template.recommend"`) || !strings.Contains(recommended, `"primary":"meeting.notes"`) {
		t.Fatalf("template recommend meeting output = %s", recommended)
	}
	fallback := runCLI(t, "template", "recommend", "--intent", "unknown-intent", "--vault", root, "--json")
	if !strings.Contains(fallback, `"primary":"note.quick"`) && !strings.Contains(fallback, `"primary":"inbox.capture"`) {
		t.Fatalf("template recommend fallback output = %s", fallback)
	}
}

func TestTemplateCompletionJournalTemplateCompletionIndexTemplateCompletionNoteTemplateCompletion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	inspect := runCLI(t, "__complete", "template", "inspect", "--vault", root, "")
	for _, want := range []string{"journal.daily\tbuiltin journal_template", "index.home\tbuiltin index_template", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(inspect, want) {
			t.Fatalf("template inspect completion missing %q:\n%s", want, inspect)
		}
	}
	journal := runCLI(t, "__complete", "journal", "daily", "show", "--vault", root, "--template", "")
	if !strings.Contains(journal, "journal.daily\tbuiltin journal_template") || !strings.Contains(journal, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("journal template completion = %s", journal)
	}
	index := runCLI(t, "__complete", "index", "page", "preview", "home", "--vault", root, "--template", "")
	if !strings.Contains(index, "index.home\tbuiltin index_template") || !strings.Contains(index, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("index template completion = %s", index)
	}
	note := runCLI(t, "__complete", "note", "add", "Demo", "--vault", root, "--template", "")
	if !strings.Contains(note, "meeting.notes\tbuiltin note_template") || !strings.Contains(note, "note.quick\tbuiltin note_template") {
		t.Fatalf("note template completion = %s", note)
	}
	deleteOut := runCLI(t, "__complete", "template", "delete", "--vault", root, "")
	if strings.Contains(deleteOut, "journal.daily") || !strings.Contains(deleteOut, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template delete should only complete local templates:\n%s", deleteOut)
	}
}

func TestTemplateFlagCompletionTemplateVarCompletionTemplateRunCompletion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	engine := runCLI(t, "__complete", "template", "create", "demo", "--engine", "")
	if !strings.Contains(engine, "simple\tengine") || !strings.Contains(engine, "go-template\tengine") {
		t.Fatalf("template engine completion = %s", engine)
	}
	body := strings.Join([]string{"---", "schema_version: pinax.template.v2", "engine: go-template", "variables:", "  url:", "    required: true", "---", "# {{ .Vars.url }}"}, "\n")
	runCLI(t, "template", "create", "video-study", "--body", body, "--vault", root, "--json")
	vars := runCLI(t, "__complete", "template", "render", "video-study", "--vault", root, "--var", "")
	if !strings.Contains(vars, "url=\trequired string") || !strings.Contains(vars, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template var completion = %s", vars)
	}
	runCLI(t, "template", "render", "video-study", "--var", "url=https://example.test", "--save-run", "video-run", "--vault", root, "--json")
	runs := runCLI(t, "__complete", "template", "render", "video-study", "--vault", root, "--run", "")
	if !strings.Contains(runs, "video-run") || !strings.Contains(runs, "render-run") || !strings.Contains(runs, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template run completion = %s", runs)
	}
}
