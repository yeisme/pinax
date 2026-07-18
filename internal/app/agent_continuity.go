package app

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// ContinuityRequest 描述一次 continuity compilation 的输入。
type ContinuityRequest struct {
	VaultPath  string
	Principal  agentprotocol.Principal
	Scope      agentprotocol.Scope
	Task       string
	Intent     string
	MaxItems   int
	MaxChars   int
	MaxSources int
	HandoffID  string
}

// AgentContinuity 编译 bounded continuity pack。
// 复用 agentcontinuity.Orchestrator，不直接访问 transport。
func (s *AgentMemoryService) AgentContinuity(ctx context.Context, req ContinuityRequest) (agentcontinuity.ContinuityPack, error) {
	ctx = ensureCtx(ctx)
	if err := req.Principal.Validate(); err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}
	if err := req.Scope.Validate(); err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}

	budget := agentcontinuity.DefaultBudget()
	if req.MaxItems > 0 {
		budget.MaxItems = req.MaxItems
	}
	if req.MaxChars > 0 {
		budget.MaxChars = req.MaxChars
	}
	if req.MaxSources > 0 {
		budget.MaxSources = req.MaxSources
	}

	compiler := agentcontext.NewCompiler(st, s.policy)
	orch := agentcontinuity.NewOrchestrator(st, compiler)
	pack, err := orch.Compile(ctx, agentcontinuity.ContinuityRequest{
		SchemaVersion: agentcontinuity.ContinuitySchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		Intent:        req.Intent,
		Budget:        budget,
		HandoffID:     req.HandoffID,
	})
	if err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}

	// 防御性检查：确保 pack 不泄漏完整 body
	if err := pack.AssertNoBody(500); err != nil {
		return agentcontinuity.ContinuityPack{}, fmt.Errorf("continuity pack body safety check failed: %w", err)
	}

	return pack, nil
}
