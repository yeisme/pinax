package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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
	ColorMode  string
	ThemeName  string
	ThemeRoles map[string]string
	Width      int
	Markdown   MarkdownOptions
	IsTerminal bool
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
		return enc.Encode(projection)
	case ModeAgent:
		return renderAgent(w, projection)
	case ModeEvents:
		return renderEvents(w, projection)
	case ModeExplain:
		return renderExplain(w, projection)
	default:
		return renderSummaryWithOptions(w, projection, opts)
	}
}

func renderSummaryWithOptions(w io.Writer, p domain.Projection, opts RenderOptions) error {
	theme := newSummaryThemeWithOptions(w, opts)
	if p.Command == "note.preview" && p.Status == "success" && p.Error == nil {
		return renderSummaryDataWithOptions(w, theme, p, opts)
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
		if p.Error.Hint != "" {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
			return renderSummaryTable(w, theme, []string{"Next step"}, [][]string{{p.Error.Hint}})
		}
		return nil
	}
	if len(p.Facts) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderSummaryFacts(w, theme, p.Facts); err != nil {
			return err
		}
	}
	if err := renderSummaryDataWithOptions(w, theme, p, opts); err != nil {
		return err
	}
	if len(p.Evidence) > 0 {
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

type summaryTheme struct {
	renderer *lipgloss.Renderer
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
		header:   style(roles.Heading).Bold(true),
		rule:     style(roles.Rule),
		success:  style(roles.Success).Bold(true),
		failed:   style(roles.Danger).Bold(true),
		numeric:  style(roles.Value),
		action:   style(roles.Link),
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
	body := trimTrailingSpaceLines(tw.Render())
	rule := strings.Repeat("━", maxRenderedLineWidth(body))
	_, err := fmt.Fprintf(w, "%s\n%s\n%s\n", theme.rule.Render(rule), body, theme.rule.Render(rule))
	return err
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

func maxRenderedLineWidth(value string) int {
	width := 0
	for _, line := range strings.Split(value, "\n") {
		if lineWidth := lipgloss.Width(line); lineWidth > width {
			width = lineWidth
		}
	}
	if width == 0 {
		return 1
	}
	return width
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
	sort.Strings(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{summaryFactLabel(key), summaryFactValue(key, facts[key])})
	}
	return renderSummaryTable(w, theme, []string{"Metric", "Value"}, rows)
}

func summaryFactLabel(key string) string {
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
	case "note.search":
		return renderSummarySearchResults(w, theme, p.Data)
	case "note.list":
		return renderSummaryNoteList(w, theme, p.Data, "notes")
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
	case "project.board.show":
		return renderSummaryProjectBoard(w, p)
	case "tag.list", "kind.list", "group.list":
		return renderSummaryDimensionList(w, theme, p.Data)
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
	case "sync.conflicts.show":
		return renderSummarySyncConflictShow(w, theme, p.Data)
	case "sync.conflicts.diff":
		return renderSummarySyncConflictDiff(w, p.Data)
	default:
		return nil
	}
}

type summarySearchData struct {
	Results []summarySearchResult `json:"results"`
	Notes   []domain.Note         `json:"notes"`
}

type summarySearchResult struct {
	Note    domain.Note `json:"note"`
	Snippet string      `json:"snippet"`
}

func renderSummarySearchResults(w io.Writer, theme summaryTheme, data any) error {
	results := summarySearchResultsFromData(data)
	if len(results) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(results))
	for _, result := range results {
		rows = append(rows, []string{
			summaryCell(result.Note.Path, 56),
			summaryCell(result.Note.Title, 32),
			summaryCell(result.Snippet, 80),
		})
	}
	return renderSummaryTable(w, theme, []string{"Path", "Title", "Preview"}, rows)
}

