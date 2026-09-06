package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	charmtable "github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/yeisme/pinax/internal/domain"
	"golang.org/x/term"
)

type Mode string

const (
	ModeSummary Mode = "summary"
	ModeAgent   Mode = "agent"
	ModeJSON    Mode = "json"
	ModeEvents  Mode = "events"
	ModeExplain Mode = "explain"
)

type RenderOptions struct {
	Style      string
	ColorMode  string
	ThemeName  string
	ThemeRoles map[string]string
	Width      int
	Markdown   MarkdownOptions
	IsTerminal bool
	JSONIndent string
	// SyncPreview and SyncLimit only affect human/agent sync previews. Machine
	// JSON keeps the complete plan and sync_view regardless of these values.
	SyncPreview  string
	SyncLimit    int
	SyncLimitSet bool
	ContentDiff  bool
}

type MarkdownOptions struct {
	Enabled bool
	Style   string
	Pager   string
}

type ThemeRoles struct {
	Accent  string
	Muted   string
	Rule    string
	Success string
	Warning string
	Danger  string
	Key     string
	Value   string
	Path    string
	Link    string
	Code    string
	Heading string
}

func Render(w io.Writer, mode Mode, projection domain.Projection) error {
	return RenderWithOptions(w, mode, projection, RenderOptions{})
}

func RenderWithOptions(w io.Writer, mode Mode, projection domain.Projection, opts RenderOptions) error {
	projection.SpecVersion = defaultString(projection.SpecVersion, "1.0")
	projection.Mode = string(mode)
	if projection.Status == "" {
		projection.Status = "success"
	}
	// 共享脱敏门禁：所有渲染模式在输出前统一递归扫描，拦截 note body、token、
	// Authorization、cookie、webhook、provider payload 与 raw/hidden prompt。
	ApplyProjectionRedaction(&projection)

	switch mode {
	case ModeJSON:
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if jsonIndentEnabled(w, opts) {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(projection)
	case ModeAgent:
		return renderAgentWithOptions(w, projection, opts)
	case ModeEvents:
		return renderEvents(w, projection)
	case ModeExplain:
		return renderExplain(w, projection)
	default:
		return renderSummaryWithOptions(w, projection, opts)
	}
}

func renderSummaryWithOptions(w io.Writer, p domain.Projection, opts RenderOptions) error {
	if strings.EqualFold(strings.TrimSpace(opts.Style), "compact") {
		return renderCompactSummaryWithOptions(w, p, opts)
	}
	theme := newSummaryThemeWithOptions(w, opts)
	if (p.Command == "project.list" || p.Command == "project.subproject.list") && p.Error == nil {
		return renderSummaryProjectList(w, theme, p)
	}
	if p.Command == "note.preview" && p.Status == "success" && p.Error == nil {
		return renderSummaryDataWithOptions(w, theme, p, opts)
	}
	// continue 的 human view 是 command-specific Resume Card（experimental UX
	// refinement）；machine envelope/--json/--agent 不变，解码失败回退 generic。
	if p.Command == "continue" && p.Status == "success" && p.Error == nil {
		if err := renderResumeCard(w, theme, p); err == nil {
			return nil
		}
	}
	if p.Status == "success" && p.Error == nil {
		if err := renderSummaryTable(w, theme, []string{"Highlights"}, [][]string{{defaultString(p.Summary, "-")}}); err != nil {
			return err
		}
	} else if err := renderSummaryTable(w, theme, []string{"Status", "Highlights"}, [][]string{{summaryStatusCell(theme, p.Status), defaultString(p.Summary, "-")}}); err != nil {
		return err
	}
	if p.Error != nil {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderSummaryTable(w, theme, []string{"Error", "Details"}, [][]string{{theme.failed.Render(p.Error.Code), defaultString(p.Error.Message, "-")}}); err != nil {
			return err
		}
		if shouldRenderErrorData(p.Command) {
			if err := renderSummaryDataWithOptions(w, theme, p, opts); err != nil {
				return err
			}
		}
		if p.Error.Hint != "" {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
			return renderSummaryTable(w, theme, []string{"Next step"}, [][]string{{p.Error.Hint}})
		}
		return nil
	}
	summaryFacts := summaryFactsForProjection(p)
	if len(summaryFacts) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderSummaryFacts(w, theme, summaryFacts); err != nil {
			return err
		}
	}
	if err := renderSummaryDataWithOptions(w, theme, p, opts); err != nil {
		return err
	}
	if len(p.Evidence) > 0 && p.Command != "api.routes" {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		rows := make([][]string, 0, len(p.Evidence))
		for _, item := range p.Evidence {
			rows = append(rows, []string{item})
		}
		if err := renderSummaryTable(w, theme, []string{"Evidence"}, rows); err != nil {
			return err
		}
	}
	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		return renderSummaryTable(w, theme, []string{"Next step"}, [][]string{{p.Actions[0].Command}})
	}
	return nil
}

func renderCompactSummaryWithOptions(w io.Writer, p domain.Projection, opts RenderOptions) error {
	if p.Command == "note.preview" && p.Status == "success" && p.Error == nil {
		return renderSummaryDataWithOptions(w, newSummaryThemeWithOptions(w, opts), p, opts)
	}
	if p.Error != nil {
		if _, err := fmt.Fprintf(w, "error=%s: %s\n", p.Error.Code, p.Error.Message); err != nil {
			return err
		}
		if p.Error.Hint != "" {
			_, err := fmt.Fprintf(w, "next=%s\n", p.Error.Hint)
			return err
		}
		return nil
	}
	if p.Summary != "" {
		if _, err := fmt.Fprintf(w, "summary=%s\n", p.Summary); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(p.Facts))
	for key := range p.Facts {
		keys = append(keys, key)
	}
	sortFactKeys(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s=%s\n", key, p.Facts[key]); err != nil {
			return err
		}
	}
	if err := renderSummaryDataWithOptions(w, newSummaryThemeWithOptions(w, opts), p, opts); err != nil {
		return err
	}
	for _, action := range p.Actions {
		if _, err := fmt.Fprintf(w, "next.%s=%s\n", action.Name, action.Command); err != nil {
			return err
		}
	}
	return nil
}

type summaryTheme struct {
	renderer *lipgloss.Renderer
	width    int
	header   lipgloss.Style
	rule     lipgloss.Style
	success  lipgloss.Style
	failed   lipgloss.Style
	numeric  lipgloss.Style
	action   lipgloss.Style
}

func newSummaryThemeWithOptions(w io.Writer, opts RenderOptions) summaryTheme {
	renderer := lipgloss.NewRenderer(w)
	if summaryColorEnabledWithOptions(w, opts) {
		renderer.SetColorProfile(termenv.TrueColor)
	} else {
		renderer.SetColorProfile(termenv.Ascii)
	}
	roles := themeRolesForOptions(opts)
	style := func(color string) lipgloss.Style { return renderer.NewStyle().Foreground(lipgloss.Color(color)) }
	return summaryTheme{
		renderer: renderer,
		width:    opts.Width,
		header:   style(roles.Heading).Bold(true),
		rule:     style(roles.Rule),
		success:  style(roles.Success).Bold(true),
		failed:   style(roles.Danger).Bold(true),
		numeric:  style(roles.Value),
		action:   style(roles.Link),
	}
}

func shouldRenderErrorData(command string) bool {
	switch command {
	case "publish.profile.validate":
		return true
	default:
		return false
	}
}

func themeRolesForOptions(opts RenderOptions) ThemeRoles {
	name := strings.ToLower(strings.TrimSpace(defaultString(opts.ThemeName, "pinax")))
	if name != "custom" {
		return builtInTheme(name)
	}
	roles := builtInTheme("pinax")
	for role, color := range opts.ThemeRoles {
		applyThemeRole(&roles, role, color)
	}
	return roles
}

func builtInTheme(name string) ThemeRoles {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mono":
		return ThemeRoles{Accent: "250", Muted: "244", Rule: "244", Success: "250", Warning: "250", Danger: "250", Key: "250", Value: "250", Path: "250", Link: "250", Code: "250", Heading: "250"}
	case "high-contrast":
		return ThemeRoles{Accent: "51", Muted: "255", Rule: "255", Success: "46", Warning: "226", Danger: "196", Key: "255", Value: "255", Path: "51", Link: "51", Code: "255", Heading: "255"}
	default:
		return ThemeRoles{Accent: "38", Muted: "250", Rule: "240", Success: "34", Warning: "178", Danger: "160", Key: "250", Value: "250", Path: "38", Link: "38", Code: "250", Heading: "250"}
	}
}

func applyThemeRole(roles *ThemeRoles, role, color string) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "accent":
		roles.Accent = color
	case "muted":
		roles.Muted = color
	case "rule":
		roles.Rule = color
	case "success":
		roles.Success = color
	case "warning":
		roles.Warning = color
	case "danger":
		roles.Danger = color
	case "key":
		roles.Key = color
	case "value":
		roles.Value = color
	case "path":
		roles.Path = color
	case "link":
		roles.Link = color
	case "code":
		roles.Code = color
	case "heading":
		roles.Heading = color
	}
}

