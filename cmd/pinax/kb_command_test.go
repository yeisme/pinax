package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKBRebuildSearchAndContextUseLanceDBSidecar(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "cloud-sync.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_cloud_sync\ntitle: Cloud Sync Design\ntags: [pinax, sync]\nkind: reference\n---\n\n# Cloud Sync Design\n\nPinax uses MinIO S3-compatible Cloud Sync for encrypted revisions.\n\n## LanceDB\n\nEach device rebuilds a local LanceDB semantic projection after pull.\n\nSECRET_BODY_SENTINEL should stay out of bounded agent context.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily\ntitle: Daily\ntags: [daily]\nkind: reference\n---\n\n# Daily\n\nLunch notes unrelated to vector search.\n")

	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("kb rebuild json invalid: %v\n%s", err, rebuildOut)
	}
	if rebuild["command"] != "kb.rebuild" || rebuild["status"] != "success" {
		t.Fatalf("kb rebuild envelope = %#v", rebuild)
	}
	facts := rebuild["facts"].(map[string]any)
	if facts["backend"] != "lancedb" || facts["provider"] != "fake" || facts["documents"] != "2" {
		t.Fatalf("kb rebuild facts = %#v", facts)
	}
	generationID, ok := facts["generation_id"].(string)
	if !ok || generationID == "" || facts["generation_status"] != "ready" {
		t.Fatalf("kb rebuild did not return a ready generation: %#v", facts)
	}
	if !fileExists(filepath.Join(root, ".pinax", "kb", "generations", generationID, "lancedb", "sidecar.jsonl")) {
		t.Fatalf("kb rebuild did not stage the sidecar store")
	}

	searchOut := runCLI(t, "kb", "search", "LanceDB semantic projection", "--vault", root, "--generation", generationID, "--agent")
	for _, want := range []string{"command=kb.search", "fact.backend=lancedb", "fact.matches=", "fact.provider=fake"} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("kb search agent missing %q:\n%s", want, searchOut)
		}
	}

	contextOut := runCLI(t, "kb", "context", "how should devices rebuild semantic search", "--vault", root, "--generation", generationID, "--limit", "1", "--json")
	var contextEnvelope map[string]any
	if err := json.Unmarshal([]byte(contextOut), &contextEnvelope); err != nil {
		t.Fatalf("kb context json invalid: %v\n%s", err, contextOut)
	}
	if contextEnvelope["command"] != "kb.context" || contextEnvelope["status"] != "success" {
		t.Fatalf("kb context envelope = %#v", contextEnvelope)
	}
	if strings.Contains(contextOut, "SECRET_BODY_SENTINEL") || strings.Contains(contextOut, "raw_body") || strings.Contains(contextOut, "\"body\"") {
		t.Fatalf("kb context leaked full body field/content:\n%s", contextOut)
	}
}

func TestKBEvaluateCandidateWritesReceiptWithoutActivation(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "evaluation.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_eval\ntitle: Evaluation Note\nkind: reference\nstatus: active\n---\n\n# Retrieval\n\nStaged LanceDB retrieval is evaluated before activation.\n")
	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("rebuild JSON invalid: %v\n%s", err, rebuildOut)
	}
	generationID := rebuild["facts"].(map[string]any)["generation_id"].(string)
	questions := make([]map[string]any, 20)
	for i := range questions {
		questions[i] = map[string]any{
			"question_id":        fmt.Sprintf("q-%03d", i+1),
			"query":              "staged LanceDB retrieval",
			"expected_citations": []string{"notes/evaluation.md#Retrieval"},
		}
	}
	suite := map[string]any{"schema_version": "pinax.kb.evaluation-suite.v1", "suite_id": "local-canary", "version": "2026-08-02", "questions": questions}
	suitePayload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	writeCLIFixture(t, filepath.Join(root, ".pinax", "kb", "evaluation-suites", "local-canary.json"), string(suitePayload))

	evaluateOut := runCLI(t, "kb", "evaluate", "--suite", ".pinax/kb/evaluation-suites/local-canary.json", "--generation", generationID, "--vault", root, "--json")
	var evaluation map[string]any
	if err := json.Unmarshal([]byte(evaluateOut), &evaluation); err != nil {
		t.Fatalf("evaluate JSON invalid: %v\n%s", err, evaluateOut)
	}
	if evaluation["command"] != "kb.evaluate" || evaluation["status"] != "success" {
		t.Fatalf("evaluate envelope = %#v", evaluation)
	}
	facts := evaluation["facts"].(map[string]any)
	if facts["status"] != "passed" || facts["generation_id"] != generationID || facts["activation_unchanged"] != "true" {
		t.Fatalf("evaluate facts = %#v", facts)
	}
	if fileExists(filepath.Join(root, ".pinax", "kb", "activation.json")) {
		t.Fatalf("candidate evaluation should not create activation descriptor")
	}
}

