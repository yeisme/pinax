package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "pinax dev") {
		t.Fatalf("version output = %q", got)
	}
}

func TestLocalVaultCLIJSONAndSafety(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Inbox Note.md"), "# Inbox Note\n\nbody\n")

	out := runCLI(t, "organize", "plan", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "success" || envelope["mode"] != "json" {
		t.Fatalf("envelope = %#v", envelope)
	}

	errOut, err := runCLIExpectError("organize", "apply", "--vault", root, "--yes", "--json")
	if err == nil {
		t.Fatalf("organize apply without snapshot succeeded: %s", errOut)
	}
	if !strings.Contains(errOut, "snapshot_required") {
		t.Fatalf("expected snapshot_required envelope, got %s", errOut)
	}
}

func TestProjectAndStorageCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	projectOut := runCLI(t, "project", "create", "research", "--name", "研究", "--description", "长期研究", "--notes-prefix", "notes/research", "--vault", root, "--json")
	var projectEnvelope map[string]any
	if err := json.Unmarshal([]byte(projectOut), &projectEnvelope); err != nil {
		t.Fatalf("project json invalid: %v\n%s", err, projectOut)
	}
	if projectEnvelope["command"] != "project.create" || projectEnvelope["status"] != "success" {
		t.Fatalf("project envelope = %#v", projectEnvelope)
	}

	listOut := runCLI(t, "project", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=project.list", "fact.projects=1", "fact.current_project=research"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("project agent output missing %q:\n%s", want, listOut)
		}
	}

	storageOut := runCLI(t, "storage", "set-s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--profile", "work", "--vault", root, "--json")
	if strings.Contains(strings.ToLower(storageOut), "secret") || strings.Contains(strings.ToLower(storageOut), "access_key") {
		t.Fatalf("storage output leaked secret-like material:\n%s", storageOut)
	}
	var storageEnvelope map[string]any
	if err := json.Unmarshal([]byte(storageOut), &storageEnvelope); err != nil {
		t.Fatalf("storage json invalid: %v\n%s", err, storageOut)
	}
	if storageEnvelope["command"] != "storage.set_s3" || storageEnvelope["status"] != "success" {
		t.Fatalf("storage envelope = %#v", storageEnvelope)
	}
}

func TestStorageSetS3RequiresBucketAndRegion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault")
	out, err := runCLIExpectError("storage", "set-s3", "--bucket", "notes", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("storage set-s3 without region succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("storage error json invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "storage.set_s3" {
		t.Fatalf("storage error envelope = %#v", envelope)
	}
}

func TestCoreMVPCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "研究", "--notes-prefix", "notes/research", "--vault", root, "--json")
	runCLI(t, "template", "init", "--vault", root, "--json")

	noteOut := runCLI(t, "note", "new", "研究日志", "--project", "research", "--tags", "pinax,sync", "--template", "mermaid", "--vault", root, "--json")
	var noteEnvelope map[string]any
	if err := json.Unmarshal([]byte(noteOut), &noteEnvelope); err != nil {
		t.Fatalf("note json invalid: %v\n%s", err, noteOut)
	}
	if noteEnvelope["command"] != "note.new" || noteEnvelope["status"] != "success" {
		t.Fatalf("note envelope = %#v", noteEnvelope)
	}

	indexOut := runCLI(t, "index", "rebuild", "--vault", root, "--agent")
	for _, want := range []string{"command=index.rebuild", "fact.notes=1"} {
		if !strings.Contains(indexOut, want) {
			t.Fatalf("index output missing %q:\n%s", want, indexOut)
		}
	}

	searchOut := runCLI(t, "search", "研究日志", "--vault", root, "--agent")
	for _, want := range []string{"command=note.search", "fact.matches=1", "fact.engine="} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("search output missing %q:\n%s", want, searchOut)
		}
	}

	syncOut := runCLI(t, "sync", "diff", "--target", "cloud", "--vault", root, "--json")
	var syncEnvelope map[string]any
	if err := json.Unmarshal([]byte(syncOut), &syncEnvelope); err != nil {
		t.Fatalf("sync json invalid: %v\n%s", err, syncOut)
	}
	if syncEnvelope["command"] != "sync.diff" || syncEnvelope["status"] != "partial" {
		t.Fatalf("sync envelope = %#v", syncEnvelope)
	}
	failed, err := runCLIExpectError("sync", "push", "--target", "cloud", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("sync push without approval got err=%v out=%s", err, failed)
	}
}

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
	content := readCLIFile(t, filepath.Join(root, "notes", "客户会议.md"))
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

func TestVaultStatsDoctorAndDashboardCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\ntags: [pinax]\n---\n\n# Active\n\nbody\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw\n\n")

	statsOut := runCLI(t, "stats", "--vault", root, "--json")
	var statsEnvelope map[string]any
	if err := json.Unmarshal([]byte(statsOut), &statsEnvelope); err != nil {
		t.Fatalf("stats json invalid: %v\n%s", err, statsOut)
	}
	if statsEnvelope["command"] != "vault.stats" || statsEnvelope["status"] != "success" || statsEnvelope["mode"] != "json" {
		t.Fatalf("stats envelope = %#v", statsEnvelope)
	}
	facts, ok := statsEnvelope["facts"].(map[string]any)
	if !ok || facts["notes"] != "2" || facts["index_status"] != "missing" {
		t.Fatalf("stats facts = %#v", statsEnvelope["facts"])
	}

	statsHuman := runCLI(t, "stats", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "状态", "重点", "success", "Vault 统计已生成。", "指标", "值", "notes", "2"} {
		if !strings.Contains(statsHuman, want) {
			t.Fatalf("stats human output missing %q:\n%s", want, statsHuman)
		}
	}
	for _, old := range []string{"状态:", "重点:", "事实:", "notes=2"} {
		if strings.Contains(statsHuman, old) {
			t.Fatalf("stats human output still uses label prose %q:\n%s", old, statsHuman)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(statsHuman), "{") {
		t.Fatalf("stats human output looks like JSON:\n%s", statsHuman)
	}

	doctorJSON := runCLI(t, "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorJSON), &doctorEnvelope); err != nil {
		t.Fatalf("doctor json invalid: %v\n%s", err, doctorJSON)
	}
	if doctorEnvelope["command"] != "vault.doctor" || doctorEnvelope["status"] != "partial" || doctorEnvelope["mode"] != "json" {
		t.Fatalf("doctor envelope = %#v", doctorEnvelope)
	}

	doctorAgent := runCLI(t, "doctor", "--vault", root, "--agent")
	for _, want := range []string{"command=vault.doctor", "status=partial", "fact.issues.total=", "issue.1.code="} {
		if !strings.Contains(doctorAgent, want) {
			t.Fatalf("doctor agent output missing %q:\n%s", want, doctorAgent)
		}
	}
	if strings.Contains(doctorAgent, "状态:") || strings.Contains(doctorAgent, "重点:") {
		t.Fatalf("doctor agent output contains human prose:\n%s", doctorAgent)
	}

	dashboardOut, dashboardErr := runDashboardUntilCanceled(t, root)
	if dashboardOut != "" {
		t.Fatalf("dashboard wrote stdout: %q", dashboardOut)
	}
	if !strings.Contains(dashboardErr, "http://127.0.0.1:") {
		t.Fatalf("dashboard stderr missing URL:\n%s", dashboardErr)
	}

	help := runCLI(t, "--help")
	for _, want := range []string{"stats", "doctor", "dashboard", "Markdown vault"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestRepairPlanJSONIsReadonlyAndSaveWritesPlanAsset(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "No Tags.md")
	writeCLIFixture(t, notePath, "# No Tags\n\nbody\n")
	beforeNote := readCLIFile(t, notePath)

	out := runCLI(t, "repair", "plan", "--vault", root, "--json")
	if got := readCLIFile(t, notePath); got != beforeNote {
		t.Fatalf("repair plan modified markdown:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "repair-plans")) {
		t.Fatalf("repair plan without --save wrote repair-plans asset")
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("repair plan json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "repair.plan" || envelope["status"] != "partial" || envelope["mode"] != "json" {
		t.Fatalf("repair plan envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.repair_plan.v1" || data["plan_id"] == "" {
		t.Fatalf("repair plan identity missing: %#v", data)
	}
	if len(data["operations"].([]any)) == 0 {
		t.Fatalf("repair plan operations missing: %#v", data)
	}
	if len(envelope["actions"].([]any)) == 0 {
		t.Fatalf("repair plan next actions missing: %#v", envelope)
	}

	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	savedData := savedEnvelope["data"].(map[string]any)
	savedPath := savedData["saved_path"].(string)
	if savedPath == "" || !strings.HasPrefix(savedPath, ".pinax/repair-plans/") {
		t.Fatalf("saved path not relative repair plan asset: %#v", savedData)
	}
	planContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(savedPath)))
	var savedPlan map[string]any
	if err := json.Unmarshal([]byte(planContent), &savedPlan); err != nil {
		t.Fatalf("saved repair plan invalid json: %v\n%s", err, planContent)
	}
	if savedPlan["schema_version"] != "pinax.repair_plan.v1" || savedPlan["status"] != "planned" {
		t.Fatalf("saved repair plan fields = %#v", savedPlan)
	}
}

func TestRepairApplyRequiresApprovalAndSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "No Tags.md"), "# No Tags\n\nbody\n")
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)
	eventsPath := filepath.Join(root, ".pinax", "events.jsonl")
	eventsBefore := ""
	if fileExists(eventsPath) {
		eventsBefore = readCLIFile(t, eventsPath)
	}

	failed, err := runCLIExpectError("repair", "apply", "--vault", root, "--plan", planID, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("repair apply without approval err=%v out=%s", err, failed)
	}
	if got := readCLIFile(t, eventsPath); got != eventsBefore {
		t.Fatalf("repair apply without approval changed events:\n%s", got)
	}

	failed, err = runCLIExpectError("repair", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if err == nil || !strings.Contains(failed, "snapshot_required") {
		t.Fatalf("repair apply without snapshot err=%v out=%s", err, failed)
	}
	if !strings.Contains(failed, "pinax git snapshot") {
		t.Fatalf("snapshot error missing runnable action:\n%s", failed)
	}
}

func TestRepairApplyLowRiskOperationsAndRejectsStalePlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "No Tags.md")
	writeCLIFixture(t, notePath, "# No Tags\n\nbody\n")
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)

	applyOut := runCLI(t, "repair", "apply", "--vault", root, "--plan", planID, "--yes", "--snapshot-message", "repair 前快照", "--json")
	if !strings.Contains(applyOut, "repair.apply") || !strings.Contains(applyOut, "\"status\":\"success\"") {
		t.Fatalf("repair apply output = %s", applyOut)
	}
	content := readCLIFile(t, notePath)
	for _, want := range []string{"schema_version: pinax.note.v1", "note_id:", "title: No Tags", "tags: []", "# No Tags"} {
		if !strings.Contains(content, want) {
			t.Fatalf("repair apply note missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "repair.apply") {
		t.Fatalf("repair apply did not append event evidence")
	}

	staleRoot := t.TempDir()
	runCLI(t, "init", staleRoot, "--title", "Vault", "--json")
	staleNotePath := filepath.Join(staleRoot, "No Tags.md")
	writeCLIFixture(t, staleNotePath, "# No Tags\n\nbody\n")
	staleSavedOut := runCLI(t, "repair", "plan", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSavedOut), &staleEnvelope); err != nil {
		t.Fatalf("stale saved repair plan json invalid: %v\n%s", err, staleSavedOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleNotePath, "# No Tags\n\nchanged\n")
	failed, err := runCLIExpectError("repair", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "repair 前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax repair plan") {
		t.Fatalf("stale repair apply err=%v out=%s", err, failed)
	}
}

func TestOrganizeSuggestCreatesReviewableAgentPlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Research Idea.md"), "# Research Idea\n\nbody #research [[Missing Target]]\n\n![Missing](missing.png)\n")
	writeCLIFixture(t, filepath.Join(root, ".agents", "skills", "Internal.md"), "# Internal\n\nagent asset\n")
	writeCLIFixture(t, filepath.Join(root, "docs", "Product.md"), "# Product\n\nproject doc\n")
	writeCLIFixture(t, filepath.Join(root, "AGENTS.md"), "# Agent Rules\n\nproject rules\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "# Daily 2026-06-06\n\nlog\n")

	out := runCLI(t, "organize", "suggest", "--vault", root, "--json")
	if fileExists(filepath.Join(root, ".pinax", "organize-plans")) {
		t.Fatalf("organize suggest without --save wrote organize-plans asset")
	}
	humanOut := runCLI(t, "organize", "suggest", "--vault", root)
	for _, want := range []string{"操作预览", "模式", "风险", "动作", "来源", "目标", "原因", "Research Idea.md", "pinax organize suggest --vault", "--save"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("organize suggest human output missing %q:\n%s", want, humanOut)
		}
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("organize suggest json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "organize.suggest" || envelope["status"] != "partial" {
		t.Fatalf("organize suggest envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.organize_plan.v1" || data["plan_id"] == "" || data["status"] != "planned" {
		t.Fatalf("organize plan identity missing: %#v", data)
	}
	operations := data["operations"].([]any)
	if len(operations) == 0 {
		t.Fatalf("organize plan operations missing: %#v", data)
	}
	operationKinds := map[string]bool{}
	for _, item := range operations {
		operation := item.(map[string]any)
		operationKinds[operation["kind"].(string)] = true
		path := fmt.Sprint(operation["path"])
		if strings.Contains(path, ".agents/") || strings.HasPrefix(path, "docs/") || path == "AGENTS.md" {
			t.Fatalf("organize plan should not include project assets: %#v", operation)
		}
		if operation["kind"] == "move" && strings.HasPrefix(path, "notes/") {
			t.Fatalf("organize plan should not move notes already under notes/: %#v", operation)
		}
	}
	for _, kind := range []string{"move", "tag_patch", "kind_patch", "status_patch", "link_resolution", "attachment_repair", "manual_review"} {
		if !operationKinds[kind] {
			t.Fatalf("organize operation kind %s missing: %#v", kind, operations)
		}
	}
	op := operations[0].(map[string]any)
	for _, key := range []string{"operation_id", "kind", "mode", "risk", "path", "reason", "evidence"} {
		if op[key] == nil || op[key] == "" {
			t.Fatalf("organize operation missing %s: %#v", key, op)
		}
	}

	savedOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved organize suggest json invalid: %v\n%s", err, savedOut)
	}
	savedData := savedEnvelope["data"].(map[string]any)
	savedPath := savedData["saved_path"].(string)
	if savedPath == "" || !strings.HasPrefix(savedPath, ".pinax/organize-plans/") {
		t.Fatalf("saved organize path invalid: %#v", savedData)
	}
	savedHumanOut := runCLI(t, "organize", "suggest", "--vault", root, "--save")
	for _, want := range []string{"saved_path", "pinax organize apply --vault", "--snapshot-message", "整理前快照"} {
		if !strings.Contains(savedHumanOut, want) {
			t.Fatalf("saved organize human output missing %q:\n%s", want, savedHumanOut)
		}
	}
	planContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(savedPath)))
	if !strings.Contains(planContent, "pinax.organize_plan.v1") || !strings.Contains(planContent, "Research Idea.md") {
		t.Fatalf("saved organize plan content invalid:\n%s", planContent)
	}
	listOut := runCLI(t, "organize", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "organize.list") || !strings.Contains(listOut, savedData["plan_id"].(string)) {
		t.Fatalf("organize list output invalid:\n%s", listOut)
	}
	listHumanOut := runCLI(t, "organize", "list", "--vault", root)
	for _, want := range []string{"已保存计划", "计划", "状态", "操作", savedData["plan_id"].(string), "pinax organize apply --vault", "--plan"} {
		if !strings.Contains(listHumanOut, want) {
			t.Fatalf("organize list human output missing %q:\n%s", want, listHumanOut)
		}
	}

	agentOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--agent")
	for _, want := range []string{"command=organize.suggest", "status=partial", "fact.plan_id=", "fact.operations=", "fact.automatic=", "fact.manual_review=", "fact.risk.low=", "fact.saved_path=.pinax/organize-plans/"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("organize suggest agent missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, "状态:") || strings.Contains(agentOut, "重点:") {
		t.Fatalf("organize suggest agent contains human prose:\n%s", agentOut)
	}
}

func TestOrganizeListEmptyGuidesSaveWorkflow(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "organize", "list", "--vault", root)
	for _, want := range []string{"plans", "0", "下一步", "pinax organize suggest --vault", "--save"} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty organize list output missing %q:\n%s", want, out)
		}
	}
}

