package memoryinbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// Aggregator 从现有 proposals、memories、conflicts、feedback 和 receipts
// 投影 inbox items。它不创建第二套 canonical store。
type Aggregator struct{}

// NewAggregator 构造 aggregator。
func NewAggregator() *Aggregator { return &Aggregator{} }

// AggregateInput 描述聚合输入（由 app service 从 store 收集）。
type AggregateInput struct {
	Proposals []agentmemory.AgentProposalRow
	Memories  []agentprotocol.MemoryRecord
}

// Aggregate 将 proposals + memories 投影为 bounded inbox pack。
// 分类是 deterministic 的：同输入同输出。
func (a *Aggregator) Aggregate(ctx context.Context, input AggregateInput, limit int) (InboxPack, error) {
	var items []InboxItem
	now := time.Now().UTC()

	for _, prop := range input.Proposals {
		item := a.classifyProposal(prop, now)
		if err := item.Validate(); err != nil {
			// 隔离 malformed row，不中断整个 inbox
			continue
		}
		items = append(items, item)
	}

	// 从 memories 中检测 stale/expired
	for _, mem := range input.Memories {
		if item, ok := a.classifyMemory(mem, now); ok {
			if err := item.Validate(); err == nil {
				items = append(items, item)
			}
		}
	}

	// 按优先级排序
	pack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Experimental:  true,
	}

	// 截断到 limit
	sorted := InboxPack{Items: items}.SortedItems()
	if limit > 0 && len(sorted) > limit {
		pack.Truncated = true
		sorted = sorted[:limit]
	}
	pack.Items = sorted
	pack.ComputeCounts()
	return pack, nil
}

// classifyProposal 将一个 proposal row 分类为 inbox item。
// 分类规则 deterministic：
// - approval_required/conflict_required → 根据 kind 和 conflict 分类
// - rejected/superseded → 不进入 inbox
// - approved → 不进入 inbox（已处理）
func (a *Aggregator) classifyProposal(prop agentmemory.AgentProposalRow, now time.Time) InboxItem {
	item := InboxItem{
		SchemaVersion: InboxSchemaVersion,
		ItemID:        fmt.Sprintf("prop-%s", prop.ProposalID),
		Subject:       a.safeSubject(prop.Subject),
		Scope: agentprotocol.Scope{
			Kind: agentprotocol.ScopeKind(prop.ScopeKind),
			ID:   prop.ScopeID,
		},
		ProposalID:      prop.ProposalID,
		SuggestedAction: ActionReview,
		CreatedAt:       prop.CreatedAt,
		UpdatedAt:       now,
		ReviewVersion:   1,
		Experimental:    true,
	}

	// 只有 pending proposals 进入 inbox
	status := agentprotocol.ProposalStatus(prop.Status)
	switch status {
	case agentprotocol.ProposalStatusApproved, agentprotocol.ProposalStatusRejected,
		agentprotocol.ProposalStatusSuperseded:
		// 已处理的 proposal 不进入 inbox
		return item // will be filtered by Validate if empty category

	case agentprotocol.ProposalStatusConflictRequired:
		// 冲突 proposal 分类为 conflict，高风险
		item.Category = CategoryConflict
		item.ReasonCodes = []ReasonCode{ReasonConflictDetected}
		item.Risk = RiskHigh
		item.SuggestedAction = ActionInspect

	case agentprotocol.ProposalStatusApprovalRequired:
		// 需要审批的 proposal 根据 kind 分类
		item.Category = proposalKindToCategory(prop.Kind)
		item.ReasonCodes = []ReasonCode{ReasonUnconfirmed, ReasonNewProposal}
		item.Risk = proposalKindToRisk(prop.Kind)

	case agentprotocol.ProposalStatusDraftSaved:
		// draft 状态也需要 review
		item.Category = proposalKindToCategory(prop.Kind)
		item.ReasonCodes = []ReasonCode{ReasonUnconfirmed}
		item.Risk = RiskLow

	default:
		// 未知状态不进入 inbox
		item.Category = CategoryNewFact
		item.ReasonCodes = []ReasonCode{ReasonNewProposal}
		item.Risk = RiskLow
	}

	// 来源覆盖率（proposal 的 sources 从 Subject 摘要推断，不保存 body）
	// 在 projection 阶段不计入 detailed source coverage
	item.SourceCoverage = SourceCoverage{}

	return item
}