// jsonIndentEnabled decides whether --json output is pretty-printed. Defaults to
// indented on an interactive terminal (human inspection) and compact when piped,
// redirected, or written to a buffer (scripts, CI, test snapshots). Callers can
// force a mode via RenderOptions.JSONIndent or the PINAX_JSON env var.
func jsonIndentEnabled(w io.Writer, opts RenderOptions) bool {
	switch strings.ToLower(strings.TrimSpace(opts.JSONIndent)) {
	case "pretty", "indent", "on", "1", "true", "yes", "always":
		return true
	case "compact", "off", "0", "false", "no", "never":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PINAX_JSON"))) {
	case "pretty", "indent", "on", "1", "true", "yes", "always":
		return true
	case "compact", "off", "0", "false", "no", "never":
		return false
	}
	if opts.IsTerminal {
		return true
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func summaryColorEnabledWithOptions(w io.Writer, opts RenderOptions) bool {
	mode := strings.ToLower(strings.TrimSpace(opts.ColorMode))
	if mode == "" {
		mode = colorModeFromEnv()
	}
	switch mode {
	case "always", "1", "true", "yes", "on":
		return true
	case "never", "0", "false", "no", "off":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if opts.ColorMode != "" {
		return opts.IsTerminal
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func colorModeFromEnv() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PINAX_COLOR"))) {
	case "always", "1", "true", "yes", "on":
		return "always"
	case "never", "0", "false", "no", "off":
		return "never"
	default:
		return "auto"
	}
}

func summaryStatusCell(theme summaryTheme, status string) string {
	label := summaryStatusLabel(status)
	switch status {
	case "success":
		return theme.success.Render(label)
	case "failed":
		return theme.failed.Render(label)
	case "partial":
		return theme.action.Render(label)
	default:
		return label
	}
}

func summaryStatusLabel(status string) string {
	switch status {
	case "success":
		return "Success"
	case "succeeded":
		return "Succeeded"
	case "failed":
		return "Failed"
	case "partial":
		return "Partial"
	default:
		return summaryHumanValue("status", status)
	}
}

func renderSummaryTable(w io.Writer, theme summaryTheme, header []string, rows [][]string) error {
	tw := charmtable.New().
		Headers(header...).
		Rows(rows...).
		Border(lipgloss.Border{Top: "─"}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		BorderColumn(false).
		BorderRow(false).
		BorderStyle(theme.rule).
		StyleFunc(summaryTableStyle(theme, header))
	// Keep the established default table geometry at the configured 100-column
	// baseline; any other explicit width opts into lipgloss table resizing.
	if theme.width >= 20 && theme.width != 100 {
		tw.Width(theme.width)
	}
	body := trimTrailingSpaceLines(tw.Render())
	_, err := fmt.Fprintln(w, indentBlock(body, "  "))
	return err
}

// indentBlock left-pads every line of a multi-line block, used to give summary
// tables a small visual margin without heavy framing rules.
func indentBlock(value, pad string) string {
	if pad == "" {
		return value
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

func summaryTableStyle(theme summaryTheme, header []string) charmtable.StyleFunc {
	return func(row, col int) lipgloss.Style {
		style := theme.renderer.NewStyle().PaddingRight(2)
		if row == charmtable.HeaderRow {
			style = style.Inherit(theme.header)
		}
		if col < len(header) {
			switch header[col] {
			case "Error":
				style = style.Inherit(theme.failed)
			case "Next step":
				style = style.Inherit(theme.action)
			}
		}
		if col < len(header) && isNumericSummaryColumn(header[col]) {
			style = style.Align(lipgloss.Right).Inherit(theme.numeric)
		}
		return style
	}
}

func isNumericSummaryColumn(header string) bool {
	switch header {
	case "Count", "Share", "Lines", "Code", "Comments", "Blank", "Notes", "Assets", "Depth":
		return true
	default:
		return false
	}
}

func trimTrailingSpaceLines(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func summaryCell(value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	if maxWidth > 0 && lipgloss.Width(value) > maxWidth {
		return ansi.Truncate(value, maxWidth, "…")
	}
	return value
}

func renderSummaryFacts(w io.Writer, theme summaryTheme, facts map[string]string) error {
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sortFactKeys(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{summaryFactLabel(key), summaryFactValue(key, facts[key])})
	}
	return renderSummaryTable(w, theme, []string{"Metric", "Value"}, rows)
}

func summaryFactsForProjection(p domain.Projection) map[string]string {
	excludePrefixes := map[string][]string{
		"asset.list":         {"asset."},
		"prompt.search":      {"prompt_asset."},
		"publish.theme.list": {"theme."},
		"trash.list":         {"entry."},
	}
	prefixes := excludePrefixes[p.Command]
	if len(prefixes) == 0 {
		return p.Facts
	}
	filtered := make(map[string]string, len(p.Facts))
	for key, value := range p.Facts {
		if hasAnyPrefix(key, prefixes) {
			continue
		}
		filtered[key] = value
	}
	return filtered
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// syncFactLabels labels the sync output view's fact keys.
var syncFactLabels = map[string]string{
	"result":           "Sync result",
	"scope":            "Sync scope",
	"added":            "Added",
	"modified":         "Modified",
	"deleted":          "Deleted",
	"renamed":          "Renamed",
	"conflicts":        "Conflicts",
	"unchanged":        "Unchanged",
	"total":            "Sync changes",
	"bytes_uploaded":   "Bytes uploaded",
	"bytes_downloaded": "Bytes downloaded",
}

func summaryFactLabel(key string) string {
	if label, ok := syncFactLabels[strings.TrimPrefix(key, "sync.")]; ok && strings.HasPrefix(key, "sync.") {
		return label
	}
	if label, ok := projectListFactLabel(key); ok {
		return label
	}
	if label, ok := numberedProjectWorkspaceFactLabel(key); ok {
		return label
	}
	labels := map[string]string{
		"action_id":                "Action ID",
		"adopted":                  "Adopted",
		"ambiguous":                "Ambiguous links",
		"applied":                  "Applied",
		"applied_metadata":         "Applied metadata",
		"applied_moves":            "Applied moves",
		"applied_updates":          "Applied updates",
		"attachment_path":          "Attachment path",
		"attachments":              "Attachments",
		"automatic":                "Automatic items",
		"backend":                  "Backend",
		"backend_required":         "Backend required",
		"backends":                 "Backends",
		"backlinks":                "Backlinks",
		"base_revision":            "Base revision",
		"broken":                   "Broken links",
		"bucket":                   "Bucket",
		"bytes":                    "Bytes",
		"candidates":               "Candidates",
		"capabilities":             "Capabilities",
		"changed":                  "Changed",
		"changed_blocks":           "Changed blocks",
		"child_folders":            "Child folders",
		"columns":                  "Columns",
		"configured":               "Configured",
		"conflicts":                "Conflicts",
		"conflict_file":            "Conflict file",
		"count":                    "Count",
		"created":                  "Created",
		"credential_source":        "Credential source",
		"current_project":          "Current project",
		"daily_index":              "Daily index",
		"date":                     "Date",
		"decision_id":              "Decision ID",
		"default_backend":          "Default backend",
		"delete_candidates":        "Delete candidates",
		"deleted":                  "Deleted",
		"device_id":                "Device ID",
		"descendant_folders":       "Descendant folders",
		"dimension":                "Dimension",
		"dimensions":               "Dimensions",
		"direction":                "Direction",
		"dry_run":                  "Dry run",
		"editor":                   "Editor",
		"editor_args":              "Editor arguments",
		"editor_executable":        "Editor executable",
		"endpoint":                 "Endpoint",
		"engine":                   "Engine",
		"failed":                   "Failed",
		"filter.created_after":     "Filter: created after",
		"filter.folder":            "Filter: folder",
		"filter.group":             "Filter: group",
		"filter.kind":              "Filter: kind",
		"filter.path_prefix":       "Filter: path prefix",
		"filter.project":           "Filter: project",
		"filter.status":            "Filter: status",
		"filter.tag":               "Filter: tag",
		"filter.under":             "Filter: under",
		"filter.updated_before":    "Filter: updated before",
		"filters":                  "Filters",
		"folder":                   "Folder",
		"folder_path":              "Folder path",
		"frontmatter_coverage":     "Frontmatter coverage",
		"group":                    "Group",
		"groups":                   "Groups",
		"hard":                     "Hard delete",
		"has_more":                 "Has more",
		"index_loaded":             "Index load",
		"index_status":             "Index status",
		"index_updated":            "Index updated",
		"issues":                   "Issues",
		"issues.total":             "Total issues",
		"items":                    "Items",
		"keep":                     "Keep",
		"key":                      "Key",
		"kind":                     "Kind",
		"ledger_seq":               "Ledger sequence",
		"ledger_status":            "Ledger status",
		"lifecycle":                "Lifecycle",
		"limit":                    "Limit",
		"link_target.candidates":   "Link target candidates",
		"link_target.matches":      "Link target matches",
		"link_target.status":       "Link target status",
		"links":                    "Outbound links",
		"manual_review":            "Manual review",
		"matches":                  "Matches",
		"max_commitments":          "Max commitments",
		"media_type":               "Media type",
		"main_path":                "Main path",
		"message":                  "Message",
		"missing":                  "Missing",
		"mode":                     "Mode",
		"moved":                    "Moved",
		"name":                     "Name",
		"network_checked":          "Network checked",
		"next_cursor":              "Next cursor",
		"note_id":                  "Note ID",
		"notes":                    "Notes",
		"notes_prefix":             "Notes path prefix",
		"opened":                   "Opened",
		"operation":                "Operation",
		"operations":               "Operations",
		"operations.automatic":     "Automatic operations",
		"operations.manual_review": "Manual review operations",
		"operations.total":         "Total operations",
		"orphans":                  "Orphan notes",
		"output.color":             "Output color",
		"output.style":             "Output style",
		"output.theme":             "Output theme",
		"output.width":             "Output width",
		"output_dir":               "Output directory",
		"output_format":            "Output format",
		"overwritten":              "Overwritten",
		"path":                     "Path",
		"period":                   "Period",
		"plan_id":                  "Plan ID",
		"planned":                  "Planned",
		"planned_moves":            "Planned moves",
		"planned_path":             "Planned path",
		"planned_updates":          "Planned updates",
		"plans":                    "Plans",
		"prefix":                   "Prefix",
		"project":                  "Project",
		"project_config":           "Project config",
		"projects":                 "Projects",
		"properties":               "Properties",
		"property":                 "Property",
		"provider":                 "Provider",
		"queries":                  "Queries",
		"query_count":              "Queries",
		"receipt_path":             "Receipt path",
		"recent":                   "Recent only",
		"recent_updates":           "Recent updates",
		"record_event":             "Record event",
		"record_event_id":          "Record event ID",
		"record_events":            "Record events",
		"record_version":           "Record version",
		"records":                  "Records",
		"records_path":             "Records path",
		"region":                   "Region",
		"remote_revision":          "Remote revision",
		"remote_write":             "Remote write",
		"removed":                  "Removed",
		"renamed":                  "Renamed",
		"resolved":                 "Resolved",
		"restored":                 "Restored",
		"returned":                 "Returned",
		"risk.low":                 "Low risks",
		"risk.medium":              "Medium risks",
		"risk.review":              "Review risks",
		"risks":                    "Risks",
		"root":                     "Root",
		"rows":                     "Rows",
		"run":                      "Run",
		"run_id":                   "Run ID",
		"run_name":                 "Run name",
		"run_saved":                "Run saved",
		"runs":                     "Runs",
		"saved_path":               "Saved path",
		"scan_duration_ms":         "Scan duration ms",
		"schema_version":           "Schema version",
		"manifest_digest":          "Manifest digest",
		"digest":                   "Digest",
		"bindings":                 "Bindings",
		"operation_id":             "Operation ID",
		"operation_status":         "Operation status",
		"capability_id":            "Capability ID",
		"binding_id":               "Binding ID",
		"retryable":                "Retryable",
		"replay_safe":              "Replay safe",
		"reconcile_required":       "Reconcile required",
		"receipt_ref":              "Receipt reference",
		"resource_ref":             "Resource reference",
		"revision_before":          "Revision before",
		"revision_after":           "Revision after",
		"idempotent_replay":        "Idempotent replay",
		"recovered_from_operation": "Recovered from operation",
		"overall_status":           "Overall status",
		"overall_maturity":         "Overall maturity",
		"contract_status":          "Contract status",
		"transport_status":         "Transport status",
		"auth_status":              "Auth status",
		"owner_status":             "Owner status",
		"mutation_recovery_status": "Mutation recovery status",
		"production_status":        "Production status",
		"scope":                    "Scope",
		"scopes":                   "Scopes",
		"secret_ref_configured":    "Secret ref configured",
		"session_status":           "Session status",
		"skipped":                  "Skipped",
		"skipped_issues":           "Skipped issues",
		"snapshot":                 "Snapshot",
		"snapshot_id":              "Snapshot ID",
		"sort":                     "Sort",
		"sorts":                    "Sorts",
		"source":                   "Source",
		"source_decision":          "Source decision",
		"source_path":              "Source path",
		"subprojects":              "Subprojects",
		"sources":                  "Sources",
		"status":                   "Status",
		"tags":                     "Tags",
		"old_tag":                  "Old tag",
		"new_tag":                  "New tag",
		"old_folder":               "Old folder",
		"new_folder":               "New folder",
		"target":                   "Target",
		"tasks":                    "Tasks",
		"template":                 "Template",
		"templates":                "Templates",
		"title":                    "Title",
		"tokens":                   "Tokens",
		"tombstones":               "Tombstones",
		"topic":                    "Topic",
		"total":                    "Total",
		"trash_path":               "Trash path",
		"type":                     "Type",
		"unresolved":               "Unresolved",
		"user_config":              "User config",
		"value":                    "Value",
		"variables":                "Variables",
		"vault":                    "Vault",
		"version":                  "Version",
		"version_backend":          "Version backend",
		"view":                     "View",
		"views":                    "Views",
		"workspace_id":             "Workspace ID",
		"worktree_state":           "Worktree state",
		"vault_root":               "Vault root",
		"workspace.full_path":      "Full path preview",
		"workspace.path":           "Workspace path",
		"workspace.project":        "Workspace project",
		"workspace.subproject":     "Workspace subproject",
		"workspace_path":           "Workspace path",
		"writes":                   "Writes",
		"written":                  "Written",
	}
	if label, ok := labels[key]; ok {
		return label
	}
	return strings.ReplaceAll(key, "_", " ")
}

func numberedProjectWorkspaceFactLabel(key string) (string, bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 2 || parts[1] == "" || !isASCIIDigitString(parts[1]) {
		return "", false
	}
	switch parts[0] {
	case "subproject":
		return "Subproject " + parts[1], true
	case "workspace":
		return "Workspace " + parts[1], true
	default:
		return "", false
	}
}

func isASCIIDigitString(value string) bool {
	for i := 0; i < len(value); i++ {
		if !isASCIIDigit(value[i]) {
			return false
		}
	}
	return value != ""
}

func projectListFactLabel(key string) (string, bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "project" || parts[1] == "" {
		return "", false
	}
	field := map[string]string{
		"slug":         "slug",
		"name":         "name",
		"description":  "description",
		"notes_prefix": "notes path prefix",
		"created_at":   "created",
	}[parts[2]]
	if field == "" {
		field = strings.ReplaceAll(parts[2], "_", " ")
	}
	return "Project " + parts[1] + " " + field, true
}

func summaryFactValue(key, value string) string {
	switch key {
	case "schema_version", "version", "path", "saved_path", "planned_path", "source_path", "output_dir", "trash_path", "records_path", "receipt_path", "endpoint", "bucket", "region", "prefix", "root", "vault", "vault_root", "workspace_path", "workspace.path", "workspace.full_path", "command", "query":
		return value
	default:
		return summaryHumanValue(key, value)
	}
}

func summaryHumanValue(_ string, value string) string {
	switch strings.TrimSpace(value) {
	case "true":
		return "Yes"
	case "false":
		return "No"
	case "success":
		return "Success"
	case "succeeded":
		return "Succeeded"
	case "failed":
		return "Failed"
	case "partial":
		return "Partial"
	case "fresh":
		return "Fresh"
	case "stale":
		return "Stale"
	case "missing":
		return "Missing"
	case "unreadable":
		return "Unreadable"
	case "configured":
		return "Configured"
	case "active":
		return "Active"
	case "archived":
		return "Archived"
	case "deleted":
		return "Deleted"
	case "trashed":
		return "Trashed"
	case "planned":
		return "Planned"
	case "pending":
		return "Pending"
	case "ready":
		return "Ready"
	case "degraded":
		return "Degraded"
	case "blocked":
		return "Blocked"
	case "not_configured":
		return "Not configured"
	case "not_applicable":
		return "Not applicable"
	case "exploratory":
		return "Exploratory"
	case "first-support":
		return "First support"
	case "mature":
		return "Mature"
	case "applied":
		return "Applied"
	case "skipped":
		return "Skipped"
	case "updated":
		return "Updated"
	case "created":
		return "Created"
	case "resolved":
		return "Resolved"
	case "broken":
		return "Broken"
	case "ambiguous":
		return "Ambiguous"
	case "automatic":
		return "Automatic"
	case "manual_review":
		return "Manual review"
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "review":
		return "Review"
	case "scan":
		return "Scan"
	case "index":
		return "Index"
	case "lazy_rebuild":
		return "Lazy rebuild"
	case "dry_run":
		return "Dry run"
	case "push":
		return "Push"
	case "pull":
		return "Pull"
	case "daily":
		return "Daily"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	case "group":
		return "Group"
	case "tag":
		return "Tag"
	case "folder":
		return "Folder"
	case "kind":
		return "Kind"
	case "status":
		return "Status"
	case "reference":
		return "Reference"
	case "inbox":
		return "Inbox"
	case "project":
		return "Project"
	case "meeting":
		return "Meeting"
	case "note":
		return "Note"
	default:
		return value
	}
}

func renderSummaryDataWithOptions(w io.Writer, theme summaryTheme, p domain.Projection, opts RenderOptions) error {
	switch p.Command {
	case "commands.list":
		return renderSummaryCommandCatalog(w, theme, p.Data)
	case "note.search":
		return renderSummarySearchResults(w, theme, p.Data)
	case "search.show":
		return renderSummarySearchShow(w, theme, p.Data)
	case "browse":
		return renderSummaryBrowse(w, theme, p.Data)
	case "api.routes":
		return renderSummaryAPIRoutes(w, theme, p.Data)
	case "connection.readiness":
		return renderSummaryConnectionReadiness(w, theme, p.Data)
	case "note.list":
		return renderSummaryNoteList(w, theme, p.Data, "notes")
	case "inbox.list", "draft.list":
		return renderSummaryNoteList(w, theme, p.Data, "notes")
	case "template.list":
		return renderSummaryDataList(w, theme, p.Data, []string{"templates"}, []summaryListColumn{{Header: "Template", Path: "name", MaxWidth: 28}, {Header: "Source", Path: "source", MaxWidth: 16}, {Header: "Kind", Path: "kind", MaxWidth: 16}, {Header: "Pack", Path: "pack.id", MaxWidth: 16}, {Header: "Maturity", Path: "maturity", MaxWidth: 18}})
	case "template.validate":
		return renderSummaryNamedDataList(w, theme, "Template issues", p.Data, []string{"issues"}, issueSummaryColumns())
	case "template.runs.prune":
		return renderSummaryNamedDataList(w, theme, "Render runs to delete", p.Data, []string{"delete_candidates"}, deleteCandidateSummaryColumns())
	case "backend.list":
		return renderSummaryDataList(w, theme, p.Data, []string{"registry", "backends"}, []summaryListColumn{{Header: "Backend", Path: "name", MaxWidth: 28}, {Header: "Kind", Path: "kind", MaxWidth: 12}, {Header: "Bucket", Path: "bucket", MaxWidth: 28}, {Header: "Region", Path: "region", MaxWidth: 18}, {Header: "Profile", Path: "profile", MaxWidth: 24}})
	case "backend.show":
		return renderSummaryBackendShow(w, theme, p.Data)
	case "backend.capabilities":
		return renderSummaryNamedDataList(w, theme, "Backend capabilities", p.Data, []string{"capabilities"}, []summaryListColumn{{Header: "Capability", Path: "name", MaxWidth: 28}, {Header: "Supported", Path: "supported", MaxWidth: 12}})
	case "backend.doctor":
		return renderSummaryNamedDataList(w, theme, "Backend issues", p.Data, []string{"issues"}, issueSummaryColumns())
	case "backend.object.list":
		return renderSummaryDataList(w, theme, p.Data, []string{"objects"}, []summaryListColumn{{Header: "Key", Path: "key", MaxWidth: 44}, {Header: "Size", Path: "size_bytes", MaxWidth: 12}, {Header: "Updated", Path: "updated_at", MaxWidth: 22}})
	case "backend.notes.list":
		return renderSummaryDataList(w, theme, p.Data, []string{"notes"}, []summaryListColumn{{Header: "Path", Path: "path", MaxWidth: 44}, {Header: "Size", Path: "size_bytes", MaxWidth: 12}, {Header: "Updated", Path: "updated_at", MaxWidth: 22}})
	case "activity.list", "activity.tail":
		return renderSummaryActivityList(w, theme, p.Data)
	case "monitor.runs", "monitor.tail":
		return renderSummaryMonitorRuns(w, theme, p.Data)
	case "sync.logs.list":
		return renderSummaryDataList(w, theme, p.Data, []string{"runs"}, []summaryListColumn{{Header: "Run ID", Path: "run_id", MaxWidth: 28}, {Header: "Direction", Path: "direction", MaxWidth: 12}, {Header: "Status", Path: "status", MaxWidth: 12}, {Header: "Backend", Path: "backend_kind", MaxWidth: 16}, {Header: "Started", Path: "started_at", MaxWidth: 22}})
	case "sync.logs.tail":
		return renderSummaryDataList(w, theme, p.Data, []string{"events"}, []summaryListColumn{{Header: "Run ID", Path: "run_id", MaxWidth: 28}, {Header: "Direction", Path: "direction", MaxWidth: 12}, {Header: "Operation", Path: "kind", MaxWidth: 18}, {Header: "Path", Path: "path", MaxWidth: 48}, {Header: "Status", Path: "status", MaxWidth: 12}, {Header: "Backend", Path: "backend_kind", MaxWidth: 16}, {Header: "Time", Path: "ts", MaxWidth: 22}})
	case "sync.logs.prune":
		return renderSummaryNamedDataList(w, theme, "Delete candidates", p.Data, []string{"delete_candidates"}, deleteCandidateSummaryColumns())
	case "sync", "sync.all", "sync.all.pull", "sync.all.push", "sync.diff", "sync.push", "sync.pull", "sync.logs.show":
		return renderSummarySyncView(w, theme, p, opts)
	case "plan.daily", "plan.weekly", "plan.monthly":
		return renderSummaryPlanningSelectedTasks(w, theme, p.Data)
	case "plan.actions":
		return renderSummaryNamedDataList(w, theme, "Action draft tasks", p.Data, []string{"draft", "tasks"}, []summaryListColumn{{Header: "Task ID", Path: "task_id", MaxWidth: 28}, {Header: "Kind", Path: "kind", MaxWidth: 12}, {Header: "Reason", Path: "reason", MaxWidth: 56}, {Header: "Confirm", Path: "requires_confirmation", MaxWidth: 10}})
	case "brain.answer":
		return renderSummaryNamedDataList(w, theme, "Brain sources", p.Data, []string{"sources"}, []summaryListColumn{{Header: "Kind", Path: "kind", MaxWidth: 14}, {Header: "Path", Path: "path", MaxWidth: 44}, {Header: "Title", Path: "title", MaxWidth: 30}, {Header: "Trust", Path: "trust", MaxWidth: 12}, {Header: "Fresh", Path: "fresh", MaxWidth: 8}})
	case "brain.maintenance_plan":
		return renderSummaryNamedDataList(w, theme, "Brain maintenance operations", p.Data, []string{"operations"}, []summaryListColumn{{Header: "Kind", Path: "kind", MaxWidth: 28}, {Header: "Risk", Path: "risk", MaxWidth: 12}, {Header: "Status", Path: "status", MaxWidth: 14}, {Header: "Next action", Path: "next_action.command", MaxWidth: 58}})
	case "note.orphans":
		return renderSummaryNoteList(w, theme, p.Data, "orphans")
	case "note.links":
		return renderSummaryLinkList(w, theme, p.Data, "links")
	case "note.backlinks":
		return renderSummaryLinkList(w, theme, p.Data, "backlinks")
	case "folder.list":
		return renderSummaryFolderList(w, theme, p.Data)
	case "folder.show":
		return renderSummaryFolderShow(w, theme, p.Data)
	case "folder.repair":
		return renderSummaryNamedDataList(w, theme, "Folder issues", p.Data, []string{"issues"}, folderIssueSummaryColumns())
	case "folder.create", "folder.adopt", "folder.delete", "folder.rename", "folder.move":
		return renderSummaryNamedDataList(w, theme, "Folder effects", p.Data, []string{"plan", "effects"}, []summaryListColumn{{Header: "Kind", Path: "kind", MaxWidth: 24}, {Header: "Path", Path: "path", MaxWidth: 42}, {Header: "Target", Path: "target", MaxWidth: 42}, {Header: "Status", Path: "status", MaxWidth: 14}})
	case "collection.import", "collection.diff":
		return renderSummaryNamedDataList(w, theme, "Collection items", p.Data, []string{"plan", "plans"}, []summaryListColumn{{Header: "Item ID", Path: "item_id", MaxWidth: 24}, {Header: "Title", Path: "title", MaxWidth: 30}, {Header: "Note", Path: "note_path", MaxWidth: 44}, {Header: "Prompt", Path: "prompt_asset_id", MaxWidth: 34}, {Header: "Note status", Path: "note_status", MaxWidth: 14}, {Header: "Prompt status", Path: "prompt_status", MaxWidth: 16}})
	case "graph.query":
		return renderSummaryNamedDataList(w, theme, "Graph results", p.Data, []string{"results"}, []summaryListColumn{{Header: "Prompt ID", Path: "prompt_asset_id", MaxWidth: 38}, {Header: "Title", Path: "title", MaxWidth: 36}})
	case "record.adopt":
		return renderSummaryNamedDataList(w, theme, "Record candidates", p.Data, []string{"candidates"}, []summaryListColumn{{Header: "Kind", Path: "object_kind", MaxWidth: 12}, {Header: "Path", Path: "path", MaxWidth: 48}, {Header: "Status", Path: "managed_status", MaxWidth: 16}, {Header: "Score", Path: "score", MaxWidth: 8}})
	case "metadata.plan":
		return renderSummaryNamedDataList(w, theme, "Metadata operations", p.Data, []string{"operations"}, []summaryListColumn{{Header: "Kind", Path: "kind", MaxWidth: 24}, {Header: "Path", Path: "path", MaxWidth: 48}, {Header: "Status", Path: "status", MaxWidth: 14}})
	case "record.history":
		return renderSummaryRecordHistory(w, theme, p.Data)
	case "asset.list":
		return renderSummaryNamedDataList(w, theme, "Assets", p.Data, []string{"assets"}, []summaryListColumn{{Header: "Path", Path: "path", MaxWidth: 42}, {Header: "Filename", Path: "filename", MaxWidth: 28}, {Header: "Media type", Path: "media_type", MaxWidth: 18}, {Header: "Size", Path: "size", MaxWidth: 12}, {Header: "Status", Path: "managed_status", MaxWidth: 16}})
	case "asset.show":
		return renderSummaryAssetShow(w, theme, p.Data)
	case "memory.list":
		return renderSummaryNamedDataList(w, theme, "Memory records", p.Data, []string{"records"}, []summaryListColumn{{Header: "Record ID", Path: "id", MaxWidth: 24}, {Header: "Type", Path: "type", MaxWidth: 12}, {Header: "Subject", Path: "subject", MaxWidth: 18}, {Header: "Predicate", Path: "predicate", MaxWidth: 24}, {Header: "Object", Path: "object", MaxWidth: 34}, {Header: "Status", Path: "status", MaxWidth: 14}})
	case "memory.recall", "memory.context":
		return renderSummaryNamedDataList(w, theme, "Memory matches", p.Data, []string{"matches"}, []summaryListColumn{{Header: "Record ID", Path: "id", MaxWidth: 24}, {Header: "Type", Path: "type", MaxWidth: 12}, {Header: "Subject", Path: "subject", MaxWidth: 18}, {Header: "Object", Path: "object", MaxWidth: 34}, {Header: "Score", Path: "score", MaxWidth: 10}, {Header: "Reason", Path: "recall_reason", MaxWidth: 42}})
	case "plugin.list":
		return renderSummaryNamedDataList(w, theme, "Plugins", p.Data, []string{"plugins"}, []summaryListColumn{{Header: "Plugin ID", Path: "id", MaxWidth: 28}, {Header: "Name", Path: "name", MaxWidth: 28}, {Header: "Version", Path: "version", MaxWidth: 14}, {Header: "Runtime", Path: "runtime", MaxWidth: 14}, {Header: "Enabled", Path: "enabled", MaxWidth: 10}, {Header: "Scope", Path: "scope", MaxWidth: 12}})
	case "plugin.permissions.list":
		return renderSummaryNamedDataList(w, theme, "Permission grants", p.Data, []string{"grants"}, []summaryListColumn{{Header: "Permission", Path: "permission", MaxWidth: 28}, {Header: "Capability", Path: "capability", MaxWidth: 28}, {Header: "Granted", Path: "granted_at", MaxWidth: 22}})
	case "prompt.search":
		return renderSummaryNamedDataList(w, theme, "Prompt assets", p.Data, []string{"prompt_assets"}, []summaryListColumn{{Header: "Prompt ID", Path: "PromptAssetID", MaxWidth: 34}, {Header: "Title", Path: "Title", MaxWidth: 30}, {Header: "Domain", Path: "Domain", MaxWidth: 24}, {Header: "Lifecycle", Path: "Lifecycle", MaxWidth: 14}, {Header: "Permission", Path: "Permission", MaxWidth: 16}})
	case "database.schema.list":
		return renderSummaryNamedDataList(w, theme, "Properties", p.Data, []string{"properties"}, []summaryListColumn{{Header: "Property", Path: "name", MaxWidth: 28}, {Header: "Type", Path: "type", MaxWidth: 14}, {Header: "Values", Path: "values", MaxWidth: 36}, {Header: "Updated", Path: "updated_at", MaxWidth: 22}})
	case "database.schema.show":
		return renderSummaryDatabaseSchemaShow(w, theme, p)
	case "profile.list":
		return renderSummaryNamedDataList(w, theme, "Profiles", p.Data, []string{"profiles"}, []summaryListColumn{{Header: "Name", Path: "name", MaxWidth: 24}, {Header: "Endpoint", Path: "endpoint", MaxWidth: 42}, {Header: "Workspace", Path: "workspace", MaxWidth: 18}, {Header: "Device", Path: "device", MaxWidth: 18}, {Header: "Scope", Path: "default_scope", MaxWidth: 20}, {Header: "Default", Path: "default", MaxWidth: 10}})
	case "profile.show":
		return renderSummaryProfileShow(w, theme, p.Data)
	case "publish.doc.provider.list":
		return renderSummaryNamedDataList(w, theme, "Document publish providers", p.Data, []string{"providers"}, []summaryListColumn{{Header: "Target", Path: "target", MaxWidth: 18}, {Header: "Provider", Path: "provider", MaxWidth: 16}})
	case "publish.profile.validate":
		return renderSummaryNamedDataList(w, theme, "Publish profile issues", p.Data, []string{"issues"}, publishIssueSummaryColumns())
	case "publish.doctor":
		return renderSummaryNamedDataList(w, theme, "Publish issues", p.Data, []string{"issues"}, publishIssueSummaryColumns())
	case "publish.plan":
		return renderSummaryNamedDataList(w, theme, "Publish selected items", p.Data, []string{"plan", "selected"}, []summaryListColumn{{Header: "Note ID", Path: "id", MaxWidth: 24}, {Header: "Kind", Path: "kind", MaxWidth: 12}, {Header: "Title", Path: "title", MaxWidth: 32}, {Header: "Source", Path: "source_path", MaxWidth: 42}, {Header: "Output", Path: "output_path", MaxWidth: 42}})
	case "publish.doc.list":
		return renderSummaryNamedDataList(w, theme, "Document publish mappings", p.Data, []string{"mappings"}, []summaryListColumn{{Header: "Note ID", Path: "note_id", MaxWidth: 24}, {Header: "Target", Path: "target", MaxWidth: 16}, {Header: "Provider", Path: "provider", MaxWidth: 14}, {Header: "Status", Path: "publish_status", MaxWidth: 18}, {Header: "Renderer", Path: "renderer", MaxWidth: 16}, {Header: "Remote path", Path: "remote_path", MaxWidth: 42}})
	case "publish.theme.list":
		return renderSummaryNamedDataList(w, theme, "Themes", p.Data, []string{"themes"}, []summaryListColumn{{Header: "Theme", Path: "name", MaxWidth: 28}, {Header: "Source", Path: "source", MaxWidth: 28}, {Header: "Contract", Path: "contract_version", MaxWidth: 28}})
	case "publish.theme.eject":
		return renderSummaryNamedScalarList(w, theme, "Theme files", p.Data, []string{"files"}, "Path", 72)
	case "publish.profile.list":
		return renderSummaryNamedDataList(w, theme, "Publish profiles", p.Data, []string{"profiles"}, []summaryListColumn{{Header: "Profile", Path: "name", MaxWidth: 24}, {Header: "Target", Path: "target", MaxWidth: 18}, {Header: "Renderer", Path: "renderer", MaxWidth: 16}, {Header: "Title", Path: "site.title", MaxWidth: 28}, {Header: "Theme", Path: "site.theme.value", MaxWidth: 30}})
	case "repair.list":
		return renderSummaryRepairList(w, theme, p.Data)
	case "project.show":
		return renderSummaryProjectShow(w, theme, p.Data)
	case "project.list":
		return renderSummaryProjectList(w, theme, p)
	case "project.subproject.list":
		return renderSummarySubprojectList(w, theme, p)
	case "project.item.add", "project.item.move", "project.item.archive", "project.item.plan", "project.item.show":
		return renderSummaryProjectItem(w, theme, p.Data)
	case "project.board.show":
		return renderSummaryProjectBoard(w, p)
	case "tag.list", "kind.list", "group.list":
		return renderSummaryDimensionList(w, theme, p.Data)
	case "view.list", "database.view.list":
		return renderSummaryNamedDataList(w, theme, "Views", p.Data, []string{"views"}, []summaryListColumn{{Header: "View", Path: "name", MaxWidth: 26}, {Header: "Group", Path: "group", MaxWidth: 16}, {Header: "Kind", Path: "kind", MaxWidth: 16}, {Header: "Status", Path: "status", MaxWidth: 14}, {Header: "Sort", Path: "sort", MaxWidth: 16}, {Header: "Display", Path: "display.mode", MaxWidth: 14}})
	case "view.show", "database.view.show":
		return renderSummaryDataList(w, theme, p.Data, []string{"result", "notes"}, []summaryListColumn{{Header: "Path", Path: "path", MaxWidth: 56}, {Header: "Title", Path: "title", MaxWidth: 32}, {Header: "Kind", Path: "kind", MaxWidth: 14}, {Header: "Tags", Path: "tags", MaxWidth: 24}, {Header: "Status", Path: "status", MaxWidth: 12}, {Header: "Updated", Path: "updated_at", MaxWidth: 20}})
	case "organize.suggest":
		return renderSummaryOrganizePlan(w, theme, p.Data)
	case "organize.list":
		return renderSummaryOrganizePlanList(w, theme, p.Data)
	case "organize.plan":
		return renderSummaryLegacyOrganizePlan(w, theme, p.Data)
	case "note.show", "note.read", "note.preview", "daily.show", "weekly.show", "monthly.show", "template.show", "template.render", "template.preview":
		return renderSummaryMarkdownDocument(w, p.Data, opts)
	case "sync.conflicts.list":
		return renderSummarySyncConflictList(w, theme, p.Data)
	case "trash.list":
		return renderSummaryNamedDataList(w, theme, "Trash entries", p.Data, []string{"entries"}, []summaryListColumn{{Header: "Kind", Path: "object_kind", MaxWidth: 14}, {Header: "Object", Path: "object_id", MaxWidth: 28}, {Header: "Title", Path: "title", MaxWidth: 28}, {Header: "Trash path", Path: "trash_path", MaxWidth: 42}, {Header: "Deleted", Path: "deleted_at", MaxWidth: 22}})
	case "token.list":
		return renderSummaryNamedDataList(w, theme, "Tokens", p.Data, []string{"tokens"}, []summaryListColumn{{Header: "ID", Path: "id", MaxWidth: 24}, {Header: "Label", Path: "label", MaxWidth: 24}, {Header: "Created", Path: "created_at", MaxWidth: 22}, {Header: "Scope", Path: "scope", MaxWidth: 18}, {Header: "Expires", Path: "expires_at", MaxWidth: 22}})
	case "vault.doctor":
		return renderSummaryNamedDataList(w, theme, "Vault issues", p.Data, []string{"issues"}, issueSummaryColumns())
	case "index.doctor":
		return renderSummaryNamedDataList(w, theme, "Index issues", p.Data, []string{"issues"}, issueSummaryColumns())
	case "storage.doctor":
		return renderSummaryNamedDataList(w, theme, "Storage issues", p.Data, []string{"issues"}, issueSummaryColumns())
	case "vault.list":
		return renderSummaryVaultList(w, theme, p.Data)
	case "vault.remote.list":
		return renderSummaryVaultRemoteList(w, theme, p.Data)
	case "sync.conflicts.show":
		return renderSummarySyncConflictShow(w, theme, p.Data)
	case "sync.conflicts.diff":
		return renderSummarySyncConflictDiff(w, p.Data)
	default:
		return nil
	}
}

func renderSummaryConnectionReadiness(w io.Writer, theme summaryTheme, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	readiness, ok := dataMap(root["readiness"])
	if !ok {
		return nil
	}
	layers, ok := dataMap(readiness["layers"])
	if !ok {
		return nil
	}
	rows := make([][]string, 0, 6)
	for _, name := range []string{"contract", "transport", "auth", "owner", "mutation_recovery", "production"} {
		layer, ok := dataMap(layers[name])
		if !ok {
			continue
		}
		rows = append(rows, []string{
			readinessLayerLabel(name),
			summaryHumanValue("status", firstDataPathString(layer, "status")),
			summaryHumanValue("maturity", firstDataPathString(layer, "maturity")),
			joinedSummaryValues(layer["blockers"]),
			joinedSummaryValues(layer["next_actions"]),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Layer", "Status", "Maturity", "Blockers", "Next action"}, rows)
}

func readinessLayerLabel(name string) string {
	switch name {
	case "contract":
		return "Contract"
	case "transport":
		return "Transport"
	case "auth":
		return "Auth"
	case "owner":
		return "Owner"
	case "mutation_recovery":
		return "Mutation recovery"
	case "production":
		return "Production"
	default:
		return name
	}
}

func joinedSummaryValues(value any) string {
	switch typed := value.(type) {
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				items = append(items, text)
			}
		}
		return strings.Join(items, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func renderSummaryCommandCatalog(w io.Writer, theme summaryTheme, data any) error {
	items := dataListMaps(data, "commands")
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nCommands"); err != nil {
		return err
	}
	columns := []summaryListColumn{{Header: "Command", Path: "name", MaxWidth: 18}, {Header: "Level", Path: "visibility", MaxWidth: 10}, {Header: "Group", Path: "group", MaxWidth: 32}, {Header: "Description", Path: "summary", MaxWidth: 58}}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		row := make([]string, 0, len(columns))
		for _, column := range columns {
			row = append(row, summaryCell(summaryColumnValue(item, column), column.MaxWidth))
		}
		rows = append(rows, row)
	}
	return renderSummaryTable(w, theme, []string{"Command", "Level", "Group", "Description"}, rows)
}

func issueSummaryColumns() []summaryListColumn {
	return []summaryListColumn{{Header: "Severity", Path: "severity", MaxWidth: 12}, {Header: "Code", Paths: []string{"code", "issue_code"}, MaxWidth: 26}, {Header: "Path", Path: "path", MaxWidth: 42}, {Header: "Message", Path: "message", MaxWidth: 48}}
}

func publishIssueSummaryColumns() []summaryListColumn {
	return []summaryListColumn{{Header: "Severity", Path: "severity", MaxWidth: 12}, {Header: "Code", Path: "code", MaxWidth: 28}, {Header: "Field", Path: "field", MaxWidth: 18}, {Header: "Message", Path: "message", MaxWidth: 52}}
}

func folderIssueSummaryColumns() []summaryListColumn {
	return []summaryListColumn{{Header: "Code", Path: "code", MaxWidth: 26}, {Header: "Path", Path: "path", MaxWidth: 38}, {Header: "Operation", Path: "operation", MaxWidth: 18}, {Header: "Message", Path: "message", MaxWidth: 48}}
}

func deleteCandidateSummaryColumns() []summaryListColumn {
	return []summaryListColumn{{Header: "Run ID", Paths: []string{"run_id", "name"}, MaxWidth: 32}, {Header: "Command", Path: "command", MaxWidth: 28}, {Header: "Status", Path: "status", MaxWidth: 18}, {Header: "Template", Path: "template", MaxWidth: 20}, {Header: "Created", Path: "created_at", MaxWidth: 22}}
}

func renderSummaryPlanningSelectedTasks(w io.Writer, theme summaryTheme, data any) error {
	items := planningSelectedTaskRows(data)
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := renderSummaryTable(w, theme, []string{"Planning selected tasks"}, [][]string{{fmt.Sprintf("%d selected", len(items))}}); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	if len(items) > limit {
		if _, err := fmt.Fprintf(w, "  showing %d/%d\n", limit, len(items)); err != nil {
			return err
		}
	}
	rows := make([][]string, 0, limit)
	for _, item := range items[:limit] {
		rows = append(rows, []string{
			summaryCell(dataPathString(item, "id"), 28),
			summaryCell(dataPathString(item, "title"), 34),
			summaryCell(firstDataPathString(item, "section_title", "section_id"), 24),
			summaryCell(dataPathString(item, "priority"), 12),
			summaryCell(dataPathString(item, "reason"), 42),
		})
	}
	return renderSummaryTable(w, theme, []string{"Task ID", "Title", "Section", "Priority", "Reason"}, rows)
}

func planningSelectedTaskRows(data any) []map[string]any {
	tasks := dataListMaps(data, "snapshot", "task", "tasks")
	if len(tasks) == 0 {
		return nil
	}
	selected := dataListScalars(data, "decision", "selected")
	if len(selected) == 0 {
		return nil
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedSet[id] = true
		}
	}
	items := make([]map[string]any, 0, len(selected))
	for _, task := range tasks {
		if selectedSet[dataPathString(task, "id")] {
			items = append(items, task)
		}
	}
	return items
}

func renderSummaryRecordHistory(w io.Writer, theme summaryTheme, data any) error {
	record, ok := dataMap(data)
	if !ok {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := [][]string{
		{"Note ID", defaultString(dataPathString(record, "note_id"), "-")},
		{"Path", defaultString(dataPathString(record, "path"), "-")},
		{"Title", defaultString(dataPathString(record, "title"), "-")},
		{"Lifecycle", defaultString(dataPathString(record, "lifecycle"), "-")},
		{"Record version", defaultString(dataPathString(record, "record_version"), "-")},
		{"Ledger sequence", defaultString(dataPathString(record, "ledger_seq"), "-")},
	}
	return renderSummaryTable(w, theme, []string{"Record details", "Value"}, rows)
}

func renderSummaryProjectItem(w io.Writer, theme summaryTheme, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	item, ok := dataMap(root["item"])
	if !ok {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := [][]string{
		{"Item ID", defaultString(dataPathString(item, "item_id"), "-")},
		{"Title", defaultString(dataPathString(item, "title"), "-")},
		{"Column", defaultString(dataPathString(item, "column"), "-")},
		{"Path", defaultString(dataPathString(item, "path"), "-")},
		{"Project", defaultString(dataPathString(item, "project"), "-")},
		{"Status", defaultString(dataPathString(item, "status"), "-")},
		{"Source", defaultString(dataPathString(item, "source_kind"), "-")},
		{"Writable", summaryBool(dataPathString(item, "writable") == "true")},
	}
	return renderSummaryTable(w, theme, []string{"Project item", "Value"}, rows)
}

func renderSummaryNamedDataList(w io.Writer, theme summaryTheme, title string, data any, listPath []string, columns []summaryListColumn) error {
	items := dataListMaps(data, listPath...)
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render(title)); err != nil {
		return err
	}
	return renderSummaryDataList(w, theme, data, listPath, columns)
}

func renderSummaryNamedScalarList(w io.Writer, theme summaryTheme, title string, data any, listPath []string, header string, maxWidth int) error {
	items := dataListScalars(data, listPath...)
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render(title)); err != nil {
		return err
	}
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	rows := make([][]string, 0, limit)
	for _, item := range items[:limit] {
		rows = append(rows, []string{summaryCell(item, maxWidth)})
	}
	return renderSummaryTable(w, theme, []string{header}, rows)
}

func renderSummaryActivityList(w io.Writer, theme summaryTheme, data any) error {
	if err := renderSummaryDataList(w, theme, data, []string{"entries"}, []summaryListColumn{{Header: "Event ID", Path: "event_id", MaxWidth: 28}, {Header: "Source", Path: "source", MaxWidth: 18}, {Header: "Kind", Path: "kind", MaxWidth: 24}, {Header: "Status", Path: "status", MaxWidth: 12}, {Header: "Object", Path: "object_ref", MaxWidth: 28}, {Header: "Time", Path: "ts", MaxWidth: 22}}); err != nil {
		return err
	}
	return renderSummaryNamedDataList(w, theme, "Activity warnings", data, []string{"warnings"}, []summaryListColumn{{Header: "Source", Path: "source", MaxWidth: 18}, {Header: "Path", Path: "path", MaxWidth: 42}, {Header: "Line", Path: "line", MaxWidth: 8}, {Header: "Message", Path: "message", MaxWidth: 56}})
}

func renderSummaryMonitorRuns(w io.Writer, theme summaryTheme, data any) error {
	if err := renderSummaryDataList(w, theme, data, []string{"runs"}, []summaryListColumn{{Header: "Run ID", Path: "run_id", MaxWidth: 28}, {Header: "Command", Path: "command", MaxWidth: 30}, {Header: "Status", Path: "status", MaxWidth: 12}, {Header: "Duration", Path: "duration_ms", MaxWidth: 12}, {Header: "Started", Path: "started_at", MaxWidth: 22}}); err != nil {
		return err
	}
	return renderSummaryNamedDataList(w, theme, "Monitor warnings", data, []string{"warnings"}, []summaryListColumn{{Header: "Source", Path: "source", MaxWidth: 18}, {Header: "Path", Path: "path", MaxWidth: 42}, {Header: "Line", Path: "line", MaxWidth: 8}, {Header: "Message", Path: "message", MaxWidth: 56}})
}

func renderSummaryAPIRoutes(w io.Writer, theme summaryTheme, data any) error {
	routes := dataListMaps(data, "routes")
	if len(routes) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("API routes")); err != nil {
		return err
	}
	rows := make([][]string, 0, len(routes))
	for _, route := range routes {
		endpoint := firstDataPathString(route, "path", "rpc_method")
		rows = append(rows, []string{summaryCell(firstDataPathString(route, "method"), 8), summaryCell(endpoint, 42), summaryCell(firstDataPathString(route, "command"), 34), summaryCell(firstDataPathString(route, "surface"), 10)})
	}
	return renderSummaryTable(w, theme, []string{"Method", "Endpoint", "Command", "Surface"}, rows)
}

func renderSummaryProjectShow(w io.Writer, theme summaryTheme, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	project, ok := dataMap(root["project"])
	if !ok {
		return nil
	}
	rows := [][]string{
		{"Slug", defaultString(firstDataPathString(project, "slug"), "-")},
		{"Name", defaultString(firstDataPathString(project, "name"), "-")},
		{"Description", defaultString(firstDataPathString(project, "description"), "-")},
		{"Notes prefix", defaultString(firstDataPathString(project, "notes_prefix"), "-")},
		{"Created", defaultString(firstDataPathString(project, "created_at"), "-")},
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Project details")); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Field", "Value"}, rows)
}

func renderSummaryAssetShow(w io.Writer, theme summaryTheme, data any) error {
	asset := assetMapFromData(data)
	if asset == nil {
		return nil
	}
	rows := [][]string{
		{"Path", defaultString(firstDataPathString(asset, "path"), "-")},
		{"Filename", defaultString(firstDataPathString(asset, "filename"), "-")},
		{"Media type", defaultString(firstDataPathString(asset, "media_type"), "-")},
		{"Size", defaultString(firstDataPathString(asset, "size_bytes", "size"), "-")},
		{"Status", defaultString(firstDataPathString(asset, "managed_status"), "-")},
		{"SHA-256", defaultString(firstDataPathString(asset, "sha256"), "-")},
	}
	if displayPath := firstDataPathString(asset, "display_path"); displayPath != "" {
		rows = append(rows, []string{"Display path", displayPath})
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Asset details")); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Field", "Value"}, rows)
}

func renderSummaryDatabaseSchemaShow(w io.Writer, theme summaryTheme, p domain.Projection) error {
	root, ok := dataMap(p.Data)
	if !ok {
		return nil
	}
	property, ok := dataMap(root["property"])
	if !ok {
		return nil
	}
	validation, _ := dataMap(root["validation"])
	rows := [][]string{
		{"Property", defaultString(p.Facts["property"], "-")},
		{"Type", defaultString(firstDataPathString(property, "type"), "-")},
		{"Values", defaultString(firstDataPathString(property, "values"), "-")},
		{"Updated", defaultString(firstDataPathString(property, "updated_at"), "-")},
		{"Validation", defaultString(firstDataPathString(validation, "status"), "-")},
		{"Checked values", defaultString(firstDataPathString(validation, "checked_values"), "0")},
		{"Invalid values", defaultString(firstDataPathString(validation, "invalid_values"), "0")},
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Property schema")); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Field", "Value"}, rows)
}

func renderSummaryProfileShow(w io.Writer, theme summaryTheme, data any) error {
	profile := profileMapFromData(data)
	if profile == nil {
		return nil
	}
	rows := [][]string{
		{"Profile", defaultString(firstDataPathString(profile, "name"), "-")},
		{"Endpoint", defaultString(firstDataPathString(profile, "endpoint"), "-")},
		{"Workspace", defaultString(firstDataPathString(profile, "workspace"), "-")},
		{"Device", defaultString(firstDataPathString(profile, "device"), "-")},
		{"Scope", defaultString(firstDataPathString(profile, "default_scope"), "-")},
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Profile details")); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Field", "Value"}, rows)
}

func renderSummaryBackendShow(w io.Writer, theme summaryTheme, data any) error {
	profile := profileMapFromData(data)
	if profile == nil {
		return nil
	}
	rows := [][]string{
		{"Name", defaultString(firstDataPathString(profile, "name"), "-")},
		{"Kind", defaultString(firstDataPathString(profile, "kind"), "-")},
		{"Bucket", defaultString(firstDataPathString(profile, "bucket"), "-")},
		{"Region", defaultString(firstDataPathString(profile, "region"), "-")},
		{"Prefix", defaultString(firstDataPathString(profile, "prefix"), "-")},
		{"Profile", defaultString(firstDataPathString(profile, "profile"), "-")},
		{"Credential", defaultString(firstDataPathString(profile, "credential_source"), "-")},
		{"Capabilities", defaultString(firstDataPathString(profile, "capabilities"), "-")},
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Backend details")); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Field", "Value"}, rows)
}

func assetMapFromData(data any) map[string]any {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	asset, ok := dataMap(root["asset"])
	if !ok {
		return nil
	}
	return asset
}

func profileMapFromData(data any) map[string]any {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	profile, ok := dataMap(root["profile"])
	if !ok {
		return nil
	}
	return profile
}

func renderSummaryVaultList(w io.Writer, theme summaryTheme, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	defaultAlias, _ := root["default"].(string)
	locals, ok := dataMap(root["locals"])
	if ok && len(locals) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, theme.header.Render("Local vaults")); err != nil {
			return err
		}
		aliases := sortedMapKeys(locals)
		rows := make([][]string, 0, len(aliases))
		for _, alias := range aliases {
			entry, _ := dataMap(locals[alias])
			isDefault := ""
			if alias == defaultAlias {
				isDefault = "*"
			}
			rows = append(rows, []string{isDefault, summaryCell(alias, 24), summaryCell(firstDataPathString(entry, "name"), 28), summaryCell(firstDataPathString(entry, "path"), 96)})
		}
		if err := renderSummaryTable(w, theme, []string{"Default", "Alias", "Name", "Path"}, rows); err != nil {
			return err
		}
	}
	remoteCache, ok := dataMap(root["remote_cache"])
	if !ok || len(remoteCache) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Remote vaults")); err != nil {
		return err
	}
	rows := [][]string{}
	for _, profile := range sortedMapKeys(remoteCache) {
		entry, _ := dataMap(remoteCache[profile])
		vaults := dataListMaps(entry, "vaults")
		for _, vault := range vaults {
			rows = append(rows, []string{summaryCell(profile, 20), summaryCell(firstDataPathString(vault, "selector"), 28), summaryCell(firstDataPathString(vault, "label"), 28), summaryCell(firstDataPathString(vault, "workspace"), 24), summaryCell(firstDataPathString(vault, "revision"), 18)})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return renderSummaryTable(w, theme, []string{"Profile", "Selector", "Label", "Workspace", "Revision"}, rows)
}

func renderSummaryVaultRemoteList(w io.Writer, theme summaryTheme, data any) error {
	items := remoteVaultRows(data)
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Remote vaults")); err != nil {
		return err
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{summaryCell(firstDataPathString(item, "profile"), 20), summaryCell(firstDataPathString(item, "selector"), 28), summaryCell(firstDataPathString(item, "label"), 28), summaryCell(firstDataPathString(item, "workspace"), 24), summaryCell(firstDataPathString(item, "revision"), 18)})
	}
	return renderSummaryTable(w, theme, []string{"Profile", "Selector", "Label", "Workspace", "Revision"}, rows)
}

