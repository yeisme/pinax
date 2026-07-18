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
type SourceCoverage struct {
	// Total 是 pack 引用的来源总数。
	Total int `json:"total"`
	// Resolved 是成功解析的来源数。
	Resolved int `json:"resolved"`
	// Missing 是无法解析的来源数。
	Missing int `json:"missing"`
	// Stale 是存在但过期的来源数。
	Stale int `json:"stale"`
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
	// Freshness 描述 pack 中最新记忆的时间戳。
	Freshness time.Time `json:"freshness"`
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