func TestKBEvaluateCLIOutputModesKeepContract(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "modes.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_modes\ntitle: Output Modes\nkind: reference\nstatus: active\n---\n\n# Modes\n\nCLI evaluation output modes share one bounded projection contract.\n")
	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("rebuild JSON invalid: %v\n%s", err, rebuildOut)
	}
	generationID := rebuild["facts"].(map[string]any)["generation_id"].(string)
	questions := make([]map[string]any, 20)
	for i := range questions {
		questions[i] = map[string]any{
			"question_id":        fmt.Sprintf("q-%03d", i+1),
			"query":              "bounded projection contract",
			"expected_citations": []string{"notes/modes.md#Modes"},
		}
	}
	suiteRel := ".pinax/kb/evaluation-suites/output-modes.json"
	suitePayload, err := json.Marshal(map[string]any{"schema_version": "pinax.kb.evaluation-suite.v1", "suite_id": "output-modes", "version": "2026-08-02", "questions": questions})
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(suiteRel)), string(suitePayload))
	args := []string{"kb", "evaluate", "--suite", suiteRel, "--generation", generationID, "--vault", root}

	jsonOut := runCLI(t, append(args, "--json")...)
	assertMachineOutputClean(t, jsonOut)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("evaluate JSON invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "kb.evaluate" || envelope["status"] != "success" || envelope["facts"].(map[string]any)["activation_unchanged"] != "true" {
		t.Fatalf("evaluate JSON envelope = %#v", envelope)
	}

	agentOut := runCLI(t, append(args, "--agent")...)
	assertMachineOutputClean(t, agentOut)
	for _, want := range []string{"command=kb.evaluate", "status=success", "fact.status=passed", "fact.activation_unchanged=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("evaluate agent missing %q:\n%s", want, agentOut)
		}
	}

	eventsOut := runCLI(t, append(args, "--events")...)
	assertMachineOutputClean(t, eventsOut)
	assertNDJSONEvents(t, eventsOut, "kb.evaluate")

	explainOut := runCLI(t, append(args, "--explain")...)
	if !strings.Contains(explainOut, "Conclusion:") || !strings.Contains(explainOut, "Evidence:") || strings.Contains(explainOut, "bounded projection contract") {
		t.Fatalf("evaluate explain output is not bounded: %s", explainOut)
	}
}

func TestKBActivateCLIUsesMatchingEvaluationReceipt(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "activation.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_activate\ntitle: Activation Note\nkind: reference\nstatus: active\n---\n\n# Activation\n\nA passed candidate can become the active generation.\n")
	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("rebuild JSON invalid: %v\n%s", err, rebuildOut)
	}
	generationID := rebuild["facts"].(map[string]any)["generation_id"].(string)
	questions := make([]map[string]any, 20)
	for i := range questions {
		questions[i] = map[string]any{"question_id": fmt.Sprintf("q-%03d", i+1), "query": "passed candidate", "expected_citations": []string{"notes/activation.md#Activation"}}
	}
	suitePayload, err := json.Marshal(map[string]any{"schema_version": "pinax.kb.evaluation-suite.v1", "suite_id": "activation-canary", "version": "2026-08-02", "questions": questions})
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	suiteRel := ".pinax/kb/evaluation-suites/activation-canary.json"
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(suiteRel)), string(suitePayload))
	evaluateOut := runCLI(t, "kb", "evaluate", "--suite", suiteRel, "--generation", generationID, "--vault", root, "--json")
	var evaluation map[string]any
	if err := json.Unmarshal([]byte(evaluateOut), &evaluation); err != nil {
		t.Fatalf("evaluate JSON invalid: %v\n%s", err, evaluateOut)
	}
	runID := evaluation["facts"].(map[string]any)["run_id"].(string)
	activateOut := runCLI(t, "kb", "activate", "--generation", generationID, "--suite", suiteRel, "--run-id", runID, "--vault", root, "--json")
	var activation map[string]any
	if err := json.Unmarshal([]byte(activateOut), &activation); err != nil {
		t.Fatalf("activate JSON invalid: %v\n%s", err, activateOut)
	}
	if activation["command"] != "kb.activate" || activation["status"] != "success" || activation["facts"].(map[string]any)["status"] != "active" {
		t.Fatalf("activate envelope = %#v", activation)
	}
	if !fileExists(filepath.Join(root, ".pinax", "kb", "activation.json")) {
		t.Fatalf("activate did not write activation descriptor")
	}
}