func remoteVaultRows(data any) []map[string]any {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var root struct {
		Profiles map[string]struct {
			Profile   string `json:"profile"`
			Workspace string `json:"workspace"`
			Vaults    []struct {
				ID        string `json:"id"`
				Label     string `json:"label"`
				Selector  string `json:"selector"`
				Workspace string `json:"workspace"`
				Revision  string `json:"revision"`
			} `json:"vaults"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(payload, &root); err != nil || len(root.Profiles) == 0 {
		return nil
	}
	profiles := make([]string, 0, len(root.Profiles))
	for profile := range root.Profiles {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	items := []map[string]any{}
	for _, profileName := range profiles {
		entry := root.Profiles[profileName]
		profileValue := entry.Profile
		if profileValue == "" {
			profileValue = profileName
		}
		for _, vault := range entry.Vaults {
			workspace := vault.Workspace
			if workspace == "" {
				workspace = entry.Workspace
			}
			items = append(items, map[string]any{"profile": profileValue, "selector": vault.Selector, "label": vault.Label, "workspace": workspace, "revision": vault.Revision, "id": vault.ID})
		}
	}
	return items
}

func renderSummaryRepairList(w io.Writer, theme summaryTheme, data any) error {
	plans := dataListMaps(data, "plans")
	if len(plans) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Repair plans")); err != nil {
		return err
	}
	limit := len(plans)
	if limit > 10 {
		limit = 10
	}
	rows := make([][]string, 0, limit)
	for _, plan := range plans[:limit] {
		rows = append(rows, []string{summaryCell(firstDataPathString(plan, "plan_id"), 28), summaryCell(firstDataPathString(plan, "status"), 14), fmt.Sprint(dataPathLen(plan, "operations")), summaryCell(firstDataPathString(plan, "created_at"), 22), summaryCell(firstDataPathString(plan, "expires_at"), 22)})
	}
	return renderSummaryTable(w, theme, []string{"Plan ID", "Status", "Operations", "Created", "Expires"}, rows)
}

func dataPathLen(item map[string]any, path string) int {
	value := dataPathValue(item, path)
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		if value == nil {
			return 0
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return 0
		}
		var items []any
		if err := json.Unmarshal(payload, &items); err != nil {
			return 0
		}
		return len(items)
	}
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderSummaryProjectList(w io.Writer, theme summaryTheme, p domain.Projection) error {
	if p.Command == "project.subproject.list" {
		return renderSummarySubprojectList(w, theme, p)
	}
	if p.Status == "success" {
		if err := renderSummaryTable(w, theme, []string{"Highlights"}, [][]string{{defaultString(p.Summary, "-")}}); err != nil {
			return err
		}
	} else if err := renderSummaryTable(w, theme, []string{"Status", "Highlights"}, [][]string{{summaryStatusCell(theme, p.Status), defaultString(p.Summary, "-")}}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	metricRows := [][]string{{"Current project", defaultString(p.Facts["current_project"], "-")}, {"Projects", defaultString(p.Facts["projects"], "0")}}
	if vault := p.Facts["vault"]; vault != "" {
		metricRows = append(metricRows, []string{"Vault", vault})
	}
	if err := renderSummaryTable(w, theme, []string{"Metric", "Value"}, metricRows); err != nil {
		return err
	}

	registry, ok := projectRegistryFromData(p.Data)
	if ok && len(registry.Projects) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		limit := len(registry.Projects)
		if limit > 10 {
			limit = 10
		}
		rows := make([][]string, 0, limit)
		for _, project := range registry.Projects[:limit] {
			current := ""
			if project.Slug == registry.CurrentProject {
				current = "*"
			}
			rows = append(rows, []string{current, summaryCell(project.Slug, 24), summaryCell(project.Name, 28), summaryCell(project.NotesPrefix, 36), summaryCell(project.Description, 44)})
		}
		if err := renderSummaryTable(w, theme, []string{"Current", "Slug", "Name", "Notes prefix", "Description"}, rows); err != nil {
			return err
		}
	}

	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		return renderSummaryTable(w, theme, []string{"Recommended next step"}, [][]string{{p.Actions[0].Command}})
	}
	return nil
}

func renderSummarySubprojectList(w io.Writer, theme summaryTheme, p domain.Projection) error {
	if p.Status == "success" {
		if err := renderSummaryTable(w, theme, []string{"Highlights"}, [][]string{{defaultString(p.Summary, "-")}}); err != nil {
			return err
		}
	} else if err := renderSummaryTable(w, theme, []string{"Status", "Highlights"}, [][]string{{summaryStatusCell(theme, p.Status), defaultString(p.Summary, "-")}}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := renderSummaryTable(w, theme, []string{"Metric", "Value"}, [][]string{{"Project", defaultString(p.Facts["project"], "-")}, {"Subprojects", defaultString(p.Facts["subprojects"], "0")}}); err != nil {
		return err
	}

	items := dataListMaps(p.Data, "subprojects")
	if len(items) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		rows := make([][]string, 0, limit)
		for _, item := range items[:limit] {
			rows = append(rows, []string{summaryCell(firstDataPathString(item, "project"), 20), summaryCell(firstDataPathString(item, "subproject"), 28), summaryCell(firstDataPathString(item, "title"), 32), summaryCell(firstDataPathString(item, "workspace_path"), 48), summaryCell(firstDataPathString(item, "template"), 18)})
		}
		if err := renderSummaryTable(w, theme, []string{"Project", "Subproject", "Title", "Workspace path", "Template"}, rows); err != nil {
			return err
		}
	}

	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		return renderSummaryTable(w, theme, []string{"Recommended next step"}, [][]string{{p.Actions[0].Command}})
	}
	return nil
}

func projectRegistryFromData(data any) (domain.ProjectRegistry, bool) {
	if data == nil {
		return domain.ProjectRegistry{}, false
	}
	if registry, ok := data.(domain.ProjectRegistry); ok {
		return registry, true
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return domain.ProjectRegistry{}, false
	}
	var root struct {
		Registry domain.ProjectRegistry `json:"registry"`
	}
	if err := json.Unmarshal(payload, &root); err != nil {
		return domain.ProjectRegistry{}, false
	}
	if len(root.Registry.Projects) == 0 && root.Registry.SchemaVersion == "" && root.Registry.CurrentProject == "" {
		return domain.ProjectRegistry{}, false
	}
	return root.Registry, true
}

type summarySearchData struct {
	Results []summarySearchResult `json:"results"`
	Notes   []domain.Note         `json:"notes"`
	Facets  *searchFacetsSummary  `json:"facets"`
}

type summarySearchResult struct {
	Note    domain.Note `json:"note"`
	Snippet string      `json:"snippet"`
	Trust   string      `json:"trust"`
	Fresh   string      `json:"fresh"`
}

// searchFacetsSummary 是 search facets 的渲染投影（与 searchops.SearchFacets 同构）。
type searchFacetsSummary struct {
	Tag    []searchFacetValue `json:"tag"`
	Kind   []searchFacetValue `json:"kind"`
	Status []searchFacetValue `json:"status"`
	Folder []searchFacetValue `json:"folder"`
	Trust  []searchFacetValue `json:"trust"`
	Fresh  []searchFacetValue `json:"fresh"`
}

type searchFacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type summaryListColumn struct {
	Header   string
	Path     string
	Paths    []string
	MaxWidth int
}

func renderSummaryDataList(w io.Writer, theme summaryTheme, data any, listPath []string, columns []summaryListColumn) error {
	items := dataListMaps(data, listPath...)
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	if len(items) > limit {
		if _, err := fmt.Fprintf(w, "  showing %d/%d\n", limit, len(items)); err != nil {
			return err
		}
	}
	headers := make([]string, 0, len(columns))
	for _, column := range columns {
		headers = append(headers, column.Header)
	}
	rows := make([][]string, 0, limit)
	for _, item := range items[:limit] {
		row := make([]string, 0, len(columns))
		for _, column := range columns {
			row = append(row, summaryCell(summaryColumnValue(item, column), column.MaxWidth))
		}
		rows = append(rows, row)
	}
	return renderSummaryTable(w, theme, headers, rows)
}

func summaryColumnValue(item map[string]any, column summaryListColumn) string {
	if len(column.Paths) > 0 {
		return firstDataPathString(item, column.Paths...)
	}
	return dataPathString(item, column.Path)
}

func dataListMaps(data any, path ...string) []map[string]any {
	if data == nil || len(path) == 0 {
		return nil
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil
	}
	current := root
	for _, part := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
	}
	items, ok := current.([]any)
	if !ok {
		return nil
	}
	maps := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			maps = append(maps, obj)
		}
	}
	return maps
}

func dataListScalars(data any, path ...string) []string {
	if data == nil || len(path) == 0 {
		return nil
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil
	}
	current := root
	for _, part := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
	}
	items, ok := current.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := agentScalarValue(item)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func dataPathString(item map[string]any, path string) string {
	return agentScalarValue(dataPathValue(item, path))
}

func dataPathValue(item map[string]any, path string) any {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var current any = item
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
		if current == nil {
			return nil
		}
	}
	return current
}

func renderSummarySearchResults(w io.Writer, theme summaryTheme, data any) error {
	results, facets := summarySearchResultsFromData(data)
	trustAware := false
	for _, result := range results {
		if strings.TrimSpace(result.Trust) != "" || strings.TrimSpace(result.Fresh) != "" {
			trustAware = true
			break
		}
	}
	if len(results) == 0 && facets == nil {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if len(results) > 0 {
		rows := make([][]string, 0, len(results))
		if trustAware {
			for _, result := range results {
				rows = append(rows, []string{
					summaryCell(result.Note.Path, 48),
					summaryCell(result.Note.Title, 28),
					summaryCell(trustBadge(result.Trust), 12),
					summaryCell(freshBadge(result.Fresh), 8),
					summaryCell(result.Snippet, 64),
				})
			}
			if err := renderSummaryTable(w, theme, []string{"Path", "Title", "Trust", "Fresh", "Preview"}, rows); err != nil {
				return err
			}
		} else {
			for _, result := range results {
				rows = append(rows, []string{
					summaryCell(result.Note.Path, 56),
					summaryCell(result.Note.Title, 32),
					summaryCell(result.Snippet, 80),
				})
			}
			if err := renderSummaryTable(w, theme, []string{"Path", "Title", "Preview"}, rows); err != nil {
				return err
			}
		}
	}
	if facets != nil {
		if err := renderSearchFacetsBlock(w, theme, facets); err != nil {
			return err
		}
	}
	return nil
}

// trustBadge/freshBadge 输出 ASCII 徽标（notty/markdown 场景由 summaryCell 保留纯文本标签）。
func trustBadge(tier string) string {
	switch strings.TrimSpace(tier) {
	case domain.TrustTierHuman:
		return "human ✓"
	case domain.TrustTierMachine:
		return "machine"
	case domain.TrustTierUnverified:
		return "unverified"
	default:
		return "-"
	}
}

func freshBadge(fresh string) string {
	if strings.TrimSpace(fresh) == domain.FreshnessStale {
		return "stale"
	}
	if strings.TrimSpace(fresh) == domain.FreshnessFresh {
		return "fresh"
	}
	return "-"
}

// renderSearchFacetsBlock 渲染 facet 计数块（置于结果表之后）。
func renderSearchFacetsBlock(w io.Writer, theme summaryTheme, facets *searchFacetsSummary) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Facets")); err != nil {
		return err
	}
	dimensions := []struct {
		name   string
		values []searchFacetValue
	}{
		{"tag", facets.Tag},
		{"kind", facets.Kind},
		{"status", facets.Status},
		{"folder", facets.Folder},
		{"trust", facets.Trust},
		{"fresh", facets.Fresh},
	}
	for _, dimension := range dimensions {
		if len(dimension.values) == 0 {
			continue
		}
		parts := make([]string, 0, len(dimension.values))
		for _, value := range dimension.values {
			parts = append(parts, fmt.Sprintf("%s=%d", value.Value, value.Count))
		}
		if _, err := fmt.Fprintf(w, "  %-7s %s\n", dimension.name, strings.Join(parts, " ")); err != nil {
			return err
		}
	}
	return nil
}

// renderSummarySearchShow 渲染 search show 一站式详情卡（有界 snippet，不输出正文全文）。
func renderSummarySearchShow(w io.Writer, theme summaryTheme, data any) error {
	var card searchShowCard
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(b, &card); err != nil {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	heading := card.Title
	if heading == "" {
		heading = card.Path
	}
	if _, err := fmt.Fprintln(w, theme.header.Render(heading), card.Kind, card.Path); err != nil {
		return err
	}
	trustLine := trustBadge(card.Trust.Tier)
	if card.Trust.LatestHumanBy != "" {
		trustLine += " (" + card.Trust.LatestHumanBy + " @ " + trustEventDate(card.Trust.LatestHumanAt) + ")"
	} else if card.Trust.VerifiedAtLatest != "" {
		trustLine += " (latest verify " + trustEventDate(card.Trust.VerifiedAtLatest) + ")"
	} else if card.Trust.GeneratedBy != "" {
		trustLine += " (generated by " + card.Trust.GeneratedBy + ")"
	}
	freshLine := freshBadge(card.Fresh)
	if card.Trust.StaleAfter != "" {
		freshLine += " (stale_after " + trustEventDate(card.Trust.StaleAfter) + ")"
	}
	lines := [][2]string{
		{"Trust", trustLine},
		{"Fresh", freshLine},
		{"Meta", searchShowMetaLine(card)},
	}
	if card.Snippet != "" {
		lines = append(lines, [2]string{"Snippet", card.Snippet})
	}
	outLine := fmt.Sprintf("%d out", card.Links.Outgoing)
	if len(card.Links.OutgoingRefs) > 0 {
		outLine += " (" + searchShowRefNames(card.Links.OutgoingRefs) + ")"
	}
	inLine := fmt.Sprintf("%d in", card.Links.Incoming)
	if len(card.Links.IncomingRefs) > 0 {
		inLine += " (" + searchShowRefNames(card.Links.IncomingRefs) + ")"
	}
	lines = append(lines, [2]string{"Links", outLine + " - " + inLine})
	if len(card.Neighbors) > 0 {
		lines = append(lines, [2]string{"Neighbors", "same-tag: " + searchShowRefNames(card.Neighbors)})
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "  %-9s %s\n", line[0], line[1]); err != nil {
			return err
		}
	}
	return nil
}

func searchShowMetaLine(card searchShowCard) string {
	parts := make([]string, 0, 5)
	if len(card.Tags) > 0 {
		parts = append(parts, "tags="+strings.Join(card.Tags, ","))
	}
	if card.Kind != "" {
		parts = append(parts, "kind="+card.Kind)
	}
	if card.Status != "" {
		parts = append(parts, "status="+card.Status)
	}
	if card.UpdatedAt != "" {
		parts = append(parts, "updated="+trustEventDate(card.UpdatedAt))
	}
	return strings.Join(parts, "  ")
}

func searchShowRefNames(refs []searchShowRefCard) string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Title != "" {
			names = append(names, ref.Title)
			continue
		}
		names = append(names, ref.Path)
	}
	return strings.Join(names, ", ")
}

func trustEventDate(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format("2006-01-02")
	}
	return value
}

type searchShowCard struct {
	Title     string              `json:"title"`
	Path      string              `json:"path"`
	Kind      string              `json:"kind"`
	Status    string              `json:"status"`
	UpdatedAt string              `json:"updated_at"`
	Tags      []string            `json:"tags"`
	Trust     searchShowTrustCard `json:"trust"`
	Fresh     string              `json:"fresh"`
	Snippet   string              `json:"snippet"`
	Links     searchShowLinksCard `json:"links"`
	Neighbors []searchShowRefCard `json:"neighbors"`
}

type searchShowTrustCard struct {
	Tier             string `json:"tier"`
	GeneratedBy      string `json:"generated_by"`
	GeneratedAt      string `json:"generated_at"`
	VerifiedCount    int    `json:"verified_count"`
	VerifiedAtLatest string `json:"verified_at_latest"`
	LatestHumanBy    string `json:"latest_human_by"`
	LatestHumanAt    string `json:"latest_human_at"`
	StaleAfter       string `json:"stale_after"`
}

type searchShowLinksCard struct {
	Outgoing     int                 `json:"outgoing"`
	OutgoingRefs []searchShowRefCard `json:"outgoing_refs"`
	Incoming     int                 `json:"incoming"`
	IncomingRefs []searchShowRefCard `json:"incoming_refs"`
}

type searchShowRefCard struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// renderSummaryBrowse 渲染合成目录导航视图（子目录 + 该层 notes）。
func renderSummaryBrowse(w io.Writer, theme summaryTheme, data any) error {
	var view browseCard
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(b, &view); err != nil {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	label := view.Path
	if label == "" {
		label = "(vault root)"
	}
	if _, err := fmt.Fprintln(w, theme.header.Render(label), fmt.Sprintf("- %d notes - %d subfolders", view.Notes, len(view.Subfolders))); err != nil {
		return err
	}
	if len(view.Subfolders) > 0 {
		if _, err := fmt.Fprintln(w, theme.header.Render("Subfolders")); err != nil {
			return err
		}
		rows := make([][]string, 0, len(view.Subfolders))
		for _, folder := range view.Subfolders {
			rows = append(rows, []string{summaryCell(folder.Path+"/", 48), fmt.Sprintf("%d notes", folder.NoteCount)})
		}
		if err := renderSummaryTable(w, theme, []string{"Folder", "Notes"}, rows); err != nil {
			return err
		}
	}
	if len(view.Items) > 0 {
		if _, err := fmt.Fprintln(w, theme.header.Render("Notes (by updated)")); err != nil {
			return err
		}
		rows := make([][]string, 0, len(view.Items))
		for _, item := range view.Items {
			rows = append(rows, []string{
				summaryCell(item.Title, 32),
				summaryCell(item.Kind, 14),
				summaryCell(trustBadge(item.Trust), 12),
				summaryCell(freshBadge(item.Fresh), 8),
				summaryCell(item.UpdatedAt, 22),
			})
		}
		if err := renderSummaryTable(w, theme, []string{"Title", "Kind", "Trust", "Fresh", "Updated"}, rows); err != nil {
			return err
		}
	}
	return nil
}

type browseCard struct {
	Path       string                `json:"path"`
	Notes      int                   `json:"notes"`
	Subfolders []browseSubfolderCard `json:"subfolders"`
	Items      []browseItemCard      `json:"items"`
}

type browseSubfolderCard struct {
	Path      string `json:"path"`
	NoteCount int    `json:"note_count"`
}

type browseItemCard struct {
	Title       string `json:"title"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	UpdatedAt   string `json:"updated_at"`
	Trust       string `json:"trust"`
	Fresh       string `json:"fresh"`
	Description string `json:"description"`
}

func summarySearchResultsFromData(data any) ([]summarySearchResult, *searchFacetsSummary) {
	var searchData summarySearchData
	b, err := json.Marshal(data)
	if err != nil {
		return nil, nil
	}
	if err := json.Unmarshal(b, &searchData); err != nil {
		return nil, nil
	}
	if len(searchData.Results) > 0 {
		return searchData.Results, searchData.Facets
	}
	results := make([]summarySearchResult, 0, len(searchData.Notes))
	for _, note := range searchData.Notes {
		results = append(results, summarySearchResult{Note: note})
	}
	return results, searchData.Facets
}

func renderSummaryMarkdownDocument(w io.Writer, data any, opts RenderOptions) error {
	body := summaryBodyFromData(data)
	if strings.TrimSpace(body) == "" {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return renderMarkdownBody(w, body, opts)
}

func summaryBodyFromData(data any) string {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	if body, ok := dataMap["body"].(string); ok {
		return body
	}
	if note, ok := dataMap["note"].(domain.Note); ok {
		return note.Body
	}
	return ""
}

func renderSummaryNoteList(w io.Writer, theme summaryTheme, data any, key string) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	notes, ok := dataMap[key].([]domain.Note)
	if !ok || len(notes) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(notes))
	for i := 0; i < len(notes); i++ {
		note := notes[i]
		tags := "-"
		if len(note.Tags) > 0 {
			tags = "#" + strings.Join(note.Tags, ",#")
		}
		rows = append(rows, []string{
			summaryCell(note.Path, 56),
			summaryCell(note.Title, 32),
			summaryCell(summaryHumanValue("note.kind", note.Kind), 10),
			summaryCell(tags, 22),
			summaryCell(summaryHumanValue("note.status", note.Status), 10),
			summaryCell(note.UpdatedAt, 20),
		})
	}
	return renderSummaryTable(w, theme, []string{"Path", "Title", "Kind", "Tags", "Status", "Updated"}, rows)
}

func renderSummaryLinkList(w io.Writer, theme summaryTheme, data any, key string) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	links, ok := dataMap[key].([]domain.NoteLink)
	if !ok || len(links) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(links))
	for _, link := range links {
		status := link.Status
		if status == "" {
			status = "broken"
			if link.TargetPath != "" {
				status = "resolved"
			}
		}
		rows = append(rows, []string{
			summaryCell(link.SourcePath, 48),
			summaryCell(link.Target, 32),
			summaryCell(link.TargetPath, 48),
			summaryHumanValue("link.status", status),
		})
	}
	return renderSummaryTable(w, theme, []string{"Source", "Target", "Path", "Status"}, rows)
}

