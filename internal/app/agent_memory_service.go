package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentMemoryService 是 Agent memory runtime 的 application service 入口。
// 它编排 store、lifecycle 和 policy，为 CLI/MCP/API/SDK 提供统一用例。
// Agent 默认只能 propose；confirmed mutation 受 policy 和 receipt 控制。
type AgentMemoryService struct {
	mu     sync.Mutex
	stores map[string]*agentmemory.Store // keyed by vault root
	policy agentmemory.Policy
}

// NewAgentMemoryService 构造 application service。
func NewAgentMemoryService() *AgentMemoryService {
	return &AgentMemoryService{
		stores: make(map[string]*agentmemory.Store),
		policy: agentmemory.DefaultPolicy(),
	}
}

// storeFor 返回（或按需打开）指定 vault root 的 agent memory store。
func (s *AgentMemoryService) storeFor(root string) (*agentmemory.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root: %w", err)
	}
	if st, ok := s.stores[abs]; ok {
		return st, nil
	}
	st, err := agentmemory.Open(abs)
	if err != nil {
		return nil, err
	}
	s.stores[abs] = st
	return st, nil
}

// Close 关闭所有打开的 store。
func (s *AgentMemoryService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for _, st := range s.stores {
		if err := st.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.stores = make(map[string]*agentmemory.Store)
	return firstErr
}

func newMemoryID(seed string) string {
	h := sha1.Sum([]byte(seed + time.Now().UTC().Format(time.RFC3339Nano)))
	return "mem_" + hex.EncodeToString(h[:12])
}

func newProposalID(seed string) string {
	h := sha1.Sum([]byte(seed + time.Now().UTC().Format(time.RFC3339Nano)))
	return "prop_" + hex.EncodeToString(h[:12])
}

func newHandoffID(seed string) string {
	h := sha1.Sum([]byte(seed + time.Now().UTC().Format(time.RFC3339Nano)))
	return "h_" + hex.EncodeToString(h[:12])
}

func newFeedbackID(seed string) string {
	h := sha1.Sum([]byte(seed + time.Now().UTC().Format(time.RFC3339Nano)))
	return "fb_" + hex.EncodeToString(h[:12])
}

// nowUTC 返回当前 UTC 时间。
func nowUTC() time.Time { return time.Now().UTC() }

// AgentMemoryFacts 是 projection facts 的结构化提取，便于 CLI/MCP/API 共用。
type AgentMemoryFacts struct {
	MemoryID    string                             `json:"memory_id,omitempty"`
	ProposalID  string                             `json:"proposal_id,omitempty"`
	Status      agentprotocol.ProposalStatus       `json:"status,omitempty"`
	Reason      agentprotocol.ProposalStatusReason `json:"reason,omitempty"`
	Conflicts   []string                           `json:"conflicts,omitempty"`
	DuplicateID string                             `json:"duplicate_id,omitempty"`
	State       agentprotocol.LifecycleState       `json:"state,omitempty"`
	Count       int                                `json:"count,omitempty"`
}

// ApproveFacts 描述一次 approve 操作的结果。
type ApproveFacts struct {
	ProposalID    string                       `json:"proposal_id"`
	MemoryID      string                       `json:"memory_id"`
	LifecycleFrom agentprotocol.LifecycleState `json:"lifecycle_from"`
	LifecycleTo   agentprotocol.LifecycleState `json:"lifecycle_to"`
	ReceiptID     string                       `json:"receipt_id"`
}

// closeStore 是测试 helper，用于关闭临时 store。
func (s *AgentMemoryService) closeStore(root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if st, ok := s.stores[abs]; ok {
		delete(s.stores, abs)
		return st.Close()
	}
	return nil
}

// ensure context always has a value
func ensureCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
