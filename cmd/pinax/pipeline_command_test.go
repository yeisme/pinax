package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pipeline_command_test.go 覆盖 pinax pipeline status/show 命令面：
// 聚合视图（human/json/agent 三模式）、freshness 徽标、--kind 过滤、
// 未知 id 稳定错误、completion 覆盖与命令树归组。

func pipelineFixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_pipeline_alpha\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-01\nstatus: active\n---\n\n# Alpha\n\nalpha body\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "orphan.md"), "plain markdown without frontmatter\n")
	return root
}

func TestPipelineStatusAggregatesPlansAndReceipts(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)

	// 两条 pending plans + 一条 metadata apply receipt。
	runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	metadataPlan := runCLI(t, "metadata", "plan", "--vault", root, "--save", "--json")
	var planEnvelope map[string]any
	if err := json.Unmarshal([]byte(metadataPlan), &planEnvelope); err != nil {
		t.Fatalf("metadata plan json invalid: %v\n%s", err, metadataPlan)
	}
	planID := planEnvelope["facts"].(map[string]any)["plan_id"].(string)
	runCLI(t, "metadata", "apply", "--vault", root, "--plan", planID, "--yes", "--json")

	human := runCLI(t, "pipeline", "status", "--vault", root)
	for _, want := range []string{"Pending plans (2)", "Recent runs (1)", "repair", "metadata", "metadata.apply", "applied", "pinax pipeline show"} {
		if !strings.Contains(human, want) {
			t.Fatalf("pipeline status human missing %q:\n%s", want, human)
		}
	}

	jsonOut := runCLI(t, "pipeline", "status", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("pipeline status json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "pipeline.status" || envelope["status"] != "success" {
		t.Fatalf("envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["plans"] != "2" || facts["receipts"] != "1" || facts["schema_version"] != "pinax.plan.v1" {
		t.Fatalf("facts = %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	if data["readonly"] != true {
		t.Fatalf("readonly = %#v", data["readonly"])
	}
	plans := data["plans"].([]any)
	kinds := map[string]bool{}
	for _, item := range plans {
		plan := item.(map[string]any)
		kinds[plan["kind"].(string)] = true
		if plan["schema_version"] != "pinax.plan.v1" {
			t.Fatalf("plan schema = %#v", plan)
		}
		if _, ok := plan["fresh"]; !ok {
			t.Fatalf("freshness missing: %#v", plan)
		}
		if _, ok := plan["op_counts"]; !ok {
			t.Fatalf("op counts missing: %#v", plan)
		}
	}
	if !kinds["repair"] || !kinds["metadata"] {
		t.Fatalf("plan kinds = %#v", kinds)
	}

	agent := runCLI(t, "pipeline", "status", "--vault", root, "--agent")
	for _, want := range []string{"command=pipeline.status", "status=success", "fact.plans=2", "fact.receipts=1"} {
		if !strings.Contains(agent, want) {
			t.Fatalf("pipeline status agent missing %q:\n%s", want, agent)
		}
	}
}

func TestPipelineStatusKindFilterAndLimit(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	runCLI(t, "metadata", "plan", "--vault", root, "--save", "--json")

	filtered := runCLI(t, "pipeline", "status", "--vault", root, "--kind", "repair", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(filtered), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, filtered)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["plans"] != "1" || facts["filter.kind"] != "repair" {
		t.Fatalf("filtered facts = %#v", facts)
	}

	limited := runCLI(t, "pipeline", "status", "--vault", root, "--limit", "0", "--json")
	var limitEnvelope map[string]any
	if err := json.Unmarshal([]byte(limited), &limitEnvelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, limited)
	}
	if limitEnvelope["facts"].(map[string]any)["limit"] != "10" {
		t.Fatalf("default limit facts = %#v", limitEnvelope["facts"])
	}
}

func TestPipelineStatusMarksStalePlan(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")

	// plan 之后改 vault ⇒ STALE 徽标。
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_pipeline_alpha\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-02\nstatus: active\n---\n\n# Alpha\n\nchanged body\n")
	out := runCLI(t, "pipeline", "status", "--vault", root)
	if !strings.Contains(out, "STALE") {
		t.Fatalf("stale badge missing:\n%s", out)
	}
	jsonOut := runCLI(t, "pipeline", "status", "--vault", root, "--json")
	if !strings.Contains(jsonOut, `"fresh":false`) {
		t.Fatalf("fresh=false missing:\n%s", jsonOut)
	}
}

func TestPipelineStatusUnreadablePlanFailsClosed(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	dir := filepath.Join(root, ".pinax", "repair-plans")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("repair plans: %v %#v", err, entries)
	}
	if err := os.WriteFile(filepath.Join(dir, entries[0].Name()), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "pipeline", "status", "--vault", root)
	for _, want := range []string{"Unreadable plans (1)", "repair"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unreadable human output missing %q:\n%s", want, out)
		}
	}
	jsonOut := runCLI(t, "pipeline", "status", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["status"] != "partial" {
		t.Fatalf("status = %#v", envelope["status"])
	}
	data := envelope["data"].(map[string]any)
	unreadable := data["unreadable"].([]any)
	if len(unreadable) != 1 {
		t.Fatalf("unreadable = %#v", unreadable)
	}
	entry := unreadable[0].(map[string]any)
	if entry["kind"] != "repair" || entry["error"] == "" {
		t.Fatalf("unreadable entry = %#v", entry)
	}
}

