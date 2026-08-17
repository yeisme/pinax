package e2e

import "testing"

func TestPublishProfile(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/publish/scripts", nil)
}
