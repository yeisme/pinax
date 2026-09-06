package continuityevidence

import (
	"testing"
	"time"
)

func reportFixtureRuns(baseID, runtime, class string, startedAt time.Time, trusted bool) ([]ContinuityRunRow, []ContinuityFeedbackEventRow) {
	runs := make([]ContinuityRunRow, 0, 1)
	events := []ContinuityFeedbackEventRow{}
	for i := 0; i < 1; i++ {
		runID := baseID
		runs = append(runs, sampleRun(runID, runtime, class, startedAt.Add(time.Duration(i)*time.Hour)))
		if trusted {
			events = append(events, ContinuityFeedbackEventRow{
				FeedbackID:  fmtSprintf("fb_%d", i),
				RunID:       runID,
				EventKind:   EventKindOutcome,
				Outcome:     string(OutcomeTrusted),
				SubmittedAt: startedAt.Add(time.Duration(i)*time.Hour + time.Minute),
			})
		}
	}
	return runs, events
}

// TestGoGateAllPass 覆盖全部 Go 门槛达成的 happy path：
// ≥30 loops、双 runtime、三 task class、trusted ≥80%、sources ≥95%、
// weekly review ≤300s、silent writes=0。
func TestGoGateAllPass(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)
	var runs []ContinuityRunRow
	var events []ContinuityFeedbackEventRow
	// 30 个 completed loops：两个 runtime 各 15，覆盖三类 task class。
	classes := []string{"implementation_debugging", "product_spec_docs", "release_operations"}
	for i := 0; i < 30; i++ {
		runtime := "codex"
		if i%2 == 1 {
			runtime = "claude-code"
		}
		class := classes[i%3]
		r, e := reportFixtureRuns(fmtSprintf("run_a_%d", i), runtime, class, start.Add(time.Duration(i)*12*time.Hour), true)
		runs = append(runs, r...)
		events = append(events, e...)
	}
	zero := 120
	events = append(events, ContinuityFeedbackEventRow{
		FeedbackID: "fb_week", EventKind: EventKindWeeklyReview,
		ReviewSeconds: &zero, SubmittedAt: start,
	})

	report := BuildReport(start, end, runs, events)
	if !report.GoReady {
		for _, gate := range report.Gates {
			if !gate.Passed {
				t.Logf("failed gate: %s (current=%s, measured=%t)", gate.Gate, gate.Current, gate.Measured)
			}
		}
		t.Fatalf("all-pass fixture must yield go_ready=true")
	}
	// Pinax-only 样本必须固定 unvalidated。
	if report.CrossProjectRouting != "unvalidated" {
		t.Fatalf("cross_project_routing = %q", report.CrossProjectRouting)
	}
	if len(report.KnownBias) != 4 {
		t.Fatalf("known_bias = %v", report.KnownBias)
	}
}

// TestGoGateThresholdBoundary 覆盖 trusted rate 80% 边界与分母定义。
func TestGoGateThresholdBoundary(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)

	// 40 个 loop：32 trusted + 8 corrected = 80% 恰好达到边界。
	var runs []ContinuityRunRow
	var events []ContinuityFeedbackEventRow
	classes := []string{"implementation_debugging", "product_spec_docs", "release_operations"}
	for i := 0; i < 40; i++ {
		runtime := "codex"
		if i%2 == 1 {
			runtime = "claude-code"
		}
		trusted := i < 32
		r, e := reportFixtureRuns(fmtSprintf("run_b_%d", i), runtime, classes[i%3], start.Add(time.Duration(i)*6*time.Hour), trusted)
		runs = append(runs, r...)
		events = append(events, e...)
		if !trusted {
			events = append(events, ContinuityFeedbackEventRow{
				FeedbackID: fmtSprintf("fb_corr_%d", i), RunID: r[0].RunID,
				EventKind: EventKindOutcome, Outcome: string(OutcomeCorrected),
				SubmittedAt: r[0].StartedAt.Add(time.Minute),
			})
		}
	}
	report := BuildReport(start, end, runs, events)
	if !report.TrustedRate.Measured {
		t.Fatal("trusted rate must be measured")
	}
	if diff := report.TrustedRate.Ratio - 0.8; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("trusted rate = %f, want exactly 0.8", report.TrustedRate.Ratio)
	}
	if !report.Gates[3].Passed {
		t.Fatalf("80%% boundary must pass trusted gate: %#v", report.Gates[3])
	}

	// 一个 trusted loop 被用户改选为 insufficient：39 分母、31 trusted → 77.5% < 80%。
	events = append(events, ContinuityFeedbackEventRow{
		FeedbackID: "fb_supersede_31", RunID: "run_b_31",
		EventKind: EventKindOutcome, Outcome: string(OutcomeInsufficient),
		SubmittedAt:          runs[31].StartedAt.Add(2 * time.Minute),
		SupersedesFeedbackID: "fb_31",
	})
	report = BuildReport(start, end, runs, events)
	if report.Gates[3].Passed {
		t.Fatalf("79%% must fail trusted gate: %#v", report.Gates[3])
	}
	if report.GoReady {
		t.Fatal("go_ready must fail when trusted gate fails")
	}
}

