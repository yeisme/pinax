package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestDailyKnowledgeWorkflow(t *testing.T) {
	t.Parallel()
	testscript.Run(t, testscript.Params{
		Dir: "testdata/daily_knowledge/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"))
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){"vault-digest": researchVaultDigest},
	})
}
