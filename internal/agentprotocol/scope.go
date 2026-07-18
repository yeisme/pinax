package agentprotocol

import (
	"fmt"
	"strings"
)

// ScopeKind 枚举从宽到窄的 scope 层级。
// scope 继承：owner > workspace > project > repository > session > task。
// 高层 scope 的 memory 对低层 scope 可见（除非被 permission 显式拒绝）。
type ScopeKind string

const (
	ScopeKindOwner      ScopeKind = "owner"
	ScopeKindWorkspace  ScopeKind = "workspace"
	ScopeKindProject    ScopeKind = "project"
	ScopeKindRepository ScopeKind = "repository"
	ScopeKindSession    ScopeKind = "session"
	ScopeKindTask       ScopeKind = "task"
)

// validScopeKinds 是允许的 scope kind 集合，用于校验。
var validScopeKinds = map[ScopeKind]bool{
	ScopeKindOwner:      true,
	ScopeKindWorkspace:  true,
	ScopeKindProject:    true,
	ScopeKindRepository: true,
	ScopeKindSession:    true,
	ScopeKindTask:       true,
}

// Scope 表示一个 Agent memory 的归属范围。
// Kind 是层级，ID 是该层级的稳定标识（如 owner_id、workspace_id、project_path）。
// Request 必须显式 scope；缺省值只能由已注册 profile/application service 补齐。
type Scope struct {
	Kind ScopeKind `json:"kind"`
	ID   string    `json:"id"`
}

// Validate 检查 scope kind 和 ID 是否合法。
func (s Scope) Validate() error {
	if !validScopeKinds[s.Kind] {
		return NewStableError(ErrCodeInvalidScope, fmt.Sprintf("unknown scope kind: %q", s.Kind))
	}
	if strings.TrimSpace(s.ID) == "" {
		return NewStableError(ErrCodeInvalidScope, fmt.Sprintf("scope %s id is required", s.Kind))
	}
	return nil
}

// Contains 判断 receiver scope 是否覆盖 given scope（层级 >= given 即覆盖）。
// owner 覆盖 workspace，workspace 覆盖 project，以此类推。
// 相同 kind + 相同 ID 视为包含。
func (s Scope) Contains(other Scope) bool {
	if s.Kind == other.Kind {
		return s.ID == other.ID
	}
	return scopeRank(s.Kind) <= scopeRank(other.Kind)
}

// scopeRank 返回层级权重，值越小层级越高（owner=0）。
func scopeRank(k ScopeKind) int {
	switch k {
	case ScopeKindOwner:
		return 0
	case ScopeKindWorkspace:
		return 1
	case ScopeKindProject:
		return 2
	case ScopeKindRepository:
		return 3
	case ScopeKindSession:
		return 4
	case ScopeKindTask:
		return 5
	default:
		return 99
	}
}
