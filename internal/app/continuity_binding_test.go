package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/vaultregistry"
	"gopkg.in/yaml.v3"
)

// setupContinuityBindingFixture 构造 fixture：临时 Git worktree + 已注册 vault
// （含 project scope）。返回 configDir、repo root、vault root。
func setupContinuityBindingFixture(t *testing.T, projectSlug string) (configDir, repoRoot, vaultRoot string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	configDir = t.TempDir()
	vaultRoot = t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(context.Background(), InitVaultRequest{VaultPath: vaultRoot, Title: "Fixture Vault"}); err != nil {
		// InitVault 的 request 形态不确定时回退到直接 init 命令不可行，
		// 这里保持 service 调用；失败即 fixture 损坏。
		t.Fatalf("init fixture vault: %v", err)
	}
	if projectSlug != "" {
		if _, err := svc.CreateProject(context.Background(), ProjectRequest{VaultPath: vaultRoot, Slug: projectSlug, Name: projectSlug}); err != nil {
			t.Fatalf("create fixture project: %v", err)
		}
	}
	if err := vaultregistry.RegisterLocal(vaultregistry.Paths{ConfigDir: configDir}, "fixture-vault", vaultRoot, true); err != nil {
		t.Fatalf("register fixture vault: %v", err)
	}
	repoRoot = initGitWorktreeFixture(t)
	return configDir, repoRoot, vaultRoot
}

func initGitWorktreeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return root
}

