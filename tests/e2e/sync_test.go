package e2e

import (
	"testing"
)

func TestSyncOfflineAndRedaction(t *testing.T) {
	runE2ETestScript(t, "testdata/sync/scripts", nil)
}
