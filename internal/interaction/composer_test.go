package interaction

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDecorateCaptureUsesDailyReviewNextAction(t *testing.T) {
	p := domain.NewProjection("inbox.capture", "Inbox note captured.")
	p.Facts["writes"] = "true"
	p.Actions = []domain.Action{{Name: "show", Command: "pinax note show n_123 --vault '/private/vault'"}}

	Decorate(&p)
	if p.Interaction == nil {
		t.Fatal("interaction projection missing")
	}
	if p.Interaction.SchemaVersion != domain.DailyInteractionSchemaVersion || p.Interaction.Phase != "capture" {
		t.Fatalf("unexpected interaction: %#v", p.Interaction)
	}
	if p.Interaction.State != "ready" || p.Interaction.ReadOnly {
		t.Fatalf("capture safety state = %#v", p.Interaction)
	}
	if got := p.Interaction.RecommendedNextAction.Command; got != "pinax inbox review --limit 5 --json" {
		t.Fatalf("capture next action = %q", got)
	}
	if p.Facts["interaction_writes"] != "true" {
		t.Fatalf("interaction facts missing writes: %#v", p.Facts)
	}
}

func TestDecorateSearchBuildsBoundedPreviewAction(t *testing.T) {
	p := domain.NewProjection("note.search", "Search completed.")
	p.Facts["writes"] = "false"
	p.Data = struct {
		Results []struct {
			Note struct {
				ID   string
				Path string
			} `json:"note"`
		} `json:"results"`
	}{Results: []struct {
		Note struct {
			ID   string
			Path string
		} `json:"note"`
	}{{Note: struct {
		ID   string
		Path string
	}{ID: "note_abc", Path: "notes/a.md"}}}}

	Decorate(&p)
	if p.Interaction == nil || p.Interaction.Phase != "retrieve" {
		t.Fatalf("unexpected interaction: %#v", p.Interaction)
	}
	if got := p.Interaction.RecommendedNextAction.Command; got != "pinax note preview note_abc --json" {
		t.Fatalf("search next action = %q", got)
	}
	if p.Interaction.ReadOnly == false || p.Interaction.Writes {
		t.Fatalf("search safety state = %#v", p.Interaction)
	}
}

func TestDecorateReviewPrefersScopedOrganizationAndSanitizesVault(t *testing.T) {
	p := domain.NewProjection("inbox.review", "Pending notes ready for review.")
	p.Facts["writes"] = "false"
	p.Actions = []domain.Action{
		{Name: "inspect_1", Command: "pinax search show note_1 --vault '/private/vault'"},
		{Name: "organize_preview", Command: "pinax organize plan --vault '/private/vault' --note note_1"},
		{Name: "next", Command: "pinax inbox review --after note_1 --limit 5 --vault '/private/vault'"},
	}

	Decorate(&p)
	if p.Interaction == nil || p.Interaction.Phase != "review" {
		t.Fatalf("unexpected interaction: %#v", p.Interaction)
	}
	got := p.Interaction.RecommendedNextAction.Command
	if got != "pinax organize plan --vault <vault> --note note_1" {
		t.Fatalf("review next action = %q", got)
	}
	if len(p.Interaction.RecoveryActions) != 2 {
		t.Fatalf("recovery actions = %#v", p.Interaction.RecoveryActions)
	}
	for _, action := range append([]domain.Action{*p.Interaction.RecommendedNextAction}, p.Interaction.RecoveryActions...) {
		if containsAbsoluteVault(action.Command) {
			t.Fatalf("absolute vault leaked in interaction action: %q", action.Command)
		}
	}
}

func TestDecorateUnknownRequiresReconcile(t *testing.T) {
	p := domain.NewErrorProjection("organize.apply", &domain.CommandError{Code: "reconcile_required", Message: "operation status is unknown", Hint: "pinax operation reconcile <operation-id> --json"})
	p.Actions = []domain.Action{{Name: "reconcile", Command: "pinax operation reconcile op_1 --json"}}

	Decorate(&p)
	if p.Interaction == nil || p.Interaction.State != "unknown" {
		t.Fatalf("unknown interaction = %#v", p.Interaction)
	}
	if p.Interaction.RecommendedNextAction == nil || p.Interaction.RecommendedNextAction.Name != "reconcile" {
		t.Fatalf("reconcile action = %#v", p.Interaction.RecommendedNextAction)
	}
}

func TestDecorateCheckpointIsWriteBearing(t *testing.T) {
	p := domain.NewProjection("continue.checkpoint", "Continuity checkpoint saved.")
	p.Actions = []domain.Action{{Name: "resume", Command: "pinax continue"}}
	Decorate(&p)
	if p.Interaction == nil || !p.Interaction.Writes || p.Interaction.ReadOnly {
		t.Fatalf("checkpoint safety state = %#v", p.Interaction)
	}
	if p.Interaction.Phase != "close" {
		t.Fatalf("checkpoint phase = %#v", p.Interaction)
	}
}

func TestDecorateIgnoresUnrelatedCommands(t *testing.T) {
	p := domain.NewProjection("token.list", "Tokens listed.")
	Decorate(&p)
	if p.Interaction != nil {
		t.Fatalf("unrelated command received interaction: %#v", p.Interaction)
	}
}

func containsAbsoluteVault(command string) bool {
	return command == "" || (len(command) > 0 && (contains(command, "/private/") || contains(command, "\\private\\")))
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
