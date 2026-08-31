package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestContinueBindingAutoResolution 覆盖唯一 enabled exact binding 自动解析：
// 在已绑定 worktree 中不带 --vault/--scope 运行 continue，scope 自动落到
// binding 的 project:pinax，并带 additive binding facts。
func TestContinueBindingAutoResolution(t *testing.T) {
	repoRoot, _ := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")

	out := runCLIInDir(t, repoRoot, "continue", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("auto-resolution envelope invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["binding_status"] != "ready" {
		t.Fatalf("binding_status = %#v, want ready", facts)
	}
	if facts["binding_id_digest"] == nil {
		t.Fatalf("binding digest fact missing: %#v", facts)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %#v", envelope)
	}
	scope, ok := data["scope"].(map[string]any)
	if !ok || scope["kind"] != "project" || scope["id"] != "pinax" {
		t.Fatalf("auto-resolved scope = %#v, want project:pinax", data["scope"])
	}
}

// TestExplicitFlagsOverrideBinding 覆盖显式参数优先：已绑定 worktree 中显式
// --vault/--scope 时完全走旧路径，binding 不改写调用者指定值。
func TestExplicitFlagsOverrideBinding(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")

	out := runCLIInDir(t, repoRoot, "continue",
		"--vault", vaultRoot, "--scope", "workspace:default", "--json")
	facts := jsonParseFacts(t, out)
	// 显式参数路径不产生 binding facts。
	if _, ok := facts["binding_status"]; ok {
		t.Fatalf("explicit flags must bypass binding facts: %#v", facts)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	data := envelope["data"].(map[string]any)
	scope := data["scope"].(map[string]any)
	if scope["kind"] != "workspace" || scope["id"] != "default" {
		t.Fatalf("explicit scope overwritten by binding: %#v", scope)
	}
}

// TestContinueBindingAmbiguousFailsClosed 覆盖损坏 registry（多条 enabled
// exact binding）时 continue fail closed，不选择任意 vault。
func TestContinueBindingAmbiguousFailsClosed(t *testing.T) {
	repoRoot, _ := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")

	// 构造损坏 registry：同一 canonical root 两条 enabled binding。
	configDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pinax")
	registryPath := filepath.Join(configDir, "continuity_bindings.yaml")
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var parsed struct {
		SchemaVersion string           `yaml:"schema_version"`
		Bindings      []map[string]any `yaml:"bindings"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse registry: %v", err)
	}
	if len(parsed.Bindings) != 1 {
		t.Fatalf("expected one binding before corruption: %d", len(parsed.Bindings))
	}
	dup := map[string]any{}
	for key, value := range parsed.Bindings[0] {
		dup[key] = value
	}
	dup["binding_id"] = "bind_dup_dupdupdupdup"
	parsed.Bindings = append(parsed.Bindings, dup)
	corrupt, err := yaml.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal corrupt registry: %v", err)
	}
	if err := os.WriteFile(registryPath, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}

	_, err = runCLIExpectErrorFromDir(t, repoRoot, "continue", "--json")
	if err == nil {
		t.Fatal("ambiguous binding must fail closed")
	}
	if !strings.Contains(err.Error(), "continuity_binding_ambiguous") {
		t.Fatalf("ambiguous error code missing: %v", err)
	}
}

// runCLIExpectErrorFromDir 在指定目录运行 CLI 并返回错误（允许失败）。
func runCLIExpectErrorFromDir(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return runCLIExpectError(args...)
}
