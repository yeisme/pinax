package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReviewCommand_HelpExists verifies the experimental `pinax review`
// intent facade exposes English help text describing the read-only inbox and
// its action/--yes flags.
func TestReviewCommand_HelpExists(t *testing.T) {
	help := runCLI(t, "review", "--help")
	for _, want := range []string{
		"Aggregate pending proposals",
		"Default is read-only",
		"experimental additive facade",
		"Usage",
		"Flags",
		"--action",
		"--item",
		"--yes",
		"--scope",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("review --help missing %q:\n%s", want, help)
		}
	}
}

// TestReviewCommand_InCommandTree verifies the root help lists the additive
// `review` entry point alongside the existing command tree.
func TestReviewCommand_InCommandTree(t *testing.T) {
	rootHelp := runCLI(t, "--help")
	if !strings.Contains(rootHelp, "  review") {
		t.Fatalf("root --help should list review:\n%s", rootHelp)
	}
}

// TestReviewCommand_DefaultReadOnly verifies that `pinax review` with no action
// defaults to the read-only list mode and emits a success projection with the
// aggregated inbox facts surface.
func TestReviewCommand_DefaultReadOnly(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "review", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("review --json invalid envelope: %v\n%s", err, out)
	}
	if envelope["command"] != "review" {
		t.Fatalf("command = %#v, want review (default list mode)", envelope["command"])
	}
	if envelope["status"] != "success" {
		t.Fatalf("status = %#v, want success (read-only default)", envelope["status"])
	}
	facts, ok := envelope["facts"].(map[string]any)
	if !ok {
		t.Fatalf("facts missing or wrong type: %#v", envelope["facts"])
	}
	for _, key := range []string{"total_items", "high_risk_count", "experimental"} {
		if _, ok := facts[key]; !ok {
			t.Fatalf("review facts missing %s: %#v", key, facts)
		}
	}
}

// TestReviewCommand_ApproveRequiresYes verifies that the write action approve
// is blocked without the explicit --yes confirmation, regardless of item.
func TestReviewCommand_ApproveRequiresYes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out, err := runCLIExpectError("review", "--action", "approve", "--item", "prop-abc", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("approve without --yes should fail, got success:\n%s", out)
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("approve error should mention --yes, got: %v", err)
	}
}

// TestReviewCommand_RejectRequiresYes verifies that the write action reject is
// blocked without the explicit --yes confirmation, regardless of item.
func TestReviewCommand_RejectRequiresYes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out, err := runCLIExpectError("review", "--action", "reject", "--item", "prop-abc", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("reject without --yes should fail, got success:\n%s", out)
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("reject error should mention --yes, got: %v", err)
	}
}

// TestReviewCommand_ShowNoYesNeeded verifies that the read-only show action is
// routed to its handler without the --yes gate. On an empty inbox the handler
// reports the item is absent rather than demanding confirmation, which proves
// show bypasses the approve/reject write gate.
func TestReviewCommand_ShowNoYesNeeded(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out, err := runCLIExpectError("review", "--action", "show", "--item", "prop-missing", "--vault", root, "--json")
	if err != nil {
		if strings.Contains(err.Error(), "requires --yes") {
			t.Fatalf("show must not gate on --yes, got: %v", err)
		}
		return
	}
	if strings.Contains(out, "requires --yes") {
		t.Fatalf("show output should not mention a --yes requirement:\n%s", out)
	}
}
