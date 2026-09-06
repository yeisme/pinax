package e2e

import "testing"

func TestTrustDiscoveryChain(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/trust_discovery/scripts", nil)
}