func TestKBLanceDBRequiresSidecar(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PINAX_KB_SIDECAR", filepath.Join(root, "missing-sidecar"))
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	if err == nil {
		t.Fatalf("kb rebuild should require lancedb sidecar:\n%s", out)
	}
	assertJSONErrorCode(t, out, "kb_sidecar_unavailable")
}

func TestKBGenerationPruneCLIIsDryRunByDefault(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "prune.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_prune\ntitle: Prune\nkind: reference\nstatus: active\n---\n\n# Prune\n\nCandidate retention is explicit.\n")
	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--allow-disk-high-water", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("rebuild JSON invalid: %v\n%s", err, rebuildOut)
	}
	generationID := rebuild["facts"].(map[string]any)["generation_id"].(string)
	pruneOut := runCLI(t, "kb", "generations", "prune", "--keep", "0", "--vault", root, "--json")
	var prune map[string]any
	if err := json.Unmarshal([]byte(pruneOut), &prune); err != nil {
		t.Fatalf("prune JSON invalid: %v\n%s", err, pruneOut)
	}
	if prune["command"] != "kb.generations.prune" || prune["status"] != "success" {
		t.Fatalf("prune envelope = %#v", prune)
	}
	facts := prune["facts"].(map[string]any)
	if facts["dry_run"] != "true" || facts["deleted"] != "0" || facts["delete_candidates"] != "1" {
		t.Fatalf("prune facts = %#v", facts)
	}
	if !fileExists(filepath.Join(root, ".pinax", "kb", "generations", generationID)) {
		t.Fatalf("dry-run removed generation %s", generationID)
	}
	applyOut, err := runCLIExpectError("kb", "generations", "prune", "--keep", "0", "--dry-run=false", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("prune apply without --yes should fail:\n%s", applyOut)
	}
	assertJSONErrorCode(t, applyOut, "approval_required")
}

func TestKBRejectsUnknownProvider(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("kb", "rebuild", "--vault", root, "--backend", "fake", "--provider", "gemni", "--allow-disk-high-water", "--json")
	if err == nil {
		t.Fatalf("kb rebuild should reject unknown provider:\n%s", out)
	}
	assertJSONErrorCode(t, out, "provider_invalid")
}

