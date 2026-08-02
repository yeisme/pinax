package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKBEvaluateCandidateWritesReceiptWithoutMutatingActivation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_eval\ntitle: Evaluation Note\nkind: reference\nstatus: active\n---\n\n# Retrieval\n\nThe local knowledge base uses a staged LanceDB generation for retrieval evaluation.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "evaluation.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	sidecar := writeEvaluationFakeSidecar(t)
	service := NewService()
	staged, err := service.KBRebuild(context.Background(), KBIndexRequest{
		VaultPath:         root,
		Backend:           "lancedb",
		Provider:          "fake",
		Model:             "fake-hash-v1",
		SidecarExecutable: sidecar,
	})
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	generationID := staged.Facts["generation_id"]
	if generationID == "" {
		t.Fatalf("staging facts missing generation id: %#v", staged.Facts)
	}

	suite := KBEvaluationSuite{SchemaVersion: KBEvaluationSuiteSchema, SuiteID: "local-canary", Version: "2026-08-02", Questions: make([]KBEvaluationQuestion, 20)}
	for i := range suite.Questions {
		suite.Questions[i] = KBEvaluationQuestion{
			QuestionID:        fmt.Sprintf("q-%03d", i+1),
			Query:             "staged LanceDB retrieval",
			ExpectedCitations: []string{"notes/evaluation.md#Retrieval"},
		}
	}
	suitePath := filepath.Join(root, ".pinax", "kb", "evaluation-suites", "local-canary.json")
	payload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatalf("mkdir suite: %v", err)
	}
	if err := os.WriteFile(suitePath, payload, 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	projection, err := service.KBEvaluate(context.Background(), KBEvaluateRequest{
		VaultPath:         root,
		Suite:             ".pinax/kb/evaluation-suites/local-canary.json",
		GenerationID:      generationID,
		K:                 5,
		SidecarExecutable: sidecar,
	})
	if err != nil {
		t.Fatalf("evaluate candidate: %v", err)
	}
	if projection.Command != "kb.evaluate" || projection.Facts["status"] != "passed" || projection.Facts["generation_id"] != generationID {
		t.Fatalf("evaluation projection = %#v", projection)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation descriptor: %v", err)
	}
	if descriptor.Sequence != 0 || descriptor.Active != nil || descriptor.Previous != nil {
		t.Fatalf("evaluation mutated activation descriptor: %#v", descriptor)
	}
	runID := projection.Facts["run_id"]
	if runID == "" {
		t.Fatalf("evaluation facts missing run id: %#v", projection.Facts)
	}
	receipt, err := ReadKBEvaluationReceipt(root, runID)
	if err != nil {
		t.Fatalf("read evaluation receipt: %v", err)
	}
	if receipt.Status != "passed" || receipt.Metrics.DatasetStatus != KBEvaluationDatasetReady || receipt.Metrics.RecallAtK != 1 || receipt.Metrics.MRRAtK != 1 {
		t.Fatalf("evaluation receipt = %#v", receipt)
	}
	receiptPayload, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "evaluations", runID, "receipt.json"))
	if err != nil {
		t.Fatalf("read receipt payload: %v", err)
	}
	if strings.Contains(string(receiptPayload), "staged LanceDB retrieval") || strings.Contains(string(receiptPayload), "permission_ids") || strings.Contains(string(receiptPayload), "vector") {
		t.Fatalf("receipt leaked evaluation/query data: %s", receiptPayload)
	}
}

func TestKBEvaluatedCandidateCanActivateAndBecomeDefaultSearchTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_activate\ntitle: Activation Note\nkind: reference\nstatus: active\n---\n\n# Activation\n\nA passed candidate becomes the default local retrieval generation.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "activation.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	sidecar := writeEvaluationFakeSidecar(t)
	service := NewService()
	staged, err := service.KBRebuild(context.Background(), KBIndexRequest{VaultPath: root, Backend: "lancedb", Provider: "fake", Model: "fake-hash-v1", SidecarExecutable: sidecar})
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	generationID := staged.Facts["generation_id"]
	suite := KBEvaluationSuite{SchemaVersion: KBEvaluationSuiteSchema, SuiteID: "activation-canary", Version: "2026-08-02", Questions: make([]KBEvaluationQuestion, 20)}
	for i := range suite.Questions {
		suite.Questions[i] = KBEvaluationQuestion{QuestionID: fmt.Sprintf("q-%03d", i+1), Query: "passed candidate retrieval", ExpectedCitations: []string{"notes/activation.md#Activation"}}
	}
	suitePath := filepath.Join(root, ".pinax", "kb", "evaluation-suites", "activation-canary.json")
	payload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatalf("mkdir suite: %v", err)
	}
	if err := os.WriteFile(suitePath, payload, 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	evaluation, err := service.KBEvaluate(context.Background(), KBEvaluateRequest{VaultPath: root, Suite: ".pinax/kb/evaluation-suites/activation-canary.json", GenerationID: generationID, SidecarExecutable: sidecar})
	if err != nil {
		t.Fatalf("evaluate candidate: %v", err)
	}
	receipt, err := ReadKBEvaluationReceipt(root, evaluation.Facts["run_id"])
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	manifest, err := ReadKBGenerationManifest(root, generationID)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := ActivateKBCandidate(root, 0, manifest, suite, receipt, kbEvaluationGateConfigHash(), "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("activate evaluated candidate: %v", err)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation: %v", err)
	}
	if descriptor.Sequence != 1 || descriptor.Active == nil || descriptor.Active.GenerationID != generationID {
		t.Fatalf("activation descriptor = %#v", descriptor)
	}
	search, err := service.KBSearch(context.Background(), KBIndexRequest{VaultPath: root, Query: "passed candidate retrieval", SidecarExecutable: sidecar})
	if err != nil {
		t.Fatalf("default active search: %v", err)
	}
	if search.Facts["generation_id"] != generationID || search.Facts["matches"] == "0" {
		t.Fatalf("active search facts = %#v", search.Facts)
	}
	activeEvaluation, err := service.KBEvaluate(context.Background(), KBEvaluateRequest{
		VaultPath:         root,
		Suite:             ".pinax/kb/evaluation-suites/activation-canary.json",
		SidecarExecutable: sidecar,
	})
	if err != nil {
		t.Fatalf("default active evaluation: %v", err)
	}
	if activeEvaluation.Facts["generation_id"] != generationID || activeEvaluation.Facts["status"] != "passed" || activeEvaluation.Facts["activation_unchanged"] != "true" {
		t.Fatalf("default active evaluation facts = %#v", activeEvaluation.Facts)
	}
}

