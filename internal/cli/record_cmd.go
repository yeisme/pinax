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
	var identityPlanSave bool
	identityCmd := &cobra.Command{Use: "identity", Short: "Audit and migrate durable vault object identities"}
	identityCmd.AddCommand(&cobra.Command{Use: "audit", Short: "Audit note identities without writing vault state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordIdentityAudit(cmd.Context(), app.IdentityMigrationRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	identityPlanCmd := &cobra.Command{Use: "plan", Short: "Generate an identity migration plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordIdentityPlan(cmd.Context(), app.IdentityMigrationRequest{VaultPath: *ctx.vaultPath, Save: identityPlanSave})
		return ctx.renderProjection(cmd, projection, err)
	}}
	identityPlanCmd.Flags().BoolVar(&identityPlanSave, "save", false, "Save the migration plan under .pinax/records/identity-migrations")
	identityCmd.AddCommand(identityPlanCmd)
	var identityApplyPlan string
	var identityApplyYes bool
	var identityApplyResume bool
	identityApplyCmd := &cobra.Command{Use: "apply", Short: "Apply a saved identity migration plan after a fresh snapshot", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RecordIdentityApply(cmd.Context(), app.IdentityMigrationApplyRequest{VaultPath: *ctx.vaultPath, PlanID: identityApplyPlan, Yes: identityApplyYes, Resume: identityApplyResume})
		return ctx.renderProjection(cmd, projection, err)
	}}
	identityApplyCmd.Flags().StringVar(&identityApplyPlan, "plan", "", "Saved identity migration plan id or path")
	identityApplyCmd.Flags().BoolVar(&identityApplyYes, "yes", false, "Approve identity migration writes")
	identityApplyCmd.Flags().BoolVar(&identityApplyResume, "resume", false, "Resume an incomplete identity migration receipt")
	identityCmd.AddCommand(identityApplyCmd)
	recordCmd.AddCommand(identityCmd)
	root.AddCommand(recordCmd)
}

func recordRequest(ctx commandBuildContext, noteRef string) app.RecordRequest {
	return app.RecordRequest{VaultPath: *ctx.vaultPath, NoteRef: noteRef}
}