// TestGoGateZeroDenominatorNotMeasured 覆盖 total=0 的 source 与无 outcome 分母：
// 指标必须 not_measured，不得按 100% 处理。
func TestGoGateZeroDenominatorNotMeasured(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)

	report := BuildReport(start, end, nil, nil)
	if report.TrustedRate.Measured || report.SourceResolvability.Measured {
		t.Fatalf("zero denominators must be not_measured: %#v / %#v", report.TrustedRate, report.SourceResolvability)
	}
	if report.GoReady {
		t.Fatal("empty window must not be go_ready")
	}

	// source total=0 的 run：source 指标 not_measured。
	runs, events := reportFixtureRuns("run_c_0", "codex", "implementation_debugging", start, true)
	runsB, eventsB := reportFixtureRuns("run_c_1", "codex", "implementation_debugging", start.Add(time.Hour), true)
	runs = append(runs, runsB...)
	events = append(events, eventsB...)
	runs[0].SourceTotal = 0
	runs[0].SourceResolved = 0
	runs[1].SourceTotal = 0
	runs[1].SourceResolved = 0
	report = BuildReport(start, end, runs, events)
	if report.SourceResolvability.Measured {
		t.Fatalf("total=0 source denominator must be not_measured: %#v", report.SourceResolvability)
	}
	if report.Gates[4].Passed {
		t.Fatal("not_measured source gate must fail")
	}
}

// TestGoGateMissingFeedbackExcluded 覆盖无 outcome 的 run 不进 outcome 分母。
func TestGoGateMissingFeedbackExcluded(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)

	runs, events := reportFixtureRuns("run_d_0", "codex", "implementation_debugging", start, true)
	runs2, events2 := reportFixtureRuns("run_d_1", "codex", "implementation_debugging", start.Add(time.Hour), true)
	runs3, events3 := reportFixtureRuns("run_d_2", "codex", "implementation_debugging", start.Add(2*time.Hour), true)
	runs = append(runs, runs2...)
	runs = append(runs, runs3...)
	events = append(events, events2...)
	events = append(events, events3...)
	runs = append(runs, sampleRun("run_no_fb", "codex", "implementation_debugging", start))
	report := BuildReport(start, end, runs, events)
	if report.MissingFeedback != 1 {
		t.Fatalf("missing_feedback = %d, want 1", report.MissingFeedback)
	}
	if report.TrustedRate.Total != 3 {
		t.Fatalf("outcome denominator = %d, want 3 (no-feedback run excluded)", report.TrustedRate.Total)
	}
}

// TestGoGateSilentWriteFailsSafety 覆盖 silent confirmed write 使 safety gate 失败。
func TestGoGateSilentWriteFailsSafety(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)
	runs, events := reportFixtureRuns("run_e_0", "codex", "implementation_debugging", start, true)
	runs[0].SilentConfirmedWriteCnt = 1
	report := BuildReport(start, end, runs, events)
	if report.SilentConfirmedWriteCount != 1 || report.Gates[6].Passed {
		t.Fatalf("silent write gate must fail: %#v", report.Gates[6])
	}
}

