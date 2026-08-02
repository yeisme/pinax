package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCanarySuiteUsesTwentyBoundedQuestions(t *testing.T) {
	vault := t.TempDir()
	if err := writeCanarySuite(vault); err != nil {
		t.Fatalf("write canary suite: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(vault, ".pinax", "kb", "evaluation-suites", "local-canary.json"))
	if err != nil {
		t.Fatalf("read canary suite: %v", err)
	}
	var suite map[string]any
	if err := json.Unmarshal(payload, &suite); err != nil {
		t.Fatalf("suite json: %v", err)
	}
	questions, ok := suite["questions"].([]any)
	if !ok || len(questions) != 20 {
		t.Fatalf("questions = %#v", suite["questions"])
	}
}

func TestWriteCanaryFailureOnlyPersistsSafeStageAndCode(t *testing.T) {
	dir := t.TempDir()
	err := writeCanaryFailure(dir, "rebuild", os.ErrNotExist)
	if err == nil {
		t.Fatalf("writeCanaryFailure should return the terminal failure")
	}
	payload, readErr := os.ReadFile(filepath.Join(dir, "failure.json"))
	if readErr != nil {
		t.Fatalf("read failure artifact: %v", readErr)
	}
	text := string(payload)
	if !strings.Contains(text, `"stage": "rebuild"`) || !strings.Contains(text, `"status": "failed"`) || strings.Contains(text, "/") {
		t.Fatalf("failure artifact is not bounded: %s", text)
	}
}

func TestParseOllamaProcessSnapshotKeepsOnlyResourceFacts(t *testing.T) {
	payload := []byte(`{"models":[{"name":"pinax-qwen3-embedding:lowmem","size":123456,"size_vram":654321},{"name":"other","size":100,"size_vram":200}]}`)
	stats, err := parseOllamaProcessSnapshot(payload)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if stats.ModelCount != 2 || stats.TotalModelBytes != 123556 || stats.TotalVRAMBytes != 654521 {
		t.Fatalf("resource stats = %#v", stats)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal resource stats: %v", err)
	}
	if strings.Contains(string(encoded), "pinax-qwen3") {
		t.Fatalf("resource stats retained a model name: %s", encoded)
	}
}

func TestOllamaUnloadStatusIsTruthful(t *testing.T) {
	if got := ollamaUnloadStatus(ollamaProcessStats{}); got != "unloaded" {
		t.Fatalf("empty process status = %q, want unloaded", got)
	}
	if got := ollamaUnloadStatus(ollamaProcessStats{ModelCount: 1}); got != "still_loaded" {
		t.Fatalf("loaded process status = %q, want still_loaded", got)
	}
}

func TestResourceHelpersDoNotInventUnavailableProcessFacts(t *testing.T) {
	maxRSS, user, system := processResourceUsage(nil)
	if maxRSS != 0 || user != 0 || system != 0 {
		t.Fatalf("nil process usage = %d/%d/%d, want zeros", maxRSS, user, system)
	}
	if got := maxInt64(3, 9, 4); got != 9 {
		t.Fatalf("maxInt64 = %d, want 9", got)
	}
}

func TestRunShadowComparePersistsBoundedArtifact(t *testing.T) {
	artifactDir := t.TempDir()
	artifact, err := runShadowCompare(artifactDir, writeShadowCompareTestSidecar(t))
	if err != nil {
		t.Fatalf("run shadow compare: %v", err)
	}
	if artifact["citation_identity_match"] != true || artifact["safe_citations"] != true || artifact["permission_empty_semantics_match"] != true {
		t.Fatalf("shadow artifact facts = %#v", artifact)
	}
	if artifact["documents_legacy"] != 2 || artifact["documents_inferrum"] != 2 || artifact["chunks_legacy"] != 2 || artifact["chunks_inferrum"] != 2 {
		t.Fatalf("shadow artifact counts = %#v", artifact)
	}
	payload, err := os.ReadFile(filepath.Join(artifactDir, "shadow-compare.json"))
	if err != nil {
		t.Fatalf("read shadow artifact: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"query_vector", "chunk_text", "vault_path", "raw_prompt", "provider_payload", "SECRET"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("shadow artifact leaked %q: %s", forbidden, text)
		}
	}
}

func TestOllamaBaseURLUsesLoopbackDefaultAndTrimsSlash(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	if got := ollamaBaseURL(); got != "http://127.0.0.1:11434" {
		t.Fatalf("default Ollama URL = %q", got)
	}
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:11434/")
	if got := ollamaBaseURL(); got != "http://127.0.0.1:11434" {
		t.Fatalf("trimmed Ollama URL = %q", got)
	}
}

func writeShadowCompareTestSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, math, pathlib, sys

op = sys.argv[1]
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)

def cosine(left, right):
    dot = sum(a * b for a, b in zip(left, right))
    left_norm = math.sqrt(sum(a * a for a in left))
    right_norm = math.sqrt(sum(a * a for a in right))
    if left_norm == 0 or right_norm == 0:
        return 0.0
    return dot / (left_norm * right_norm)

if op == "rebuild":
    (store / "records.json").write_text(json.dumps(req.get("records", [])), encoding="utf-8")
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(req.get("records", []))}))
elif op == "search":
    rows = json.loads((store / "records.json").read_text(encoding="utf-8"))
    allowed = set(req.get("allowed_ids") or [])
    if allowed:
        rows = [row for row in rows if row.get("id") in allowed]
    query = req.get("query_vector") or []
    rows.sort(key=lambda row: (-cosine(query, row.get("vector") or []), str((row.get("metadata") or {}).get("source_ref") or "")))
    limit = int(req.get("limit") or 20)
    hits = [{"id": row["id"], "score": cosine(query, row.get("vector") or []), "metadata": row.get("metadata") or {}} for row in rows[:limit]]
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","total":len(hits),"hits":hits}))
else:
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write shadow test sidecar: %v", err)
	}
	return path
}
