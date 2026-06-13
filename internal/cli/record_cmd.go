package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addRecordCommands(root *cobra.Command, ctx commandBuildContext) {
	var recordAdoptPlan bool
	recordCmd := &cobra.Command{Use: "record", Short: "Manage the vault record ledger"}
	recordCmd.AddCommand(&cobra.Command{Use: "init", Short: "Initialize the record ledger", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordInit(cmd.Context(), recordRequest(ctx, ""))
		return ctx.renderProjection(cmd, projection, err)
	}})
	recordCmd.AddCommand(&cobra.Command{Use: "status", Short: "Show record ledger status", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordStatus(cmd.Context(), recordRequest(ctx, ""))
		return ctx.renderProjection(cmd, projection, err)
	}})
	recordAdoptCmd := &cobra.Command{Use: "adopt [query]", Short: "Register existing Markdown notes in the record ledger", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		projection, err := ctx.svc.RecordAdopt(cmd.Context(), app.RecordRequest{VaultPath: *ctx.vaultPath, NoteRef: query, Plan: recordAdoptPlan})
		return ctx.renderProjection(cmd, projection, err)
	}}
	recordAdoptCmd.Flags().BoolVar(&recordAdoptPlan, "plan", false, "Only output the adoption plan; do not write the record ledger")
	recordCmd.AddCommand(recordAdoptCmd)
	recordCmd.AddCommand(&cobra.Command{Use: "history <query>", Short: "Show the current history summary for one record by note ref", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordHistory(cmd.Context(), recordRequest(ctx, args[0]))
		return ctx.renderProjection(cmd, projection, err)
	}})
	root.AddCommand(recordCmd)
}

func recordRequest(ctx commandBuildContext, noteRef string) app.RecordRequest {
	return app.RecordRequest{VaultPath: *ctx.vaultPath, NoteRef: noteRef}
}
