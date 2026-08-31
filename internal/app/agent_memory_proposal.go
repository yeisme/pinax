package app

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentMemoryProposeRequest 描述一次 propose 操作的输入。
type AgentMemoryProposeRequest struct {
	VaultPath string
	Principal agentprotocol.Principal
	Scope     agentprotocol.Scope
	Kind      agentprotocol.MemoryKind
	Subject   string
	Summary   string
	Object    string
	Sources   agentprotocol.SourceRefList
}

// AgentMemoryPropose 执行 propose → review 流程。
// Agent 默认只能 propose；review 返回 draft_saved/approval_required/conflict_required/rejected。
// 不自动 confirm。
func (s *AgentMemoryService) AgentMemoryPropose(ctx context.Context, req AgentMemoryProposeRequest) (AgentMemoryFacts, *agentprotocol.ProposalReview, error) {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckPropose(req.Principal); err != nil {
		return AgentMemoryFacts{}, nil, err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return AgentMemoryFacts{}, nil, err
	}
	now := nowUTC()
	proposalID := newProposalID(req.Subject + req.Principal.PrincipalID)
	prop := agentprotocol.Proposal{
		SchemaVersion:  agentprotocol.SchemaVersion,
		ProposalID:     proposalID,
		Principal:      req.Principal,
		Scope:          req.Scope,
		Kind:           req.Kind,
		Subject:        req.Subject,
		Summary:        req.Summary,
		Object:         req.Object,
		RequestedState: agentprotocol.LifecycleConfirmed,
		Sources:        req.Sources,
		Reason:         "agent proposal",
		CreatedAt:      now,
	}
	if err := prop.Validate(); err != nil {
		return AgentMemoryFacts{}, nil, err
	}

	// 获取现有 memories 用于 duplicate/conflict 检查
	existing, err := st.ListMemories(ctx, req.Scope)
	if err != nil {
		return AgentMemoryFacts{}, nil, err
	}
	review := s.policy.EvaluateProposal(prop, existing)

	// 持久化 proposal
	if err := st.SaveProposal(ctx, prop, review.Status, review.Reason); err != nil {
		return AgentMemoryFacts{}, nil, err
	}

	facts := AgentMemoryFacts{
		ProposalID:  proposalID,
		Status:      review.Status,
		Reason:      review.Reason,
		Conflicts:   review.ConflictingMemoryIDs,
		DuplicateID: review.DuplicateMemoryID,
	}
	return facts, &review, nil
}

// AgentMemoryListProposals 列出指定 scope 的 proposals。
func (s *AgentMemoryService) AgentMemoryListProposals(ctx context.Context, vaultPath string, scope agentprotocol.Scope) ([]agentmemory.AgentProposalRow, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.ListProposals(ctx, scope)
}

// AgentMemoryShowProposal 显示单个 proposal 详情。
func (s *AgentMemoryService) AgentMemoryShowProposal(ctx context.Context, vaultPath, proposalID string) (agentmemory.AgentProposalRow, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return agentmemory.AgentProposalRow{}, err
	}
	var row agentmemory.AgentProposalRow
	if err := st.DB().WithContext(ctx).First(&row, "proposal_id = ?", proposalID).Error; err != nil {
		return agentmemory.AgentProposalRow{}, fmt.Errorf("get proposal %s: %w", proposalID, err)
	}
	return row, nil
}

// AgentMemoryApprove 批准一条 proposal，创建 confirmed memory 并写 receipt。
// 只有有 approve capability 的 principal 才能执行。
func (s *AgentMemoryService) AgentMemoryApprove(ctx context.Context, vaultPath, proposalID string, approver agentprotocol.Principal) (ApproveFacts, error) {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckApprove(approver); err != nil {
		return ApproveFacts{}, err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return ApproveFacts{}, err
	}
	propRow, err := s.AgentMemoryShowProposal(ctx, vaultPath, proposalID)
	if err != nil {
		return ApproveFacts{}, err
	}
	if propRow.Status != string(agentprotocol.ProposalStatusApprovalRequired) &&
		propRow.Status != string(agentprotocol.ProposalStatusConflictRequired) {
		return ApproveFacts{}, fmt.Errorf("proposal %s is not in approval-required state (status=%s)", proposalID, propRow.Status)
	}

	now := nowUTC()
	memoryID := newMemoryID(proposalID + propRow.Subject)
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            memoryID,
		Kind:          agentprotocol.MemoryKind(propRow.Kind),
		Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKind(propRow.ScopeKind), ID: propRow.ScopeID},
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       propRow.Subject,
		Summary:       propRow.Summary,
		Object:        propRow.Object,
		Confidence:    agentprotocol.ConfidenceHigh,
		CreatorID:     approver.PrincipalID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	// 从 proposal sources 读取
	propSources, _ := s.getProposalSources(ctx, st, proposalID)
	m.Sources = propSources

	if err := m.Validate(); err != nil {
		return ApproveFacts{}, err
	}
	if err := st.SaveMemory(ctx, m); err != nil {
		return ApproveFacts{}, err
	}

	// 更新 proposal status
	if err := st.UpdateProposalStatus(ctx, proposalID, agentprotocol.ProposalStatusApproved, agentprotocol.ReasonValid, memoryID); err != nil {
		return ApproveFacts{}, err
	}

	receiptID := "rcpt_" + proposalID
	return ApproveFacts{
		ProposalID:    proposalID,
		MemoryID:      memoryID,
		LifecycleFrom: agentprotocol.LifecycleProposed,
		LifecycleTo:   agentprotocol.LifecycleConfirmed,
		ReceiptID:     receiptID,
	}, nil
}

// AgentMemoryReject 拒绝一条 proposal。
// 与 approve 对齐：action 前重新验证 proposal 状态，已消费（approved/
// rejected/superseded）或缺失的 proposal 是 stale action，拒绝并要求 refresh。
func (s *AgentMemoryService) AgentMemoryReject(ctx context.Context, vaultPath, proposalID string, reviewer agentprotocol.Principal, reason string) error {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckReview(reviewer); err != nil {
		return err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return err
	}
	propRow, err := s.AgentMemoryShowProposal(ctx, vaultPath, proposalID)
	if err != nil {
		return err
	}
	switch agentprotocol.ProposalStatus(propRow.Status) {
	case agentprotocol.ProposalStatusApproved, agentprotocol.ProposalStatusRejected, agentprotocol.ProposalStatusSuperseded:
		// 终态 proposal：任何新 action 都是 stale，必须失败而非假成功。
		return fmt.Errorf("proposal %s is already consumed (status=%s); refresh the review inbox before acting", proposalID, propRow.Status)
	}
	return st.UpdateProposalStatus(ctx, proposalID, agentprotocol.ProposalStatusRejected, agentprotocol.ProposalStatusReason(reason), "")
}

// getProposalSources 读取 proposal 关联的 sources（从 proposal row 的 scope 推断或独立存储）。
// 当前实现：proposal 不独立存 source row，approve 时从 proposal 内容推断。
func (s *AgentMemoryService) getProposalSources(ctx context.Context, st *agentmemory.Store, proposalID string) (agentprotocol.SourceRefList, error) {
	// proposal sources 在当前模型中通过 proposal→memory 传递；
	// 如果需要更完整追踪，可在 AgentProposalRow 中增加 sources JSON 列。
	// 当前返回空列表，由调用方补充，或由 approve 逻辑从 prop 提取。
	return nil, nil
}
