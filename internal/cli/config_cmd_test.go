package cli

import (
	"bytes"
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
	for _, want := range []string{"command=config.get", "status=success", "fact.key=output.theme", "fact.value=high-contrast"} {
		if !strings.Contains(got, want) {
			t.Fatalf("config get output missing %q:\n%s", want, got)
		}
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
