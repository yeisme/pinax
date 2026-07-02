package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPluginInstallWritesRegistryLockAndAudit(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "project-dashboard")
	writePluginTestFile(t, filepath.Join(pluginDir, "dist", "plugin.wasm"), "fake wasm bytes")
	writePluginTestFile(t, filepath.Join(pluginDir, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: project-dashboard
name: Project Dashboard
version: 0.1.0
runtime:
  kind: wasm
  entrypoint: dist/plugin.wasm
capabilities:
  - id: render_dashboard
    kind: view.render
permissions:
  vault:
    read: projection
  network: false
budgets:
  timeout_ms: 3000
  max_input_bytes: 262144
  max_output_bytes: 262144
  max_memory_mb: 64
`)

	store := Store{Root: root, Now: func() time.Time { return time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) }}
	installed, err := store.Install(pluginDir, "vault")
	if err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if installed.Enabled || installed.ID != "project-dashboard" || installed.ManifestSHA256 == "" {
		t.Fatalf("installed plugin = %#v", installed)
	}
	for _, path := range []string{store.registryPath(), store.lockPath(), filepath.Join(root, ".pinax", "events", "plugin-audit.jsonl")} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(body), root) || strings.Contains(string(body), "fake wasm bytes") {
			t.Fatalf("plugin asset leaked host path or entrypoint body:\n%s", body)
		}
	}
}
