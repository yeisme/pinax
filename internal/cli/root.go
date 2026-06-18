package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	pinaxconfig "github.com/yeisme/pinax/internal/config"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/output"
	"github.com/yeisme/pinax/internal/remoteapi"
	"github.com/yeisme/pinax/internal/vaultregistry"
	"golang.org/x/term"
)

type Deps struct {
	Service *app.Service
	Version string
}

const rootHelpGroupAnnotation = "pinax.help.group"

type helpCommandGroup struct {
	Title    string
	Commands []*cobra.Command
}

const pinaxHelpTemplate = `{{with (or .Long .Short)}}Summary
  {{.}}

{{end}}{{if or .Runnable .HasSubCommands}}Usage
  {{.UseLine}}

{{end}}{{if .HasAvailableSubCommands}}{{if groupedCommandHelp .}}Available Commands
{{range groupedCommandHelp .}}{{.Title}}
{{range .Commands}}  {{rpad .Name $.NamePadding }} {{.Short}}
{{end}}
{{end}}{{else}}Available Commands
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}
{{end}}{{end}}{{if .HasAvailableLocalFlags}}Flags
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasAvailableInheritedFlags}}Global Flags
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasExample}}Examples
{{.Example}}

{{end}}{{if .HasSubCommands}}Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`

func NewRootCommand(version string) *cobra.Command {
	return NewRootCommandWithDeps(Deps{Version: version})
}

