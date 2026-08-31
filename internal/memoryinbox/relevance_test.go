package memoryinbox

import (
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func relevanceItem(id string, category InboxCategory, risk RiskLevel, subject string) InboxItem {
	return InboxItem{
		SchemaVersion:   InboxSchemaVersion,
		ItemID:          id,
		Category:        category,
		ReasonCodes:     []ReasonCode{ReasonNewProposal},
		Risk:            risk,
		Subject:         subject,
		Scope:           agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"},
		SuggestedAction: ActionReview,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		Experimental:    true,
	}
}

// TestReviewRelevanceConflictAlwaysRelevant 覆盖 conflict category 永远相关。
func TestReviewRelevanceConflictAlwaysRelevant(t *testing.T) {
	t.Parallel()
	items := []InboxItem{
		relevanceItem("prop_a", CategoryNewFact, RiskLow, "unrelated note about databases"),
		relevanceItem("prop_b", CategoryConflict, RiskMedium, "conflicting approach"),
	}
	relevant := RelevanceProjection(items, "shipping continuity contracts", "")
	if len(relevant) != 1 || relevant[0].Item.ItemID != "prop_b" {
		t.Fatalf("conflict item must be relevant: %#v", relevant)
	}
	found := false
	for _, code := range relevant[0].ReasonCodes {
		if code == ReasonConflictsWithDecision {
			found = true
		}
	}
	if !found {
		t.Fatalf("reason codes = %v, want %s", relevant[0].ReasonCodes, ReasonConflictsWithDecision)
	}
}

// TestReviewRelevanceObjectiveTokenOverlap 覆盖 objective token 重合：
// subject 与 objective 有实质重合 → affects_objective；无重合 → 不进 inline。
func TestReviewRelevanceObjectiveTokenOverlap(t *testing.T) {
	t.Parallel()
	items := []InboxItem{
		relevanceItem("prop_match", CategoryNewFact, RiskLow, "checkpoint caps need owner review"),
		relevanceItem("prop_other", CategoryNewFact, RiskLow, "gardening ideas for the weekend"),
	}
	relevant := RelevanceProjection(items, "review checkpoint caps", "")
	if len(relevant) != 1 || relevant[0].Item.ItemID != "prop_match" {
		t.Fatalf("token overlap relevance wrong: %#v", relevant)
	}

	// 空 objective：token 规则不触发，避免 false positive。
	relevant = RelevanceProjection(items, "", "")
	if len(relevant) != 0 {
		t.Fatalf("empty objective must not trigger token rule: %#v", relevant)
	}
}

// TestReviewRelevanceStaleSourceAndHighRisk 覆盖 stale 分类与高 risk 决策项。
func TestReviewRelevanceStaleSourceAndHighRisk(t *testing.T) {
	t.Parallel()
	items := []InboxItem{
		relevanceItem("prop_stale", CategoryStale, RiskLow, "old source drifted"),
		relevanceItem("prop_high", CategoryDecision, RiskHigh, "unrelated decision"),
	}
	relevant := RelevanceProjection(items, "", "")
	if len(relevant) != 2 {
		t.Fatalf("stale + high-risk decision must be relevant: %#v", relevant)
	}
	// 排序：high risk 优先。
	if relevant[0].Item.ItemID != "prop_high" {
		t.Fatalf("risk priority ordering wrong: %#v", relevant)
	}
}

// TestReviewRelevanceDeterministicOrdering 覆盖排序确定性：
// 同 risk 同 reason 数按 item_id 字典序，保证 golden 稳定。
func TestReviewRelevanceDeterministicOrdering(t *testing.T) {
	t.Parallel()
	items := []InboxItem{
		relevanceItem("prop_zz", CategoryConflict, RiskMedium, "conflict z"),
		relevanceItem("prop_aa", CategoryConflict, RiskMedium, "conflict a"),
	}
	first := RelevanceProjection(items, "", "")
	second := RelevanceProjection(items, "", "")
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("relevance size wrong")
	}
	if first[0].Item.ItemID != second[0].Item.ItemID || first[1].Item.ItemID != second[1].Item.ItemID {
		t.Fatalf("relevance ordering not deterministic: %v vs %v", first[0].Item.ItemID, second[0].Item.ItemID)
	}
	if first[0].Item.ItemID != "prop_aa" {
		t.Fatalf("tie must order by item_id: %#v", first)
	}
}

// TestInlineAttentionBoundedItem 覆盖 inline item 是 bounded 投影：
// relevance 输出不含 body（InboxItem 结构本身无 body 字段，这里固定契约）。
func TestInlineAttentionBoundedItem(t *testing.T) {
	t.Parallel()
	items := []InboxItem{relevanceItem("prop_1", CategoryConflict, RiskHigh, "bounded subject")}
	relevant := RelevanceProjection(items, "", "")
	if len(relevant) != 1 {
		t.Fatalf("expected one relevant item")
	}
	item := relevant[0].Item
	if item.Subject == "" || item.ItemID == "" || len(item.ReasonCodes) == 0 {
		t.Fatalf("inline item must carry bounded refs and reason codes: %#v", item)
	}
}
