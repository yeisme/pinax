package e2e

import "testing"

func TestUnifiedWorkspace(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/unified_workspace/scripts", nil)
}
