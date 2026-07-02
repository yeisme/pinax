package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	for _, want := range []string{"preview body"} {
		if !strings.Contains(noteSummary, want) {
			t.Fatalf("note preview summary missing %q:\n%s", want, noteSummary)
		}
	}
	for _, forbidden := range []string{"Highlights", "Local note read", "Tags", "research,client"} {
		if strings.Contains(noteSummary, forbidden) {
			t.Fatalf("note preview should render body only, found %q:\n%s", forbidden, noteSummary)
		}
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "empty-preview.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_empty_preview\ntitle: Empty Preview\ntags: []\n---\n")
	emptySummary := runCLI(t, "note", "preview", "Empty Preview", "--vault", root)
	if strings.TrimSpace(emptySummary) != "" {
		t.Fatalf("empty note preview should be silent on success:\n%s", emptySummary)
	}
	emptyJSON := runCLI(t, "note", "preview", "Empty Preview", "--vault", root, "--json")
	if !strings.Contains(emptyJSON, `"command":"note.preview"`) || !strings.Contains(emptyJSON, `"status":"success"`) {
		t.Fatalf("empty note preview json should keep machine envelope:\n%s", emptyJSON)
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
	anime := runCLI(t, "template", "recommend", "--intent", "动漫", "--vault", root, "--json")
	if !strings.Contains(anime, `"primary":"idea.anime_watch"`) && !strings.Contains(anime, `"primary":"media.anime"`) {
		t.Fatalf("template recommend anime output = %s", anime)
	}
	paper := runCLI(t, "template", "recommend", "--intent", "论文", "--vault", root, "--json")
	if !strings.Contains(paper, `"primary":"idea.paper_read"`) && !strings.Contains(paper, `"primary":"reading.paper"`) {
		t.Fatalf("template recommend paper output = %s", paper)
	}
	novel := runCLI(t, "template", "recommend", "--intent", "写小说", "--vault", root, "--json")
	if !strings.Contains(novel, `"primary":"idea.novel_write"`) && !strings.Contains(novel, `"primary":"writing.novel"`) {
		t.Fatalf("template recommend novel writing output = %s", novel)
	}
	sticky := runCLI(t, "template", "recommend", "--intent", "便签", "--vault", root, "--json")
	if !strings.Contains(sticky, `"command":"template.recommend"`) || !strings.Contains(sticky, `"primary":"sticky.capture"`) {
		t.Fatalf("template recommend sticky output = %s", sticky)
	}
	projectSignal := runCLI(t, "template", "recommend", "--intent", "子项目看板线索", "--vault", root, "--json")
	if !strings.Contains(projectSignal, `"primary":"sticky.project_signal"`) {
		t.Fatalf("template recommend project signal output = %s", projectSignal)
	}
	stockIndicator := runCLI(t, "template", "recommend", "--intent", "K线", "--vault", root, "--json")
	if !strings.Contains(stockIndicator, `"primary":"learning.stock.indicator"`) {
		t.Fatalf("template recommend stock indicator output = %s", stockIndicator)
	}
	stockRisk := runCLI(t, "template", "recommend", "--intent", "风险规则", "--vault", root, "--json")
	if !strings.Contains(stockRisk, `"primary":"learning.stock.risk_rule"`) {
		t.Fatalf("template recommend stock risk output = %s", stockRisk)
	}
	fallback := runCLI(t, "template", "recommend", "--intent", "unknown-intent", "--vault", root, "--json")
	if !strings.Contains(fallback, `"primary":"note.quick"`) && !strings.Contains(fallback, `"primary":"inbox.capture"`) {
		t.Fatalf("template recommend fallback output = %s", fallback)
	}
}

