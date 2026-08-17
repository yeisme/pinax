package e2e

import "testing"

func TestPluginRuntime(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/plugin_runtime/scripts", nil)
}