func summarySearchResultsFromData(data any) []summarySearchResult {
	var searchData summarySearchData
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(b, &searchData); err != nil {
		return nil
	}
	if len(searchData.Results) > 0 {
		return searchData.Results
	}
	results := make([]summarySearchResult, 0, len(searchData.Notes))
	for _, note := range searchData.Notes {
		results = append(results, summarySearchResult{Note: note})
	}
	return results
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
		status := "broken"
		if link.TargetPath != "" {
			status = "resolved"
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

func renderAgent(w io.Writer, p domain.Projection) error {
	lines := []string{
		"spec_version=" + p.SpecVersion,
		"mode=agent",
		"command=" + p.Command,
		"status=" + p.Status,
	}
	keys := make([]string, 0, len(p.Facts))
	for key := range p.Facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "fact."+key+"="+quoteAgentValue(p.Facts[key]))
	}
	if p.Error != nil {
		lines = append(lines, "error.code="+quoteAgentValue(p.Error.Code))
		if p.Error.Message != "" {
			lines = append(lines, "error.message="+quoteAgentValue(p.Error.Message))
		}
		if p.Error.Hint != "" {
			lines = append(lines, "error.hint="+quoteAgentValue(p.Error.Hint))
		}
	}
	if data, ok := p.Data.(map[string]any); ok {
		if candidates, ok := data["candidates"].([]domain.Note); ok {
			for i, note := range candidates {
				prefix := fmt.Sprintf("candidate.%d.", i+1)
				lines = append(lines, prefix+"path="+quoteAgentValue(note.Path))
				lines = append(lines, prefix+"note_id="+quoteAgentValue(note.ID))
				lines = append(lines, prefix+"title="+quoteAgentValue(note.Title))
			}
		}
		if candidates, ok := data["candidates"].([]domain.VaultObjectCandidate); ok {
			for i, candidate := range candidates {
				prefix := fmt.Sprintf("candidate.%d.", i+1)
				lines = append(lines, prefix+"object_kind="+quoteAgentValue(string(candidate.ObjectKind)))
				lines = append(lines, prefix+"path="+quoteAgentValue(candidate.Path))
				if candidate.NoteID != "" {
					lines = append(lines, prefix+"note_id="+quoteAgentValue(candidate.NoteID))
				}
				if candidate.Title != "" {
					lines = append(lines, prefix+"title="+quoteAgentValue(candidate.Title))
				}
				if candidate.ManagedStatus != "" {
					lines = append(lines, prefix+"managed_status="+quoteAgentValue(candidate.ManagedStatus))
				}
			}
		}
	}
	if report, ok := p.Data.(domain.VaultDoctorReport); ok {
		for i, issue := range report.Issues {
			prefix := fmt.Sprintf("issue.%d.", i+1)
			lines = append(lines, prefix+"code="+quoteAgentValue(issue.Code))
			lines = append(lines, prefix+"severity="+quoteAgentValue(issue.Severity))
			if issue.Path != "" {
				lines = append(lines, prefix+"path="+quoteAgentValue(issue.Path))
			}
		}
	}
	for _, action := range p.Actions {
		lines = append(lines, "action."+action.Name+"="+quoteAgentValue(action.Command))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func renderEvents(w io.Writer, p domain.Projection) error {
	start := map[string]any{
		"spec_version": p.SpecVersion,
		"mode":         "events",
		"command":      p.Command,
		"type":         "start",
		"seq":          1,
	}
	endType := "end"
	if p.Status == "failed" {
		endType = "error"
	}
	end := map[string]any{
		"spec_version": p.SpecVersion,
		"mode":         "events",
		"command":      p.Command,
		"type":         endType,
		"seq":          2,
		"status":       p.Status,
		"summary":      p.Summary,
	}
	if len(p.Facts) > 0 {
		end["facts"] = p.Facts
	}
	if len(p.Actions) > 0 {
		end["actions"] = p.Actions
	}
	if len(p.Evidence) > 0 {
		end["evidence"] = p.Evidence
	}
	if p.Error != nil {
		end["error"] = p.Error
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(start); err != nil {
		return err
	}
	return enc.Encode(end)
}

func renderExplain(w io.Writer, p domain.Projection) error {
	if _, err := fmt.Fprintf(w, "Conclusion: %s\n", defaultString(p.Summary, p.Status)); err != nil {
		return err
	}
	evidence := p.Evidence
	if len(evidence) == 0 && len(p.Facts) > 0 {
		keys := make([]string, 0, len(p.Facts))
		for key := range p.Facts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			evidence = append(evidence, key+"="+p.Facts[key])
		}
	}
	if len(evidence) == 0 {
		evidence = []string{"Command projection generated"}
	}
	if _, err := fmt.Fprintf(w, "Evidence: %s\n", strings.Join(evidence, "; ")); err != nil {
		return err
	}
	if p.Error != nil {
		if _, err := fmt.Fprintf(w, "Risk: %s\n", p.Error.Message); err != nil {
			return err
		}
	}
	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintf(w, "Recommended next step: %s\n", p.Actions[0].Command); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Confidence: 0.8")
	return err
}

func quoteAgentValue(value string) string {
	if strings.ContainsAny(value, " \t\n\"") {
		b, _ := json.Marshal(value)
		quoted := strings.ReplaceAll(string(b), "\\u003c", "<")
		quoted = strings.ReplaceAll(quoted, "\\u003e", ">")
		quoted = strings.ReplaceAll(quoted, "\\u0026", "&")
		return quoted
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
