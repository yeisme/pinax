package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrainAnswerPreviewIsEvidenceFirstAndBodySafe(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alice.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alice\ntitle: Alice Meeting\nkind: reference\n---\n\n# Alice Meeting\n\nAlice needs the roadmap update and budget status before Friday.\n\nSECRET_BODY_SENTINEL should not appear in answer preview.\n")
	runCLI(t, "index", "refresh", "--vault", root, "--json")

	out := runCLI(t, "brain", "answer", "Alice roadmap budget", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "brain.answer", "success")
	if strings.Contains(out, "SECRET_BODY_SENTINEL") || strings.Contains(out, "raw_provider_payload") || strings.Contains(out, "Authorization") {
		t.Fatalf("brain answer leaked forbidden content:\n%s", out)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("brain answer json invalid: %v\n%s", err, out)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.agent_brain.answer.v1" {
		t.Fatalf("schema_version = %#v", data["schema_version"])
	}
	for _, key := range []string{"answer", "claims", "sources", "open_questions", "next_actions", "cost", "body_exposure", "context_bundle"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("brain answer missing %s: %#v", key, data)
		}
	}
	if data["body_exposure"] != "bounded_projection" {
		t.Fatalf("body exposure = %#v", data["body_exposure"])
	}
	cost := data["cost"].(map[string]any)
	if cost["cost_class"] != "none" || cost["provider_id"] != "extractive" || cost["model"] != "none" || cost["network_required"] != false {
		t.Fatalf("cost = %#v", cost)
	}
	sources := data["sources"].([]any)
	if len(sources) == 0 {
		t.Fatalf("sources missing: %#v", data)
	}
}

func TestBrainMaintainPlanOnlyAndSavePlanEvidence(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\n\nCitation candidate. SECRET_BODY_SENTINEL Authorization: Bearer token\n")

	dryRun := runCLI(t, "brain", "maintain", "--dry-run", "--vault", root, "--json")
	assertJSONCommandStatus(t, dryRun, "brain.maintenance_plan", "success")
	if strings.Contains(dryRun, "SECRET_BODY_SENTINEL") || strings.Contains(dryRun, "Authorization") || strings.Contains(dryRun, "Bearer") {
		t.Fatalf("brain maintain dry-run leaked secret:\n%s", dryRun)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "brain-maintenance-plans")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create plan directory")
	}

	saved := runCLI(t, "brain", "maintain", "--save-plan", "--vault", root, "--json")
	assertJSONCommandStatus(t, saved, "brain.maintenance_plan", "success")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(saved), &envelope); err != nil {
		t.Fatalf("brain maintain json invalid: %v\n%s", err, saved)
	}
	facts := envelope["facts"].(map[string]any)
	planPath, _ := facts["plan_path"].(string)
	if planPath == "" || !fileExists(filepath.Join(root, filepath.FromSlash(planPath))) {
		t.Fatalf("saved plan_path invalid: %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.agent_brain.maintenance_plan.v1" || data["writes"] != false {
		t.Fatalf("maintenance plan data = %#v", data)
	}
}