func TestTemplateRecommendWorkflowFields(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "template", "recommend", "--intent", "meeting", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("recommend json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "template.recommend" || envelope["status"] != "success" {
		t.Fatalf("recommend envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	recommendations := data["recommendations"].([]any)
	if len(recommendations) == 0 || len(recommendations) > 4 {
		t.Fatalf("recommendations length = %d: %#v", len(recommendations), recommendations)
	}
	primary := recommendations[0].(map[string]any)
	for _, key := range []string{"scenario_id", "maturity", "pack", "fit_reason", "preview_command", "create_command", "proof_gate", "after_create_actions"} {
		if _, ok := primary[key]; !ok {
			t.Fatalf("primary recommendation missing %s: %#v", key, primary)
		}
	}
	if primary["template"] != "meeting.notes" || primary["scenario_id"] != "meeting-decision" || !strings.Contains(primary["preview_command"].(string), "pinax template preview meeting.notes") || !strings.Contains(primary["create_command"].(string), "pinax note add") {
		t.Fatalf("primary workflow recommendation = %#v", primary)
	}
	if _, ok := data["primary"]; !ok {
		t.Fatalf("recommendation removed legacy primary field: %#v", data)
	}

	agent := runCLI(t, "template", "recommend", "--intent", "meeting", "--vault", root, "--agent")
	for _, want := range []string{"command=template.recommend", "fact.primary=meeting.notes", "recommendation.0.template=meeting.notes", "recommendation.0.scenario_id=meeting-decision", "recommendation.0.proof_gate="} {
		if !strings.Contains(agent, want) {
			t.Fatalf("recommend agent missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)
}

func TestTemplateInspectWorkflowFields(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "template", "inspect", "meeting.notes", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("inspect json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	for _, key := range []string{"template", "template_kind", "scenario_id", "maturity", "pack", "lifecycle", "source"} {
		if facts[key] == "" || facts[key] == nil {
			t.Fatalf("inspect facts missing %s: %#v", key, facts)
		}
	}
	data := envelope["data"].(map[string]any)
	workflow := data["workflow"].(map[string]any)
	if workflow["scenario_id"] != "meeting-decision" || workflow["lifecycle"] != "published_executable" {
		t.Fatalf("inspect workflow = %#v", workflow)
	}
	for _, key := range []string{"variable_schema", "output_policy", "proof_gate", "after_create_actions"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("inspect data missing %s: %#v", key, data)
		}
	}
}

func TestTemplatePreviewWorkflowFields(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	before := listVaultFilesForTemplateTest(t, root)
	out := runCLI(t, "template", "preview", "meeting.notes", "--title", "Client Meeting", "--vault", root, "--json")
	after := listVaultFilesForTemplateTest(t, root)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("template preview wrote files\nbefore=%#v\nafter=%#v", before, after)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("preview json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["read_only"] != "true" || facts["writes"] != "false" || facts["scenario_id"] != "meeting-decision" {
		t.Fatalf("preview facts = %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	for _, key := range []string{"workflow", "output_policy", "proof_gate", "write_impact", "body_exposure", "next_command"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("preview data missing %s: %#v", key, data)
		}
	}
	if !strings.Contains(data["next_command"].(string), "pinax note add") {
		t.Fatalf("preview next command = %#v", data["next_command"])
	}
}

func TestTemplatePackAndLifecycleCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	starter := runCLI(t, "template", "list", "--pack", "starter", "--vault", root, "--json")
	if !strings.Contains(starter, `"pack":{"id":"starter"`) || !strings.Contains(starter, "sticky.capture") || strings.Contains(starter, "meeting.notes") {
		t.Fatalf("starter pack output = %s", starter)
	}

	draft := strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: note_template", "name: meeting.draft", "title: Meeting Draft", "engine: go-template", "scenario_id: meeting-decision", "intents: [meeting]", "lifecycle: draft_design", "pack:", "  id: local-workflows", "  source: vault-local", "output:", "  path_pattern: drafts/{{ .Title }}.md", "defaults:", "  kind: meeting", "  status: draft", "---", "# {{ .Title }}"}, "\n")
	runCLI(t, "template", "create", "meeting.draft", "--body", draft, "--vault", root, "--json")
	out := runCLI(t, "template", "recommend", "--intent", "meeting", "--vault", root, "--json")
	if strings.Contains(out, `"primary":{"name":"meeting.draft"`) || !strings.Contains(out, `"lifecycle":"draft_design"`) || !strings.Contains(out, `"executable":false`) {
		t.Fatalf("draft lifecycle recommendation output = %s", out)
	}

	local := strings.Join([]string{"---", "schema_version: pinax.template.v2", "kind: note_template", "name: meeting.notes", "title: Local Meeting", "engine: go-template", "pack:", "  id: local-workflows", "  source: vault-local", "output:", "  path_pattern: local/{{ .Title }}.md", "defaults:", "  kind: meeting", "  status: active", "---", "# {{ .Title }}"}, "\n")
	runCLI(t, "template", "create", "meeting.notes", "--body", local, "--overwrite", "--vault", root, "--json")
	inspect := runCLI(t, "template", "inspect", "meeting.notes", "--vault", root, "--json")
	if !strings.Contains(inspect, `"source":"vault-local"`) || !strings.Contains(inspect, `"lifecycle":"overridden"`) || !strings.Contains(inspect, `"pack":"local-workflows"`) {
		t.Fatalf("local override inspect = %s", inspect)
	}
}

func TestTemplateUseEvidence(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "note", "add", "Client Meeting", "--template", "meeting.notes", "--dir", "index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note add template json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "note.new" || envelope["status"] != "success" {
		t.Fatalf("note add template envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{
		"template":          "meeting.notes",
		"template_pack":     "focused",
		"scenario_id":       "meeting-decision",
		"proof_gate.status": "review_optional",
	} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; facts=%#v", key, facts[key], want, facts)
		}
	}
	if facts["template_use_id"] == "" || facts["effective_path"] == "" {
		t.Fatalf("template use facts missing id/path: %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	use, ok := data["template_use"].(map[string]any)
	if !ok {
		t.Fatalf("template_use missing from data: %#v", data)
	}
	for _, key := range []string{"template_use_id", "template", "template_pack", "scenario_id", "effective_path", "proof_gate", "next_actions"} {
		if _, ok := use[key]; !ok {
			t.Fatalf("template_use missing %s: %#v", key, use)
		}
	}
	if strings.Contains(out, "raw prompt") || strings.Contains(out, "Authorization: Bearer") {
		t.Fatalf("template use output leaked forbidden content:\n%s", out)
	}
}

func listVaultFilesForTemplateTest(t *testing.T, root string) []string {
	t.Helper()
	files := []string{}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("walk vault: %v", err)
	}
	sort.Strings(files)
	return files
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
