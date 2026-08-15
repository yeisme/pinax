package continuitydogfood

import (
	"testing"
	"time"
)

func TestValidateFollowUpWindowRejectsEarlyAndLateRuns(t *testing.T) {
	started := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	if err := validateFollowUpWindow(started, started.Add(6*24*time.Hour)); err == nil {
		t.Fatal("expected early follow-up to be rejected")
	}
	if err := validateFollowUpWindow(started, started.Add(10*24*time.Hour)); err != nil {
		t.Fatalf("expected day-10 follow-up to pass: %v", err)
	}
	if err := validateFollowUpWindow(started, started.Add(15*24*time.Hour)); err == nil {
		t.Fatal("expected late follow-up to be rejected")
	}
}

func TestSummarizeFollowUpReportsParticipantDenominator(t *testing.T) {
	cases := []FollowUpCase{
		{CaseID: "task-01", ReuseVerified: true, SourceTotal: 1, SourceResolved: 1, ReexplanationStillRequired: false},
		{CaseID: "task-02", ReuseVerified: false, SourceTotal: 1, SourceMissing: 1, ReexplanationStillRequired: true, FailureClass: "source_resolution"},
	}
	summary := summarizeFollowUp("followup-1", "cohort-1", cases)
	if summary.TaskSamples != 2 || summary.ReuseVerified != 1 || summary.ReuseRate != 0.5 {
		t.Fatalf("follow-up summary = %+v", summary)
	}
	if summary.SourceResolvableRatio != 0.5 || summary.ReexplanationStillRequired != 1 {
		t.Fatalf("follow-up source/reexplanation metrics = %+v", summary)
	}
}