func NewRootCommandWithDeps(deps Deps) *cobra.Command {
	cobra.EnableCommandSorting = false
	svc := deps.Service
	if svc == nil {
		svc = app.NewService()
	}
	version := deps.Version
	if version == "" {
		version = "dev"
	}
	var jsonMode bool
	var agentMode bool
	var eventsMode bool
	var explainMode bool
	var vaultPath string
	var apiURL string
	var apiToken string
	var apiTokenFile string
	var colorMode string
	var themeName string
	var renderWidth int
	var markdownStyle string
	configResult := pinaxconfig.LoadResult{Config: pinaxconfig.DefaultConfig()}
	renderOptions := output.RenderOptions{}
	var yes bool
	var snapshotMessage string
	var title string
	var projectName string
	var projectDescription string
	var projectNotesPrefix string
	var storageRoot string
	var s3Bucket string
	var s3Region string
	var s3Prefix string
	var s3Endpoint string
	var s3Profile string
	var noteProject string
	var noteGroup string
	var noteFolder string
	var noteKind string
	var noteTags string
	var noteTemplate string
	var noteBody string
	var noteFrom string
	var noteDir string
	var noteSlug string
	var noteStatus string
	var noteUseStdin bool
	var noteDryRun bool
	var noteOpen bool
	var noteView string
	var noteDisplay string
	var noteRefreshRendered bool
	var noteSnapshot string
	var noteRuns bool
	var noteListTag string
	var noteListProject string
	var noteListStatus string
	var noteListSort string
	var noteListPathPrefix string
	var noteListProperties []string
	var noteStrictProperties bool
	var noteListCreatedAfter string
	var noteListUpdatedBefore string
	var noteRecent bool
	var noteLimit int
	var noteEditor string
	var noteHard bool
	var journalDate string
	var journalPrev bool
	var journalNext bool
	var templateSourcePath string
	var templateBody string
	var templateUseStdin bool
	var templateOverwrite bool
	var templateEngine string
	var templateSaveRun string
	var templateRun string
	var templateRuns bool
	var renderKeep int
	var renderDryRun bool
	var templateVars []string
	var queryLazyIndex bool
	var queryCursor string
	var databaseViewQuery string
	var databaseViewColumns []string
	var databaseSchemaType string
	var databaseSchemaValues string
	var syncTarget string
	var syncDryRun bool
	var syncBaseRevision string
	var syncRemoteRevision string
	var cloudEndpoint string
	var cloudWorkspace string
	var cloudDevice string
	var cloudSecretRef string
	var cloudEncryptionSecretRef string
	var staleAfter string
	var repairSave bool
	var repairPlanID string
	var organizeSave bool
	var searchLinkTarget string
	var searchHasAttachment bool
	var searchCreatedAfter string
	var searchUpdatedAfter string
	var searchAllowStale bool
	var searchAt string
	var searchChangedSince string
	var searchRevision string
	var searchIncludeDirty bool
	var importConflict string
	var importDryRun bool
	var dashboardPort int
	var backendName string
	var backendRoot string
	var backendRemote string
	var backendDryRun bool
	var planFromPeriod string
	var planWithTaskBridge bool
	var planDryRun bool
	var planSave bool
	var briefingTopic string
	var briefingSource string
	var briefingLimit int
	var briefingDryRun bool
	var feishuWebhook string
	var feishuSecretRef string
	var feishuTitle string
	var feishuText string
	var deliveryDryRun bool

	ctx := commandBuildContext{svc: svc, version: version, jsonMode: &jsonMode, agentMode: &agentMode, eventsMode: &eventsMode, explainMode: &explainMode, vaultPath: &vaultPath, apiURL: &apiURL, apiToken: &apiToken, apiTokenFile: &apiTokenFile, colorMode: &colorMode, themeName: &themeName, renderWidth: &renderWidth, markdownStyle: &markdownStyle, configResult: &configResult, renderOptions: &renderOptions, yes: &yes, snapshotMessage: &snapshotMessage, title: &title, projectName: &projectName, projectDescription: &projectDescription, projectNotesPrefix: &projectNotesPrefix, storageRoot: &storageRoot, s3Bucket: &s3Bucket, s3Region: &s3Region, s3Prefix: &s3Prefix, s3Endpoint: &s3Endpoint, s3Profile: &s3Profile, noteProject: &noteProject, noteGroup: &noteGroup, noteFolder: &noteFolder, noteKind: &noteKind, noteTags: &noteTags, noteTemplate: &noteTemplate, noteBody: &noteBody, noteFrom: &noteFrom, noteDir: &noteDir, noteSlug: &noteSlug, noteStatus: &noteStatus, noteUseStdin: &noteUseStdin, noteDryRun: &noteDryRun, noteOpen: &noteOpen, noteView: &noteView, noteDisplay: &noteDisplay, noteRefreshRendered: &noteRefreshRendered, noteSnapshot: &noteSnapshot, noteRuns: &noteRuns, noteListTag: &noteListTag, noteListProject: &noteListProject, noteListStatus: &noteListStatus, noteListSort: &noteListSort, noteListPathPrefix: &noteListPathPrefix, noteListProperties: &noteListProperties, noteStrictProperties: &noteStrictProperties, noteListCreatedAfter: &noteListCreatedAfter, noteListUpdatedBefore: &noteListUpdatedBefore, noteRecent: &noteRecent, noteLimit: &noteLimit, noteEditor: &noteEditor, noteHard: &noteHard, journalDate: &journalDate, journalPrev: &journalPrev, journalNext: &journalNext, templateSourcePath: &templateSourcePath, templateBody: &templateBody, templateUseStdin: &templateUseStdin, templateOverwrite: &templateOverwrite, templateEngine: &templateEngine, templateSaveRun: &templateSaveRun, templateRun: &templateRun, templateRuns: &templateRuns, renderKeep: &renderKeep, renderDryRun: &renderDryRun, templateVars: &templateVars, queryLazyIndex: &queryLazyIndex, queryCursor: &queryCursor, databaseViewQuery: &databaseViewQuery, databaseViewColumns: &databaseViewColumns, databaseSchemaType: &databaseSchemaType, databaseSchemaValues: &databaseSchemaValues, syncTarget: &syncTarget, syncDryRun: &syncDryRun, syncBaseRevision: &syncBaseRevision, syncRemoteRevision: &syncRemoteRevision, cloudEndpoint: &cloudEndpoint, cloudWorkspace: &cloudWorkspace, cloudDevice: &cloudDevice, cloudSecretRef: &cloudSecretRef, staleAfter: &staleAfter, repairSave: &repairSave, repairPlanID: &repairPlanID, organizeSave: &organizeSave, searchLinkTarget: &searchLinkTarget, searchHasAttachment: &searchHasAttachment, searchCreatedAfter: &searchCreatedAfter, searchUpdatedAfter: &searchUpdatedAfter, searchAllowStale: &searchAllowStale, searchAt: &searchAt, searchChangedSince: &searchChangedSince, searchRevision: &searchRevision, searchIncludeDirty: &searchIncludeDirty, importConflict: &importConflict, importDryRun: &importDryRun, dashboardPort: &dashboardPort, backendName: &backendName, backendRoot: &backendRoot, backendRemote: &backendRemote, backendDryRun: &backendDryRun, planFromPeriod: &planFromPeriod, planWithTaskBridge: &planWithTaskBridge, planDryRun: &planDryRun, planSave: &planSave, briefingTopic: &briefingTopic, briefingSource: &briefingSource, briefingLimit: &briefingLimit, briefingDryRun: &briefingDryRun, feishuWebhook: &feishuWebhook, feishuSecretRef: &feishuSecretRef, feishuTitle: &feishuTitle, feishuText: &feishuText, deliveryDryRun: &deliveryDryRun}

	cmd := &cobra.Command{
		Use:           "pinax",
		Short:         "Local-first Markdown vault notes CLI",
		Long:          "Pinax manages local Markdown vault notes, index projections, version evidence, and the local dashboard.",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "help" || cmd.CommandPath() == "pinax completion" {
				return nil
			}
			if err := validateOutputMode(cmd, jsonMode, agentMode, eventsMode, explainMode); err != nil {
				return err
			}
			return loadCommandConfig(cmd, &ctx)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.CompletionOptions.HiddenDefaultCmd = true
	cmd.PersistentFlags().BoolVar(&jsonMode, "json", false, "Emit a JSON envelope")
	cmd.PersistentFlags().BoolVar(&agentMode, "agent", false, "Emit agent key=value output")
	cmd.PersistentFlags().BoolVar(&eventsMode, "events", false, "Emit an NDJSON event stream")
	cmd.PersistentFlags().BoolVar(&explainMode, "explain", false, "Emit an English reviewable explanation")
	cmd.PersistentFlags().StringVar(&vaultPath, "vault", ".", "Pinax vault path")
	_ = cmd.RegisterFlagCompletionFunc("vault", vaultFlagCompletion)
	cmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Remote Pinax API URL; also PINAX_API_URL")
	cmd.PersistentFlags().StringVar(&apiToken, "api-token", "", "Remote Pinax API bearer token; prefer PINAX_API_TOKEN or --api-token-file")
	cmd.PersistentFlags().StringVar(&apiTokenFile, "api-token-file", "", "Read remote Pinax API bearer token from a file")
	cmd.PersistentFlags().StringVar(&colorMode, "color", "", "Human output color mode: auto, always, or never")
	cmd.PersistentFlags().StringVar(&themeName, "theme", "", "Human output theme: pinax, mono, high-contrast, or custom")
	cmd.PersistentFlags().IntVar(&renderWidth, "width", 0, "Human output width; 0 uses the configured default")
	cmd.PersistentFlags().StringVar(&markdownStyle, "markdown-style", "", "Markdown render style: auto, ascii, dark, light, or notty")
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "cli.flag", "flag_error", err.Error(), cmd.CommandPath()+" --help")
	})

	addConfigCommands(cmd, ctx)

	addVersionCommands(cmd, ctx)
	addAssetCommands(cmd, ctx)

	addVaultCommands(cmd, ctx)
	addRecordCommands(cmd, ctx)

	addJournalCommands(cmd, ctx)

	addInboxCommands(cmd, ctx)
	addDraftCommands(cmd, ctx)
	addDimensionRootCommands(cmd, ctx)
	addViewCommands(cmd, ctx)
	addFolderCommands(cmd, ctx)

	addNoteCommands(cmd, ctx)

	searchCmd := &cobra.Command{
		Use:     "search <query>",
		Short:   "Search local notes",
		Example: "pinax search \"project retrospective\" --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.search", "argument_required", "search requires a query", "pinax search <query> --vault <vault>")
			}
			projection, err := svc.SearchProjection(cmd.Context(), app.SearchRequest{VaultPath: vaultPath, Query: args[0], Tags: splitCSV(noteTags), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus, CreatedAfter: searchCreatedAfter, UpdatedAfter: searchUpdatedAfter, LinkTarget: searchLinkTarget, HasAttachment: searchHasAttachment, Limit: noteLimit, Sort: noteListSort, AllowStale: searchAllowStale, At: searchAt, IncludeDirty: searchIncludeDirty, ChangedSince: searchChangedSince, Revision: searchRevision})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	searchCmd.Flags().StringVar(&noteTags, "tag", "", "Filter by tag")
	searchCmd.Flags().StringVar(&noteGroup, "group", "", "Filter by group")
	searchCmd.Flags().StringVar(&noteFolder, "folder", "", "Filter by folder")
	searchCmd.Flags().StringVar(&noteKind, "kind", "", "Filter by kind")
	searchCmd.Flags().StringVar(&noteStatus, "status", "", "Filter by status")
	searchCmd.Flags().StringVar(&searchCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	searchCmd.Flags().StringVar(&searchUpdatedAfter, "updated-after", "", "Filter by minimum update date; format YYYY-MM-DD or RFC3339")
	searchCmd.Flags().StringVar(&searchLinkTarget, "link-target", "", "Filter by link target")
	searchCmd.Flags().BoolVar(&searchHasAttachment, "has-attachment", false, "Return only notes with attachment references")
	searchCmd.Flags().BoolVar(&searchAllowStale, "allow-stale", false, "Allow stale index partial results")
	searchCmd.Flags().StringVar(&searchAt, "at", "", "Read the specified projection through the version backend; currently supports HEAD")
	searchCmd.Flags().BoolVar(&searchIncludeDirty, "include-dirty", false, "Include dirty worktree content in version-aware search")
	searchCmd.Flags().StringVar(&searchChangedSince, "changed-since", "", "Filter to notes changed after the revision")
	searchCmd.Flags().StringVar(&searchRevision, "revision", "", "Read the historical projection for the specified revision")
	searchCmd.Flags().StringVar(&noteListSort, "sort", "", "Sort: relevance, updated, created, title, or path")
	_ = searchCmd.RegisterFlagCompletionFunc("sort", staticCompletion("sort", "relevance", "updated", "created", "title", "path"))
	searchCmd.Flags().IntVar(&noteLimit, "limit", 0, "Limit the number of results")
	cmd.AddCommand(searchCmd)

	queryCmd := &cobra.Command{
		Use:   "query",
		Short: "Query the local notes database",
		Long:  "Query the local notes database. Common workflow: pinax index status --vault ./my-notes, pinax query explain 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax query run 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax database view save active --query 'SELECT title FROM notes' --vault ./my-notes.",
	}
	queryRunCmd := &cobra.Command{Use: "run <sql>", Short: "Run a Pinax SQL query", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "query.run", "argument_required", "query run requires SQL", "pinax query run 'SELECT title FROM notes LIMIT 20' --vault <vault>")
		}
		projection, err := svc.QueryRun(cmd.Context(), app.QueryRequest{VaultPath: vaultPath, SQL: args[0], LazyIndex: queryLazyIndex, Limit: noteLimit, Sort: noteListSort, Cursor: queryCursor})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	queryRunCmd.Flags().StringVar(&noteListSort, "sort", "", "Sort by property")
	queryRunCmd.Flags().IntVar(&noteLimit, "limit", 0, "Limit the number of results")
	queryRunCmd.Flags().StringVar(&queryCursor, "cursor", "", "Pagination cursor")
	queryRunCmd.Flags().BoolVar(&queryLazyIndex, "lazy-index", false, "Allow explicit lazy index loading")
	_ = queryRunCmd.RegisterFlagCompletionFunc("sort", staticCompletion("property", "title", "updated_at", "created_at", "status", "path"))
	queryCmd.AddCommand(queryRunCmd)
	queryCmd.AddCommand(&cobra.Command{Use: "explain <sql>", Short: "Explain the Pinax SQL query plan", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "query.explain", "argument_required", "query explain requires SQL", "pinax query explain 'SELECT title FROM notes LIMIT 20' --vault <vault>")
		}
		projection, err := svc.QueryExplain(cmd.Context(), app.QueryRequest{VaultPath: vaultPath, SQL: args[0]})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	cmd.AddCommand(queryCmd)

	databaseCmd := &cobra.Command{Use: "database", Short: "Manage local notes database views", Long: "Manage local notes database views. Common workflow: pinax index status --vault ./my-notes, pinax query explain 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax query run 'SELECT title FROM notes LIMIT 20' --vault ./my-notes, pinax database view save active --query 'SELECT title FROM notes' --vault ./my-notes."}
	databaseViewCmd := &cobra.Command{Use: "view", Short: "Manage database views"}
	databaseViewSaveCmd := &cobra.Command{Use: "save <name>", Short: "Save a database view", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "database.view.save", "argument_required", "database view save requires a name", "pinax database view save <name> --vault <vault>")
		}
		var projection domain.Projection
		var err error
		if strings.TrimSpace(databaseViewQuery) != "" {
			projection, err = svc.SaveDatabaseView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0], Kind: noteKind, Query: databaseViewQuery, Columns: databaseViewColumns, Limit: noteLimit})
		} else {
			projection, err = svc.SaveView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0], Tags: splitCSV(noteListTag), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteListStatus, Sort: noteListSort, Limit: noteLimit, CreatedAfter: noteListCreatedAfter, UpdatedBefore: noteListUpdatedBefore})
			projection.Command = "database.view.save"
		}
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	databaseViewSaveCmd.Flags().StringVar(&noteListTag, "tag", "", "Filter by tags; comma-separated values are allowed")
	databaseViewSaveCmd.Flags().StringVar(&noteGroup, "group", "", "Filter by group")
	databaseViewSaveCmd.Flags().StringVar(&noteFolder, "folder", "", "Filter by folder")
	databaseViewSaveCmd.Flags().StringVar(&noteKind, "kind", "", "Filter by kind")
	databaseViewSaveCmd.Flags().StringVar(&noteListStatus, "status", "", "Filter by status")
	databaseViewSaveCmd.Flags().StringVar(&noteListCreatedAfter, "created-after", "", "Filter by minimum creation date; format YYYY-MM-DD or RFC3339")
	databaseViewSaveCmd.Flags().StringVar(&noteListUpdatedBefore, "updated-before", "", "Filter by maximum update date; format YYYY-MM-DD or RFC3339")
	databaseViewSaveCmd.Flags().StringVar(&noteListSort, "sort", "", "Sort: updated, path, or title")
	databaseViewSaveCmd.Flags().StringVar(&databaseViewQuery, "query", "", "Pinax SQL query")
	databaseViewSaveCmd.Flags().StringArrayVar(&databaseViewColumns, "column", nil, "Display columns; repeatable")
	databaseViewSaveCmd.Flags().IntVar(&noteLimit, "limit", 0, "Limit the number of results")
	databaseViewCmd.AddCommand(databaseViewSaveCmd)
	databaseViewCmd.AddCommand(&cobra.Command{Use: "list", Short: "List database views", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.ListViews(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		projection.Command = "database.view.list"
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	databaseViewCmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show a database view", ValidArgsFunction: savedViewCompletion(func() string { return vaultPath }), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "database.view.show", "argument_required", "database view show requires a name", "pinax database view show <name> --vault <vault>")
		}
		projection, err := svc.ShowDatabaseView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0]})
		projection.Command = "database.view.show"
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	databaseViewDeleteCmd := &cobra.Command{Use: "delete <name>", Short: "Delete a database view", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "database.view.delete", "argument_required", "database view delete requires a name", "pinax database view delete <name> --vault <vault> --yes")
		}
		projection, err := svc.DeleteView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0], Yes: yes})
		projection.Command = "database.view.delete"
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	databaseViewDeleteCmd.Flags().BoolVar(&yes, "yes", false, "Confirm database view deletion")
	databaseViewCmd.AddCommand(databaseViewDeleteCmd)
	databaseCmd.AddCommand(databaseViewCmd)
	databaseSchemaCmd := &cobra.Command{Use: "schema", Short: "Manage database property schema"}
	databaseSchemaCmd.AddCommand(&cobra.Command{Use: "infer", Short: "Infer property schema", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.DatabaseSchemaInfer(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	databaseSchemaSetCmd := &cobra.Command{Use: "set <property>", Short: "Set a property type", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "database.schema.set", "argument_required", "database schema set requires a property name", "pinax database schema set status --type select --vault <vault>")
		}
		projection, err := svc.DatabaseSchemaSet(cmd.Context(), app.DatabaseSchemaRequest{VaultPath: vaultPath, Name: args[0], Type: databaseSchemaType, Values: splitCSV(databaseSchemaValues)})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	databaseSchemaSetCmd.Flags().StringVar(&databaseSchemaType, "type", "", "Property type")
	databaseSchemaSetCmd.Flags().StringVar(&databaseSchemaValues, "values", "", "Allowed select/list values, comma-separated")
	_ = databaseSchemaSetCmd.RegisterFlagCompletionFunc("type", staticCompletion("type", "string", "number", "boolean", "date", "select", "list", "link"))
	databaseSchemaCmd.AddCommand(databaseSchemaSetCmd)
	databaseCmd.AddCommand(databaseSchemaCmd)
	cmd.AddCommand(databaseCmd)

	importCmd := &cobra.Command{Use: "import", Short: "Import local Markdown content"}
	importMarkdownCmd := &cobra.Command{Use: "markdown <source>", Short: "Import a local Markdown file or directory", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "import.markdown", "argument_required", "import markdown requires a source file or directory", "pinax import markdown <source> --vault <vault>")
		}
		projection, err := svc.ImportMarkdown(cmd.Context(), app.ImportMarkdownRequest{VaultPath: vaultPath, Source: args[0], Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus, Tags: splitCSV(noteTags), Conflict: importConflict, DryRun: importDryRun, Yes: yes})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	importMarkdownCmd.Flags().StringVar(&noteGroup, "group", "", "Import target group")
	importMarkdownCmd.Flags().StringVar(&noteFolder, "folder", "", "Import target folder")
	importMarkdownCmd.Flags().StringVar(&noteKind, "kind", "", "Import note kind")
	importMarkdownCmd.Flags().StringVar(&noteStatus, "status", "", "Import note status")
	importMarkdownCmd.Flags().StringVar(&noteTags, "tags", "", "Import note tags, comma-separated")
	importMarkdownCmd.Flags().StringVar(&importConflict, "conflict", "skip", "Conflict strategy: skip, rename, or overwrite")
	importMarkdownCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Only output the import plan; do not write the vault")
	importMarkdownCmd.Flags().BoolVar(&yes, "yes", false, "Confirm import writes")
	importCmd.AddCommand(importMarkdownCmd)
	cmd.AddCommand(importCmd)

	exportCmd := &cobra.Command{Use: "export", Short: "Export local Markdown content"}
	exportMarkdownCmd := &cobra.Command{Use: "markdown <output-dir>", Short: "Export a Markdown bundle", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "export.markdown", "argument_required", "export markdown requires an output directory", "pinax export markdown <output-dir> --vault <vault>")
		}
		projection, err := svc.ExportMarkdown(cmd.Context(), app.ExportMarkdownRequest{VaultPath: vaultPath, OutputDir: args[0], Tags: splitCSV(noteListTag), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	exportMarkdownCmd.Flags().StringVar(&noteListTag, "tag", "", "Filter by tag")
	exportMarkdownCmd.Flags().StringVar(&noteGroup, "group", "", "Filter by group")
	exportMarkdownCmd.Flags().StringVar(&noteFolder, "folder", "", "Filter by folder")
	exportMarkdownCmd.Flags().StringVar(&noteKind, "kind", "", "Filter by kind")
	exportMarkdownCmd.Flags().StringVar(&noteStatus, "status", "", "Filter by status")
	exportCmd.AddCommand(exportMarkdownCmd)
	cmd.AddCommand(exportCmd)

	addProjectCommands(cmd, ctx)

	addStorageCommands(cmd, ctx)
	addAPICommands(cmd, ctx)
	addProfileCommands(cmd, ctx)

	addTemplateCommands(cmd, ctx)

	addIndexCommands(cmd, ctx)

	briefingCmd := &cobra.Command{Use: "briefing", Short: "Manage daily hot-notes briefing"}
	briefingRecipeCmd := &cobra.Command{Use: "recipe", Short: "Manage briefing recipes"}
	briefingRecipeInitCmd := &cobra.Command{Use: "init", Short: "Create the default briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.BriefingRecipeInit(cmd.Context(), app.BriefingRecipeRequest{VaultPath: vaultPath, Topic: briefingTopic, Limit: briefingLimit})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	briefingRecipeInitCmd.Flags().StringVar(&briefingTopic, "topic", "", "briefing topic")
	briefingRecipeInitCmd.Flags().IntVar(&briefingLimit, "limit", 0, "Maximum number of candidates")
	briefingRecipeCmd.AddCommand(briefingRecipeInitCmd)
	briefingRecipeCmd.AddCommand(&cobra.Command{Use: "show", Short: "Show the briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.BriefingRecipeShow(cmd.Context(), app.BriefingRecipeRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	briefingRecipeSetCmd := &cobra.Command{Use: "set", Short: "Update the briefing recipe", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.BriefingRecipeSet(cmd.Context(), app.BriefingRecipeRequest{VaultPath: vaultPath, Topic: briefingTopic, Limit: briefingLimit, Source: briefingSource})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	briefingRecipeSetCmd.Flags().StringVar(&briefingTopic, "topic", "", "briefing topic")
	briefingRecipeSetCmd.Flags().IntVar(&briefingLimit, "limit", 0, "Maximum number of candidates")
	briefingRecipeSetCmd.Flags().StringVar(&briefingSource, "source", "", "New research source id")
	briefingRecipeCmd.AddCommand(briefingRecipeSetCmd)

	briefingDeliverCmd := &cobra.Command{Use: "deliver", Short: "Deliver a briefing"}
	feishuCmd := &cobra.Command{Use: "feishu", Short: "Deliver a briefing through a Feishu webhook", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.DeliverFeishu(cmd.Context(), app.FeishuDeliveryRequest{VaultPath: vaultPath, WebhookURL: feishuWebhook, SecretRef: feishuSecretRef, Title: feishuTitle, Text: feishuText, DryRun: deliveryDryRun, Yes: yes})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	feishuCmd.Flags().StringVar(&feishuWebhook, "webhook", "", "Feishu webhook URL")
	feishuCmd.Flags().StringVar(&feishuSecretRef, "secret-ref", "", "Webhook secret reference; do not output the raw value")
	feishuCmd.Flags().StringVar(&feishuTitle, "title", "", "Delivery title")
	feishuCmd.Flags().StringVar(&feishuText, "text", "", "Delivery text")
	feishuCmd.Flags().BoolVar(&deliveryDryRun, "dry-run", false, "Only generate a receipt preview; do not send the HTTP POST")
	feishuCmd.Flags().BoolVar(&yes, "yes", false, "Confirm sending the Feishu webhook")
	briefingDeliverCmd.AddCommand(feishuCmd)
	briefingCmd.AddCommand(briefingDeliverCmd)
	briefingCmd.AddCommand(briefingRecipeCmd)

	briefingRunCmd := &cobra.Command{Use: "run", Short: "Run the daily hot-notes briefing", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.BriefingRun(cmd.Context(), app.BriefingRunRequest{VaultPath: vaultPath, DryRun: briefingDryRun, Yes: yes})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}}
	briefingRunCmd.Flags().BoolVar(&briefingDryRun, "dry-run", false, "Only output candidates; do not write the vault or deliver")
	briefingRunCmd.Flags().BoolVar(&yes, "yes", false, "Confirm writing briefing candidate notes")
	briefingCmd.AddCommand(briefingRunCmd)
	cmd.AddCommand(briefingCmd)

	cloudCmd := &cobra.Command{Use: "cloud", Short: "Manage Pinax cloud sync state"}
	cloudLoginCmd := &cobra.Command{
		Use:     "login",
		Short:   "Configure Pinax cloud backend state",
		Example: "pinax cloud login --endpoint https://cloud.example.test --workspace ws_123 --device laptop --secret-ref op://pinax/cloud-token --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.CloudLogin(cmd.Context(), app.CloudLoginRequest{VaultPath: vaultPath, Endpoint: cloudEndpoint, WorkspaceID: cloudWorkspace, DeviceID: cloudDevice, SecretRef: cloudSecretRef, EncryptionSecretRef: cloudEncryptionSecretRef})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	cloudLoginCmd.Flags().StringVar(&cloudEndpoint, "endpoint", "", "Pinax cloud backend URL")
	cloudLoginCmd.Flags().StringVar(&cloudWorkspace, "workspace", "", "Pinax cloud workspace id")
	cloudLoginCmd.Flags().StringVar(&cloudDevice, "device", "", "Local device id")
	cloudLoginCmd.Flags().StringVar(&cloudSecretRef, "secret-ref", "", "Cloud auth token reference; do not save the raw token")
	cloudLoginCmd.Flags().StringVar(&cloudEncryptionSecretRef, "encryption-secret-ref", "", "Shared encryption secret reference; defaults to --secret-ref for old configs")
	cloudCmd.AddCommand(cloudLoginCmd)
	cloudBackendCmd := &cobra.Command{Use: "backend", Short: "Configure Cloud Sync transport backend"}
	cloudBackendSetCmd := &cobra.Command{Use: "set", Short: "Set Cloud Sync transport backend"}
	cloudBackendSetS3Cmd := &cobra.Command{
		Use:     "s3",
		Short:   "Configure S3-compatible direct Cloud Sync backend",
		Example: "pinax cloud backend set s3 --bucket notes --region us-east-1 --prefix pinax-sync/ --profile work --workspace personal --device laptop --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.CloudBackendSetS3(cmd.Context(), app.CloudBackendSetRequest{VaultPath: vaultPath, Kind: "s3", Bucket: s3Bucket, Region: s3Region, Prefix: s3Prefix, Endpoint: s3Endpoint, Profile: s3Profile, WorkspaceID: cloudWorkspace, DeviceID: cloudDevice, SecretRef: cloudSecretRef})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	cloudBackendSetS3Cmd.Flags().StringVar(&s3Bucket, "bucket", "", "S3 bucket name")
	cloudBackendSetS3Cmd.Flags().StringVar(&s3Region, "region", "", "S3 region")
	cloudBackendSetS3Cmd.Flags().StringVar(&s3Prefix, "prefix", "", "S3 object key prefix")
	cloudBackendSetS3Cmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	cloudBackendSetS3Cmd.Flags().StringVar(&s3Profile, "profile", "", "S3 credential profile name; do not save the secret")
	cloudBackendSetS3Cmd.Flags().StringVar(&cloudWorkspace, "workspace", "", "Cloud workspace id")
	cloudBackendSetS3Cmd.Flags().StringVar(&cloudDevice, "device", "", "Local device id")
	cloudBackendSetS3Cmd.Flags().StringVar(&cloudSecretRef, "secret-ref", "", "Secret manager reference; do not save the raw secret")
	cloudBackendSetRcloneCmd := &cobra.Command{
		Use:     "rclone",
		Short:   "Configure rclone direct Cloud Sync backend",
		Example: "pinax cloud backend set rclone --remote onedrive:PinaxSync --workspace personal --device laptop --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.CloudBackendSetRclone(cmd.Context(), app.CloudBackendSetRequest{VaultPath: vaultPath, Kind: "rclone", Remote: backendRemote, WorkspaceID: cloudWorkspace, DeviceID: cloudDevice})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	cloudBackendSetRcloneCmd.Flags().StringVar(&backendRemote, "remote", "", "Rclone remote and path, for example onedrive:PinaxSync")
	cloudBackendSetRcloneCmd.Flags().StringVar(&cloudWorkspace, "workspace", "", "Cloud workspace id")
	cloudBackendSetRcloneCmd.Flags().StringVar(&cloudDevice, "device", "", "Local device id")
	cloudBackendSetCmd.AddCommand(cloudBackendSetRcloneCmd)
	cloudBackendSetCmd.AddCommand(cloudBackendSetS3Cmd)
	cloudBackendCmd.AddCommand(cloudBackendSetCmd)
	cloudCmd.AddCommand(cloudBackendCmd)
	cloudCmd.AddCommand(&cobra.Command{Use: "status", Short: "Show Pinax cloud state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.CloudStatus(cmd.Context(), app.CloudRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	cloudCmd.AddCommand(&cobra.Command{Use: "logout", Short: "Log out the local cloud device session", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.CloudLogout(cmd.Context(), app.CloudRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	cloudCmd.AddCommand(&cobra.Command{Use: "doctor", Short: "Diagnose Pinax cloud state", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.CloudDoctor(cmd.Context(), app.CloudRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}})
	cmd.AddCommand(cloudCmd)

	addSyncCommands(cmd, ctx)

	addMetadataRepairOrganizeCommands(cmd, ctx)

	addGitCommands(cmd, ctx)

	addProofCommands(cmd, ctx)

	planCmd := &cobra.Command{Use: "plan", Short: "Manage personal planning workflows"}
	planDailyCmd := &cobra.Command{
		Use:   "daily",
		Short: "Generate a daily plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanDaily(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	planDailyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planDailyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planDailyCmd.Flags().BoolVar(&planSave, "save", false, "Save a plan snapshot")
	planDailyCmd.Flags().BoolVar(&yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planDailyCmd)
	planWeeklyCmd := &cobra.Command{
		Use:   "weekly",
		Short: "Generate a weekly plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanWeekly(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	planWeeklyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planWeeklyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planWeeklyCmd.Flags().BoolVar(&planSave, "save", false, "Save a plan snapshot")
	planWeeklyCmd.Flags().BoolVar(&yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planWeeklyCmd)
	planMonthlyCmd := &cobra.Command{
		Use:   "monthly",
		Short: "Generate a monthly plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanMonthly(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	planMonthlyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "Read task facts from TaskBridge")
	planMonthlyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "Preview the plan only; do not write")
	planMonthlyCmd.Flags().BoolVar(&planSave, "save", false, "Save a plan snapshot")
	planMonthlyCmd.Flags().BoolVar(&yes, "yes", false, "Confirm plan writes")
	planCmd.AddCommand(planMonthlyCmd)
	planActionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "Generate TaskBridge action drafts",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanActions(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, FromPeriod: planFromPeriod, Save: planSave})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	planActionsCmd.Flags().StringVar(&planFromPeriod, "from", "daily", "Source planning period: daily or weekly")
	planActionsCmd.Flags().BoolVar(&planSave, "save", false, "Save action drafts")
	planCmd.AddCommand(planActionsCmd)
	planSnapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Generate a plan snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanSnapshot(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	planCmd.AddCommand(planSnapshotCmd)
	cmd.AddCommand(planCmd)

	backendCmd := &cobra.Command{
		Use:   "backend",
		Short: "Manage vault backend providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.list", "argument_unexpected", "backend does not accept positional arguments", "pinax backend list --vault <vault>")
			}
			projection, err := svc.ListBackends(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendListRun := func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.list", "argument_unexpected", "backend list does not accept positional arguments", "pinax backend list --vault <vault>")
		}
		projection, err := svc.ListBackends(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
	}
	backendCmd.AddCommand(&cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all vault backends",
		RunE:    backendListRun,
	})
	backendAddCmd := &cobra.Command{
		Use:     "add <kind> <name>",
		Short:   "Add a backend profile",
		Example: "pinax backend add s3 work-s3 --bucket notes --region us-east-1 --vault ./my-notes\npinax backend add rclone work-drive --remote workdrive:pinax --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.add", "argument_required", "backend add requires a backend kind and name", "pinax backend add <kind> <name> --vault <vault>")
			}
			projection, err := svc.AddBackend(cmd.Context(), app.BackendAddRequest{VaultPath: vaultPath, Name: args[1], Kind: args[0], Root: backendRoot, Bucket: s3Bucket, Region: s3Region, Prefix: s3Prefix, Endpoint: s3Endpoint, Profile: s3Profile, Remote: backendRemote})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendAddCmd.Flags().StringVar(&backendRoot, "root", "", "Local backend root directory")
	backendAddCmd.Flags().StringVar(&s3Bucket, "bucket", "", "S3 bucket name")
	backendAddCmd.Flags().StringVar(&s3Region, "region", "", "S3 region")
	backendAddCmd.Flags().StringVar(&s3Prefix, "prefix", "", "S3 object key prefix")
	backendAddCmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3-compatible endpoint URL")
	backendAddCmd.Flags().StringVar(&s3Profile, "profile", "", "S3 credential profile name")
	backendAddCmd.Flags().StringVar(&backendRemote, "remote", "", "rclone remote path")
	backendCmd.AddCommand(backendAddCmd)
	backendShowCmd := &cobra.Command{
		Use:     "show <name>",
		Aliases: []string{"status"},
		Short:   "Show backend status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.show", "argument_required", "backend show requires a backend name", "pinax backend show <name> --vault <vault>")
			}
			projection, err := svc.BackendShow(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendCmd.AddCommand(backendShowCmd)
	backendDoctorCmd := &cobra.Command{
		Use:   "doctor <name>",
		Short: "Diagnose backend configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.doctor", "argument_required", "backend doctor requires a backend name", "pinax backend doctor <name> --vault <vault>")
			}
			projection, err := svc.BackendDoctor(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendCmd.AddCommand(backendDoctorCmd)
	backendCapabilitiesCmd := &cobra.Command{
		Use:   "capabilities <name>",
		Short: "Show backend capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.capabilities", "argument_required", "backend capabilities requires a backend name", "pinax backend capabilities <name> --vault <vault>")
			}
			projection, err := svc.BackendCapabilities(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendCmd.AddCommand(backendCapabilitiesCmd)
	backendDiffCmd := &cobra.Command{
		Use:   "diff <name>",
		Short: "Generate a backend dry-run sync plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.diff", "argument_required", "backend diff requires a backend name", "pinax backend diff <name> --vault <vault>")
			}
			projection, err := svc.BackendDiff(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendCmd.AddCommand(backendDiffCmd)
	backendPushCmd := &cobra.Command{
		Use:   "push <name>",
		Short: "Run backend push sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.push", "argument_required", "backend push requires a backend name", "pinax backend push <name> --vault <vault> --dry-run")
			}
			projection, err := svc.BackendPush(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: args[0], DryRun: backendDryRun, Yes: yes})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendPushCmd.Flags().BoolVar(&backendDryRun, "dry-run", false, "Preview the plan only; do not write")
	backendPushCmd.Flags().BoolVar(&yes, "yes", false, "Confirm writes")
	backendCmd.AddCommand(backendPushCmd)
	backendPullCmd := &cobra.Command{
		Use:   "pull <name>",
		Short: "Run backend pull sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.pull", "argument_required", "backend pull requires a backend name", "pinax backend pull <name> --vault <vault> --dry-run")
			}
			projection, err := svc.BackendPull(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: args[0], DryRun: backendDryRun, Yes: yes})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendPullCmd.Flags().BoolVar(&backendDryRun, "dry-run", false, "Preview the plan only; do not write")
	backendPullCmd.Flags().BoolVar(&yes, "yes", false, "Confirm writes")
	backendCmd.AddCommand(backendPullCmd)
	backendRemoveCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a backend profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.remove", "argument_required", "backend remove requires a backend name", "pinax backend remove <name> --vault <vault>")
			}
			projection, err := svc.RemoveBackend(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendCmd.AddCommand(backendRemoveCmd)
	backendObjectCmd := &cobra.Command{Use: "object", Short: "Browse backend objects"}
	backendObjectListCmd := &cobra.Command{
		Use:   "list <name> [prefix]",
		Short: "List backend objects",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.object.list", "argument_required", "backend object list requires a backend name", "pinax backend object list <name> [prefix] --vault <vault>")
			}
			prefix := ""
			if len(args) == 2 {
				prefix = args[1]
			}
			projection, err := svc.BackendObjectList(cmd.Context(), app.BackendObjectListRequest{VaultPath: vaultPath, Name: args[0], Prefix: prefix})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendObjectCmd.AddCommand(backendObjectListCmd)
	backendObjectStatCmd := &cobra.Command{
		Use:   "stat <name> <key>",
		Short: "Show backend object status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.object.stat", "argument_required", "backend object stat requires a backend name and key", "pinax backend object stat <name> <key> --vault <vault>")
			}
			projection, err := svc.BackendObjectStat(cmd.Context(), app.BackendObjectStatRequest{VaultPath: vaultPath, Name: args[0], Key: args[1]})
			return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), renderOptions, projection, err)
		},
	}
	backendObjectCmd.AddCommand(backendObjectStatCmd)
	backendCmd.AddCommand(backendObjectCmd)
	cmd.AddCommand(backendCmd)

	addMCPCommands(cmd, ctx)

	installRemoteMode(cmd, ctx)

	annotateRootHelpGroups(cmd)
	applyHelpTemplate(cmd)
	return cmd
}

