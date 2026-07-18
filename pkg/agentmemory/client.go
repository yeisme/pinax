// Package agentmemory 提供面向 embedded Go consumer 的 Agent memory runtime facade。
//
// 这是 experimental public API（schema v1），让消费方在不直接依赖 internal store 的情况下
// 编译 context、recall memory、提交 proposal、创建 handoff 和记录 feedback。
// 所有写操作默认走 proposal → review → approve 流程；不暴露 internal store 细节。
package agentmemory

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/app"
)

// Experimental 标记当前 SDK 为实验性 API。
const Experimental = "agent memory SDK is experimental (v1); API may evolve before stable-from"

// Client 是 embedded consumer 使用的 Agent memory facade。
// 它包装 application service，不暴露 internal store。
type Client struct {
	svc *app.AgentMemoryService
}

// NewClient 构造一个新 Client。
func NewClient() *Client {
	return &Client{svc: app.NewAgentMemoryService()}
}

// Close 释放底层资源。
func (c *Client) Close() error {
	return c.svc.Close()
}

// CompileContext 编译 bounded context pack。
func (c *Client) CompileContext(ctx context.Context, vaultPath string, principal agentprotocol.Principal, scope agentprotocol.Scope, entities []string, maxItems, maxChars int) (agentprotocol.ContextPack, error) {
	return c.svc.AgentContextRuntime(ctx, app.AgentContextRequest{
		VaultPath: vaultPath,
		Principal: principal,
		Scope:     scope,
		Entities:  entities,
		MaxItems:  maxItems,
		MaxChars:  maxChars,
	})
}

// Recall 检索 bounded memories。
func (c *Client) Recall(ctx context.Context, vaultPath string, scope agentprotocol.Scope, kinds []agentprotocol.MemoryKind) ([]agentprotocol.MemoryRecord, error) {
	return c.svc.AgentMemoryRecallQuery(ctx, vaultPath, app.RecallQuery{Scope: scope, Kinds: kinds})
}

// Propose 提交一条 memory proposal。
// Agent 默认只能 propose；返回 review 结论。
func (c *Client) Propose(ctx context.Context, vaultPath string, principal agentprotocol.Principal, scope agentprotocol.Scope, kind agentprotocol.MemoryKind, subject, summary string, sources agentprotocol.SourceRefList) (app.AgentMemoryFacts, *agentprotocol.ProposalReview, error) {
	return c.svc.AgentMemoryPropose(ctx, app.AgentMemoryProposeRequest{
		VaultPath: vaultPath,
		Principal: principal,
		Scope:     scope,
		Kind:      kind,
		Subject:   subject,
		Summary:   summary,
		Sources:   sources,
	})
}

// Approve 批准一条 proposal（需要 owner principal）。
func (c *Client) Approve(ctx context.Context, vaultPath, proposalID string, approver agentprotocol.Principal) (app.ApproveFacts, error) {
	return c.svc.AgentMemoryApprove(ctx, vaultPath, proposalID, approver)
}

// CreateHandoff 创建一条 cross-agent handoff。
func (c *Client) CreateHandoff(ctx context.Context, req HandoffRequest) (string, error) {
	return c.svc.AgentHandoffCreate(ctx, app.AgentHandoffCreateRequest{
		VaultPath: req.VaultPath,
		From:      req.From,
		To:        req.To,
		Scope:     req.Scope,
		Objective: req.Objective,
		Decisions: req.Decisions,
		Blockers:  req.Blockers,
	})
}

// AddFeedback 记录一条 recall feedback。
func (c *Client) AddFeedback(ctx context.Context, vaultPath string, principal agentprotocol.Principal, scope agentprotocol.Scope, kind agentprotocol.FeedbackKind, memoryID, comment string) (string, error) {
	return c.svc.AgentFeedbackAdd(ctx, app.AgentFeedbackAddRequest{
		VaultPath: vaultPath,
		Principal: principal,
		Scope:     scope,
		Kind:      kind,
		MemoryID:  memoryID,
		Comment:   comment,
	})
}

// HandoffRequest 是 CreateHandoff 的输入。
type HandoffRequest struct {
	VaultPath string
	From      agentprotocol.Principal
	To        agentprotocol.Principal
	Scope     agentprotocol.Scope
	Objective string
	Decisions []string
	Blockers  []string
}

// Version 返回 SDK schema 版本。
func (c *Client) Version() string {
	return fmt.Sprintf("%s (experimental)", agentprotocol.SchemaVersion)
}
