package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// gitFixtureOutput 在 fixture 目录运行 git 并返回 stdout。
func gitFixtureOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func mustNowUTC() time.Time { return time.Now().UTC() }

func agentcontinuityFreshnessStaleConst() string { return agentcontinuity.FreshnessStatusStale }

// repoSourceFixture 返回带两个已提交文件的临时 Git worktree。
func repoSourceFixture(t *testing.T) (root, committedFile, committedFileRev string) {
	t.Helper()
	root = initGitWorktreeFixture(t)
	committedFile = "docs/design.md"
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("# design\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitFixture(t, root, "add", "docs/design.md")
	runGitFixture(t, root, "commit", "-m", "add design")
	committedFileRev = gitFixtureOutput(t, root, "rev-parse", "HEAD:docs/design.md")
	return root, committedFile, committedFileRev
}

func runGitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitFixtureOutput(t, dir, args...)
}

// TestRepositorySourceResolvedAndStale 覆盖 resolved 与 revision 漂移。
func TestRepositorySourceResolvedAndStale(t *testing.T) {
	t.Parallel()
	root, rel, rev := repoSourceFixture(t)

	// 已提交文件 + 匹配 revision → resolved。
	res, err := ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: rel, Span: "rev:" + rev,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Status != repoSourceResolved {
		t.Fatalf("status = %s, want resolved", res.Status)
	}
	if res.ObservedAt.IsZero() {
		t.Fatal("resolved source must carry observed time")
	}

	// revision 漂移 → stale。
	res, err = ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: rel, Span: "rev:deadbeefdeadbeef",
	})
	if err != nil {
		t.Fatalf("resolve stale: %v", err)
	}
	if res.Status != repoSourceStale {
		t.Fatalf("status = %s, want stale", res.Status)
	}

	// 无 revision → 只查存在性。
	res, _ = ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: rel,
	})
	if res.Status != repoSourceResolved {
		t.Fatalf("no-revision status = %s, want resolved", res.Status)
	}
}

// TestRepositorySourceMissing 覆盖 deleted file 与不存在的 ref。
func TestRepositorySourceMissing(t *testing.T) {
	t.Parallel()
	root, _, _ := repoSourceFixture(t)
	res, err := ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: "docs/deleted.md",
	})
	if err != nil {
		t.Fatalf("missing must not error: %v", err)
	}
	if res.Status != repoSourceMissing {
		t.Fatalf("status = %s, want missing", res.Status)
	}
}

// TestRepositorySourceBoundaryEscape 覆盖路径边界：`..`、绝对路径、Windows
// 分隔符变体、symlink 逃逸、目录 ref（ambiguous）。这些是 fail-safe 边界，
// 泄漏任何一条都会把 evidence 解析越出绑定 worktree。
func TestRepositorySourceBoundaryEscape(t *testing.T) {
	t.Parallel()
	root, _, _ := repoSourceFixture(t)
	outside := filepath.Dir(root)

	cases := []struct {
		name string
		ref  string
		want repositorySourceStatus
	}{
		{"parent traversal", "../outside.txt", repoSourceMissing},
		{"absolute path", "/etc/passwd", repoSourceMissing},
		{"windows separator", `docs\design.md`, repoSourceMissing},
		{"glob multiple", "docs/*.md", repoSourceResolved}, // 单文件 → resolved
		{"directory ref", "docs", repoSourceAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
				Kind: agentprotocol.SourceKindRepository, Ref: tc.ref,
			})
			if err != nil {
				t.Fatalf("boundary case must not error: %v", err)
			}
			if res.Status != tc.want {
				t.Fatalf("status = %s, want %s", res.Status, tc.want)
			}
		})
	}

	// symlink 逃逸：repo 内 symlink 指向 worktree 外的文件。
	if runtime.GOOS == "windows" {
		t.Skip("symlink requires privileges on windows")
	}
	outsideFile := filepath.Join(outside, "secret-outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	res, err := ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: "escape-link",
	})
	if err != nil {
		t.Fatalf("symlink escape must not error: %v", err)
	}
	if res.Status != repoSourceMissing {
		t.Fatalf("symlink escape must be rejected as missing, got %s", res.Status)
	}

	// glob 也必须走相同 containment 检查，不能因为先 Glob 再 Stat 就把
	// repo 内指向外部的 symlink 当作已解析 evidence。
	res, err = ResolveRepositorySource(context.Background(), root, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: "escape-*",
	})
	if err != nil {
		t.Fatalf("glob symlink escape must not error: %v", err)
	}
	if res.Status != repoSourceMissing {
		t.Fatalf("glob symlink escape must be rejected as missing, got %s", res.Status)
	}
}

