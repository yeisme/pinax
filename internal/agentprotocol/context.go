package agentprotocol

import (
	"fmt"
	"strings"
)

// ContextSchemaVersion 是 context request/pack 的 schema 版本。
const ContextSchemaVersion = "yeisme.agent_context_pack.v1"

// ContextBudget 限制 context pack 的大小。
// Compiler 按确定性顺序截断，超限时设 truncated=true 并提供 drill-down actions。
type ContextBudget struct {
	MaxItems int `json:"max_items,omitempty"`
	MaxChars int `json:"max_chars,omitempty"`
}

// ContextRequest 描述一次 context compilation 请求。
// Principal 和 Scope 必须显式提供；intent/entities/filter 可选。
type ContextRequest struct {
	SchemaVersion string        `json:"schema_version"`
	Principal     Principal     `json:"principal"`
	Scope         Scope         `json:"scope"`
	Task          string        `json:"task,omitempty"`
	Intent        string        `json:"intent,omitempty"`
	Entities      []string      `json:"entities,omitempty"`
	KindFilter    []MemoryKind  `json:"kind_filter,omitempty"`
	Budget        ContextBudget `json:"budget,omitempty"`
}

// Validate 检查 request 必填字段。
func (r ContextRequest) Validate() error {
	if err := r.Principal.Validate(); err != nil {
		return fmt.Errorf("principal: %w", err)
	}
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	for i, k := range r.KindFilter {
		if !validMemoryKinds[k] {
			return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("kind_filter[%d]: unknown kind %q", i, k))
		}
	}
	if r.Budget.MaxItems < 0 || r.Budget.MaxChars < 0 {
		return NewStableError(ErrCodeValidationFailed, "budget values must be non-negative")
	}
	return nil
}

// ContextEntry 是 context pack 中的一个 bounded 条目。
// 不输出完整 note body；Preview 是受限摘要。
type ContextEntry struct {
	MemoryID    string         `json:"memory_id"`
	Kind        MemoryKind     `json:"kind"`
	Subject     string         `json:"subject,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Preview     string         `json:"preview,omitempty"`
	Confidence  Confidence     `json:"confidence,omitempty"`
	Sources     SourceRefList  `json:"sources,omitempty"`
	State       LifecycleState `json:"state"`
	ScoreReason string         `json:"score_reason,omitempty"`
}

// ContextConflict 描述一组冲突的 memory。
type ContextConflict struct {
	MemoryIDs []string `json:"memory_ids"`
	Reason    string   `json:"reason,omitempty"`
}

// NextAction 是 context pack 提供的安全 drill-down 操作建议。
type NextAction struct {
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// ContextPack 是 compiler 产出的 bounded context bundle。
type ContextPack struct {
	SchemaVersion  string            `json:"schema_version"`
	Principal      Principal         `json:"principal"`
	Scope          Scope             `json:"scope"`
	Facts          []ContextEntry    `json:"facts,omitempty"`
	Decisions      []ContextEntry    `json:"decisions,omitempty"`
	Preferences    []ContextEntry    `json:"preferences,omitempty"`
	Procedures     []ContextEntry    `json:"procedures,omitempty"`
	OpenTasks      []ContextEntry    `json:"open_tasks,omitempty"`
	FailedAttempts []ContextEntry    `json:"failed_attempts,omitempty"`
	Conflicts      []ContextConflict `json:"conflicts,omitempty"`
	Sources        SourceRefList     `json:"sources,omitempty"`
	NextActions    []NextAction      `json:"next_actions,omitempty"`
	Truncated      bool              `json:"truncated,omitempty"`
}

// EntryCount 返回 pack 中所有 entry 的总数。
func (p ContextPack) EntryCount() int {
	return len(p.Facts) + len(p.Decisions) + len(p.Preferences) +
		len(p.Procedures) + len(p.OpenTasks) + len(p.FailedAttempts)
}

// EntryPreviewChars 返回所有 entry preview 的总字符数。
func (p ContextPack) EntryPreviewChars() int {
	total := 0
	for _, entries := range [][]ContextEntry{p.Facts, p.Decisions, p.Preferences, p.Procedures, p.OpenTasks, p.FailedAttempts} {
		for _, e := range entries {
			total += len(e.Preview) + len(e.Summary) + len(e.Subject)
		}
	}
	return total
}

// AssertNoBody 报告 pack 中是否有任何 entry 携带了看起来像完整 body 的大段文本。
// 这是一个防御性检查，用于 contract test。
func (p ContextPack) AssertNoBody(maxPreviewChars int) error {
	if maxPreviewChars <= 0 {
		return nil
	}
	check := func(entries []ContextEntry) error {
		for i, e := range entries {
			if len(e.Preview) > maxPreviewChars {
				return NewStableError(ErrCodeValidationFailed,
					fmt.Sprintf("entry[%d] preview exceeds %d chars (kind=%s)", i, maxPreviewChars, e.Kind))
			}
			if strings.Contains(strings.ToLower(e.ScoreReason), "body") {
				return NewStableError(ErrCodeValidationFailed,
					fmt.Sprintf("entry[%d] score_reason references body (kind=%s)", i, e.Kind))
			}
		}
		return nil
	}
	for _, entries := range [][]ContextEntry{p.Facts, p.Decisions, p.Preferences, p.Procedures, p.OpenTasks, p.FailedAttempts} {
		if err := check(entries); err != nil {
			return err
		}
	}
	return nil
}
