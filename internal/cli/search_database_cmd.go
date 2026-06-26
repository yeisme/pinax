package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func addSearchCommand(root *cobra.Command, ctx commandBuildContext) {
	searchCmd := &cobra.Command{
		Use:     "search <query>",
		Short:   "Search local notes",
		Example: "pinax search \"project retrospective\" --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, ctx.outputMode(), "note.search", "argument_required", "search requires a query", "pinax search <query> --vault <vault>")
			}
			projection, err := ctx.svc.SearchProjection(cmd.Context(), app.SearchRequest{VaultPath: *ctx.vaultPath, Query: args[0], Tags: splitCSV(*ctx.noteTags), Group: *ctx.noteGroup, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteStatus, CreatedAfter: *ctx.searchCreatedAfter, UpdatedAfter: *ctx.searchUpdatedAfter, LinkTarget: *ctx.searchLinkTarget, HasAttachment: *ctx.searchHasAttachment, Limit: *ctx.noteLimit, Sort: *ctx.noteListSort, AllowStale: *ctx.searchAllowStale, At: *ctx.searchAt, IncludeDirty: *ctx.searchIncludeDirty, ChangedSince: *ctx.searchChangedSince, Revision: *ctx.searchRevision})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	searchCmd.Flags().StringVar(ctx.noteTags, "tag", "", "Filter by tag")
	searchCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Filter by group")
	searchCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Filter by folder")
	searchCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Filter by kind")
	searchCmd.Flags().StringVar(ctx.noteStatus, "status", "", "Filter by status")
	searchCmd.Flags().StringVar(ctx.searchCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	searchCmd.Flags().StringVar(ctx.searchUpdatedAfter, "updated-after", "", "Filter by minimum update date; format YYYY-MM-DD or RFC3339")
	searchCmd.Flags().StringVar(ctx.searchLinkTarget, "link-target", "", "Filter by link target")
	searchCmd.Flags().BoolVar(ctx.searchHasAttachment, "has-attachment", false, "Return only notes with attachment references")
	searchCmd.Flags().BoolVar(ctx.searchAllowStale, "allow-stale", false, "Allow stale index partial results")
	searchCmd.Flags().StringVar(ctx.searchAt, "at", "", "Read the specified projection through the version backend; currently supports HEAD")
	searchCmd.Flags().BoolVar(ctx.searchIncludeDirty, "include-dirty", false, "Include dirty worktree content in version-aware search")
	searchCmd.Flags().StringVar(ctx.searchChangedSince, "changed-since", "", "Filter to notes changed after the revision")
	searchCmd.Flags().StringVar(ctx.searchRevision, "revision", "", "Read the historical projection for the specified revision")
	searchCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort: relevance, updated, created, title, or path")
	_ = searchCmd.RegisterFlagCompletionFunc("sort", staticCompletion("sort", "relevance", "updated", "created", "title", "path"))
	searchCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	root.AddCommand(searchCmd)
}

func addQueryCommands(root *cobra.Command, ctx commandBuildContext) {
	queryCmd := &cobra.Command{
		Use:   "query",
		Short: "Query the local notes database",
		Long:  "Query the local notes database. Common workflow: pinax index status --vault ./my-notes, pinax query explain 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax query run 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax database view save active --query 'SELECT title FROM notes' --vault ./my-notes.",
	}
	queryRunCmd := &cobra.Command{Use: "run <sql>", Short: "Run a Pinax SQL query", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "query.run", "argument_required", "query run requires SQL", "pinax query run 'SELECT title FROM notes LIMIT 20' --vault <vault>")
		}
		projection, err := ctx.svc.QueryRun(cmd.Context(), app.QueryRequest{VaultPath: *ctx.vaultPath, SQL: args[0], LazyIndex: *ctx.queryLazyIndex, Limit: *ctx.noteLimit, Sort: *ctx.noteListSort, Cursor: *ctx.queryCursor})
		return ctx.renderProjection(cmd, projection, err)
	}}
	queryRunCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort by property")
	queryRunCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	queryRunCmd.Flags().StringVar(ctx.queryCursor, "cursor", "", "Pagination cursor")
	queryRunCmd.Flags().BoolVar(ctx.queryLazyIndex, "lazy-index", false, "Allow explicit lazy index loading")
	_ = queryRunCmd.RegisterFlagCompletionFunc("sort", staticCompletion("property", "title", "updated_at", "created_at", "status", "path"))
	queryCmd.AddCommand(queryRunCmd)
	queryCmd.AddCommand(&cobra.Command{Use: "explain <sql>", Short: "Explain the Pinax SQL query plan", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "query.explain", "argument_required", "query explain requires SQL", "pinax query explain 'SELECT title FROM notes LIMIT 20' --vault <vault>")
		}
		projection, err := ctx.svc.QueryExplain(cmd.Context(), app.QueryRequest{VaultPath: *ctx.vaultPath, SQL: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}})
	root.AddCommand(queryCmd)
}

