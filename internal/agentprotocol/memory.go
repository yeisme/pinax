package agentprotocol

import (
	"fmt"
	"strings"
	"time"
)

// MemoryKind 是 memory record 的语义类型。
// 旧消费者应把未知 kind 当通用 record 处理，而不是失败。
type MemoryKind string

const (
	MemoryKindFact       MemoryKind = "fact"
	MemoryKindDecision   MemoryKind = "decision"
	MemoryKindPreference MemoryKind = "preference"
	MemoryKindProcedure  MemoryKind = "procedure"
	MemoryKindEvent      MemoryKind = "event"
	MemoryKindTask       MemoryKind = "task"
	MemoryKindFailure    MemoryKind = "failure"
)

var validMemoryKinds = map[MemoryKind]bool{
	MemoryKindFact:       true,
	MemoryKindDecision:   true,
	MemoryKindPreference: true,
	MemoryKindProcedure:  true,
	MemoryKindEvent:      true,
	MemoryKindTask:       true,
	MemoryKindFailure:    true,
}

// LifecycleState 是 memory record 的生命周期状态。
type LifecycleState string

const (
	LifecycleProposed   LifecycleState = "proposed"
	LifecycleConfirmed  LifecycleState = "confirmed"
	LifecycleRejected   LifecycleState = "rejected"
	LifecycleSuperseded LifecycleState = "superseded"
	LifecycleExpired    LifecycleState = "expired"
	LifecycleConflicted LifecycleState = "conflicted"
)

var validLifecycleStates = map[LifecycleState]bool{
	LifecycleProposed:   true,
	LifecycleConfirmed:  true,
	LifecycleRejected:   true,
	LifecycleSuperseded: true,
	LifecycleExpired:    true,
	LifecycleConflicted: true,
}

// Confidence 表示 memory 的置信度分级。
type Confidence string

const (
	ConfidenceLow      Confidence = "low"
	ConfidenceMedium   Confidence = "medium"
	ConfidenceHigh     Confidence = "high"
	ConfidenceVerified Confidence = "verified"
)

var validConfidence = map[Confidence]bool{
	ConfidenceLow:      true,
	ConfidenceMedium:   true,
	ConfidenceHigh:     true,
	ConfidenceVerified: true,
}

// MemoryRecord 是 Agent memory 的 canonical runtime view。
// 在现有 internal/memory.Record 之上增加 kind、scope、source、creator、
// supersession/conflict 和 schema version。
type MemoryRecord struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	Kind          MemoryKind     `json:"kind"`
	Scope         Scope          `json:"scope"`
	State         LifecycleState `json:"state"`
	Subject       string         `json:"subject,omitempty"`
	Predicate     string         `json:"predicate,omitempty"`
	Object        string         `json:"object,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Confidence    Confidence     `json:"confidence,omitempty"`
	Sources       SourceRefList  `json:"sources,omitempty"`
	CreatorID     string         `json:"creator_id"`
	SupersedesID  string         `json:"supersedes_id,omitempty"`
	ConflictsWith []string       `json:"conflicts_with,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	// Metadata 携带 optional adapter-specific 信息，unknown key 应被忽略。
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate 检查 memory record 的必填字段和枚举值。
func (m MemoryRecord) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return NewStableError(ErrCodeValidationFailed, "memory id is required")
	}
	if !validMemoryKinds[m.Kind] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown memory kind: %q", m.Kind))
	}
	if err := m.Scope.Validate(); err != nil {
		return err
	}
	if !validLifecycleStates[m.State] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown lifecycle state: %q", m.State))
	}
	if m.Confidence != "" && !validConfidence[m.Confidence] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown confidence: %q", m.Confidence))
	}
	if strings.TrimSpace(m.CreatorID) == "" {
		return NewStableError(ErrCodeValidationFailed, "creator_id is required")
	}
	if err := m.Sources.Validate(); err != nil {
		return err
	}
	return nil
}

// IsRecallable 判断 memory 是否应出现在默认 confirmed recall 结果中。
// proposed/rejected/expired/superseded 默认不参与 recall。
func (m MemoryRecord) IsRecallable() bool {
	switch m.State {
	case LifecycleConfirmed, LifecycleConflicted:
		return true
	default:
		return false
	}
}

// IsExpired 判断 memory 是否已过期（基于 ExpiresAt）。
func (m MemoryRecord) IsExpired(now time.Time) bool {
	return m.ExpiresAt != nil && !m.ExpiresAt.After(now) && !m.ExpiresAt.Equal(now)
}

// ValidTransition 判断 lifecycle 转换是否合法。
// 非法 transition 返回 false，调用方应返回稳定 error code。
func ValidTransition(from, to LifecycleState) bool {
	switch from {
	case LifecycleProposed:
		return to == LifecycleConfirmed || to == LifecycleRejected
	case LifecycleConfirmed:
		return to == LifecycleSuperseded || to == LifecycleExpired || to == LifecycleConflicted
	case LifecycleConflicted:
		return to == LifecycleConfirmed || to == LifecycleSuperseded
	case LifecycleSuperseded:
		return to == LifecycleConfirmed
	case LifecycleExpired:
		return to == LifecycleConfirmed
	default:
		return false
	}
}