func renderSummaryDimensionList(w io.Writer, theme summaryTheme, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	items, ok := dataMap["items"].([]domain.DimensionCount)
	if !ok || len(items) == 0 {
		return nil
	}
	dimension, _ := dataMap["dimension"].(string)
	labels := make([]string, 0, len(items))
	total := 0
	maxCount := 0
	for _, item := range items {
		label := summaryDimensionLabel(dimension, item.Value)
		labels = append(labels, label)
		total += item.Count
		if item.Count > maxCount {
			maxCount = item.Count
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(items))
	for i, item := range items {
		rows = append(rows, []string{labels[i], fmt.Sprint(item.Count), summaryPercent(item.Count, total), summaryBar(item.Count, maxCount, 10)})
	}
	return renderSummaryTable(w, theme, []string{summaryDimensionHeader(dimension), "Count", "Share", "Heat"}, rows)
}

func renderSummaryFolderList(w io.Writer, theme summaryTheme, data any) error {
	folders := summaryFoldersFromData(data, "folders")
	if len(folders) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(folders))
	for _, folder := range folders {
		rows = append(rows, folderSummaryRow(folder))
	}
	return renderSummaryTable(w, theme, []string{"Path", "Purpose", "Managed", "Exists", "Empty", "Notes", "Assets", "Depth"}, rows)
}

