package app

import (
	"context"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentMemoryRecall 检索 memory records（read-only，不写 receipt）。
func (s *AgentMemoryService) AgentMemoryRecall(ctx context.Context, vaultPath string, query agentmemory.RecallQuery) ([]agentprotocol.MemoryRecord, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.Recall(ctx, query)
}

// AgentMemorySupersede 将 old memory 标记为 superseded，由 replacement 接替。
func (s *AgentMemoryService) AgentMemorySupersede(ctx context.Context, vaultPath, oldID, replacementID string, principal agentprotocol.Principal) error {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckConfirm(principal); err != nil {
		return err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return err
	}
	ls := agentmemory.NewLifecycleService(st)
	return ls.Supersede(ctx, oldID, replacementID, nowUTC())
}

// AgentMemoryExpire 标记 memory 为 expired。
func (s *AgentMemoryService) AgentMemoryExpire(ctx context.Context, vaultPath, memoryID string, principal agentprotocol.Principal) error {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckConfirm(principal); err != nil {
		return err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return err
	}
	ls := agentmemory.NewLifecycleService(st)
	return ls.Expire(ctx, memoryID, nowUTC())
}

// AgentMemoryResolveConflict 解决冲突，选择 winner，其余标记为 superseded。
func (s *AgentMemoryService) AgentMemoryResolveConflict(ctx context.Context, vaultPath, winnerID string, principal agentprotocol.Principal) error {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckConfirm(principal); err != nil {
		return err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return err
	}
	ls := agentmemory.NewLifecycleService(st)
	return ls.ResolveConflict(ctx, winnerID, nowUTC())
}

// AgentMemoryRollback 回滚一条 superseded memory 为 confirmed。
func (s *AgentMemoryService) AgentMemoryRollback(ctx context.Context, vaultPath, supersededID string, principal agentprotocol.Principal) error {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckConfirm(principal); err != nil {
		return err
	}
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return err
	}
	ls := agentmemory.NewLifecycleService(st)
	return ls.RollbackSupersede(ctx, supersededID, nowUTC())
}

// AgentMemoryStatus 返回某 scope 的 memory 计数摘要。
func (s *AgentMemoryService) AgentMemoryStatus(ctx context.Context, vaultPath string, scope agentprotocol.Scope) (map[agentprotocol.LifecycleState]int, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.CountByScope(ctx, scope)
}
