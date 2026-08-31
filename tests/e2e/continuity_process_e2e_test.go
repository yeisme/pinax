package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentadapter"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

// continuityRepoFixture 构造带已提交文件的临时 Git worktree（process e2e）。
func continuityRepoFixture(t *testing.T) (root, fileRel, fileRev string) {
	t.Helper()
	root = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "test")
	fileRel = "openspec/changes/pinax-x/proposal.md"
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(fileRel)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, fileRel), []byte("# proposal\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	fileRev = strings.TrimSpace(run("rev-parse", "HEAD:"+fileRel))
	return root, fileRel, fileRev
}

// gitRun 在 fixture repo 运行 git 并返回 stdout（e2e 辅助）。
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

// TestContinuitySourceProcessE2E 覆盖 repository source 全场景：
// resolvable ref、deleted file、drifted revision、symlink escape、
// directory ambiguous、无 binding。一个 missing source 不得丢弃全部价值。
func TestContinuitySourceProcessE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("process e2e requires git")
	}
	t.Parallel()
	ctx := context.Background()
	repoRoot, fileRel, fileRev := continuityRepoFixture(t)
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"}
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// symlink escape fixture（repo 内 symlink 指向 worktree 外）。
	if err := os.WriteFile(filepath.Join(filepath.Dir(repoRoot), "outside-secret.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(filepath.Join(filepath.Dir(repoRoot), "outside-secret.txt"), filepath.Join(repoRoot, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// drift fixture：第二个已提交文件，随后修改并提交以造成 revision 漂移。
	driftRel := "docs/drifted.md"
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, driftRel), []byte("# v1\n"), 0o644); err != nil {
		t.Fatalf("write drift: %v", err)
	}
	gitRun(t, repoRoot, "add", driftRel)
	gitRun(t, repoRoot, "commit", "-m", "add drifted")
	driftRev := strings.TrimSpace(gitRun(t, repoRoot, "rev-parse", "HEAD:docs/drifted.md"))
	if err := os.WriteFile(filepath.Join(repoRoot, driftRel), []byte("# v2\n"), 0o644); err != nil {
		t.Fatalf("drift write: %v", err)
	}
	gitRun(t, repoRoot, "add", driftRel)
	gitRun(t, repoRoot, "commit", "-m", "drift")

	principal := agentadapter.CodexPrincipal("codex-e2e")
	handoffID, err := svc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath:    vault,
		From:         principal,
		To:           agentadapter.OrdoPrincipal("ordo-e2e"),
		Scope:        scope,
		Objective:    "source coverage e2e",
		CurrentState: "handoff with mixed repository sources",
		Decisions:    []string{"keep repository evidence bounded"},
		Blockers:     []string{"one source deleted upstream"},
		Verification: []string{"go test ./internal/app"},
		Sources: agentprotocol.SourceRefList{
			{Kind: agentprotocol.SourceKindRepository, Ref: fileRel, Span: "rev:" + fileRev},
			{Kind: agentprotocol.SourceKindRepository, Ref: driftRel, Span: "rev:" + driftRev},
			{Kind: agentprotocol.SourceKindRepository, Ref: "openspec/changes/pinax-x/deleted.md"},
			{Kind: agentprotocol.SourceKindRepository, Ref: "escape"},
			{Kind: agentprotocol.SourceKindRepository, Ref: "openspec/changes/pinax-x"},
		},
	})
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}

	// 绑定路径：RepoRoot 参与解析。
	pack, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
		VaultPath: vault,
		Principal: principal,
		Scope:     scope,
		RepoRoot:  repoRoot,
		HandoffID: handoffID,
	})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	cov := pack.SourceCoverage
	if cov.Total != 5 {
		t.Fatalf("total = %d, want 5", cov.Total)
	}
	if cov.Resolved != 1 || cov.Stale != 1 || cov.Missing != 2 || cov.Ambiguous != 1 {
		t.Fatalf("coverage buckets = %#v, want 1/1/2/1", cov)
	}
	// partial trustworthy：可信 section 保留，next action 指向 inspect。
	if pack.PackStatus != agentcontinuity.PackStatusPartial {
		t.Fatalf("pack_status = %q", pack.PackStatus)
	}
	if pack.EvidenceStatus != agentcontinuity.EvidenceStatusPartial {
		t.Fatalf("evidence_status = %q", pack.EvidenceStatus)
	}
	if pack.SectionCount() == 0 {
		t.Fatal("partial pack must keep trustworthy sections")
	}
	if pack.RecommendedNextAction == nil || pack.RecommendedNextAction.Name != "Inspect source evidence" {
		t.Fatalf("next action = %#v", pack.RecommendedNextAction)
	}

	// 无 binding：repository source 全部 missing，不访问任意路径。
	noBinding, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
		VaultPath: vault,
		Principal: principal,
		Scope:     scope,
		HandoffID: handoffID,
	})
	if err != nil {
		t.Fatalf("continue no binding: %v", err)
	}
	if noBinding.SourceCoverage.Resolved != 0 || noBinding.SourceCoverage.Total != 5 {
		t.Fatalf("no-binding coverage = %#v", noBinding.SourceCoverage)
	}
}

