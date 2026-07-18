package e2e

import "testing"

func TestShareLANReadOnly(t *testing.T) {
	runE2ETestScript(t, "testdata/share/scripts", nil)
}
