package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// recordRun 通过 CLI 记录一次 recorded run，返回 run ID。
func recordRun(t *testing.T, vaultRoot, runtimeID, class string) string {
	t.Helper()
	out := runCLI(t, "continue",
		"--vault", vaultRoot,
		"--scope", "project:pinax",
		"--record-run", "--runtime", runtimeID, "--task-class", class,
		"--json")
	facts := jsonParseFacts(t, out)
	runID, _ := facts["continuity_run_id"].(string)
	if runID == "" {
		t.Fatalf("recorded continue must return continuity_run_id: %#v", facts)
	}
	return runID
}

// TestRecordedContinueWritesReceipt 覆盖 opt-in receipt：一次 recorded run
// 产生 opaque run ID；普通 continue 保持零写入。
func TestRecordedContinueWritesReceipt(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	runID := recordRun(t, vaultRoot, "codex", "implementation_debugging")

	out := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	facts := jsonParseFacts(t, out)
	if facts["total_runs"] != "1" {
		t.Fatalf("report total_runs = %#v, want 1: %s", facts["total_runs"], out)
	}
	if facts["missing_feedback"] != "1" {
		t.Fatalf("missing_feedback = %#v, want 1", facts["missing_feedback"])
	}
	_ = runID
}

// TestContinueReadOnlyDefault 覆盖默认 read-only 语义（即使存在 binding 表
// 迁移路径，普通 continue 也不得写 continuity_runs）。
func TestContinueReadOnlyDefault(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	runCLI(t, "continue", "--vault", vaultRoot, "--scope", "project:pinax", "--json")

	out := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	facts := jsonParseFacts(t, out)
	if facts["total_runs"] != "0" {
		t.Fatalf("plain continue must not write run receipts: %#v", facts)
	}
}

// TestRecordedContinueTaskClassValidation 覆盖 invalid enum 在写入前失败。
func TestRecordedContinueTaskClassValidation(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	if _, err := runCLIExpectError("continue",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--record-run", "--runtime", "codex", "--task-class", "nonsense", "--json"); err == nil {
		t.Fatal("invalid task class must fail")
	}
	if _, err := runCLIExpectError("continue",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--record-run", "--task-class", "implementation_debugging", "--json"); err == nil {
		t.Fatal("--record-run without --runtime must fail")
	}
}

// TestContinueFeedbackCommandContract 覆盖四值 outcome、supersede、稳定错误。
func TestContinueFeedbackCommandContract(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	runID := recordRun(t, vaultRoot, "codex", "product_spec_docs")

	out := runCLI(t, "continue", "feedback", "--vault", vaultRoot,
		"--run", runID, "--outcome", "trusted", "--json")
	facts := jsonParseFacts(t, out)
	if facts["outcome"] != "trusted" {
		t.Fatalf("feedback outcome = %#v", facts)
	}

	// 用户改选：append superseding event。
	out = runCLI(t, "continue", "feedback", "--vault", vaultRoot,
		"--run", runID, "--outcome", "corrected", "--json")
	facts = jsonParseFacts(t, out)
	if facts["supersedes"] == nil || facts["supersedes"] == "" {
		t.Fatalf("resubmission must supersede previous event: %#v", facts)
	}

	// invalid outcome 与缺失 run 都不写。
	if _, err := runCLIExpectError("continue", "feedback", "--vault", vaultRoot,
		"--run", runID, "--outcome", "meh", "--json"); err == nil {
		t.Fatal("invalid outcome must fail")
	}
	if _, err := runCLIExpectError("continue", "feedback", "--vault", vaultRoot,
		"--run", "crun_missing", "--outcome", "trusted", "--json"); err == nil {
		t.Fatal("missing run must fail")
	}
}

// TestContinueFeedbackSupersedeLatestFold 覆盖 report 使用最新有效 outcome，
// 不原地覆盖而保留变更事实。
func TestContinueFeedbackSupersedeLatestFold(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	runID := recordRun(t, vaultRoot, "codex", "release_operations")

	runCLI(t, "continue", "feedback", "--vault", vaultRoot, "--run", runID, "--outcome", "trusted", "--json")
	runCLI(t, "continue", "feedback", "--vault", vaultRoot, "--run", runID, "--outcome", "corrected", "--json")

	out := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("report envelope invalid: %v", err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("report data missing: %s", out)
	}
	outcomeCounts, ok := data["outcome_counts"].(map[string]any)
	if !ok {
		t.Fatalf("outcome_counts missing: %s", out)
	}
	// latest fold：corrected=1，trusted 键不出现（零计数被 omit）。
	if outcomeCounts["corrected"] != float64(1) {
		t.Fatalf("outcome fold must use latest value: %#v", outcomeCounts)
	}
	if _, present := outcomeCounts["trusted"]; present {
		t.Fatalf("superseded trusted outcome must not count: %#v", outcomeCounts)
	}
	if data["completed_loops"] != float64(1) {
		t.Fatalf("completed_loops = %#v, want 1", data["completed_loops"])
	}
}

