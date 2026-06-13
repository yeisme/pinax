package e2e

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestRecordLedger(t *testing.T) {
	runE2ETestScript(t, "testdata/records/scripts", func(env *testscript.Env) error {
		env.Vars = append(env.Vars, "NO_COLOR=")
		return nil
	})
}
