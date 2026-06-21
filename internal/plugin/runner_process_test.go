package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonRunnerUsesStdinEnvelopeAndLimitedEnvironment(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "runner.log")
	callPath := filepath.Join(root, "call.json")
	writePluginTestFile(t, filepath.Join(root, "plugin.py"), "print('fixture')")
	writePluginTestFile(t, filepath.Join(fakeBin, "python3"), "#!/bin/sh\nif [ ! -f \"$1\" ]; then\n  printf 'missing script: %s\\n' \"$1\" >&2\n  exit 42\nfi\ncase \"$1\" in\n  /*) ;;\n  *) printf 'script path is not absolute: %s\\n' \"$1\" >&2; exit 43 ;;\nesac\nprintf 'pwd=%s env=%s args=%s\\n' \"$PWD\" \"$SECRET_TOKEN\" \"$*\" > \"$PINAX_RUNNER_LOG\"\ncat >"+shellQuotePluginTest(callPath)+"\nprintf '%s\\n' '{\"schema_version\":\"pinax.plugin.result.v1\",\"status\":\"success\",\"facts\":{\"rows\":\"1\"},\"data\":{\"ok\":true}}'\n")
	if err := os.Chmod(filepath.Join(fakeBin, "python3"), 0o755); err != nil {
		t.Fatalf("chmod fake python: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PINAX_RUNNER_LOG", logPath)
	t.Setenv("SECRET_TOKEN", "raw-secret")

	manifest := Manifest{ID: "py-plugin", Version: "0.1.0", Runtime: Runtime{Kind: RuntimePython, Entrypoint: "plugin.py"}}
	result, err := ExternalRunner{}.Run(context.Background(), ExternalRunRequest{Manifest: manifest, PluginRoot: root, Capability: "import_notes", Input: map[string]any{"title": "Alpha", "Authorization": "Bearer raw"}, Budgets: RunnerBudgets{TimeoutMS: 1000, MaxInputBytes: 4096, MaxOutputBytes: 4096, MaxMemoryMB: 64}})
	if err != nil {
		t.Fatalf("run python plugin: %v", err)
	}
	if result.Facts["rows"] != "1" {
		t.Fatalf("result = %#v", result)
	}
	logBody := string(readPluginTestFile(t, logPath))
	if strings.Contains(logBody, "raw-secret") || strings.Contains(logBody, "pwd="+root) {
		t.Fatalf("runner log leaked env secret or used plugin root as cwd:\n%s", logBody)
	}
	callBody := string(readPluginTestFile(t, callPath))
	if !strings.Contains(callBody, PluginCallSchema) || strings.Contains(callBody, "Authorization") {
		t.Fatalf("runner call invalid or leaked sensitive input:\n%s", callBody)
	}
}

func TestJavaScriptRunnerMissingExecutableReturnsStableCode(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	manifest := Manifest{ID: "js-plugin", Version: "0.1.0", Runtime: Runtime{Kind: RuntimeJavaScript, Entrypoint: "plugin.js"}}
	_, err := ExternalRunner{}.Run(context.Background(), ExternalRunRequest{Manifest: manifest, PluginRoot: t.TempDir(), Capability: "render", Budgets: RunnerBudgets{TimeoutMS: 1000, MaxInputBytes: 1024, MaxOutputBytes: 1024, MaxMemoryMB: 64}})
	if !strings.Contains(err.Error(), "plugin_runner_unavailable") || RunnerErrorCode(err) != "plugin_runner_unavailable" {
		t.Fatalf("missing js runner err = %v", err)
	}
}

func TestProcessRunnerUsesEntrypointWithoutShellExpansion(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "process.log")
	entrypoint := filepath.Join(root, "plugin process")
	writePluginTestFile(t, entrypoint, "#!/bin/sh\nprintf 'args=%s shell=%s\\n' \"$*\" \"$SHELL\" > \"$PINAX_RUNNER_LOG\"\nprintf '%s\\n' '{\"schema_version\":\"pinax.plugin.result.v1\",\"status\":\"success\",\"facts\":{\"mode\":\"process\"}}'\n")
	if err := os.Chmod(entrypoint, 0o755); err != nil {
		t.Fatalf("chmod process entrypoint: %v", err)
	}
	t.Setenv("PINAX_RUNNER_LOG", logPath)
	t.Setenv("SHELL", "/bin/zsh")
	manifest := Manifest{ID: "proc-plugin", Version: "0.1.0", Runtime: Runtime{Kind: RuntimeProcess, Entrypoint: filepath.Base(entrypoint)}}
	result, err := ExternalRunner{}.Run(context.Background(), ExternalRunRequest{Manifest: manifest, PluginRoot: root, Capability: "diagnose", Budgets: RunnerBudgets{TimeoutMS: 1000, MaxInputBytes: 1024, MaxOutputBytes: 1024, MaxMemoryMB: 64}})
	if err != nil {
		t.Fatalf("run process plugin: %v", err)
	}
	if result.Facts["mode"] != "process" {
		t.Fatalf("process result = %#v", result)
	}
	logBody := string(readPluginTestFile(t, logPath))
	if strings.Contains(logBody, "/bin/zsh") || strings.Contains(logBody, ";") {
		t.Fatalf("process runner used shell-like environment:\n%s", logBody)
	}
}

func readPluginTestFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

func shellQuotePluginTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