// TestContinuityReportGoGateFailsOnEmptyWindow 覆盖空窗口：
// go_ready=false，trusted/source 指标为 not_measured，bias 与 routing 固定。
func TestContinuityReportGoGateFailsOnEmptyWindow(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	out := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	facts := jsonParseFacts(t, out)
	if facts["go_ready"] != "false" {
		t.Fatalf("empty window go_ready = %#v, want false", facts["go_ready"])
	}
	if !strings.Contains(facts["trusted_rate"].(string), "not_measured") {
		t.Fatalf("trusted_rate must be not_measured: %#v", facts)
	}
	if !strings.Contains(facts["source_resolvability"].(string), "not_measured") {
		t.Fatalf("source_resolvability must be not_measured: %#v", facts)
	}
	if facts["cross_project_routing"] != "unvalidated" {
		t.Fatalf("cross_project_routing = %#v", facts["cross_project_routing"])
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	data := envelope["data"].(map[string]any)
	bias, ok := data["known_bias"].([]any)
	if !ok || len(bias) != 4 {
		t.Fatalf("known_bias must list 4 biases: %#v", data["known_bias"])
	}
}

// TestContinuityReportKnownBiasAndNotMeasured 覆盖缺 outcome 的 run 不被
// 计入 outcome 分母，而进入 missing_feedback。
func TestContinuityReportKnownBiasAndNotMeasured(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	recordRun(t, vaultRoot, "codex", "implementation_debugging")
	recordRun(t, vaultRoot, "claude-code", "product_spec_docs")

	out := runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["total_runs"] != "2" || facts["missing_feedback"] != "2" {
		t.Fatalf("missing feedback accounting wrong: %#v", facts)
	}
	if !strings.Contains(facts["trusted_rate"].(string), "not_measured") {
		t.Fatalf("no-outcome runs must keep trusted_rate not_measured: %#v", facts["trusted_rate"])
	}
}

// TestWeeklyReviewCommandEvidence 覆盖 weekly review 时长事件（空 inbox 0 秒）。
func TestWeeklyReviewCommandEvidence(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	out := runCLI(t, "continue", "feedback", "--vault", vaultRoot,
		"--weekly-review", "--review-seconds", "0", "--json")
	facts := jsonParseFacts(t, out)
	if facts["event_kind"] != "weekly_review" {
		t.Fatalf("weekly review event = %#v", facts)
	}

	out = runCLI(t, "continue", "report", "--vault", vaultRoot, "--since", "6w", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("envelope invalid: %v", err)
	}
	data := envelope["data"].(map[string]any)
	weeks, ok := data["weekly_reviews"].([]any)
	if !ok || len(weeks) != 1 {
		t.Fatalf("weekly_reviews missing: %s", out)
	}
	week := weeks[0].(map[string]any)
	if week["burden_passed"] != true {
		t.Fatalf("0s weekly review must pass burden gate: %#v", week)
	}

	// 负时长与缺失 flag 必须失败。
	if _, err := runCLIExpectError("continue", "feedback", "--vault", vaultRoot,
		"--weekly-review", "--review-seconds", "-5", "--json"); err == nil {
		t.Fatal("negative review_seconds must fail")
	}
	if _, err := runCLIExpectError("continue", "feedback", "--vault", vaultRoot,
		"--weekly-review", "--json"); err == nil {
		t.Fatal("weekly-review without --review-seconds must fail")
	}
	if _, err := runCLIExpectError("continue", "feedback", "--vault", vaultRoot,
		"--weekly-review", "--review-seconds", "10", "--run", "crun_ignored", "--outcome", "trusted", "--json"); err == nil {
		t.Fatal("weekly-review must reject run/outcome flags instead of silently ignoring them")
	}
}
