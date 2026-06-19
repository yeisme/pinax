package e2e

import "testing"

func TestPublishProfile(t *testing.T) {
	runE2ETestScript(t, "testdata/publish/scripts", nil)
}
