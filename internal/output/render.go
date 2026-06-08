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

func Render(w io.Writer, mode Mode, projection domain.Projection) error {
	projection.SpecVersion = defaultString(projection.SpecVersion, "1.0")
	projection.Mode = string(mode)
	if projection.Status == "" {
		projection.Status = "success"
	}

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
		return renderSummary(w, projection)
	}
}

func renderSummary(w io.Writer, p domain.Projection) error {
	theme := newSummaryTheme(w)
	if err := renderSummaryTable(w, theme, []string{"状态", "重点"}, [][]string{{summaryStatusCell(theme, p.Status), defaultString(p.Summary, "-")}}); err != nil {
		return err
	}
	if p.Error != nil {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderSummaryTable(w, theme, []string{"错误", "说明"}, [][]string{{theme.failed.Render(p.Error.Code), defaultString(p.Error.Message, "-")}}); err != nil {
			return err
		}
		if p.Error.Hint != "" {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
			return renderSummaryTable(w, theme, []string{"下一步"}, [][]string{{p.Error.Hint}})
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
	if err := renderSummaryData(w, theme, p); err != nil {
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
		if err := renderSummaryTable(w, theme, []string{"证据"}, rows); err != nil {
			return err
		}
	}
	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		return renderSummaryTable(w, theme, []string{"下一步"}, [][]string{{p.Actions[0].Command}})
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

func newSummaryTheme(w io.Writer) summaryTheme {
	renderer := lipgloss.NewRenderer(w)
	if summaryColorEnabled(w) {
		renderer.SetColorProfile(termenv.TrueColor)
	} else {
		renderer.SetColorProfile(termenv.Ascii)
	}
	style := func() lipgloss.Style { return renderer.NewStyle() }
	return summaryTheme{
		renderer: renderer,
		header:   style().Bold(true).Foreground(lipgloss.Color("250")),
		rule:     style().Foreground(lipgloss.Color("240")),
		success:  style().Bold(true).Foreground(lipgloss.Color("34")),
		failed:   style().Bold(true).Foreground(lipgloss.Color("160")),
		numeric:  style().Foreground(lipgloss.Color("250")),
		action:   style().Foreground(lipgloss.Color("38")),
	}
}

func summaryColorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PINAX_COLOR"))) {
	case "always", "1", "true", "yes", "on":
		return true
	case "never", "0", "false", "no", "off":
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func summaryStatusCell(theme summaryTheme, status string) string {
	switch status {
	case "success":
		return theme.success.Render(status)
	case "failed":
		return theme.failed.Render(status)
	case "partial":
		return theme.action.Render(status)
	default:
		return status
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
			case "错误":
				style = style.Inherit(theme.failed)
			case "下一步":
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
	case "数量", "行数", "代码", "注释", "空行":
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
		rows = append(rows, []string{key, facts[key]})
	}
	return renderSummaryTable(w, theme, []string{"指标", "值"}, rows)
}

func renderSummaryData(w io.Writer, theme summaryTheme, p domain.Projection) error {
	switch p.Command {
	case "note.list":
		return renderSummaryNoteList(w, theme, p.Data, "notes")
	case "note.orphans":
		return renderSummaryNoteList(w, theme, p.Data, "orphans")
	case "note.links":
		return renderSummaryLinkList(w, theme, p.Data, "links")
	case "note.backlinks":
		return renderSummaryLinkList(w, theme, p.Data, "backlinks")
	case "tag.list", "folder.list", "kind.list", "group.list":
		return renderSummaryDimensionList(w, theme, p.Data)
	case "organize.suggest":
		return renderSummaryOrganizePlan(w, theme, p.Data)
	case "organize.list":
		return renderSummaryOrganizePlanList(w, theme, p.Data)
	case "organize.plan":
		return renderSummaryLegacyOrganizePlan(w, theme, p.Data)
	default:
		return nil
	}
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
			summaryCell(note.Kind, 10),
			summaryCell(tags, 22),
			summaryCell(note.Status, 10),
			summaryCell(note.UpdatedAt, 20),
		})
	}
	return renderSummaryTable(w, theme, []string{"路径", "标题", "分类", "标签", "状态", "更新时间"}, rows)
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
			status,
		})
	}
	return renderSummaryTable(w, theme, []string{"来源", "目标", "路径", "状态"}, rows)
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
	for _, item := range items {
		label := summaryDimensionLabel(dimension, item.Value)
		labels = append(labels, label)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := make([][]string, 0, len(items))
	for i, item := range items {
		rows = append(rows, []string{labels[i], fmt.Sprint(item.Count)})
	}
	return renderSummaryTable(w, theme, []string{summaryDimensionHeader(dimension), "数量"}, rows)
}

