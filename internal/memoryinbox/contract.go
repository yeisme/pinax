// Package memoryinbox 实现 Memory Inbox 聚合投影。
//
// Inbox item 由现有 proposal、memory lifecycle、feedback、conflict、receipt
// 和 activity state 投影而成。它不是新的 truth store——只有无法从既有状态
// 重建的用户 review decision 才允许进入 additive GORM state。
//
// 分类规则是 deterministic 的，输出 category、reason_codes、risk、
// source_coverage、suggested_action。
package memoryinbox

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// InboxSchemaVersion 是 memory inbox 的 schema 版本。
const InboxSchemaVersion = "yeisme.memory_inbox.v1"

// InboxCategory 是 inbox item 的分类。
type InboxCategory string

const (
	CategoryNewFact    InboxCategory = "new_fact"
	CategoryPreference InboxCategory = "preference"
	CategoryDecision   InboxCategory = "decision"
	CategoryLesson     InboxCategory = "lesson"
	CategoryCommitment InboxCategory = "commitment"
	CategoryConflict   InboxCategory = "conflict"
	CategoryDuplicate  InboxCategory = "duplicate"
	CategoryStale      InboxCategory = "stale"
	CategoryExpired    InboxCategory = "expired"
)

// AllCategories 返回所有支持的分类，按确定性顺序。
func AllCategories() []InboxCategory {
	return []InboxCategory{
		CategoryConflict, CategoryDuplicate, CategoryStale, CategoryExpired,
		CategoryNewFact, CategoryPreference, CategoryDecision, CategoryLesson, CategoryCommitment,
	}
}

// RiskLevel 是 inbox item 的风险分级。
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ReasonCode 是分类的稳定原因码，解释为什么进入该分类。
type ReasonCode string

const (
	ReasonNewProposal       ReasonCode = "new_proposal"
	ReasonUnconfirmed       ReasonCode = "unconfirmed"
	ReasonConflictDetected  ReasonCode = "conflict_detected"
	ReasonDuplicateSubject  ReasonCode = "duplicate_subject"
	ReasonStaleSource       ReasonCode = "stale_source"
	ReasonExpiryReached     ReasonCode = "expiry_reached"
	ReasonConflictingScope  ReasonCode = "conflicting_scope"
	ReasonCrossScopePromo   ReasonCode = "cross_scope_promotion"
	ReasonBulkActionBlocked ReasonCode = "bulk_action_blocked"
)

// SuggestedAction 是 inbox item 建议的下一步操作。
type SuggestedAction string

const (
	ActionReview    SuggestedAction = "review"
	ActionApprove   SuggestedAction = "approve"
	ActionReject    SuggestedAction = "reject"
	ActionSupersede SuggestedAction = "supersede"
	ActionExpire    SuggestedAction = "expire"
	ActionRestore   SuggestedAction = "restore"
	ActionInspect   SuggestedAction = "inspect"
)

// InboxItem 是 inbox 中的一个条目投影。
// 不包含 proposal body 或完整 memory 内容。
type InboxItem struct {
	SchemaVersion string        `json:"schema_version"`
	ItemID        string        `json:"item_id"`
	Category      InboxCategory `json:"category"`
	ReasonCodes   []ReasonCode  `json:"reason_codes"`
	Risk          RiskLevel     `json:"risk"`
	// Subject 是 item 的简短描述（脱敏，不含 body）。
	Subject string `json:"subject"`
	// Scope 是 item 所属的范围。
	Scope agentprotocol.Scope `json:"scope"`
	// SourceCoverage 描述来源解析情况。
	SourceCoverage SourceCoverage `json:"source_coverage"`
	// SuggestedAction 是建议的下一步操作。
	SuggestedAction SuggestedAction `json:"suggested_action"`
	// ProposalID 关联的 proposal ID（如有）。
	ProposalID string `json:"proposal_id,omitempty"`
	// MemoryID 关联的 memory ID（如有）。
	MemoryID string `json:"memory_id,omitempty"`
	// HandoffID 关联的 handoff ID（如有）。
	HandoffID string `json:"handoff_id,omitempty"`
	// ReceiptID 关联的 receipt ID（如有，用于 restore）。
	ReceiptID string `json:"receipt_id,omitempty"`
	// CreatedAt item 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt item 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// ReviewVersion 用于乐观锁。
	ReviewVersion int `json:"review_version"`
	// Experimental 标记此 surface 为实验性。
	Experimental bool `json:"experimental"`
}

