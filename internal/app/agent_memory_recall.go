package app

import (
	"context"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// RecallQuery 是 CLI/API 用的 recall 查询构造器（避免 CLI 直接 import agentmemory）。
type RecallQuery struct {
	Scope  agentprotocol.Scope
	Text   string
	Kinds  []agentprotocol.MemoryKind
	States []agentprotocol.LifecycleState
	Limit  int
}

// AgentMemoryRecallQuery 通过 RecallQuery 执行 recall。
func (s *AgentMemoryService) AgentMemoryRecallQuery(ctx context.Context, vaultPath string, q RecallQuery) ([]agentprotocol.MemoryRecord, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.Recall(ctx, agentmemory.RecallQuery{
		Scope:  q.Scope,
		Text:   q.Text,
		Kinds:  q.Kinds,
		States: q.States,
		Limit:  q.Limit,
	})
}
