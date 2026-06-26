package cli

import (
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addFolderCommands(root *cobra.Command, ctx commandBuildContext) {
	var listPurpose string
	var listUnder string
	var listIncludeEmpty bool
	var listDepth int
	var createPurpose string
	var createDryRun bool
	var createYes bool
	var renameDryRun bool
	var renameYes bool
	var moveDryRun bool
	var moveYes bool
	var deleteDryRun bool
	var deleteYes bool
	var deleteEmptyOnly bool
	var adoptPurpose string
	var adoptDryRun bool
	var adoptYes bool
	var repairPlan bool

	folderCmd := &cobra.Command{Use: "folder", Short: "Manage vault folders"}
	folderListCmd := &cobra.Command{Use: "list", Short: "List vault folders", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ListFolders(cmd.Context(), app.FolderListRequest{VaultPath: *ctx.vaultPath, Purpose: listPurpose, Under: listUnder, IncludeEmpty: listIncludeEmpty, Depth: listDepth})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderListCmd.Flags().StringVar(&listPurpose, "purpose", "all", "Folder purpose: notes, assets, generic, or all")
	folderListCmd.Flags().StringVar(&listUnder, "under", "", "Only list this folder and its descendants")
	folderListCmd.Flags().BoolVar(&listIncludeEmpty, "include-empty", false, "Include empty folders and registry-only folders")
	folderListCmd.Flags().IntVar(&listDepth, "depth", 0, "Maximum folder depth; 0 means unlimited")
	_ = folderListCmd.RegisterFlagCompletionFunc("purpose", staticCompletion("purpose", "notes", "assets", "generic", "all"))
	_ = folderListCmd.RegisterFlagCompletionFunc("under", folderPathCompletion(func() string { return *ctx.vaultPath }))
	_ = folderListCmd.RegisterFlagCompletionFunc("depth", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{strconv.Itoa(1), strconv.Itoa(2), strconv.Itoa(3)}, cobra.ShellCompDirectiveNoFileComp
	})

	folderShowCmd := &cobra.Command{Use: "show <path>", Short: "Show vault folder details", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.show", "argument_required", "folder show requires a folder path", "pinax folder show <path> --vault <vault>")
		}
		projection, err := ctx.svc.ShowFolder(cmd.Context(), app.FolderRequest{VaultPath: *ctx.vaultPath, Path: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderShowCmd.ValidArgsFunction = folderPathCompletion(func() string { return *ctx.vaultPath })

	folderCreateCmd := &cobra.Command{Use: "create <path>", Short: "Create and register a vault folder", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.create", "argument_required", "folder create requires a folder path", "pinax folder create <path> --vault <vault>")
		}
		projection, err := ctx.svc.CreateFolder(cmd.Context(), app.FolderOperationRequest{VaultPath: *ctx.vaultPath, Path: args[0], Purpose: createPurpose, DryRun: createDryRun, Yes: createYes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderCreateCmd.Flags().StringVar(&createPurpose, "purpose", "generic", "Folder purpose: notes, assets, or generic")
	folderCreateCmd.Flags().BoolVar(&createDryRun, "dry-run", false, "Preview the plan only; do not write files or the registry")
	folderCreateCmd.Flags().BoolVar(&createYes, "yes", false, "Confirm remote folder creation when using --api-url")
	_ = folderCreateCmd.RegisterFlagCompletionFunc("purpose", staticCompletion("purpose", "notes", "assets", "generic"))

	folderRenameCmd := &cobra.Command{Use: "rename <old> <new>", Short: "Rename a vault folder", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.rename", "argument_required", "folder rename requires old and new folder paths", "pinax folder rename <old> <new> --vault <vault> --yes")
		}
		projection, err := ctx.svc.RenameFolder(cmd.Context(), app.FolderOperationRequest{VaultPath: *ctx.vaultPath, Path: args[0], TargetPath: args[1], DryRun: renameDryRun, Yes: renameYes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderRenameCmd.ValidArgsFunction = folderPathCompletion(func() string { return *ctx.vaultPath })
	folderRenameCmd.Flags().BoolVar(&renameDryRun, "dry-run", false, "Preview the plan only; do not write folders or the registry")
	folderRenameCmd.Flags().BoolVar(&renameYes, "yes", false, "Confirm folder rename")

	folderMoveCmd := &cobra.Command{Use: "move <path> <target-parent>", Short: "Move a vault folder to the target parent folder", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.move", "argument_required", "folder move requires path and target-parent", "pinax folder move <path> <target-parent> --vault <vault> --yes")
		}
		projection, err := ctx.svc.MoveFolder(cmd.Context(), app.FolderOperationRequest{VaultPath: *ctx.vaultPath, Path: args[0], TargetParent: args[1], DryRun: moveDryRun, Yes: moveYes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderMoveCmd.ValidArgsFunction = folderPathCompletion(func() string { return *ctx.vaultPath })
	folderMoveCmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "Preview the plan only; do not write folders or the registry")
	folderMoveCmd.Flags().BoolVar(&moveYes, "yes", false, "Confirm folder move")

	folderDeleteCmd := &cobra.Command{Use: "delete <path>", Short: "Delete an empty vault folder", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.delete", "argument_required", "folder delete requires a folder path", "pinax folder delete <path> --empty-only --vault <vault> --yes")
		}
		projection, err := ctx.svc.DeleteFolder(cmd.Context(), app.FolderOperationRequest{VaultPath: *ctx.vaultPath, Path: args[0], EmptyOnly: deleteEmptyOnly, DryRun: deleteDryRun, Yes: deleteYes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderDeleteCmd.ValidArgsFunction = folderPathCompletion(func() string { return *ctx.vaultPath })
	folderDeleteCmd.Flags().BoolVar(&deleteEmptyOnly, "empty-only", false, "Allow deleting only empty folders")
	folderDeleteCmd.Flags().BoolVar(&deleteDryRun, "dry-run", false, "Preview the plan only; do not delete folders or the registry")
	folderDeleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "Confirm empty folder deletion")

	folderAdoptCmd := &cobra.Command{Use: "adopt <path>", Short: "Adopt an existing vault folder", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "folder.adopt", "argument_required", "folder adopt requires a folder path", "pinax folder adopt <path> --purpose notes --vault <vault> --yes")
		}
		projection, err := ctx.svc.AdoptFolder(cmd.Context(), app.FolderOperationRequest{VaultPath: *ctx.vaultPath, Path: args[0], Purpose: adoptPurpose, DryRun: adoptDryRun, Yes: adoptYes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderAdoptCmd.Flags().StringVar(&adoptPurpose, "purpose", "generic", "Folder purpose: notes, assets, or generic")
	folderAdoptCmd.Flags().BoolVar(&adoptDryRun, "dry-run", false, "Preview the plan only; do not write the registry")
	folderAdoptCmd.Flags().BoolVar(&adoptYes, "yes", false, "Confirm folder adoption")
	_ = folderAdoptCmd.RegisterFlagCompletionFunc("purpose", staticCompletion("purpose", "notes", "assets", "generic"))

	folderRepairCmd := &cobra.Command{Use: "repair", Short: "Generate a vault folder repair plan", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.RepairFolders(cmd.Context(), app.FolderRepairRequest{VaultPath: *ctx.vaultPath, Plan: repairPlan})
		return ctx.renderProjection(cmd, projection, err)
	}}
	folderRepairCmd.Flags().BoolVar(&repairPlan, "plan", false, "Only generate the repair plan; do not write the vault")

	folderCmd.AddCommand(folderListCmd, folderShowCmd, folderCreateCmd, folderRenameCmd, folderMoveCmd, folderDeleteCmd, folderAdoptCmd, folderRepairCmd)
	root.AddCommand(folderCmd)
}