func annotateRootHelpGroups(cmd *cobra.Command) {
	groups := map[string]string{
		"init":       "Local vault",
		"vault":      "Local vault",
		"project":    "Local vault",
		"record":     "Local vault",
		"note":       "Note workflows",
		"journal":    "Note workflows",
		"inbox":      "Note workflows",
		"template":   "Note workflows",
		"import":     "Note workflows",
		"export":     "Note workflows",
		"search":     "Organization and search",
		"view":       "Organization and search",
		"folder":     "Organization and search",
		"query":      "Organization and search",
		"database":   "Organization and search",
		"organize":   "Organization and search",
		"metadata":   "Organization and search",
		"repair":     "Organization and search",
		"plan":       "Automation and integrations",
		"briefing":   "Automation and integrations",
		"sync":       "Automation and integrations",
		"backend":    "Automation and integrations",
		"cloud":      "Automation and integrations",
		"mcp":        "Automation and integrations",
		"git":        "Automation and integrations",
		"config":     "Configuration and maintenance",
		"storage":    "Configuration and maintenance",
		"index":      "Configuration and maintenance",
		"asset":      "Configuration and maintenance",
		"version":    "Configuration and maintenance",
		"completion": "Configuration and maintenance",
	}
	for _, child := range cmd.Commands() {
		group, ok := groups[child.Name()]
		if !ok {
			continue
		}
		if child.Annotations == nil {
			child.Annotations = map[string]string{}
		}
		child.Annotations[rootHelpGroupAnnotation] = group
	}
}

