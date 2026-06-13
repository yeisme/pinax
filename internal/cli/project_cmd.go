package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addProjectCommands(root *cobra.Command, ctx commandBuildContext) {
	projectCmd := &cobra.Command{Use: "project", Short: "Manage projects in the vault"}
	var boardNoteDisplay string
	var boardColumns string
	var boardFormat string
	var itemColumn string
	var itemBody string
	projectCreateCmd := &cobra.Command{
		Use:     "create <slug>",
		Short:   "Create a vault project",
		Example: "pinax project create research --name \"Research\" --notes-prefix notes/research --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.create", "argument_required", "project create requires a slug", "pinax project create <slug> --name <name> --vault <vault>")
			}
			projection, err := ctx.svc.CreateProject(cmd.Context(), app.ProjectRequest{VaultPath: *ctx.vaultPath, Slug: args[0], Name: *ctx.projectName, Description: *ctx.projectDescription, NotesPrefix: *ctx.projectNotesPrefix})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	projectCreateCmd.Flags().StringVar(ctx.projectName, "name", "", "Project name")
	projectCreateCmd.Flags().StringVar(ctx.projectDescription, "description", "", "Project description")
	projectCreateCmd.Flags().StringVar(ctx.projectNotesPrefix, "notes-prefix", "", "Project note path prefix")
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List vault projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListProjects(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:     "switch <slug>",
		Short:   "Switch the current vault project",
		Example: "pinax project switch research --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.switch", "argument_required", "project switch requires a slug", "pinax project switch <slug> --vault <vault>")
			}
			projection, err := ctx.svc.SwitchProject(cmd.Context(), app.ProjectRequest{VaultPath: *ctx.vaultPath, Slug: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	boardCmd := &cobra.Command{Use: "board", Short: "View the local project board"}
	boardShowCmd := &cobra.Command{
		Use:     "show <project>",
		Short:   "Show the local project board",
		Example: "pinax project board show research --note-display card --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.show", "argument_required", "project board show requires a project slug", "pinax project board show <project> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardShow(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], NoteDisplay: boardNoteDisplay})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardShowCmd.Flags().StringVar(&boardNoteDisplay, "note-display", "card", "Embedded note display level: card, detail, or context")
	_ = boardShowCmd.RegisterFlagCompletionFunc("note-display", staticCompletion("note-display", "card", "detail", "context"))
	boardCmd.AddCommand(boardShowCmd)
	boardConfigureCmd := &cobra.Command{
		Use:     "configure <project>",
		Short:   "Save project board configuration",
		Example: "pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.configure", "argument_required", "project board configure requires a project slug", "pinax project board configure <project> --columns inbox,next --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardConfigure(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Columns: splitCSV(boardColumns)})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardConfigureCmd.Flags().StringVar(&boardColumns, "columns", "", "Comma-separated board columns")
	boardCmd.AddCommand(boardConfigureCmd)
	boardPlanCmd := &cobra.Command{
		Use:     "plan <project>",
		Short:   "Generate a project board plan snapshot",
		Example: "pinax project board plan research --save --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.plan", "argument_required", "project board plan requires a project slug", "pinax project board plan <project> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardPlan(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], NoteDisplay: boardNoteDisplay, Save: *ctx.planSave})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardPlanCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save project board snapshot evidence")
	boardPlanCmd.Flags().StringVar(&boardNoteDisplay, "note-display", "card", "Embedded note display level: card, detail, or context")
	_ = boardPlanCmd.RegisterFlagCompletionFunc("note-display", staticCompletion("note-display", "card", "detail", "context"))
	boardCmd.AddCommand(boardPlanCmd)
	boardExportCmd := &cobra.Command{
		Use:     "export <project>",
		Short:   "Export a project board",
		Example: "pinax project board export research --format markdown --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.export", "argument_required", "project board export requires a project slug", "pinax project board export <project> --format markdown --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardExport(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], NoteDisplay: boardNoteDisplay, Format: boardFormat})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardExportCmd.Flags().StringVar(&boardFormat, "format", "markdown", "Export format: markdown")
	_ = boardExportCmd.RegisterFlagCompletionFunc("format", staticCompletion("format", "markdown"))
	boardCmd.AddCommand(boardExportCmd)
	projectCmd.AddCommand(boardCmd)
	itemCmd := &cobra.Command{Use: "item", Short: "Manage local project work items"}
	itemAddCmd := &cobra.Command{
		Use:     "add <project> <title>",
		Short:   "Create a local project work item",
		Example: "pinax project item add research \"Implement board projection\" --column next --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.item.add", "argument_required", "project item add requires a project and title", "pinax project item add <project> <title> --column next --vault <vault>")
			}
			projection, err := ctx.svc.ProjectItemAdd(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, Project: args[0], Title: args[1], Column: itemColumn, Body: itemBody})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	itemAddCmd.Flags().StringVar(&itemColumn, "column", "next", "Target board column")
	itemAddCmd.Flags().StringVar(&itemBody, "body", "", "Work item body")
	_ = itemAddCmd.RegisterFlagCompletionFunc("column", staticCompletion("column", "inbox", "next", "doing", "blocked", "review", "done"))
	itemMoveCmd := &cobra.Command{
		Use:     "move <item> <column>",
		Short:   "Move a local project work item",
		Example: "pinax project item move item_abc123 doing --vault ./my-notes --json\npinax project item move item_abc123 done --yes --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.item.move", "argument_required", "project item move requires an item id and target column", "pinax project item move <item> <column> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectItemMove(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, ItemID: args[0], Column: args[1], Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	itemMoveCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm move to a high-risk column")
	itemArchiveCmd := &cobra.Command{
		Use:     "archive <item>",
		Short:   "Archive a local project work item",
		Example: "pinax project item archive item_abc123 --yes --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.item.archive", "argument_required", "project item archive requires an item id", "pinax project item archive <item> --yes --vault <vault>")
			}
			projection, err := ctx.svc.ProjectItemArchive(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, ItemID: args[0], Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	itemArchiveCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm work item archive")
	itemCmd.AddCommand(itemAddCmd, itemMoveCmd, itemArchiveCmd)
	projectCmd.AddCommand(itemCmd)
	root.AddCommand(projectCmd)

}
