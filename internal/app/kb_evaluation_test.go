package app

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestKBEvaluationSuiteValidatesQuestionIDsAndCitationRefs(t *testing.T) {
	suite := validKBEvaluationSuite()
	if err := suite.Validate(); err != nil {
		t.Fatalf("valid evaluation suite rejected: %v", err)
	}
	payload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if parsed, err := ParseKBEvaluationSuite(payload); err != nil || parsed.SuiteID != suite.SuiteID {
		t.Fatalf("parse suite = %#v err=%v", parsed, err)
	}
	if got := suite.DatasetStatus(); got != KBEvaluationDatasetInsufficient {
		t.Fatalf("dataset status = %q, want %q for a small canary suite", got, KBEvaluationDatasetInsufficient)
	}

	duplicate := suite
	duplicate.Questions = append(duplicate.Questions, duplicate.Questions[0])
	var cmdErr *domain.CommandError
	if !errors.As(duplicate.Validate(), &cmdErr) || cmdErr.Code != "kb_evaluation_suite_invalid" {
		t.Fatalf("duplicate question error = %#v, want kb_evaluation_suite_invalid", duplicate.Validate())
	}

	unsafe := suite
	unsafe.Questions = append([]KBEvaluationQuestion(nil), suite.Questions...)
	unsafe.Questions[0].ExpectedCitations = []string{"/private/absolute.md"}
	if !errors.As(unsafe.Validate(), &cmdErr) || cmdErr.Code != "kb_evaluation_suite_invalid" {
		t.Fatalf("unsafe citation error = %#v, want kb_evaluation_suite_invalid", unsafe.Validate())
	}

	for _, forbidden := range []string{"raw_prompt", "secret", "permission_ids"} {
		if strings.Contains(strings.ToLower(string(payload)), forbidden) {
			t.Fatalf("suite JSON contains forbidden sentinel %q: %s", forbidden, payload)
		}
	}
}

func TestKBEvaluationMetricsComputeRecallMRRAndCitationCoverage(t *testing.T) {
	suite := validKBEvaluationSuite()
	metrics, err := ComputeKBEvaluationMetrics(suite, []KBEvaluationQuestionResult{
		{QuestionID: "q-001", Status: KBEvaluationStatusPassed, RetrievedCitations: []string{"notes/a.md#intro", "notes/a.md#details"}},
		{QuestionID: "q-002", Status: KBEvaluationStatusNoHit, RetrievedCitations: []string{"notes/other.md#x"}},
	}, 2)
	if err != nil {
		t.Fatalf("compute metrics: %v", err)
	}
	if metrics.TotalQuestions != 2 || metrics.StatusCounts[string(KBEvaluationStatusPassed)] != 1 || metrics.StatusCounts[string(KBEvaluationStatusNoHit)] != 1 {
		t.Fatalf("metric counts = %#v", metrics)
	}
	if metrics.RecallAtK != 2.0/3.0 {
		t.Fatalf("Recall@K = %v, want %v", metrics.RecallAtK, 2.0/3.0)
	}
	if metrics.MRRAtK != 0.5 {
		t.Fatalf("MRR@K = %v, want 0.5", metrics.MRRAtK)
	}
	if metrics.CitationCoverage != 0.5 {
		t.Fatalf("citation coverage = %v, want 0.5", metrics.CitationCoverage)
	}
}

func TestKBEvaluationReceiptBindsPassedRunToCandidate(t *testing.T) {
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	if err := ValidateKBEvaluationReceiptForCandidate(receipt, manifest, suite, "sha256:gate"); err != nil {
		t.Fatalf("valid candidate receipt rejected: %v", err)
	}

	wrong := receipt
	wrong.GenerationID = "gen-other"
	var cmdErr *domain.CommandError
	if !errors.As(ValidateKBEvaluationReceiptForCandidate(wrong, manifest, suite, "sha256:gate"), &cmdErr) || cmdErr.Code != "kb_evaluation_gate_failed" {
		t.Fatalf("wrong generation error = %#v, want kb_evaluation_gate_failed", ValidateKBEvaluationReceiptForCandidate(wrong, manifest, suite, "sha256:gate"))
	}

	failed := receipt
	failed.Status = "failed"
	if !errors.As(ValidateKBEvaluationReceiptForCandidate(failed, manifest, suite, "sha256:gate"), &cmdErr) || cmdErr.Code != "kb_evaluation_gate_failed" {
		t.Fatalf("failed receipt error = %#v, want kb_evaluation_gate_failed", ValidateKBEvaluationReceiptForCandidate(failed, manifest, suite, "sha256:gate"))
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	for _, forbidden := range []string{"how is Inferrum used", "permission_ids", "allowed_ids", "vector", "raw_prompt", "secret"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("receipt contains forbidden sentinel %q: %s", forbidden, payload)
		}
	}
}

func TestActivateKBCandidateRequiresMatchingReceiptAndPreservesPrevious(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	if err := ActivateKBCandidate(root, 0, manifest, suite, receipt, "sha256:gate", "2026-08-01T00:03:00Z"); err != nil {
		t.Fatalf("activate candidate: %v", err)
	}
	first, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read first activation: %v", err)
	}
	if first.Sequence != 1 || first.Active == nil || first.Active.GenerationID != manifest.GenerationID || first.Previous != nil {
		t.Fatalf("first activation = %#v", first)
	}

	wrong := receipt
	wrong.GenerationID = "gen-other"
	if err := ActivateKBCandidate(root, 1, manifest, suite, wrong, "sha256:gate", "2026-08-01T00:04:00Z"); err == nil {
		t.Fatalf("wrong receipt identity should block activation")
	}
	unchanged, err := ReadKBActivationDescriptor(root)
	if err != nil || unchanged.Sequence != 1 || unchanged.Active.GenerationID != manifest.GenerationID {
		t.Fatalf("failed activation changed descriptor: %#v err=%v", unchanged, err)
	}
}

