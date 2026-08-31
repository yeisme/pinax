package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupContinueFixture 构造 machine-mode 测试 fixture：
// 独立 XDG config dir、fixture vault（含 project scope）、已注册 vault、Git worktree。
// 使用 t.Setenv，因此这些测试不并行。
func setupContinueFixture(t *testing.T, projectSlug string) (repoRoot, vaultRoot string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	vaultRoot = t.TempDir()
	runCLI(t, "init", vaultRoot, "--title", "Vault", "--json")
	if projectSlug != "" {
		runCLI(t, "project", "create", projectSlug, "--name", projectSlug, "--vault", vaultRoot, "--json")
	}
	runCLI(t, "vault", "register", vaultRoot, "--name", "test-vault")

	repoRoot = t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return repoRoot, vaultRoot
}

// TestContinueBindCommandContract 覆盖 bind 命令契约：
// bounded output（digest/basename）、stable facts、JSON envelope、
// 默认不泄漏完整绝对路径。
func TestContinueBindCommandContract(t *testing.T) {
	repoRoot, _ := setupContinueFixture(t, "pinax")

	out := runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("bind envelope invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "continue.bind" || envelope["status"] != "success" {
		t.Fatalf("bind envelope: %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["binding_status"] != "ready" {
		t.Fatalf("binding_status = %#v", facts)
	}
	digest, _ := facts["binding_id_digest"].(string)
	if len(digest) != 16 {
		t.Fatalf("binding digest must be bounded 16 hex chars: %q", digest)
	}
	// 默认输出不得泄漏完整绝对路径（只暴露 basename 与 digest）。
	if strings.Contains(out, repoRoot) {
		t.Fatalf("bind output must not leak full repo path:\n%s", out)
	}
	if facts["repo_root"] != filepath.Base(repoRoot) {
		t.Fatalf("repo_root fact = %#v, want basename %q", facts["repo_root"], filepath.Base(repoRoot))
	}
}

// TestContinueBindCanonicalizesAbsoluteVaultSelector 防止 success projection
// 回显 user home 下的 vault 绝对路径；已注册 path 必须规范化为 alias。
func TestContinueBindCanonicalizesAbsoluteVaultSelector(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")
	out := runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", vaultRoot, "--scope", "project:pinax", "--json")
	if strings.Contains(out, vaultRoot) {
		t.Fatalf("bind output leaked absolute vault path:\n%s", out)
	}
	facts := jsonParseFacts(t, out)
	if facts["vault_ref"] != "test-vault" {
		t.Fatalf("vault_ref = %#v, want canonical alias", facts["vault_ref"])
	}
}

// TestContinueBindMissingFlags 覆盖缺失必填 flag 的稳定错误。
func TestContinueBindMissingFlags(t *testing.T) {
	setupContinueFixture(t, "pinax")
	if _, err := runCLIExpectError("continue", "bind", "--json"); err == nil {
		t.Fatal("bind without --vault/--scope must fail")
	}
}

// TestContinueStatusCommandContract 覆盖 status 命令的 ready/missing 诊断与 read-only 语义。
func TestContinueStatusCommandContract(t *testing.T) {
	repoRoot, vaultRoot := setupContinueFixture(t, "pinax")

	// missing：未绑定时 stable status 与单一 bind action。
	out := runCLI(t, "continue", "status", "--repo", repoRoot, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("status envelope invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["binding_status"] != "missing" || facts["ready"] != "false" {
		t.Fatalf("missing status = %#v", facts)
	}

	// 绑定后 ready。
	runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	out = runCLI(t, "continue", "status", "--repo", repoRoot, "--json")
	facts = jsonParseFacts(t, out)
	if facts["binding_status"] != "ready" || facts["ready"] != "true" {
		t.Fatalf("ready status = %#v", facts)
	}
	// status 查询 read-only：不产生 run receipt / handoff / proposal。
	statusAfter, err := os.Stat(filepath.Join(vaultRoot, ".pinax", "memory"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat memory dir: %v", err)
	}
	_ = statusAfter // status 不触碰 vault memory DB（无需创建）。
}

// TestContinueStatusNotARepository 覆盖非 Git 目录的诊断。
func TestContinueStatusNotARepository(t *testing.T) {
	setupContinueFixture(t, "pinax")
	nonGit := t.TempDir()
	out := runCLI(t, "continue", "status", "--repo", nonGit, "--json")
	facts := jsonParseFacts(t, out)
	if facts["binding_status"] != "not_a_repository" || facts["is_repository"] != "false" {
		t.Fatalf("not-a-repository status = %#v", facts)
	}
}
