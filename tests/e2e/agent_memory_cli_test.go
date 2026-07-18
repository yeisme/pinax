package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestAgentMemoryCLI exercises the experimental `pinax agent` command tree
// via testscript: propose → approve → recall → context → handoff → feedback.
func TestAgentMemoryCLI(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/agent_memory/scripts",
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"validate-json-envelope": cmdValidateJSONEnvelope,
		},
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"PINAX_REPO_ROOT="+repoRoot,
				"NO_COLOR=1",
			)
			return nil
		},
	})
}
