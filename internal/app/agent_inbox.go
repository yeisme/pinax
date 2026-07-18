package app

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/memoryinbox"
)

// InboxRequest 描述一次 inbox 聚合请求。
type InboxRequest struct {
	VaultPath string
	Scope     agentprotocol.Scope
	Limit     int
}

// InboxItemDetailRequest 描述单个 inbox item 的查询条件。
type InboxItemDetailRequest struct {
	VaultPath string
	Scope     agentprotocol.Scope
	ItemID    string
}

// MemoryInbox 聚合 proposals 和 memories 投影为 inbox pack。
// 不创建第二套 canonical store。
func (s *AgentMemoryService) MemoryInbox(ctx context.Context, req InboxRequest) (memoryinbox.InboxPack, error) {
	ctx = ensureCtx(ctx)
	if err := req.Scope.Validate(); err != nil {
		return memoryinbox.InboxPack{}, err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return memoryinbox.InboxPack{}, err
	}

	// 收集 proposals（pending 状态的）
	proposals, err := st.ListProposals(ctx, req.Scope)
	if err != nil {
		return memoryinbox.InboxPack{}, fmt.Errorf("list proposals: %w", err)
	}

	// 收集 memories（用于 stale/expired 检测）
	memories, err := st.ListMemories(ctx, req.Scope)
	if err != nil {
		return memoryinbox.InboxPack{}, fmt.Errorf("list memories: %w", err)
	}

	agg := memoryinbox.NewAggregator()
	pack, err := agg.Aggregate(ctx, memoryinbox.AggregateInput{
		Proposals: proposals,
		Memories:  memories,
	}, req.Limit)
	if err != nil {
		return memoryinbox.InboxPack{}, err
	}

	return pack, nil
}

// InboxItemDetail 返回单个 inbox item 的详情（不含 body）。
func (s *AgentMemoryService) InboxItemDetail(ctx context.Context, req InboxItemDetailRequest) (memoryinbox.InboxItem, error) {
	pack, err := s.MemoryInbox(ctx, InboxRequest{VaultPath: req.VaultPath, Scope: req.Scope})
	if err != nil {
		return memoryinbox.InboxItem{}, err
	}
	for _, item := range pack.Items {
		if item.ItemID == req.ItemID {
			return item, nil
		}
	}
	return memoryinbox.InboxItem{}, fmt.Errorf("inbox item %s not found", req.ItemID)
}
