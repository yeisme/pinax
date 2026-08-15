package continuitydogfood

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyzeDecisionProducesEvidenceBoundedIterateReceipt(t *testing.T) {
	cohortRun, followUpRun := writeDecisionFixtures(t, 10)
	result, err := AnalyzeDecision(DecisionConfig{
		CohortRun:   cohortRun,
		FollowUpRun: followUpRun,
		ParentDir:   t.TempDir(),
		RunID:       "decision-test",
		Now:         func() time.Time { return time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("analyze decision: %v", err)
	}
	if result.Receipt.Decision != "iterate" || result.Receipt.Maturity != "experimental" {
		t.Fatalf("receipt = %#v", result.Receipt)
	}
	if len(result.Receipt.FailureClasses) != 2 || result.Receipt.FailureClasses[0] != "external_validity" || result.Receipt.FailureClasses[1] != "commercial_signal" {
		t.Fatalf("failure classes = %#v", result.Receipt.FailureClasses)
	}
	if result.Analysis.TaskLevel.Completion.Numerator != 10 || result.Analysis.TaskLevel.Reuse.Numerator != 10 {
		t.Fatalf("task metrics = %#v", result.Analysis.TaskLevel)
	}
	if result.Analysis.TaskLevel.SourceResolvability.Value != 1 || result.Analysis.TaskLevel.SilentPromotion.Numerator != 0 {
		t.Fatalf("trust metrics = %#v", result.Analysis.TaskLevel)
	}
	if result.Analysis.Commercial.WillingnessToPay.Status != "not_measured" {
		t.Fatalf("commercial metric = %#v", result.Analysis.Commercial)
	}
	for _, rel := range []string{"summary.json", "command.txt", "stdout.log", "stderr.log", "env.json", "artifacts/analysis.json", "artifacts/decision.json"} {
		if _, statErr := os.Stat(filepath.Join(result.RunDir, rel)); statErr != nil {
			t.Fatalf("missing evidence %s: %v", rel, statErr)
		}
	}
	payload, err := os.ReadFile(filepath.Join(result.RunDir, "artifacts", "decision.json"))
	if err != nil {
		t.Fatalf("read decision evidence: %v", err)
	}
	if containsForbiddenDecisionEvidence(string(payload)) {
		t.Fatalf("decision evidence contains forbidden content: %s", payload)
	}
}

func TestAnalyzeDecisionRejectsMismatchedFollowUpSample(t *testing.T) {
	cohortRun, followUpRun := writeDecisionFixtures(t, 2)
	followUpPath := filepath.Join(followUpRun, "artifacts", "followup.json")
	payload, err := os.ReadFile(followUpPath)
	if err != nil {
		t.Fatalf("read follow-up fixture: %v", err)
	}
	var followUp struct {
		SchemaVersion string         `json:"schema_version"`
		Cases         []FollowUpCase `json:"cases"`
	}
	if err := json.Unmarshal(payload, &followUp); err != nil {
		t.Fatalf("decode follow-up fixture: %v", err)
	}
	followUp.Cases[0].SourceDigest = "mismatched-digest"
	writeDecisionJSON(t, followUpPath, followUp)

	_, err = AnalyzeDecision(DecisionConfig{CohortRun: cohortRun, FollowUpRun: followUpRun, ParentDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected mismatched sample evidence to fail")
	}
}

func writeDecisionFixtures(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	cohortRun := filepath.Join(root, "cohort")
	followUpRun := filepath.Join(root, "followup")
	if err := os.MkdirAll(filepath.Join(cohortRun, "artifacts"), 0o755); err != nil {
		t.Fatalf("create cohort fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(followUpRun, "artifacts"), 0o755); err != nil {
		t.Fatalf("create follow-up fixture: %v", err)
	}

	cases := make([]CaseResult, 0, count)
	followUpCases := make([]FollowUpCase, 0, count)
	for index := 0; index < count; index++ {
		caseID := "task-" + twoDigit(index+1)
		taskID := "sample-" + twoDigit(index+1)
		digest := "digest-" + twoDigit(index+1)
		cases = append(cases, CaseResult{
			CaseID: caseID, TaskSampleID: taskID, SourceDigest: digest,
			Completed: true, ContinuationSuccess: true, ReexplanationReduced: true,
			ProposalApprovalRequired: true, ReviewItemObserved: true, OwnerApproved: true,
			SourceTotal: 1, SourceResolved: 1,
		})
		followUpCases = append(followUpCases, FollowUpCase{
			CaseID: caseID, TaskSampleID: taskID, SourceDigest: digest,
			ReuseVerified: true, SourceTotal: 1, SourceResolved: 1,
		})
	}
	cohortSummary := Summary{
		SchemaVersion: SchemaVersion, RunID: "cohort", Status: "success", TaskSamples: count,
		Completed: count, CompletionRate: 1, ContinuationSuccess: count, ContinuationRate: 1,
		SourceTotal: count, SourceResolved: count, SourceResolvableRatio: 1,
		BaselineReexplanation: count, ReexplanationReduced: count, SilentPromotion: 0,
		UnderFiveMinutes:    count,
		FailureDistribution: map[string]int{}, SevenDayReuseMeasured: false, WillingnessToPayMeasured: false,
		KnownBias: []string{"single local operator"},
	}
	followUpSummary := FollowUpSummary{
		SchemaVersion: FollowUpSchemaVersion, RunID: "followup", PriorCohortRunID: "cohort", Status: "success",
		TaskSamples: count, ReuseVerified: count, ReuseRate: 1,
		SourceTotal: count, SourceResolved: count, SourceResolvableRatio: 1,
		FailureDistribution: map[string]int{}, ObservedAt: "2026-08-10T01:00:00Z",
		KnownBias: []string{"single local operator", "willingness-to-pay not measured"},
	}
	writeDecisionJSON(t, filepath.Join(cohortRun, "summary.json"), cohortSummary)
	writeDecisionJSON(t, filepath.Join(cohortRun, "artifacts", "cohort.json"), struct {
		SchemaVersion string       `json:"schema_version"`
		Cases         []CaseResult `json:"cases"`
	}{SchemaVersion: CohortSchemaVersion, Cases: cases})
	writeDecisionJSON(t, filepath.Join(followUpRun, "summary.json"), followUpSummary)
	writeDecisionJSON(t, filepath.Join(followUpRun, "artifacts", "followup.json"), struct {
		SchemaVersion string         `json:"schema_version"`
		Cases         []FollowUpCase `json:"cases"`
	}{SchemaVersion: FollowUpSchemaVersion, Cases: followUpCases})
	return cohortRun, followUpRun
}

func writeDecisionJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func twoDigit(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return "10"
}
