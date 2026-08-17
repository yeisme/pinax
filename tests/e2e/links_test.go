package e2e

import (
	"testing"
)

func TestLinkProjection(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/links/scripts", nil)
}
