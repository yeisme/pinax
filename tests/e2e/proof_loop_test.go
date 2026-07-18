package e2e

import (
	"testing"
)

// TestProofLoop drives the Pinax agent-safe proof loop end to end through the
// real CLI binary: init → capture → index → search → links → doctor → plan →
// snapshot → apply, plus deterministic fixture coverage and redaction gates.
func TestProofLoop(t *testing.T) {
	runE2ETestScript(t, "testdata/proof_loop/scripts", nil)
}

// TestProofLoopReleaseCoreFiveMinute is the canonical release gate alias for the
// five-minute proof loop: a user or agent can drive a real Markdown vault from
// empty directory through capture, retrieve, diagnose, plan, snapshot, apply,
// and restore using only the installed pinax binary — no provider credentials,
// Cloud Sync, daemon, MCP, dashboard, or source checkout required.
func TestProofLoopReleaseCoreFiveMinute(t *testing.T) {
	runE2ETestScript(t, "testdata/proof_loop/scripts", nil)
}
