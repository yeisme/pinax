package e2e

import "testing"

func TestObsidianCompat(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/obsidian_compat/scripts", nil)
}