func TestKBEvaluationReceiptStoreIsImmutable(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	path, err := WriteKBEvaluationReceipt(root, receipt)
	if err != nil {
		t.Fatalf("write evaluation receipt: %v", err)
	}
	if want := filepath.Join(root, ".pinax", "kb", "evaluations", receipt.RunID, "receipt.json"); path != want {
		t.Fatalf("receipt path = %q, want %q", path, want)
	}
	got, err := ReadKBEvaluationReceipt(root, receipt.RunID)
	if err != nil {
		t.Fatalf("read evaluation receipt: %v", err)
	}
	if got.RunID != receipt.RunID || got.GenerationManifestHash != receipt.GenerationManifestHash {
		t.Fatalf("read receipt = %#v, want %#v", got, receipt)
	}
	changed := receipt
	changed.GenerationID = "gen-other"
	if _, err := WriteKBEvaluationReceipt(root, changed); err == nil {
		t.Fatalf("rewriting an immutable receipt with changed identity should fail")
	}
}

func TestKBEvaluationGateRejectsIdentityMismatches(t *testing.T) {
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	base := validKBEvaluationReceipt(manifest, suite)
	cases := []struct {
		name   string
		mutate func(*KBEvaluationReceipt)
	}{
		{"generation", func(r *KBEvaluationReceipt) { r.GenerationID = "gen-other" }},
		{"source", func(r *KBEvaluationReceipt) { r.SourceDigest = "sha256:other" }},
		{"model", func(r *KBEvaluationReceipt) { r.Model = "other-model" }},
		{"derived_digest", func(r *KBEvaluationReceipt) { r.ModelManifestDigest = "sha256:other" }},
		{"profile", func(r *KBEvaluationReceipt) { r.ProfileHash = "sha256:other" }},
		{"dimension", func(r *KBEvaluationReceipt) { r.EmbeddingDim = 768 }},
		{"suite", func(r *KBEvaluationReceipt) { r.SuiteID = "other-suite" }},
		{"manifest", func(r *KBEvaluationReceipt) { r.GenerationManifestHash = "sha256:other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receipt := base
			tc.mutate(&receipt)
			var cmdErr *domain.CommandError
			if !errors.As(ValidateKBEvaluationReceiptForCandidate(receipt, manifest, suite, "sha256:gate"), &cmdErr) || cmdErr.Code != "kb_evaluation_gate_failed" {
				t.Fatalf("mismatch error = %#v, want kb_evaluation_gate_failed", ValidateKBEvaluationReceiptForCandidate(receipt, manifest, suite, "sha256:gate"))
			}
		})
	}
	wrongSuite := suite
	wrongSuite.SuiteID = "other-suite"
	var cmdErr *domain.CommandError
	if !errors.As(ValidateKBEvaluationReceiptForCandidate(base, manifest, wrongSuite, "sha256:gate"), &cmdErr) || cmdErr.Code != "kb_evaluation_gate_failed" {
		t.Fatalf("wrong suite error = %#v, want kb_evaluation_gate_failed", ValidateKBEvaluationReceiptForCandidate(base, manifest, wrongSuite, "sha256:gate"))
	}
	missing := KBEvaluationReceipt{}
	if !errors.As(ValidateKBEvaluationReceiptForCandidate(missing, manifest, suite, "sha256:gate"), &cmdErr) || cmdErr.Code != "kb_evaluation_receipt_invalid" {
		t.Fatalf("missing receipt error = %#v, want kb_evaluation_receipt_invalid", ValidateKBEvaluationReceiptForCandidate(missing, manifest, suite, "sha256:gate"))
	}
}

func validKBEvaluationSuite() KBEvaluationSuite {
	return KBEvaluationSuite{
		SchemaVersion: "pinax.kb.evaluation-suite.v1",
		SuiteID:       "personal-canary",
		Version:       "2026-08-01",
		Questions: []KBEvaluationQuestion{
			{QuestionID: "q-001", Query: "how is Inferrum used?", ExpectedCitations: []string{"notes/a.md#intro", "notes/a.md#details"}},
			{QuestionID: "q-002", Query: "what is the provider profile?", ExpectedCitations: []string{"notes/provider.md#profile"}},
		},
	}
}

func validKBEvaluationReceipt(manifest KBGenerationManifest, suite KBEvaluationSuite) KBEvaluationReceipt {
	return KBEvaluationReceipt{
		SchemaVersion:          KBEvaluationReceiptSchema,
		RunID:                  "run-20260801-001",
		Status:                 "passed",
		GenerationID:           manifest.GenerationID,
		SourceSnapshot:         manifest.SourceSnapshot,
		SourceDigest:           manifest.SourceDigest,
		Provider:               manifest.Provider,
		Model:                  manifest.Model,
		ModelManifestDigest:    manifest.ModelManifestDigest,
		ProfileHash:            manifest.ProfileHash,
		EmbeddingDim:           manifest.EmbeddingDim,
		SuiteID:                suite.SuiteID,
		SuiteVersion:           suite.Version,
		GateConfigHash:         "sha256:gate",
		GenerationManifestHash: HashKBGenerationManifest(manifest),
		Metrics:                KBEvaluationMetrics{DatasetStatus: KBEvaluationDatasetReady, TotalQuestions: len(suite.Questions), K: 5},
		CreatedAt:              "2026-08-01T00:02:00Z",
	}
}
