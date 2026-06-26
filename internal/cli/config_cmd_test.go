package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigGetUsesLayeredConfigAndExplicitFlags(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "output:\n  theme: mono\n")
	writeCLITestFile(t, filepath.Join(vault, ".pinax", "config.yaml"), "output:\n  theme: pinax\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_OUTPUT_THEME", "mono")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--vault", vault, "--theme", "high-contrast", "config", "get", "output.theme", "--agent"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute config get: %v\noutput:\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"command=config.get", "status=success", "fact.key=output.theme", "fact.value=high-contrast", "fact.source=flag", "fact.writable=true", "fact.write_scopes=user,project", "pinax config set output.theme <value> --scope user"} {
		if !strings.Contains(got, want) {
			t.Fatalf("config get output missing %q:\n%s", want, got)
		}
	}
}

func TestConfigDoctorExposesSettingsControlProjection(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "output:\n  color: never\n  theme: mono\n")
	writeCLITestFile(t, filepath.Join(vault, ".pinax", "config.yaml"), "output:\n  theme: high-contrast\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_OUTPUT_COLOR", "auto")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--vault", vault, "config", "doctor", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute config doctor: %v\noutput:\n%s", err, out.String())
	}

	var envelope struct {
		Command string         `json:"command"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("config doctor json: %v\n%s", err, out.String())
	}
	if envelope.Command != "config.doctor" {
		t.Fatalf("command = %q", envelope.Command)
	}
	settings, ok := envelope.Data["settings"].([]any)
	if !ok || len(settings) == 0 {
		t.Fatalf("settings projection missing from doctor data: %#v", envelope.Data)
	}
	byKey := map[string]map[string]any{}
	for _, raw := range settings {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("setting item is not object: %#v", raw)
		}
		key, _ := item["key"].(string)
		byKey[key] = item
	}
	if got := byKey["output.theme"]; got["source"] != "project" || got["writable"] != true || got["write_scope"] != "project" {
		t.Fatalf("output.theme setting = %#v", got)
	}
	if got := byKey["output.color"]; got["source"] != "env" || got["writable"] != false || got["write_scope"] != "env" {
		t.Fatalf("output.color setting = %#v", got)
	}
	if got := byKey["output.width"]; got["source"] != "default" || got["writable"] != true || got["next_action"] == "" {
		t.Fatalf("output.width setting = %#v", got)
	}
	diagnostics, ok := envelope.Data["diagnostics"].(map[string]any)
	if !ok || diagnostics["write_mode"] != "local_config_write_requires_scope" || diagnostics["redaction_status"] != "enabled" || diagnostics["token_status"] != "not_inspected" {
		t.Fatalf("diagnostics missing bounded settings status: %#v", envelope.Data["diagnostics"])
	}
	for _, leak := range []string{"Authorization", "Bearer ", "raw-secret", "api-token"} {
		if strings.Contains(out.String(), leak) {
			t.Fatalf("config doctor leaked secret-like output %q:\n%s", leak, out.String())
		}
	}
	if strings.Contains(out.String(), "storage.token") {
		t.Fatalf("config doctor leaked secret-like output:\n%s", out.String())
	}
}

func TestConfigSetAndUnsetProjectScopePreserveOtherFields(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	projectConfig := filepath.Join(vault, ".pinax", "config.yaml")
	writeCLITestFile(t, projectConfig, "output:\n  color: never\n  theme: pinax\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("NO_COLOR", "")

	setCmd := NewRootCommand("test")
	var setOut bytes.Buffer
	setCmd.SetOut(&setOut)
	setCmd.SetArgs([]string{"--vault", vault, "config", "set", "output.theme", "mono", "--scope", "project", "--agent"})
	if err := setCmd.Execute(); err != nil {
		t.Fatalf("config set: %v\n%s", err, setOut.String())
	}

	getCmd := NewRootCommand("test")
	var getOut bytes.Buffer
	getCmd.SetOut(&getOut)
	getCmd.SetArgs([]string{"--vault", vault, "config", "get", "output.theme", "--agent"})
	if err := getCmd.Execute(); err != nil {
		t.Fatalf("config get after set: %v\n%s", err, getOut.String())
	}
	if !strings.Contains(getOut.String(), "fact.value=mono") {
		t.Fatalf("config get after set:\n%s", getOut.String())
	}
	body, err := os.ReadFile(projectConfig)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if !strings.Contains(string(body), "color: never") {
		t.Fatalf("set did not preserve existing color:\n%s", string(body))
	}

	unsetCmd := NewRootCommand("test")
	var unsetOut bytes.Buffer
	unsetCmd.SetOut(&unsetOut)
	unsetCmd.SetArgs([]string{"--vault", vault, "config", "unset", "output.theme", "--scope", "project", "--agent"})
	if err := unsetCmd.Execute(); err != nil {
		t.Fatalf("config unset: %v\n%s", err, unsetOut.String())
	}
	body, err = os.ReadFile(projectConfig)
	if err != nil {
		t.Fatalf("read project config after unset: %v", err)
	}
	if strings.Contains(string(body), "theme:") || !strings.Contains(string(body), "color: never") {
		t.Fatalf("unset did not remove only theme:\n%s", string(body))
	}
}

func TestConfigSetRequiresExplicitScope(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--vault", vault, "config", "set", "output.theme", "mono", "--agent"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("config set without scope succeeded:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "error.code=config_scope_required") {
		t.Fatalf("config set error output:\n%s", out.String())
	}
	for _, path := range []string{filepath.Join(xdg, "pinax", "config.yaml"), filepath.Join(vault, ".pinax", "config.yaml")} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("config file was written unexpectedly at %s", path)
		}
	}
}

func writeCLITestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