// TestContinuityCrossRuntimeCheckpointResumeE2E 覆盖 4.3：
// Codex checkpoint → Claude resume，以及反向流程；core schema 无
// runtime-specific required field，confirmed memory count 不变。
func TestContinuityCrossRuntimeCheckpointResumeE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("process e2e requires git")
	}
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"}
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	codex := agentadapter.CodexPrincipal("codex-e2e")
	claude := agentadapter.OrdoPrincipal("claude-e2e")

	sourceRev := "abc123def456"
	sources := agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindRepository, Ref: "docs/design.md", Span: "rev:" + sourceRev}}

	// 方向 1：Codex checkpoint → Claude resume。
	checkpoint, err := svc.ContinuityCheckpoint(ctx, app.ContinuityCheckpointRequest{
		VaultPath:    vault,
		Principal:    codex,
		Scope:        scope,
		Objective:    "cross-runtime handoff continuity",
		CurrentState: "codex finished the binding slice",
		Decisions:    []string{"binding registry is user-level"},
		Blockers:     []string{"none"},
		Verification: []string{"go test ./internal/continuitybinding"},
		FollowUps:    []string{"claude resumes with recorded continue"},
		Sources:      sources,
		DurableCandidates: []app.DurableCandidate{
			{Kind: "decision", Subject: "cross-runtime schema stays provider-neutral", Summary: "runtime names never enter memory identity"},
		},
		ToRuntime: "claude-code",
	})
	if err != nil {
		t.Fatalf("codex checkpoint: %v", err)
	}
	if checkpoint.ConfirmedCreated != 0 {
		t.Fatalf("checkpoint must not confirm memory: %d", checkpoint.ConfirmedCreated)
	}

	resume, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
		VaultPath: vault, Principal: claude, Scope: scope, HandoffID: checkpoint.HandoffID,
	})
	if err != nil {
		t.Fatalf("claude resume: %v", err)
	}
	if resume.Objective != "cross-runtime handoff continuity" {
		t.Fatalf("objective lost across runtimes: %q", resume.Objective)
	}
	if resume.CurrentState != "codex finished the binding slice" {
		t.Fatalf("current state lost: %q", resume.CurrentState)
	}
	if resume.HandoffStatus != agentcontinuity.HandoffStatusExplicit {
		t.Fatalf("handoff status = %s", resume.HandoffStatus)
	}
	// verification/follow-up 语义保留在 sections。
	found := map[string]bool{}
	for _, section := range resume.Sections {
		for _, item := range section.Items {
			if item == "go test ./internal/continuitybinding" {
				found["verification"] = true
			}
			if item == "claude resumes with recorded continue" {
				found["follow_up"] = true
			}
		}
	}
	if !found["verification"] || !found["follow_up"] {
		t.Fatalf("verification/follow-up sections missing: %#v", resume.Sections)
	}

	// 方向 2：Claude checkpoint → Codex resume。
	reverse, err := svc.ContinuityCheckpoint(ctx, app.ContinuityCheckpointRequest{
		VaultPath:    vault,
		Principal:    claude,
		Scope:        scope,
		Objective:    "reverse direction checkpoint",
		CurrentState: "claude verified resume semantics",
		Decisions:    []string{"same provider-neutral handoff contract"},
		Sources:      sources,
		ToRuntime:    "codex",
	})
	if err != nil {
		t.Fatalf("claude checkpoint: %v", err)
	}
	resumeReverse, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
		VaultPath: vault, Principal: codex, Scope: scope, HandoffID: reverse.HandoffID,
	})
	if err != nil {
		t.Fatalf("codex resume: %v", err)
	}
	if resumeReverse.Objective != "reverse direction checkpoint" {
		t.Fatalf("reverse objective lost: %q", resumeReverse.Objective)
	}

	// confirmed memory count 不变：两次 checkpoint 只产生 proposal（proposed 态
	// 存在 proposal 表而非 memory 表，用 ListProposals 核对）。
	status, err := svc.AgentMemoryStatus(ctx, vault, scope)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status[agentprotocol.LifecycleConfirmed] != 0 {
		t.Fatalf("confirmed memory created by checkpoint: %d", status[agentprotocol.LifecycleConfirmed])
	}
	proposals, err := svc.AgentMemoryListProposals(ctx, vault, scope)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected exactly one pending proposal: %d", len(proposals))
	}
}

