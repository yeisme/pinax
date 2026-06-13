package e2e

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestJournalIndexTemplateStarterTemplatesIndexPageRefreshTemplateRecommendTemplateCompletionTemplateNextAction(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/journal_index_template/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "NO_COLOR=")
			return nil
		},
	})
}
