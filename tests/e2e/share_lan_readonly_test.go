package e2e

import "testing"

func TestShareLANReadOnly(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/share/scripts", nil)
}