func summaryDimensionHeader(dimension string) string {
	switch dimension {
	case "group":
		return "分组"
	case "tag":
		return "标签"
	case "folder":
		return "文件夹"
	case "kind":
		return "分类"
	default:
		return "值"
	}
}

func summaryDimensionLabel(dimension, value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	switch dimension {
	case "group":
		return "(未分组)"
	case "tag":
		return "(无标签)"
	case "folder":
		return "(无文件夹)"
	case "kind":
		return "(未分类)"
	default:
		return "(空)"
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
	return renderSummaryTable(w, theme, []string{"操作预览", "模式", "风险", "动作", "来源", "目标", "原因"}, rows)
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
			summaryCell(op.Kind, 16),
			summaryCell(op.Path, 48),
			summaryCell(op.Target, 48),
			summaryCell(op.Reason, 40),
			summaryCell(op.Status, 12),
		})
	}
	if len(ops) > limit {
		rows = append(rows, []string{"更多", "-", "-", "-", fmt.Sprintf("还有 %d 条，使用 --json 查看完整计划", len(ops)-limit), "-"})
	}
	return renderSummaryTable(w, theme, []string{"操作预览", "动作", "来源", "目标", "原因", "状态"}, rows)
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
			summaryCell(plan.Status, 12),
			fmt.Sprint(plan.Operations),
			summaryTimeCell(plan.CreatedAt),
			summaryTimeCell(plan.ExpiresAt),
			summaryCell(plan.SavedPath, 48),
		})
	}
	return renderSummaryTable(w, theme, []string{"已保存计划", "状态", "操作", "创建", "过期", "路径"}, rows)
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
			summaryCell(op.Kind, 18),
			summaryCell(op.Path, 44),
			summaryCell(op.Target, 44),
			summaryCell(op.Reason, 36),
		})
	}
	if len(ops) > limit {
		rows = append(rows, []string{"更多", "-", "-", "-", "-", "-", fmt.Sprintf("还有 %d 条，使用 --json 查看完整计划", len(ops)-limit)})
	}
	return rows
}

func organizeModeLabel(mode string) string {
	switch mode {
	case "automatic":
		return "自动"
	case "manual_review":
		return "需复核"
	default:
		return mode
	}
}

func organizeRiskLabel(risk string) string {
	switch risk {
	case "low":
		return "低"
	case "medium":
		return "中"
	case "review":
		return "复核"
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
	if _, err := fmt.Fprintf(w, "结论: %s\n", defaultString(p.Summary, p.Status)); err != nil {
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
		evidence = []string{"命令 projection 已生成"}
	}
	if _, err := fmt.Fprintf(w, "证据: %s\n", strings.Join(evidence, "; ")); err != nil {
		return err
	}
	if p.Error != nil {
		if _, err := fmt.Fprintf(w, "风险: %s\n", p.Error.Message); err != nil {
			return err
		}
	}
	if len(p.Actions) > 0 {
		if _, err := fmt.Fprintf(w, "推荐下一步: %s\n", p.Actions[0].Command); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "置信度: 0.8")
	return err
}

func quoteAgentValue(value string) string {
	if strings.ContainsAny(value, " \t\n\"") {
		b, _ := json.Marshal(value)
		return string(b)
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