func renderSummaryFolderShow(w io.Writer, theme summaryTheme, data any) error {
	children := summaryFoldersFromData(data, "children")
	if len(children) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(children))
	for _, child := range children {
		rows = append(rows, folderSummaryRow(child))
	}
	return renderSummaryTable(w, theme, []string{"Children", "Purpose", "Managed", "Exists", "Empty", "Notes", "Assets", "Depth"}, rows)
}

func folderSummaryRow(folder domain.FolderInfo) []string {
	return []string{
		summaryCell(folder.Path, 56),
		summaryCell(string(folder.Purpose), 12),
		summaryCell(string(folder.ManagedStatus), 14),
		summaryBool(folder.Exists),
		summaryBool(folder.Empty),
		fmt.Sprint(folder.NoteCount),
		fmt.Sprint(folder.AssetCount),
		fmt.Sprint(folder.Depth),
	}
}

func summaryFoldersFromData(data any, key string) []domain.FolderInfo {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	value, ok := dataMap[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []domain.FolderInfo:
		return typed
	case []any:
		folders := make([]domain.FolderInfo, 0, len(typed))
		for _, item := range typed {
			b, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var folder domain.FolderInfo
			if err := json.Unmarshal(b, &folder); err == nil && folder.Path != "" {
				folders = append(folders, folder)
			}
		}
		return folders
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var folders []domain.FolderInfo
		if err := json.Unmarshal(b, &folders); err != nil {
			return nil
		}
		return folders
	}
}

func summaryBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func renderSummaryProjectBoard(w io.Writer, p domain.Projection) error {
	board, ok := summaryProjectBoardFromData(p.Data)
	if !ok {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	projectLine := "Project: " + board.ProjectSlug
	if board.Subproject != "" {
		projectLine += " / " + board.Subproject
	}
	if _, err := fmt.Fprintln(w, projectLine); err != nil {
		return err
	}
	if board.WorkspacePath != "" {
		if _, err := fmt.Fprintln(w, "Path: "+board.WorkspacePath); err != nil {
			return err
		}
	}
	if board.Workspace != nil && len(board.Workspace.Directories) > 0 {
		parts := make([]string, 0, len(board.Workspace.Directories))
		for _, dir := range board.Workspace.Directories {
			parts = append(parts, dir.Name+" "+dir.Status)
		}
		if _, err := fmt.Fprintln(w, "Structure: "+strings.Join(parts, " | ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Board: inbox %d | next %d | doing %d | blocked %d | review %d | done %d\n", board.Facts.Inbox, board.Facts.Next, board.Facts.Doing, board.Facts.Blocked, board.Facts.Review, board.Facts.Done); err != nil {
		return err
	}
	for _, column := range []string{"inbox", "next", "doing", "blocked", "review"} {
		items := boardItemsForColumn(board.Items, column)
		if len(items) == 0 {
			continue
		}
		if _, err := fmt.Fprintln(w, "\n"+boardColumnSummaryName(column)); err != nil {
			return err
		}
		limit := len(items)
		if limit > 5 {
			limit = 5
		}
		for _, item := range items[:limit] {
			if _, err := fmt.Fprintln(w, "- "+summaryBoardItemLine(item)); err != nil {
				return err
			}
		}
		if len(items) > limit {
			if _, err := fmt.Fprintf(w, "... %d more, use --json for full list\n", len(items)-limit); err != nil {
				return err
			}
		}
	}
	if len(board.Items) == 0 {
		if _, err := fmt.Fprintln(w, "\nNo project items yet."); err != nil {
			return err
		}
	}
	if board.Facts.Blocked > 0 || board.Facts.Review > 0 || len(board.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\nRisks"); err != nil {
			return err
		}
		if board.Facts.Blocked > 0 {
			if _, err := fmt.Fprintf(w, "- %d blocked item needs owner review.\n", board.Facts.Blocked); err != nil {
				return err
			}
		}
		if board.Facts.Review > 0 {
			if _, err := fmt.Fprintf(w, "- %d review item may become reusable output.\n", board.Facts.Review); err != nil {
				return err
			}
		}
		if len(board.Warnings) > 0 {
			if _, err := fmt.Fprintf(w, "- %d board warning needs cleanup.\n", len(board.Warnings)); err != nil {
				return err
			}
		}
	}
	if len(p.Actions) > 0 {
		_, err := fmt.Fprintln(w, "\nRecommended next step:\n"+p.Actions[0].Command)
		return err
	}
	return nil
}

func summaryProjectBoardFromData(data any) (domain.ProjectBoard, bool) {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return domain.ProjectBoard{}, false
	}
	value, ok := dataMap["board"]
	if !ok {
		return domain.ProjectBoard{}, false
	}
	if board, ok := value.(domain.ProjectBoard); ok {
		return board, true
	}
	b, err := json.Marshal(value)
	if err != nil {
		return domain.ProjectBoard{}, false
	}
	var board domain.ProjectBoard
	if err := json.Unmarshal(b, &board); err != nil {
		return domain.ProjectBoard{}, false
	}
	return board, true
}

func boardItemsForColumn(items []domain.BoardItem, column string) []domain.BoardItem {
	out := make([]domain.BoardItem, 0)
	for _, item := range items {
		if item.Column == column {
			out = append(out, item)
		}
	}
	return out
}

func boardColumnSummaryName(column string) string {
	switch column {
	case "inbox":
		return "Inbox"
	case "next":
		return "Next"
	case "doing":
		return "Doing"
	case "blocked":
		return "Blocked"
	case "review":
		return "Review"
	default:
		return column
	}
}

func summaryBoardItemLine(item domain.BoardItem) string {
	parts := []string{}
	if item.Priority != "" {
		parts = append(parts, "["+item.Priority+"]")
	}
	parts = append(parts, item.Title, "id="+item.ItemID)
	if due := defaultString(item.DueAt, item.Due); due != "" {
		parts = append(parts, "due="+due)
	}
	if len(item.Labels) > 0 {
		parts = append(parts, "labels="+strings.Join(item.Labels, ","))
	}
	if item.Milestone != "" {
		parts = append(parts, "milestone="+item.Milestone)
	}
	if len(item.BlockedBy) > 0 {
		parts = append(parts, "blocked_by="+strings.Join(item.BlockedBy, ","))
	}
	return strings.Join(parts, " ")
}

func summaryPercent(count, total int) string {
	if total <= 0 || count <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", (count*100+total/2)/total)
}

func summaryBar(count, maxCount, width int) string {
	if count <= 0 || maxCount <= 0 || width <= 0 {
		return "-"
	}
	filled := (count*width + maxCount/2) / maxCount
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled)
}

