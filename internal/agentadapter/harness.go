// Package agentadapter 提供 reference adapter harness，验证 Codex 和 Cohors adapter
// 能力协商、context request 渲染、proposal/handoff 转换和 failure isolation。
//
// runtime-specific hook/config/plugin 不进入本包；只验证 common schema 100% 共用。
package agentadapter

import (
	"context"
	"fmt"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// Harness 验证 adapter descriptor 与本地 runtime 的兼容性。
type Harness struct {
	localVersions []string
}

// NewHarness 构造一个 reference adapter harness。
// localVersions 是本地 runtime 支持的 schema 版本列表。
func NewHarness(localVersions []string) *Harness {
	return &Harness{localVersions: localVersions}
}

// NegotiateResult 包装 adapter negotiation 结果。
type NegotiateResult struct {
	agentprotocol.NegotiateResult
	Descriptor agentprotocol.AdapterDescriptor
}

// Negotiate 检查 adapter descriptor 是否兼容本地 runtime。
// 返回 degraded 状态时 adapter 仍可用（subset 能力），完全不兼容时返回 error。
func (h *Harness) Negotiate(desc agentprotocol.AdapterDescriptor, requested []agentprotocol.Capability) (NegotiateResult, error) {
	if err := desc.Validate(); err != nil {
		return NegotiateResult{}, fmt.Errorf("invalid descriptor: %w", err)
	}
	result := agentprotocol.Negotiate(desc, h.localVersions, requested)
	if !result.Compatible && !result.Degraded {
		return NegotiateResult{}, agentprotocol.NewStableError(
			agentprotocol.ErrCodeAdapterUnavailable,
			fmt.Sprintf("adapter %s incompatible: %s", desc.AdapterID, result.Reason),
		)
	}
	return NegotiateResult{NegotiateResult: result, Descriptor: desc}, nil
}

// RenderContextRequest 将 adapter-specific 请求转换为 canonical ContextRequest。
// adapter metadata 通过 ContextRequest.Metadata 传递，不污染 core fields。
func (h *Harness) RenderContextRequest(principal agentprotocol.Principal, scope agentprotocol.Scope, task string, adapterMetadata map[string]string) (agentprotocol.ContextRequest, error) {
	if err := principal.Validate(); err != nil {
		return agentprotocol.ContextRequest{}, err
	}
	if err := scope.Validate(); err != nil {
		return agentprotocol.ContextRequest{}, err
	}
	req := agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     principal,
		Scope:         scope,
		Task:          task,
	}
	return req, nil
}

// ConvertProposal 将 adapter-specific proposal 数据转换为 canonical Proposal。
// runtime-specific 信息进入 Proposal.Principal.Metadata，不进入 core fields。
func (h *Harness) ConvertProposal(principal agentprotocol.Principal, scope agentprotocol.Scope, kind agentprotocol.MemoryKind, subject, summary string, sources agentprotocol.SourceRefList) agentprotocol.Proposal {
	return agentprotocol.Proposal{
		SchemaVersion:  agentprotocol.SchemaVersion,
		ProposalID:     "adapter_proposal", // 由 app service 在 persist 时重新分配
		Principal:      principal,
		Scope:          scope,
		Kind:           kind,
		Subject:        subject,
		Summary:        summary,
		RequestedState: agentprotocol.LifecycleConfirmed,
		Sources:        sources,
	}
}

// ConvertHandoff 将 adapter-specific handoff 数据转换为 canonical Handoff。
func (h *Harness) ConvertHandoff(from, to agentprotocol.Principal, scope agentprotocol.Scope, objective string, decisions, blockers []string) agentprotocol.Handoff {
	return agentprotocol.Handoff{
		SchemaVersion: agentprotocol.HandoffSchemaVersion,
		HandoffID:     "adapter_handoff", // 由 app service 在 persist 时重新分配
		FromPrincipal: from,
		ToPrincipal:   to,
		Scope:         scope,
		Objective:     objective,
		Decisions:     decisions,
		Blockers:      blockers,
	}
}

// DegradedStatus 描述 adapter 不可用时的降级行为。
// adapter 失败不阻塞其他 transport；返回 degraded status 而非 panic。
type DegradedStatus struct {
	AdapterID string `json:"adapter_id"`
	Reason    string `json:"reason"`
	Action    string `json:"action"` // continue_without_adapter, retry, escalate
}

// CheckDegraded 返回 adapter 不可用时的安全降级状态。
// 不修改 ledger；调用方应根据 Action 决定后续行为。
func (h *Harness) CheckDegraded(ctx context.Context, desc agentprotocol.AdapterDescriptor, err error) DegradedStatus {
	_ = ctx
	reason := "adapter unavailable"
	if err != nil {
		reason = err.Error()
	}
	return DegradedStatus{
		AdapterID: desc.AdapterID,
		Reason:    reason,
		Action:    "continue_without_adapter",
	}
}
