package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKBReviewOverviewTruthStateFixturesKeepLayersIndependent(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, root string)
		sourceStatus  string
		indexStatus   string
		sidecarStatus string
	}{
		{name: "empty_sources", setup: func(*testing.T, string) {}, sourceStatus: "empty_sources", indexStatus: "missing", sidecarStatus: "not_checked"},
		{name: "candidate_only", setup: func(t *testing.T, root string) { mustMkdir(t, filepath.Join(root, ".pinax", "kb", "generations")) }, sourceStatus: "empty_sources", indexStatus: "candidate_only", sidecarStatus: "not_checked"},
		{name: "present_unversioned", setup: func(t *testing.T, root string) { mustMkdir(t, filepath.Join(root, ".pinax", "kb", "lancedb")) }, sourceStatus: "empty_sources", indexStatus: "present_unversioned", sidecarStatus: "not_checked"},
		{name: "active_sidecar_missing", setup: func(t *testing.T, root string) {
			writeReviewTruthActive(t, root, KBGenerationStatusReady, false, "sha256:missing")
		}, sourceStatus: "ready", indexStatus: "missing", sidecarStatus: "unavailable"},
		{name: "active_failed", setup: func(t *testing.T, root string) {
			writeReviewTruthActive(t, root, KBGenerationStatusFailed, true, "sha256:failed")
		}, sourceStatus: "ready", indexStatus: "failed", sidecarStatus: "projection_present"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.sourceStatus == "ready" {
				mustMkdir(t, filepath.Join(root, "notes"))
				writeReviewTruthNote(t, root)
			}
			tt.setup(t, root)
			projection, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
			if err != nil {
				t.Fatalf("overview: %v", err)
			}
			if projection.Facts["source_status"] != tt.sourceStatus || projection.Facts["index_status"] != tt.indexStatus || projection.Facts["sidecar_status"] != tt.sidecarStatus {
				t.Fatalf("facts = %#v, want source=%s index=%s sidecar=%s", projection.Facts, tt.sourceStatus, tt.indexStatus, tt.sidecarStatus)
			}
			if projection.Facts["answer_mode"] != "not_generated" {
				t.Fatalf("answer mode = %#v", projection.Facts["answer_mode"])
			}
		})
	}
}

func TestKBReviewOverviewUsesWorkbenchTruthVocabularyWithoutInferringReadiness(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "notes"))
	writeReviewTruthNote(t, root)
	writeReviewTruthActive(t, root, KBGenerationStatusReady, false, "sha256:missing")

	projection, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if projection.Facts["provider_status"] != "not_checked" {
		t.Fatalf("provider status = %q, want not_checked until a provider doctor runs", projection.Facts["provider_status"])
	}
	if projection.Facts["sidecar_status"] != "unavailable" {
		t.Fatalf("sidecar status = %q, want unavailable", projection.Facts["sidecar_status"])
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("overview data = %#v, want object", projection.Data)
	}
	truth, ok := data["truth_states"].(map[string]string)
	if !ok {
		t.Fatalf("truth_states = %#v, want string map", data["truth_states"])
	}
	for key, want := range map[string]string{
		"source_status":     "ready",
		"provider_status":   "not_checked",
		"sidecar_status":    "unavailable",
		"generation_status": "missing",
		"evaluation_status": "not_generated",
		"answer_mode":       "not_generated",
		"rerank":            "passthrough",
	} {
		if truth[key] != want {
			t.Errorf("truth_states[%q] = %q, want %q", key, truth[key], want)
		}
	}
	supported, ok := data["supported_sources"].([]map[string]string)
	if !ok {
		t.Fatalf("supported_sources = %#v, want rows", data["supported_sources"])
	}
	seen := map[string]string{}
	for _, row := range supported {
		seen[row["type"]] = row["status"]
	}
	for key, want := range map[string]string{"pdf": "unsupported", "web": "unsupported", "code_repository": "unsupported", "feishu": "not_configured", "image_ocr": "not_configured"} {
		if seen[key] != want {
			t.Errorf("supported_sources[%q] = %q, want %q", key, seen[key], want)
		}
	}
}

func TestKBReviewSourceCapabilitiesHaveRealNextActionsWithoutFalseReadiness(t *testing.T) {
	root := t.TempDir()
	projection, err := NewService().KBReviewSources(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("sources projection: %v", err)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("sources data = %#v, want object", projection.Data)
	}
	rows, ok := data["unsupported"].([]map[string]string)
	if !ok {
		t.Fatalf("unsupported rows = %#v, want rows", data["unsupported"])
	}
	want := map[string]string{
		"pdf":             "unsupported",
		"web":             "unsupported",
		"code_repository": "unsupported",
		"feishu":          "not_configured",
		"image_ocr":       "not_configured",
	}
	if len(rows) != len(want) {
		t.Fatalf("unsupported row count = %d, want %d", len(rows), len(want))
	}
	for _, row := range rows {
		typeName := row["type"]
		if row["status"] != want[typeName] {
			t.Fatalf("%s status = %q, want %q", typeName, row["status"], want[typeName])
		}
		if row["next_action"] == "" || !strings.Contains(row["next_action"], "pinax kb import <source> --vault <vault> --dry-run") {
			t.Fatalf("%s next_action = %q, want a real import fallback", typeName, row["next_action"])
		}
		if row["status"] == "ready" || row["status"] == "indexed" || row["status"] == "uploaded" || row["status"] == "synchronized" {
			t.Fatalf("%s reported false readiness: %#v", typeName, row)
		}
	}

	overview, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("overview projection: %v", err)
	}
	overviewData, ok := overview.Data.(map[string]any)
	if !ok {
		t.Fatalf("overview data = %#v, want object", overview.Data)
	}
	capabilities, ok := overviewData["supported_sources"].([]map[string]string)
	if !ok {
		t.Fatalf("supported source capabilities = %#v, want rows", overviewData["supported_sources"])
	}
	for _, row := range capabilities {
		if row["status"] != "ready" && row["next_action"] == "" {
			t.Fatalf("unsupported overview capability lacks next_action: %#v", row)
		}
	}
}

