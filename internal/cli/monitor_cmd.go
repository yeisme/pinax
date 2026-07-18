package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addMonitorCommands(root *cobra.Command, ctx commandBuildContext) {
	var command string
	var query string
	var status string
	var since string
	var until string
	var limit int

	request := func() app.MonitorRequest {
		return app.MonitorRequest{VaultPath: *ctx.vaultPath, Command: command, Query: query, Status: status, Since: since, Until: until, Limit: limit}
	}
	addQueryFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&command, "command", "", "Filter by monitored command such as note.search or index.rebuild")
		cmd.Flags().StringVar(&query, "query", "", "Case-insensitive query across safe monitor fields")
		cmd.Flags().StringVar(&status, "status", "", "Filter by normalized status")
		cmd.Flags().StringVar(&since, "since", "", "Only include runs at or after this RFC3339 timestamp")
		cmd.Flags().StringVar(&until, "until", "", "Only include runs at or before this RFC3339 timestamp")
		cmd.Flags().IntVar(&limit, "limit", 50, "Maximum monitor runs to read")
		_ = cmd.RegisterFlagCompletionFunc("command", staticCompletion("command", "note.search", "index.init", "index.refresh", "index.rebuild", "index.repair", "query.run", "dataview.run", "database.view.render"))
		_ = cmd.RegisterFlagCompletionFunc("status", staticCompletion("status", "success", "partial", "failed"))
	}

	monitorCmd := &cobra.Command{Use: "monitor", Short: "Inspect performance monitor traces"}
	runsCmd := &cobra.Command{Use: "runs", Short: "List monitor runs", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MonitorList(cmd.Context(), request())
		return ctx.renderProjection(cmd, projection, err)
	}}
	showCmd := &cobra.Command{Use: "show <run-id>", Short: "Show one monitor run", Args: cobra.ExactArgs(1), ValidArgsFunction: monitorRunCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MonitorShow(cmd.Context(), app.MonitorRequest{VaultPath: *ctx.vaultPath, RunID: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	tailCmd := &cobra.Command{Use: "tail", Short: "Read the latest monitor runs", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MonitorTail(cmd.Context(), request())
		return ctx.renderProjection(cmd, projection, err)
	}}
	summaryCmd := &cobra.Command{Use: "summary", Short: "Summarize monitor runs", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MonitorSummary(cmd.Context(), request())
		return ctx.renderProjection(cmd, projection, err)
	}}
	manageCmd := &cobra.Command{Use: "manage", Short: "Summarize monitor log maintenance", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.MonitorManage(cmd.Context(), app.MonitorRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}}

	addQueryFlags(runsCmd)
	addQueryFlags(tailCmd)
	addQueryFlags(summaryCmd)
	monitorCmd.AddCommand(runsCmd, showCmd, tailCmd, summaryCmd, manageCmd)
	root.AddCommand(monitorCmd)
}
