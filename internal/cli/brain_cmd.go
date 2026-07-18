package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addBrainCommands(root *cobra.Command, ctx commandBuildContext) {
	var answerLimit int
	var maintainDryRun bool
	var maintainSavePlan bool
	brainCmd := &cobra.Command{Use: "brain", Short: "Preview bounded Agent Brain context and answers"}
	answerCmd := &cobra.Command{Use: "answer <question>", Short: "Preview a citation-first bounded answer", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "brain.answer", "argument_required", "brain answer requires a question", "pinax brain answer <question> --vault <vault> --json")
		}
		projection, err := ctx.svc.BrainAnswerPreview(cmd.Context(), app.BrainAnswerRequest{VaultPath: *ctx.vaultPath, Question: args[0], Limit: answerLimit})
		return ctx.renderProjection(cmd, projection, err)
	}}
	answerCmd.Flags().IntVar(&answerLimit, "limit", 5, "Limit bounded evidence sources")
	maintainCmd := &cobra.Command{Use: "maintain", Short: "Preview Agent Brain maintenance candidates", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.BrainMaintenancePlan(cmd.Context(), app.BrainMaintenanceRequest{VaultPath: *ctx.vaultPath, DryRun: maintainDryRun, SavePlan: maintainSavePlan})
		return ctx.renderProjection(cmd, projection, err)
	}}
	maintainCmd.Flags().BoolVar(&maintainDryRun, "dry-run", false, "Only output the maintenance plan preview; do not write plan evidence")
	maintainCmd.Flags().BoolVar(&maintainSavePlan, "save-plan", false, "Write CLI-authored plan evidence under .pinax/brain-maintenance-plans")
	brainCmd.AddCommand(answerCmd, maintainCmd)
	root.AddCommand(brainCmd)
}
