package continuitybinding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func sampleBinding(root string) Binding {
	return Binding{
		SchemaVersion:     BindingSchemaVersion,
		CanonicalRepoRoot: root,
		VaultRef:          "yeisme-notes",
		ScopeKind:         "project",
		ScopeID:           "pinax",
		Enabled:           true,
	}
}

func initGitWorktree(t *testing.T) string {
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

// TestRegistryCreateUpdateDisable 覆盖 create/update/disable/list 的完整生命周期。
func TestRegistryCreateUpdateDisable(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "repo")

	registry, err := LoadRegistry(configDir)
	if err != nil {
		t.Fatalf("load empty registry: %v", err)
	}
	saved := registry.Upsert(sampleBinding(root), now)
	if saved.BindingID != "bind_"+Digest(root) {
		t.Fatalf("binding id = %q, want digest-based id", saved.BindingID)
	}
	if saved.RepoRootDigest != Digest(root) {
		t.Fatalf("digest mismatch: %q vs %q", saved.RepoRootDigest, Digest(root))
	}
	if err := SaveRegistry(configDir, registry); err != nil {
		t.Fatalf("save: %v", err)
	}

	// update：同一 canonical root 重新绑定应替换而非追加。
	reloaded, err := LoadRegistry(configDir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	updated := reloaded.Upsert(Binding{
		CanonicalRepoRoot: root, VaultRef: "other-vault",
		ScopeKind: "workspace", ScopeID: "default", Enabled: true,
	}, now.Add(time.Hour))
	if updated.CreatedAt != now {
		t.Fatalf("update must keep created_at: %v", updated.CreatedAt)
	}
	if len(reloaded.Bindings) != 1 {
		t.Fatalf("rebinding same root must not duplicate: %d rows", len(reloaded.Bindings))
	}
	if err := SaveRegistry(configDir, reloaded); err != nil {
		t.Fatalf("save update: %v", err)
	}

	// disable：rollback 保留数据。
	reloaded, _ = LoadRegistry(configDir)
	if n := reloaded.Disable(root, now.Add(2*time.Hour)); n != 1 {
		t.Fatalf("disable count = %d, want 1", n)
	}
	if got := reloaded.EnabledFor(root); len(got) != 0 {
		t.Fatalf("disabled binding must not resolve: %d", len(got))
	}
	if got := reloaded.AnyFor(root); len(got) != 1 {
		t.Fatalf("disabled binding must remain listed: %d", len(got))
	}
}

// TestRegistryUniqueEnabledExactBinding 证明 Upsert 保证同一 root 唯一 enabled。
func TestRegistryUniqueEnabledExactBinding(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	registry := Registry{}
	rootA := filepath.Join(t.TempDir(), "a")
	rootB := filepath.Join(t.TempDir(), "b")
	registry.Upsert(sampleBinding(rootA), now)
	registry.Upsert(sampleBinding(rootB), now)
	registry.Upsert(sampleBinding(rootA), now.Add(time.Minute))
	if got := registry.EnabledFor(rootA); len(got) != 1 {
		t.Fatalf("expected exactly one enabled binding for rootA, got %d", len(got))
	}
	if len(registry.Bindings) != 2 {
		t.Fatalf("distinct roots must both persist: %d rows", len(registry.Bindings))
	}
}

// TestRegistryUpsertRepairsDuplicateRoot 固定显式 rebind 是损坏 registry 的
// 可恢复路径：同一 canonical root 的重复记录必须收敛为一个 enabled binding。
func TestRegistryUpsertRepairsDuplicateRoot(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	root := filepath.Join(t.TempDir(), "repo")
	registry := Registry{}
	first := registry.Upsert(sampleBinding(root), now)
	duplicate := first
	duplicate.BindingID = "bind_duplicate"
	registry.Bindings = append(registry.Bindings, duplicate)

	registry.Upsert(sampleBinding(root), now.Add(time.Minute))
	if got := registry.EnabledFor(root); len(got) != 1 {
		t.Fatalf("rebind must repair duplicate enabled rows, got %d", len(got))
	}
	if len(registry.Bindings) != 1 {
		t.Fatalf("rebind must collapse duplicate root rows, got %d", len(registry.Bindings))
	}
}

// TestRegistryCorruptFailsClosed 覆盖损坏 registry：解析必须 fail closed，
// 返回 continuity_binding_invalid，不允许悄悄忽略损坏状态。
func TestRegistryCorruptFailsClosed(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	if err := os.WriteFile(RegistryPath(configDir), []byte("bindings: [broken\n"), 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}
	_, err := LoadRegistry(configDir)
	if err == nil {
		t.Fatal("corrupt registry must fail closed")
	}
	if code := stableCode(err); code != ErrCodeBindingInvalid {
		t.Fatalf("corrupt registry code = %q, want %q", code, ErrCodeBindingInvalid)
	}
}

// TestRegistrySchemaVersionMismatchFailsClosed 覆盖未知 schema 版本。
func TestRegistrySchemaVersionMismatchFailsClosed(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	if err := os.WriteFile(RegistryPath(configDir), []byte("schema_version: pinax.continuity_binding.v0\nbindings: []\n"), 0o600); err != nil {
		t.Fatalf("write versioned registry: %v", err)
	}
	_, err := LoadRegistry(configDir)
	if code := stableCode(err); code != ErrCodeBindingInvalid {
		t.Fatalf("unknown schema code = %q, want %q", code, ErrCodeBindingInvalid)
	}
}

// TestRegistryOwnerOnlyPermissions 验证 registry 文件权限为 0600（owner-only）。
func TestRegistryOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("owner-only bits are POSIX semantics")
	}
	configDir := t.TempDir()
	registry := Registry{}
	registry.Upsert(sampleBinding(filepath.Join(t.TempDir(), "repo")), time.Now().UTC())
	if err := SaveRegistry(configDir, registry); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(RegistryPath(configDir))
	if err != nil {
		t.Fatalf("stat registry: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("registry perm = %o, want 0600", perm)
	}
}

