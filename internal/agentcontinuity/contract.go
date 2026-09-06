// Package agentcontinuity 实现 Agent 连续工作的产品编排层。
//
// Continuity Pack 是 runtime context 的产品 projection，不创建第二套
// memory ranking、ledger 或 source resolver。它只编排 scope resolution、
// handoff selection、context compilation、source coverage、budget truncation
// 和 safe next actions。
package agentcontinuity

import (
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// ContinuitySchemaVersion 是 continuity pack 的 schema 版本。
const ContinuitySchemaVersion = "yeisme.agent_continuity.v1"

// ContinuityRequest 描述一次 continuity compilation 请求。
// Principal 和 Scope 必须显式提供；intent/task 可选。
type ContinuityRequest struct {
	SchemaVersion string                  `json:"schema_version"`
	Principal     agentprotocol.Principal `json:"principal"`
	Scope         agentprotocol.Scope     `json:"scope"`
	Task          string                  `json:"task,omitempty"`
	// Intent 描述用户意图（可选），用于 entity boosting。
	Intent string `json:"intent,omitempty"`
	// Budget 限制 continuity pack 的大小。
	Budget ContinuityBudget `json:"budget"`
	// HandoffID 显式指定要消费的 handoff；空则自动选择最近可消费 handoff。
	HandoffID string `json:"handoff_id,omitempty"`
}

// ContinuityBudget 限制 continuity pack 的 section 大小。
type ContinuityBudget struct {
	MaxItems   int `json:"max_items"`
	MaxChars   int `json:"max_chars"`
	MaxSources int `json:"max_sources"`
}

// DefaultBudget 返回合理的默认 budget。
func DefaultBudget() ContinuityBudget {
	return ContinuityBudget{MaxItems: 20, MaxChars: 6000, MaxSources: 15}
}

// Validate 检查 request 必填字段。
func (r ContinuityRequest) Validate() error {
	if r.SchemaVersion == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "schema_version is required")
	}
	if err := r.Principal.Validate(); err != nil {
		return fmt.Errorf("principal: %w", err)
	}
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if r.Budget.MaxItems <= 0 {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "budget.max_items must be positive")
	}
	if r.Budget.MaxChars <= 0 {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "budget.max_chars must be positive")
	}
	return nil
}

// HandoffStatus 表示 handoff 在 continuity 中的可用状态。
type HandoffStatus string

const (
	// HandoffStatusConsumed 表示找到了可消费的 handoff。
	HandoffStatusConsumed HandoffStatus = "consumed"
	// HandoffStatusMissing 表示没有匹配的 handoff，degrade to context-only。
	HandoffStatusMissing HandoffStatus = "missing"
	// HandoffStatusExplicit 表示用户显式指定了 handoff。
	HandoffStatusExplicit HandoffStatus = "explicit"
	// HandoffStatusDegraded 表示 handoff 存在但 adapter degraded。
	HandoffStatusDegraded HandoffStatus = "degraded"
)

// SourceCoverage 描述 continuity pack 中来源的解析情况。
// Ambiguous 是 additive optional 分桶：ref 不能唯一解析时计入，
// 同时整体视为 unresolved；旧 total/resolved/missing/stale 语义不变。
type SourceCoverage struct {
	// Total 是 pack 引用的来源总数。
	Total int `json:"total"`
	// Resolved 是成功解析的来源数。
	Resolved int `json:"resolved"`
	// Missing 是无法解析的来源数。
	Missing int `json:"missing"`
	// Stale 是存在但过期的来源数。
	Stale int `json:"stale"`
	// Ambiguous 是 ref 不能唯一解析的来源数（additive）。
	Ambiguous int `json:"ambiguous,omitempty"`
}

// ResolvableRatio 返回来源可解析比例。
func (sc SourceCoverage) ResolvableRatio() float64 {
	if sc.Total == 0 {
		return 0
	}
	return float64(sc.Resolved) / float64(sc.Total)
}