func groupedCommandHelp(cmd *cobra.Command) []helpCommandGroup {
	if cmd.CommandPath() != "pinax" {
		return nil
	}
	order := []string{"Local vault", "Note workflows", "Organization and search", "Automation and integrations", "Configuration and maintenance"}
	groups := make(map[string][]*cobra.Command, len(order))
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		group := child.Annotations[rootHelpGroupAnnotation]
		if group == "" && child.Name() == "completion" {
			group = "Configuration and maintenance"
		}
		if group == "" {
			group = "Other"
		}
		groups[group] = append(groups[group], child)
	}
	result := make([]helpCommandGroup, 0, len(order))
	for _, title := range order {
		if len(groups[title]) == 0 {
			continue
		}
		result = append(result, helpCommandGroup{Title: title, Commands: groups[title]})
		delete(groups, title)
	}
	if len(groups["Other"]) > 0 {
		result = append(result, helpCommandGroup{Title: "Other", Commands: groups["Other"]})
	}
	return result
}

func applyHelpTemplate(cmd *cobra.Command) {
	cobra.AddTemplateFunc("groupedCommandHelp", groupedCommandHelp)
	applyHelpTemplateRecursive(cmd)
}

func applyHelpTemplateRecursive(cmd *cobra.Command) {
	cmd.SetHelpTemplate(pinaxHelpTemplate)
	for _, child := range cmd.Commands() {
		applyHelpTemplateRecursive(child)
	}
}

