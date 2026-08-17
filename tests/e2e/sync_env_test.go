package e2e

import (
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestSyncEnvLoader exercises the encrypted runtime dotenv loader lifecycle
// end-to-end via testscript: init, set, list, unlock (in-memory), materialize
// (0600), clean, doctor, wrong-key failure, and the managed .gitignore block.
// The fake unlock provider is keyed by PINAX_SYNC_FAKE_KEY; XDG_CONFIG_HOME
// points at a writable per-script dir so the device-local profile store works
// in the sandboxed HOME=/no-home testscript environment.
func TestSyncEnvLoader(t *testing.T) {
	t.Parallel()
	runE2ETestScript(t, "testdata/sync_env/scripts", func(env *testscript.Env) error {
		env.Vars = append(env.Vars,
			"XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"),
			"PINAX_SYNC_FAKE_KEY=env-loader-e2e-key",
		)
		return nil
	})
}
