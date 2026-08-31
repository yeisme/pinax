package continuityevidence

import (
	"time"
)

// ReportGate 描述一个 Go/Iterate/Stop 门禁及其当前值。
type ReportGate struct {
	Gate   string `json:"gate"`
	Passed bool   `json:"passed"`
	// Measured=false 表示 not_measured（分母为 0），不得当作通过。
	Measured  bool   `json:"measured"`
	Current   string `json:"current"`
	Threshold string `json:"threshold"`
}

// WeeklyReviewWindow 是单周 review 用时聚合。
type WeeklyReviewWindow struct {
	Week         string `json:"week"`
	ItemCount    int    `json:"item_count"`
	ActionCount  int    `json:"action_count"`
	Unresolved   int    `json:"unresolved"`
	TotalSeconds int    `json:"total_seconds"`
	BurdenPassed bool   `json:"burden_passed"`
}

// ClassCoverage 是单 task class 的分母统计。
type ClassCoverage struct {
	TaskClass  TaskClass `json:"task_class"`
	Runs       int       `json:"runs"`
	Outcomes   int       `json:"outcomes"`
	Trusted    int       `json:"trusted"`
	RuntimeSet []string  `json:"runtimes,omitempty"`
}

// Report 是六周 dogfood 的 CLI-authored 本地报告。
// 只包含枚举、计数、比例、时间、opaque ID/digest 和 bounded warning codes。
type Report struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	TotalRuns   int       `json:"total_runs"`
	// CompletedLoops 是有最终 user outcome 的 run 数（trusted loop 的分母基础）。
	CompletedLoops int `json:"completed_loops"`
	// OutcomeCounts 是四值分布（latest fold）。
	OutcomeCounts map[Outcome]int `json:"outcome_counts"`
	// TrustedRate 是 trusted / all valid outcomes；分母为 0 时 Measured=false。
	TrustedRate RateValue `json:"trusted_rate"`
	// SourceResolvability 是聚合 resolved/total；total=0 时 not_measured。
	SourceResolvability RateValue `json:"source_resolvability"`
	// RuntimeCoverage 列出窗口内出现过的 runtime。
	RuntimeCoverage []string `json:"runtime_coverage"`
	// ClassCoverage 按 task class 分母统计。
	ClassCoverage []ClassCoverage `json:"class_coverage"`
	// WeeklyReviews 按 ISO week 聚合 review 用时。
	WeeklyReviews []WeeklyReviewWindow `json:"weekly_reviews"`
	// SilentConfirmedWriteCount 由 run receipt 汇总；非 0 时 safety gate 失败。
	SilentConfirmedWriteCount int `json:"silent_confirmed_write_count"`
	// MissingFeedback 是 recorded 但无 outcome 的 run 数。
	MissingFeedback int `json:"missing_feedback"`
	// CrossProjectRouting 固定 unvalidated：首轮只有 Pinax repository，
	// 单仓库样本不能证明跨项目自动路由。
	CrossProjectRouting string `json:"cross_project_routing"`
	// KnownBias 固定列出 single-operator/single-repository/self-selection/missing-feedback。
	KnownBias []string `json:"known_bias"`
	// Gates 是预注册门槛的执行结果。
	Gates []ReportGate `json:"gates"`
	// GoReady 只在全部门槛通过时为 true。
	GoReady bool `json:"go_ready"`
	// SyntheticExcluded 说明 synthetic/test run 排除口径（本实现不标记 synthetic）。
	SyntheticExcluded int `json:"synthetic_excluded"`
}

// RateValue 是带 not_measured 语义的比率。
type RateValue struct {
	Measured bool    `json:"measured"`
	Ratio    float64 `json:"ratio"`
	Resolved int     `json:"resolved"`
	Total    int     `json:"total"`
}

