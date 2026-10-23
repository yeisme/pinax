package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestDailyReviewFollowthroughWorkflow(t *testing.T) {
	testscript.Run(t, testscript.Params{Dir: "testdata/daily_review_followthrough/scripts", Setup: func(env *testscript.Env) error {
		env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"), "PINAX_API_URL=")
		return nil
	}, Cmds: map[string]func(*testscript.TestScript, bool, []string){"vault-digest": researchVaultDigest, "review-fact": func(ts *testscript.TestScript, neg bool, args []string) {
		if neg || len(args) != 3 {
			ts.Fatalf("usage: review-fact <file> <key> <env>")
		}
		var p struct {
			Facts map[string]string `json:"facts"`
		}
		if json.Unmarshal([]byte(ts.ReadFile(args[0])), &p) != nil || p.Facts[args[1]] == "" {
			ts.Fatalf("missing fact")
		}
		ts.Setenv(args[2], p.Facts[args[1]])
	}, "review-action": func(ts *testscript.TestScript, neg bool, args []string) {
		if neg || len(args) != 3 {
			ts.Fatalf("usage: review-action <file> <name> <env>")
		}
		var p struct {
			Actions []struct {
				Name    string `json:"name"`
				Command string `json:"command"`
			} `json:"actions"`
		}
		if json.Unmarshal([]byte(ts.ReadFile(args[0])), &p) != nil {
			ts.Fatalf("invalid projection")
		}
		for _, a := range p.Actions {
			if a.Name == args[1] {
				ts.Setenv(args[2], a.Command)
				return
			}
		}
		ts.Fatalf("missing action")
	}}})
}
