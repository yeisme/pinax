package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/yeisme/pinax/internal/cloudclient/mlptest"
)

func TestCloud(t *testing.T) {
	fake := mlptest.New(mlptest.Config{VaultID: "ws_123", SessionToken: "fake-token"})
	t.Cleanup(fake.Close)
	testscript.Run(t, testscript.Params{
		Dir: "testdata/cloud/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "PINAX_FAKE_CLOUD_URL="+fake.Endpoint(), "PINAX_CLOUD_TOKEN=fake-token")
			return nil
		},
	})
}
