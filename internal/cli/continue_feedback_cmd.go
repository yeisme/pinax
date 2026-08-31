package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// timeNowUTC 是 report 命令的当前时间注入点（测试可替换）。
var timeNowUTC = func() time.Time { return time.Now().UTC() }

// addContinueFeedbackSubcommand 注册 additive experimental
// `pinax continue feedback`。outcome 只能来自用户四选一提交；
// Agent 不得按任务完成度、语气或工具日志推断。
func addContinueFeedbackSubcommand(parent *cobra.Command, ctx commandBuildContext) {
	var runID, outcome string
	var reviewSeconds int
	var weeklyReview bool

	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Experimental: submit a four-value continuation outcome (user-submitted only)",
		Long: `Submit the user's outcome for a recorded continuity run:
trusted, corrected, wrong_project, or insufficient. Resubmitting appends a
superseding event; the previous event is kept for local audit.

		With --weekly-review (no --run), record bounded weekly review duration instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reviewSecondsSet := cmd.Flags().Changed("review-seconds")
			if weeklyReview {
				if strings.TrimSpace(runID) != "" || strings.TrimSpace(outcome) != "" {
					return renderCommandError(cmd, ctx.outputMode(), "continue.feedback", "validation_failed",
						"--weekly-review cannot be combined with --run or --outcome",
						"pinax continue feedback --weekly-review --review-seconds <non-negative integer>")
				}
				if !reviewSecondsSet || reviewSeconds < 0 {
					return renderCommandError(cmd, ctx.outputMode(), "continue.feedback", "validation_failed",
						"--weekly-review requires --review-seconds <non-negative integer>",
						"pinax continue feedback --weekly-review --review-seconds 0")
				}
			}
			result, err := agentSvc.ContinuityFeedback(cmd.Context(), app.ContinuityFeedbackRequest{
				VaultPath:        agentVaultPath(ctx),
				RunID:            runID,
				Outcome:          outcome,
				ReviewSeconds:    reviewSeconds,
				WeeklyReview:     weeklyReview,
				ReviewSecondsSet: reviewSecondsSet,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.feedback", err)
			}
			proj := domain.NewProjection("continue.feedback", "Continuity feedback recorded.")
			proj.Facts["feedback_id"] = result.FeedbackID
			proj.Facts["event_kind"] = result.EventKind
			if result.Outcome != "" {
				proj.Facts["outcome"] = result.Outcome
			}
			if result.Supersedes != "" {
				proj.Facts["supersedes"] = result.Supersedes
			}
			proj.Facts["experimental"] = "true"
			proj.Data = result
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Recorded continuity run ID")
	cmd.Flags().StringVar(&outcome, "outcome", "", "User outcome: trusted|corrected|wrong_project|insufficient")
	cmd.Flags().IntVar(&reviewSeconds, "review-seconds", 0, "Bounded weekly review duration in seconds (non-negative)")
	cmd.Flags().BoolVar(&weeklyReview, "weekly-review", false, "Record a weekly review duration event (no run required)")
	parent.AddCommand(cmd)
}

// addContinueReportSubcommand 注册 additive experimental
// `pinax continue report`。报告以真实 continuation loop 为分母，
// 明确列出 known bias，Pinax-only 样本固定 cross_project_routing=unvalidated。
func addContinueReportSubcommand(parent *cobra.Command, ctx commandBuildContext) {
	var since string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Experimental: build the local continuity dogfood report for a time window",
		Long: `Aggregate recorded runs, user outcomes, source coverage, runtime/task-class
coverage and weekly review burden into the Go/Iterate/Stop gate report.

Metrics with a zero denominator are reported as not_measured, never as 100%.
The single-repository dogfood cannot validate cross-project routing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := agentSvc.ContinuityReport(cmd.Context(), app.ContinuityReportRequest{
				VaultPath: agentVaultPath(ctx),
				Since:     since,
				Now:       timeNowUTC(),
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.report", err)
			}
			proj := domain.NewProjection("continue.report", "Continuity dogfood report built.")
			proj.Facts["total_runs"] = fmt.Sprintf("%d", report.TotalRuns)
			proj.Facts["completed_loops"] = fmt.Sprintf("%d", report.CompletedLoops)
			proj.Facts["missing_feedback"] = fmt.Sprintf("%d", report.MissingFeedback)
			proj.Facts["trusted_rate"] = reportRateString(report.TrustedRate)
			proj.Facts["source_resolvability"] = reportRateString(report.SourceResolvability)
			proj.Facts["silent_confirmed_write_count"] = fmt.Sprintf("%d", report.SilentConfirmedWriteCount)
			proj.Facts["cross_project_routing"] = report.CrossProjectRouting
			proj.Facts["go_ready"] = fmt.Sprintf("%t", report.GoReady)
			proj.Facts["runtime_coverage"] = strings.Join(report.RuntimeCoverage, ",")
			proj.Facts["experimental"] = "true"
			for _, gate := range report.Gates {
				status := "fail"
				if gate.Passed {
					status = "pass"
				}
				if !gate.Measured {
					status = "not_measured"
				}
				proj.Facts["gate_"+gate.Gate] = fmt.Sprintf("%s (%s; want %s)", status, gate.Current, gate.Threshold)
			}
			proj.Data = report
			return ctx.renderProjection(cmd, proj, nil)
		},
	}
	cmd.Flags().StringVar(&since, "since", "6w", "Time window (e.g. 6w, 30d, 24h)")
	parent.AddCommand(cmd)
}

func reportRateString(rate app.RateValue) string {
	if !rate.Measured {
		return "not_measured"
	}
	return fmt.Sprintf("%.2f (%d/%d)", rate.Ratio, rate.Resolved, rate.Total)
}
