package agentmemory

import (
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// Policy 定义 Agent memory runtime 的授权规则。
// adapter 默认只能 propose；unsourced preference 不能自动 confirm；
// confirmed mutation 需要 owner capability 或显式 policy 授权。
type Policy struct {
	// AllowAutoConfirm 控制是否允许在 propose 时自动 confirm。
	// 默认 false：adapter proposal 始终需要 review。
	AllowAutoConfirm bool
}

// DefaultPolicy 返回保守的默认策略。
func DefaultPolicy() Policy {
	return Policy{AllowAutoConfirm: false}
}

// CheckPropose 检查 principal 是否被允许提交 proposal。
// adapter trust 默认有 propose capability；如果没有则拒绝。
func (p Policy) CheckPropose(principal agentprotocol.Principal) error {
	if !principal.HasCapability(agentprotocol.CapabilityPropose) {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeInsufficientScope,
			"principal lacks propose capability",
		).WithDetail("principal_id", principal.PrincipalID)
	}
	return nil
}

// CheckConfirm 检查 principal 是否被允许直接 confirm memory。
// 只有 owner trust 或显式 confirm capability 才允许。
// adapter 默认不能 confirm——必须走 proposal → review → approve 流程。
func (p Policy) CheckConfirm(principal agentprotocol.Principal) error {
	if principal.CanConfirm() {
		return nil
	}
	return agentprotocol.NewStableError(
		agentprotocol.ErrCodeApprovalRequired,
		"principal cannot directly confirm memory; use proposal → review → approve",
	).WithDetail("principal_id", principal.PrincipalID)
}

// CheckReview 检查 principal 是否被允许 review proposal。
func (p Policy) CheckReview(principal agentprotocol.Principal) error {
	if principal.HasCapability(agentprotocol.CapabilityReview) || principal.HasCapability(agentprotocol.CapabilityApprove) {
		return nil
	}
	return agentprotocol.NewStableError(
		agentprotocol.ErrCodeInsufficientScope,
		"principal lacks review/approve capability",
	).WithDetail("principal_id", principal.PrincipalID)
}

// CheckApprove 检查 principal 是否被允许 approve proposal（confirm 结果）。
func (p Policy) CheckApprove(principal agentprotocol.Principal) error {
	if principal.HasCapability(agentprotocol.CapabilityApprove) || principal.Trust == agentprotocol.TrustLevelOwner {
		return nil
	}
	return agentprotocol.NewStableError(
		agentprotocol.ErrCodeInsufficientScope,
		"principal lacks approve capability",
	).WithDetail("principal_id", principal.PrincipalID)
}

// CheckHandoff 检查 principal 是否被允许创建 handoff。
func (p Policy) CheckHandoff(principal agentprotocol.Principal) error {
	if !principal.HasCapability(agentprotocol.CapabilityHandoff) {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeInsufficientScope,
			"principal lacks handoff capability",
		).WithDetail("principal_id", principal.PrincipalID)
	}
	return nil
}

// CheckFeedback 检查 principal 是否被允许提交 feedback。
func (p Policy) CheckFeedback(principal agentprotocol.Principal) error {
	if !principal.HasCapability(agentprotocol.CapabilityFeedback) {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeInsufficientScope,
			"principal lacks feedback capability",
		).WithDetail("principal_id", principal.PrincipalID)
	}
	return nil
}

// EvaluateProposal 运行 proposal 的 source、scope、duplicate、conflict 和 policy 检查。
// 返回 review 结论（status + reason）。
func (p Policy) EvaluateProposal(prop agentprotocol.Proposal, existing []agentprotocol.MemoryRecord) agentprotocol.ProposalReview {
	review := agentprotocol.ProposalReview{
		ProposalID: prop.ProposalID,
		Reason:     agentprotocol.ReasonValid,
	}

	// 1. unsourced → 不能自动 confirm，需要 approval
	if prop.Sources.IsEmpty() {
		review.Status = agentprotocol.ProposalStatusApprovalRequired
		review.Reason = agentprotocol.ReasonUnsourced
		review.Message = "proposal lacks sources; owner approval required"
		return review
	}

	// 2. duplicate 检查：同 scope + 同 subject + 同 kind + 同 object + confirmed
	for _, m := range existing {
		if m.Scope == prop.Scope && m.Subject == prop.Subject &&
			m.Kind == prop.Kind && m.Object == prop.Object &&
			m.State == agentprotocol.LifecycleConfirmed {
			review.Status = agentprotocol.ProposalStatusRejected
			review.Reason = agentprotocol.ReasonDuplicate
			review.DuplicateMemoryID = m.ID
			review.Message = "duplicate of existing confirmed memory"
			return review
		}
	}

	// 3. conflict 检查：同 scope + 同 subject 但不同 object/sources
	var conflicts []string
	for _, m := range existing {
		if m.Scope == prop.Scope && m.Subject == prop.Subject &&
			m.Kind == prop.Kind && m.State == agentprotocol.LifecycleConfirmed &&
			m.Object != prop.Object {
			conflicts = append(conflicts, m.ID)
		}
	}
	if len(conflicts) > 0 {
		review.Status = agentprotocol.ProposalStatusConflictRequired
		review.Reason = agentprotocol.ReasonConflictExisting
		review.ConflictingMemoryIDs = conflicts
		review.Message = "conflicts with existing confirmed memory"
		return review
	}

	// 4. policy: 如果不允许自动 confirm，返回 approval_required
	if !p.AllowAutoConfirm {
		review.Status = agentprotocol.ProposalStatusApprovalRequired
		review.Reason = agentprotocol.ReasonValid
		review.Message = "valid proposal; owner approval required"
		return review
	}

	// 5. auto-confirm allowed
	review.Status = agentprotocol.ProposalStatusApproved
	review.Reason = agentprotocol.ReasonValid
	return review
}
