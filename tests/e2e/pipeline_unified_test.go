package e2e

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestPipelineUnifiedE2E drives the unified pipeline UX end to end:
// plan save → vault drift → plan_stale rejection → --allow-stale apply →
// receipt surfaces in pinax pipeline status → pinax pipeline show in both
// plan and receipt forms, including the --events stage contract.
func TestPipelineUnifiedE2E(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/pipeline/scripts", func(env *testscript.Env) error {
		env.Vars = append(env.Vars, "NO_COLOR=")
		return nil
	})
}