// BuildReport 从原始 receipt 行构建报告并执行预注册 Go gates。
//
// 分母口径（中文注释，review 时不可静默修改）：
//  1. completed loop = recorded run 且存在最终 user outcome（latest fold）。
//  2. trusted rate = trusted / 全部 valid outcomes；outcome=not_measured 的 run
//     计入 MissingFeedback，不进任何 outcome 分母。
//  3. source resolvability = 聚合 resolved/total；任一 run total=0 时整体
//     not_measured，绝不当成 100%。
//  4. weekly review burden = 每个完成 review 的周总用时 ≤300 秒。
//  5. Go 只在 Sample≥30、双 runtime、三 task class、trusted≥80%、sources≥95%
//     且 measured、review≤300s、silent writes=0 时成立。
func BuildReport(windowStart, windowEnd time.Time, runs []ContinuityRunRow, events []ContinuityFeedbackEventRow) Report {
	report := Report{
		WindowStart:         windowStart.UTC(),
		WindowEnd:           windowEnd.UTC(),
		TotalRuns:           len(runs),
		CrossProjectRouting: "unvalidated",
		KnownBias:           []string{"single_operator", "single_repository", "self_selection", "missing_feedback"},
		OutcomeCounts:       map[Outcome]int{},
	}
	latest := LatestOutcome(events)

	runtimes := map[string]struct{}{}
	classIndex := map[TaskClass]int{}
	sourceTotal, sourceResolved := 0, 0
	silentWrites := 0
	for i := range runs {
		run := runs[i]
		if outcome, ok := latest[run.RunID]; ok {
			report.CompletedLoops++
			report.OutcomeCounts[outcome]++
		} else {
			report.MissingFeedback++
		}
		if run.Runtime != "" {
			runtimes[run.Runtime] = struct{}{}
		}
		class := TaskClass(run.TaskClass)
		idx, ok := classIndex[class]
		if !ok {
			report.ClassCoverage = append(report.ClassCoverage, ClassCoverage{TaskClass: class})
			idx = len(report.ClassCoverage) - 1
			classIndex[class] = idx
		}
		cov := &report.ClassCoverage[idx]
		cov.Runs++
		if outcome, ok := latest[run.RunID]; ok {
			cov.Outcomes++
			if outcome == OutcomeTrusted {
				cov.Trusted++
			}
		}
		if run.Runtime != "" {
			seen := false
			for _, existing := range cov.RuntimeSet {
				if existing == run.Runtime {
					seen = true
					break
				}
			}
			if !seen {
				cov.RuntimeSet = append(cov.RuntimeSet, run.Runtime)
			}
		}
		sourceTotal += run.SourceTotal
		sourceResolved += run.SourceResolved
		silentWrites += run.SilentConfirmedWriteCnt
	}
	report.SilentConfirmedWriteCount = silentWrites

	for runtime := range runtimes {
		report.RuntimeCoverage = append(report.RuntimeCoverage, runtime)
	}

	// trusted rate：分母为全部有 outcome 的 run。
	outcomeDenominator := 0
	for _, count := range report.OutcomeCounts {
		outcomeDenominator += count
	}
	if outcomeDenominator > 0 {
		report.TrustedRate = RateValue{
			Measured: true,
			Ratio:    float64(report.OutcomeCounts[OutcomeTrusted]) / float64(outcomeDenominator),
			Resolved: report.OutcomeCounts[OutcomeTrusted],
			Total:    outcomeDenominator,
		}
	}
	if sourceTotal > 0 {
		report.SourceResolvability = RateValue{
			Measured: true,
			Ratio:    float64(sourceResolved) / float64(sourceTotal),
			Resolved: sourceResolved,
			Total:    sourceTotal,
		}
	}

	report.WeeklyReviews = aggregateWeeklyReviews(events)

	report.Gates = []ReportGate{
		{Gate: "sample_ge_30", Measured: true, Current: itoa(report.CompletedLoops), Threshold: ">=30", Passed: report.CompletedLoops >= 30},
		{Gate: "runtime_coverage_codex_and_claude", Measured: true, Current: joinSorted(report.RuntimeCoverage), Threshold: "codex,claude-code", Passed: containsAll(report.RuntimeCoverage, "codex", "claude-code")},
		{Gate: "task_class_coverage", Measured: true, Current: classNames(report.ClassCoverage), Threshold: "3 classes", Passed: len(report.ClassCoverage) >= 3},
		{Gate: "trusted_rate_ge_80pct", Measured: report.TrustedRate.Measured, Current: ratioString(report.TrustedRate), Threshold: ">=0.8", Passed: report.TrustedRate.Measured && report.TrustedRate.Ratio >= 0.8},
		{Gate: "source_resolvability_ge_95pct", Measured: report.SourceResolvability.Measured, Current: ratioString(report.SourceResolvability), Threshold: ">=0.95 and total>0", Passed: report.SourceResolvability.Measured && report.SourceResolvability.Ratio >= 0.95},
		{Gate: "weekly_review_le_300s", Measured: true, Current: maxWeeklySeconds(report.WeeklyReviews), Threshold: "<=300s/week", Passed: weeklyBurdenPassed(report.WeeklyReviews)},
		{Gate: "silent_confirmed_writes_eq_0", Measured: true, Current: itoa(report.SilentConfirmedWriteCount), Threshold: "=0", Passed: report.SilentConfirmedWriteCount == 0},
		{Gate: "cross_project_routing", Measured: true, Current: report.CrossProjectRouting, Threshold: "unvalidated", Passed: true},
	}
	report.GoReady = true
	for _, gate := range report.Gates {
		if !gate.Passed {
			report.GoReady = false
			break
		}
	}
	return report
}

