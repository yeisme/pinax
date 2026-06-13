package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestSummaryColorModes(t *testing.T) {
	projection := domain.NewProjection("test.summary", "Color output test.")
	projection.Facts["notes"] = "2"

	t.Setenv("NO_COLOR", "")
	t.Setenv("PINAX_COLOR", "always")
	var colored bytes.Buffer
	if err := Render(&colored, ModeSummary, projection); err != nil {
		t.Fatalf("render colored summary: %v", err)
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("summary with PINAX_COLOR=always missing ANSI:\n%s", colored.String())
	}
	for _, old := range []string{"\x1b[38;5;63m", "\x1b[38;5;141m", "\x1b[1;38;5;42m"} {
		if strings.Contains(colored.String(), old) {
			t.Fatalf("summary still uses old high-saturation palette %q:\n%s", old, colored.String())
		}
	}
	for _, want := range []string{"\x1b[38;5;240m", "\x1b[1;38;5;250m", "Highlights"} {
		if !strings.Contains(colored.String(), want) {
			t.Fatalf("summary missing refined palette token %q:\n%s", want, colored.String())
		}
	}

	t.Setenv("PINAX_COLOR", "never")
	var plain bytes.Buffer
	if err := Render(&plain, ModeSummary, projection); err != nil {
		t.Fatalf("render plain summary: %v", err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("summary with PINAX_COLOR=never contains ANSI:\n%s", plain.String())
	}
}

func TestMachineOutputsNeverUseANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("PINAX_COLOR", "always")
	projection := domain.NewProjection("test.machine", "Color output test.")
	projection.Facts["notes"] = "2"

	for _, mode := range []Mode{ModeJSON, ModeAgent, ModeEvents} {
		var out bytes.Buffer
		if err := Render(&out, mode, projection); err != nil {
			t.Fatalf("render %s: %v", mode, err)
		}
		if strings.Contains(out.String(), "\x1b[") {
			t.Fatalf("%s output contains ANSI:\n%s", mode, out.String())
		}
	}
}

func TestSummaryRendersEnglishFactKeysAndCommonValues(t *testing.T) {
	projection := domain.NewProjection("note.links", "Link check completed.")
	projection.Status = "partial"
	projection.Facts["notes"] = "2"
	projection.Facts["index_status"] = "fresh"
	projection.Facts["schema_version"] = "pinax.test.v1"
	projection.Facts["dry_run"] = "false"
	projection.Facts["remote_write"] = "true"
	projection.Facts["filter.updated_before"] = "2026-06-08"
	projection.Facts["issues.total"] = "1"
	projection.Facts["saved_path"] = ".pinax/plans/demo.json"
	projection.Data = map[string]any{"links": []domain.NoteLink{{SourcePath: "alpha.md", Target: "Missing"}}}

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render summary: %v", err)
	}
	got := summary.String()
	for _, want := range []string{"Partial", "Notes", "Index status", "Schema version", "Dry run", "Remote write", "Filter: updated before", "Total issues", "Saved path", "Fresh", "No", "Yes", "Broken"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing English text %q:\n%s", want, got)
		}
	}
	for _, machineText := range []string{"partial", "notes", "index_status", "schema_version", "dry_run", "remote_write", "filter.updated_before", "issues.total", "broken"} {
		if strings.Contains(got, machineText) {
			t.Fatalf("summary leaked machine text %q:\n%s", machineText, got)
		}
	}

	var agent bytes.Buffer
	if err := RenderWithOptions(&agent, ModeAgent, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	for _, want := range []string{"status=partial", "fact.notes=2", "fact.index_status=fresh", "fact.schema_version=pinax.test.v1", "fact.dry_run=false", "fact.remote_write=true"} {
		if !strings.Contains(agent.String(), want) {
			t.Fatalf("agent output missing stable key %q:\n%s", want, agent.String())
		}
	}
}

