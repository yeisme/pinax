// Package agentprotocol defines the versioned, provider-neutral Agent memory
// runtime contract: principal, scope, memory record, context request/pack,
// proposal, handoff, feedback and adapter descriptor.
//
// 此包只持有 DTO、校验和稳定错误码；不依赖 GORM、CLI 渲染或具体 runtime SDK。
// 所有 schema 使用 `yeisme.agent_*.*.v1` 版本号，unknown optional metadata 应可忽略。
package agentprotocol

// StableError 是 Agent memory runtime 的稳定错误信封。
// Code 是稳定英文标识符（snake_case），用于跨 runtime/transport 对齐；
// Message 是人类可读描述；Details 携带可选 diagnostic key/value。
type StableError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func (e *StableError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// 常用稳定错误码（stable English codes）。
const (
	// ErrCodeInvalidScope — scope 缺失、格式错误或不可解析。
	ErrCodeInvalidScope = "invalid_scope"
	// ErrCodeInsufficientScope — principal 权限不足以覆盖请求 scope。
	ErrCodeInsufficientScope = "insufficient_scope"
	// ErrCodePermissionUnknown — source 可见性无法判定（private source 越权）。
	ErrCodePermissionUnknown = "permission_unknown"
	// ErrCodeValidationFailed — DTO 字段校验失败。
	ErrCodeValidationFailed = "validation_failed"
	// ErrCodeApprovalRequired — 写操作需要 owner 显式批准。
	ErrCodeApprovalRequired = "approval_required"
	// ErrCodeConflictRequired — 存在冲突 confirmed memory，需要显式解决。
	ErrCodeConflictRequired = "conflict_required"
	// ErrCodeRejected — proposal 被 review 拒绝。
	ErrCodeRejected = "rejected"
	// ErrCodeAdapterUnavailable — reference adapter 不可用或版本不支持。
	ErrCodeAdapterUnavailable = "adapter_unavailable"
	// ErrCodeBudgetExceeded — context pack 超出请求 budget（非致命，仅 truncated）。
	ErrCodeBudgetExceeded = "budget_exceeded"
)

// NewStableError 构造一个 StableError。
func NewStableError(code, message string) *StableError {
	return &StableError{Code: code, Message: message}
}

// WithDetail 追加一个 detail key/value 并返回原 error（便于链式调用）。
func (e *StableError) WithDetail(key, value string) *StableError {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}
