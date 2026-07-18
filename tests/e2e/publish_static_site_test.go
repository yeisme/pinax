package e2e

import (
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestPublishStaticSite(t *testing.T) {
	runE2ETestScript(t, "testdata/publish_static_site/scripts", func(env *testscript.Env) error {
		repoRoot, err := filepath.Abs("../..")
		if err != nil {
			return err
		}
		env.Vars = append(env.Vars, "PINAX_WEB_RENDERER_DIR="+filepath.Join(repoRoot, "web", "pinax-web-renderer"))
		return nil
	})
}
