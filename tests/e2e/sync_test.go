package e2e

import (
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestSyncOfflineAndRedaction(t *testing.T) {
	runE2ETestScript(t, "testdata/sync/scripts", func(env *testscript.Env) error {
		env.Vars = append(env.Vars, "XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"))
		return nil
	})
}