// TestWeeklyBurdenGateBoundary 覆盖每周 review 300s 边界。
func TestWeeklyBurdenGateBoundary(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	over := 301
	ok := 300
	events := []ContinuityFeedbackEventRow{
		{FeedbackID: "w1", EventKind: EventKindWeeklyReview, ReviewSeconds: &ok, SubmittedAt: start},
		{FeedbackID: "w2", EventKind: EventKindWeeklyReview, ReviewSeconds: &over, SubmittedAt: start.Add(7 * 24 * time.Hour)},
	}
	report := BuildReport(start.Add(-time.Hour), start.Add(8*24*time.Hour), nil, events)
	if len(report.WeeklyReviews) != 2 {
		t.Fatalf("weekly aggregation wrong: %#v", report.WeeklyReviews)
	}
	if !report.WeeklyReviews[0].BurdenPassed || report.WeeklyReviews[1].BurdenPassed {
		t.Fatalf("burden boundary wrong: %#v", report.WeeklyReviews)
	}
	if report.Gates[5].Passed {
		t.Fatal("weekly burden gate must fail when a week exceeds 300s")
	}
}

func findReportGate(t *testing.T, report Report, gate string) ReportGate {
	t.Helper()
	for _, g := range report.Gates {
		if g.Gate == gate {
			return g
		}
	}
	t.Fatalf("gate %q not found", gate)
	return ReportGate{}
}

// TestWeeklyBurdenGateNotVacuouslyPassedWithoutReviews 覆盖零 weekly review：
// 负担未知，Measured=false、Passed=false，不得当作 0s 通过。
func TestWeeklyBurdenGateNotVacuouslyPassedWithoutReviews(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)
	var runs []ContinuityRunRow
	var events []ContinuityFeedbackEventRow
	classes := []string{"implementation_debugging", "product_spec_docs", "release_operations"}
	for i := 0; i < 30; i++ {
		runtime := "codex"
		if i%2 == 1 {
			runtime = "claude-code"
		}
		r, e := reportFixtureRuns(fmtSprintf("run_w_%d", i), runtime, classes[i%3], start.Add(time.Duration(i)*12*time.Hour), true)
		runs = append(runs, r...)
		events = append(events, e...)
	}
	// 不提交任何 weekly review 事件。
	report := BuildReport(start, end, runs, events)
	gate := findReportGate(t, report, "weekly_review_le_300s")
	if gate.Measured || gate.Passed {
		t.Fatalf("zero weekly reviews must not vacuously pass: %#v", gate)
	}
	if gate.Current != "no weekly reviews" {
		t.Fatalf("current = %q", gate.Current)
	}
	if report.GoReady {
		t.Fatal("go_ready must be false when weekly burden is unmeasured")
	}
}

// TestSourceResolvabilityAnyZeroSourceRunNotMeasured 覆盖预注册口径：
// 任一 run total=0 时整体 not_measured，即使其余 run 全部 resolved。
func TestSourceResolvabilityAnyZeroSourceRunNotMeasured(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)
	var runs []ContinuityRunRow
	var events []ContinuityFeedbackEventRow
	for i := 0; i < 10; i++ {
		r, e := reportFixtureRuns(fmtSprintf("run_s_%d", i), "codex", "implementation_debugging", start.Add(time.Duration(i)*time.Hour), true)
		runs = append(runs, r...)
		events = append(events, e...)
	}
	runs = append(runs, sampleRun("run_s_zero", "codex", "implementation_debugging", start.Add(11*time.Hour)))
	runs[len(runs)-1].SourceTotal = 0
	runs[len(runs)-1].SourceResolved = 0

	report := BuildReport(start, end, runs, events)
	if report.SourceResolvability.Measured {
		t.Fatalf("any run with total=0 must make the metric not_measured: %#v", report.SourceResolvability)
	}
	gate := findReportGate(t, report, "source_resolvability_ge_95pct")
	if gate.Measured || gate.Passed {
		t.Fatalf("source gate must fail when metric is not_measured: %#v", gate)
	}
}

// TestRuntimeCoverageSortedDeterministic 覆盖 runtime_coverage 输出排序稳定。
func TestRuntimeCoverageSortedDeterministic(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * 7 * 24 * time.Hour)
	runs := []ContinuityRunRow{
		sampleRun("run_r_1", "codex", "implementation_debugging", start),
		sampleRun("run_r_2", "claude-code", "implementation_debugging", start.Add(time.Hour)),
	}
	for i := 0; i < 10; i++ {
		report := BuildReport(start, end, runs, nil)
		if len(report.RuntimeCoverage) != 2 || report.RuntimeCoverage[0] != "claude-code" || report.RuntimeCoverage[1] != "codex" {
			t.Fatalf("runtime coverage must be sorted deterministically: %#v", report.RuntimeCoverage)
		}
	}
}
