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
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/yeisme/pinax/internal/app"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	pinaxconfig "github.com/yeisme/pinax/internal/config"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/output"
	"github.com/yeisme/pinax/internal/remoteapi"
	"github.com/yeisme/pinax/internal/vaultregistry"
	"github.com/yeisme/pinax/pkg/pinaxclient"
	"golang.org/x/term"
)

type Deps struct {
	Service *app.Service
	Version string
}

const (
	rootHelpGroupAnnotation      = "pinax.help.group"
	rootHelpVisibilityAnnotation = "pinax.help.visibility"
	rootHelpVisibilityCore       = "core"
	rootHelpVisibilityAdvanced   = "advanced"
)

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
{{end}}{{end}}{{with helpLocalFlagUsages .}}Flags
{{. | trimTrailingWhitespaces}}

{{end}}{{with helpInheritedFlagUsages .}}Global Flags
{{. | trimTrailingWhitespaces}}

{{end}}{{if .HasExample}}Examples
{{.Example}}

{{end}}{{if .HasSubCommands}}Use "{{.CommandPath}} [command] --help" for more information about a command.
{{if eq .CommandPath "pinax"}}Run "pinax commands" to see the complete command catalog.
{{end}}
{{end}}`

func NewRootCommand(version string) *cobra.Command {
	return NewRootCommandWithDeps(Deps{Version: version})
}

func init() {
	cobra.EnableCommandSorting = false
}

func NewRootCommandWithDeps(deps Deps) *cobra.Command {
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
	var connectionMode string
	var apiToken string
	var apiTokenFile string
	var colorMode string
	var outputStyle string
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
	var s3AddressingStyle string
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
	var noteVerifyActor string
	var noteVerifyNote string
	var metadataTrustFields bool
	var metadataTrustStaleAfter string
	var searchTrust string
	var searchStale string
	var searchFacets bool
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
	var databaseViewLanguage string
	var databaseViewDisplay string
	var databaseViewGroupBy string
	var databaseViewCalendar string
	var databaseViewBoardColumn string
	var databaseSchemaType string
	var databaseSchemaValues string
	var syncTarget string
	var syncDryRun bool
	var syncBaseRevision string
	var syncRemoteRevision string
	var syncPreview string
	var syncLimit int
	var syncLimitSet bool
	var syncContentDiff bool
	var syncProgress string
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
	var searchEngine string
	var searchLazyIndex string
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
	var planFromPeriod string
	var planTaskReview bool
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

	ctx := commandBuildContext{svc: svc, version: version, jsonMode: &jsonMode, agentMode: &agentMode, eventsMode: &eventsMode, explainMode: &explainMode, vaultPath: &vaultPath, apiURL: &apiURL, connectionMode: &connectionMode, apiToken: &apiToken, apiTokenFile: &apiTokenFile, colorMode: &colorMode, outputStyle: &outputStyle, themeName: &themeName, renderWidth: &renderWidth, markdownStyle: &markdownStyle, configResult: &configResult, renderOptions: &renderOptions, yes: &yes, snapshotMessage: &snapshotMessage, title: &title, projectName: &projectName, projectDescription: &projectDescription, projectNotesPrefix: &projectNotesPrefix, storageRoot: &storageRoot, s3Bucket: &s3Bucket, s3Region: &s3Region, s3Prefix: &s3Prefix, s3Endpoint: &s3Endpoint, s3Profile: &s3Profile, s3AddressingStyle: &s3AddressingStyle, noteProject: &noteProject, noteGroup: &noteGroup, noteFolder: &noteFolder, noteKind: &noteKind, noteTags: &noteTags, noteTemplate: &noteTemplate, noteBody: &noteBody, noteFrom: &noteFrom, noteDir: &noteDir, noteSlug: &noteSlug, noteStatus: &noteStatus, noteUseStdin: &noteUseStdin, noteDryRun: &noteDryRun, noteOpen: &noteOpen, noteView: &noteView, noteDisplay: &noteDisplay, noteRefreshRendered: &noteRefreshRendered, noteSnapshot: &noteSnapshot, noteRuns: &noteRuns, noteListTag: &noteListTag, noteListProject: &noteListProject, noteListStatus: &noteListStatus, noteListSort: &noteListSort, noteListPathPrefix: &noteListPathPrefix, noteListProperties: &noteListProperties, noteVerifyActor: &noteVerifyActor, noteVerifyNote: &noteVerifyNote, metadataTrustFields: &metadataTrustFields, metadataTrustStaleAfter: &metadataTrustStaleAfter, searchTrust: &searchTrust, searchStale: &searchStale, searchFacets: &searchFacets, noteStrictProperties: &noteStrictProperties, noteListCreatedAfter: &noteListCreatedAfter, noteListUpdatedBefore: &noteListUpdatedBefore, noteRecent: &noteRecent, noteLimit: &noteLimit, noteEditor: &noteEditor, noteHard: &noteHard, journalDate: &journalDate, journalPrev: &journalPrev, journalNext: &journalNext, templateSourcePath: &templateSourcePath, templateBody: &templateBody, templateUseStdin: &templateUseStdin, templateOverwrite: &templateOverwrite, templateEngine: &templateEngine, templateSaveRun: &templateSaveRun, templateRun: &templateRun, templateRuns: &templateRuns, renderKeep: &renderKeep, renderDryRun: &renderDryRun, templateVars: &templateVars, queryLazyIndex: &queryLazyIndex, queryCursor: &queryCursor, databaseViewQuery: &databaseViewQuery, databaseViewColumns: &databaseViewColumns, databaseViewLanguage: &databaseViewLanguage, databaseViewDisplay: &databaseViewDisplay, databaseViewGroupBy: &databaseViewGroupBy, databaseViewCalendar: &databaseViewCalendar, databaseViewBoardColumn: &databaseViewBoardColumn, databaseSchemaType: &databaseSchemaType, databaseSchemaValues: &databaseSchemaValues, syncTarget: &syncTarget, syncDryRun: &syncDryRun, syncBaseRevision: &syncBaseRevision, syncRemoteRevision: &syncRemoteRevision, syncPreview: &syncPreview, syncLimit: &syncLimit, syncLimitSet: &syncLimitSet, syncContentDiff: &syncContentDiff, syncProgress: &syncProgress, cloudEndpoint: &cloudEndpoint, cloudWorkspace: &cloudWorkspace, cloudDevice: &cloudDevice, cloudSecretRef: &cloudSecretRef, cloudEncryptionSecretRef: &cloudEncryptionSecretRef, staleAfter: &staleAfter, repairSave: &repairSave, repairPlanID: &repairPlanID, organizeSave: &organizeSave, searchLinkTarget: &searchLinkTarget, searchHasAttachment: &searchHasAttachment, searchCreatedAfter: &searchCreatedAfter, searchUpdatedAfter: &searchUpdatedAfter, searchAllowStale: &searchAllowStale, searchEngine: &searchEngine, searchLazyIndex: &searchLazyIndex, searchAt: &searchAt, searchChangedSince: &searchChangedSince, searchRevision: &searchRevision, searchIncludeDirty: &searchIncludeDirty, importConflict: &importConflict, importDryRun: &importDryRun, dashboardPort: &dashboardPort, backendName: &backendName, backendRoot: &backendRoot, backendRemote: &backendRemote, planFromPeriod: &planFromPeriod, planTaskReview: &planTaskReview, planDryRun: &planDryRun, planSave: &planSave, briefingTopic: &briefingTopic, briefingSource: &briefingSource, briefingLimit: &briefingLimit, briefingDryRun: &briefingDryRun, feishuWebhook: &feishuWebhook, feishuSecretRef: &feishuSecretRef, feishuTitle: &feishuTitle, feishuText: &feishuText, deliveryDryRun: &deliveryDryRun}

	cmd := &cobra.Command{
		Use:           "pinax",
		Short:         "Personal local knowledge CLI for Markdown",
		Long:          "Pinax helps one person capture, find, organize, and protect a local Markdown knowledge base. Run pinax commands when you need optional advanced integrations.",
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
	cmd.PersistentFlags().StringVar(&connectionMode, "connection-mode", "", "Connection owner mode: local-vault, remote-service, or self-hosted-service")
	cmd.PersistentFlags().StringVar(&apiToken, "api-token", "", "Remote Pinax API bearer token; prefer PINAX_API_TOKEN or --api-token-file")
	cmd.PersistentFlags().StringVar(&apiTokenFile, "api-token-file", "", "Read remote Pinax API bearer token from a file")
	cmd.PersistentFlags().StringVar(&colorMode, "color", "", "Human output color mode: auto, always, or never")
	cmd.PersistentFlags().StringVar(&outputStyle, "output-style", "", "Human output layout: table or compact")
	cmd.PersistentFlags().StringVar(&themeName, "theme", "", "Human output theme: pinax, mono, high-contrast, or custom")
	cmd.PersistentFlags().IntVar(&renderWidth, "width", 0, "Human output width; 0 uses the configured default")
	cmd.PersistentFlags().StringVar(&markdownStyle, "markdown-style", "", "Markdown render style: auto, ascii, dark, light, or notty")
	_ = cmd.RegisterFlagCompletionFunc("color", staticCompletion("color", "auto", "always", "never"))
	_ = cmd.RegisterFlagCompletionFunc("output-style", staticCompletion("output-style", "table", "compact"))
	_ = cmd.RegisterFlagCompletionFunc("theme", staticCompletion("theme", "pinax", "mono", "high-contrast", "custom"))
	_ = cmd.RegisterFlagCompletionFunc("markdown-style", staticCompletion("markdown-style", "auto", "ascii", "dark", "light", "notty"))
	_ = cmd.RegisterFlagCompletionFunc("connection-mode", staticCompletion("connection-mode", "local-vault", "remote-service", "self-hosted-service"))
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "cli.flag", "flag_error", err.Error(), cmd.CommandPath()+" --help")
	})

	addConfigCommands(cmd, ctx)
	addConnectionCommands(cmd, ctx)
	addOperationCommands(cmd, ctx)

	addVersionCommands(cmd, ctx)
	addBackupCommands(cmd, ctx)
	addAssetCommands(cmd, ctx)
	addPromptCommands(cmd, ctx)
	addCollectionCommands(cmd, ctx)
	addGraphCommands(cmd, ctx)
	addPublishCommands(cmd, ctx)
	addShareCommands(cmd, ctx)
	addPluginCommands(cmd, ctx)

	addVaultCommands(cmd, ctx)
	addRecordCommands(cmd, ctx)
	addActivityCommands(cmd, ctx)
	addMonitorCommands(cmd, ctx)

	addJournalCommands(cmd, ctx)

	addInboxCommands(cmd, ctx)
	addDraftCommands(cmd, ctx)
	addDimensionRootCommands(cmd, ctx)
	addViewCommands(cmd, ctx)
	addFolderCommands(cmd, ctx)

	addNoteCommands(cmd, ctx)

	addSearchCommand(cmd, ctx)
	addBrowseCommand(cmd, ctx)
	addMemoryCommands(cmd, ctx)
	addBrainCommands(cmd, ctx)
	addAgentCommands(cmd, ctx)
	addContinueCommands(cmd, ctx)
	addReviewCommands(cmd, ctx)
	addQueryCommands(cmd, ctx)
	addDataviewCommands(cmd, ctx)
	addDatabaseCommands(cmd, ctx)
	addImportExportCommands(cmd, ctx)
	addKnowledgeCommands(cmd, ctx)

	addProjectCommands(cmd, ctx)
	addTrashCommands(cmd, ctx)
	addTaskCommands(cmd, ctx)

	addStorageCommands(cmd, ctx)
	addAPICommands(cmd, ctx)
	addProfileCommands(cmd, ctx)

	addTemplateCommands(cmd, ctx)

	addIndexCommands(cmd, ctx)

	addBriefingCommands(cmd, ctx)
	addCapsaCommands(cmd, ctx)

	addSyncCommands(cmd, ctx)

	addMetadataRepairOrganizeCommands(cmd, ctx)

	addPipelineCommands(cmd, ctx)

	addProofCommands(cmd, ctx)

	addPlanningCommands(cmd, ctx)
	addBackendCommands(cmd, ctx)

	addMCPCommands(cmd, ctx)
	addCommandsCommand(cmd, ctx)

	installRemoteMode(cmd, ctx)

	annotateRootHelpGroups(cmd)
	annotateRootHelpVisibility(cmd)
	applyHelpTemplate(cmd)
	return cmd
}

func annotateRootHelpGroups(cmd *cobra.Command) {
	groups := map[string]string{
		"init":       "Start here",
		"vault":      "Start here",
		"project":    "Find and organize",
		"trash":      "Local vault",
		"task":       "Local vault",
		"record":     "Local vault",
		"activity":   "Configuration and maintenance",
		"note":       "Capture and write",
		"journal":    "Capture and write",
		"inbox":      "Capture and write",
		"draft":      "Note workflows",
		"template":   "Note workflows",
		"import":     "Note workflows",
		"export":     "Note workflows",
		"knowledge":  "Automation and integrations",
		"search":     "Find and organize",
		"memory":     "Organization and search",
		"brain":      "Organization and search",
		"agent":      "Organization and search",
		"continue":   "Organization and search",
		"review":     "Organization and search",
		"graph":      "Organization and search",
		"dataview":   "Organization and search",
		"proof":      "Organization and search",
		"view":       "Organization and search",
		"folder":     "Organization and search",
		"query":      "Organization and search",
		"database":   "Organization and search",
		"organize":   "Organization and search",
		"metadata":   "Organization and search",
		"repair":     "Organization and search",
		"pipeline":   "Organization and search",
		"plan":       "Automation and integrations",
		"prompt":     "Automation and integrations",
		"collection": "Automation and integrations",
		"briefing":   "Automation and integrations",
		"capsa":      "Automation and integrations",
		"sync":       "Automation and integrations",
		"backend":    "Automation and integrations",
		"cloud":      "Automation and integrations",
		"publish":    "Automation and integrations",
		"share":      "Automation and integrations",
		"plugin":     "Automation and integrations",
		"api":        "Automation and integrations",
		"token":      "Automation and integrations",
		"profile":    "Automation and integrations",
		"mcp":        "Automation and integrations",
		"git":        "Automation and integrations",
		"config":     "Configuration and maintenance",
		"connection": "Configuration and maintenance",
		"monitor":    "Configuration and maintenance",
		"storage":    "Configuration and maintenance",
		"index":      "Configuration and maintenance",
		"asset":      "Configuration and maintenance",
		"backup":     "Local safety",
		"version":    "Configuration and maintenance",
		"commands":   "More commands",
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

func annotateRootHelpVisibility(cmd *cobra.Command) {
	core := map[string]bool{
		"init": true, "vault": true, "note": true, "inbox": true, "journal": true,
		"search": true, "project": true, "backup": true, "commands": true,
	}
	for _, child := range cmd.Commands() {
		if child.Annotations == nil {
			child.Annotations = map[string]string{}
		}
		visibility := rootHelpVisibilityAdvanced
		if core[child.Name()] {
			visibility = rootHelpVisibilityCore
		}
		child.Annotations[rootHelpVisibilityAnnotation] = visibility
	}
}

func groupedCommandHelp(cmd *cobra.Command) []helpCommandGroup {
	if cmd.CommandPath() != "pinax" {
		return nil
	}
	order := []string{"Start here", "Capture and write", "Find and organize", "Local safety", "More commands"}
	groups := make(map[string][]*cobra.Command, len(order))
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() || child.Annotations[rootHelpVisibilityAnnotation] != rootHelpVisibilityCore {
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

// helpTemplateFuncsOnce guards cobra.AddTemplateFunc: it mutates a package
// global template-func map, so concurrent root-command construction (parallel
// tests, in-process API servers) crashes with concurrent map writes.
var helpTemplateFuncsOnce sync.Once

func applyHelpTemplate(cmd *cobra.Command) {
	helpTemplateFuncsOnce.Do(func() {
		cobra.AddTemplateFunc("groupedCommandHelp", groupedCommandHelp)
		cobra.AddTemplateFunc("helpLocalFlagUsages", helpLocalFlagUsages)
		cobra.AddTemplateFunc("helpInheritedFlagUsages", helpInheritedFlagUsages)
	})
	applyHelpTemplateRecursive(cmd)
}

func helpLocalFlagUsages(cmd *cobra.Command) string {
	flags := cmd.LocalFlags()
	if cmd.CommandPath() == "pinax" {
		return personalFlagUsages(flags)
	}
	return flags.FlagUsages()
}

func helpInheritedFlagUsages(cmd *cobra.Command) string {
	flags := cmd.InheritedFlags()
	if isPersonalHelpCommand(cmd) {
		return personalFlagUsages(flags)
	}
	return flags.FlagUsages()
}

func isPersonalHelpCommand(cmd *cobra.Command) bool {
	if cmd == nil || cmd.CommandPath() == "pinax" {
		return true
	}
	top := cmd
	for top.Parent() != nil && top.Parent() != cmd.Root() {
		top = top.Parent()
	}
	return top.Annotations[rootHelpVisibilityAnnotation] == rootHelpVisibilityCore
}

func personalFlagUsages(flags *pflag.FlagSet) string {
	allowed := map[string]bool{
		"agent": true, "events": true, "explain": true, "help": true, "json": true, "output-style": true, "vault": true,
	}
	filtered := pflag.NewFlagSet("personal-help", pflag.ContinueOnError)
	flags.VisitAll(func(flag *pflag.Flag) {
		if allowed[flag.Name] {
			filtered.AddFlag(flag)
		}
	})
	return filtered.FlagUsages()
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
	syncPreview := strings.TrimSpace(*ctx.syncPreview)
	if syncPreview == "" {
		syncPreview = "status"
	}
	*ctx.renderOptions = output.RenderOptions{Style: result.Config.Output.Style, ColorMode: result.Config.Output.Color, ThemeName: result.Config.Output.Theme, ThemeRoles: result.Config.Themes.Custom, Width: result.Config.Output.Width, Markdown: output.MarkdownOptions{Enabled: result.Config.Output.Markdown.Enabled, Style: result.Config.Output.Markdown.Style, Pager: result.Config.Output.Markdown.Pager}, IsTerminal: isTerminalIO(cmd), SyncPreview: syncPreview, SyncLimit: *ctx.syncLimit, SyncLimitSet: *ctx.syncLimitSet, ContentDiff: *ctx.syncContentDiff}
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
	add("connection-mode", "remote.mode")
	add("color", "output.color")
	add("output-style", "output.style")
	if cmd.CommandPath() != "pinax publish profile init" {
		add("theme", "output.theme")
	}
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
	if remoteRPCMutationMethod(rpc.Method) && !remoteRPCBool(rpc.Params, "dry_run") {
		operationID := strings.TrimSpace(remoteRPCString(rpc.Params, "operation_id"))
		idempotencyKey := strings.TrimSpace(remoteRPCString(rpc.Params, "idempotency_key"))
		if (operationID == "") != (idempotencyKey == "") {
			return renderCommandError(cmd, ctx.outputMode(), remoteRPCMutationCommand(rpc.Method), "operation_identity_required", "Remote mutation identity is incomplete", "Provide both --operation-id and --idempotency-key, or omit both to generate a new pair")
		}
	}
	token, err := remoteAPIToken(ctx)
	if errors.Is(err, errRemoteTokenConflict) {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_token_conflict", "Choose only one remote API token source", "Use only one of --api-token, --api-token-file, PINAX_API_TOKEN, or PINAX_API_TOKEN_FILE")
	}
	if err != nil {
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", "remote_api_token_unreadable", "Remote API token file could not be read", "Check --api-token-file permissions")
	}
	descriptor := resolvedConnectionDescriptor(cmd, ctx)
	if descriptor.Status == "blocked" {
		code, message, hint := connectionBlockerError(descriptor.Blockers)
		return renderCommandError(cmd, ctx.outputMode(), "remote.api", code, message, hint)
	}
	client, clientErr := pinaxclient.New(pinaxclient.Config{BaseURL: remoteAPIURL(ctx), Token: token})
	if clientErr != nil {
		projection, callErr := remoteapi.Adapt(pinaxclient.Projection{}, clientErr)
		return ctx.renderProjection(cmd, projection, callErr)
	}
	if rpc.Method == "Pinax.Folder.Rename" && !remoteRPCBool(rpc.Params, "dry_run") && strings.TrimSpace(remoteRPCString(rpc.Params, "expected_revision")) == "" {
		preview := cloneRemoteRPCRequest(rpc)
		preview.Params["dry_run"] = true
		preview.Params["yes"] = false
		preview.Params["operation_id"] = ""
		preview.Params["idempotency_key"] = ""
		previewProjection, previewErr := client.CallRPC(cmd.Context(), preview)
		if previewErr != nil {
			projection, callErr := remoteapi.Adapt(previewProjection, previewErr)
			return ctx.renderProjection(cmd, projection, callErr)
		}
		expectedRevision := strings.TrimSpace(previewProjection.Facts["revision_before"])
		if expectedRevision == "" {
			return renderCommandError(cmd, ctx.outputMode(), "folder.rename", "upstream_invalid_response", "Folder rename preflight returned no revision", "Retry the dry-run and inspect the owner contract")
		}
		rpc.Params["expected_revision"] = expectedRevision
	}
	if remoteRPCMutationMethod(rpc.Method) && !remoteRPCBool(rpc.Params, "dry_run") && strings.TrimSpace(remoteRPCString(rpc.Params, "operation_id")) == "" {
		identity, identityErr := pinaxclient.NewMutationIdentity()
		if identityErr != nil {
			return renderCommandError(cmd, ctx.outputMode(), remoteRPCMutationCommand(rpc.Method), "operation_identity_unavailable", "Remote mutation identity could not be generated", "Retry after a secure random source is available")
		}
		rpc.Params["operation_id"] = identity.OperationID
		rpc.Params["idempotency_key"] = identity.IdempotencyKey
	}
	var wireProjection pinaxclient.Projection
	var callErr error
	if remoteRPCMutationMethod(rpc.Method) && !remoteRPCBool(rpc.Params, "dry_run") {
		wireProjection, callErr = client.CallMutationRPC(cmd.Context(), rpc, pinaxclient.MutationIdentity{
			OperationID: remoteRPCString(rpc.Params, "operation_id"), IdempotencyKey: remoteRPCString(rpc.Params, "idempotency_key"),
		})
	} else {
		wireProjection, callErr = client.CallRPC(cmd.Context(), rpc)
	}
	projection, callErr := remoteapi.Adapt(wireProjection, callErr)
	return ctx.renderProjection(cmd, projection, callErr)
}

func remoteRPCString(params map[string]any, key string) string {
	value, _ := params[key].(string)
	return value
}

func remoteRPCBool(params map[string]any, key string) bool {
	value, _ := params[key].(bool)
	return value
}

func remoteRPCMutationMethod(method string) bool {
	return method == "Pinax.Inbox.Capture" || method == "Pinax.Folder.Rename"
}

func remoteRPCMutationCommand(method string) string {
	if method == "Pinax.Folder.Rename" {
		return "folder.rename"
	}
	return "inbox.capture"
}

func cloneRemoteRPCRequest(request pinaxclient.RPCRequest) pinaxclient.RPCRequest {
	params := make(map[string]any, len(request.Params))
	for key, value := range request.Params {
		params[key] = value
	}
	return pinaxclient.RPCRequest{Method: request.Method, Params: params}
}

func connectionBlockerError(blockers []string) (string, string, string) {
	for _, blocker := range blockers {
		switch blocker {
		case "tls_required":
			return "tls_required", "Remote API endpoints must use HTTPS", "Use an HTTPS URL or a loopback HTTP URL"
		case "credential_required":
			return "credential_required", "Remote API mode requires a controlled credential source", "Use --api-token-file or PINAX_API_TOKEN_FILE"
		case "connection_mode_endpoint_conflict", "connection_mode_invalid":
			return "connection_mode_invalid", "Connection mode does not match the owner endpoint", "Use remote-service or self-hosted-service for non-loopback endpoints"
		}
	}
	return "connection_blocked", "Connection policy blocked the remote request", "Run pinax connection doctor --json"
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
	if root == "backup" || root == "capsa" || root == "sync" || root == "commands" || root == "connection" {
		return true
	}
	if source != "config" {
		return false
	}
	switch root {
	case "api", "backup", "commands", "config", "connection", "token", "profile", "vault", "completion", "help":
		return true
	default:
		return false
	}
}

type RemoteCommandCoverageEntry struct {
	CommandPath string
	Status      string
	Reason      string
	RPCMethod   string
}

func RemoteCommandCoverage(root *cobra.Command) []RemoteCommandCoverageEntry {
	entries := []RemoteCommandCoverageEntry{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.IsAvailableCommand() && cmd.Runnable() {
			entries = append(entries, classifyRemoteCommand(cmd.CommandPath()))
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(entries, func(i, j int) bool { return entries[i].CommandPath < entries[j].CommandPath })
	return entries
}

func classifyRemoteCommand(commandPath string) RemoteCommandCoverageEntry {
	rel := strings.TrimPrefix(commandPath, "pinax ")
	if rel == "pinax" {
		return RemoteCommandCoverageEntry{CommandPath: commandPath, Status: "local_only", Reason: "root_help"}
	}
	if method, ok := remoteSupportedRPCMethods()[rel]; ok {
		return RemoteCommandCoverageEntry{CommandPath: commandPath, Status: "remote_supported", RPCMethod: method}
	}
	root, _, _ := strings.Cut(rel, " ")
	switch root {
	case "api", "backup", "commands", "config", "connection", "token", "profile", "vault", "completion", "help":
		return RemoteCommandCoverageEntry{CommandPath: commandPath, Status: "local_only", Reason: "local_runtime_or_configuration"}
	case "cloud", "sync":
		return RemoteCommandCoverageEntry{CommandPath: commandPath, Status: "local_only", Reason: "cloud_sync_runs_locally"}
	default:
		return RemoteCommandCoverageEntry{CommandPath: commandPath, Status: "unsupported", Reason: "not_in_remote_capability_registry"}
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
	return pinaxclient.LoadTokenFile(path)
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

func templateNameCompletion(vaultPathValue func() string, kind string, includeBuiltins bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Template completion reads metadata only: it does not render templates, execute SQL, or write the vault; it only returns names and source descriptions.
		root := completionVaultRoot(vaultPathValue())
		items := app.TemplateCompletionItems(root, kind, includeBuiltins, true)
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

func firstNoteRefCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveDefault
		}
		return noteRefCompletion(vaultPathValue)(cmd, args, toComplete)
	}
}

func projectSubprojectCompletion(vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		root := completionVaultRoot(vaultPathValue())
		items, err := projectSubprojectCompletionItems(root, args[0])
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
		items := assetCompletionItems(root)
		return filterCompletionItems(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func assetCompletionItems(root string) []string {
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
	return items
}

func projectSubprojectCompletionItems(root, project string) ([]string, error) {
	project = strings.TrimSpace(project)
	if project == "" || strings.ContainsAny(project, `/\`) || strings.Contains(project, "..") {
		return []string{}, nil
	}
	dir := filepath.Join(root, ".pinax", "project-workspaces", project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var workspace domain.ProjectWorkspace
		if err := json.Unmarshal(b, &workspace); err != nil {
			continue
		}
		subproject := strings.TrimSpace(workspace.Subproject)
		if subproject == "" {
			subproject = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		description := strings.TrimSpace(workspace.Title)
		if description == "" {
			description = "subproject"
		}
		items = append(items, subproject+"\t"+description)
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