func loadCommandConfig(cmd *cobra.Command, ctx *commandBuildContext) error {
	registryPaths := vaultregistry.DefaultPaths()
	flags := explicitConfigFlags(cmd)
	initUsesCurrentDirDefault := cmd.CommandPath() == "pinax init" && flags["vault"] == ""
	if _, explicitVault := flags["vault"]; !initUsesCurrentDirDefault && !explicitVault && strings.TrimSpace(os.Getenv("PINAX_VAULT")) == "" && strings.TrimSpace(*ctx.vaultPath) == "." {
		userPaths := pinaxconfig.ResolvePaths(pinaxconfig.PathOptions{XDGConfigHome: os.Getenv("XDG_CONFIG_HOME")})
		userOnly, err := pinaxconfig.Load(pinaxconfig.LoadOptions{UserConfigPath: userPaths.User})
		if err == nil && strings.TrimSpace(userOnly.Config.Vault) != "" {
			*ctx.vaultPath = userOnly.Config.Vault
		} else if alias := vaultregistry.DefaultAlias(registryPaths); alias != "" {
			*ctx.vaultPath = alias
		}
	}
	if resolved, _, err := vaultregistry.ResolveSelector(registryPaths, *ctx.vaultPath); err == nil && !vaultregistry.IsRemoteSelector(resolved) {
		*ctx.vaultPath = resolved
	}
	paths := pinaxconfig.ResolvePaths(pinaxconfig.PathOptions{VaultPath: *ctx.vaultPath, XDGConfigHome: os.Getenv("XDG_CONFIG_HOME")})
	// Cobra/pflag defaults do not mean the user chose a value explicitly; do not blindly BindPFlag or flag defaults will override user/project config.
	result, err := pinaxconfig.Load(pinaxconfig.LoadOptions{VaultPath: *ctx.vaultPath, UserConfigPath: paths.User, ProjectConfigPath: paths.Project, ExplicitFlags: flags})
	if err != nil {
		return renderConfigError(cmd, ctx.outputMode(), err)
	}
	if initUsesCurrentDirDefault {
		result.Config.Vault = *ctx.vaultPath
	}
	if resolved, _, err := vaultregistry.ResolveSelector(registryPaths, result.Config.Vault); err == nil {
		if vaultregistry.IsRemoteSelector(resolved) {
			return renderCommandError(cmd, ctx.outputMode(), "vault.select", "remote_vault_readonly", "remote vault selectors are discovery-only and cannot be used as a local vault", "Run pinax vault remote list, then sync or register a local vault path")
		}
		result.Config.Vault = resolved
	}
	*ctx.configResult = result
	if result.Config.Vault != "" {
		*ctx.vaultPath = result.Config.Vault
	}
	*ctx.renderOptions = output.RenderOptions{ColorMode: result.Config.Output.Color, ThemeName: result.Config.Output.Theme, ThemeRoles: result.Config.Themes.Custom, Width: result.Config.Output.Width, Markdown: output.MarkdownOptions{Enabled: result.Config.Output.Markdown.Enabled, Style: result.Config.Output.Markdown.Style, Pager: result.Config.Output.Markdown.Pager}, IsTerminal: isTerminalIO(cmd)}
	return nil
}

