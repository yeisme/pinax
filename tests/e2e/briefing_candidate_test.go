package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestBriefingCandidate(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/briefing_candidate/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			return nil
		},
	})
}