func TestKBProviderListAndDoctorContracts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	runCLI(t, "init", root, "--title", "Vault", "--json")

	listOut := runCLI(t, "kb", "provider", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listOut, "kb.provider.list", "success")
	for _, want := range []string{`"name":"gemini"`, `"name":"openai"`, `"name":"ollama"`, `"name":"fake"`, `"credential_source":"env:OPENAI_API_KEY"`, `"local_only":true`} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("provider list missing %q:\n%s", want, listOut)
		}
	}
	for _, forbidden := range []string{"sk-", "Authorization", "Bearer"} {
		if strings.Contains(listOut, forbidden) {
			t.Fatalf("provider list leaked %q:\n%s", forbidden, listOut)
		}
	}

	agentOut := runCLI(t, "kb", "provider", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=kb.provider.list", "fact.providers=4", "fact.default_provider=gemini", "provider.4.name=fake", "provider.4.default_model=fake-hash-v1", "provider.4.configured=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("provider list agent missing %q:\n%s", want, agentOut)
		}
	}
	defaultListOut := runCLI(t, "kb", "provider", "list", "--vault", root)
	for _, want := range []string{"Providers", "Provider", "Model", "Configured", "Credential", "Local only", "gemini", "openai", "ollama", "fake", "fake-hash-v1", "env:OPENAI_API_KEY"} {
		if !strings.Contains(defaultListOut, want) {
			t.Fatalf("provider list default output missing %q:\n%s", want, defaultListOut)
		}
	}
	for _, forbidden := range []string{"sk-", "Authorization", "Bearer"} {
		if strings.Contains(defaultListOut, forbidden) {
			t.Fatalf("provider list default output leaked %q:\n%s", forbidden, defaultListOut)
		}
	}

	fakeDoctor := runCLI(t, "kb", "provider", "doctor", "fake", "--vault", root, "--json")
	assertJSONCommandStatus(t, fakeDoctor, "kb.provider.doctor", "success")
	if !strings.Contains(fakeDoctor, `"available":true`) || !strings.Contains(fakeDoctor, `"embed_ready":true`) || !strings.Contains(fakeDoctor, `"provider":"fake"`) {
		t.Fatalf("fake doctor output invalid:\n%s", fakeDoctor)
	}

	missingOpenAI, err := runCLIExpectError("kb", "provider", "doctor", "openai", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("openai doctor without key should fail:\n%s", missingOpenAI)
	}
	assertJSONErrorCode(t, missingOpenAI, "provider_not_configured")
	for _, forbidden := range []string{"Authorization", "Bearer", "raw_provider_payload", "provider_payload"} {
		if strings.Contains(missingOpenAI, forbidden) {
			t.Fatalf("provider doctor leaked %q:\n%s", forbidden, missingOpenAI)
		}
	}
}

func TestKBImportTextCopiesIntoVaultAndKeepsDryRunReadOnly(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(source, "idea.txt"), "Local-first semantic notebook with Gemini embeddings.\n")

	dryRun := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--dry-run", "--json")
	assertJSONCommandStatus(t, dryRun, "kb.import", "success")
	if strings.Contains(runCLI(t, "note", "list", "--vault", root, "--json"), "idea") {
		t.Fatalf("kb import dry-run wrote note")
	}

	importOut := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &envelope); err != nil {
		t.Fatalf("kb import json invalid: %v\n%s", err, importOut)
	}
	if envelope["command"] != "kb.import" || envelope["status"] != "success" {
		t.Fatalf("kb import envelope = %#v", envelope)
	}
	if envelope["facts"].(map[string]any)["imported"] != "1" {
		t.Fatalf("kb import facts = %#v", envelope["facts"])
	}
}

func TestKBImportDuplicateTitlesDoNotOverwrite(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(source, "a", "idea.txt"), "Alpha")
	writeCLIFixture(t, filepath.Join(source, "b", "idea.txt"), "Beta")

	importOut := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &envelope); err != nil {
		t.Fatalf("kb import json invalid: %v\n%s", err, importOut)
	}
	if envelope["facts"].(map[string]any)["imported"] != "2" {
		t.Fatalf("kb import facts = %#v", envelope["facts"])
	}
	matches, err := filepath.Glob(filepath.Join(root, "notes", "kb", "imports", "idea*.md"))
	if err != nil || len(matches) != 2 {
		t.Fatalf("imported files = %#v err=%v", matches, err)
	}
}

func writeFakeKBSidecar(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, pathlib, sys

op = sys.argv[1] if len(sys.argv) > 1 else ""
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)
sidecar_path = store / "sidecar.jsonl"

if op == "doctor":
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","dependency":"fake-sidecar"}))
elif op == "rebuild":
    records = req.get("records", [])
    with sidecar_path.open("w", encoding="utf-8") as f:
        for record in records:
            assert "chunk_text" not in record
            assert "vault_path" not in record.get("metadata", {})
            f.write(json.dumps(record, ensure_ascii=False) + "\n")
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(records)}))
elif op == "search":
    rows = []
    if sidecar_path.exists():
        rows = [json.loads(line) for line in sidecar_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    hits = []
    for idx, row in enumerate(rows[:req.get("limit", 8) or 8]):
        hits.append({"id": row["id"], "score": 1.0 - idx * 0.01, "metadata": row.get("metadata", {})})
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","total":len(hits),"hits":hits}))
else:
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake kb sidecar: %v", err)
	}
	return path
}
