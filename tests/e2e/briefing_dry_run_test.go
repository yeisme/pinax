package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestBriefingDryRun(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/briefing_dry_run/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			return nil
		},
	})
}
