package app

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentContextRequest 描述一次 context compilation 的输入。
type AgentContextRequest struct {
	VaultPath  string
	Principal  agentprotocol.Principal
	Scope      agentprotocol.Scope
	Task       string
	Intent     string
	Entities   []string
	KindFilter []agentprotocol.MemoryKind
	MaxItems   int
	MaxChars   int
}

// AgentContextRuntime 编译 context request → projection。
// 复用 agentcontext.Compiler，不直接访问 transport。
// 输出 pinax.agent_context_pack.v1。
func (s *AgentMemoryService) AgentContextRuntime(ctx context.Context, req AgentContextRequest) (agentprotocol.ContextPack, error) {
	ctx = ensureCtx(ctx)
	if err := req.Principal.Validate(); err != nil {
		return agentprotocol.ContextPack{}, err
	}
	if err := req.Scope.Validate(); err != nil {
		return agentprotocol.ContextPack{}, err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}

	compiler := agentcontext.NewCompiler(st, s.policy)
	pack, err := compiler.Compile(ctx, agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		Intent:        req.Intent,
		Entities:      req.Entities,
		KindFilter:    req.KindFilter,
		Budget: agentprotocol.ContextBudget{
			MaxItems: req.MaxItems,
			MaxChars: req.MaxChars,
		},
	})
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}

	// 防御性检查：确保 pack 不泄漏完整 body
	if err := pack.AssertNoBody(500); err != nil {
		return agentprotocol.ContextPack{}, fmt.Errorf("context pack body safety check failed: %w", err)
	}

	return pack, nil
}

// AgentContextRuntimeWithLegacy 合并新 agent memory 和旧 memory rows 后编译 context。
// 旧 records 通过 legacy mapping 读取，不静默 backfill。
func (s *AgentMemoryService) AgentContextRuntimeWithLegacy(ctx context.Context, req AgentContextRequest, legacyRecords []agentmemory.LegacyRecord, workspaceID string) (agentprotocol.ContextPack, error) {
	ctx = ensureCtx(ctx)
	// 先取新 store 的 memories
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}
	newMems, err := st.Recall(ctx, agentmemory.RecallQuery{
		Scope: req.Scope,
		Kinds: req.KindFilter,
	})
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}

	// 映射旧 records
	legacyMems, err := agentmemory.EnsureLegacyViewed(ctx, st, legacyRecords, workspaceID)
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}

	// 合并：只取 recallable legacy records
	allMems := make([]agentprotocol.MemoryRecord, 0, len(newMems)+len(legacyMems))
	allMems = append(allMems, newMems...)
	for _, lm := range legacyMems {
		if lm.IsRecallable() {
			allMems = append(allMems, lm)
		}
	}

	// 直接编译合并后的 list（bypass recall，因为已取）
	compiler := agentcontext.NewCompiler(st, s.policy)
	pack, err := compiler.CompileMemories(ctx, agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		Entities:      req.Entities,
		KindFilter:    req.KindFilter,
		Budget: agentprotocol.ContextBudget{
			MaxItems: req.MaxItems,
			MaxChars: req.MaxChars,
		},
	}, allMems)
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}
	return pack, nil
}