func summaryDimensionHeader(dimension string) string {
	switch dimension {
	case "group":
		return "Group"
	case "tag":
		return "Tags"
	case "folder":
		return "Folder"
	case "kind":
		return "Kind"
	default:
		return "Value"
	}
}

func summaryDimensionLabel(dimension, value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	switch dimension {
	case "group":
		return "(ungrouped)"
	case "tag":
		return "(untagged)"
	case "folder":
		return "(no folder)"
	case "kind":
		return "(uncategorized)"
	default:
		return "(empty)"
	}
}

func renderSummaryOrganizePlan(w io.Writer, theme summaryTheme, data any) error {
	plan, ok := data.(domain.OrganizePlan)
	if !ok || len(plan.Operations) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := organizeOperationRows(plan.Operations, 18)
	return renderSummaryTable(w, theme, []string{"Operation preview", "Mode", "Risk", "Action", "Source", "Target", "Reason"}, rows)
}

func renderSummaryLegacyOrganizePlan(w io.Writer, theme summaryTheme, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	ops, ok := dataMap["operations"].([]domain.PlanOperation)
	if !ok || len(ops) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	limit := minInt(len(ops), 18)
	rows := make([][]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		op := ops[i]
		rows = append(rows, []string{
			fmt.Sprint(i + 1),
			summaryCell(summaryHumanValue("operation.kind", op.Kind), 16),
			summaryCell(op.Path, 48),
			summaryCell(op.Target, 48),
			summaryCell(op.Reason, 40),
			summaryCell(summaryHumanValue("operation.status", op.Status), 12),
		})
	}
	if len(ops) > limit {
		rows = append(rows, []string{"More", "-", "-", "-", fmt.Sprintf("%d more entries; use --json to view the full plan", len(ops)-limit), "-"})
	}
	return renderSummaryTable(w, theme, []string{"Operation preview", "Action", "Source", "Target", "Reason", "Status"}, rows)
}

