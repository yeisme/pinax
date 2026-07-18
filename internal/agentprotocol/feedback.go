package agentprotocol

import (
	"fmt"
	"strings"
	"time"
)

// FeedbackKind 是 recall feedback 的结论类型。
// Feedback 独立记录结论，不静默改写 memory content。
type FeedbackKind string

const (
	FeedbackUseful       FeedbackKind = "useful"
	FeedbackIrrelevant   FeedbackKind = "irrelevant"
	FeedbackStale        FeedbackKind = "stale"
	FeedbackIncorrect    FeedbackKind = "incorrect"
	FeedbackMissing      FeedbackKind = "missing"
	FeedbackCompleted    FeedbackKind = "completed"
	FeedbackScopeTooWide FeedbackKind = "scope_too_wide"
)

var validFeedbackKinds = map[FeedbackKind]bool{
	FeedbackUseful:       true,
	FeedbackIrrelevant:   true,
	FeedbackStale:        true,
	FeedbackIncorrect:    true,
	FeedbackMissing:      true,
	FeedbackCompleted:    true,
	FeedbackScopeTooWide: true,
}

// Feedback 记录一次 context/recall 的质量反馈。
type Feedback struct {
	SchemaVersion string       `json:"schema_version"`
	FeedbackID    string       `json:"feedback_id"`
	Principal     Principal    `json:"principal"`
	Scope         Scope        `json:"scope"`
	Kind          FeedbackKind `json:"kind"`
	// MemoryID 指向被反馈的 memory（如适用）。
	MemoryID string `json:"memory_id,omitempty"`
	// ContextRequestRef 指向触发反馈的 context request（如适用）。
	ContextRequestRef string    `json:"context_request_ref,omitempty"`
	Comment           string    `json:"comment,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// Validate 检查 feedback 必填字段。
func (f Feedback) Validate() error {
	if strings.TrimSpace(f.FeedbackID) == "" {
		return NewStableError(ErrCodeValidationFailed, "feedback_id is required")
	}
	if err := f.Principal.Validate(); err != nil {
		return fmt.Errorf("principal: %w", err)
	}
	if err := f.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if !validFeedbackKinds[f.Kind] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown feedback kind: %q", f.Kind))
	}
	return nil
}
