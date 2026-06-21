package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginValidateManifestJSONContract(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "project-dashboard")
	writeCLIFixture(t, filepath.Join(pluginDir, "dist", "plugin.wasm"), "fake wasm bytes")
	writeCLIFixture(t, filepath.Join(pluginDir, "schemas", "render-input.json"), `{"type":"object"}`)
	writeCLIFixture(t, filepath.Join(pluginDir, "schemas", "render-output.json"), `{"type":"object"}`)
	writeCLIFixture(t, filepath.Join(pluginDir, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: project-dashboard
name: Project Dashboard
version: 0.1.0
runtime:
  kind: wasm
  entrypoint: dist/plugin.wasm
capabilities:
  - id: render_dashboard
    kind: view.render
    input_schema: schemas/render-input.json
    output_schema: schemas/render-output.json
permissions:
  vault:
    read: projection
    write: action_plan
  filesystem:
    read: none
    write: temp
  network: false
budgets:
  timeout_ms: 3000
  max_input_bytes: 262144
  max_output_bytes: 262144
  max_memory_mb: 64
`)

	out := runCLI(t, "plugin", "validate", pluginDir, "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "plugin.validate", "success")
	facts := jsonParseFacts(t, out)
	if facts["plugin_id"] != "project-dashboard" || facts["version"] != "0.1.0" || facts["runtime"] != "wasm" || facts["capabilities"] != "1" || facts["write_status"] != "false" {
		t.Fatalf("plugin validate facts = %#v", facts)
	}
	if fileExists(filepath.Join(root, ".pinax", "plugins", "registry.json")) || fileExists(filepath.Join(root, ".pinax", "plugins", "plugin-lock.json")) {
		t.Fatalf("plugin validate wrote registry or lock files")
	}
	if strings.Contains(out, root) {
		t.Fatalf("plugin validate leaked local root:\n%s", out)
	}
}

func TestPluginValidateRejectsSecretBearingManifest(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "unsafe-plugin")
	secret := "sk-live-pinax-secret-1234567890"
	writeCLIFixture(t, filepath.Join(pluginDir, "plugin.py"), "print('ok')")
	writeCLIFixture(t, filepath.Join(pluginDir, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
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
    EXAMPLE_TOKEN: `+secret+`
budgets:
  timeout_ms: 3000
  max_input_bytes: 262144
  max_output_bytes: 262144
  max_memory_mb: 64
`)

	out, err := runCLIExpectError("plugin", "validate", pluginDir, "--vault", root, "--json")
	if err == nil {
		t.Fatalf("plugin validate should reject secret manifest:\n%s", out)
	}
	assertJSONErrorCode(t, out, "plugin_manifest_secret_rejected")
	if strings.Contains(out, secret) || strings.Contains(out, root) {
		t.Fatalf("plugin validate leaked secret or local root:\n%s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("error output must remain JSON: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "plugins", "registry.json")); !os.IsNotExist(err) {
		t.Fatalf("plugin validate wrote registry after rejected manifest")
	}
}

