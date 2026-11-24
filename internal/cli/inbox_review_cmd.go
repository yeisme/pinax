package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addInboxReviewCommand(parent *cobra.Command, ctx commandBuildContext) {
	var limit int
	var after string
	cmd := &cobra.Command{
		Use: "review", Short: "Preview a small read-only batch of inbox and unclassified index notes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.InboxReview(cmd.Context(), app.InboxReviewRequest{
				VaultPath: *ctx.vaultPath, Limit: limit, After: after,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Batch size, from 1 to 20")
	cmd.Flags().StringVar(&after, "after", "", "Continue after a note reference from the current pending list; omit after changes")
	parent.AddCommand(cmd)
}

// addInboxJudgmentCommand 暴露实验性 inbox 判断入口。默认关闭：配置
// judgment.mode=off 时命令只报告 off，不装配 SDK、不读取凭据、不发起外部
// 调用。--dry-run 只构建确定性投影（零 provider 调用），供启用前核对候选。
func addInboxJudgmentCommand(parent *cobra.Command, ctx commandBuildContext) {
	var noteRef string
	var limit int
	var dryRun bool
	var attemptID string
	var actor string
	var yes bool
	cmd := &cobra.Command{
		Use:   "judgment",
		Short: "Suggest inbox links and duplicates through the experimental judgment switch",
		Long: "Evaluate one pending inbox note against bounded, authorized candidates. " +
			"Suggestions are advisory: the original inbox review flow and every write gate stay authoritative. " +
			"The command is dormant until judgment.enabled=true and judgment.mode=shadow|assist are set explicitly.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 0 || limit > 8 {
				return renderCommandError(cmd, ctx.outputMode(), app.InboxJudgmentCommand, "invalid_limit",
					"Judgment candidate limit must be between 1 and 8",
					"pinax inbox judgment --limit 5 --vault <vault>")
			}
			projection, err := ctx.svc.InboxJudgment(cmd.Context(), app.InboxJudgmentRequest{
				VaultPath: *ctx.vaultPath,
				NoteRef:   noteRef,
				Limit:     limit,
				DryRun:    dryRun,
				AttemptID: attemptID,
				Actor:     actor,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	cmd.Flags().StringVar(&noteRef, "note", "", "Inbox note reference; omit to use the first pending inbox note")
	cmd.Flags().IntVar(&limit, "limit", 5, "Candidate context size, from 1 to 8")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Build the deterministic projection only; zero provider calls and zero credential reads")
	cmd.Flags().StringVar(&attemptID, "attempt", "", "Start an explicit new judgment attempt; omit to reconcile the original attempt identity")
	cmd.Flags().StringVar(&actor, "actor", "", "Owner-local principal for the judgment projection")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the remote provider dispatch when using --api-url")
	_ = cmd.RegisterFlagCompletionFunc("note", noteRefCompletion(func() string { return *ctx.vaultPath }))
	parent.AddCommand(cmd)
	registerRemoteCommand(remoteCommandSpec{
		CommandPath: "inbox judgment",
		Method:      "Pinax.Inbox.Judgment",
		Flags: []remoteParamSpec{
			s("note", "note"), i("limit", "limit"), b("dry_run", "dry-run"),
			s("attempt", "attempt"), s("actor", "actor"), b("yes", "yes"),
		},
	})
}