func addDataviewCommands(root *cobra.Command, ctx commandBuildContext) {
	dataviewCmd := &cobra.Command{Use: "dataview", Short: "Run safe Dataview-compatible queries", Long: "Run safe Dataview-compatible queries. Supported forms: TABLE, LIST, and TASK with FROM, WHERE, SORT, GROUP BY, and LIMIT."}
	dataviewRunCmd := &cobra.Command{Use: "run <query>", Short: "Run a Dataview-compatible query", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "dataview.run", "argument_required", "dataview run requires a query", "pinax dataview run 'TABLE title FROM #pinax LIMIT 5' --vault <vault>")
		}
		projection, err := ctx.svc.DataviewRun(cmd.Context(), app.DataviewRequest{VaultPath: *ctx.vaultPath, Query: args[0], LazyIndex: *ctx.queryLazyIndex, Limit: *ctx.noteLimit, Sort: *ctx.noteListSort, Cursor: *ctx.queryCursor})
		return ctx.renderProjection(cmd, projection, err)
	}}
	dataviewRunCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort by property")
	dataviewRunCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	dataviewRunCmd.Flags().StringVar(ctx.queryCursor, "cursor", "", "Pagination cursor")
	dataviewRunCmd.Flags().BoolVar(ctx.queryLazyIndex, "lazy-index", false, "Allow explicit lazy index loading")
	dataviewCmd.AddCommand(dataviewRunCmd)
	dataviewCmd.AddCommand(&cobra.Command{Use: "explain <query>", Short: "Explain a Dataview-compatible query plan", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "dataview.explain", "argument_required", "dataview explain requires a query", "pinax dataview explain 'LIST FROM #pinax LIMIT 5' --vault <vault>")
		}
		projection, err := ctx.svc.DataviewExplain(cmd.Context(), app.DataviewRequest{VaultPath: *ctx.vaultPath, Query: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}})
	root.AddCommand(dataviewCmd)
}

