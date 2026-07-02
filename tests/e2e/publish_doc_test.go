package e2e

import "testing"

func TestPublishDoc(t *testing.T) {
	runE2ETestScript(t, "testdata/publish_doc/scripts", nil)
}
