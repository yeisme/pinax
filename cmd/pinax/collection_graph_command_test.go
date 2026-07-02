package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionImportDiffDoctorExportAndGraphCommands(t *testing.T) {
	root := t.TempDir()
	bundle := writeContentBundleFixture(t, root)

	dryRunOut := runCLI(t, "collection", "import", "--from", bundle, "--dry-run", "--vault", root, "--json")
	dryRun := parseCollectionEnvelope(t, dryRunOut)
	if dryRun["command"] != "collection.import" || dryRun["status"] != "success" {
		t.Fatalf("dry-run envelope = %#v", dryRun)
	}
	dryFacts := dryRun["facts"].(map[string]any)
	if dryFacts["items"] != "3" || dryFacts["complete_items"] != "2" || dryFacts["missing_prompt_items"] != "1" || dryFacts["local_write"] != "false" {
		t.Fatalf("dry-run facts = %#v", dryFacts)
	}
	if fileExists(filepath.Join(root, "notes", "collections", "upma-image-prompts", "upma-case-001.md")) {
		t.Fatalf("dry-run wrote collection notes")
	}

	blocked, err := runCLIExpectError("collection", "import", "--from", bundle, "--vault", root, "--json")
	if err == nil || !strings.Contains(blocked, "confirmation_required") {
		t.Fatalf("import without --yes should be blocked: err=%v out=%s", err, blocked)
	}

	importOut := runCLI(t, "collection", "import", "--from", bundle, "--yes", "--vault", root, "--json")
	imported := parseCollectionEnvelope(t, importOut)
	importFacts := imported["facts"].(map[string]any)
	if importFacts["imported_notes"] != "3" || importFacts["imported_prompts"] != "2" || importFacts["local_write"] != "true" {
		t.Fatalf("import facts = %#v", importFacts)
	}
	if !fileExists(filepath.Join(root, "notes", "collections", "upma-image-prompts", "upma-case-001.md")) {
		t.Fatalf("collection note was not written")
	}
	if !strings.Contains(importOut, ".pinax/receipts/") {
		t.Fatalf("import did not report receipt evidence: %s", importOut)
	}

	searchOut := runCLI(t, "prompt", "search", "storyboard", "--domain", "visual_generation", "--vault", root, "--json")
	searchEnvelope := parsePromptEnvelope(t, searchOut)
	if searchEnvelope["facts"].(map[string]any)["results"] != "1" {
		t.Fatalf("prompt search after collection import = %#v", searchEnvelope)
	}

	diffOut := runCLI(t, "collection", "diff", "--from", bundle, "--vault", root, "--json")
	diff := parseCollectionEnvelope(t, diffOut)
	diffFacts := diff["facts"].(map[string]any)
	if diffFacts["new_items"] != "0" || diffFacts["existing_items"] != "3" || diffFacts["missing_prompt_items"] != "1" {
		t.Fatalf("diff facts = %#v", diffFacts)
	}

	doctorOut := runCLI(t, "collection", "doctor", "--from", bundle, "--vault", root, "--json")
	doctor := parseCollectionEnvelope(t, doctorOut)
	doctorFacts := doctor["facts"].(map[string]any)
	if doctorFacts["status"] != "issues" || doctorFacts["missing_prompt_items"] != "1" {
		t.Fatalf("doctor facts = %#v", doctorFacts)
	}
	if strings.Contains(doctorOut, "full hidden prompt should not leak") {
		t.Fatalf("doctor leaked prompt body:\n%s", doctorOut)
	}

	exportPath := filepath.Join(root, "eikona-bundle.json")
	exportOut := runCLI(t, "collection", "export", "--to", exportPath, "--format", "eikona.prompt_bundle.v1", "--vault", root, "--json")
	exported := parseCollectionEnvelope(t, exportOut)
	if exported["facts"].(map[string]any)["exported_prompts"] != "2" {
		t.Fatalf("export facts = %#v", exported["facts"])
	}
	exportBody, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	var exportedBundle map[string]any
	if err := json.Unmarshal(exportBody, &exportedBundle); err != nil {
		t.Fatalf("export is not json: %v\n%s", err, string(exportBody))
	}
	if exportedBundle["schema_version"] != "eikona.prompt_bundle.v1" || len(exportedBundle["prompts"].([]any)) != 2 {
		t.Fatalf("unexpected export body: %#v", exportedBundle)
	}

	rebuildOut := runCLI(t, "graph", "rebuild", "--vault", root, "--json")
	rebuild := parseCollectionEnvelope(t, rebuildOut)
	if rebuild["facts"].(map[string]any)["nodes"] != "11" || rebuild["facts"].(map[string]any)["edges"] != "10" {
		t.Fatalf("graph rebuild facts = %#v", rebuild["facts"])
	}
	if !fileExists(filepath.Join(root, ".pinax", "graph", "prompt_graph.json")) {
		t.Fatalf("graph rebuild did not write prompt graph projection")
	}

	queryOut := runCLI(t, "graph", "query", "--kind", "technique", "--match", "storyboard", "--vault", root, "--json")
	query := parseCollectionEnvelope(t, queryOut)
	queryFacts := query["facts"].(map[string]any)
	if queryFacts["results"] != "1" || queryFacts["graph_engine"] != "prompt_graph" {
		t.Fatalf("graph query facts = %#v", queryFacts)
	}
	if strings.Contains(queryOut, "wide cinematic storyboard panel") {
		t.Fatalf("graph query leaked full prompt body:\n%s", queryOut)
	}

	agentOut := runCLI(t, "graph", "query", "--kind", "category", "--match", "poster", "--vault", root, "--agent")
	for _, want := range []string{"command=graph.query", "fact.results=1", "fact.graph_engine=prompt_graph"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent graph output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, root) || strings.Contains(agentOut, "full hidden prompt should not leak") {
		t.Fatalf("graph agent output leaked sensitive content:\n%s", agentOut)
	}
}

func writeContentBundleFixture(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "pinax-content-bundle.json")
	body := `{
  "schema_version": "pinax.content_bundle.v1",
  "id": "upma-image-prompts",
  "title": "Upma Image Prompts",
  "source": {"id":"upma-image-prompts","url":"https://www.upma.cn/image-prompts"},
  "items": [
    {"id":"upma-case-001","title":"Storyboard Case","category":"storyboard","language":"en","prompt":"wide cinematic storyboard panel with careful lighting","source_url":"https://www.upma.cn/image-prompts","featured":true,"techniques":["storyboard"],"styles":["cinematic"],"subjects":["city"],"tags":["storyboard"]},
    {"id":"upma-case-002","title":"Poster Case","category":"poster","language":"zh","prompt":"full hidden prompt should not leak","source_url":"https://www.upma.cn/image-prompts","techniques":["poster"],"styles":["minimal"],"subjects":["product"],"tags":["poster"]},
    {"id":"upma-case-003","title":"Unavailable Case","category":"poster","language":"ja","prompt":"","source_url":"https://www.upma.cn/image-prompts/3","tags":["placeholder"]}
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write content bundle fixture: %v", err)
	}
	return path
}

func parseCollectionEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("collection output is not JSON: %v\n%s", err, out)
	}
	return envelope
}
