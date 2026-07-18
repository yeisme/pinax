package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestSyncRepoDeclarative exercises the declarative sync repository lifecycle:
// init, secret set/list, bootstrap, plan, doctor. The runtime unlock uses the
// fake provider keyed by PINAX_SYNC_FAKE_KEY; XDG_CONFIG_HOME points at a
// writable per-script dir so the device-local stored:// secret store works in
// the sandboxed HOME=/no-home testscript environment.
func TestSyncRepoDeclarative(t *testing.T) {
	runE2ETestScript(t, "testdata/sync_repo/scripts", func(env *testscript.Env) error {
		env.Vars = append(env.Vars,
			"XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"),
			"PINAX_SYNC_FAKE_KEY=repo-declarative-test-key",
		)
		// Ensure the host PATH includes fake CLIs already wired by runE2ETestScript.
		return nil
	})
	// keep os import used if the helper ever needs direct env access.
	_ = os.Getenv("PINAX_SYNC_FAKE_KEY")
}
