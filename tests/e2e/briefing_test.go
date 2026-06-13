package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestBriefing(t *testing.T) {
	feishu := newFakeHTTPServer(t)
	testscript.Run(t, testscript.Params{
		Dir: "testdata/briefing/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "PINAX_FAKE_FEISHU_URL="+feishu.URL)
			return nil
		},
	})
}
