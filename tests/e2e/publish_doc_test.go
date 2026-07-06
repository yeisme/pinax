package e2e

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestPublishDoc(t *testing.T) {
	runE2ETestScript(t, "testdata/publish_doc/scripts", func(env *testscript.Env) error {
		// fake-lark-cli 在工作目录写 calllog，断言 renderer=native-docx 时未走 markdown +create。
		env.Vars = append(env.Vars, "PINAX_FAKE_LARK_CALLLOG=lark-calllog.jsonl")
		return nil
	})
}