func TestKBEvaluateTimeoutPersistsReceiptWithoutRetryingQuestions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_timeout\ntitle: Timeout Note\nkind: reference\nstatus: active\n---\n\n# Timeout\n\nTimeouts remain explicit and are not silently retried.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "timeout.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	service := NewService()
	staged, err := service.KBRebuild(context.Background(), KBIndexRequest{VaultPath: root, Backend: "lancedb", Provider: "fake", Model: "fake-hash-v1", SidecarExecutable: writeEvaluationFakeSidecar(t)})
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	suite := KBEvaluationSuite{SchemaVersion: KBEvaluationSuiteSchema, SuiteID: "timeout-suite", Version: "2026-08-02", Questions: make([]KBEvaluationQuestion, 20)}
	for i := range suite.Questions {
		suite.Questions[i] = KBEvaluationQuestion{QuestionID: fmt.Sprintf("q-%03d", i+1), Query: "timeout query", ExpectedCitations: []string{"notes/timeout.md#Timeout"}}
	}
	suitePath := filepath.Join(root, ".pinax", "kb", "evaluation-suites", "timeout-suite.json")
	payload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatalf("mkdir suite: %v", err)
	}
	if err := os.WriteFile(suitePath, payload, 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	projection, err := service.KBEvaluate(context.Background(), KBEvaluateRequest{
		VaultPath:         root,
		Suite:             ".pinax/kb/evaluation-suites/timeout-suite.json",
		GenerationID:      staged.Facts["generation_id"],
		SidecarExecutable: writeTimeoutEvaluationSidecar(t),
		SidecarTimeout:    100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("timeout evaluation should persist a terminal receipt: %v", err)
	}
	if projection.Facts["status"] != "failed" || projection.Facts["activation_unchanged"] != "true" {
		t.Fatalf("timeout projection = %#v", projection)
	}
	receipt, err := ReadKBEvaluationReceipt(root, projection.Facts["run_id"])
	if err != nil {
		t.Fatalf("read timeout receipt: %v", err)
	}
	if receipt.Metrics.StatusCounts["timeout"] != len(suite.Questions) || receipt.Metrics.FailureCounts["timeout"] != len(suite.Questions) {
		t.Fatalf("timeout receipt metrics = %#v", receipt.Metrics)
	}
	countPayload, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "generations", staged.Facts["generation_id"], "timeout-count"))
	if err != nil {
		t.Fatalf("read timeout sidecar count: %v", err)
	}
	if strings.TrimSpace(string(countPayload)) != fmt.Sprint(len(suite.Questions)) {
		t.Fatalf("timeout sidecar calls = %q, want one per question", countPayload)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation: %v", err)
	}
	if descriptor.Sequence != 0 || descriptor.Active != nil {
		t.Fatalf("timeout evaluation mutated activation: %#v", descriptor)
	}
}

func writeEvaluationFakeSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, pathlib, sys

op = sys.argv[1] if len(sys.argv) > 1 else ""
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)
sidecar_path = store / "sidecar.jsonl"

if op == "rebuild":
    records = req.get("records", [])
    sidecar_path.write_text("".join(json.dumps(row) + "\n" for row in records), encoding="utf-8")
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(records)}))
elif op == "search":
    rows = [json.loads(line) for line in sidecar_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    allowed = set(req.get("allowed_ids") or [])
    rows = [row for row in rows if not allowed or row.get("id") in allowed]
    limit = req.get("limit") or 8
    hits = [{"id": row["id"], "score": 1.0 - i * 0.01, "metadata": row.get("metadata", {})} for i, row in enumerate(rows[:limit])]
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","total":len(rows),"hits":hits}))
else:
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write evaluation fake sidecar: %v", err)
	}
	return path
}

func writeTimeoutEvaluationSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "timeout-sidecar")
	body := `#!/usr/bin/env python3
import json, pathlib, sys, time

req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
count_path = store.parent / "timeout-count"
try:
    count = int(count_path.read_text(encoding="utf-8"))
except Exception:
    count = 0
count_path.write_text(str(count + 1), encoding="utf-8")
time.sleep(0.5)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write timeout evaluation sidecar: %v", err)
	}
	return path
}