func TestPluginInstallRegistryLockInspectAndEnableDisable(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePluginFixture(t, root, "project-dashboard")

	installOut := runCLI(t, "plugin", "install", pluginDir, "--scope", "vault", "--vault", root, "--json")
	assertJSONCommandStatus(t, installOut, "plugin.install", "success")
	installFacts := jsonParseFacts(t, installOut)
	if installFacts["plugin_id"] != "project-dashboard" || installFacts["enabled"] != "false" || installFacts["runtime"] != "wasm" {
		t.Fatalf("install facts = %#v", installFacts)
	}
	registryPath := filepath.Join(root, ".pinax", "plugins", "registry.json")
	lockPath := filepath.Join(root, ".pinax", "plugins", "plugin-lock.json")
	auditPath := filepath.Join(root, ".pinax", "events", "plugin-audit.jsonl")
	for _, path := range []string{registryPath, lockPath, auditPath} {
		if !fileExists(path) {
			t.Fatalf("expected plugin asset missing: %s", path)
		}
	}
	for _, body := range []string{string(readCLITestFile(t, registryPath)), string(readCLITestFile(t, lockPath)), string(readCLITestFile(t, auditPath)), installOut} {
		if strings.Contains(body, root) || strings.Contains(body, "fake wasm bytes") {
			t.Fatalf("plugin install leaked local root or entrypoint bytes:\n%s", body)
		}
	}

	listOut := runCLI(t, "plugin", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listOut, "plugin.list", "success")
	if facts := jsonParseFacts(t, listOut); facts["plugins"] != "1" || facts["enabled"] != "0" {
		t.Fatalf("list facts = %#v", facts)
	}

	inspectOut := runCLI(t, "plugin", "inspect", "project-dashboard", "--vault", root, "--json")
	assertJSONCommandStatus(t, inspectOut, "plugin.inspect", "success")
	if facts := jsonParseFacts(t, inspectOut); facts["enabled"] != "false" || facts["plugin_id"] != "project-dashboard" {
		t.Fatalf("inspect facts = %#v", facts)
	}

	noYesOut, noYesErr := runCLIExpectError("plugin", "enable", "project-dashboard", "--vault", root, "--json")
	if noYesErr == nil {
		t.Fatalf("plugin enable without --yes should fail:\n%s", noYesOut)
	}
	assertJSONErrorCode(t, noYesOut, "approval_required")

	enableOut := runCLI(t, "plugin", "enable", "project-dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, enableOut, "plugin.enable", "success")
	if facts := jsonParseFacts(t, enableOut); facts["enabled"] != "true" {
		t.Fatalf("enable facts = %#v", facts)
	}

	disableOut := runCLI(t, "plugin", "disable", "project-dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, disableOut, "plugin.disable", "success")
	if facts := jsonParseFacts(t, disableOut); facts["enabled"] != "false" {
		t.Fatalf("disable facts = %#v", facts)
	}
}

func TestPluginCommandFamilyOutputContract(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePluginFixture(t, root, "project-dashboard")
	runCLI(t, "plugin", "install", pluginDir, "--scope", "vault", "--vault", root, "--json")

	helpOut := runCLI(t, "plugin", "--help")
	for _, want := range []string{"validate", "install", "list", "inspect", "enable", "disable", "permissions", "doctor", "uninstall", "run"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("plugin help missing %q:\n%s", want, helpOut)
		}
	}

	agentOut := runCLI(t, "plugin", "list", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=plugin.list", "status=success", "fact.plugins=1", "fact.enabled=0"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("plugin list agent missing %q:\n%s", want, agentOut)
		}
	}

	eventsOut := runCLI(t, "plugin", "doctor", "--vault", root, "--events")
	assertNDJSONEvents(t, eventsOut, "plugin.doctor")
	if !strings.Contains(eventsOut, "registry_readable") || strings.Contains(eventsOut, root) {
		t.Fatalf("plugin doctor events invalid or leaked root:\n%s", eventsOut)
	}

	permsOut := runCLI(t, "plugin", "permissions", "list", "project-dashboard", "--vault", root, "--json")
	assertJSONCommandStatus(t, permsOut, "plugin.permissions.list", "success")
	if facts := jsonParseFacts(t, permsOut); facts["grants"] != "0" || facts["plugin_id"] != "project-dashboard" {
		t.Fatalf("permissions facts = %#v", facts)
	}

	runOut, runErr := runCLIExpectError("plugin", "run", "project-dashboard", "render_dashboard", "--vault", root, "--dry-run", "--json")
	if runErr == nil {
		t.Fatalf("disabled plugin run should fail:\n%s", runOut)
	}
	assertJSONErrorCode(t, runOut, "plugin_disabled")

	runCLI(t, "plugin", "enable", "project-dashboard", "--yes", "--vault", root, "--json")
	runOut, runErr = runCLIExpectError("plugin", "run", "project-dashboard", "render_dashboard", "--vault", root, "--dry-run", "--json")
	if runErr == nil {
		t.Fatalf("plugin run without projection.read should fail:\n%s", runOut)
	}
	assertJSONErrorCode(t, runOut, "plugin_permission_denied")

	grantOut := runCLI(t, "plugin", "permissions", "grant", "project-dashboard", "projection.read", "--capability", "render_dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, grantOut, "plugin.permissions.grant", "success")
	if facts := jsonParseFacts(t, grantOut); facts["grants"] != "1" || facts["permission"] != "projection.read" {
		t.Fatalf("grant facts = %#v", facts)
	}
	runOut, runErr = runCLIExpectError("plugin", "run", "project-dashboard", "render_dashboard", "--vault", root, "--dry-run", "--json")
	if runErr == nil {
		t.Fatalf("plugin run without runner should fail after grant:\n%s", runOut)
	}
	assertJSONErrorCode(t, runOut, "plugin_runner_unavailable")
	if strings.Contains(runOut, root) || strings.Contains(runOut, "fake wasm bytes") {
		t.Fatalf("plugin run leaked local root or entrypoint bytes:\n%s", runOut)
	}

	noYesOut, noYesErr := runCLIExpectError("plugin", "uninstall", "project-dashboard", "--vault", root, "--json")
	if noYesErr == nil {
		t.Fatalf("uninstall without --yes should fail:\n%s", noYesOut)
	}
	assertJSONErrorCode(t, noYesOut, "approval_required")

	uninstallOut := runCLI(t, "plugin", "uninstall", "project-dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, uninstallOut, "plugin.uninstall", "success")
	if facts := jsonParseFacts(t, uninstallOut); facts["plugins"] != "0" || facts["plugin_id"] != "project-dashboard" {
		t.Fatalf("uninstall facts = %#v", facts)
	}
}

func TestPluginRunUnavailableContract(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePluginFixture(t, root, "project-dashboard")
	runCLI(t, "plugin", "install", pluginDir, "--scope", "vault", "--vault", root, "--json")
	runCLI(t, "plugin", "enable", "project-dashboard", "--yes", "--vault", root, "--json")
	runCLI(t, "plugin", "permissions", "grant", "project-dashboard", "projection.read", "--capability", "render_dashboard", "--yes", "--vault", root, "--json")
	out, err := runCLIExpectError("plugin", "run", "project-dashboard", "render_dashboard", "--vault", root, "--dry-run", "--json")
	if err == nil {
		t.Fatalf("plugin run without runtime adapter should fail:\n%s", out)
	}
	assertJSONErrorCode(t, out, "plugin_runner_unavailable")
	if strings.Contains(out, root) || strings.Contains(out, "fake wasm bytes") {
		t.Fatalf("plugin run leaked local root or entrypoint bytes:\n%s", out)
	}
}

func TestPluginRunPythonExternalRunnerContract(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePythonPluginFixture(t, root, "py-importer")
	fakeBin := filepath.Join(root, "bin")
	writeCLIFixture(t, filepath.Join(fakeBin, "python3"), "#!/bin/sh\nif [ ! -f \"$1\" ]; then\n  printf 'missing script\\n' >&2\n  exit 42\nfi\ncase \"$1\" in\n  /*) ;;\n  *) printf 'script path is not absolute\\n' >&2; exit 43 ;;\nesac\ncat >/dev/null\nprintf '%s\\n' '{\"schema_version\":\"pinax.plugin.result.v1\",\"status\":\"success\",\"facts\":{\"rows\":\"1\"},\"data\":{\"ok\":true}}'\n")
	if err := os.Chmod(filepath.Join(fakeBin, "python3"), 0o755); err != nil {
		t.Fatalf("chmod fake python: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	runCLI(t, "plugin", "install", pluginDir, "--scope", "vault", "--vault", root, "--json")
	runCLI(t, "plugin", "enable", "py-importer", "--yes", "--vault", root, "--json")
	runCLI(t, "plugin", "permissions", "grant", "py-importer", "projection.read", "--capability", "import_notes", "--yes", "--vault", root, "--json")

	out := runCLI(t, "plugin", "run", "py-importer", "import_notes", "--vault", root, "--dry-run", "--json")
	assertJSONCommandStatus(t, out, "plugin.run", "success")
	facts := jsonParseFacts(t, out)
	if facts["plugin_id"] != "py-importer" || facts["runtime"] != "python" || facts["capability"] != "import_notes" || facts["result_status"] != "success" || facts["write_status"] != "false" || facts["rows"] != "1" {
		t.Fatalf("plugin run facts = %#v", facts)
	}
	registryPath := filepath.Join(root, ".pinax", "plugins", "registry.json")
	lockPath := filepath.Join(root, ".pinax", "plugins", "plugin-lock.json")
	auditPath := filepath.Join(root, ".pinax", "events", "plugin-audit.jsonl")
	for _, body := range []string{out, string(readCLITestFile(t, registryPath)), string(readCLITestFile(t, lockPath)), string(readCLITestFile(t, auditPath))} {
		if strings.Contains(body, root) || strings.Contains(body, "plugin source sentinel") {
			t.Fatalf("plugin run leaked local root or entrypoint body:\n%s", body)
		}
	}
	auditBody := string(readCLITestFile(t, auditPath))
	for _, want := range []string{`"type":"plugin.run"`, `"plugin_id":"py-importer"`, `"capability":"import_notes"`, `"status":"success"`} {
		if !strings.Contains(auditBody, want) {
			t.Fatalf("audit missing %s:\n%s", want, auditBody)
		}
	}
}

func TestPluginPermissionsGrantRevokeAndRunDenyByDefault(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePluginFixture(t, root, "project-dashboard")
	runCLI(t, "plugin", "install", pluginDir, "--scope", "vault", "--vault", root, "--json")
	runCLI(t, "plugin", "enable", "project-dashboard", "--yes", "--vault", root, "--json")

	out, err := runCLIExpectError("plugin", "run", "project-dashboard", "render_dashboard", "--vault", root, "--dry-run", "--json")
	if err == nil {
		t.Fatalf("plugin run without projection.read should fail:\n%s", out)
	}
	assertJSONErrorCode(t, out, "plugin_permission_denied")

	noYesOut, noYesErr := runCLIExpectError("plugin", "permissions", "grant", "project-dashboard", "projection.read", "--capability", "render_dashboard", "--vault", root, "--json")
	if noYesErr == nil {
		t.Fatalf("permission grant without --yes should fail:\n%s", noYesOut)
	}
	assertJSONErrorCode(t, noYesOut, "approval_required")

	grantOut := runCLI(t, "plugin", "permissions", "grant", "project-dashboard", "projection.read", "--capability", "render_dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, grantOut, "plugin.permissions.grant", "success")
	listOut := runCLI(t, "plugin", "permissions", "list", "project-dashboard", "--vault", root, "--json")
	if facts := jsonParseFacts(t, listOut); facts["grants"] != "1" || !strings.Contains(listOut, "projection.read") {
		t.Fatalf("permissions list after grant invalid facts=%#v out=%s", facts, listOut)
	}

	revokeOut := runCLI(t, "plugin", "permissions", "revoke", "project-dashboard", "projection.read", "--capability", "render_dashboard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, revokeOut, "plugin.permissions.revoke", "success")
	if facts := jsonParseFacts(t, revokeOut); facts["grants"] != "0" {
		t.Fatalf("revoke facts = %#v", facts)
	}
}

func writePluginFixture(t *testing.T, root, id string) string {
	t.Helper()
	pluginDir := filepath.Join(root, "plugins", id)
	writeCLIFixture(t, filepath.Join(pluginDir, "dist", "plugin.wasm"), "fake wasm bytes")
	writeCLIFixture(t, filepath.Join(pluginDir, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: `+id+`
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
	return pluginDir
}

func writePythonPluginFixture(t *testing.T, root, id string) string {
	t.Helper()
	pluginDir := filepath.Join(root, "plugins", id)
	writeCLIFixture(t, filepath.Join(pluginDir, "plugin.py"), "# plugin source sentinel\n")
	writeCLIFixture(t, filepath.Join(pluginDir, "pinax-plugin.yaml"), `schema_version: pinax.plugin.v1
id: `+id+`
name: Python Importer
version: 0.1.0
runtime:
  kind: python
  entrypoint: plugin.py
capabilities:
  - id: import_notes
    kind: import.transform
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
	return pluginDir
}

func readCLITestFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}