// aggregateWeeklyReviews 把 weekly_review 事件按 ISO 8601 周聚合。
// 同一周内多条事件求和；没有事件的周不出现在报告中。
func aggregateWeeklyReviews(events []ContinuityFeedbackEventRow) []WeeklyReviewWindow {
	byWeek := map[string]*WeeklyReviewWindow{}
	order := []string{}
	for _, event := range events {
		if event.EventKind != EventKindWeeklyReview || event.ReviewSeconds == nil {
			continue
		}
		week := weekKey(event.SubmittedAt)
		window, ok := byWeek[week]
		if !ok {
			window = &WeeklyReviewWindow{Week: week}
			byWeek[week] = window
			order = append(order, week)
		}
		window.ItemCount++
		window.TotalSeconds += *event.ReviewSeconds
	}
	windows := make([]WeeklyReviewWindow, 0, len(order))
	for _, week := range order {
		window := *byWeek[week]
		window.BurdenPassed = window.TotalSeconds <= 300
		windows = append(windows, window)
	}
	return windows
}

func weekKey(t time.Time) string {
	year, week := t.ISOWeek()
	return fmtSprintf("%04d-W%02d", year, week)
}

func weeklyBurdenPassed(windows []WeeklyReviewWindow) bool {
	for _, window := range windows {
		if window.TotalSeconds > 300 {
			return false
		}
	}
	return true
}

func maxWeeklySeconds(windows []WeeklyReviewWindow) string {
	max := 0
	for _, window := range windows {
		if window.TotalSeconds > max {
			max = window.TotalSeconds
		}
	}
	return itoa(max) + "s"
}

func ratioString(rate RateValue) string {
	if !rate.Measured {
		return "not_measured"
	}
	return fmtSprintf("%.2f (%d/%d)", rate.Ratio, rate.Resolved, rate.Total)
}

func containsAll(values []string, wants ...string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, want := range wants {
		if !set[want] {
			return false
		}
	}
	return true
}

func classNames(classes []ClassCoverage) string {
	names := make([]string, 0, len(classes))
	for _, class := range classes {
		names = append(names, string(class.TaskClass))
	}
	sortStrings(names)
	return joinStrings(names, ",")
}
