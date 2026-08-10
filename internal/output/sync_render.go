package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// renderSummarySyncView is the human-facing projection for all sync commands.
// It deliberately consumes only data.sync_view so status, JSON, agent, and
// events cannot drift into separate change classification rules.
func renderSummarySyncView(w io.Writer, theme summaryTheme, p domain.Projection, opts RenderOptions) error {
	root, ok := dataMap(p.Data)
	if !ok {
		return nil
	}
	view, ok := dataMap(root["sync_view"])
	if !ok {
		return renderSummaryNamedDataList(w, theme, "Sync operations", p.Data, []string{"plan", "operations"}, []summaryListColumn{{Header: "Kind", Path: "kind", MaxWidth: 18}, {Header: "Path", Path: "path", MaxWidth: 48}, {Header: "Status", Path: "status", MaxWidth: 14}})
	}
	if strings.EqualFold(strings.TrimSpace(opts.Style), "compact") {
		return renderCompactSyncView(w, p, view, opts)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Sync summary")); err != nil {
		return err
	}
	counts, _ := dataMap(view["counts"])
	rows := [][]string{
		{"Added", summaryCell(dataPathString(counts, "added"), 12)},
		{"Modified", summaryCell(dataPathString(counts, "modified"), 12)},
		{"Deleted", summaryCell(dataPathString(counts, "deleted"), 12)},
		{"Renamed", summaryCell(dataPathString(counts, "renamed"), 12)},
		{"Conflicts", summaryCell(dataPathString(counts, "conflicts"), 12)},
		{"Unchanged", summaryCell(dataPathString(counts, "unchanged"), 12)},
	}
	if err := renderSummaryTable(w, theme, []string{"Metric", "Value"}, rows); err != nil {
		return err
	}

	preview := syncPreviewMode(opts)
	if preview == "none" {
		return renderSyncRevisionTable(w, theme, view)
	}
	if preview == "diff" || opts.ContentDiff {
		if err := renderSyncMetadataDiff(w, theme, view); err != nil {
			return err
		}
	}
	// Keep the long-standing section label so existing scripts and users can
	// still recognize the operation listing while the richer sync view is
	// rendered above it.
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Sync operations")); err != nil {
		return err
	}
	if err := renderSyncChanges(w, theme, view, opts); err != nil {
		return err
	}
	if opts.ContentDiff {
		return renderSyncContentDiff(w, theme, root["content_diff"])
	}
	return nil
}

func renderSyncRevisionTable(w io.Writer, theme summaryTheme, view map[string]any) error {
	revisions, _ := dataMap(view["revisions"])
	rows := [][]string{
		{"Base", summaryCell(dataPathString(revisions, "base"), 48)},
		{"Remote before", summaryCell(dataPathString(revisions, "remote_before"), 48)},
		{"Remote after", summaryCell(dataPathString(revisions, "remote_after"), 48)},
		{"Local after", summaryCell(dataPathString(revisions, "local_after"), 48)},
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return renderSummaryTable(w, theme, []string{"Revision", "Value"}, rows)
}

func renderSyncMetadataDiff(w io.Writer, theme summaryTheme, view map[string]any) error {
	revisions, _ := dataMap(view["revisions"])
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Metadata diff")); err != nil {
		return err
	}
	lines := []string{
		"--- local/base",
		"+++ remote/head",
		"- revision: " + defaultString(dataPathString(revisions, "base"), "(none)"),
		"+ revision: " + defaultString(firstDataPathString(revisions, "remote_after", "remote_before"), "(none)"),
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, "  "+line); err != nil {
			return err
		}
	}
	return nil
}

func renderSyncChanges(w io.Writer, theme summaryTheme, view map[string]any, opts RenderOptions) error {
	changes := dataListMaps(view, "changes")
	limit := syncPreviewLimit(opts, len(changes))
	shown := limit
	if shown > len(changes) {
		shown = len(changes)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Changes")); err != nil {
		return err
	}
	if len(changes) == 0 {
		_, err := fmt.Fprintln(w, "  (none)")
		return err
	}
	rows := make([][]string, 0, shown+1)
	for _, change := range changes[:shown] {
		path := firstDataPathString(change, "path", "to_path", "from_path")
		if from := dataPathString(change, "from_path"); from != "" && dataPathString(change, "to_path") != "" {
			path = from + " -> " + dataPathString(change, "to_path")
		}
		rows = append(rows, []string{
			dataPathString(change, "code"),
			summaryCell(path, 72),
			dataPathString(change, "state"),
			dataPathString(change, "operation"),
		})
	}
	rows = append(rows, []string{"", fmt.Sprintf("shown %d/%d", shown, len(changes)), "", ""})
	return renderSummaryTable(w, theme, []string{"Code/Kind", "Path", "State/Status", "Operation"}, rows)
}

func renderCompactSyncView(w io.Writer, p domain.Projection, view map[string]any, opts RenderOptions) error {
	if _, err := fmt.Fprintln(w, defaultString(p.Summary, p.Status)); err != nil {
		return err
	}
	counts, _ := dataMap(view["counts"])
	for _, key := range []string{"added", "modified", "deleted", "renamed", "conflicts", "unchanged"} {
		if _, err := fmt.Fprintf(w, "%s=%s\n", key, dataPathString(counts, key)); err != nil {
			return err
		}
	}
	if syncPreviewMode(opts) == "none" {
		return nil
	}
	changes := dataListMaps(view, "changes")
	limit := syncPreviewLimit(opts, len(changes))
	if limit > len(changes) {
		limit = len(changes)
	}
	for _, change := range changes[:limit] {
		path := firstDataPathString(change, "path", "to_path", "from_path")
		if from := dataPathString(change, "from_path"); from != "" && dataPathString(change, "to_path") != "" {
			path = from + " -> " + dataPathString(change, "to_path")
		}
		if _, err := fmt.Fprintf(w, "%s %s\n", dataPathString(change, "code"), path); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "changes_shown=%d/%d\n", limit, len(changes)); err != nil {
		return err
	}
	if opts.ContentDiff {
		root, _ := dataMap(p.Data)
		return renderCompactContentDiff(w, root["content_diff"])
	}
	return nil
}

func renderSyncContentDiff(w io.Writer, theme summaryTheme, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	files := dataListMaps(root, "files")
	if len(files) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, theme.header.Render("Content diff")); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(w, "  %s\n", firstDataPathString(file, "path")); err != nil {
			return err
		}
		for _, hunk := range dataListScalars(file, "hunks") {
			if _, err := fmt.Fprintf(w, "    %s\n", hunk); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderCompactContentDiff(w io.Writer, data any) error {
	root, ok := dataMap(data)
	if !ok {
		return nil
	}
	for _, file := range dataListMaps(root, "files") {
		if _, err := fmt.Fprintf(w, "content.%s.lines=%s\n", firstDataPathString(file, "path"), dataPathString(file, "line_count")); err != nil {
			return err
		}
	}
	return nil
}

func syncPreviewMode(opts RenderOptions) string {
	preview := strings.ToLower(strings.TrimSpace(opts.SyncPreview))
	if preview == "" {
		preview = "status"
	}
	switch preview {
	case "none", "diff", "status":
		return preview
	default:
		return "status"
	}
}

func syncPreviewLimit(opts RenderOptions, total int) int {
	if opts.SyncLimitSet {
		if opts.SyncLimit < 0 {
			return 0
		}
		return opts.SyncLimit
	}
	if opts.SyncLimit > 0 {
		return opts.SyncLimit
	}
	if total > 10 {
		return 10
	}
	return total
}
