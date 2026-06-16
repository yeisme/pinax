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