func TestOrganizeApplySavedPlanRejectsStaleAndMoves(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "Research Idea.md")
	writeCLIFixture(t, source, "# Research Idea\n\nbody #research\n")
	savedOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved organize suggest json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)

	failed, err := runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("organize apply saved plan without approval err=%v out=%s", err, failed)
	}
	failedHuman, err := runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID)
	if err == nil {
		t.Fatalf("organize apply human without approval unexpectedly succeeded: %s", failedHuman)
	}
	for _, want := range []string{"approval_required", "organize apply 需要 --yes", "pinax organize suggest --vault", "--save", "pinax organize apply --vault", "--plan", "--snapshot-message"} {
		if !strings.Contains(failedHuman, want) {
			t.Fatalf("organize apply human error missing %q:\n%s", want, failedHuman)
		}
	}
	failed, err = runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if err == nil || !strings.Contains(failed, "snapshot_required") {
		t.Fatalf("organize apply saved plan without snapshot err=%v out=%s", err, failed)
	}

	applyOut := runCLI(t, "organize", "apply", "--vault", root, "--plan", planID, "--yes", "--snapshot-message", "整理前快照", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("organize apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyEnvelope["command"] != "organize.apply" || applyFacts["plan_id"] != planID || applyFacts["applied_moves"] != "1" || applyFacts["applied_metadata"] != "2" {
		t.Fatalf("organize apply envelope = %#v", applyEnvelope)
	}
	target := filepath.Join(root, "notes", "research-idea.md")
	if fileExists(source) || !fileExists(target) {
		t.Fatalf("organize apply did not move source into notes/")
	}
	targetContent := readCLIFile(t, target)
	for _, want := range []string{"tags: [research]", "status: active", "# Research Idea"} {
		if !strings.Contains(targetContent, want) {
			t.Fatalf("organize apply did not apply metadata %q:\n%s", want, targetContent)
		}
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "organize.apply") {
		t.Fatalf("organize apply did not append event evidence")
	}

	staleRoot := t.TempDir()
	runCLI(t, "init", staleRoot, "--title", "Vault", "--json")
	staleSource := filepath.Join(staleRoot, "Stale Note.md")
	writeCLIFixture(t, staleSource, "# Stale Note\n\nbody\n")
	staleOut := runCLI(t, "organize", "suggest", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale organize suggest json invalid: %v\n%s", err, staleOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleSource, "# Stale Note\n\nchanged\n")
	failed, err = runCLIExpectError("organize", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "整理前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax organize suggest") {
		t.Fatalf("stale organize apply err=%v out=%s", err, failed)
	}
}

func TestDailyInboxWorkflowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	editorLog := filepath.Join(root, "daily-editor.log")
	editor := writeFakeEditor(t, root, editorLog)
	date := time.Now().UTC().Format("2006-01-02")
	dailyRel := filepath.ToSlash(filepath.Join("notes", "daily", date+".md"))

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
	dailyDatedRel := filepath.ToSlash(filepath.Join("notes", "daily", journalDate+".md"))
	dailyDatedOut := runCLI(t, "daily", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(dailyDatedOut, dailyDatedRel) || !strings.Contains(dailyDatedOut, `"period":"daily"`) {
		t.Fatalf("daily show --date output invalid:\n%s", dailyDatedOut)
	}
	dailyNextOut := runCLI(t, "daily", "show", "--date", journalDate, "--next", "--vault", root, "--json")
	if !strings.Contains(dailyNextOut, "notes/daily/2026-06-07.md") || !strings.Contains(dailyNextOut, `"date":"2026-06-07"`) {
		t.Fatalf("daily show --next output invalid:\n%s", dailyNextOut)
	}

	weeklyOut := runCLI(t, "weekly", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(weeklyOut, "weekly.show") || !strings.Contains(weeklyOut, "notes/weekly/2026-W23.md") || !strings.Contains(weeklyOut, `"period":"weekly"`) {
		t.Fatalf("weekly show output invalid:\n%s", weeklyOut)
	}
	weeklyKeyOut := runCLI(t, "weekly", "show", "--date", "2026-W23", "--vault", root, "--json")
	if !strings.Contains(weeklyKeyOut, "notes/weekly/2026-W23.md") || !strings.Contains(weeklyKeyOut, `"date":"2026-W23"`) {
		t.Fatalf("weekly show should accept completion key:\n%s", weeklyKeyOut)
	}
	monthlyOut := runCLI(t, "monthly", "show", "--date", journalDate, "--vault", root)
	for _, want := range []string{"状态", "重点", "notes/monthly/2026-06.md", "period", "monthly", "Monthly 2026-06"} {
		if !strings.Contains(monthlyOut, want) {
			t.Fatalf("monthly show human output missing %q:\n%s", want, monthlyOut)
		}
	}
	monthlyKeyOut := runCLI(t, "monthly", "show", "--date", "2026-06", "--vault", root, "--json")
	if !strings.Contains(monthlyKeyOut, "notes/monthly/2026-06.md") || !strings.Contains(monthlyKeyOut, `"date":"2026-06"`) {
		t.Fatalf("monthly show should accept completion key:\n%s", monthlyKeyOut)
	}

	captureOut := runCLI(t, "inbox", "capture", "Inbox Idea", "--body", "正文", "--tags", "idea", "--vault", root, "--json")
	var captureEnvelope map[string]any
	if err := json.Unmarshal([]byte(captureOut), &captureEnvelope); err != nil {
		t.Fatalf("inbox capture json invalid: %v\n%s", err, captureOut)
	}
	captureFacts := captureEnvelope["facts"].(map[string]any)
	inboxPath := captureFacts["path"].(string)
	if captureEnvelope["command"] != "inbox.capture" || !strings.HasPrefix(inboxPath, "notes/inbox/") || captureFacts["kind"] != "inbox" || captureFacts["status"] != "inbox" {
		t.Fatalf("inbox capture envelope = %#v", captureEnvelope)
	}
	inboxContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(inboxPath)))
	for _, want := range []string{"kind: inbox", "status: inbox", "正文"} {
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
		{args: []string{"folder", "list", "--vault", root, "--json"}, cmd: "folder.list", want: "architecture"},
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
}

func TestSavedViewsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "工作参考", "--group", "work", "--folder", "refs", "--kind", "reference", "--status", "active", "--tags", "work", "--body", "正文", "--slug", "work-ref", "--vault", root, "--json")
	runCLI(t, "note", "new", "个人草稿", "--folder", "drafts", "--kind", "fleeting", "--status", "draft", "--tags", "personal", "--body", "正文", "--slug", "personal-draft", "--vault", root, "--json")

	saveOut := runCLI(t, "view", "save", "active-work", "--group", "work", "--status", "active", "--kind", "reference", "--sort", "title", "--vault", root, "--json")
	var saveEnvelope map[string]any
	if err := json.Unmarshal([]byte(saveOut), &saveEnvelope); err != nil {
		t.Fatalf("view save json invalid: %v\n%s", err, saveOut)
	}
	if saveEnvelope["command"] != "view.save" || saveEnvelope["facts"].(map[string]any)["view"] != "active-work" {
		t.Fatalf("view save envelope = %#v", saveEnvelope)
	}
	viewsPath := filepath.Join(root, ".pinax", "views.json")
	viewsContent := readCLIFile(t, viewsPath)
	if !strings.Contains(viewsContent, "active-work") || !strings.Contains(viewsContent, "pinax.views.v1") {
		t.Fatalf("views asset invalid:\n%s", viewsContent)
	}

	listOut := runCLI(t, "view", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "view.list") || !strings.Contains(listOut, "active-work") {
		t.Fatalf("view list output invalid:\n%s", listOut)
	}
	showOut := runCLI(t, "view", "show", "active-work", "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("view show json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "view.show" || showFacts["view"] != "active-work" || showFacts["returned"] != "1" || !strings.Contains(showOut, "工作参考") || strings.Contains(showOut, "个人草稿") {
		t.Fatalf("view show envelope = %#v out=%s", showEnvelope, showOut)
	}

	failed, err := runCLIExpectError("view", "delete", "active-work", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("view delete without approval err=%v out=%s", err, failed)
	}
	deleteOut := runCLI(t, "view", "delete", "active-work", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "view.delete") || strings.Contains(readCLIFile(t, viewsPath), "active-work") {
		t.Fatalf("view delete failed:\n%s\nviews=%s", deleteOut, readCLIFile(t, viewsPath))
	}
	if !fileExists(filepath.Join(root, "notes", "work", "refs", "work-ref.md")) {
		t.Fatalf("view delete removed note")
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
	for _, want := range []string{"状态", "重点", "success", "孤立笔记已列出。", "路径", "标题", "分类", "notes/gamma.md", "Gamma", "reference"} {
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

func TestNoteAttachmentCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nBody.\n")
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, source, "png-bytes")

	attachOut := runCLI(t, "note", "attach", "Alpha", source, "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("note attach json invalid: %v\n%s", err, attachOut)
	}
	attachFacts := attachEnvelope["facts"].(map[string]any)
	attachmentPath, _ := attachFacts["attachment_path"].(string)
	if attachEnvelope["command"] != "note.attach" || attachFacts["path"] != "notes/alpha.md" || attachmentPath == "" || !strings.HasPrefix(attachmentPath, "attachments/note_alpha/") {
		t.Fatalf("note attach envelope = %#v", attachEnvelope)
	}
	if !fileExists(filepath.Join(root, attachmentPath)) {
		t.Fatalf("attachment file missing at %s", attachmentPath)
	}
	alphaContent := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	if !strings.Contains(alphaContent, "![diagram.png](/"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../../"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../"+attachmentPath+")") {
		t.Fatalf("note body missing attachment reference:\n%s", alphaContent)
	}

	attachmentsOut := runCLI(t, "note", "attachments", "Alpha", "--vault", root, "--json")
	var attachmentsEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachmentsOut), &attachmentsEnvelope); err != nil {
		t.Fatalf("note attachments json invalid: %v\n%s", err, attachmentsOut)
	}
	attachmentsFacts := attachmentsEnvelope["facts"].(map[string]any)
	if attachmentsEnvelope["command"] != "note.attachments" || attachmentsFacts["attachments"] != "1" || attachmentsFacts["missing"] != "0" || !strings.Contains(attachmentsOut, attachmentPath) {
		t.Fatalf("note attachments envelope = %#v out=%s", attachmentsEnvelope, attachmentsOut)
	}

	before := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	failed, err := runCLIExpectError("note", "attach", "Alpha", filepath.Join(sourceDir, "missing.png"), "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "attachment_source_missing") {
		t.Fatalf("missing attachment err=%v out=%s", err, failed)
	}
	if after := readCLIFile(t, filepath.Join(root, "notes", "alpha.md")); after != before {
		t.Fatalf("missing source modified note:\nbefore=%s\nafter=%s", before, after)
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
	for _, want := range []string{"工具笔记", "notes/work/inbox/tool-note.md", "#pinax", "group=work", "folder=inbox", "kind=reference"} {
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

func TestIndexSearchDatabaseAndFiltersCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "认证方案", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth,design", "--body", "OAuth 登录设计 [[Identity]]", "--slug", "auth-design", "--vault", root, "--json")

	initOut := runCLI(t, "index", "init", "--vault", root, "--json")
	var initEnvelope map[string]any
	if err := json.Unmarshal([]byte(initOut), &initEnvelope); err != nil {
		t.Fatalf("index init json invalid: %v\n%s", err, initOut)
	}
	if initEnvelope["command"] != "index.init" || initEnvelope["status"] != "success" {
		t.Fatalf("index init envelope = %#v", initEnvelope)
	}
	initFacts := initEnvelope["facts"].(map[string]any)
	if initFacts["schema_version"] == "" || initFacts["index_status"] == "" || initFacts["path"] != ".pinax/index.sqlite" {
		t.Fatalf("index init facts = %#v", initFacts)
	}

	rebuildOut := runCLI(t, "index", "rebuild", "--vault", root, "--json")
	var rebuildEnvelope map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuildEnvelope); err != nil {
		t.Fatalf("index rebuild json invalid: %v\n%s", err, rebuildOut)
	}
	rebuildFacts := rebuildEnvelope["facts"].(map[string]any)
	for _, key := range []string{"notes", "tags", "links", "tokens", "dimensions", "schema_version"} {
		if value, ok := rebuildFacts[key]; !ok || value == "" {
			t.Fatalf("index rebuild facts missing %s: %#v", key, rebuildFacts)
		}
	}

	statusOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("index status json invalid: %v\n%s", err, statusOut)
	}
	statusFacts := statusEnvelope["facts"].(map[string]any)
	if statusEnvelope["command"] != "index.status" || statusFacts["index_status"] != "fresh" || statusFacts["schema_version"] == "" {
		t.Fatalf("index status envelope = %#v", statusEnvelope)
	}

	searchOut := runCLI(t, "search", "认证", "--tag", "auth", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--limit", "5", "--vault", root, "--json")
	var searchEnvelope map[string]any
	if err := json.Unmarshal([]byte(searchOut), &searchEnvelope); err != nil {
		t.Fatalf("search json invalid: %v\n%s", err, searchOut)
	}
	searchFacts := searchEnvelope["facts"].(map[string]any)
	for _, want := range []string{"engine", "index_status", "total", "returned", "filter.tag", "filter.group", "filter.folder", "filter.kind", "filter.status"} {
		if searchFacts[want] == "" {
			t.Fatalf("search facts missing %s: %#v", want, searchFacts)
		}
	}
	if searchFacts["engine"] != "index" || searchFacts["index_status"] != "fresh" || searchFacts["returned"] != "1" {
		t.Fatalf("search facts = %#v", searchFacts)
	}
	data := searchEnvelope["data"].(map[string]any)
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("search results = %#v", data)
	}
	result := results[0].(map[string]any)
	if result["score"] == nil || len(result["matched_fields"].([]any)) == 0 || result["snippet"] == "" {
		t.Fatalf("search result missing score/matches/snippet: %#v", result)
	}

	runCLI(t, "note", "new", "Auth A", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth", "--body", "认证 checklist", "--slug", "a-auth", "--vault", root, "--json")
	sortedOut := runCLI(t, "search", "认证", "--tag", "auth", "--sort", "title", "--limit", "1", "--vault", root, "--json")
	var sortedEnvelope map[string]any
	if err := json.Unmarshal([]byte(sortedOut), &sortedEnvelope); err != nil {
		t.Fatalf("sorted search json invalid: %v\n%s", err, sortedOut)
	}
	sortedFacts := sortedEnvelope["facts"].(map[string]any)
	if sortedFacts["sort"] != "title" || sortedFacts["returned"] != "1" {
		t.Fatalf("sorted search facts = %#v", sortedFacts)
	}
	sortedResults := sortedEnvelope["data"].(map[string]any)["results"].([]any)
	sortedNote := sortedResults[0].(map[string]any)["note"].(map[string]any)
	if sortedNote["title"] != "Auth A" {
		t.Fatalf("sorted search first note = %#v", sortedNote)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md"), strings.Replace(readCLIFile(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md")), "OAuth", "OAuth2", 1))
	staleOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale status json invalid: %v\n%s", err, staleOut)
	}
	if staleEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("expected stale index after note change: %#v", staleEnvelope)
	}

	staleSearchOut := runCLI(t, "search", "认证", "--allow-stale", "--vault", root, "--json")
	var staleSearchEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSearchOut), &staleSearchEnvelope); err != nil {
		t.Fatalf("stale search json invalid: %v\n%s", err, staleSearchOut)
	}
	if staleSearchEnvelope["status"] != "partial" || staleSearchEnvelope["facts"].(map[string]any)["engine"] != "index" || staleSearchEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("stale search envelope = %#v", staleSearchEnvelope)
	}
	if len(staleSearchEnvelope["actions"].([]any)) == 0 {
		t.Fatalf("stale search missing rebuild action: %#v", staleSearchEnvelope)
	}

	invalidDate, err := runCLIExpectError("search", "认证", "--updated-after", "not-a-date", "--vault", root, "--json")
	if err == nil || !strings.Contains(invalidDate, "invalid_date_filter") {
		t.Fatalf("invalid date filter err=%v out=%s", err, invalidDate)
	}

	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index for fallback search: %v", err)
	}
	fallbackOut := runCLI(t, "search", "认证", "--vault", root, "--json")
	var fallbackEnvelope map[string]any
	if err := json.Unmarshal([]byte(fallbackOut), &fallbackEnvelope); err != nil {
		t.Fatalf("fallback search json invalid: %v\n%s", err, fallbackOut)
	}
	fallbackFacts := fallbackEnvelope["facts"].(map[string]any)
	if fallbackFacts["engine"] == "index" || fallbackFacts["index_status"] != "missing" || fallbackFacts["returned"] != "2" {
		t.Fatalf("fallback search facts = %#v", fallbackFacts)
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
	if !strings.Contains(stdinOut, "note.new") || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "stdin-note.md")), "stdin body") {
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
	for _, want := range []string{"状态", "重点", "success", "路径", "标题", "Inline Note", "notes/work/inline.md"} {
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
	for _, want := range []string{"create", "read", "open", "edit", "rename", "move", "archive", "delete", "tag"} {
		if !strings.Contains(help, want) {
			t.Fatalf("note help missing %q:\n%s", want, help)
		}
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
	if !strings.Contains(log, "--wait") || !strings.Contains(log, "notes/editable.md") {
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
	if !strings.Contains(log, "notes/open-after-create.md") {
		t.Fatalf("fake editor log missing created note path:\n%s", log)
	}

	agentOut := runCLI(t, "note", "list", "--recent", "--vault", root, "--agent")
	for _, want := range []string{"command=note.list", "fact.recent=true", "fact.sort=updated"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("recent agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestAgentOutputMode(t *testing.T) {
	root := t.TempDir()
	out := runCLI(t, "init", root, "--title", "Vault", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=vault.init", "status=success"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
		}
	}
}

func TestInitWithoutArgUsesVaultFlagDefault(t *testing.T) {
	root := t.TempDir()
	out := runCLIInDir(t, root, "init", "--title", "Vault")
	for _, want := range []string{"状态", "重点", "success", "Pinax vault 已初始化。", "指标", "vault", "下一步", "pinax validate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	for _, old := range []string{"状态:", "重点:", "推荐下一步:", "vault="} {
		if strings.Contains(out, old) {
			t.Fatalf("init output still uses label prose %q:\n%s", old, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); err != nil {
		t.Fatalf("init without arg did not create config in cwd: %v", err)
	}
}

func TestMissingRequiredArgReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("note", "show", "--json")
	if err == nil {
		t.Fatalf("note show without arg succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "note.show" {
		t.Fatalf("error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "argument_required" {
		t.Fatalf("error object = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "pinax note show <note>") {
		t.Fatalf("error hint = %#v", errorObject["hint"])
	}
}

func TestFlagErrorReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("validate", "--json", "--bogus")
	if err == nil {
		t.Fatalf("unknown flag succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json flag error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.flag" {
		t.Fatalf("flag error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "flag_error" {
		t.Fatalf("flag error object = %#v", envelope["error"])
	}
}

func TestOutputModesAreMutuallyExclusive(t *testing.T) {
	out, err := runCLIExpectError("version", "--json", "--agent")
	if err == nil {
		t.Fatalf("conflicting output modes succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json mode conflict output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.output_mode" {
		t.Fatalf("mode conflict envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "output_mode_conflict" {
		t.Fatalf("mode conflict error = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "只保留一个输出模式") {
		t.Fatalf("mode conflict hint = %#v", errorObject["hint"])
	}
}

func TestApplyHelpDocumentsSafetyFlags(t *testing.T) {
	out := runCLI(t, "organize", "apply", "--help")
	for _, want := range []string{"--yes", "--snapshot-message", "先运行 pinax organize suggest --save", "Git snapshot"} {
		if !strings.Contains(out, want) {
			t.Fatalf("organize apply help missing %q:\n%s", want, out)
		}
	}
}

func TestEventsAndExplainOutputModes(t *testing.T) {
	root := t.TempDir()
	events := runCLI(t, "init", root, "--events")
	lines := strings.Split(strings.TrimSpace(events), "\n")
	if len(lines) != 2 {
		t.Fatalf("events lines = %q", events)
	}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %d invalid: %v\n%s", i, err, events)
		}
		if _, ok := event["error"]; ok && event["error"] == nil {
			t.Fatalf("event %d contains null error field: %#v", i, event)
		}
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) {
		t.Fatalf("events missing start/end:\n%s", events)
	}

	explain := runCLI(t, "validate", "--vault", root, "--explain")
	for _, want := range []string{"结论:", "证据:", "置信度:", "推荐下一步:"} {
		if !strings.Contains(explain, want) {
			t.Fatalf("explain output missing %q:\n%s", want, explain)
		}
	}
}

func TestNotebookCoreOutputContractAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Contract Note", "--body", "正文", "--tags", "contract", "--vault", root, "--json")

	rootHelp := runCLI(t, "--help")
	for _, want := range []string{"daily", "inbox", "view", "import", "export", "index", "organize"} {
		if !strings.Contains(rootHelp, want) {
			t.Fatalf("root help missing %q:\n%s", want, rootHelp)
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
	for _, want := range []string{"init", "status", "rebuild"} {
		if !strings.Contains(indexHelp, want) {
			t.Fatalf("index help missing %q:\n%s", want, indexHelp)
		}
	}
	organizeHelp := runCLI(t, "organize", "--help")
	for _, want := range []string{"suggest", "list", "apply"} {
		if !strings.Contains(organizeHelp, want) {
			t.Fatalf("organize help missing %q:\n%s", want, organizeHelp)
		}
	}

	humanOut := runCLI(t, "view", "list", "--vault", root)
	if !strings.Contains(humanOut, "状态") || !strings.Contains(humanOut, "重点") || strings.Contains(humanOut, "状态:") || strings.HasPrefix(strings.TrimSpace(humanOut), "{") {
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

func TestHumanOutputIsPolishedForNotebookViewsAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "project", "create", "personal", "--name", "个人", "--notes-prefix", "notes/personal", "--vault", root, "--json")
	runCLI(t, "note", "new", "工作笔记", "--group", "work", "--kind", "reference", "--tags", "work", "--body", "正文", "--vault", root, "--json")
	runCLI(t, "note", "new", "个人笔记", "--group", "personal", "--kind", "reference", "--tags", "personal", "--body", "正文", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# 原始笔记\n\n正文\n")

	groupOut := runCLI(t, "group", "list", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "状态", "重点", "success", "指标", "值", "分组", "数量", "work", "personal", "(未分组)"} {
		if !strings.Contains(groupOut, want) {
			t.Fatalf("group list polished output missing %q:\n%s", want, groupOut)
		}
	}
	for _, old := range []string{"Pinax", "摘要", "统计", "列表", "状态:", "事实:", "dimension=group, dimensions=", "dimension=group"} {
		if strings.Contains(groupOut, old) {
			t.Fatalf("group list still uses old prose %q:\n%s", old, groupOut)
		}
	}

	helpOut := runCLI(t, "metadata")
	for _, want := range []string{"简介", "用法", "可用命令", "参数", "全局参数", "pinax metadata [command] --help"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("metadata help missing %q:\n%s", want, helpOut)
		}
	}
	for _, old := range []string{"Usage:", "Available Commands:", "Flags:", "Global Flags:"} {
		if strings.Contains(helpOut, old) {
			t.Fatalf("metadata help still contains English cobra heading %q:\n%s", old, helpOut)
		}
	}

	planOut := runCLI(t, "metadata", "plan", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "状态", "重点", "success", "Metadata 计划已生成。", "指标", "值", "planned_updates", "下一步"} {
		if !strings.Contains(planOut, want) {
			t.Fatalf("metadata plan polished output missing %q:\n%s", want, planOut)
		}
	}
	for _, old := range []string{"Pinax", "摘要", "统计", "状态:", "重点:", "事实: planned_updates=", "planned_updates="} {
		if strings.Contains(planOut, old) {
			t.Fatalf("metadata plan still uses old prose %q:\n%s", old, planOut)
		}
	}
}

func TestBackendProviderCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "backend", "--help")
	for _, want := range []string{"list", "add", "status", "doctor", "capabilities", "diff", "push", "pull", "remove"} {
		if !strings.Contains(help, want) {
			t.Fatalf("backend help missing %q:\n%s", want, help)
		}
	}

	// backend add s3
	addOut := runCLI(t, "backend", "add", "s3", "--name", "work-s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--profile", "work", "--vault", root, "--json")
	var addEnvelope map[string]any
	if err := json.Unmarshal([]byte(addOut), &addEnvelope); err != nil {
		t.Fatalf("backend add json invalid: %v\n%s", err, addOut)
	}
	if addEnvelope["command"] != "backend.add" || addEnvelope["status"] != "success" {
		t.Fatalf("backend add envelope = %#v", addEnvelope)
	}
	addFacts := addEnvelope["facts"].(map[string]any)
	if addFacts["name"] != "work-s3" || addFacts["kind"] != "s3" {
		t.Fatalf("backend add facts = %#v", addFacts)
	}
	if strings.Contains(strings.ToLower(addOut), "secret") || strings.Contains(strings.ToLower(addOut), "access_key") {
		t.Fatalf("backend add output leaked secret-like material:\n%s", addOut)
	}

	// backend list
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "backend.list" || listEnvelope["status"] != "success" {
		t.Fatalf("backend list envelope = %#v", listEnvelope)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listFacts["backends"] != "1" || listFacts["default_backend"] != "work-s3" {
		t.Fatalf("backend list facts = %#v", listFacts)
	}

	// backend status
	statusOut := runCLI(t, "backend", "status", "--name", "work-s3", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("backend status json invalid: %v\n%s", err, statusOut)
	}
	if statusEnvelope["command"] != "backend.status" {
		t.Fatalf("backend status envelope = %#v", statusEnvelope)
	}

	// backend doctor
	doctorOut := runCLI(t, "backend", "doctor", "--name", "work-s3", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("backend doctor json invalid: %v\n%s", err, doctorOut)
	}
	if doctorEnvelope["command"] != "backend.doctor" {
		t.Fatalf("backend doctor envelope = %#v", doctorEnvelope)
	}

	// backend capabilities
	capOut := runCLI(t, "backend", "capabilities", "--name", "work-s3", "--vault", root, "--agent")
	for _, want := range []string{"command=backend.capabilities", "status=success", "fact.name=work-s3", "fact.kind=s3"} {
		if !strings.Contains(capOut, want) {
			t.Fatalf("backend capabilities agent output missing %q:\n%s", want, capOut)
		}
	}

	// backend diff
	diffOut := runCLI(t, "backend", "diff", "--name", "work-s3", "--vault", root, "--json")
	var diffEnvelope map[string]any
	if err := json.Unmarshal([]byte(diffOut), &diffEnvelope); err != nil {
		t.Fatalf("backend diff json invalid: %v\n%s", err, diffOut)
	}
	if diffEnvelope["command"] != "backend.diff" {
		t.Fatalf("backend diff envelope = %#v", diffEnvelope)
	}

	// backend push dry-run
	pushDryRun := runCLI(t, "backend", "push", "--name", "work-s3", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(pushDryRun, "backend.push") || !strings.Contains(pushDryRun, `"dry_run":true`) {
		t.Fatalf("backend push dry-run output invalid:\n%s", pushDryRun)
	}

	// backend push without approval
	pushFail, err := runCLIExpectError("backend", "push", "--name", "work-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(pushFail, "approval_required") {
		t.Fatalf("backend push without approval err=%v out=%s", err, pushFail)
	}

	// backend add rclone
	rcloneOut := runCLI(t, "backend", "add", "rclone", "--name", "work-drive", "--remote", "workdrive:pinax", "--vault", root, "--json")
	if !strings.Contains(rcloneOut, "backend.add") {
		t.Fatalf("backend add rclone output = %s", rcloneOut)
	}

	// backend remove
	removeOut := runCLI(t, "backend", "remove", "--name", "work-drive", "--vault", root, "--json")
	if !strings.Contains(removeOut, "backend.remove") {
		t.Fatalf("backend remove output = %s", removeOut)
	}
	// verify removed
	listAfterRemove := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listAfterEnvelope map[string]any
	if err := json.Unmarshal([]byte(listAfterRemove), &listAfterEnvelope); err != nil {
		t.Fatalf("backend list after remove json invalid: %v\n%s", err, listAfterRemove)
	}
	if listAfterEnvelope["facts"].(map[string]any)["backends"] != "1" {
		t.Fatalf("expected 1 backend after remove: %s", listAfterRemove)
	}

	// backend add without name
	noName, err := runCLIExpectError("backend", "add", "s3", "--bucket", "b", "--region", "r", "--vault", root, "--json")
	if err == nil || !strings.Contains(noName, "backend_name_required") {
		t.Fatalf("backend add without name err=%v out=%s", err, noName)
	}

	// backend add invalid kind
	badKind, err := runCLIExpectError("backend", "add", "ftp", "--name", "x", "--vault", root, "--json")
	if err == nil || !strings.Contains(badKind, "backend_kind_invalid") {
		t.Fatalf("backend add invalid kind err=%v out=%s", err, badKind)
	}

	// backend add s3 missing required fields
	missingS3, err := runCLIExpectError("backend", "add", "s3", "--name", "bad-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingS3, "backend_config_incomplete") {
		t.Fatalf("backend add s3 missing fields err=%v out=%s", err, missingS3)
	}

	// backend status not found
	notFound, err := runCLIExpectError("backend", "status", "--name", "nonexistent", "--vault", root, "--json")
	if err == nil || !strings.Contains(notFound, "backend_not_found") {
		t.Fatalf("backend status not found err=%v out=%s", err, notFound)
	}

	// legacy storage compatibility: storage commands still work
	storageOut := runCLI(t, "storage", "set-s3", "--bucket", "legacy-bucket", "--region", "us-east-1", "--vault", root, "--json")
	if !strings.Contains(storageOut, "storage.set_s3") {
		t.Fatalf("storage set-s3 still works:\n%s", storageOut)
	}
}

func TestBackendLegacyStorageProjection(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	// Only set legacy storage.json, no backends.json yet.
	runCLI(t, "storage", "set-s3", "--bucket", "notes", "--region", "us-east-1", "--vault", root, "--json")
	// Remove any backends.json that might have been created.
	os.Remove(filepath.Join(root, ".pinax", "backends.json"))
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list legacy json invalid: %v\n%s", err, listOut)
	}
	facts := listEnvelope["facts"].(map[string]any)
	if facts["backends"] != "1" {
		t.Fatalf("expected 1 backend from legacy storage: %s", listOut)
	}
}

func TestPlanningWorkflowsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "plan", "--help")
	for _, want := range []string{"daily", "weekly", "monthly", "actions", "snapshot"} {
		if !strings.Contains(help, want) {
			t.Fatalf("plan help missing %q:\n%s", want, help)
		}
	}

	// plan daily dry-run
	dailyDryRun := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--json")
	var dailyEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyDryRun), &dailyEnvelope); err != nil {
		t.Fatalf("plan daily json invalid: %v\n%s", err, dailyDryRun)
	}
	if dailyEnvelope["command"] != "plan.daily" || dailyEnvelope["status"] != "success" {
		t.Fatalf("plan daily envelope = %#v", dailyEnvelope)
	}
	dailyFacts := dailyEnvelope["facts"].(map[string]any)
	if dailyFacts["period"] != "daily" || dailyFacts["dry_run"] != "true" || dailyFacts["max_commitments"] != "3" {
		t.Fatalf("plan daily facts = %#v", dailyFacts)
	}

	// plan daily with save
	dailyOut := runCLI(t, "plan", "daily", "--save", "--yes", "--vault", root, "--json")
	var dailySaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyOut), &dailySaveEnvelope); err != nil {
		t.Fatalf("plan daily save json invalid: %v\n%s", err, dailyOut)
	}
	if dailySaveEnvelope["command"] != "plan.daily" {
		t.Fatalf("plan daily save envelope = %#v", dailySaveEnvelope)
	}
	dailySaveFacts := dailySaveEnvelope["facts"].(map[string]any)
	if dailySaveFacts["dry_run"] != nil {
		t.Fatalf("plan daily save should not be dry_run: %#v", dailySaveFacts)
	}
	if dailySaveFacts["saved_path"] == "" || !strings.Contains(dailySaveFacts["saved_path"].(string), ".pinax/planning/snapshots/") {
		t.Fatalf("plan daily saved_path invalid: %#v", dailySaveFacts)
	}

	// plan weekly
	weeklyOut := runCLI(t, "plan", "weekly", "--dry-run", "--vault", root, "--json")
	var weeklyEnvelope map[string]any
	if err := json.Unmarshal([]byte(weeklyOut), &weeklyEnvelope); err != nil {
		t.Fatalf("plan weekly json invalid: %v\n%s", err, weeklyOut)
	}
	weeklyFacts := weeklyEnvelope["facts"].(map[string]any)
	if weeklyFacts["period"] != "weekly" || weeklyFacts["max_commitments"] != "7" {
		t.Fatalf("plan weekly facts = %#v", weeklyFacts)
	}

	// plan monthly
	monthlyOut := runCLI(t, "plan", "monthly", "--dry-run", "--vault", root, "--json")
	var monthlyEnvelope map[string]any
	if err := json.Unmarshal([]byte(monthlyOut), &monthlyEnvelope); err != nil {
		t.Fatalf("plan monthly json invalid: %v\n%s", err, monthlyOut)
	}
	monthlyFacts := monthlyEnvelope["facts"].(map[string]any)
	if monthlyFacts["period"] != "monthly" || monthlyFacts["max_commitments"] != "15" {
		t.Fatalf("plan monthly facts = %#v", monthlyFacts)
	}

	// plan actions dry-run
	actionsDryRun := runCLI(t, "plan", "actions", "--from", "daily", "--vault", root, "--json")
	var actionsEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsDryRun), &actionsEnvelope); err != nil {
		t.Fatalf("plan actions json invalid: %v\n%s", err, actionsDryRun)
	}
	if actionsEnvelope["command"] != "plan.actions" {
		t.Fatalf("plan actions envelope = %#v", actionsEnvelope)
	}

	// plan actions with save
	actionsSave := runCLI(t, "plan", "actions", "--from", "daily", "--save", "--vault", root, "--json")
	var actionsSaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsSave), &actionsSaveEnvelope); err != nil {
		t.Fatalf("plan actions save json invalid: %v\n%s", err, actionsSave)
	}
	actionsSaveFacts := actionsSaveEnvelope["facts"].(map[string]any)
	if actionsSaveFacts["saved_path"] == "" || !strings.Contains(actionsSaveFacts["saved_path"].(string), ".pinax/planning/actions/") {
		t.Fatalf("plan actions saved_path invalid: %#v", actionsSaveFacts)
	}

	// plan snapshot
	snapshotOut := runCLI(t, "plan", "snapshot", "--vault", root, "--json")
	var snapshotEnvelope map[string]any
	if err := json.Unmarshal([]byte(snapshotOut), &snapshotEnvelope); err != nil {
		t.Fatalf("plan snapshot json invalid: %v\n%s", err, snapshotOut)
	}
	if snapshotEnvelope["command"] != "plan.snapshot" {
		t.Fatalf("plan snapshot envelope = %#v", snapshotEnvelope)
	}
	snapshotFacts := snapshotEnvelope["facts"].(map[string]any)
	if snapshotFacts["snapshot_id"] == "" || snapshotFacts["saved_path"] == "" {
		t.Fatalf("plan snapshot facts = %#v", snapshotFacts)
	}

	// plan daily agent output
	agentOut := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--agent")
	for _, want := range []string{"command=plan.daily", "status=success", "fact.period=daily", "fact.dry_run=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("plan daily agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func runDashboardUntilCanceled(t *testing.T, root string) (string, string) {
	t.Helper()
	cmd := newRootCommand()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"dashboard", "--vault", root, "--port", "0"})
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	time.AfterFunc(50*time.Millisecond, cancel)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dashboard command failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

func runCLIWithInput(t *testing.T, input string, args ...string) string {
	t.Helper()
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pinax %v failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFakeEditor(t *testing.T, root, logPath string) string {
	t.Helper()
	path := filepath.Join(root, "fake-editor.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + shellQuoteForTest(logPath) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return path
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLIExpectError(args...)
	if err != nil {
		t.Fatalf("pinax %v failed: %v\n%s", args, err, out)
	}
	return out
}

func completionValueLines(out string) []string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "Completion ended") {
			continue
		}
		value, _, _ := strings.Cut(line, "\t")
		values = append(values, value)
	}
	return values
}

func runCLIInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	return runCLI(t, args...)
}

func runCLIExpectError(args ...string) (string, error) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func readCLIFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func writeCLIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