// ContinuitySection 是 continuity pack 中的一个 bounded section。
type ContinuitySection struct {
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Items     []string `json:"items,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	Omitted   int      `json:"omitted,omitempty"`
}

// ReviewAttention 是与当前 continuation 相关的单个 bounded review 提示。
// 不含 proposal body；只有会改变 objective/decision/blocker/conflict/
// recommended next action 的 pending item 才能进入。
type ReviewAttention struct {
	ItemID      string   `json:"item_id"`
	Subject     string   `json:"subject"`
	ReasonCodes []string `json:"reason_codes"`
	Risk        string   `json:"risk"`
}

// 状态常量（additive English enum values，machine contract 稳定）。
const (
	// PackStatusReady 表示 handoff、决策与来源均可解析且无冲突。
	PackStatusReady = "ready"
	// PackStatusPartial 表示部分来源 missing/stale、存在 conflict 或
	// handoff 缺失；其余可信 section 保留。
	PackStatusPartial = "partial"

	// EvidenceStatusResolved 表示全部 source 成功解析。
	EvidenceStatusResolved = "resolved"
	// EvidenceStatusPartial 表示至少一个 source stale/missing/ambiguous。
	EvidenceStatusPartial = "partial"
	// EvidenceStatusNotMeasured 表示 total=0（不得当成 100%）。
	EvidenceStatusNotMeasured = "not_measured"

	// FreshnessStatusFresh 表示最新 evidence 在 freshness policy 内。
	FreshnessStatusFresh = "fresh"
	// FreshnessStatusStale 表示 evidence observed/revision time 超出 policy。
	FreshnessStatusStale = "stale"
	// FreshnessStatusNotMeasured 表示没有可计算 freshness 的 evidence。
	FreshnessStatusNotMeasured = "not_measured"

	// FreshnessPolicy 是 evidence freshness 策略窗口（14 天）。
	FreshnessPolicy = 14 * 24 * time.Hour
)

// 稳定 warning code（Resume Card 展示；conflict/source warning 不得被
// budget 丢弃，排序固定以便 golden test）。
const (
	WarningHandoffMissing   = "handoff_missing"
	WarningDecisionConflict = "decision_conflict"
	WarningSourceMissing    = "source_missing"
	WarningSourceStale      = "source_stale"
	WarningSourceAmbiguous  = "source_ambiguous"
	WarningContextDegraded  = "context_degraded"
	WarningBindingMissing   = "binding_missing"
	WarningReviewAttention  = "review_attention"
	// WarningReviewAttentionUnavailable 表示 inbox 状态不可读（IO/store 故障），
	// 0 条 review attention 是"未测量"而不是"没有待办"。
	WarningReviewAttentionUnavailable = "review_attention_unavailable"
)

// ContinuityPack 是 orchestrator 产出的 bounded 产品 projection。
// 它包含目标、关键决策、偏好、未完成事项、失败经验、冲突、来源和下一步命令。
// 完整 note body、完整 transcript、raw prompt 与 provider payload 永不进入。
type ContinuityPack struct {
	SchemaVersion string                  `json:"schema_version"`
	Principal     agentprotocol.Principal `json:"principal"`
	Scope         agentprotocol.Scope     `json:"scope"`
	Task          string                  `json:"task,omitempty"`
	// Objective 来自最近的 handoff 或 scope 推断。
	Objective string `json:"objective,omitempty"`
	// CurrentState 描述任务当前进度。
	CurrentState string `json:"current_state,omitempty"`
	// Sections 按 kind 组织的 bounded sections。
	Sections []ContinuitySection `json:"sections"`
	// Conflicts 描述当前 scope 的记忆冲突。
	Conflicts []agentprotocol.ContextConflict `json:"conflicts,omitempty"`
	// Sources 引用的来源（bounded，不含 body）。
	Sources agentprotocol.SourceRefList `json:"sources,omitempty"`
	// SourceCoverage 描述来源解析情况。
	SourceCoverage SourceCoverage `json:"source_coverage"`
	// HandoffStatus 表示 handoff 的可用性。
	HandoffStatus HandoffStatus `json:"handoff_status"`
	// HandoffID 是被消费的 handoff ID（如有）。
	HandoffID string `json:"handoff_id,omitempty"`
	// Truncated 表示 pack 是否被 budget 截断。
	Truncated bool `json:"truncated"`
	// Freshness 是支持本 pack 的最新 evidence 的 observed/revision 时间。
	// 没有任何 evidence 时为零值；生成时刻见 GeneratedAt。
	Freshness time.Time `json:"freshness"`
	// GeneratedAt 是本次响应的生成时刻（additive；不得冒充 evidence freshness）。
	GeneratedAt time.Time `json:"generated_at,omitempty"`
	// FreshnessStatus 是 freshness 相对 policy 的状态（fresh|stale|not_measured）。
	FreshnessStatus string `json:"freshness_status,omitempty"`
	// EvidenceStatus 是 source evidence 的整体状态（resolved|partial|not_measured）。
	EvidenceStatus string `json:"evidence_status,omitempty"`
	// PackStatus 是 pack 整体状态（ready|partial）。
	PackStatus string `json:"pack_status,omitempty"`
	// WarningCodes 是稳定 warning 分类（bounded，固定排序）。
	WarningCodes []string `json:"warning_codes,omitempty"`
	// RecommendedNextAction 是首屏唯一推荐的下一步；其余 actions 在 NextActions。
	RecommendedNextAction *agentprotocol.NextAction `json:"recommended_next_action,omitempty"`
	// ReviewAttentionCount 是与当前 continuation 相关的 review item 数。
	ReviewAttentionCount int `json:"review_attention_count,omitempty"`
	// ReviewAttention 是唯一 inline 提示的 bounded review item（≤1）。
	ReviewAttention *ReviewAttention `json:"review_attention,omitempty"`
	// ReviewAttentionUnavailable 表示 inbox 状态不可读：0 条 attention 是
	// 未测量而非没有待办，receipt 必须携带 warning code 以区分两者。
	ReviewAttentionUnavailable bool `json:"review_attention_unavailable,omitempty"`
	// BindingStatus 是 binding 解析状态（additive，由 app 层填充）。
	BindingStatus string `json:"binding_status,omitempty"`
	// BindingIDDigest 是解析到的 binding 的 bounded digest（additive）。
	BindingIDDigest string `json:"binding_id_digest,omitempty"`
	// ContinuityRunID 是 opt-in recorded run 的 opaque ID（仅 --record-run）。
	ContinuityRunID string `json:"continuity_run_id,omitempty"`
	// NextActions 是安全的 drill-down 操作建议。
	NextActions []agentprotocol.NextAction `json:"next_actions,omitempty"`
	// Experimental 标记此 surface 为实验性。
	Experimental bool `json:"experimental"`
}

// SectionCount 返回所有 section 的 item 总数。
func (p ContinuityPack) SectionCount() int {
	n := 0
	for _, s := range p.Sections {
		n += len(s.Items)
	}
	return n
}

// AssertNoBody 检查 pack 是否泄漏了看起来像完整 body 的大段文本。
func (p ContinuityPack) AssertNoBody(maxItemChars int) error {
	for _, s := range p.Sections {
		for _, item := range s.Items {
			if len(item) > maxItemChars {
				return fmt.Errorf("section %s item exceeds %d chars (body leak)", s.Kind, maxItemChars)
			}
		}
	}
	return nil
}
