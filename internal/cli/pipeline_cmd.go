package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// pipeline_cmd.go 实现统一管道交互面命令组（pinax-pipeline-unified-ux-v1）：
//
//	pinax pipeline status [--limit N] [--kind organize,metadata,…]
//	pinax pipeline show <plan-id|receipt-id>
//
// 两个命令都严格只读，不写 vault、.pinax/** 或远端。

func addPipelineCommands(root *cobra.Command, ctx commandBuildContext) {
	var limit int
	var kind string

	pipelineCmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Inspect unified plan/apply pipeline status",
		Long:  "Read-only aggregation across plan/apply pipelines: pending saved plans (organize, metadata, repair, restore) with freshness, and recent apply receipts (organize, metadata, repair, restore, sync, publish, proof loop). This command never writes the vault, .pinax assets, or remotes.",
	}
	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "List pending saved plans and recent apply receipts",
		Example: "pinax pipeline status --vault ./my-notes\npinax pipeline status --vault ./my-notes --kind organize,repair --limit 5 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PipelineStatus(cmd.Context(), app.PipelineStatusRequest{VaultPath: *ctx.vaultPath, Limit: limit, Kind: kind})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	statusCmd.Flags().IntVar(&limit, "limit", 10, "Maximum recent receipts to list (1-50)")
	statusCmd.Flags().StringVar(&kind, "kind", "all", "Filter by pipeline kind: all, organize, metadata, repair, restore, sync, publish, proof_loop")
	_ = statusCmd.RegisterFlagCompletionFunc("kind", staticCompletion("kind", "all", "organize", "metadata", "repair", "restore", "sync", "publish", "proof_loop"))
	pipelineCmd.AddCommand(statusCmd)

	showCmd := &cobra.Command{
		Use:               "show <plan-id|receipt-id>",
		Short:             "Show one saved plan or apply receipt",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: pipelineIDCompletion(func() string { return *ctx.vaultPath }),
		Example:           "pinax pipeline show organize-abc123 --vault ./my-notes\npinax pipeline show apply-9f2c1d --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.PipelineShow(cmd.Context(), app.PipelineShowRequest{VaultPath: *ctx.vaultPath, ID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	pipelineCmd.AddCommand(showCmd)

	root.AddCommand(pipelineCmd)
}

// pipelineIDCompletion 补全已保存 plan id 与最近 receipt id（只读）。
func pipelineIDCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items, err := pipelineCompletionItems(cmd.Context(), completionVaultRoot(vaultPathValue()))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func pipelineCompletionItems(ctx context.Context, root string) ([]string, error) {
	projection, err := app.NewService().PipelineStatus(ctx, app.PipelineStatusRequest{VaultPath: root, Limit: 10})
	if err != nil {
		return nil, err
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return []string{}, nil
	}
	items := make([]string, 0)
	if plans, ok := data["plans"].([]domain.PipelinePlanView); ok {
		for _, plan := range plans {
			if id := strings.TrimSpace(plan.PlanID); id != "" {
				items = append(items, id+"\t"+plan.Kind+" plan")
			}
		}
	}
	if receipts, ok := data["receipts"].([]domain.PipelineReceiptView); ok {
		for _, receipt := range receipts {
			if id := strings.TrimSpace(receipt.ReceiptID); id != "" {
				items = append(items, id+"\t"+receipt.Pipeline+" receipt")
			}
		}
	}
	return items, nil
}
