package memoryinbox

import (
	"sort"
	"strings"
)

// RelevanceReasonCode 是 review item 与当前 continuation 相关的确定性原因码。
// 只有携带至少一个原因码的 item 才能进入 inline attention。
const (
	// ReasonAffectsObjective：item subject 与当前 objective 有实质 token 重合，
	// 批准/拒绝会改变下一步。
	ReasonAffectsObjective = "affects_objective"
	// ReasonConflictsWithDecision：item 是 conflict 分类，直接影响当前决策。
	ReasonConflictsWithDecision = "conflicts_with_decision"
	// ReasonBlocksCurrentWork：高风险 decision/commitment item 可能阻塞当前工作。
	ReasonBlocksCurrentWork = "blocks_current_work"
	// ReasonStaleSourceForScope：stale/expired item 提示来源漂移。
	ReasonStaleSourceForScope = "stale_source_for_scope"
)

// ReviewAttentionCandidate 是一个命中 relevance 规则的 inbox item。
type ReviewAttentionCandidate struct {
	Item        InboxItem
	ReasonCodes []string
}

// RelevanceProjection 从 inbox items 中确定性地投影出会影响当前
// objective/decision/blocker/conflict/next action 的 pending item。
//
// 规则是闭合的、与实现无关的（中文注释）：
//  1. conflict category → conflicts_with_decision（冲突永远相关）。
//  2. stale/expired category → stale_source_for_scope（来源漂移永远相关）。
//  3. high risk 的 decision/commitment → blocks_current_work。
//  4. subject 与 objective 出现 ≥1 个实质 token 重合（长度≥3，非停用词）
//     → affects_objective。objective 为空时该规则不触发，避免 false positive。
//  5. 结果按（风险降序、reason 数降序、item_id 字典序）排序，保证 golden 稳定。
func RelevanceProjection(items []InboxItem, objective, _ string) []ReviewAttentionCandidate {
	objectiveTokens := meaningfulTokens(objective)
	var candidates []ReviewAttentionCandidate
	for _, item := range items {
		var reasons []string
		if item.Category == CategoryConflict {
			reasons = append(reasons, ReasonConflictsWithDecision)
		}
		if item.Category == CategoryStale || item.Category == CategoryExpired {
			reasons = append(reasons, ReasonStaleSourceForScope)
		}
		if item.Risk == RiskHigh && (item.Category == CategoryDecision || item.Category == CategoryCommitment) {
			reasons = append(reasons, ReasonBlocksCurrentWork)
		}
		if len(objectiveTokens) > 0 {
			subjectTokens := meaningfulTokens(item.Subject)
			for token := range subjectTokens {
				if _, ok := objectiveTokens[token]; ok {
					reasons = append(reasons, ReasonAffectsObjective)
					break
				}
			}
		}
		if len(reasons) == 0 {
			continue
		}
		candidates = append(candidates, ReviewAttentionCandidate{Item: item, ReasonCodes: reasons})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if riskValue(candidates[i].Item.Risk) != riskValue(candidates[j].Item.Risk) {
			return riskValue(candidates[i].Item.Risk) > riskValue(candidates[j].Item.Risk)
		}
		if len(candidates[i].ReasonCodes) != len(candidates[j].ReasonCodes) {
			return len(candidates[i].ReasonCodes) > len(candidates[j].ReasonCodes)
		}
		return candidates[i].Item.ItemID < candidates[j].Item.ItemID
	})
	return candidates
}

// isTokenRune 判断一个 rune 是否为 token 字符。
func isTokenRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '一' && r <= '鿿') || r == '-'
}

// meaningfulTokens 提取长度 ≥3 的小写 token 集合（简单停用词排除）。
// token 重合是确定性规则，不是语义匹配——保持可测试、可解释。
func meaningfulTokens(text string) map[string]struct{} {
	stopwords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "this": true,
		"that": true, "from": true, "into": true, "are": true, "was": true,
	}
	tokens := map[string]struct{}{}
	for _, raw := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		// token 字符：小写字母、数字、CJK、连字符；其余视为分隔符。
		return !isTokenRune(r)
	}) {
		raw = strings.Trim(raw, "-")
		if len([]rune(raw)) < 3 || stopwords[raw] {
			continue
		}
		tokens[raw] = struct{}{}
	}
	return tokens
}
