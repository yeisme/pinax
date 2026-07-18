package agentprotocol

import (
	"fmt"
	"strings"
	"time"
)

// ProposalStatus 是 memory proposal 的 review 结果状态。
type ProposalStatus string

const (
	ProposalStatusDraftSaved       ProposalStatus = "draft_saved"
	ProposalStatusApprovalRequired ProposalStatus = "approval_required"
	ProposalStatusConflictRequired ProposalStatus = "conflict_required"
	ProposalStatusApproved         ProposalStatus = "approved"
	ProposalStatusRejected         ProposalStatus = "rejected"
	ProposalStatusSuperseded       ProposalStatus = "superseded"
)

// RiskLevel 是 proposal 的风险分级，影响是否需要更高级别审批。
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ProposalStatusReason 是 review service 返回的稳定原因码。
type ProposalStatusReason string

const (
	ReasonValid            ProposalStatusReason = "valid"
	ReasonDuplicate        ProposalStatusReason = "duplicate"
	ReasonConflictExisting ProposalStatusReason = "conflict_existing"
	ReasonUnsourced        ProposalStatusReason = "unsourced"
	ReasonPolicyDenied     ProposalStatusReason = "policy_denied"
	ReasonScopeMismatch    ProposalStatusReason = "scope_mismatch"
)

// Proposal 是 Agent 提交的 memory 候选。
// Agent 默认只能 propose；review 通过后由 owner service confirm。
type Proposal struct {
	SchemaVersion  string         `json:"schema_version"`
	ProposalID     string         `json:"proposal_id"`
	Principal      Principal      `json:"principal"`
	Scope          Scope          `json:"scope"`
	Kind           MemoryKind     `json:"kind"`
	Subject        string         `json:"subject,omitempty"`
	Summary        string         `json:"summary,omitempty"`
	Object         string         `json:"object,omitempty"`
	RequestedState LifecycleState `json:"requested_state"`
	Sources        SourceRefList  `json:"sources,omitempty"`
	Risk           RiskLevel      `json:"risk,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Validate 检查 proposal 必填字段。
func (p Proposal) Validate() error {
	if strings.TrimSpace(p.ProposalID) == "" {
		return NewStableError(ErrCodeValidationFailed, "proposal_id is required")
	}
	if err := p.Principal.Validate(); err != nil {
		return fmt.Errorf("principal: %w", err)
	}
	if err := p.Scope.Validate(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if !validMemoryKinds[p.Kind] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown kind: %q", p.Kind))
	}
	if p.RequestedState != "" && !validLifecycleStates[p.RequestedState] {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown requested_state: %q", p.RequestedState))
	}
	if err := p.Sources.Validate(); err != nil {
		return fmt.Errorf("sources: %w", err)
	}
	return nil
}

// ProposalReview 是 review service 对 proposal 的评估结果。
type ProposalReview struct {
	ProposalID string               `json:"proposal_id"`
	Status     ProposalStatus       `json:"status"`
	Reason     ProposalStatusReason `json:"reason"`
	Message    string               `json:"message,omitempty"`
	// ConflictingMemoryIDs 列出与 proposal 冲突的现有 memory。
	ConflictingMemoryIDs []string `json:"conflicting_memory_ids,omitempty"`
	// DuplicateMemoryID 如果 proposal 与现有 memory 重复，指向该 memory。
	DuplicateMemoryID string `json:"duplicate_memory_id,omitempty"`
}

// Receipt 是 proposal/handoff/feedback 写操作的脱敏审计记录。
// 不包含完整 body 或 secret。
type Receipt struct {
	SchemaVersion string         `json:"schema_version"`
	ReceiptID     string         `json:"receipt_id"`
	Kind          string         `json:"kind"` // proposal_approved, handoff_created, feedback_added 等
	PrincipalID   string         `json:"principal_id"`
	Scope         Scope          `json:"scope"`
	MemoryID      string         `json:"memory_id,omitempty"`
	ProposalID    string         `json:"proposal_id,omitempty"`
	SourceRefs    SourceRefList  `json:"source_refs,omitempty"`
	LifecycleFrom LifecycleState `json:"lifecycle_from,omitempty"`
	LifecycleTo   LifecycleState `json:"lifecycle_to,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
