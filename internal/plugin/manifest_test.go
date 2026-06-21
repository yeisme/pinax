package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidationAcceptsSafeWASMPlugin(t *testing.T) {
	root := t.TempDir()
	writePluginTestFile(t, filepath.Join(root, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
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

	result, err := ValidateManifestPath(root)
	if err != nil {
		t.Fatalf("validate manifest: %v", err)
	}
	if result.Manifest.ID != "project-dashboard" || result.Manifest.Runtime.Kind != RuntimeWASM || result.CapabilityCount != 1 || result.WriteStatus {
		t.Fatalf("result = %#v", result)
	}
}

func TestManifestValidationRejectsSensitiveContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "authorization", body: "Authorization: Bearer raw-token"},
		{name: "cookie", body: "Cookie: session=raw-cookie"},
		{name: "webhook", body: "webhook_url: https://hooks.example.invalid/raw"},
		{name: "secret", body: "api_token: raw-secret-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writePluginTestFile(t, filepath.Join(root, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: unsafe-plugin
name: Unsafe Plugin
version: 0.1.0
runtime:
  kind: python
  entrypoint: plugin.py
capabilities:
  - id: import_notes
    kind: import.transform
permissions:
  env:
    VALUE: `+tc.body+`
budgets:
  timeout_ms: 3000
  max_input_bytes: 262144
  max_output_bytes: 262144
  max_memory_mb: 64
`)
			_, err := ValidateManifestPath(root)
			validationErr, ok := err.(*ValidationError)
			if !ok || validationErr.Code != "plugin_manifest_secret_rejected" {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}

func TestManifestValidationRejectsUnsupportedRuntimeAndCapability(t *testing.T) {
	root := t.TempDir()
	writePluginTestFile(t, filepath.Join(root, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: bad-plugin
name: Bad Plugin
version: 0.1.0
runtime:
  kind: ruby
  entrypoint: /tmp/plugin.rb
capabilities:
  - id: replace_core
    kind: core.override
budgets:
  timeout_ms: 0
  max_input_bytes: 0
  max_output_bytes: 0
  max_memory_mb: 0
`)

	_, err := ValidateManifestPath(root)
	validationErr, ok := err.(*ValidationError)
	if !ok || validationErr.Code != "plugin_manifest_runtime_invalid" || len(validationErr.Issues) < 3 {
		t.Fatalf("err = %#v", err)
	}
}

func writePluginTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
