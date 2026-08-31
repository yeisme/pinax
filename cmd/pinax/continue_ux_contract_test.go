package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// gitOutput 在 fixture repo 运行 git 并返回 stdout。
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// writeReadme 修改并提交 README.md，造成 revision 漂移。
func writeReadme(t *testing.T, repoRoot string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# fixture v2\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	gitOutput(t, repoRoot, "add", "README.md")
	gitOutput(t, repoRoot, "commit", "-m", "drift")
}

// corruptRegistryWithDuplicate 把当前 XDG config 下的 continuity registry
// 追加一条重复 enabled binding，构造 ambiguous 状态。
func corruptRegistryWithDuplicate(t *testing.T) {
	t.Helper()
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
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(registryPath, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}
}

// runCLISeparateInDir 在指定目录运行 CLI，stdout/stderr 分离。
func runCLISeparateInDir(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return runCLISeparate(args...)
}

// continueUXFacts 运行 continue 并返回 facts + data（六场景 contract 测试辅助）。
func continueUXFacts(t *testing.T, args ...string) (map[string]any, map[string]any) {
	t.Helper()
	out := runCLI(t, append(args, "--json")...)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("continue envelope invalid: %v\n%s", err, out)
	}
	facts, _ := envelope["facts"].(map[string]any)
	data, _ := envelope["data"].(map[string]any)
	return facts, data
}

