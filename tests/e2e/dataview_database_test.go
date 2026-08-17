package e2e

import "testing"

func TestDataviewDatabase(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/dataview_database/scripts", nil)
}
