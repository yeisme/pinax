package agentcontinuity

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func basePack() ContinuityPack {
	return ContinuityPack{
		SchemaVersion: ContinuitySchemaVersion,
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"},
		Sections: []ContinuitySection{
			{Kind: "decision", Title: "Key decisions", Items: []string{"d1"}},
		},
		HandoffStatus: HandoffStatusConsumed,
		HandoffID:     "h_1",
		Truncated:     false,
		Experimental:  true,
	}
}

func TestSummaryLinePreservesUTF8WhenObjectiveIsTruncated(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.Objective = strings.Repeat("连续性验收", 30)
	got := SummaryLine(pack)
	if !utf8.ValidString(got) {
		t.Fatalf("summary line is invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("long objective was not truncated: %q", got)
	}
}

// TestResumeCardMissingHandoffContextOnly 覆盖无 handoff：
// context-only partial + handoff_missing warning + checkpoint next action。
func TestResumeCardMissingHandoffContextOnly(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.HandoffStatus = HandoffStatusMissing
	pack.HandoffID = ""
	pack.Freshness = time.Now().UTC()
	pack.RefreshDerived()

	if pack.WarningCodes[0] != WarningHandoffMissing {
		t.Fatalf("warning order: %v", pack.WarningCodes)
	}
	if pack.PackStatus != PackStatusPartial {
		t.Fatalf("pack_status = %q, want partial", pack.PackStatus)
	}
	if pack.RecommendedNextAction == nil || pack.RecommendedNextAction.Name != "Create a continuity checkpoint" {
		t.Fatalf("next action = %#v", pack.RecommendedNextAction)
	}
	// 可信 section 不得因 handoff 缺失被丢弃。
	if pack.SectionCount() == 0 {
		t.Fatal("partial pack must keep trustworthy sections")
	}
}

// TestResumeCardConflictKeepsBothFacts 覆盖冲突决策：两边保留、
// conflict warning 优先于 source warning、next action 指向 review。
func TestResumeCardConflictKeepsBothFacts(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.Conflicts = []agentprotocol.ContextConflict{{MemoryIDs: []string{"m1", "m2"}, Reason: "incompatible"}}
	pack.SourceCoverage = SourceCoverage{Total: 2, Resolved: 1, Missing: 1}
	pack.Freshness = time.Now().UTC()
	pack.RefreshDerived()

	want := []string{WarningDecisionConflict, WarningSourceMissing}
	if len(pack.WarningCodes) != 2 || pack.WarningCodes[0] != want[0] || pack.WarningCodes[1] != want[1] {
		t.Fatalf("warning order = %v, want %v", pack.WarningCodes, want)
	}
	if pack.PackStatus != PackStatusPartial || pack.EvidenceStatus != EvidenceStatusPartial {
		t.Fatalf("status = %q/%q, want partial/partial", pack.PackStatus, pack.EvidenceStatus)
	}
	if pack.RecommendedNextAction.Name != "Resolve memory conflicts" {
		t.Fatalf("conflict must win next action priority: %#v", pack.RecommendedNextAction)
	}
}

// TestResumeCardPartialSourceKeepsTrustworthySections 覆盖部分 source missing：
// partial pack、missing 计数可见、可信内容保留、不猜事实。
func TestResumeCardPartialSourceKeepsTrustworthySections(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.SourceCoverage = SourceCoverage{Total: 3, Resolved: 2, Missing: 1}
	pack.Freshness = time.Now().UTC()
	pack.RefreshDerived()

	if pack.EvidenceStatus != EvidenceStatusPartial {
		t.Fatalf("evidence_status = %q", pack.EvidenceStatus)
	}
	if pack.SourceCoverage.Missing != 1 {
		t.Fatalf("missing count = %d", pack.SourceCoverage.Missing)
	}
	if pack.SectionCount() != 1 {
		t.Fatal("partial pack must keep remaining trustworthy content")
	}
	if pack.RecommendedNextAction.Name != "Inspect source evidence" {
		t.Fatalf("next action = %#v", pack.RecommendedNextAction)
	}
}

// TestResumeCardAmbiguousSourceCounted 覆盖 ambiguous 分桶：
// ref 不能唯一解析计入 ambiguous，整体视为 partial。
func TestResumeCardAmbiguousSourceCounted(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.SourceCoverage = SourceCoverage{Total: 2, Resolved: 1, Ambiguous: 1}
	pack.Freshness = time.Now().UTC()
	pack.RefreshDerived()

	if pack.SourceCoverage.Ambiguous != 1 || pack.EvidenceStatus != EvidenceStatusPartial {
		t.Fatalf("ambiguous not counted as partial: %#v / %q", pack.SourceCoverage, pack.EvidenceStatus)
	}
}

// TestNextActionReviewAttentionPriority 覆盖 review attention 触发 review next action。
func TestNextActionReviewAttentionPriority(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.SourceCoverage = SourceCoverage{Total: 1, Resolved: 1}
	pack.Freshness = time.Now().UTC()
	pack.ReviewAttentionCount = 1
	pack.ReviewAttention = &ReviewAttention{ItemID: "prop_1", Subject: "s", ReasonCodes: []string{"affects_objective"}, Risk: "medium"}
	pack.RefreshDerived()

	if pack.RecommendedNextAction.Name != "Review pending memory item" {
		t.Fatalf("review attention must surface review action: %#v", pack.RecommendedNextAction)
	}
}

// TestNextActionReadyFallsBackToFirstAction 覆盖 ready 路径：
// 无 conflict/无 source 问题 → 回退到第一个 drill-down action。
func TestNextActionReadyFallsBackToFirstAction(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.SourceCoverage = SourceCoverage{Total: 1, Resolved: 1}
	pack.Freshness = time.Now().UTC()
	pack.NextActions = []agentprotocol.NextAction{{Name: "View handoff details", Command: "pinax agent handoff show h_1"}}
	pack.RefreshDerived()

	if pack.PackStatus != PackStatusReady || pack.EvidenceStatus != EvidenceStatusResolved {
		t.Fatalf("ready status wrong: %q/%q", pack.PackStatus, pack.EvidenceStatus)
	}
	if pack.RecommendedNextAction.Name != "View handoff details" {
		t.Fatalf("ready fallback wrong: %#v", pack.RecommendedNextAction)
	}
	if len(pack.WarningCodes) != 0 {
		t.Fatalf("ready pack must have no warnings: %v", pack.WarningCodes)
	}
}

// TestFreshnessStatusBoundary 覆盖 freshness 边界：
// 14 天内 fresh、14 天外 stale、零值 not_measured；生成时刻不参与判断。
func TestFreshnessStatusBoundary(t *testing.T) {
	t.Parallel()
	base := basePack()
	base.SourceCoverage = SourceCoverage{Total: 1, Resolved: 1}

	base.Freshness = time.Time{}
	base.RefreshDerived()
	if base.FreshnessStatus != FreshnessStatusNotMeasured {
		t.Fatalf("zero freshness = %q", base.FreshnessStatus)
	}

	base.Freshness = time.Now().UTC().Add(-13 * 24 * time.Hour)
	base.RefreshDerived()
	if base.FreshnessStatus != FreshnessStatusFresh {
		t.Fatalf("13-day freshness = %q, want fresh", base.FreshnessStatus)
	}

	base.Freshness = time.Now().UTC().Add(-15 * 24 * time.Hour)
	base.RefreshDerived()
	if base.FreshnessStatus != FreshnessStatusStale {
		t.Fatalf("15-day freshness = %q, want stale", base.FreshnessStatus)
	}
}

// TestResumeCardWarningOrderingDeterministic 覆盖 warning 固定排序与 budget 约束：
// conflict/source warning 位于 budget 之外，不被截断逻辑丢弃。
func TestResumeCardWarningOrderingDeterministic(t *testing.T) {
	t.Parallel()
	pack := basePack()
	pack.HandoffStatus = HandoffStatusMissing
	pack.Conflicts = []agentprotocol.ContextConflict{{MemoryIDs: []string{"m1"}}}
	pack.SourceCoverage = SourceCoverage{Total: 3, Resolved: 0, Missing: 1, Stale: 1, Ambiguous: 1}
	pack.RefreshDerived()

	want := []string{
		WarningHandoffMissing, WarningDecisionConflict,
		WarningSourceMissing, WarningSourceStale, WarningSourceAmbiguous,
	}
	if len(pack.WarningCodes) != len(want) {
		t.Fatalf("warnings = %v, want %v", pack.WarningCodes, want)
	}
	for i := range want {
		if pack.WarningCodes[i] != want[i] {
			t.Fatalf("warning[%d] = %q, want %q (full %v)", i, pack.WarningCodes[i], want[i], pack.WarningCodes)
		}
	}
}
