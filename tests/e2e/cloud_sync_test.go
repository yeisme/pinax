package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/yeisme/pinax/internal/remote"
)

func TestCloud(t *testing.T) {
	fake := remote.NewFakeServer()
	t.Cleanup(fake.Close)
	testscript.Run(t, testscript.Params{
		Dir: "testdata/cloud/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "PINAX_FAKE_CLOUD_URL="+fake.URL, "PINAX_CLOUD_TOKEN=fake-token")
			return nil
		},
	})
}