func TestKBReviewRunProjectsPartialAndPassthroughWithoutAnswerGeneration(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	receipt.Status = "failed"
	receipt.Metrics.StatusCounts = map[string]int{"partial": 1}
	if _, err := WriteKBEvaluationReceipt(root, receipt); err != nil {
		t.Fatalf("write receipt: %v", err)
	}

	projection, err := NewService().KBReviewRun(context.Background(), KBReviewRequest{VaultPath: root, RunID: receipt.RunID})
	if err != nil {
		t.Fatalf("run projection: %v", err)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("run data = %#v, want object", projection.Data)
	}
	if data["evaluation_status"] != "partial" || data["rerank"] != "passthrough" || data["answer_mode"] != "not_generated" {
		t.Fatalf("run truth state = %#v, want partial/passthrough/not_generated", data)
	}
}

func TestKBReviewOverviewProjectsLatestEvaluationReceiptSummary(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	receipt.RunID = "run-latest-review"
	receipt.Status = "failed"
	receipt.CreatedAt = "2026-08-02T12:34:56Z"
	receipt.Metrics.StatusCounts = map[string]int{"partial": 1}
	if _, err := WriteKBEvaluationReceipt(root, receipt); err != nil {
		t.Fatalf("write receipt: %v", err)
	}

	projection, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	for key, want := range map[string]string{
		"evaluation_count":  "1",
		"last_run_id":       receipt.RunID,
		"last_run_at":       receipt.CreatedAt,
		"evaluation_status": "partial",
	} {
		if projection.Facts[key] != want {
			t.Errorf("overview facts[%q] = %q, want %q; facts=%#v", key, projection.Facts[key], want, projection.Facts)
		}
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("overview data = %#v, want object", projection.Data)
	}
	truth, ok := data["truth_states"].(map[string]string)
	if !ok || truth["evaluation_status"] != "partial" {
		t.Fatalf("overview truth states = %#v, want partial evaluation", data["truth_states"])
	}
}

func TestKBReviewEvaluationStatusKeepsNoHitsAndDatasetStatesExplicit(t *testing.T) {
	tests := []struct {
		name    string
		receipt KBEvaluationReceipt
		want    string
	}{
		{name: "no_hits", receipt: KBEvaluationReceipt{Status: "failed", Metrics: KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetReady, TotalQuestions: 2, StatusCounts: map[string]int{"no_hit": 2}}}, want: "no_hits"},
		{name: "partial", receipt: KBEvaluationReceipt{Status: "failed", Metrics: KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetReady, TotalQuestions: 2, StatusCounts: map[string]int{"no_hit": 1}}}, want: "partial"},
		{name: "insufficient", receipt: KBEvaluationReceipt{Status: "failed", Metrics: KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetInsufficient, TotalQuestions: 1}}, want: KBEvaluationDatasetInsufficient},
		{name: "passed", receipt: KBEvaluationReceipt{Status: "passed", Metrics: KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetReady, TotalQuestions: 2}}, want: "passed"},
		{name: "failed", receipt: KBEvaluationReceipt{Status: "failed", Metrics: KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetReady, TotalQuestions: 2}}, want: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kbReviewEvaluationStatus(tt.receipt); got != tt.want {
				t.Fatalf("evaluation status = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeReviewTruthActive(t *testing.T, root string, status KBGenerationStatus, storePresent bool, sourceDigest string) {
	t.Helper()
	if sourceDigest == "sha256:missing" {
		notes, err := scanNotes(root)
		if err != nil {
			t.Fatalf("scan truth notes: %v", err)
		}
		sourceDigest = kbSourceDigest(notes)
	}
	manifest := validKBGenerationManifest()
	manifest.GenerationID = "gen-truth-" + string(status)
	manifest.Status = status
	manifest.SourceDigest = sourceDigest
	manifest.SourceSnapshot = kbSourceSnapshot(sourceDigest)
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if storePresent {
		mustMkdir(t, filepath.Join(root, ".pinax", "kb", "generations", manifest.GenerationID, "lancedb"))
	}
	ref := validKBActivationRef(manifest.GenerationID)
	ref.SourceDigest = manifest.SourceDigest
	ref.SourceSnapshot = manifest.SourceSnapshot
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit descriptor: %v", err)
	}
}

func writeReviewTruthNote(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "notes", "truth.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: truth-note\ntitle: Truth\nkind: reference\nstatus: active\n---\n\n# Truth\n\nState fixture.\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