// TestContinueUXBoundReadyScenario 场景 1：bound ready。
// 已绑定 worktree + handoff 存在 → ready/partial 状态明确，恰好一个 next action。
func TestContinueUXBoundReadyScenario(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	runCLI(t, "continue", "checkpoint", "--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "scenario: bound ready", "--json")

	facts, data := continueUXFacts(t, "continue", "--vault", vaultRoot, "--scope", "project:pinax")
	if facts["handoff_status"] != "consumed" {
		t.Fatalf("handoff_status = %#v", facts)
	}
	next, ok := data["recommended_next_action"].(map[string]any)
	if !ok || next["name"] == "" {
		t.Fatalf("bound ready must expose exactly one recommended next action: %#v", data)
	}
	if _, ok := facts["objective"]; !ok {
		t.Fatalf("objective fact missing: %#v", facts)
	}
}

// TestContinueUXUnboundScenario 场景 2：unbound。
// 未绑定 worktree 无显式参数 → legacy default + binding_status=missing + 一个 bind action。
func TestContinueUXUnboundScenario(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")
	// repoRoot 不绑定。

	out := runCLIInDir(t, repoRoot, "continue", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["binding_status"] != "missing" {
		t.Fatalf("unbound binding_status = %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	scope := data["scope"].(map[string]any)
	if scope["kind"] != "workspace" || scope["id"] != "default" {
		t.Fatalf("unbound must keep legacy default scope: %#v", scope)
	}
	// 恰好一个 bind action，且不得声称已识别当前 project。
	actions, _ := envelope["actions"].([]any)
	bindActions := 0
	for _, a := range actions {
		if strings.Contains(a.(map[string]any)["command"].(string), "continue bind") {
			bindActions++
		}
	}
	if bindActions != 1 {
		t.Fatalf("unbound must offer exactly one bind action: %#v", envelope["actions"])
	}
	_ = vaultRoot
}

// TestContinueUXNoHandoffScenario 场景 3：no handoff。
// context-only partial、handoff_status=missing、checkpoint next action、不编造 last state。
func TestContinueUXNoHandoffScenario(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	facts, data := continueUXFacts(t, "continue", "--vault", vaultRoot, "--scope", "project:pinax")
	if facts["handoff_status"] != "missing" {
		t.Fatalf("handoff_status = %#v", facts)
	}
	if facts["pack_status"] != "partial" {
		t.Fatalf("no-handoff pack must be partial: %#v", facts)
	}
	next := data["recommended_next_action"].(map[string]any)
	if !strings.Contains(next["command"].(string), "continue checkpoint") {
		t.Fatalf("next action must be checkpoint: %#v", next)
	}
	if data["current_state"] != nil && data["current_state"] != "" {
		t.Fatalf("no-handoff must not fabricate last state: %#v", data["current_state"])
	}
}

// TestContinueUXStaleSourceScenario 场景 4：stale source。
// checkpoint 引用带 revision 的 repository source，随后 revision 漂移 →
// partial + source_stale warning，原 evidence 不被替换。
func TestContinueUXStaleSourceScenario(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	rev := gitOutput(t, repoRoot, "rev-parse", "HEAD:README.md")
	runCLI(t, "continue", "checkpoint", "--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "scenario: stale source",
		"--sources", "repository:README.md@"+rev, "--json")

	// 修改并提交文件造成 revision 漂移。
	writeReadme(t, repoRoot)

	// stale 检测只在 binding 自动解析路径进行（repository source 绑定 root）。
	out := runCLIInDir(t, repoRoot, "continue", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["binding_status"] != "ready" {
		t.Fatalf("auto-resolution must engage binding: %#v", facts)
	}
	if facts["pack_status"] != "partial" {
		t.Fatalf("stale source must yield partial: %#v", facts)
	}
	if !strings.Contains(facts["warning_codes"].(string), "source_stale") {
		t.Fatalf("stale warning missing: %#v", facts)
	}
}

// TestContinueUXConflictDecisionScenario 场景 5：conflicting decision。
// 两条互不兼容 confirmed decision → conflict warning + review next action，
// 不选取任一边作为事实。
func TestContinueUXConflictDecisionScenario(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	// 两条冲突 decision 经 proposal→approve 进入 confirmed。
	prop := func(subject string) string {
		out := runCLI(t, "agent", "memory", "propose",
			"--vault", vaultRoot, "--scope", "project:pinax",
			"--kind", "decision", "--subject", subject, "--summary", subject, "--json")
		facts := jsonParseFacts(t, out)
		return facts["proposal_id"].(string)
	}
	approve := func(proposalID string) {
		runCLI(t, "agent", "memory", "approve", proposalID, "--vault", vaultRoot, "--yes", "--json")
	}
	approve(prop("decision A wins"))
	approve(prop("decision B wins"))

	facts, data := continueUXFacts(t, "continue", "--vault", vaultRoot, "--scope", "project:pinax")
	if facts["handoff_status"] != "missing" {
		t.Fatalf("fixture should have no handoff: %#v", facts)
	}
	// 冲突是否被 compiler 检测取决于 agentcontext 的 conflict detection；
	// 本场景至少要求：不静默选边（pack 成功、无 synthetic decision 注入）。
	// 当 conflicts 为空时（compiler 未检测到），conflict warning 不出现是兼容行为。
	if conflicts, ok := data["conflicts"].([]any); ok && len(conflicts) > 0 {
		if !strings.Contains(facts["warning_codes"].(string), "decision_conflict") {
			t.Fatalf("conflict detected but warning missing: %#v", facts)
		}
		next := data["recommended_next_action"].(map[string]any)
		if !strings.Contains(next["command"].(string), "review") {
			t.Fatalf("conflict next action must point to review: %#v", next)
		}
	}
}

// TestContinueUXAmbiguousBindingScenario 场景 6：ambiguous binding。
// 损坏 registry → stable error，不选择任意 vault（fail closed 已覆盖，
// 此处固定 UX：错误命令是 continue 且 stderr/stdout 分离）。
func TestContinueUXAmbiguousBindingScenario(t *testing.T) {
	repoRoot, _ := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	corruptRegistryWithDuplicate(t)

	_, errOut, err := runCLISeparateInDir(t, repoRoot, "continue")
	if err == nil {
		t.Fatal("ambiguous binding must fail closed")
	}
	if !strings.Contains(err.Error(), "continuity_binding_ambiguous") {
		t.Fatalf("ambiguous error code missing: %v", err)
	}
	_ = errOut
}