// TestBindingValidateRejectsURLAndEmptyFields 覆盖校验边界：
// remote URL/userinfo 形态、空字段、未知 scope kind 都必须被拒绝。
func TestBindingValidateRejectsURLAndEmptyFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*Binding)
		wantErr bool
	}{
		{"valid", func(b *Binding) {}, false},
		{"missing id", func(b *Binding) { b.BindingID = "" }, true},
		{"wrong schema", func(b *Binding) { b.SchemaVersion = "v0" }, true},
		{"missing root", func(b *Binding) { b.CanonicalRepoRoot = "" }, true},
		{"missing digest", func(b *Binding) { b.RepoRootDigest = "" }, true},
		{"missing vault", func(b *Binding) { b.VaultRef = "" }, true},
		{"unknown scope kind", func(b *Binding) { b.ScopeKind = "org" }, true},
		{"missing scope id", func(b *Binding) { b.ScopeID = "" }, true},
		{"url-like root", func(b *Binding) { b.CanonicalRepoRoot = "ssh://git@example.com/repo" }, true},
		{"scp-like root", func(b *Binding) { b.CanonicalRepoRoot = "git@example.com:org/repo" }, true},
		{"digest mismatch", func(b *Binding) { b.RepoRootDigest = "deadbeef" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "repo")
			b := sampleBinding(root)
			b.BindingID = "bind_x"
			b.RepoRootDigest = Digest(root)
			tc.mutate(&b)
			err := b.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

// TestCanonicalizeRepoRootSymlink 证明 symlink 路径与真实路径命中同一 key。
func TestCanonicalizeRepoRootSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink requires privileges on windows")
	}
	real := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	gotReal, err := CanonicalizeRepoRoot(real)
	if err != nil {
		t.Fatalf("canonicalize real: %v", err)
	}
	gotLink, err := CanonicalizeRepoRoot(link)
	if err != nil {
		t.Fatalf("canonicalize link: %v", err)
	}
	if gotReal != gotLink {
		t.Fatalf("symlink must canonicalize to same key: %q vs %q", gotLink, gotReal)
	}
}

// TestDetectWorktreeRoot 覆盖 Git worktree 检测与非 Git 目录边界。
func TestDetectWorktreeRoot(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := initGitWorktree(t)
	root, err := DetectWorktreeRoot(context.Background(), repo)
	if err != nil {
		t.Fatalf("detect worktree: %v", err)
	}
	canonical, _ := CanonicalizeRepoRoot(repo)
	if root != canonical {
		t.Fatalf("worktree root = %q, want %q", root, canonical)
	}
	// 子目录也必须解析到同一 worktree root（Git 语义）。
	sub := filepath.Join(repo, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rootSub, err := DetectWorktreeRoot(context.Background(), sub)
	if err != nil {
		t.Fatalf("detect from subdir: %v", err)
	}
	if rootSub != canonical {
		t.Fatalf("subdir root = %q, want %q", rootSub, canonical)
	}
	// 非 Git 目录返回错误（not_a_repository 语义由 app 层映射）。
	if _, err := DetectWorktreeRoot(context.Background(), t.TempDir()); err == nil {
		t.Fatal("non-git dir must fail detection")
	}
}

// stableCode 是测试 helper：从 error 提取稳定 code。
// StableError.Error() 格式固定为 "code: message"。
func stableCode(err error) string {
	// 回退：StableError.Error() 格式是 "code: message"。
	msg := err.Error()
	for i := 0; i < len(msg); i++ {
		if msg[i] == ':' {
			return msg[:i]
		}
	}
	return ""
}
