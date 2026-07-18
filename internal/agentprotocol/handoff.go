package agentprotocol

import (
	"fmt"
	"strings"
	"time"
)

// HandoffSchemaVersion 是 cross-agent handoff 的 schema 版本。
const HandoffSchemaVersion = "yeisme.agent_handoff.v1"

// Handoff 携带 bounded working state，用于 cross-agent 任务交接。
// 不等同于 confirmed memory；不要求共享完整 transcript 或 model reasoning。
type Handoff struct {
	SchemaVersion string    `json:"schema_version"`
	HandoffID     string    `json:"handoff_id"`
	FromPrincipal Principal `json:"from_principal"`
	ToPrincipal   Principal `json:"to_principal"`
	Scope         Scope     `json:"scope"`
	Objective     string    `json:"objective"`
	CreatedAt     time.Time `json:"created_at"`
	// CurrentState 描述任务当前进度摘要。
	CurrentState string `json:"current_state,omitempty"`
	// Decisions 是已做出的关键决策列表。
	Decisions []string `json:"decisions,omitempty"`
	// CompletedWork 是已完成的工作项。
	CompletedWork []string `json:"completed_work,omitempty"`
	// Blockers 是当前阻塞项。
	Blockers []string `json:"blockers,omitempty"`
	// Verification 是验证标准或已验证结果。
	Verification []string `json:"verification,omitempty"`
	// FollowUps 是建议的后续操作。
	FollowUps []string `json:"follow_ups,omitempty"`
	// Sources 引用相关 evidence。
	Sources SourceRefList `json:"sources,omitempty"`
	// RequestedNextCapability 是接收方应具备的能力。
	RequestedNextCapability string `json:"requested_next_capability,omitempty"`
}

// Validate 检查 handoff 必填字段。
func (h Handoff) Validate() error {
	if strings.TrimSpace(h.HandoffID) == "" {
		return NewStableError(ErrCodeValidationFailed, "handoff_id is required")
	}
	if err := h.FromPrincipal.Validate(); err != nil {
		return fmt.Errorf("from_principal: %w", err)
	}
	if err := h.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if strings.TrimSpace(h.Objective) == "" {
		return NewStableError(ErrCodeValidationFailed, "handoff objective is required")
	}
	if err := h.Sources.Validate(); err != nil {
		return fmt.Errorf("sources: %w", err)
	}
	return nil
}