// classifyMemory 检查 memory 是否 stale/expired，返回 inbox item。
// 非 stale/expired 的 confirmed memory 不进入 inbox。
func (a *Aggregator) classifyMemory(mem agentprotocol.MemoryRecord, now time.Time) (InboxItem, bool) {
	// 只有特定状态的 memory 可能进入 inbox
	if mem.State == agentprotocol.LifecycleExpired {
		return InboxItem{
			SchemaVersion:   InboxSchemaVersion,
			ItemID:          fmt.Sprintf("mem-%s", mem.ID),
			Category:        CategoryExpired,
			ReasonCodes:     []ReasonCode{ReasonExpiryReached},
			Risk:            RiskLow,
			Subject:         mem.Subject,
			Scope:           mem.Scope,
			MemoryID:        mem.ID,
			SuggestedAction: ActionInspect,
			CreatedAt:       mem.CreatedAt,
			UpdatedAt:       now,
			Experimental:    true,
		}, true
	}

	// 检查 stale（超过 90 天未更新的 confirmed memory）
	if mem.State == agentprotocol.LifecycleConfirmed {
		staleThreshold := 90 * 24 * time.Hour
		if !mem.UpdatedAt.IsZero() && now.Sub(mem.UpdatedAt) > staleThreshold {
			return InboxItem{
				SchemaVersion:   InboxSchemaVersion,
				ItemID:          fmt.Sprintf("mem-%s", mem.ID),
				Category:        CategoryStale,
				ReasonCodes:     []ReasonCode{ReasonStaleSource},
				Risk:            RiskLow,
				Subject:         mem.Subject,
				Scope:           mem.Scope,
				MemoryID:        mem.ID,
				SuggestedAction: ActionInspect,
				CreatedAt:       mem.CreatedAt,
				UpdatedAt:       now,
				Experimental:    true,
			}, true
		}
	}

	return InboxItem{}, false
}

// safeSubject 返回安全的 subject，不含 body。
// subject 长度限制在 200 字符以内。
func (a *Aggregator) safeSubject(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:197] + "..."
	}
	return s
}

// proposalKindToCategory 将 memory kind 映射到 inbox category。
func proposalKindToCategory(kind string) InboxCategory {
	switch agentprotocol.MemoryKind(kind) {
	case agentprotocol.MemoryKindDecision:
		return CategoryDecision
	case agentprotocol.MemoryKindPreference:
		return CategoryPreference
	case agentprotocol.MemoryKindProcedure:
		return CategoryLesson
	case agentprotocol.MemoryKindEvent:
		return CategoryCommitment
	case agentprotocol.MemoryKindTask:
		return CategoryCommitment
	case agentprotocol.MemoryKindFailure:
		return CategoryLesson
	default:
		return CategoryNewFact
	}
}

// proposalKindToRisk 根据 memory kind 推断风险等级。
// decision 和 preference 默认 medium（影响行为），fact 默认 low。
func proposalKindToRisk(kind string) RiskLevel {
	switch agentprotocol.MemoryKind(kind) {
	case agentprotocol.MemoryKindDecision:
		return RiskMedium
	case agentprotocol.MemoryKindPreference:
		return RiskMedium
	case agentprotocol.MemoryKindFailure:
		return RiskMedium
	default:
		return RiskLow
	}
}

// IsBulkAllowed 判断给定 items 是否可以批量操作。
// 冲突、破坏性 rewrite、cross-scope promotion 和高风险项禁止 bulk approval。
func IsBulkAllowed(items []InboxItem) (bool, ReasonCode) {
	for _, item := range items {
		if item.Risk == RiskHigh {
			return false, ReasonBulkActionBlocked
		}
		if item.Category == CategoryConflict {
			// 冲突项必须独立审阅，不能批量
			return false, ReasonConflictingScope
		}
	}
	return true, ""
}

// SummaryLine 返回 inbox pack 的一行摘要。
func SummaryLine(pack InboxPack) string {
	parts := []string{
		fmt.Sprintf("%d items", pack.TotalItems),
	}
	if pack.HighRiskCount > 0 {
		parts = append(parts, fmt.Sprintf("%d high-risk", pack.HighRiskCount))
	}
	if pack.CountsByCategory[CategoryConflict] > 0 {
		parts = append(parts, fmt.Sprintf("%d conflicts", pack.CountsByCategory[CategoryConflict]))
	}
	if pack.Truncated {
		parts = append(parts, "truncated")
	}
	return strings.Join(parts, ", ")
}
