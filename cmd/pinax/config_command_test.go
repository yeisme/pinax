package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAppearanceAndKeymapContractsCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("NO_COLOR", "")

	vault := filepath.Join(root, "vault")
	runCLI(t, "init", vault, "--title", "Vault", "--json")
	for _, tc := range []struct {
		key   string
		value string
	}{
		{key: "output.theme", value: "high-contrast"},
		{key: "output.color", value: "auto"},
		{key: "output.markdown.style", value: "dark"},
		{key: "themes.custom.accent", value: "cyan"},
	} {
		setOut := runCLI(t, "config", "set", tc.key, tc.value, "--scope", "user", "--vault", vault, "--json")
		assertJSONCommandStatus(t, setOut, "config.set", "success")
		getOut := runCLI(t, "config", "get", tc.key, "--vault", vault, "--json")
		assertJSONCommandStatus(t, getOut, "config.get", "success")
		if !strings.Contains(getOut, `"value":"`+tc.value+`"`) {
			t.Fatalf("config get %s missing value %q:\n%s", tc.key, tc.value, getOut)
		}
	}

	editorOut := runCLI(t, "config", "set", "editor.command", "code --wait", "--scope", "user", "--vault", vault, "--json")
	assertJSONCommandStatus(t, editorOut, "config.set", "success")

	doctorOut := runCLI(t, "config", "doctor", "--vault", vault, "--json")
	var envelope struct {
		Data struct {
			Settings []map[string]any `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(doctorOut), &envelope); err != nil {
		t.Fatalf("config doctor json: %v\n%s", err, doctorOut)
	}
	if len(envelope.Data.Settings) == 0 {
		t.Fatalf("config doctor missing settings: %s", doctorOut)
	}
	if strings.Contains(doctorOut, "pinax keymap") || strings.Contains(doctorOut, "ui.keymap") {
		t.Fatalf("config/keymap contract invented unsupported keymap command or config:\n%s", doctorOut)
	}
	if !strings.Contains(doctorOut, "editor.command") {
		t.Fatalf("config doctor missing supported editor.command setting:\n%s", doctorOut)
	}
}