// SourceCoverage 描述来源解析情况（与 agentcontinuity 相同语义，独立定义避免包依赖）。
type SourceCoverage struct {
	Total    int `json:"total"`
	Resolved int `json:"resolved"`
	Missing  int `json:"missing"`
	Stale    int `json:"stale"`
}

// InboxPack 是 inbox 的 bounded 投影。
type InboxPack struct {
	SchemaVersion string      `json:"schema_version"`
	Items         []InboxItem `json:"items"`
	// CountsByCategory 按 category 统计。
	CountsByCategory map[InboxCategory]int `json:"counts_by_category"`
	// TotalItems 是 item 总数。
	TotalItems int `json:"total_items"`
	// HighRiskCount 是高风险 item 数。
	HighRiskCount int `json:"high_risk_count"`
	// Truncated 表示是否因 limit 截断。
	Truncated bool `json:"truncated"`
	// Experimental 标记此 surface 为实验性。
	Experimental bool `json:"experimental"`
}

// Validate 检查 inbox item 的必填字段。
func (item InboxItem) Validate() error {
	if item.SchemaVersion == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "schema_version is required")
	}
	if strings.TrimSpace(item.ItemID) == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "item_id is required")
	}
	if !isValidCategory(item.Category) {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, fmt.Sprintf("unknown category: %s", item.Category))
	}
	if !isValidRisk(item.Risk) {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, fmt.Sprintf("unknown risk: %s", item.Risk))
	}
	if err := item.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	return nil
}

// Validate 检查 inbox pack 的完整性。
func (p InboxPack) Validate() error {
	if p.SchemaVersion == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "schema_version is required")
	}
	for _, item := range p.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("item %s: %w", item.ItemID, err)
		}
	}
	return nil
}

// ComputeCounts 重新计算 CountsByCategory 和 HighRiskCount。
func (p *InboxPack) ComputeCounts() {
	p.CountsByCategory = make(map[InboxCategory]int)
	p.HighRiskCount = 0
	p.TotalItems = len(p.Items)
	for _, item := range p.Items {
		p.CountsByCategory[item.Category]++
		if item.Risk == RiskHigh {
			p.HighRiskCount++
		}
	}
}

// SortedItems 返回按 category 优先级和 risk 排序的 items 副本。
// 顺序：conflict > duplicate > stale/expired > new/preference/decision/lesson/commitment。
// 高风险优先于低风险。
func (p InboxPack) SortedItems() []InboxItem {
	items := make([]InboxItem, len(p.Items))
	copy(items, p.Items)
	sort.SliceStable(items, func(i, j int) bool {
		pi := categoryPriority(items[i].Category)
		pj := categoryPriority(items[j].Category)
		if pi != pj {
			return pi < pj
		}
		return riskValue(items[i].Risk) > riskValue(items[j].Risk)
	})
	return items
}

func isValidCategory(c InboxCategory) bool {
	for _, valid := range AllCategories() {
		if c == valid {
			return true
		}
	}
	return false
}

func isValidRisk(r RiskLevel) bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

// categoryPriority 返回 category 的排序优先级（越小越优先）。
func categoryPriority(c InboxCategory) int {
	switch c {
	case CategoryConflict:
		return 0
	case CategoryDuplicate:
		return 1
	case CategoryStale:
		return 2
	case CategoryExpired:
		return 3
	case CategoryNewFact:
		return 4
	case CategoryPreference:
		return 5
	case CategoryDecision:
		return 6
	case CategoryLesson:
		return 7
	case CategoryCommitment:
		return 8
	}
	return 99
}

// riskValue 返回 risk 的数值（越大越危险）。
func riskValue(r RiskLevel) int {
	switch r {
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	case RiskLow:
		return 1
	}
	return 0
}