func explicitConfigFlags(cmd *cobra.Command) map[string]string {
	flags := map[string]string{}
	add := func(flagName, key string) {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			flag = cmd.InheritedFlags().Lookup(flagName)
		}
		if flag != nil && flag.Changed {
			flags[key] = flag.Value.String()
		}
	}
	add("vault", "vault")
	add("api-url", "remote.api_url")
	add("color", "output.color")
	add("theme", "output.theme")
	add("width", "output.width")
	add("markdown-style", "output.markdown.style")
	return flags
}

func renderConfigError(cmd *cobra.Command, mode output.Mode, err error) error {
	code := pinaxconfig.ErrorCode(err)
	if code == "" {
		code = "config_error"
	}
	return renderCommandError(cmd, mode, "config.load", code, err.Error(), "Run pinax config doctor --vault <vault> to inspect config sources")
}

func parseDurationDays(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 90 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(value)
}

func selectedMode(jsonMode, agentMode, eventsMode, explainMode bool) output.Mode {
	switch {
	case jsonMode:
		return output.ModeJSON
	case agentMode:
		return output.ModeAgent
	case eventsMode:
		return output.ModeEvents
	case explainMode:
		return output.ModeExplain
	default:
		return output.ModeSummary
	}
}

func validateOutputMode(cmd *cobra.Command, jsonMode, agentMode, eventsMode, explainMode bool) error {
	selected := 0
	for _, enabled := range []bool{jsonMode, agentMode, eventsMode, explainMode} {
		if enabled {
			selected++
		}
	}
	if selected <= 1 {
		return nil
	}
	errMode := selectedMode(jsonMode, agentMode, eventsMode, explainMode)
	return renderCommandError(cmd, errMode, "cli.output_mode", "output_mode_conflict", "Choose only one output mode", "Keep only one output mode: --json, --agent, --events, or --explain")
}

func renderCommandError(cmd *cobra.Command, mode output.Mode, command, code, message, hint string) error {
	err := &domain.CommandError{Code: code, Message: message, Hint: hint}
	projection := domain.NewErrorProjection(command, err)
	projection.Actions = []domain.Action{{Name: "help", Command: hint}}
	return renderProjection(cmd.OutOrStdout(), mode, projection, err)
}

func splitKeyValueVars(values []string) (map[string]string, *domain.CommandError) {
	vars := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, &domain.CommandError{Code: "template_variable_invalid", Message: "Template variables must use key=value", Hint: "Use --var client=Acme"}
		}
		vars[key] = val
	}
	return vars, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (ctx commandBuildContext) renderProjection(cmd *cobra.Command, projection domain.Projection, err error) error {
	return renderProjectionWithOptions(cmd.OutOrStdout(), ctx.outputMode(), *ctx.renderOptions, projection, err)
}

func renderProjection(w io.Writer, mode output.Mode, projection domain.Projection, err error) error {
	return renderProjectionWithOptions(w, mode, output.RenderOptions{}, projection, err)
}

func installRemoteMode(root *cobra.Command, ctx commandBuildContext) {
	var wrap func(*cobra.Command)
	wrap = func(c *cobra.Command) {
		if c.RunE != nil {
			localRun := c.RunE
			c.RunE = func(cmd *cobra.Command, args []string) error {
				apiURL, source := remoteAPIURLSource(cmd, ctx)
				if apiURL == "" || remoteModeLocalCommand(cmd, source) {
					return localRun(cmd, args)
				}
				return runRemoteCommand(cmd, args, ctx)
			}
		}
		for _, child := range c.Commands() {
			wrap(child)
		}
	}
	wrap(root)
}

var errRemoteTokenConflict = errors.New("remote token source conflict")

func runRemoteCommand(cmd *cobra.Command, args []string, ctx commandBuildContext) error {
	if vaultFlag := cmd.Root().PersistentFlags().Lookup("vault"); vaultFlag != nil && vaultFlag.Changed {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_vault_conflict", "Remote API mode cannot be combined with an explicit --vault", "Remove --vault or omit --api-url")
	}
	rpc, ok := remoteRPCRequestForCommand(cmd, args)
	if !ok {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_command_unsupported", "Command is not supported by remote API mode", "Run pinax api routes to list supported remote commands")
	}
	token, err := remoteAPIToken(ctx)
	if errors.Is(err, errRemoteTokenConflict) {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_token_conflict", "Choose only one remote API token source", "Use only one of --api-token, --api-token-file, PINAX_API_TOKEN, or PINAX_API_TOKEN_FILE")
	}
	if err != nil {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_api_token_unreadable", "Remote API token file could not be read", "Check --api-token-file permissions")
	}
	client := remoteapi.NewClient(remoteapi.Config{BaseURL: remoteAPIURL(ctx), Token: token})
	projection, callErr := client.Call(cmd.Context(), rpc)
	return ctx.renderProjection(cmd, projection, callErr)
}

func remoteAPIURL(ctx commandBuildContext) string {
	url, _ := remoteAPIURLSource(nil, ctx)
	return url
}

func remoteAPIURLSource(cmd *cobra.Command, ctx commandBuildContext) (string, string) {
	if ctx.apiURL != nil && strings.TrimSpace(*ctx.apiURL) != "" {
		if cmd == nil {
			return strings.TrimSpace(*ctx.apiURL), "flag"
		}
		if flag := cmd.Root().PersistentFlags().Lookup("api-url"); flag != nil && flag.Changed {
			return strings.TrimSpace(*ctx.apiURL), "flag"
		}
	}
	if value := strings.TrimSpace(os.Getenv("PINAX_API_URL")); value != "" {
		return value, "env"
	}
	if ctx.configResult != nil {
		if value := strings.TrimSpace(ctx.configResult.Config.Remote.APIURL); value != "" {
			return value, "config"
		}
	}
	return "", ""
}

