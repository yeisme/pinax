package agentprotocol

import (
	"fmt"
	"strings"
)

// SchemaVersion 是 Agent memory runtime DTO 的 schema 版本。
const SchemaVersion = "yeisme.agent_memory.v1"

// TrustLevel 表示 principal 的信任等级，决定默认可执行操作。
type TrustLevel string

const (
	// TrustLevelAdapter — 外部 Agent runtime adapter，默认只能 propose。
	TrustLevelAdapter TrustLevel = "adapter"
	// TrustLevelCollaborator — 协作者，可 review 但不能直接 confirm。
	TrustLevelCollaborator TrustLevel = "collaborator"
	// TrustLevelOwner — vault owner，可直接 confirm 和 approve。
	TrustLevelOwner TrustLevel = "owner"
)

// Capability 表示 principal 持有的能力。Agent 默认只能 propose；
// confirmed mutation 需要 owner capability 或显式 policy 授权。
type Capability string

const (
	CapabilityRead     Capability = "read"
	CapabilityPropose  Capability = "propose"
	CapabilityReview   Capability = "review"
	CapabilityApprove  Capability = "approve"
	CapabilityConfirm  Capability = "confirm"
	CapabilityHandoff  Capability = "handoff"
	CapabilityFeedback Capability = "feedback"
)

// Principal 表示发起 memory 操作的 actor。
// Runtime 只用于 adapter metadata，不参与 memory identity。
// PrincipalID 是稳定身份标识，跨 session 保持一致。
type Principal struct {
	SchemaVersion string       `json:"schema_version"`
	PrincipalID   string       `json:"principal_id"`
	Runtime       string       `json:"runtime,omitempty"`
	AgentID       string       `json:"agent_id,omitempty"`
	OwnerID       string       `json:"owner_id,omitempty"`
	WorkspaceID   string       `json:"workspace_id,omitempty"`
	Capabilities  []Capability `json:"capabilities"`
	Trust         TrustLevel   `json:"trust"`
	// Metadata 携带 optional adapter-specific 信息，unknown key 应被忽略。
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate 检查 principal 必填字段和 trust/capability 一致性。
func (p Principal) Validate() error {
	if strings.TrimSpace(p.PrincipalID) == "" {
		return NewStableError(ErrCodeValidationFailed, "principal_id is required")
	}
	if p.Trust != "" {
		if !validTrustLevels[p.Trust] {
			return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown trust level: %q", p.Trust))
		}
	}
	for _, cap := range p.Capabilities {
		if !validCapabilities[cap] {
			return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown capability: %q", cap))
		}
	}
	return nil
}

// HasCapability 判断 principal 是否持有给定能力。
func (p Principal) HasCapability(cap Capability) bool {
	for _, c := range p.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// CanConfirm 判断 principal 是否默认可以直接 confirm memory。
// 只有 owner trust 或显式 confirm capability 才允许。
func (p Principal) CanConfirm() bool {
	return p.Trust == TrustLevelOwner || p.HasCapability(CapabilityConfirm)
}

var validTrustLevels = map[TrustLevel]bool{
	TrustLevelAdapter:      true,
	TrustLevelCollaborator: true,
	TrustLevelOwner:        true,
}

var validCapabilities = map[Capability]bool{
	CapabilityRead:     true,
	CapabilityPropose:  true,
	CapabilityReview:   true,
	CapabilityApprove:  true,
	CapabilityConfirm:  true,
	CapabilityHandoff:  true,
	CapabilityFeedback: true,
}

// DefaultAdapterPrincipal 构造一个典型 adapter principal（只能 propose/read/feedback）。
// 用于 reference adapter harness 和测试。
func DefaultAdapterPrincipal(principalID, runtime string) Principal {
	return Principal{
		SchemaVersion: SchemaVersion,
		PrincipalID:   principalID,
		Runtime:       runtime,
		Trust:         TrustLevelAdapter,
		Capabilities:  []Capability{CapabilityRead, CapabilityPropose, CapabilityFeedback},
	}
}
