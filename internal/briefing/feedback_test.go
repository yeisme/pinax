package briefing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeedbackWritesEventAndWeight(t *testing.T) {
	root := t.TempDir()
	state, err := ApplyFeedback(root, FeedbackRequest{CandidateID: "brief_ai", EvidenceID: "ev_ai", Action: FeedbackAccept, Reason: "useful"})
	if err != nil {
		t.Fatalf("apply feedback: %v", err)
	}
	if state.SchemaVersion != FeedbackStateSchemaVersion || len(state.Events) != 1 || state.Events[0].WeightDelta <= 0 {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "briefing", "feedback.jsonl")); err != nil {
		t.Fatalf("feedback asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "events.jsonl")); err != nil {
		t.Fatalf("event asset missing: %v", err)
	}
}

func TestFeedbackPreferenceWeights(t *testing.T) {
	positive := PreferenceWeight(FeedbackMoreLikeThis)
	negative := PreferenceWeight(FeedbackLessLikeThis)
	if positive <= 0 || negative >= 0 || PreferenceWeight("unknown") != 0 {
		t.Fatalf("weights positive=%v negative=%v unknown=%v", positive, negative, PreferenceWeight("unknown"))
	}
}