func addDatabaseCommands(root *cobra.Command, ctx commandBuildContext) {
	databaseCmd := &cobra.Command{Use: "database", Short: "Manage local notes database views", Long: "Manage local notes database views. Common workflow: pinax index status --vault ./my-notes, pinax query explain 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax query run 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax database view save active --query 'SELECT title FROM notes' --vault ./my-notes."}
	databaseViewCmd := &cobra.Command{Use: "view", Short: "Manage database views"}
	databaseViewSaveCmd := &cobra.Command{Use: "save <name>", Short: "Save a database view", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.view.save", "argument_required", "database view save requires a name", "pinax database view save <name> --vault <vault>")
		}
		var projection domain.Projection
		var err error
		if strings.TrimSpace(*ctx.databaseViewQuery) != "" {
			projection, err = ctx.svc.SaveDatabaseView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0], Kind: *ctx.noteKind, Display: *ctx.databaseViewDisplay, Language: *ctx.databaseViewLanguage, Query: *ctx.databaseViewQuery, Columns: *ctx.databaseViewColumns, GroupBy: *ctx.databaseViewGroupBy, CalendarField: *ctx.databaseViewCalendar, BoardColumn: *ctx.databaseViewBoardColumn, Limit: *ctx.noteLimit})
		} else {
			projection, err = ctx.svc.SaveView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0], Tags: splitCSV(*ctx.noteListTag), Group: *ctx.noteGroup, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteListStatus, Sort: *ctx.noteListSort, Limit: *ctx.noteLimit, CreatedAfter: *ctx.noteListCreatedAfter, UpdatedBefore: *ctx.noteListUpdatedBefore})
			projection.Command = "database.view.save"
		}
		return ctx.renderProjection(cmd, projection, err)
	}}
	databaseViewSaveCmd.Flags().StringVar(ctx.noteListTag, "tag", "", "Filter by tags; comma-separated values are allowed")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Filter by group")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Filter by folder")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Filter by kind")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteListStatus, "status", "", "Filter by status")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteListCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteListUpdatedBefore, "updated-before", "", "Filter by maximum update date; format YYYY-MM-DD or RFC3339")
	databaseViewSaveCmd.Flags().StringVar(ctx.noteListSort, "sort", "", "Sort: updated, path, or title")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewQuery, "query", "", "Pinax SQL query")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewLanguage, "language", "sql", "Query language: sql or dataview")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewDisplay, "display", "", "Display mode: table, board, list, or calendar")
	databaseViewSaveCmd.Flags().StringArrayVar(ctx.databaseViewColumns, "column", nil, "Display columns; repeatable")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewGroupBy, "group-by", "", "Group rows by property")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewCalendar, "calendar-field", "", "Calendar date property")
	databaseViewSaveCmd.Flags().StringVar(ctx.databaseViewBoardColumn, "board-column", "", "Board column property")
	databaseViewSaveCmd.Flags().IntVar(ctx.noteLimit, "limit", 0, "Limit the number of results")
	_ = databaseViewSaveCmd.RegisterFlagCompletionFunc("language", staticCompletion("language", "sql", "dataview"))
	_ = databaseViewSaveCmd.RegisterFlagCompletionFunc("display", staticCompletion("display", "table", "board", "list", "calendar"))
	databaseViewCmd.AddCommand(databaseViewSaveCmd)
	databaseViewCmd.AddCommand(&cobra.Command{Use: "list", Short: "List database views", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.ListViews(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		projection.Command = "database.view.list"
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseViewCmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show a database view", ValidArgsFunction: savedViewCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.view.show", "argument_required", "database view show requires a name", "pinax database view show <name> --vault <vault>")
		}
		projection, err := ctx.svc.ShowDatabaseView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0]})
		projection.Command = "database.view.show"
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseViewCmd.AddCommand(&cobra.Command{Use: "render <name>", Short: "Render a database view", ValidArgsFunction: savedViewCompletion(func() string { return *ctx.vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.view.render", "argument_required", "database view render requires a name", "pinax database view render <name> --vault <vault>")
		}
		projection, err := ctx.svc.RenderDatabaseView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseViewDeleteCmd := &cobra.Command{Use: "delete <name>", Short: "Delete a database view", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.view.delete", "argument_required", "database view delete requires a name", "pinax database view delete <name> --vault <vault> --yes")
		}
		projection, err := ctx.svc.DeleteView(cmd.Context(), app.ViewRequest{VaultPath: *ctx.vaultPath, Name: args[0], Yes: *ctx.yes})
		projection.Command = "database.view.delete"
		return ctx.renderProjection(cmd, projection, err)
	}}
	databaseViewDeleteCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm database view deletion")
	databaseViewCmd.AddCommand(databaseViewDeleteCmd)
	databaseCmd.AddCommand(databaseViewCmd)
	databaseSchemaCmd := &cobra.Command{Use: "schema", Short: "Manage database property schema"}
	databaseSchemaCmd.AddCommand(&cobra.Command{Use: "infer", Short: "Infer property schema", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.DatabaseSchemaInfer(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseSchemaCmd.AddCommand(&cobra.Command{Use: "list", Short: "List property schema overrides", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := ctx.svc.DatabaseSchemaList(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseSchemaCmd.AddCommand(&cobra.Command{Use: "show <property>", Short: "Show a property schema override", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.schema.show", "argument_required", "database schema show requires a property name", "pinax database schema show <property> --vault <vault>")
		}
		projection, err := ctx.svc.DatabaseSchemaShow(cmd.Context(), app.DatabaseSchemaRequest{VaultPath: *ctx.vaultPath, Name: args[0]})
		return ctx.renderProjection(cmd, projection, err)
	}})
	databaseSchemaSetCmd := &cobra.Command{Use: "set <property>", Short: "Set a property type", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "database.schema.set", "argument_required", "database schema set requires a property name", "pinax database schema set status --type select --vault <vault>")
		}
		projection, err := ctx.svc.DatabaseSchemaSet(cmd.Context(), app.DatabaseSchemaRequest{VaultPath: *ctx.vaultPath, Name: args[0], Type: *ctx.databaseSchemaType, Values: splitCSV(*ctx.databaseSchemaValues)})
		return ctx.renderProjection(cmd, projection, err)
	}}
	databaseSchemaSetCmd.Flags().StringVar(ctx.databaseSchemaType, "type", "", "Property type")
	databaseSchemaSetCmd.Flags().StringVar(ctx.databaseSchemaValues, "values", "", "Allowed select/list values, comma-separated")
	_ = databaseSchemaSetCmd.RegisterFlagCompletionFunc("type", staticCompletion("type", "text", "string", "number", "checkbox", "boolean", "date", "select", "multi_select", "list", "url", "email", "person_text", "relation", "link", "rollup", "formula"))
	databaseSchemaCmd.AddCommand(databaseSchemaSetCmd)
	databaseCmd.AddCommand(databaseSchemaCmd)
	root.AddCommand(databaseCmd)
}

func addImportExportCommands(root *cobra.Command, ctx commandBuildContext) {
	importCmd := &cobra.Command{Use: "import", Short: "Import local Markdown content"}
	importMarkdownCmd := &cobra.Command{Use: "markdown <source>", Short: "Import a local Markdown file or directory", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "import.markdown", "argument_required", "import markdown requires a source file or directory", "pinax import markdown <source> --vault <vault>")
		}
		projection, err := ctx.svc.ImportMarkdown(cmd.Context(), app.ImportMarkdownRequest{VaultPath: *ctx.vaultPath, Source: args[0], Group: *ctx.noteGroup, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteStatus, Tags: splitCSV(*ctx.noteTags), Conflict: *ctx.importConflict, DryRun: *ctx.importDryRun, Yes: *ctx.yes})
		return ctx.renderProjection(cmd, projection, err)
	}}
	importMarkdownCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Import target group")
	importMarkdownCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Import target folder")
	importMarkdownCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Import note kind")
	importMarkdownCmd.Flags().StringVar(ctx.noteStatus, "status", "", "Import note status")
	importMarkdownCmd.Flags().StringVar(ctx.noteTags, "tags", "", "Import note tags, comma-separated")
	importMarkdownCmd.Flags().StringVar(ctx.importConflict, "conflict", "skip", "Conflict strategy: skip, rename, or overwrite")
	importMarkdownCmd.Flags().BoolVar(ctx.importDryRun, "dry-run", false, "Only output the import plan; do not write the vault")
	importMarkdownCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm import writes")
	importCmd.AddCommand(importMarkdownCmd)
	root.AddCommand(importCmd)

	exportCmd := &cobra.Command{Use: "export", Short: "Export local Markdown content"}
	exportMarkdownCmd := &cobra.Command{Use: "markdown <output-dir>", Short: "Export a Markdown bundle", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, ctx.outputMode(), "export.markdown", "argument_required", "export markdown requires an output directory", "pinax export markdown <output-dir> --vault <vault>")
		}
		projection, err := ctx.svc.ExportMarkdown(cmd.Context(), app.ExportMarkdownRequest{VaultPath: *ctx.vaultPath, OutputDir: args[0], Tags: splitCSV(*ctx.noteListTag), Group: *ctx.noteGroup, Folder: *ctx.noteFolder, Kind: *ctx.noteKind, Status: *ctx.noteStatus})
		return ctx.renderProjection(cmd, projection, err)
	}}
	exportMarkdownCmd.Flags().StringVar(ctx.noteListTag, "tag", "", "Filter by tag")
	exportMarkdownCmd.Flags().StringVar(ctx.noteGroup, "group", "", "Filter by group")
	exportMarkdownCmd.Flags().StringVar(ctx.noteFolder, "folder", "", "Filter by folder")
	exportMarkdownCmd.Flags().StringVar(ctx.noteKind, "kind", "", "Filter by kind")
	exportMarkdownCmd.Flags().StringVar(ctx.noteStatus, "status", "", "Filter by status")
	exportCmd.AddCommand(exportMarkdownCmd)
	root.AddCommand(exportCmd)
}