func TestNoteTagRecordFactsRenderInAllModes(t *testing.T) {
	projection := domain.NewProjection("note.tag", "Note tags updated.")
	projection.Facts["record_event"] = "note.metadata_updated"
	projection.Facts["ledger_seq"] = "2"
	projection.Facts["record_version"] = "2"
	projection.Facts["index_updated"] = "true"
	projection.Facts["tags"] = "safe,research"

	var jsonOut bytes.Buffer
	if err := RenderWithOptions(&jsonOut, ModeJSON, projection, RenderOptions{ColorMode: "always"}); err != nil {
		t.Fatalf("render json: %v", err)
	}
	for _, want := range []string{"\"command\":\"note.tag\"", "\"record_event\":\"note.metadata_updated\"", "\"ledger_seq\":\"2\"", "\"index_updated\":\"true\""} {
		if !strings.Contains(jsonOut.String(), want) {
			t.Fatalf("json output missing %q:\n%s", want, jsonOut.String())
		}
	}
	if strings.Contains(jsonOut.String(), "\x1b[") {
		t.Fatalf("json output contains ANSI:\n%s", jsonOut.String())
	}

	var agent bytes.Buffer
	if err := RenderWithOptions(&agent, ModeAgent, projection, RenderOptions{ColorMode: "always"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	for _, want := range []string{"command=note.tag", "fact.record_event=note.metadata_updated", "fact.ledger_seq=2", "fact.index_updated=true"} {
		if !strings.Contains(agent.String(), want) {
			t.Fatalf("agent output missing %q:\n%s", want, agent.String())
		}
	}
	if strings.Contains(agent.String(), "\x1b[") {
		t.Fatalf("agent output contains ANSI:\n%s", agent.String())
	}

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render summary: %v", err)
	}
	for _, want := range []string{"Record event", "Ledger sequence", "Record version", "Index updated", "note.metadata_updated"} {
		if !strings.Contains(summary.String(), want) {
			t.Fatalf("summary missing %q:\n%s", want, summary.String())
		}
	}
}

func TestSummaryOmitsSuccessExecutionStatus(t *testing.T) {
	projection := domain.NewProjection("asset.show", "Asset loaded.")
	projection.Facts["asset_path"] = "assets/diagram.png"
	var success bytes.Buffer
	if err := RenderWithOptions(&success, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render success summary: %v", err)
	}
	got := success.String()
	if strings.Contains(got, "Success") || strings.Contains(got, "status=success") || strings.Contains(got, "Status") {
		t.Fatalf("success summary leaked execution status:\n%s", got)
	}

	projection.Status = "partial"
	var partial bytes.Buffer
	if err := RenderWithOptions(&partial, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render partial summary: %v", err)
	}
	if !strings.Contains(partial.String(), "Status") || !strings.Contains(partial.String(), "Partial") {
		t.Fatalf("partial summary missing status:\n%s", partial.String())
	}
}

func TestRenderWithOptionsColorModeOverridesEnvironment(t *testing.T) {
	projection := domain.NewProjection("test.summary", "Color output test.")
	projection.Facts["notes"] = "2"
	t.Setenv("NO_COLOR", "1")
	t.Setenv("PINAX_COLOR", "never")

	var forced bytes.Buffer
	if err := RenderWithOptions(&forced, ModeSummary, projection, RenderOptions{ColorMode: "always", ThemeName: "high-contrast"}); err != nil {
		t.Fatalf("render forced color summary: %v", err)
	}
	if !strings.Contains(forced.String(), "\x1b[") {
		t.Fatalf("forced color summary missing ANSI:\n%s", forced.String())
	}

	var plain bytes.Buffer
	if err := RenderWithOptions(&plain, ModeSummary, projection, RenderOptions{ColorMode: "never", ThemeName: "pinax"}); err != nil {
		t.Fatalf("render plain summary: %v", err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("plain summary contains ANSI:\n%s", plain.String())
	}
}

func TestRenderWithOptionsMachineOutputsIgnoreHumanTheme(t *testing.T) {
	projection := domain.NewProjection("test.machine", "Machine output test.")
	projection.Facts["notes"] = "2"

	for _, mode := range []Mode{ModeJSON, ModeAgent, ModeEvents} {
		var out bytes.Buffer
		if err := RenderWithOptions(&out, mode, projection, RenderOptions{ColorMode: "always", ThemeName: "high-contrast"}); err != nil {
			t.Fatalf("render %s: %v", mode, err)
		}
		if strings.Contains(out.String(), "\x1b[") {
			t.Fatalf("%s output contains ANSI:\n%s", mode, out.String())
		}
	}
}

func TestSummaryMarkdownRenderingForNoteBody(t *testing.T) {
	projection := domain.NewProjection("note.show", "Local note loaded.")
	projection.Data = map[string]any{"note": domain.Note{Title: "Demo", Path: "notes/demo.md"}, "body": "# Heading\n\n- item\n"}

	var out bytes.Buffer
	if err := RenderWithOptions(&out, ModeSummary, projection, RenderOptions{ColorMode: "never", Width: 80, Markdown: MarkdownOptions{Enabled: true, Style: "ascii"}}); err != nil {
		t.Fatalf("render markdown note: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "• item") || strings.Contains(got, "- item") {
		t.Fatalf("markdown body was not rendered for humans:\n%s", got)
	}
}

func TestSummaryMarkdownDisabledKeepsPlainBody(t *testing.T) {
	projection := domain.NewProjection("template.render", "Template rendered.")
	projection.Data = map[string]any{"body": "# Heading\n\n- item\n"}

	var out bytes.Buffer
	if err := RenderWithOptions(&out, ModeSummary, projection, RenderOptions{ColorMode: "never", Width: 80, Markdown: MarkdownOptions{Enabled: false, Style: "ascii"}}); err != nil {
		t.Fatalf("render plain markdown body: %v", err)
	}
	if !strings.Contains(out.String(), "# Heading") {
		t.Fatalf("disabled markdown did not keep raw body:\n%s", out.String())
	}
}

func TestSummaryDimensionListRendersVisualShare(t *testing.T) {
	projection := domain.NewProjection("tag.list", "Organization view listed.")
	projection.Facts["dimension"] = "tag"
	projection.Facts["dimensions"] = "2"
	projection.Facts["notes"] = "3"
	projection.Data = map[string]any{
		"dimension": "tag",
		"items": []domain.DimensionCount{
			{Value: "research", Count: 3},
			{Value: "client", Count: 1},
		},
	}

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render summary: %v", err)
	}
	got := summary.String()
	for _, want := range []string{"Tags", "Count", "Share", "Heat", "research", "client", "75%", "25%", "##########", "###"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing tag visual %q:\n%s", want, got)
		}
	}

	var agent bytes.Buffer
	if err := RenderWithOptions(&agent, ModeAgent, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	if strings.Contains(agent.String(), "Share") || strings.Contains(agent.String(), "##########") {
		t.Fatalf("agent output leaked human visualization:\n%s", agent.String())
	}
}

func TestRenderWithOptionsCustomThemeFallsBackToPinaxRoles(t *testing.T) {
	projection := domain.NewProjection("test.summary", "Custom theme test.")
	projection.Status = "failed"

	var out bytes.Buffer
	if err := RenderWithOptions(&out, ModeSummary, projection, RenderOptions{ColorMode: "always", ThemeName: "custom", ThemeRoles: map[string]string{"danger": "196"}}); err != nil {
		t.Fatalf("render custom theme: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b[1;38;5;196mFailed") {
		t.Fatalf("custom danger role not applied:\n%s", got)
	}
	if !strings.Contains(got, "\x1b[38;5;240m") {
		t.Fatalf("custom theme did not fall back to pinax rule role:\n%s", got)
	}
}

func TestMarkdownRenderingDoesNotChangeJSONBody(t *testing.T) {
	projection := domain.NewProjection("note.show", "Local note loaded.")
	projection.Data = map[string]any{"body": "# Heading\n\n- item\n"}

	var out bytes.Buffer
	if err := RenderWithOptions(&out, ModeJSON, projection, RenderOptions{ColorMode: "always", Width: 80, Markdown: MarkdownOptions{Enabled: true, Style: "dark"}}); err != nil {
		t.Fatalf("render json markdown body: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "# Heading") || !strings.Contains(got, "- item") || strings.Contains(got, "\x1b[") || strings.Contains(got, "• item") {
		t.Fatalf("json body was styled or changed:\n%s", got)
	}
}

func TestTemplateInspectAgentOutputContract(t *testing.T) {
	projection := domain.NewProjection("template.inspect", "Template inspection complete.")
	projection.Facts["template"] = "meeting.notes"
	projection.Facts["use_cases"] = "meeting,sync"
	projection.Facts["after_create_action_count"] = "1"
	projection.Actions = []domain.Action{{Name: "create", Command: "pinax note add 'Meeting notes' --template meeting.notes --vault ./my-notes --json"}}
	var agent bytes.Buffer
	if err := RenderWithOptions(&agent, ModeAgent, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	got := agent.String()
	for _, want := range []string{"command=template.inspect", "fact.template=meeting.notes", "fact.after_create_action_count=1", "action.create="} {
		if !strings.Contains(got, want) {
			t.Fatalf("agent output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Template inspection complete") || strings.Contains(got, "Recommended next step") || strings.Contains(got, "Highlights") {
		t.Fatalf("agent output leaked human prose:\n%s", got)
	}
}

func TestProjectionActionsAgentActionsJSONActions(t *testing.T) {
	projection := domain.NewProjection("template.inspect", "Template inspection complete.")
	projection.Actions = []domain.Action{{Name: "primary", Command: "pinax template preview journal.daily --vault ./my-notes --json"}}
	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render summary: %v", err)
	}
	if !strings.Contains(summary.String(), "pinax template preview journal.daily --vault ./my-notes --json") {
		t.Fatalf("summary missing action:\n%s", summary.String())
	}
	var jsonOut bytes.Buffer
	if err := RenderWithOptions(&jsonOut, ModeJSON, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render json: %v", err)
	}
	if !strings.Contains(jsonOut.String(), `"actions"`) || !strings.Contains(jsonOut.String(), `"command":"pinax template preview journal.daily --vault ./my-notes --json"`) {
		t.Fatalf("json missing action:\n%s", jsonOut.String())
	}
	var agent bytes.Buffer
	if err := RenderWithOptions(&agent, ModeAgent, projection, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	if !strings.Contains(agent.String(), "action.primary=") {
		t.Fatalf("agent missing action:\n%s", agent.String())
	}
}
