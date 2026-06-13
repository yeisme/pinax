package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addViewCommands(root *cobra.Command, ctx commandBuildContext) {

	viewCmd := &cobra.Command{Use: "view", Short: "Manage saved note search views"}
	viewSaveCmd := &cobra.Command{Use: "save <name>", Short: "Save a set of note search filters", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "view.save", "argument_required", "view save requires a name", "pinax view save <name> --vault <vault>")
		}
		projection, err := ctx.svc.SaveView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0], Tags: splitCSV(*ctx.noteListTag), Group: *ctx.noteGroup, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteListStatus, Sort: *ctx.noteListSort, Limit: *ctx.noteLimit, CreatedAfter: *ctx.noteListCreatedAfter, UpdatedBefore: *ctx.noteListUpdatedBefore})
		return ctx.renderProjection(cmd, projection, err)
	}}
	viewSaveCmd.Flags().StringVar(ctx.noteListTag, "tag", "", "Filter by tags; comma-separated values are allowed")
	viewSaveCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Filter by group")
	viewSaveCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Filter by folder")
	viewSaveCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Filter by kind")
	viewSaveCmd.Flags().StringVar(ctx.noteListStatus, "status", "", "Filter by status")
	viewSaveCmd.Flags().StringVar(ctx.noteListCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	viewSaveCmd.Flags().StringVar(ctx.noteListUpdatedBefore, "updated-before", "", "Filter by maximum update date; format YYYY-MM-DD or RFC3339")
	viewSaveCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort: updated, path, or title")
	viewSaveCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	viewCmd.AddCommand(viewSaveCmd)
	viewCmd.AddCommand(&cobra.Command{Use: "list", Short: "List saved views", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ListViews(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	viewShowCmd := &cobra.Command{Use: "show <name>", Short: "Search notes by saved view", ValidArgsFunction: savedViewCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "view.show", "argument_required", "view show requires a name", "pinax view show <name> --vault <vault>")
		}
		projection, err := ctx.svc.ShowView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	viewCmd.AddCommand(viewShowCmd)
	viewDeleteCmd := &cobra.Command{Use: "delete <name>", Short: "Delete a saved view", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "view.delete", "argument_required", "view delete requires a name", "pinax view delete <name> --vault <vault> --yes")
		}
		projection, err := ctx.svc.DeleteView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0], Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	viewDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm saved view deletion")
	viewCmd.AddCommand(viewDeleteCmd)
	root.AddCommand(viewCmd)

}
