package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestDoctorRepairAndOrganizeUseLinkEvidence(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	sourcePath := filepath.Join(root, "Source.md")
	writeCLIFixture(t, sourcePath, pinaxNoteFixture("note_source", "Source", "[]", "[[Missing Target]]\n\n[[Shared]]\n"))
	writeCLIFixture(t, filepath.Join(root, "First.md"), pinaxNoteFixture("note_first", "Shared", "[]", "first\n"))
	writeCLIFixture(t, filepath.Join(root, "Second.md"), pinaxNoteFixture("note_second", "Shared", "[]", "second\n"))
	writeCLIFixture(t, filepath.Join(root, "Orphan.md"), pinaxNoteFixture("note_orphan", "Orphan", "[]", "solo\n"))
	beforeSource := readCLIFile(t, sourcePath)

	doctorOut := runCLI(t, "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("doctor json invalid: %v\n%s", err, doctorOut)
	}
	doctorData := doctorEnvelope["data"].(map[string]any)
	issues := doctorData["issues"].([]any)
	for _, code := range []string{"broken_link", "ambiguous_link", "orphan_note"} {
		if !hasIssue(issues, code) {
			t.Fatalf("doctor issue %s missing: %#v", code, issues)
		}
	}

	repairOut := runCLI(t, "repair", "plan", "--vault", root, "--json")
	if got := readCLIFile(t, sourcePath); got != beforeSource {
		t.Fatalf("repair plan rewrote note body:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "repair-plans")) {
		t.Fatalf("repair plan without --save wrote repair plan asset")
	}
	var repairEnvelope map[string]any
	if err := json.Unmarshal([]byte(repairOut), &repairEnvelope); err != nil {
		t.Fatalf("repair plan json invalid: %v\n%s", err, repairOut)
	}
	repairOps := repairEnvelope["data"].(map[string]any)["operations"].([]any)
	for _, want := range []struct {
		kind   string
		target string
	}{
		{kind: "link_resolution", target: "Missing Target"},
		{kind: "link_rewrite", target: "Shared"},
		{kind: "orphan_review", target: "Orphan"},
	} {
		if !hasManualReviewOperation(repairOps, want.kind, want.target) {
			t.Fatalf("repair operation %s target %q missing: %#v", want.kind, want.target, repairOps)
		}
	}

	organizeOut := runCLI(t, "organize", "suggest", "--vault", root, "--json")
	if got := readCLIFile(t, sourcePath); got != beforeSource {
		t.Fatalf("organize suggest rewrote note body:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "organize-plans")) {
		t.Fatalf("organize suggest without --save wrote organize plan asset")
	}
	var organizeEnvelope map[string]any
	if err := json.Unmarshal([]byte(organizeOut), &organizeEnvelope); err != nil {
		t.Fatalf("organize suggest json invalid: %v\n%s", err, organizeOut)
	}
	organizeOps := organizeEnvelope["data"].(map[string]any)["operations"].([]any)
	for _, want := range []struct {
		kind   string
		target string
	}{
		{kind: "link_resolution", target: "Missing Target"},
		{kind: "link_rewrite", target: "Shared"},
		{kind: "orphan_review", target: "Orphan"},
	} {
		if !hasManualReviewOperation(organizeOps, want.kind, want.target) {
			t.Fatalf("organize operation %s target %q missing: %#v", want.kind, want.target, organizeOps)
		}
	}
}

func TestRepairApplyRequiresApprovalAndSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "No Tags.md"), pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
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
	if !strings.Contains(failed, "pinax version snapshot") || strings.Contains(failed, "pinax git snapshot") {
		t.Fatalf("snapshot error missing version action or leaked git action:\n%s", failed)
	}
}

func TestRepairApplyProjectionOnlyPlanDoesNotRequireSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved projection repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)
	data := savedEnvelope["data"].(map[string]any)
	ops := fmt.Sprint(data["operations"])
	if !strings.Contains(ops, "index_rebuild") || strings.Contains(ops, "metadata_patch") || strings.Contains(ops, "tags_patch") {
		t.Fatalf("expected projection-only operations, got %s", ops)
	}

	applyOut := runCLI(t, "repair", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if strings.Contains(applyOut, "snapshot_required") || !strings.Contains(applyOut, "repair.apply") {
		t.Fatalf("projection-only repair apply output = %s", applyOut)
	}
}

