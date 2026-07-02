package e2e

import "testing"

func TestObsidianCompat(t *testing.T) {
	runE2ETestScript(t, "testdata/obsidian_compat/scripts", nil)
}
