package e2e

import (
	"testing"
)

func TestLinkProjection(t *testing.T) {
	runE2ETestScript(t, "testdata/links/scripts", nil)
}