func TestRepairApplyLowRiskOperationsAndRejectsStalePlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "No Tags.md")
	writeCLIFixture(t, notePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
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
	writeCLIFixture(t, staleNotePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
	staleSavedOut := runCLI(t, "repair", "plan", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSavedOut), &staleEnvelope); err != nil {
		t.Fatalf("stale saved repair plan json invalid: %v\n%s", err, staleSavedOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleNotePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "changed\n"))
	failed, err := runCLIExpectError("repair", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "repair 前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax repair plan") {
		t.Fatalf("stale repair apply err=%v out=%s", err, failed)
	}
}

func TestOrganizeSuggestCreatesReviewableAgentPlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Research Idea.md"), pinaxNoteFixture("note_research_idea", "Research Idea", "[]", "body #research [[Missing Target]]\n\n![Missing](missing.png)\n"))
	writeCLIFixture(t, filepath.Join(root, ".agents", "skills", "Internal.md"), "# Internal\n\nagent asset\n")
	writeCLIFixture(t, filepath.Join(root, "docs", "Product.md"), "# Product\n\nproject doc\n")
	writeCLIFixture(t, filepath.Join(root, "AGENTS.md"), "# Agent Rules\n\nproject rules\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "# Daily 2026-06-06\n\nlog\n")

	out := runCLI(t, "organize", "suggest", "--vault", root, "--json")
	if fileExists(filepath.Join(root, ".pinax", "organize-plans")) {
		t.Fatalf("organize suggest without --save wrote organize-plans asset")
	}
	humanOut := runCLI(t, "organize", "suggest", "--vault", root)
	for _, want := range []string{"Operation preview", "Mode", "Risk", "Action", "Source", "Target", "Reason", "Research Idea.md", "pinax organize plan --vault", "--save"} {
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
	for _, kind := range []string{"move", "tag_patch", "kind_patch", "status_patch", "link_resolution", "attachment_repair"} {
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
	for _, want := range []string{"Saved path", "pinax organize apply --vault", "--snapshot-message", "snapshot before organize"} {
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
	for _, want := range []string{"Saved plans", "Planned", "Operation", savedData["plan_id"].(string), "pinax organize apply --vault", "--plan"} {
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
	for _, want := range []string{"Plans", "0", "Next step", "pinax organize plan --vault", "--save"} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty organize list output missing %q:\n%s", want, out)
		}
	}
}

func TestOrganizeApplySavedPlanRejectsStaleAndMoves(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "Research Idea.md")
	writeCLIFixture(t, source, pinaxNoteFixture("note_research_idea", "Research Idea", "[]", "body #research\n"))
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
	for _, want := range []string{"approval_required", "organize apply requires --yes", "pinax organize plan --vault", "--save", "pinax organize apply --vault", "--plan", "--snapshot-message"} {
		if !strings.Contains(failedHuman, want) {
			t.Fatalf("organize apply human error missing %q:\n%s", want, failedHuman)
		}
	}
	failed, err = runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if err == nil || !strings.Contains(failed, "snapshot_required") {
		t.Fatalf("organize apply saved plan without snapshot err=%v out=%s", err, failed)
	}
	if !strings.Contains(failed, "pinax version snapshot") || strings.Contains(failed, "pinax git snapshot") {
		t.Fatalf("organize snapshot error missing version action or leaked git action:\n%s", failed)
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
	writeCLIFixture(t, staleSource, pinaxNoteFixture("note_stale", "Stale Note", "[]", "body\n"))
	staleOut := runCLI(t, "organize", "suggest", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale organize suggest json invalid: %v\n%s", err, staleOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleSource, pinaxNoteFixture("note_stale", "Stale Note", "[]", "changed\n"))
	failed, err = runCLIExpectError("organize", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "整理前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax organize plan") {
		t.Fatalf("stale organize apply err=%v out=%s", err, failed)
	}
}
