package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addActivityCommands(root *cobra.Command, ctx commandBuildContext) {
	var source string
	var query string
	var status string
	var object string
	var since string
	var until string
	var limit int

	request := func() app.ActivityRequest {
		return app.ActivityRequest{VaultPath: *ctx.vaultPath, Source: source, Query: query, Status: status, Object: object, Since: since, Until: until, Limit: limit}
	}
	addQueryFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&source, "source", "all", "Activity source: all, vault_events, monitor_runs, sync_runs, sync_daemon, api_audit, or record_ledger")
		cmd.Flags().StringVar(&query, "query", "", "Case-insensitive query across safe activity fields")
		cmd.Flags().StringVar(&status, "status", "", "Filter by normalized status")
		cmd.Flags().StringVar(&object, "object", "", "Filter by object reference, path, run id, or event id")
		cmd.Flags().StringVar(&since, "since", "", "Only include activity at or after this RFC3339 timestamp")
		cmd.Flags().StringVar(&until, "until", "", "Only include activity at or before this RFC3339 timestamp")
		cmd.Flags().IntVar(&limit, "limit", 50, "Maximum activity entries to read")
		_ = cmd.RegisterFlagCompletionFunc("source", staticCompletion("source", "all", "vault_events", "monitor_runs", "sync_runs", "sync_daemon", "api_audit", "record_ledger"))
	}

	activityCmd := &cobra.Command{Use: "activity", Short: "Inspect unified vault activity and logs"}
	sourcesCmd := &cobra.Command{Use: "sources", Short: "List activity log sources", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ActivitySources(cmd.Context(), app.ActivityRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}
	listCmd := &cobra.Command{Use: "list", Short: "List recent vault activity", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ActivityList(cmd.Context(), request())
		return ctx.renderProjection(cmd, projection, err)
	}}
	showCmd := &cobra.Command{Use: "show <event-id>", Short: "Show one activity event", Args: cobra.ExactArgs(1), ValidArgsFunction: activityEventCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ActivityShow(cmd.Context(), app.ActivityRequest{VaultPath: *ctx.vaultPath, EventID: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	tailCmd := &cobra.Command{Use: "tail", Short: "Read the latest activity snapshot", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ActivityTail(cmd.Context(), request())
		return ctx.renderProjection(cmd, projection, err)
	}}
	manageCmd := &cobra.Command{Use: "manage", Short: "Summarize activity log maintenance", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ActivityManage(cmd.Context(), app.ActivityRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}

	addQueryFlags(listCmd)
	addQueryFlags(tailCmd)
	activityCmd.AddCommand(sourcesCmd, listCmd, showCmd, tailCmd, manageCmd)
	root.AddCommand(activityCmd)
}