func TestPipelineShowPlanFormGroupsOperations(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	planOut := runCLI(t, "metadata", "plan", "--vault", root, "--save", "--json")
	var planEnvelope map[string]any
	if err := json.Unmarshal([]byte(planOut), &planEnvelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, planOut)
	}
	planID := planEnvelope["facts"].(map[string]any)["plan_id"].(string)

	human := runCLI(t, "pipeline", "show", planID, "--vault", root)
	for _, want := range []string{planID, "metadata", "pinax.metadata_plan.v1", "fresh", "Metadata writes", "Changed paths", "pinax metadata apply --vault"} {
		if !strings.Contains(human, want) {
			t.Fatalf("pipeline show human missing %q:\n%s", want, human)
		}
	}
	if strings.Contains(human, "Vault writes (") {
		t.Fatalf("metadata plan should not contain vault writes:\n%s", human)
	}

	jsonOut := runCLI(t, "pipeline", "show", planID, "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "pipeline.show" {
		t.Fatalf("command = %#v", envelope["command"])
	}
	data := envelope["data"].(map[string]any)
	operations := data["operations"].([]any)
	if len(operations) == 0 {
		t.Fatalf("operations empty: %#v", data)
	}
	for _, item := range operations {
		op := item.(map[string]any)
		if op["group"] != "metadata_write" {
			t.Fatalf("op group = %#v", op)
		}
	}
	if data["next"] == "" {
		t.Fatalf("next missing: %#v", data)
	}
}

func TestPipelineShowReceiptForm(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	applyOut := runCLI(t, "metadata", "apply", "--vault", root, "--yes", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, applyOut)
	}
	receiptID := applyEnvelope["facts"].(map[string]any)["receipt_id"].(string)

	human := runCLI(t, "pipeline", "show", receiptID, "--vault", root)
	for _, want := range []string{receiptID, "metadata.apply", "Applied", "Changed paths"} {
		if !strings.Contains(human, want) {
			t.Fatalf("pipeline show receipt human missing %q:\n%s", want, human)
		}
	}

	jsonOut := runCLI(t, "pipeline", "show", receiptID, "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, jsonOut)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["pipeline"] != "metadata" || facts["status"] != "applied" || facts["ledger_seq"] == "" {
		t.Fatalf("facts = %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	if _, ok := data["changed_paths"].([]any); !ok {
		t.Fatalf("changed paths = %#v", data)
	}
}

func TestPipelineShowUnknownIDStableError(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	out, err := runCLIExpectError("pipeline", "show", "plan-nope", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("unknown id should fail:\n%s", out)
	}
	if !strings.Contains(out, "pipeline_id_not_found") {
		t.Fatalf("error code missing:\n%s", out)
	}
	if !strings.Contains(out, "pinax pipeline status") {
		t.Fatalf("hint missing:\n%s", out)
	}
}

func TestPipelineCommandTreeAndCompletion(t *testing.T) {
	t.Parallel()
	root := pipelineFixtureVault(t)
	runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")

	help := runCLI(t, "--help")
	if strings.Contains(help, "Other\n") {
		t.Fatalf("pipeline must be grouped:\n%s", help)
	}
	catalog := runCLI(t, "commands")
	if !strings.Contains(catalog, "pipeline") {
		t.Fatalf("command catalog missing pipeline:\n%s", catalog)
	}

	// completion：plan id / receipt id 补全 + kind flag 补全。
	completion := runCLI(t, "__complete", "pipeline", "show", "--vault", root, "")
	if !strings.Contains(completion, "repair-") || !strings.Contains(completion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("pipeline show completion missing plan id:\n%s", completion)
	}
	kindCompletion := runCLI(t, "__complete", "pipeline", "status", "--kind", "")
	for _, want := range []string{"organize", "metadata", "repair", "restore", "sync", "publish", "proof_loop"} {
		if !strings.Contains(kindCompletion, want) {
			t.Fatalf("kind completion missing %q:\n%s", want, kindCompletion)
		}
	}
}