func remoteModeLocalCommand(cmd *cobra.Command, source string) bool {
	path := strings.TrimPrefix(cmd.CommandPath(), "pinax ")
	if path == "pinax" {
		return true
	}
	root, _, _ := strings.Cut(path, " ")
	if root == "cloud" || root == "sync" {
		return true
	}
	if source != "config" {
		return false
	}
	switch root {
	case "api", "config", "token", "profile", "vault", "completion", "help":
		return true
	default:
		return false
	}
}

func remoteAPIToken(ctx commandBuildContext) (string, error) {
	flagToken := ""
	if ctx.apiToken != nil {
		flagToken = strings.TrimSpace(*ctx.apiToken)
	}
	flagTokenFile := ""
	if ctx.apiTokenFile != nil {
		flagTokenFile = strings.TrimSpace(*ctx.apiTokenFile)
	}
	envToken := strings.TrimSpace(os.Getenv("PINAX_API_TOKEN"))
	envTokenFile := strings.TrimSpace(os.Getenv("PINAX_API_TOKEN_FILE"))
	sources := 0
	for _, value := range []string{flagToken, flagTokenFile, envToken, envTokenFile} {
		if value != "" {
			sources++
		}
	}
	if sources > 1 {
		return "", errRemoteTokenConflict
	}
	if flagToken != "" {
		return flagToken, nil
	}
	if flagTokenFile != "" {
		return readRemoteAPITokenFile(flagTokenFile)
	}
	if envToken != "" {
		return envToken, nil
	}
	if envTokenFile != "" {
		return readRemoteAPITokenFile(envTokenFile)
	}
	return "", nil
}

func readRemoteAPITokenFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func remoteRPCRequestForCommand(cmd *cobra.Command, args []string) (remoteapi.RPCRequest, bool) {
	params := map[string]any{}
	switch strings.TrimPrefix(cmd.CommandPath(), "pinax ") {
	case "folder list":
		params["purpose"] = stringFlag(cmd, "purpose")
		params["include_empty"] = boolFlag(cmd, "include-empty")
		params["depth"] = intFlag(cmd, "depth")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.List", Params: params}, true
	case "folder show":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Show", Params: params}, true
	case "folder create":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		params["purpose"] = stringFlag(cmd, "purpose")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Create", Params: params}, true
	case "folder rename":
		if len(args) != 2 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		params["target_path"] = args[1]
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Rename", Params: params}, true
	case "folder move":
		if len(args) != 2 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		params["target_parent"] = args[1]
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Move", Params: params}, true
	case "folder delete":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		params["empty_only"] = boolFlag(cmd, "empty-only")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Delete", Params: params}, true
	case "folder adopt":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["path"] = args[0]
		params["purpose"] = stringFlag(cmd, "purpose")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Folder.Adopt", Params: params}, true
	case "folder repair":
		return remoteapi.RPCRequest{Method: "Pinax.Folder.RepairPlan", Params: params}, true
	case "inbox list":
		return remoteapi.RPCRequest{Method: "Pinax.Inbox.List", Params: params}, true
	case "inbox show":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["display"] = stringFlag(cmd, "display")
		return remoteapi.RPCRequest{Method: "Pinax.Inbox.Show", Params: params}, true
	case "inbox capture":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["title"] = args[0]
		params["body"] = stringFlag(cmd, "body")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Inbox.Capture", Params: params}, true
	case "inbox promote":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["to"] = stringFlag(cmd, "to")
		params["group"] = stringFlag(cmd, "group")
		params["folder"] = stringFlag(cmd, "folder")
		params["kind"] = stringFlag(cmd, "kind")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Inbox.Promote", Params: params}, true
	case "inbox discard":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Inbox.Discard", Params: params}, true
	case "draft list":
		return remoteapi.RPCRequest{Method: "Pinax.Draft.List", Params: params}, true
	case "draft show":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["display"] = stringFlag(cmd, "display")
		return remoteapi.RPCRequest{Method: "Pinax.Draft.Show", Params: params}, true
	case "draft create":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["title"] = args[0]
		params["body"] = stringFlag(cmd, "body")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Draft.Create", Params: params}, true
	case "draft promote":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["status"] = stringFlag(cmd, "status")
		params["folder"] = stringFlag(cmd, "folder")
		params["kind"] = stringFlag(cmd, "kind")
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Draft.Promote", Params: params}, true
	case "draft archive":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Draft.Archive", Params: params}, true
	case "draft discard":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["dry_run"] = boolFlag(cmd, "dry-run")
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.Draft.Discard", Params: params}, true
	case "note list":
		params["tags"] = splitCSV(stringFlag(cmd, "tag"))
		params["project"] = stringFlag(cmd, "project")
		group := stringFlag(cmd, "group")
		if group == "" {
			group = stringFlag(cmd, "project")
		}
		params["group"] = group
		params["folder"] = stringFlag(cmd, "folder")
		params["kind"] = stringFlag(cmd, "kind")
		params["status"] = stringFlag(cmd, "status")
		params["created_after"] = stringFlag(cmd, "created-after")
		params["updated_before"] = stringFlag(cmd, "updated-before")
		params["recent"] = boolFlag(cmd, "recent")
		params["limit"] = intFlag(cmd, "limit")
		params["sort"] = stringFlag(cmd, "sort")
		params["path_prefix"] = stringFlag(cmd, "path-prefix")
		params["properties"] = stringArrayFlag(cmd, "property")
		params["strict_properties"] = boolFlag(cmd, "strict-properties")
		return remoteapi.RPCRequest{Method: "Pinax.Note.List", Params: params}, true
	case "note show", "note read", "note preview":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["ref"] = args[0]
		params["display"] = stringFlag(cmd, "display")
		return remoteapi.RPCRequest{Method: "Pinax.Note.Read", Params: params}, true
	case "project board show":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["project"] = args[0]
		params["note_display"] = stringFlag(cmd, "note-display")
		return remoteapi.RPCRequest{Method: "Pinax.ProjectBoard.Show", Params: params}, true
	case "project item move":
		if len(args) != 2 {
			return remoteapi.RPCRequest{}, false
		}
		params["item_id"] = args[0]
		params["column"] = args[1]
		params["action"] = "move"
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: params}, true
	case "project item archive":
		if len(args) != 1 {
			return remoteapi.RPCRequest{}, false
		}
		params["item_id"] = args[0]
		params["action"] = "archive"
		params["yes"] = boolFlag(cmd, "yes")
		return remoteapi.RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: params}, true
	default:
		return remoteapi.RPCRequest{}, false
	}
}

func stringFlag(cmd *cobra.Command, name string) string {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}

func boolFlag(cmd *cobra.Command, name string) bool {
	return stringFlag(cmd, name) == "true"
}

func intFlag(cmd *cobra.Command, name string) int {
	value := stringFlag(cmd, name)
	if value == "" {
		return 0
	}
	var parsed int
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func stringArrayFlag(cmd *cobra.Command, name string) []string {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return nil
	}
	if value, ok := flag.Value.(interface{ GetSlice() []string }); ok {
		return value.GetSlice()
	}
	return nil
}

func renderProjectionWithOptions(w io.Writer, mode output.Mode, opts output.RenderOptions, projection domain.Projection, err error) error {
	if renderErr := output.RenderWithOptions(w, mode, projection, opts); renderErr != nil {
		return renderErr
	}
	return err
}

func isTerminalIO(cmd *cobra.Command) bool {
	in, inOK := cmd.InOrStdin().(*os.File)
	out, outOK := cmd.OutOrStdout().(*os.File)
	return inOK && outOK && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

func isoWeekDate(key string) (time.Time, error) {
	var year int
	var week int
	if _, err := fmt.Sscanf(key, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()+6)%7)
	return monday.AddDate(0, 0, (week-1)*7), nil
}

func templateRenderRunCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		root := completionVaultRoot(vaultPathValue())
		items, err := renderRunCompletionItems(filepath.Join(root, ".pinax", "renders", "templates", args[0], "index.json"))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func noteRenderRunCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		root := completionVaultRoot(vaultPathValue())
		notePath, err := completionNotePath(root, args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		mirror := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(notePath), "notes/"), filepath.Ext(notePath))
		items, err := renderRunCompletionItems(filepath.Join(root, ".pinax", "renders", filepath.FromSlash(mirror), "index.json"))
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func renderRunCompletionItems(indexPath string) ([]string, error) {
	b, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Latest  string            `json:"latest"`
		Aliases map[string]string `json:"aliases"`
		Runs    []struct {
			RunID     string `json:"run_id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	items := []string{}
	if idx.Latest != "" {
		items = append(items, "latest\trender-run")
	}
	seen := map[string]bool{"latest": idx.Latest != ""}
	aliases := make([]string, 0, len(idx.Aliases))
	for alias := range idx.Aliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		if alias != "" && !seen[alias] {
			items = append(items, alias+"\trender-run")
			seen[alias] = true
		}
	}
	for _, run := range idx.Runs {
		if run.RunID != "" && !seen[run.RunID] {
			desc := "render-run"
			if run.CreatedAt != "" {
				desc = "render-run " + run.CreatedAt
			}
			items = append(items, run.RunID+"\t"+desc)
			seen[run.RunID] = true
		}
	}
	return items, nil
}

func completionNotePath(root, ref string) (string, error) {
	var matched string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || matched != "" {
			return nil
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "schema_version: pinax.note.v1") {
			return nil
		}
		stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		title := completionNoteTitle(rel, entry.Name(), string(b))
		if rel == ref || strings.TrimPrefix(rel, "notes/") == strings.TrimPrefix(ref, "notes/") || stem == ref || title == ref {
			matched = rel
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if matched == "" {
		return "", os.ErrNotExist
	}
	return matched, nil
}

func templateNameCompletion(vaultPathValue func() string, kind string, includeBuiltins, includeLocal bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Template completion reads metadata only: it does not render templates, execute SQL, or write the vault; it only returns names and source descriptions.
		root := completionVaultRoot(vaultPathValue())
		items := app.TemplateCompletionItems(root, kind, includeBuiltins, includeLocal)
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func templateVarCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		items := app.TemplateVariableCompletionItems(completionVaultRoot(vaultPathValue()), args[0])
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func staticCompletion(description string, values ...string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		items := make([]string, 0, len(values))
		for _, value := range values {
			if toComplete == "" || strings.HasPrefix(value, toComplete) {
				items = append(items, value+"\t"+description)
			}
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}
}

func savedViewCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		root := completionVaultRoot(vaultPathValue())
		items, err := savedViewCompletionItems(root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func noteRefCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		root := completionVaultRoot(vaultPathValue())
		items, err := noteCompletionItems(root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func assetRefCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		root := completionVaultRoot(vaultPathValue())
		items, err := assetCompletionItems(root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func assetCompletionItems(root string) ([]string, error) {
	assets, _, err := noteindex.ListAssets(root)
	if err != nil || len(assets) == 0 {
		manifest, manifestErr := pinaxassets.Load(root)
		if manifestErr == nil && len(manifest.Assets) > 0 {
			assets = manifest.Assets
		} else {
			assets = scanAssetCompletionAssets(root)
		}
	}
	linkedNotes := map[string]int{}
	if links, _, linkErr := noteindex.ListAssetLinks(root); linkErr == nil {
		seen := map[string]map[string]bool{}
		for _, link := range links {
			if link.Status != "resolved" || link.AssetPath == "" || link.SourcePath == "" {
				continue
			}
			if seen[link.AssetPath] == nil {
				seen[link.AssetPath] = map[string]bool{}
			}
			seen[link.AssetPath][link.SourcePath] = true
		}
		for path, sources := range seen {
			linkedNotes[path] = len(sources)
		}
	}
	seenItems := map[string]bool{}
	items := make([]string, 0, len(assets)*3)
	for _, asset := range assets {
		description := asset.MediaType
		if extType := mime.TypeByExtension("." + asset.Extension); extType != "" && !strings.Contains(description, "/") {
			description = extType
		}
		if description == "" {
			description = "asset"
		}
		linked := linkedNotes[asset.Path]
		description += " linked_notes=" + fmt.Sprint(linked)
		if linked == 0 {
			description += " orphan"
		}
		for _, value := range []string{asset.Filename, asset.Path, asset.Stem} {
			value = strings.TrimSpace(value)
			if value == "" || seenItems[value] {
				continue
			}
			seenItems[value] = true
			items = append(items, value+"\t"+description)
		}
	}
	sort.Strings(items)
	return items, nil
}

func scanAssetCompletionAssets(root string) []domain.Asset {
	assets := make([]domain.Asset, 0)
	for _, relRoot := range []string{"assets", "attachments"} {
		base := filepath.Join(root, relRoot)
		_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			ext := filepath.Ext(entry.Name())
			if strings.EqualFold(ext, ".md") {
				return nil
			}
			mediaType := mime.TypeByExtension(ext)
			if mediaType == "" {
				mediaType = "application/octet-stream"
			}
			assets = append(assets, domain.Asset{Path: rel, Filename: entry.Name(), Stem: strings.TrimSuffix(entry.Name(), ext), Extension: strings.TrimPrefix(ext, "."), MediaType: mediaType, ManagedStatus: domain.ManagedStatusUnmanaged})
			return nil
		})
	}
	return assets
}

func completionVaultRoot(vaultPath string) string {
	root := strings.TrimSpace(vaultPath)
	resolved, _, err := vaultregistry.ResolveSelector(vaultregistry.DefaultPaths(), root)
	if err == nil && resolved != "" && !vaultregistry.IsRemoteSelector(resolved) {
		return resolved
	}
	if root == "" {
		return "."
	}
	return root
}

func savedViewCompletionItems(root string) ([]string, error) {
	path := filepath.Join(root, ".pinax", "views.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var registry domain.SavedViewRegistry
	if err := json.Unmarshal(b, &registry); err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Views))
	for _, view := range registry.Views {
		if strings.TrimSpace(view.Name) != "" {
			items = append(items, view.Name+"\tview")
		}
	}
	sort.Strings(items)
	return items, nil
}

func noteCompletionItems(root string) ([]string, error) {
	items := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "schema_version: pinax.note.v1") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		title := completionNoteTitle(rel, entry.Name(), string(b))
		items = append(items, title+"\tnote")
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(items)
	return items, nil
}

func completionNoteTitle(rel, filename, content string) string {
	title := strings.TrimSuffix(filename, filepath.Ext(filename))
	if parsed := completionMarkdownTitle(content); parsed != "" {
		title = parsed
	}
	if alias := completionDailyShellFriendlyTitle(rel, title); alias != "" {
		return alias
	}
	return title
}

func completionDailyShellFriendlyTitle(rel, title string) string {
	path := filepath.ToSlash(strings.TrimPrefix(rel, "notes/"))
	if !strings.HasPrefix(path, "daily/") || filepath.Ext(path) != ".md" {
		return ""
	}
	key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if _, err := time.Parse("2006-01-02", key); err != nil {
		return ""
	}
	if title == "Daily "+key || title == "Daily-"+key {
		return "Daily-" + key
	}
	return ""
}

func completionMarkdownTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "title:")), "\"")
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func filterCompletionItems(items []string, toComplete string) []string {
	if toComplete == "" {
		return items
	}
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		value, _, _ := strings.Cut(item, "\t")
		if strings.HasPrefix(value, toComplete) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func vaultFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	items := make([]string, 0)
	registryItems, err := vaultregistry.CompletionItems(vaultregistry.DefaultPaths())
	if err == nil {
		items = append(items, registryItems...)
	}
	items = append(items, localDirectoryCompletionItems(toComplete)...)
	return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveDefault
}

func localDirectoryCompletionItems(toComplete string) []string {
	dir := "."
	prefix := toComplete
	if strings.ContainsAny(toComplete, `/\\`) {
		dir = filepath.Dir(toComplete)
		prefix = filepath.Base(toComplete)
		if dir == "" {
			dir = "."
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		candidate := entry.Name() + "/"
		if dir != "." {
			candidate = filepath.ToSlash(filepath.Join(dir, entry.Name())) + "/"
		}
		items = append(items, candidate+"\tlocal directory")
	}
	sort.Strings(items)
	return items
}

func journalDateCompletion(period string, vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		items, err := existingJournalDateCompletions(vaultPathValue(), period)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if toComplete == "" {
			return items, cobra.ShellCompDirectiveNoFileComp
		}
		filtered := make([]string, 0, len(items))
		for _, item := range items {
			value, _, _ := strings.Cut(item, "\t")
			if strings.HasPrefix(value, toComplete) {
				filtered = append(filtered, item)
			}
		}
		return filtered, cobra.ShellCompDirectiveNoFileComp
	}
}

func existingJournalDateCompletions(vaultPath, period string) ([]string, error) {
	root := strings.TrimSpace(vaultPath)
	if root == "" {
		root = "."
	}
	dir := filepath.Join(root, "notes", period)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".md")
		if !validJournalKey(period, key) || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, journalCompletionItem(period, key))
	}
	return items, nil
}

func validJournalKey(period, key string) bool {
	switch period {
	case "weekly":
		_, err := isoWeekDate(key)
		return err == nil
	case "monthly":
		_, err := time.Parse("2006-01", key)
		return err == nil
	default:
		_, err := time.Parse("2006-01-02", key)
		return err == nil
	}
}

func journalCompletionItem(period, key string) string {
	switch period {
	case "weekly":
		start, _ := isoWeekDate(key)
		_, week := start.ISOWeek()
		end := start.AddDate(0, 0, 6)
		return fmt.Sprintf("%s\tweek%d(%s--%s)", key, week, start.Format("2006-01-02"), end.Format("2006-01-02"))
	case "monthly":
		start, _ := time.Parse("2006-01", key)
		end := start.AddDate(0, 1, -1)
		return fmt.Sprintf("%s\t%s--%s", key, start.Format("2006-01-02"), end.Format("2006-01-02"))
	default:
		return key + "\t" + key
	}
}
