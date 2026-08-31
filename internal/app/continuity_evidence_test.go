package app

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/continuityevidence"
)

// TestContinuityReportExcludesRowsAfterWindowEnd 固定 report 的窗口为
// [window_start, window_end]；未来/时钟漂移数据不能污染当前决策。
func TestContinuityReportExcludesRowsAfterWindowEnd(t *testing.T) {
	t.Parallel()
	svc, vault, _ := checkpointScopeFixture(t)
	store, err := svc.continuityEvidenceStoreFor(vault)
	if err != nil {
		t.Fatalf("open evidence store: %v", err)
	}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	future := continuityevidence.ContinuityRunRow{
		RunID: "crun_future", ScopeKind: "project", ScopeIDDigest: "future",
		Runtime: "codex", TaskClass: "release_operations", StartedAt: now.Add(time.Hour),
	}
	if err := store.CreateRun(context.Background(), future); err != nil {
		t.Fatalf("create future run: %v", err)
	}
	report, err := svc.ContinuityReport(context.Background(), ContinuityReportRequest{
		VaultPath: vault, Since: "6w", Now: now,
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.TotalRuns != 0 {
		t.Fatalf("future run leaked into report window: %d", report.TotalRuns)
	}
}
