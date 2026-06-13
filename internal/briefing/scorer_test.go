package briefing

import "testing"

func TestScoreEvidenceRanksRelevantNovelTrustedCandidates(t *testing.T) {
	recipe := DefaultRecipe()
	ledger := BuildEvidenceLedger([]EvidenceItem{
		{SourceID: "hermes:hot", URL: "https://example.test/ai", Title: "AI tooling for agents", Summary: "Local agent workflow and markdown notes"},
		{SourceID: "user:curated", URL: "https://example.test/garden", Title: "Garden tools", Summary: "Soil and watering"},
		{SourceID: "fake:default", URL: "https://example.test/existing", Title: "Existing Note", Summary: "Already in vault"},
	})
	scores := ScoreEvidence(recipe, ledger, []string{"# Agent workflow\nMarkdown notes and local tooling", "# Existing Note\nAlready in vault"})
	if len(scores) != 3 {
		t.Fatalf("scores = %#v", scores)
	}
	if scores[0].Evidence.Title != "AI tooling for agents" {
		t.Fatalf("top score = %#v", scores)
	}
	if scores[0].Relevance <= scores[1].Relevance || scores[0].Total <= scores[1].Total {
		t.Fatalf("relevant candidate not ranked above unrelated: %#v", scores)
	}
	var existing CandidateScore
	for _, score := range scores {
		if score.Evidence.Title == "Existing Note" {
			existing = score
		}
	}
	if existing.Novelty >= 0.5 {
		t.Fatalf("existing vault title should have low novelty: %#v", existing)
	}
}

func TestScoreEvidenceUsesRecipeLimit(t *testing.T) {
	recipe := DefaultRecipe()
	recipe.Limit = 1
	ledger := BuildEvidenceLedger([]EvidenceItem{{SourceID: "fake:default", URL: "https://example.test/a", Title: "A"}, {SourceID: "fake:default", URL: "https://example.test/b", Title: "B"}})
	if got := ScoreEvidence(recipe, ledger, nil); len(got) != 1 {
		t.Fatalf("limited scores len = %d", len(got))
	}
}
