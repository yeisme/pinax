package continuitydogfood

import (
	"strings"
	"testing"
)

func TestParseAgentNotesSelectsUniqueSourceIDs(t *testing.T) {
	input := `spec_version=1.0
mode=agent
command=note.list
status=success
fact.returned=3
fact.total=32
note.1.note_id=note_alpha
note.1.kind=task
note.2.note_id=note_beta
note.2.kind=reference
note.3.note_id=note_alpha
note.3.kind=task
`
	notes, err := parseAgentNotes(input, 10)
	if err != nil {
		t.Fatalf("parse notes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %+v, want 2 unique notes", notes)
	}
	if notes[0].ID != "note_alpha" || notes[1].ID != "note_beta" {
		t.Fatalf("unexpected note order: %+v", notes)
	}
	if total := parseAgentNoteTotal(input, len(notes)); total != 32 {
		t.Fatalf("total = %d, want 32", total)
	}
}

func TestSummarizeComputesMeasuredCohortMetrics(t *testing.T) {
	cases := []CaseResult{
		{Completed: true, ContinuationSuccess: true, SourceTotal: 1, SourceResolved: 1, BaselineReexplanationRequired: true, ReexplanationReduced: true},
		{Completed: false, ContinuationSuccess: false, SourceTotal: 1, SourceMissing: 1, BaselineReexplanationRequired: true, FailureClass: "source_resolution"},
	}
	summary := summarize("run-1", cases)
	if summary.TaskSamples != 2 || summary.Completed != 1 || summary.ContinuationSuccess != 1 {
		t.Fatalf("summary counts = %+v", summary)
	}
	if summary.SourceResolvableRatio != 0.5 || summary.CompletionRate != 0.5 || summary.ContinuationRate != 0.5 {
		t.Fatalf("summary ratios = %+v", summary)
	}
	if summary.SilentPromotion != 0 {
		t.Fatalf("silent promotion = %d, want 0", summary.SilentPromotion)
	}
}

func TestEvidencePayloadContainsNoSourceIDsOrBodies(t *testing.T) {
	result := CaseResult{
		CaseID:              "task-01",
		SourceDigest:        digest("note_private_source"),
		Completed:           true,
		ContinuationSuccess: true,
	}
	payload, err := marshalEvidence([]CaseResult{result})
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	for _, forbidden := range []string{"note_private_source", "SECRET_BODY_SENTINEL", "raw_prompt"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("evidence contains forbidden value %q: %s", forbidden, payload)
		}
	}
}
