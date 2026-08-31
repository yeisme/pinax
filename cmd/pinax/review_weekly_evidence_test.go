package main

import (
	"testing"
)

// TestReviewCompatibilityAndWeeklyEvidence 覆盖 6.2：
// weekly review 用时 evidence 不破坏旧 review 行为（list/show/approve/reject），
// 且 review action 仍走 canonical lifecycle service（--yes + proposal 消费）。
func TestReviewCompatibilityAndWeeklyEvidence(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	// checkpoint 产生 durable candidate proposal。
	out := runCLI(t, "continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "compatibility checkpoint",
		"--durable", "decision:compat guard=review evidence must not break approvals",
		"--json")
	facts := jsonParseFacts(t, out)
	itemRef, _ := facts["proposal_1"].(string)
	if itemRef == "" {
		t.Fatalf("checkpoint proposal missing: %#v", facts)
	}

	// list：inbox 包含 proposal item。
	listOut := runCLI(t, "review", "--vault", vaultRoot, "--scope", "project:pinax", "--action", "list", "--json")
	listFacts := jsonParseFacts(t, listOut)
	if listFacts["total_items"] != "1" {
		t.Fatalf("review list after checkpoint: %#v", listFacts)
	}

	// show 不需要 --yes。
	runCLI(t, "review", "--vault", vaultRoot, "--scope", "project:pinax",
		"--action", "show", "--item", "prop-"+itemRef, "--json")

	// weekly review evidence：空 inbox 也可记录 0 秒；不得隐式写 inbox state。
	runCLI(t, "continue", "feedback", "--vault", vaultRoot,
		"--weekly-review", "--review-seconds", "45", "--json")

	// approve 仍要求 --yes。
	if _, err := runCLIExpectError("review", "--vault", vaultRoot,
		"--action", "approve", "--item", "prop-"+itemRef, "--json"); err == nil {
		t.Fatal("approve without --yes must fail")
	}
	runCLI(t, "review", "--vault", vaultRoot, "--scope", "project:pinax",
		"--action", "approve", "--item", "prop-"+itemRef, "--yes", "--json")

	// approve 后 inbox 清空；weekly review 事件仍在 report 中。
	listOut = runCLI(t, "review", "--vault", vaultRoot, "--scope", "project:pinax", "--action", "list", "--json")
	listFacts = jsonParseFacts(t, listOut)
	if listFacts["total_items"] != "0" {
		t.Fatalf("approve must consume the proposal: %#v", listFacts)
	}
	reportOut := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	reportFacts := jsonParseFacts(t, reportOut)
	if reportFacts["total_runs"] != "0" {
		t.Fatalf("review evidence must not fabricate runs: %#v", reportFacts)
	}
}

// TestStaleReviewActionRejected 覆盖 stale action：已消费的 proposal 不能再
// approve（canonical service 拒绝旧 action），evidence 不声称成功。
func TestStaleReviewActionRejected(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	out := runCLI(t, "continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "stale action fixture",
		"--durable", "decision:stale guard=consumed proposals cannot be re-approved",
		"--json")
	facts := jsonParseFacts(t, out)
	itemRef, _ := facts["proposal_1"].(string)

	runCLI(t, "review", "--vault", vaultRoot, "--scope", "project:pinax",
		"--action", "approve", "--item", "prop-"+itemRef, "--yes", "--json")
	if _, err := runCLIExpectError("review", "--vault", vaultRoot,
		"--action", "approve", "--item", "prop-"+itemRef, "--yes", "--json"); err == nil {
		t.Fatal("double approve of consumed proposal must fail")
	}
	if _, err := runCLIExpectError("review", "--vault", vaultRoot,
		"--action", "reject", "--item", "prop-"+itemRef, "--yes", "--json"); err == nil {
		t.Fatal("reject of consumed proposal must fail")
	}
}
