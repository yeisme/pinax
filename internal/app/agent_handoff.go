package app

import (
	"context"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentHandoffCreateRequest 描述一次 handoff create 的输入。
type AgentHandoffCreateRequest struct {
	VaultPath               string
	From                    agentprotocol.Principal
	To                      agentprotocol.Principal
	Scope                   agentprotocol.Scope
	Objective               string
	CurrentState            string
	Decisions               []string
	CompletedWork           []string
	Blockers                []string
	Verification            []string
	FollowUps               []string
	Sources                 agentprotocol.SourceRefList
	RequestedNextCapability string
}

// AgentHandoffCreate 创建一条 cross-agent handoff。
// Handoff 不自动 confirm；它只是 bounded working state。
func (s *AgentMemoryService) AgentHandoffCreate(ctx context.Context, req AgentHandoffCreateRequest) (string, error) {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckHandoff(req.From); err != nil {
		return "", err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return "", err
	}
	handoffID := newHandoffID(req.Objective + req.From.PrincipalID + req.To.PrincipalID)
	h := agentprotocol.Handoff{
		SchemaVersion:           agentprotocol.HandoffSchemaVersion,
		HandoffID:               handoffID,
		FromPrincipal:           req.From,
		ToPrincipal:             req.To,
		Scope:                   req.Scope,
		Objective:               req.Objective,
		CurrentState:            req.CurrentState,
		Decisions:               req.Decisions,
		CompletedWork:           req.CompletedWork,
		Blockers:                req.Blockers,
		Verification:            req.Verification,
		FollowUps:               req.FollowUps,
		Sources:                 req.Sources,
		RequestedNextCapability: req.RequestedNextCapability,
		CreatedAt:               nowUTC(),
	}
	if err := h.Validate(); err != nil {
		return "", err
	}
	if err := st.SaveHandoff(ctx, h); err != nil {
		return "", err
	}
	return handoffID, nil
}

// AgentHandoffList 列出指定 scope 的 handoffs。
func (s *AgentMemoryService) AgentHandoffList(ctx context.Context, vaultPath string, scope agentprotocol.Scope) ([]agentmemory.AgentHandoffRow, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.ListHandoffs(ctx, scope)
}
