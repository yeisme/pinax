package app

import (
	"context"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// AgentFeedbackAddRequest 描述一次 feedback add 的输入。
type AgentFeedbackAddRequest struct {
	VaultPath  string
	Principal  agentprotocol.Principal
	Scope      agentprotocol.Scope
	Kind       agentprotocol.FeedbackKind
	MemoryID   string
	ContextRef string
	Comment    string
}

// AgentFeedbackAdd 记录一条 recall feedback。
// Feedback 不静默改写 memory content。
func (s *AgentMemoryService) AgentFeedbackAdd(ctx context.Context, req AgentFeedbackAddRequest) (string, error) {
	ctx = ensureCtx(ctx)
	if err := s.policy.CheckFeedback(req.Principal); err != nil {
		return "", err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return "", err
	}
	feedbackID := newFeedbackID(string(req.Kind) + req.Principal.PrincipalID + req.MemoryID)
	f := agentprotocol.Feedback{
		SchemaVersion:     agentprotocol.SchemaVersion,
		FeedbackID:        feedbackID,
		Principal:         req.Principal,
		Scope:             req.Scope,
		Kind:              req.Kind,
		MemoryID:          req.MemoryID,
		ContextRequestRef: req.ContextRef,
		Comment:           req.Comment,
		CreatedAt:         nowUTC(),
	}
	if err := f.Validate(); err != nil {
		return "", err
	}
	if err := st.SaveFeedback(ctx, f); err != nil {
		return "", err
	}
	return feedbackID, nil
}

// AgentFeedbackList 列出指定 scope 的 feedback。
func (s *AgentMemoryService) AgentFeedbackList(ctx context.Context, vaultPath string, scope agentprotocol.Scope) ([]agentmemory.AgentFeedbackRow, error) {
	ctx = ensureCtx(ctx)
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return st.ListFeedback(ctx, scope)
}
