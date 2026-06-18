package notes

import (
	"strings"
	"testing"
)

func TestRenderBriefingCandidateMarkdown(t *testing.T) {
	body := RenderBriefingCandidateMarkdown(BriefingCandidate{Title: "AI tooling for agents", URL: "https://example.test/ai", Summary: "Local agent workflow", Topic: "AI tooling", Tags: []string{"briefing", "candidate"}, Backlinks: []string{"Agent workflow"}})
	for _, want := range []string{"schema_version: pinax.note.v1", "kind: briefing_candidate", "tags: [briefing, candidate]", "source_url: https://example.test/ai", "[[Agent workflow]]", "Local agent workflow"} {
		if !strings.Contains(body, want) {
			t.Fatalf("candidate markdown missing %q:\n%s", want, body)
		}
	}
}
