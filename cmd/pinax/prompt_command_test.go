package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptImportSearchShowResolveCommands(t *testing.T) {
	root := t.TempDir()
	assetFile := writePromptAssetFixture(t, root, "novel_character_portrait_v1")

	importOut := runCLI(t, "prompt", "import", "--from", assetFile, "--vault", root, "--json")
	envelope := parsePromptEnvelope(t, importOut)
	if envelope["command"] != "prompt.import" || envelope["status"] != "success" {
		t.Fatalf("import envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["prompt_asset_id"] != "novel_character_portrait_v1" || facts["permission"] != "internal" || facts["lifecycle"] != "draft" {
		t.Fatalf("import facts = %#v", facts)
	}

	searchOut := runCLI(t, "prompt", "search", "portrait", "--domain", "visual_generation", "--tag", "character", "--vault", root, "--json")
	searchEnvelope := parsePromptEnvelope(t, searchOut)
	if searchEnvelope["command"] != "prompt.search" {
		t.Fatalf("search command = %#v", searchEnvelope)
	}
	searchFacts := searchEnvelope["facts"].(map[string]any)
	if searchFacts["results"] != "1" {
		t.Fatalf("search facts = %#v", searchFacts)
	}

	showOut := runCLI(t, "prompt", "show", "novel_character_portrait_v1", "--vault", root, "--json")
	showEnvelope := parsePromptEnvelope(t, showOut)
	showFacts := showEnvelope["facts"].(map[string]any)
	if showFacts["prompt_asset_id"] != "novel_character_portrait_v1" || showFacts["version"] == "" {
		t.Fatalf("show facts = %#v", showFacts)
	}

	agentOut := runCLI(t, "prompt", "resolve", "pinax://prompt/novel_character_portrait_v1", "--vault", root, "--agent")
	for _, want := range []string{"command=prompt.resolve", "fact.prompt_asset_id=novel_character_portrait_v1", "fact.lifecycle=draft", "fact.permission=internal", "action.resolve="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, root) || strings.Contains(agentOut, "Create a portrait") {
		t.Fatalf("agent output leaked path or prompt body:\n%s", agentOut)
	}
}

func TestPromptCreateCommandImportsFixture(t *testing.T) {
	root := t.TempDir()
	assetFile := writePromptAssetFixture(t, root, "novel_character_portrait_v2")

	out := runCLI(t, "prompt", "create", "--from", assetFile, "--vault", root, "--json")
	envelope := parsePromptEnvelope(t, out)
	if envelope["command"] != "prompt.create" || envelope["status"] != "success" {
		t.Fatalf("create envelope = %#v", envelope)
	}
	if envelope["facts"].(map[string]any)["prompt_asset_id"] != "novel_character_portrait_v2" {
		t.Fatalf("create facts = %#v", envelope["facts"])
	}
}

func TestPromptImportRejectsInvalidSchema(t *testing.T) {
	root := t.TempDir()
	badFile := filepath.Join(root, "bad.yaml")
	if err := os.WriteFile(badFile, []byte("schema_version: yeisme.prompt_asset.v1\nid: bad\ndomain: visual_generation\nvariables: {}\n"), 0o644); err != nil {
		t.Fatalf("write invalid fixture: %v", err)
	}

	out, err := runCLIExpectError("prompt", "import", "--from", badFile, "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "prompt_asset_invalid") || !strings.Contains(out, "permission") || !strings.Contains(out, "prompt_template") {
		t.Fatalf("invalid import err=%v out=%s", err, out)
	}
}

func TestPromptShowRejectsUnknownAsset(t *testing.T) {
	root := t.TempDir()
	out, err := runCLIExpectError("prompt", "show", "missing_prompt", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "prompt_asset_not_found") {
		t.Fatalf("missing prompt err=%v out=%s", err, out)
	}
}

func TestPromptLifecycleAndFeedbackCommands(t *testing.T) {
	root := t.TempDir()
	assetFile := writePromptAssetFixture(t, root, "novel_character_portrait_v3")
	runCLI(t, "prompt", "import", "--from", assetFile, "--vault", root, "--json")

	lifecycleOut := runCLI(t, "prompt", "lifecycle", "novel_character_portrait_v3", "--to", "tested", "--reason", "fixture render passed", "--vault", root, "--json")
	lifecycleEnvelope := parsePromptEnvelope(t, lifecycleOut)
	if lifecycleEnvelope["command"] != "prompt.lifecycle" || lifecycleEnvelope["status"] != "success" {
		t.Fatalf("lifecycle envelope = %#v", lifecycleEnvelope)
	}
	if lifecycleEnvelope["facts"].(map[string]any)["lifecycle"] != "tested" {
		t.Fatalf("lifecycle facts = %#v", lifecycleEnvelope["facts"])
	}

	feedbackFile := filepath.Join(root, "feedback.json")
	feedbackBody := `{"schema_version":"eikona.prompt_usage_feedback.v1","feedback_id":"feedback_001","prompt_asset_id":"novel_character_portrait_v3","external_run_ref":"eikona://run/001","decision":"accepted","reason":"accepted fixture","artifact_refs":["eikona://artifact/001"]}`
	if err := os.WriteFile(feedbackFile, []byte(feedbackBody), 0o644); err != nil {
		t.Fatalf("write feedback fixture: %v", err)
	}
	feedbackOut := runCLI(t, "prompt", "feedback", "import", "--from", feedbackFile, "--vault", root, "--json")
	feedbackEnvelope := parsePromptEnvelope(t, feedbackOut)
	if feedbackEnvelope["command"] != "prompt.feedback.import" || feedbackEnvelope["facts"].(map[string]any)["imported"] != "true" {
		t.Fatalf("feedback envelope = %#v", feedbackEnvelope)
	}
	duplicateOut := runCLI(t, "prompt", "feedback", "import", "--from", feedbackFile, "--vault", root, "--json")
	duplicateEnvelope := parsePromptEnvelope(t, duplicateOut)
	if duplicateEnvelope["facts"].(map[string]any)["imported"] != "false" {
		t.Fatalf("duplicate feedback envelope = %#v", duplicateEnvelope)
	}

	agentOut := runCLI(t, "prompt", "feedback", "import", "--from", feedbackFile, "--vault", root, "--agent")
	if strings.Contains(agentOut, root) || strings.Contains(agentOut, "artifact/001") {
		t.Fatalf("feedback agent output leaked path or artifact detail:\n%s", agentOut)
	}
}

func writePromptAssetFixture(t *testing.T, root, id string) string {
	t.Helper()
	path := filepath.Join(root, id+".yaml")
	body := "schema_version: yeisme.prompt_asset.v1\n" +
		"id: " + id + "\n" +
		"title: Novel character portrait\n" +
		"domain: visual_generation\n" +
		"tags: [character, portrait]\n" +
		"lifecycle: draft\n" +
		"permission: internal\n" +
		"variables:\n" +
		"  character_name:\n" +
		"    type: string\n" +
		"    required: true\n" +
		"prompt_template: |\n" +
		"  Create a portrait of {{character_name}}.\n" +
		"source_refs:\n" +
		"  - uri: pinax://note/character-brief\n" +
		"    label: Character brief\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write prompt asset fixture: %v", err)
	}
	return path
}

func parsePromptEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("prompt output is not JSON: %v\n%s", err, out)
	}
	return envelope
}
