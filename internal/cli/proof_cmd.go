package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

// proof_cmd.go 注册 pinax proof 命令组。proof loop run 是本地 agent 主入口，
// 把 capture/retrieve/diagnose/plan/snapshot/apply 串成一条可调用、可审计的工作流。
func addProofCommands(root *cobra.Command, ctx commandBuildContext) {
	var proofApply bool
	var proofYes bool

	proofCmd := &cobra.Command{
		Use:   "proof",
		Short: "Run the local agent-safe proof loop",
		Long:  "Run the local Capture → Retrieve → Diagnose → Plan → Snapshot → Apply safely proof loop in one command. Defaults to a read-only preview; pass --apply --yes to execute approved repair/organize operations after a fresh snapshot.",
	}
	loopCmd := &cobra.Command{
		Use:   "loop run",
		Short: "Run the full proof loop and emit one projection with a proof_loop_run_id",
		Example: "  pinax proof loop run --vault ./my-notes --json\n" +
			"  pinax proof loop run --vault ./my-notes --apply --yes --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projection, err := ctx.svc.ProofLoopRun(cmd.Context(), app.ProofLoopRunRequest{VaultPath: *ctx.vaultPath, Apply: proofApply, Yes: proofYes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	loopCmd.Flags().BoolVar(&proofApply, "apply", false, "Execute approved repair/organize apply after a fresh snapshot (requires --yes)")
	loopCmd.Flags().BoolVar(&proofYes, "yes", false, "Approve the apply phase")
	proofCmd.AddCommand(loopCmd)
	root.AddCommand(proofCmd)
}