// TestContinuityThreeTaskClassDogfoodE2E 覆盖 7.2 三类 task 流程骨架：
// resume → work fixture → checkpoint → 另一 runtime resume → feedback → report。
// 每类任务 source coverage 被测量，silent confirmed writes=0。
func TestContinuityThreeTaskClassDogfoodE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("process e2e requires git")
	}
	t.Parallel()
	ctx := context.Background()
	repoRoot, fileRel, fileRev := continuityRepoFixture(t)
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"}
	svc := app.NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	codex := agentadapter.CodexPrincipal("codex-dogfood")
	claude := agentadapter.OrdoPrincipal("claude-dogfood")

	classes := []string{"implementation_debugging", "product_spec_docs", "release_operations"}
	runtimes := [][2]string{{"codex", "claude-code"}, {"claude-code", "codex"}, {"codex", "claude-code"}}

	for i, class := range classes {
		from, to := runtimes[i][0], runtimes[i][1]
		// resume（显式 handoff 模拟上次 closeout）。
		resume, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
			VaultPath: vault, Principal: codex, Scope: scope,
			RepoRoot: repoRoot, Task: fmt.Sprintf("task class %s", class),
		})
		if err != nil {
			t.Fatalf("[%s] resume: %v", class, err)
		}
		_ = resume

		// work fixture：checkpoint 记录可继续状态。
		checkpoint, err := svc.ContinuityCheckpoint(ctx, app.ContinuityCheckpointRequest{
			VaultPath:    vault,
			Principal:    codex,
			Scope:        scope,
			Objective:    fmt.Sprintf("%s objective", class),
			CurrentState: fmt.Sprintf("%s work completed", class),
			Decisions:    []string{fmt.Sprintf("%s decision", class)},
			Blockers:     []string{},
			Verification: []string{"go test ./..."},
			FollowUps:    []string{fmt.Sprintf("%s follow-up", class)},
			Sources:      agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindRepository, Ref: fileRel, Span: "rev:" + fileRev}},
			ToRuntime:    to,
		})
		if err != nil {
			t.Fatalf("[%s] checkpoint: %v", class, err)
		}

		// 另一 runtime resume。
		other, err := svc.AgentContinuity(ctx, app.ContinuityRequest{
			VaultPath: vault, Principal: claude, Scope: scope, HandoffID: checkpoint.HandoffID, RepoRoot: repoRoot,
		})
		if err != nil {
			t.Fatalf("[%s] other runtime resume: %v", class, err)
		}
		if other.Objective != fmt.Sprintf("%s objective", class) {
			t.Fatalf("[%s] objective lost", class)
		}

		// recorded run + feedback。
		runID, err := svc.ContinuityRecordRun(ctx, app.ContinuityRunRecord{
			VaultPath:      vault,
			Scope:          scope,
			Runtime:        from,
			TaskClass:      class,
			HandoffStatus:  string(other.HandoffStatus),
			SourceTotal:    other.SourceCoverage.Total,
			SourceResolved: other.SourceCoverage.Resolved,
			SourceStale:    other.SourceCoverage.Stale,
			SourceMissing:  other.SourceCoverage.Missing,
			WarningCodes:   other.WarningCodes,
		})
		if err != nil {
			t.Fatalf("[%s] record run: %v", class, err)
		}
		if _, err := svc.ContinuityFeedback(ctx, app.ContinuityFeedbackRequest{
			VaultPath: vault, RunID: runID, Outcome: "trusted",
		}); err != nil {
			t.Fatalf("[%s] feedback: %v", class, err)
		}
	}

	report, err := svc.ContinuityReport(ctx, app.ContinuityReportRequest{
		VaultPath: vault, Since: "6w", Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	// 三类 task class 都有分母，coverage 被测量，silent writes=0。
	if len(report.ClassCoverage) != 3 {
		t.Fatalf("task class coverage = %d, want 3", len(report.ClassCoverage))
	}
	if !report.SourceResolvability.Measured {
		t.Fatal("source coverage must be measured")
	}
	if report.SilentConfirmedWriteCount != 0 {
		t.Fatalf("silent confirmed writes = %d, want 0", report.SilentConfirmedWriteCount)
	}
	if report.CrossProjectRouting != "unvalidated" {
		t.Fatalf("routing claim = %q", report.CrossProjectRouting)
	}
	// 3 个 loop 远小于 30 样本门槛：go_ready=false，样本门槛失败。
	if report.GoReady {
		t.Fatal("3 loops must not pass the sample gate")
	}
}
