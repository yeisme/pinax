package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addProjectCommands(root *cobra.Command, ctx commandBuildContext) {
	projectCmd := &cobra.Command{Use: "project", Short: "Manage projects in the vault"}
	var subprojectTitle string
	var subprojectTemplate string
	var learningTitle string
	var learningProjectName string
	var learningNotesPrefix string
	var learningPreset string
	var learningDryRun bool
	var learningNoStarterItems bool
	var boardSubproject string
	var boardCompact bool
	var boardNoteDisplay string
	var boardView string
	var boardViewGroup string
	var boardViewSort string
	var boardViewDisplay string
	var boardColumns string
	var boardFormat string
	var itemSubproject string
	var itemColumn string
	var itemBody string
	var itemLabels string
	var itemMilestone string
	var itemPriority string
	var itemDueAt string
	var itemBlockedBy string
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
	projectDeleteCmd := &cobra.Command{
		Use:               "delete <slug>",
		Short:             "Move a project to trash",
		Example:           "pinax project delete history --vault ./my-notes --yes --json",
		ValidArgsFunction: projectSlugCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.delete", "argument_required", "project delete requires a slug", "pinax project delete <slug> --vault <vault> --yes")
			}
			projection, err := ctx.svc.ProjectDelete(cmd.Context(), app.ProjectDeleteRequest{VaultPath: *ctx.vaultPath, Project: args[0], Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	projectDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm moving the project to trash")
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List vault projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ListProjects(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	})
	projectShowCmd := &cobra.Command{
		Use:               "show <slug>",
		Short:             "Show a vault project",
		Example:           "pinax project show research --vault ./my-notes --json",
		ValidArgsFunction: projectSlugCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.show", "argument_required", "project show requires a slug", "pinax project show <slug> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectShow(cmd.Context(), app.ProjectRequest{VaultPath: *ctx.vaultPath, Slug: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	projectCmd.AddCommand(projectShowCmd)
	subprojectCmd := &cobra.Command{Use: "subproject", Short: "Manage project subproject workspaces"}
	subprojectCreateCmd := &cobra.Command{
		Use:     "create <project> <slug>",
		Short:   "Create a project subproject workspace",
		Example: "pinax project subproject create research stock-learning --title \"Stock Learning\" --template scenario --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.subproject.create", "argument_required", "project subproject create requires project and slug", "pinax project subproject create <project> <slug> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectSubprojectCreate(cmd.Context(), app.ProjectWorkspaceRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: args[1], Title: subprojectTitle, Template: subprojectTemplate})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	subprojectCreateCmd.Flags().StringVar(&subprojectTitle, "title", "", "Subproject title")
	subprojectCreateCmd.Flags().StringVar(&subprojectTemplate, "template", "scenario", "Workspace template: scenario")
	subprojectCreateCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm remote subproject workspace creation when using --api-url")
	subprojectCreateCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	_ = subprojectCreateCmd.RegisterFlagCompletionFunc("template", staticCompletion("template", "scenario"))
	subprojectListCmd := &cobra.Command{
		Use:     "list [project]",
		Short:   "List project subproject workspaces",
		Example: "pinax project subproject list research --vault ./my-notes --json\npinax project subproject list --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.subproject.list", "argument_required", "project subproject list accepts at most one project", "pinax project subproject list [project] --vault <vault>")
			}
			project := ""
			if len(args) == 1 {
				project = args[0]
			}
			projection, err := ctx.svc.ProjectSubprojectList(cmd.Context(), app.ProjectWorkspaceRequest{VaultPath: *ctx.vaultPath, Project: project})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	subprojectListCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	subprojectShowCmd := &cobra.Command{
		Use:     "show <project> <slug>",
		Short:   "Show a project subproject workspace",
		Example: "pinax project subproject show research stock-learning --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.subproject.show", "argument_required", "project subproject show requires project and slug", "pinax project subproject show <project> <slug> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectSubprojectShow(cmd.Context(), app.ProjectWorkspaceRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: args[1]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	subprojectShowCmd.ValidArgsFunction = projectThenSubprojectCompletion(func() string { return *ctx.vaultPath })
	subprojectDeleteCmd := &cobra.Command{
		Use:               "delete <project> <slug>",
		Short:             "Move a project subproject workspace to trash",
		Example:           "pinax project subproject delete history-learning history-info --vault ./my-notes --yes --json",
		ValidArgsFunction: projectThenSubprojectCompletion(func() string { return *ctx.vaultPath }),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.subproject.delete", "argument_required", "project subproject delete requires project and slug", "pinax project subproject delete <project> <slug> --vault <vault> --yes")
			}
			projection, err := ctx.svc.ProjectSubprojectDelete(cmd.Context(), app.ProjectSubprojectDeleteRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: args[1], Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	subprojectDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm moving the subproject workspace to trash")
	subprojectCmd.AddCommand(subprojectCreateCmd, subprojectListCmd, subprojectShowCmd, subprojectDeleteCmd)
	projectCmd.AddCommand(subprojectCmd)
	learningCmd := &cobra.Command{Use: "learning", Short: "Manage long-term learning project packs"}
	learningInitCmd := &cobra.Command{
		Use:     "init <project> <slug>",
		Short:   "Initialize a long-term learning project pack",
		Example: "pinax project learning init investing stock-learning --title \"学习炒股的全部笔记\" --project-name \"学习炒股\" --notes-prefix notes/investing --preset stock-learning --vault ./stock-learning-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.learning.init", "argument_required", "project learning init requires project and slug", "pinax project learning init <project> <slug> --title <title> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectLearningInit(cmd.Context(), app.ProjectLearningRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: args[1], Title: learningTitle, ProjectName: learningProjectName, NotesPrefix: learningNotesPrefix, Preset: learningPreset, DryRun: learningDryRun, NoStarterItems: learningNoStarterItems})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	learningInitCmd.Flags().StringVar(&learningTitle, "title", "", "Learning project title")
	learningInitCmd.Flags().StringVar(&learningProjectName, "project-name", "", "Vault project display name")
	learningInitCmd.Flags().StringVar(&learningNotesPrefix, "notes-prefix", "", "Project note path prefix")
	learningInitCmd.Flags().StringVar(&learningPreset, "preset", "learning", "Learning preset: learning or stock-learning")
	learningInitCmd.Flags().BoolVar(&learningDryRun, "dry-run", false, "Preview learning project initialization without writing")
	learningInitCmd.Flags().BoolVar(&learningNoStarterItems, "no-starter-items", false, "Skip starter board items")
	_ = learningInitCmd.RegisterFlagCompletionFunc("preset", staticCompletion("preset", "learning", "stock-learning"))
	learningCmd.AddCommand(learningInitCmd)
	projectCmd.AddCommand(learningCmd)
	projectSwitchCmd := &cobra.Command{
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
	}
	projectSwitchCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	projectCmd.AddCommand(projectSwitchCmd)
	boardCmd := &cobra.Command{Use: "board", Short: "View the local project board"}
	boardShowCmd := &cobra.Command{
		Use:     "show <project>",
		Short:   "Show the local project board",
		Example: "pinax project board show research --note-display card --vault ./my-notes --json\npinax project board show research --view active --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.show", "argument_required", "project board show requires a project slug", "pinax project board show <project> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardShow(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: boardSubproject, View: boardView, NoteDisplay: boardNoteDisplay, Compact: boardCompact})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardShowCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	boardShowCmd.Flags().StringVar(&boardSubproject, "subproject", "", "Limit the board to a project subproject workspace")
	boardShowCmd.Flags().BoolVar(&boardCompact, "compact", false, "Render a compact human board summary")
	boardShowCmd.Flags().StringVar(&boardView, "view", "", "Saved board view name")
	boardShowCmd.Flags().StringVar(&boardNoteDisplay, "note-display", "card", "Embedded note display level: card, detail, or context")
	_ = boardShowCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
	_ = boardShowCmd.RegisterFlagCompletionFunc("note-display", staticCompletion("note-display", "card", "detail", "context"))
	boardCmd.AddCommand(boardShowCmd)
	boardViewCmd := &cobra.Command{Use: "view", Short: "Manage saved project board views"}
	boardViewSaveCmd := &cobra.Command{
		Use:     "save <project> <view>",
		Short:   "Save a project board view",
		Example: "pinax project board view save research active --columns inbox,next,doing --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.view.save", "argument_required", "project board view save requires project and view", "pinax project board view save <project> <view> --columns inbox,next --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardViewSave(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: boardSubproject, View: args[1], Columns: splitCSV(boardColumns), GroupBy: boardViewGroup, Sort: boardViewSort, Display: boardViewDisplay})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardViewSaveCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	boardViewSaveCmd.Flags().StringVar(&boardSubproject, "subproject", "", "Save a subproject-scoped board view")
	boardViewSaveCmd.Flags().StringVar(&boardColumns, "columns", "", "Comma-separated board columns")
	boardViewSaveCmd.Flags().StringVar(&boardViewGroup, "group", "", "Saved grouping mode")
	boardViewSaveCmd.Flags().StringVar(&boardViewSort, "sort", "", "Saved sort key")
	boardViewSaveCmd.Flags().StringVar(&boardViewDisplay, "display", "card", "Saved display mode: card, detail, or context")
	_ = boardViewSaveCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
	_ = boardViewSaveCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "card", "detail", "context"))
	boardViewCmd.AddCommand(boardViewSaveCmd)
	boardCmd.AddCommand(boardViewCmd)
	boardConfigureCmd := &cobra.Command{
		Use:     "configure <project>",
		Short:   "Save project board configuration",
		Example: "pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.configure", "argument_required", "project board configure requires a project slug", "pinax project board configure <project> --columns inbox,next --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardConfigure(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: boardSubproject, Columns: splitCSV(boardColumns)})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardConfigureCmd.Flags().StringVar(&boardSubproject, "subproject", "", "Configure a subproject-scoped board")
	boardConfigureCmd.Flags().StringVar(&boardColumns, "columns", "", "Comma-separated board columns")
	boardConfigureCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	_ = boardConfigureCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
	boardCmd.AddCommand(boardConfigureCmd)
	boardPlanCmd := &cobra.Command{
		Use:     "plan <project>",
		Short:   "Generate a project board plan snapshot",
		Example: "pinax project board plan research --save --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.plan", "argument_required", "project board plan requires a project slug", "pinax project board plan <project> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardPlan(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: boardSubproject, NoteDisplay: boardNoteDisplay, Save: *ctx.planSave})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardPlanCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	boardPlanCmd.Flags().BoolVar(ctx.planSave, "save", false, "Save project board snapshot evidence")
	boardPlanCmd.Flags().StringVar(&boardSubproject, "subproject", "", "Limit the board plan to a project subproject workspace")
	boardPlanCmd.Flags().StringVar(&boardNoteDisplay, "note-display", "card", "Embedded note display level: card, detail, or context")
	_ = boardPlanCmd.RegisterFlagCompletionFunc("note-display", staticCompletion("note-display", "card", "detail", "context"))
	_ = boardPlanCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
	boardCmd.AddCommand(boardPlanCmd)
	boardExportCmd := &cobra.Command{
		Use:     "export <project>",
		Short:   "Export a project board",
		Example: "pinax project board export research --format markdown --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.board.export", "argument_required", "project board export requires a project slug", "pinax project board export <project> --format markdown --vault <vault>")
			}
			projection, err := ctx.svc.ProjectBoardExport(cmd.Context(), app.ProjectBoardRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: boardSubproject, NoteDisplay: boardNoteDisplay, Format: boardFormat})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	boardExportCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	boardExportCmd.Flags().StringVar(&boardSubproject, "subproject", "", "Limit the board export to a project subproject workspace")
	boardExportCmd.Flags().StringVar(&boardFormat, "format", "markdown", "Export format: markdown")
	_ = boardExportCmd.RegisterFlagCompletionFunc("format", staticCompletion("format", "markdown"))
	_ = boardExportCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
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
			projection, err := ctx.svc.ProjectItemAdd(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, Project: args[0], Subproject: itemSubproject, Title: args[1], Column: itemColumn, Body: itemBody, Labels: splitCSV(itemLabels), Milestone: itemMilestone, Priority: itemPriority, DueAt: itemDueAt, BlockedBy: splitCSV(itemBlockedBy)})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	itemAddCmd.Flags().StringVar(&itemSubproject, "subproject", "", "Create the item inside a project subproject workspace")
	itemAddCmd.Flags().StringVar(&itemColumn, "column", "next", "Target board column")
	itemAddCmd.Flags().StringVar(&itemBody, "body", "", "Work item body")
	itemAddCmd.Flags().StringVar(&itemLabels, "labels", "", "Comma-separated item labels")
	itemAddCmd.Flags().StringVar(&itemMilestone, "milestone", "", "Item milestone")
	itemAddCmd.Flags().StringVar(&itemPriority, "priority", "", "Item priority")
	itemAddCmd.Flags().StringVar(&itemDueAt, "due-at", "", "Item due date")
	itemAddCmd.Flags().StringVar(&itemBlockedBy, "blocked-by", "", "Comma-separated blocking item ids")
	itemAddCmd.ValidArgsFunction = projectSlugCompletion(func() string { return *ctx.vaultPath })
	_ = itemAddCmd.RegisterFlagCompletionFunc("column", staticCompletion("column", "inbox", "next", "doing", "blocked", "review", "done"))
	_ = itemAddCmd.RegisterFlagCompletionFunc("subproject", projectSubprojectCompletion(func() string { return *ctx.vaultPath }))
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
	_ = itemMoveCmd.RegisterFlagCompletionFunc("column", staticCompletion("column", "inbox", "next", "doing", "blocked", "review", "done"))
	itemShowCmd := &cobra.Command{
		Use:     "show <item>",
		Short:   "Show a local project work item",
		Example: "pinax project item show item_abc123 --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.item.show", "argument_required", "project item show requires an item id", "pinax project item show <item> --vault <vault>")
			}
			projection, err := ctx.svc.ProjectItemShow(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, ItemID: args[0]})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	var itemPlanAction string
	itemPlanCmd := &cobra.Command{
		Use:     "plan <item>",
		Short:   "Generate a project work item change plan",
		Example: "pinax project item plan item_abc123 --action move --column doing --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "project.item.plan", "argument_required", "project item plan requires an item id", "pinax project item plan <item> --action move --column doing --vault <vault>")
			}
			projection, err := ctx.svc.ProjectItemPlan(cmd.Context(), app.ProjectItemRequest{VaultPath: *ctx.vaultPath, ItemID: args[0], Action: itemPlanAction, Column: itemColumn, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	itemPlanCmd.Flags().StringVar(&itemPlanAction, "action", "archive", "Planned action: archive or move")
	itemPlanCmd.Flags().StringVar(&itemColumn, "column", "", "Target board column for --action move")
	itemPlanCmd.Flags().BoolVar(ctx.yes, "yes", false, "Allow high-risk plan checks to pass after a snapshot")
	_ = itemPlanCmd.RegisterFlagCompletionFunc("action", staticCompletion("action", "archive", "move"))
	_ = itemPlanCmd.RegisterFlagCompletionFunc("column", staticCompletion("column", "inbox", "next", "doing", "blocked", "review", "done"))
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
	itemCmd.AddCommand(itemAddCmd, itemMoveCmd, itemShowCmd, itemPlanCmd, itemArchiveCmd)
	projectCmd.AddCommand(itemCmd)
	root.AddCommand(projectCmd)

}