// TestRepositorySourceCrossBindingBlocked 证明跨 binding 访问被拒绝：
// root A 中的 ref 无法解析到 root B 的文件（由 containment 保证）。
func TestRepositorySourceCrossBindingBlocked(t *testing.T) {
	t.Parallel()
	rootA, _, _ := repoSourceFixture(t)
	rootB := initGitWorktreeFixture(t)
	if err := os.WriteFile(filepath.Join(rootB, "only-in-b.md"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	res, err := ResolveRepositorySource(context.Background(), rootA, agentprotocol.SourceRef{
		Kind: agentprotocol.SourceKindRepository, Ref: "only-in-b.md",
	})
	if err != nil {
		t.Fatalf("cross-binding must not error: %v", err)
	}
	if res.Status != repoSourceMissing {
		t.Fatalf("cross-binding ref must be missing in root A, got %s", res.Status)
	}
}

// TestSourceCoverageRepositoryKind 覆盖 coverage 扩展：repository source 计入
// resolved/stale/missing/ambiguous 分桶，unknown kind 仍按 unresolved 兼容。
func TestSourceCoverageRepositoryKind(t *testing.T) {
	t.Parallel()
	root, rel, rev := repoSourceFixture(t)

	sources := agentprotocol.SourceRefList{
		{Kind: agentprotocol.SourceKindRepository, Ref: rel, Span: "rev:" + rev},
		{Kind: agentprotocol.SourceKindRepository, Ref: rel, Span: "rev:deadbeef00000000"},
		{Kind: agentprotocol.SourceKindRepository, Ref: "docs/deleted.md"},
		{Kind: agentprotocol.SourceKindRepository, Ref: "docs"},
		{Kind: "openagent_custom_future_kind", Ref: "anything"},
	}
	result := resolveContinuitySourceCoverage(context.Background(), "", sources, root)
	if result.Coverage.Total != 5 {
		t.Fatalf("total = %d, want 5", result.Coverage.Total)
	}
	if result.Coverage.Resolved != 1 || result.Coverage.Stale != 1 || result.Coverage.Missing != 2 || result.Coverage.Ambiguous != 1 {
		t.Fatalf("coverage buckets wrong: %#v", result.Coverage)
	}
	if result.LatestObserved.IsZero() {
		t.Fatal("repository observed time must feed freshness")
	}
}

// TestFreshnessFromEvidenceNotGeneration 覆盖 freshness 与 generated_at 分离：
// 旧 evidence（24 天前）即使刚生成 pack 也必须 stale。
func TestFreshnessFromEvidenceNotGeneration(t *testing.T) {
	t.Parallel()
	vault := t.TempDir()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "freshness"}

	handoffID, err := svc.AgentHandoffCreate(context.Background(), AgentHandoffCreateRequest{
		VaultPath: vault,
		From:      agentPrincipalForTest(),
		To:        agentPrincipalForTest(),
		Scope:     scope,
		Objective: "old handoff",
	})
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	old := mustNowUTC().Add(-24 * 24 * time.Hour)
	// 把 handoff 的 created_at 拨回 24 天前，模拟旧 evidence（GORM update）。
	st, err := svc.storeFor(vault)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := st.DB().Model(&agentmemory.AgentHandoffRow{}).Where("handoff_id = ?", handoffID).Update("created_at", old).Error; err != nil {
		t.Fatalf("backdate handoff: %v", err)
	}

	pack, err := svc.AgentContinuity(context.Background(), ContinuityRequest{
		VaultPath: vault, Principal: agentPrincipalForTest(), Scope: scope,
	})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if pack.FreshnessStatus != agentcontinuityFreshnessStaleConst() {
		t.Fatalf("freshness_status = %q, want stale (old evidence, new pack)", pack.FreshnessStatus)
	}
	if pack.GeneratedAt.IsZero() {
		t.Fatal("generated_at must be set")
	}
	if !pack.Freshness.Equal(old) && pack.Freshness.After(mustNowUTC().Add(-time.Hour)) {
		t.Fatalf("freshness must reflect evidence time, got %v", pack.Freshness)
	}
}