func renderSummaryOrganizePlanList(w io.Writer, theme summaryTheme, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	plans, ok := dataMap["plans"].([]domain.OrganizePlanSummary)
	if !ok || len(plans) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(plans))
	for _, plan := range plans {
		rows = append(rows, []string{
			summaryCell(plan.PlanID, 28),
			summaryCell(summaryHumanValue("plan.status", plan.Status), 12),
			fmt.Sprint(plan.Operations),
			summaryTimeCell(plan.CreatedAt),
			summaryTimeCell(plan.ExpiresAt),
			summaryCell(plan.SavedPath, 48),
		})
	}
	return renderSummaryTable(w, theme, []string{"Saved plans", "Status", "Operation", "Created", "Expires", "Path"}, rows)
}

func organizeOperationRows(ops []domain.OrganizeOperation, limit int) [][]string {
	if limit <= 0 || limit > len(ops) {
		limit = len(ops)
	}
	rows := make([][]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		op := ops[i]
		rows = append(rows, []string{
			fmt.Sprint(i + 1),
			summaryCell(organizeModeLabel(op.Mode), 12),
			summaryCell(organizeRiskLabel(op.Risk), 12),
			summaryCell(summaryHumanValue("operation.kind", op.Kind), 18),
			summaryCell(op.Path, 44),
			summaryCell(op.Target, 44),
			summaryCell(op.Reason, 36),
		})
	}
	if len(ops) > limit {
		rows = append(rows, []string{"More", "-", "-", "-", "-", "-", fmt.Sprintf("%d more entries; use --json to view the full plan", len(ops)-limit)})
	}
	return rows
}

func organizeModeLabel(mode string) string {
	switch mode {
	case "automatic":
		return "Automatic"
	case "manual_review":
		return "Manual review"
	default:
		return mode
	}
}

func organizeRiskLabel(risk string) string {
	switch risk {
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "review":
		return "Review"
	default:
		return risk
	}
}

func summaryTimeCell(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	if len(value) >= len("2006-01-02T15:04") {
		return strings.Replace(value[:len("2006-01-02T15:04")], "T", " ", 1)
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderSummarySyncConflictList(w io.Writer, theme summaryTheme, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	conflicts, ok := dataMap["conflicts"].([]domain.SyncConflictEntry)
	if !ok || len(conflicts) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		rows = append(rows, []string{summaryCell(conflict.File, 64), summaryCell(conflict.MainPath, 64), summaryTimeCell(conflict.Modified)})
	}
	return renderSummaryTable(w, theme, []string{"Conflict file", "Main path", "Updated"}, rows)
}

func renderSummarySyncConflictShow(w io.Writer, theme summaryTheme, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	conflict, _ := dataMap["conflict"].(domain.SyncConflictEntry)
	mainBody, _ := dataMap["main_body"].(string)
	body, _ := dataMap["body"].(string)
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := [][]string{{"Main", summaryCell(conflict.MainPath, 56), summaryCell(mainBody, 80)}, {"Conflict", summaryCell(conflict.File, 56), summaryCell(body, 80)}}
	return renderSummaryTable(w, theme, []string{"Side", "Path", "Preview"}, rows)
}

func renderSummarySyncConflictDiff(w io.Writer, data any) error {
	dataMap, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	diff, _ := dataMap["diff"].(string)
	if strings.TrimSpace(diff) == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, "\n"+strings.TrimRight(diff, "\n"))
	return err
}

func sortFactKeys(keys []string) {
	sort.SliceStable(keys, func(i, j int) bool {
		iRank := factKeyShapeRank(keys[i])
		jRank := factKeyShapeRank(keys[j])
		if iRank != jRank {
			return iRank < jRank
		}
		return naturalKeyLess(keys[i], keys[j])
	})
}

func factKeyShapeRank(key string) int {
	if strings.Contains(key, ".") {
		return 1
	}
	return 0
}

func naturalKeyLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ad, bd := isASCIIDigit(a[i]), isASCIIDigit(b[j])
		if ad && bd {
			ai, bj := i, j
			for i < len(a) && isASCIIDigit(a[i]) {
				i++
			}
			for j < len(b) && isASCIIDigit(b[j]) {
				j++
			}
			an := strings.TrimLeft(a[ai:i], "0")
			bn := strings.TrimLeft(b[bj:j], "0")
			if an == "" {
				an = "0"
			}
			if bn == "" {
				bn = "0"
			}
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			if i-ai != j-bj {
				return i-ai < j-bj
			}
			continue
		}
		if a[i] != b[j] {
			return a[i] < b[j]
		}
		i++
		j++
	}
	return len(a) < len(b)
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
