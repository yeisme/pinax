package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// resumeCardSection 是 Resume Card 的单个区块输入（从 projection.Data 的
// JSON 视图解码，renderer 不依赖 agentcontinuity 内部类型）。
type resumeCardSection struct {
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Items     []string `json:"items,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	Omitted   int      `json:"omitted,omitempty"`
}

type resumeCardCoverage struct {
	Total     int `json:"total"`
	Resolved  int `json:"resolved"`
	Missing   int `json:"missing"`
	Stale     int `json:"stale"`
	Ambiguous int `json:"ambiguous,omitempty"`
}

type resumeCardNextAction struct {
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// resumeCardData 是 continue projection data 的 bounded JSON 视图。
// 只取渲染所需字段；正文、transcript、raw prompt 永不进入。
type resumeCardData struct {
	Objective             string                `json:"objective"`
	CurrentState          string                `json:"current_state"`
	Sections              []resumeCardSection   `json:"sections"`
	HandoffStatus         string                `json:"handoff_status"`
	SourceCoverage        resumeCardCoverage    `json:"source_coverage"`
	EvidenceStatus        string                `json:"evidence_status"`
	FreshnessStatus       string                `json:"freshness_status"`
	PackStatus            string                `json:"pack_status"`
	WarningCodes          []string              `json:"warning_codes"`
	RecommendedNextAction *resumeCardNextAction `json:"recommended_next_action"`
	ReviewAttentionCount  int                   `json:"review_attention_count"`
	Conflicts             []struct {
		MemoryIDs []string `json:"memory_ids"`
		Reason    string   `json:"reason,omitempty"`
	} `json:"conflicts"`
}

// decodeResumeCard 把 projection.Data 解码为 bounded JSON 视图。
// 解码失败返回 ok=false，调用方回退到 generic renderer。
func decodeResumeCard(data any) (resumeCardData, bool) {
	raw, err := json.Marshal(data)
	if err != nil {
		return resumeCardData{}, false
	}
	var card resumeCardData
	if err := json.Unmarshal(raw, &card); err != nil {
		return resumeCardData{}, false
	}
	return card, true
}

// resumeCardBlock 是六个固定区块之一。
type resumeCardBlock struct {
	Title string
	Lines []string
}

// renderResumeCard 渲染 command-specific 的紧凑 Resume Card：
// Objective / Last state / Key decisions / Blockers / Recommended next action /
// Evidence status。卡面 bounded，不倾倒完整 note、handoff、transcript 或
// source body。这是 experimental UX refinement；machine envelope 不变。
func renderResumeCard(w io.Writer, theme summaryTheme, p domain.Projection) error {
	card, ok := decodeResumeCard(p.Data)
	if !ok {
		return fmt.Errorf("continue projection data is not a resume card")
	}

	status := theme.success.Render("ready")
	if card.PackStatus == "partial" {
		status = theme.action.Render("partial")
	}

	if err := renderSummaryTable(w, theme, []string{"Resume Card", "Status"}, [][]string{{defaultString(p.Summary, "Continuity pack"), status}}); err != nil {
		return err
	}

	blocks := buildResumeCardBlocks(card)
	for _, block := range blocks {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		rows := make([][]string, 0, len(block.Lines))
		for _, line := range block.Lines {
			rows = append(rows, []string{line})
		}
		if err := renderSummaryTable(w, theme, []string{block.Title}, rows); err != nil {
			return err
		}
	}
	return nil
}

// buildResumeCardBlocks 生成六个固定区块的 bounded 内容。
// 每区块最多 3 行、每行最多 120 字符；conflict/source warning 位于
// budget 之外，由 Evidence status 与 warning 行保证不被丢弃。
func buildResumeCardBlocks(card resumeCardData) []resumeCardBlock {
	objective := strings.TrimSpace(card.Objective)
	if objective == "" {
		objective = "-"
	}
	state := strings.TrimSpace(card.CurrentState)
	if card.HandoffStatus == "missing" {
		state = "Handoff: missing (context-only)"
	} else if state == "" {
		state = "-"
	}

	decisions := resumeCardSectionLines(card, []string{"decision", "handoff_decision"}, 3)
	if len(decisions) == 0 {
		decisions = []string{"-"}
	}

	blockers := resumeCardSectionLines(card, []string{"blocker"}, 3)
	if len(card.Conflicts) > 0 {
		reasons := make([]string, 0, len(card.Conflicts))
		for _, conflict := range card.Conflicts {
			reasons = append(reasons, defaultString(conflict.Reason, "conflicting decisions"))
		}
		blockers = append([]string{fmt.Sprintf("conflict: %s", strings.Join(reasons, "; "))}, blockers...)
	}
	if len(blockers) == 0 {
		blockers = []string{"-"}
	}

	next := "-"
	if card.RecommendedNextAction != nil {
		next = card.RecommendedNextAction.Name
		if card.RecommendedNextAction.Command != "" {
			next = fmt.Sprintf("%s: %s", next, card.RecommendedNextAction.Command)
		}
	}

	evidence := []string{
		fmt.Sprintf("sources: %d/%d resolved", card.SourceCoverage.Resolved, card.SourceCoverage.Total),
	}
	if card.SourceCoverage.Stale > 0 || card.SourceCoverage.Missing > 0 || card.SourceCoverage.Ambiguous > 0 {
		evidence = append(evidence, fmt.Sprintf("stale=%d missing=%d ambiguous=%d",
			card.SourceCoverage.Stale, card.SourceCoverage.Missing, card.SourceCoverage.Ambiguous))
	}
	evidence = append(evidence,
		fmt.Sprintf("evidence=%s freshness=%s", defaultString(card.EvidenceStatus, "not_measured"), defaultString(card.FreshnessStatus, "not_measured")))
	if len(card.WarningCodes) > 0 {
		evidence = append(evidence, "warnings: "+strings.Join(card.WarningCodes, ", "))
	}
	if card.ReviewAttentionCount > 0 {
		evidence = append(evidence, fmt.Sprintf("review attention: %d item(s)", card.ReviewAttentionCount))
	}

	return []resumeCardBlock{
		{Title: "Objective", Lines: []string{resumeCardClamp(objective)}},
		{Title: "Last state", Lines: []string{resumeCardClamp(state)}},
		{Title: "Key decisions", Lines: resumeCardClampAll(decisions)},
		{Title: "Blockers / conflicts", Lines: resumeCardClampAll(blockers)},
		{Title: "Recommended next action", Lines: []string{resumeCardClamp(next)}},
		{Title: "Evidence status", Lines: evidence},
	}
}

// resumeCardSectionLines 从 sections 中抽取指定 kind 的前 max 行（bounded）。
func resumeCardSectionLines(card resumeCardData, kinds []string, max int) []string {
	kindSet := map[string]bool{}
	for _, kind := range kinds {
		kindSet[kind] = true
	}
	var lines []string
	for _, section := range card.Sections {
		if !kindSet[section.Kind] {
			continue
		}
		for _, item := range section.Items {
			lines = append(lines, item)
			if len(lines) == max {
				return lines
			}
		}
	}
	return lines
}

// resumeCardClamp 把单行截断到 120 字符（卡面 bounded，不倾倒 body）。
func resumeCardClamp(line string) string {
	line = strings.TrimSpace(line)
	runes := []rune(line)
	if len(runes) <= 120 {
		return line
	}
	return string(runes[:117]) + "..."
}

func resumeCardClampAll(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, resumeCardClamp(line))
	}
	return out
}
