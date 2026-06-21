package e2e

import "testing"

func TestDataviewDatabase(t *testing.T) {
	runE2ETestScript(t, "testdata/dataview_database/scripts", nil)
}