// TestContinuityBindingBindStatusReady 覆盖 bind → status ready 主路径。
func TestContinuityBindingBindStatusReady(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, _ := setupContinuityBindingFixture(t, "pinax")
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	result, err := svc.ContinuityBind(context.Background(), ContinuityBindingRequest{
		RepoPath:  repoRoot,
		VaultRef:  "fixture-vault",
		ScopeKind: "project",
		ScopeID:   "pinax",
		ConfigDir: configDir,
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !result.Ready || result.BindingStatus != "ready" {
		t.Fatalf("bind result = %#v", result)
	}
	if result.BindingDigest == "" || len(result.BindingDigest) != 16 {
		t.Fatalf("binding digest must be bounded 16 hex chars: %q", result.BindingDigest)
	}

	status, err := svc.ContinuityBindingStatus(context.Background(), ContinuityBindingStatusRequest{RepoPath: repoRoot, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Ready || status.BindingStatus != continuitybinding.StatusReady {
		t.Fatalf("status = %#v", status)
	}
	if !status.VaultResolved || !status.ScopeValid {
		t.Fatalf("status must resolve vault and scope: %#v", status)
	}
}

// TestContinuityBindingRejectsMissingTargets 覆盖绑定目标不存在：
// 未注册 vault、缺失 project scope、非 Git 目录都必须失败且不写 registry。
func TestContinuityBindingRejectsMissingTargets(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, vaultRoot := setupContinuityBindingFixture(t, "pinax")
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	cases := []struct {
		name string
		req  ContinuityBindingRequest
	}{
		{"unregistered vault", ContinuityBindingRequest{RepoPath: repoRoot, VaultRef: "ghost", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir}},
		{"missing project scope", ContinuityBindingRequest{RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "nope", ConfigDir: configDir}},
		{"unbindable scope kind", ContinuityBindingRequest{RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "session", ScopeID: "s1", ConfigDir: configDir}},
		{"not a git worktree", ContinuityBindingRequest{RepoPath: vaultRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ContinuityBind(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected bind failure")
			}
			se, ok := err.(*agentprotocol.StableError)
			if !ok || se.Code != continuitybinding.ErrCodeBindingInvalid {
				t.Fatalf("bind error code = %#v, want %s", err, continuitybinding.ErrCodeBindingInvalid)
			}
		})
	}

	// 校验失败不得写部分 registry 状态。
	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(registry.Bindings) != 0 {
		t.Fatalf("failed bind must not write partial state: %d bindings", len(registry.Bindings))
	}
}

// TestContinuityResolutionPrecedence 覆盖解析优先级：
// 唯一 enabled exact binding 自动解析；未绑定 → missing；多条 → ambiguous 报错。
func TestContinuityResolutionPrecedence(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, _ := setupContinuityBindingFixture(t, "pinax")
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// missing：未绑定时解析返回 missing（不是错误，允许 legacy default）。
	missing, err := svc.ContinuityResolveBinding(context.Background(), ContinuityResolveRequest{RepoPath: repoRoot, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("missing binding must not error: %v", err)
	}
	if missing.Status != continuitybinding.StatusMissing {
		t.Fatalf("missing status = %s", missing.Status)
	}

	if _, err := svc.ContinuityBind(context.Background(), ContinuityBindingRequest{
		RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	resolved, err := svc.ContinuityResolveBinding(context.Background(), ContinuityResolveRequest{RepoPath: repoRoot, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Status != continuitybinding.StatusReady || resolved.Binding.ScopeID != "pinax" {
		t.Fatalf("resolved = %#v", resolved)
	}

	// ambiguous：直接构造损坏 registry（两条 enabled exact binding）。
	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	dup := registry.Bindings[0]
	dup.BindingID = "bind_duplicate"
	registry.Bindings = append(registry.Bindings, dup)
	// 绕过 SaveRegistry 的唯一性无校验，直接写损坏 YAML（模拟旧版本损坏迁移）。
	data, err := yaml.Marshal(continuitybinding.Registry{
		SchemaVersion: continuitybinding.BindingSchemaVersion,
		Bindings:      registry.Bindings,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(continuitybinding.RegistryPath(configDir), data, 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}
	_, err = svc.ContinuityResolveBinding(context.Background(), ContinuityResolveRequest{RepoPath: repoRoot, ConfigDir: configDir})
	se, ok := err.(*agentprotocol.StableError)
	if !ok || se.Code != continuitybinding.ErrCodeBindingAmbiguous {
		t.Fatalf("ambiguous resolve error = %#v, want %s", err, continuitybinding.ErrCodeBindingAmbiguous)
	}
	status, err := svc.ContinuityBindingStatus(context.Background(), ContinuityBindingStatusRequest{RepoPath: repoRoot, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("ambiguous status must be readable: %v", err)
	}
	if status.BindingStatus != continuitybinding.StatusAmbiguous || status.Ready {
		t.Fatalf("ambiguous status = %#v", status)
	}
}

// TestContinuityBindingDeletedVaultBecomesInvalid 覆盖 vault/scope 在绑定后被删除。
func TestContinuityBindingDeletedVaultBecomesInvalid(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, _ := setupContinuityBindingFixture(t, "pinax")
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	if _, err := svc.ContinuityBind(context.Background(), ContinuityBindingRequest{
		RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	// 删除 vault 后 status 必须 invalid 且 ready=false，解析 fail closed。
	if err := vaultregistry.SaveRegistry(vaultregistry.Paths{ConfigDir: configDir}, vaultregistry.Registry{
		SchemaVersion: vaultregistry.RegistrySchemaVersion,
		Locals:        map[string]vaultregistry.LocalVault{},
	}); err != nil {
		t.Fatalf("wipe vault registry: %v", err)
	}
	status, err := svc.ContinuityBindingStatus(context.Background(), ContinuityBindingStatusRequest{RepoPath: repoRoot, ConfigDir: configDir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.BindingStatus != continuitybinding.StatusInvalid || status.Ready {
		t.Fatalf("deleted vault status = %#v, want invalid/ready=false", status)
	}
	if _, err := svc.ContinuityResolveBinding(context.Background(), ContinuityResolveRequest{RepoPath: repoRoot, ConfigDir: configDir}); err == nil {
		t.Fatal("resolve with deleted vault must fail closed")
	}
}

// TestContinuityBindingStatusReadOnly 证明 status 查询不产生任何写入。
func TestContinuityBindingStatusReadOnly(t *testing.T) {
	t.Parallel()
	configDir, repoRoot, _ := setupContinuityBindingFixture(t, "pinax")
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// 未绑定时 status 不得创建 registry 文件。
	if _, err := svc.ContinuityBindingStatus(context.Background(), ContinuityBindingStatusRequest{RepoPath: repoRoot, ConfigDir: configDir}); err != nil {
		t.Fatalf("status: %v", err)
	}
	if _, err := os.Stat(continuitybinding.RegistryPath(configDir)); !os.IsNotExist(err) {
		t.Fatalf("read-only status must not create registry: %v", err)
	}
	// 绑定后 status 不得改动 registry 的 updated_at。
	if _, err := svc.ContinuityBind(context.Background(), ContinuityBindingRequest{
		RepoPath: repoRoot, VaultRef: "fixture-vault", ScopeKind: "project", ScopeID: "pinax", ConfigDir: configDir,
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	before, _ := continuitybinding.LoadRegistry(configDir)
	time.Sleep(10 * time.Millisecond)
	if _, err := svc.ContinuityBindingStatus(context.Background(), ContinuityBindingStatusRequest{RepoPath: repoRoot, ConfigDir: configDir}); err != nil {
		t.Fatalf("status: %v", err)
	}
	after, _ := continuitybinding.LoadRegistry(configDir)
	if fmt.Sprint(before.Bindings) != fmt.Sprint(after.Bindings) {
		t.Fatalf("read-only status must not mutate registry")
	}
}
