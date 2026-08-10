// Package agentmemory 实现 Agent memory runtime 的持久化存储层。
//
// 本包在现有 internal/memory.Record 之上引入 canonical runtime view，增加
// principal、scope、source、supersession、conflict、proposal、handoff 和 feedback。
// 所有 schema 变更都是 additive：只新增表和 nullable 列，不 rename/drop/narrow 现有 schema。
package agentmemory

import (
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentMemoryRow 是 Agent memory record 的持久化行。
// LegacyRecordID nullable 引用旧 internal/memory.Record.ID，用于兼容映射。
type AgentMemoryRow struct {
	ID             string     `gorm:"primaryKey;column:id" json:"id"`
	Kind           string     `gorm:"index;column:kind" json:"kind"`
	ScopeKind      string     `gorm:"index;column:scope_kind" json:"scope_kind"`
	ScopeID        string     `gorm:"index;column:scope_id" json:"scope_id"`
	State          string     `gorm:"index;column:state" json:"state"`
	Subject        string     `gorm:"index;column:subject" json:"subject,omitempty"`
	Predicate      string     `gorm:"column:predicate" json:"predicate,omitempty"`
	Object         string     `gorm:"column:object_row" json:"object,omitempty"`
	Summary        string     `gorm:"column:summary" json:"summary,omitempty"`
	Confidence     string     `gorm:"index;column:confidence" json:"confidence,omitempty"`
	CreatorID      string     `gorm:"index;column:creator_id" json:"creator_id"`
	SupersedesID   string     `gorm:"column:supersedes_id" json:"supersedes_id,omitempty"`
	ExpiresAt      *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	LegacyRecordID string     `gorm:"index;column:legacy_record_id" json:"legacy_record_id,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentMemoryRow) TableName() string { return "agent_memory_records" }

// ToProtocol 将持久化行转换为 protocol DTO。
func (r AgentMemoryRow) ToProtocol(sources []agentprotocol.SourceRef, conflicts []string) agentprotocol.MemoryRecord {
	return agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            r.ID,
		Kind:          agentprotocol.MemoryKind(r.Kind),
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKind(r.ScopeKind), ID: r.ScopeID},
		State:         agentprotocol.LifecycleState(r.State),
		Subject:       r.Subject,
		Predicate:     r.Predicate,
		Object:        r.Object,
		Summary:       r.Summary,
		Confidence:    agentprotocol.Confidence(r.Confidence),
		Sources:       sources,
		CreatorID:     r.CreatorID,
		SupersedesID:  r.SupersedesID,
		ConflictsWith: conflicts,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		ExpiresAt:     r.ExpiresAt,
	}
}

// FromProtocol 将 protocol DTO 转换为持久化行（不含 sources/conflicts，这些存独立表）。
func FromProtocol(m agentprotocol.MemoryRecord) AgentMemoryRow {
	return AgentMemoryRow{
		ID:           m.ID,
		Kind:         string(m.Kind),
		ScopeKind:    string(m.Scope.Kind),
		ScopeID:      m.Scope.ID,
		State:        string(m.State),
		Subject:      m.Subject,
		Predicate:    m.Predicate,
		Object:       m.Object,
		Summary:      m.Summary,
		Confidence:   string(m.Confidence),
		CreatorID:    m.CreatorID,
		SupersedesID: m.SupersedesID,
		ExpiresAt:    m.ExpiresAt,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// AgentSourceRow 是 Agent memory record 的 source 引用。
type AgentSourceRow struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	MemoryID  string    `gorm:"index;column:memory_id" json:"memory_id"`
	Kind      string    `gorm:"index;column:kind" json:"kind"`
	Ref       string    `gorm:"index;column:ref" json:"ref"`
	Label     string    `gorm:"column:label" json:"label,omitempty"`
	Span      string    `gorm:"column:span" json:"span,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentSourceRow) TableName() string { return "agent_memory_sources" }

// AgentConflictRow 记录 memory 之间的冲突关系。
type AgentConflictRow struct {
	ID            string    `gorm:"primaryKey;column:id" json:"id"`
	MemoryID      string    `gorm:"index;column:memory_id" json:"memory_id"`
	ConflictsWith string    `gorm:"index;column:conflicts_with" json:"conflicts_with"`
	Reason        string    `gorm:"column:reason" json:"reason,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentConflictRow) TableName() string { return "agent_memory_conflicts" }

// AgentPrincipalRow 记录已注册的 principal。
type AgentPrincipalRow struct {
	PrincipalID  string    `gorm:"primaryKey;column:principal_id" json:"principal_id"`
	Runtime      string    `gorm:"column:runtime" json:"runtime,omitempty"`
	AgentID      string    `gorm:"column:agent_id" json:"agent_id,omitempty"`
	OwnerID      string    `gorm:"index;column:owner_id" json:"owner_id,omitempty"`
	WorkspaceID  string    `gorm:"index;column:workspace_id" json:"workspace_id,omitempty"`
	Trust        string    `gorm:"column:trust" json:"trust"`
	Capabilities string    `gorm:"column:capabilities" json:"capabilities,omitempty"` // comma-separated
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentPrincipalRow) TableName() string { return "agent_principals" }

// AgentProposalRow 持久化 memory proposal。
type AgentProposalRow struct {
	ProposalID        string     `gorm:"primaryKey;column:proposal_id" json:"proposal_id"`
	PrincipalID       string     `gorm:"index;column:principal_id" json:"principal_id"`
	ScopeKind         string     `gorm:"index;column:scope_kind" json:"scope_kind"`
	ScopeID           string     `gorm:"index;column:scope_id" json:"scope_id"`
	Kind              string     `gorm:"column:kind" json:"kind"`
	Subject           string     `gorm:"column:subject" json:"subject,omitempty"`
	Summary           string     `gorm:"column:summary" json:"summary,omitempty"`
	Object            string     `gorm:"column:object_row" json:"object,omitempty"`
	RequestedState    string     `gorm:"column:requested_state" json:"requested_state"`
	Risk              string     `gorm:"column:risk" json:"risk,omitempty"`
	Reason            string     `gorm:"column:reason" json:"reason,omitempty"`
	Status            string     `gorm:"index;column:status" json:"status"`
	ReviewReason      string     `gorm:"column:review_reason" json:"review_reason,omitempty"`
	ResultingMemoryID string     `gorm:"column:resulting_memory_id" json:"resulting_memory_id,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at" json:"created_at"`
	ReviewedAt        *time.Time `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
}

func (AgentProposalRow) TableName() string { return "agent_proposals" }

// AgentHandoffRow 持久化 cross-agent handoff。
type AgentHandoffRow struct {
	HandoffID               string                      `gorm:"primaryKey;column:handoff_id" json:"handoff_id"`
	FromPrincipal           string                      `gorm:"index;column:from_principal" json:"from_principal"`
	ToPrincipal             string                      `gorm:"index;column:to_principal" json:"to_principal"`
	ScopeKind               string                      `gorm:"column:scope_kind" json:"scope_kind"`
	ScopeID                 string                      `gorm:"column:scope_id" json:"scope_id"`
	Objective               string                      `gorm:"column:objective" json:"objective"`
	CurrentState            string                      `gorm:"column:current_state" json:"current_state,omitempty"`
	Decisions               string                      `gorm:"column:decisions" json:"decisions,omitempty"`           // newline-separated
	CompletedWork           string                      `gorm:"column:completed_work" json:"completed_work,omitempty"` // newline-separated
	Blockers                string                      `gorm:"column:blockers" json:"blockers,omitempty"`
	Verification            string                      `gorm:"column:verification" json:"verification,omitempty"`
	FollowUps               string                      `gorm:"column:follow_ups" json:"follow_ups,omitempty"`
	RequestedNextCapability string                      `gorm:"column:requested_next_capability" json:"requested_next_capability,omitempty"`
	CreatedAt               time.Time                   `gorm:"column:created_at" json:"created_at"`
	Sources                 agentprotocol.SourceRefList `gorm:"-" json:"sources,omitempty"`
}

func (AgentHandoffRow) TableName() string { return "agent_handoffs" }

// AgentHandoffSourceRow 是 handoff 的 source 引用。
type AgentHandoffSourceRow struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	HandoffID string    `gorm:"index;column:handoff_id" json:"handoff_id"`
	Kind      string    `gorm:"index;column:kind" json:"kind"`
	Ref       string    `gorm:"index;column:ref" json:"ref"`
	Label     string    `gorm:"column:label" json:"label,omitempty"`
	Span      string    `gorm:"column:span" json:"span,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentHandoffSourceRow) TableName() string { return "agent_handoff_sources" }

// AgentFeedbackRow 持久化 recall feedback。
type AgentFeedbackRow struct {
	FeedbackID        string    `gorm:"primaryKey;column:feedback_id" json:"feedback_id"`
	PrincipalID       string    `gorm:"index;column:principal_id" json:"principal_id"`
	ScopeKind         string    `gorm:"index;column:scope_kind" json:"scope_kind"`
	ScopeID           string    `gorm:"index;column:scope_id" json:"scope_id"`
	Kind              string    `gorm:"index;column:kind" json:"kind"`
	MemoryID          string    `gorm:"index;column:memory_id" json:"memory_id,omitempty"`
	ContextRequestRef string    `gorm:"column:context_request_ref" json:"context_request_ref,omitempty"`
	Comment           string    `gorm:"column:comment" json:"comment,omitempty"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentFeedbackRow) TableName() string { return "agent_feedback" }
