package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func writeBrainTrustVault(t *testing.T, root string) {
	t.Helper()
	fixtures := map[string]string{
		filepath.Join(root, "notes", "alpha.md"): "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nupdated_at: 2026-09-06T10:00:00+00:00\nverified:\n  - by: human:ye\n    at: 2026-09-06T10:30:00+00:00\n---\n\n# Alpha\n\nalpha body marker\n",
		filepath.Join(root, "notes", "beta.md"):  "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nupdated_at: 2026-09-06T09:00:00+00:00\n---\n\n# Beta\n\nbeta body marker\n",
		filepath.Join(root, "notes", "gamma.md"): "---\nschema_version: pinax.note.v1\nnote_id: note_gamma\ntitle: Gamma\nupdated_at: 2026-09-06T08:00:00+00:00\nverified:\n  - by: agent:pinax/0.9.0\n    at: 2026-09-05T08:00:00+00:00\n---\n\n# Gamma\n\ngamma body marker\n",
	}
	for path, body := range fixtures {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
}

func TestBrainAnswerSourcesCarryTrustAndOrderCLI(t *testing.T) {
	t.Parallel()
	svc := NewService()
	root := t.TempDir()
	writeBrainTrustVault(t, root)

	projection, err := svc.BrainAnswerPreview(context.Background(), BrainAnswerRequest{VaultPath: root, Question: "marker"})
	if err != nil {
		t.Fatalf("brain answer: %v", err)
	}
	answer, ok := projection.Data.(domain.AgentBrainAnswer)
	if !ok {
		t.Fatalf("brain answer data type = %T", projection.Data)
	}
	if len(answer.Sources) < 3 {
		t.Fatalf("sources = %#v", answer.Sources)
	}
	byPath := map[string]domain.AgentBrainSource{}
	for _, source := range answer.Sources {
		byPath[source.Path] = source
	}
	if byPath["notes/alpha.md"].Trust != domain.TrustTierHuman {
		t.Fatalf("alpha trust = %q", byPath["notes/alpha.md"].Trust)
	}
	if byPath["notes/beta.md"].Trust != domain.TrustTierUnverified {
		t.Fatalf("beta trust = %q", byPath["notes/beta.md"].Trust)
	}
	if byPath["notes/gamma.md"].Trust != domain.TrustTierMachine {
		t.Fatalf("gamma trust = %q", byPath["notes/gamma.md"].Trust)
	}
	// 排序：human 在前，unverified 默认排在 human 之后（不剔除）。
	first := answer.Sources[0]
	if first.Trust != domain.TrustTierHuman {
		t.Fatalf("first source = %#v, want human first", first)
	}
	for i, source := range answer.Sources {
		if source.Trust == domain.TrustTierUnverified {
			for _, earlier := range answer.Sources[:i] {
				if earlier.Trust == domain.TrustTierHuman {
					continue
				}
			}
			break
		}
	}
	lastHumanIndex := -1
	firstUnverifiedIndex := -1
	for i, source := range answer.Sources {
		if source.Trust == domain.TrustTierHuman && i > lastHumanIndex {
			lastHumanIndex = i
		}
		if source.Trust == domain.TrustTierUnverified && firstUnverifiedIndex == -1 {
			firstUnverifiedIndex = i
		}
	}
	if lastHumanIndex > firstUnverifiedIndex {
		t.Fatalf("unverified must sort after human: sources=%#v", answer.Sources)
	}
}

func TestBrainMaintenanceProposesReverifyForStaleHumanNotes(t *testing.T) {
	t.Parallel()
	svc := NewService()
	root := t.TempDir()
	writeBrainTrustVault(t, root)
	// alpha 是 human verified 但已 stale。
	stalePath := filepath.Join(root, "notes", "alpha.md")
	body, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("read alpha: %v", err)
	}
	updated := "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nupdated_at: 2026-09-06T10:00:00+00:00\nverified:\n  - by: human:ye\n    at: 2026-09-06T10:30:00+00:00\nstale_after: 2020-01-01T00:00:00+00:00\n---\n\n# Alpha\n\nalpha body marker\n"
	if string(body) == updated {
		t.Fatalf("fixture drift")
	}
	if err := os.WriteFile(stalePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write stale alpha: %v", err)
	}

	projection, err := svc.BrainMaintenancePlan(context.Background(), BrainMaintenanceRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("brain maintain: %v", err)
	}
	plan, ok := projection.Data.(domain.AgentBrainMaintenancePlan)
	if !ok {
		t.Fatalf("plan data type = %T", projection.Data)
	}
	found := false
	for _, operation := range plan.Operations {
		if operation.Kind != "reverify_stale_human" {
			continue
		}
		found = true
		if len(operation.Evidence) == 0 || !containsBrainEvidence(operation.Evidence, "notes/alpha.md") {
			t.Fatalf("reverify evidence = %#v", operation.Evidence)
		}
		if operation.Status != "candidate" {
			t.Fatalf("reverify status = %q, want candidate", operation.Status)
		}
		if !containsBrainAction(operation.NextAction.Command, "note verify") {
			t.Fatalf("reverify next action = %q", operation.NextAction.Command)
		}
	}
	if !found {
		t.Fatalf("maintenance plan missing reverify_stale_human: %#v", plan.Operations)
	}
	// 只候选不写盘：alpha 内容保持不变（未被自动 verify）。
	after, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("re-read alpha: %v", err)
	}
	if string(after) != updated {
		t.Fatalf("brain maintain must not modify notes")
	}
}

func containsBrainEvidence(evidence []string, want string) bool {
	for _, item := range evidence {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func containsBrainAction(command, want string) bool {
	return strings.Contains(command, want)
}
