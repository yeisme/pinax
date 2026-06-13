package briefing

import "testing"

func TestCandidateNotesReviewQueue(t *testing.T) {
	recipe := DefaultRecipe()
	scores := []CandidateScore{{Evidence: EvidenceItem{EvidenceID: "ev_ai", URL: "https://example.test/ai", Title: "AI tooling for agents", Summary: "Local agent workflow"}, Total: 0.9}}
	queue, candidates := BuildCandidateNotes(recipe, scores, []string{"Agent workflow"})
	if queue.SchemaVersion != ReviewQueueSchemaVersion || len(queue.Items) != 1 || len(candidates) != 1 {
		t.Fatalf("queue=%#v candidates=%#v", queue, candidates)
	}
	if queue.Items[0].Status != "pending_review" || queue.Items[0].Path == "" || candidates[0].Body == "" {
		t.Fatalf("candidate output invalid: queue=%#v candidate=%#v", queue, candidates[0])
	}
}
