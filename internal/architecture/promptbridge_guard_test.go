package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestPromptStateLayersStayBehindBridge locks the integration boundary from
// pinax-prompt-repository-import-v1: only internal/promptbridge may open the
// shared engine or touch source adapters; application and CLI layers use
// public SDK DTO types through bridge ports and never read promptrepo state
// directly.
func TestPromptStateLayersStayBehindBridge(t *testing.T) {
	repoRoot := findRepoRoot(t)
	blocked := map[string]bool{
		"github.com/yeisme/promptrepo/engine": true,
		"github.com/yeisme/promptrepo/source": true,
	}
	checkGoImports(t, repoRoot, []string{"internal/app", "internal/cli", "cmd/pinax"}, func(path, imp string) {
		if blocked[imp] {
			t.Fatalf("%s imports %s; the shared engine and source adapters stay behind internal/promptbridge", path, imp)
		}
		for _, forbidden := range []string{"internal/registry", "templates/registry", "github.com/yeisme/registry"} {
			if strings.Contains(imp, forbidden) {
				t.Fatalf("%s imports %s; Registry internals are off limits", path, imp)
			}
		}
	})
}

// TestPromptBridgeStaysConsumerNeutral keeps the bridge away from CLI/output
// rendering, Registry internals, and promptrepo state internals.
func TestPromptBridgeStaysConsumerNeutral(t *testing.T) {
	repoRoot := findRepoRoot(t)
	blocked := map[string]bool{
		modulePath + "/internal/cli":    true,
		modulePath + "/internal/output": true,
	}
	checkGoImports(t, repoRoot, []string{filepath.Join("internal", "promptbridge")}, func(path, imp string) {
		if blocked[imp] {
			t.Fatalf("%s imports %s; the bridge must return domain projections only", path, imp)
		}
		if imp == "github.com/yeisme/promptrepo/source" && !strings.Contains(path, "testfixture") {
			t.Fatalf("%s imports %s; source adapters stay behind the SDK engine", path, imp)
		}
	})
}
