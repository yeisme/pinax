package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDailyInteractionProjectionSharesJSONAndAgentFacts(t *testing.T) {
	p := domain.NewProjection("inbox.review", "Pending notes ready for review.")
	p.Facts["interaction_schema_version"] = domain.DailyInteractionSchemaVersion
	p.Facts["interaction_phase"] = "review"
	p.Facts["interaction_state"] = "ready"
	p.Facts["interaction_read_only"] = "true"
	p.Interaction = &domain.DailyInteraction{
		SchemaVersion:   domain.DailyInteractionSchemaVersion,
		Phase:           "review",
		State:           "ready",
		ReadOnly:        true,
		SourceStatus:    "fresh",
		FreshnessStatus: "fresh",
		RecommendedNextAction: &domain.Action{
			Name:    "organize_preview",
			Command: "pinax organize plan --note note_1 --json",
		},
	}

	var jsonOut bytes.Buffer
	if err := RenderWithOptions(&jsonOut, ModeJSON, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatal(err)
	}
	jsonText := jsonOut.String()
	for _, want := range []string{"pinax.daily_interaction.v1", `"phase":"review"`, `"state":"ready"`, "pinax organize plan --note note_1 --json"} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("JSON interaction missing %q: %s", want, jsonText)
		}
	}

	var agentOut bytes.Buffer
	if err := RenderWithOptions(&agentOut, ModeAgent, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"interaction_schema_version=pinax.daily_interaction.v1", "interaction_phase=review", "interaction_state=ready"} {
		if !strings.Contains(agentOut.String(), want) {
			t.Fatalf("agent interaction missing %q: %s", want, agentOut.String())
		}
	}
}

func TestDailyInteractionProjectionAppearsInEventsAndExplain(t *testing.T) {
	p := domain.NewProjection("inbox.review", "Pending notes ready for review.")
	p.Facts["interaction_phase"] = "review"
	p.Interaction = &domain.DailyInteraction{
		SchemaVersion:   domain.DailyInteractionSchemaVersion,
		Phase:           "review",
		State:           "partial",
		ReadOnly:        true,
		SourceStatus:    "stale",
		FreshnessStatus: "stale",
		WarningCodes:    []string{"source_stale"},
		RecommendedNextAction: &domain.Action{
			Name:    "refresh",
			Command: "pinax index refresh --vault <vault> --json",
		},
	}

	var events bytes.Buffer
	if err := RenderWithOptions(&events, ModeEvents, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(events.String(), `"interaction"`) || !strings.Contains(events.String(), `"state":"partial"`) {
		t.Fatalf("events interaction missing: %s", events.String())
	}

	var explain bytes.Buffer
	if err := RenderWithOptions(&explain, ModeExplain, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Interaction: phase=review state=partial", "Recommended next step: pinax index refresh --vault <vault> --json"} {
		if !strings.Contains(explain.String(), want) {
			t.Fatalf("explain interaction missing %q: %s", want, explain.String())
		}
	}
}
