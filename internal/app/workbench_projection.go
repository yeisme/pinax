package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/continuitybinding"
	"github.com/yeisme/pinax/internal/domain"
)

// workbench_projection.go 实现 Workbench continuity typed projection facade
// （pinax-workbench-continuity-projection-v1，合同见该 change design.md）。
//
// Workbench BFF 只持 opaque projectRef（= binding_id）；本 facade 做 exact-by-id
// 解析、binding 状态诊断透传、ContinuityPack 的有界 resume card 投影与
// evidence-observed 时效派生。缺项/歧义一律 fail-closed 返回稳定错误码与唯一
// 恢复 action，不做跨 vault 搜索、目录名匹配或最近使用猜测。

// WorkbenchProjectionActions / WorkbenchProjectionErrors 是 packet 与 envelope
// digest 的固定合同域（canonical、无时间戳）。
var (
	WorkbenchProjectionActions = []string{
		"resume_card.read",
		"binding.status.read",
		"checkpoint.propose",
	}
	WorkbenchProjectionErrors = []string{
		"binding_not_found",
		"binding_disabled",
		"binding_invalid",
		"validation_failed",
	}
)

type WorkbenchProjectionRequest struct {
	ProjectRef string
	ConfigDir  string
	TTLSeconds int
	Now        time.Time
}

type WorkbenchProjectionResult struct {
	Projection domain.WorkbenchContinuityProjection
}

// WorkbenchContract 返回 envelope/packet 共用合同标识。
func WorkbenchContract() domain.WorkbenchProjectionContract {
	return domain.WorkbenchProjectionContract{
		Identity: domain.WorkbenchContinuityContractIdentity,
		Version:  domain.WorkbenchContinuityContractVersion,
		Digest:   domain.ComputeWorkbenchContractDigest(WorkbenchProjectionActions, WorkbenchProjectionErrors),
	}
}

// WorkbenchProviderPacket 返回根仓消费的紧凑 provider packet（静态合同描述）。
func WorkbenchProviderPacket() domain.WorkbenchProviderPacket {
	return domain.WorkbenchProviderPacket{
		SchemaVersion: domain.WorkbenchProviderPacketSchemaVersion,
		Contract:      WorkbenchContract(),
		Owner:         "cli/pinax",
		Availability: domain.WorkbenchPacketAvailability{
			Mode:  "local-cli",
			Entry: "pinax continue workbench <projectRef> --json",
		},
		Actions: []domain.WorkbenchPacketAction{
			{Name: "resume_card.read", Effect: "read_only", Entry: "pinax continue workbench <projectRef> --json"},
			{Name: "binding.status.read", Effect: "read_only", Entry: "pinax continue workbench <projectRef> --json"},
			{Name: "checkpoint.propose", Effect: "durable_candidate_only", Requires: "--yes confirmation",
				Entry: "pinax continue checkpoint", Receipt: "handoff id + proposal ids"},
		},
		ScopeRevision: "binding registry schema pinax.continuity_binding.v1",
		Errors:        append([]string(nil), WorkbenchProjectionErrors...),
		Recovery:      domain.WorkbenchPacketRecovery{Policy: "one stable action per error code, see envelope.recovery"},
		EvidenceRefs:  []string{"openspec/changes/pinax-workbench-continuity-projection-v1/"},
	}
}

// WorkbenchContinuityProjection 按 opaque projectRef 组装 typed facade envelope。
// 查询全程 read-only；错误态返回投影（含 recovery）而非裸错误。
func (s *AgentMemoryService) WorkbenchContinuityProjection(ctx context.Context, req WorkbenchProjectionRequest) (domain.WorkbenchContinuityProjection, error) {
	ctx = ensureCtx(ctx)
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	projectRef := strings.TrimSpace(req.ProjectRef)
	if projectRef == "" {
		err := &domain.CommandError{Code: "validation_failed", Message: "workbench projection requires a projectRef", Hint: "pass the binding_id as projectRef"}
		return domain.WorkbenchContinuityProjection{}, err
	}

	envelope := domain.WorkbenchContinuityProjection{
		SchemaVersion: domain.WorkbenchContinuityProjectionSchemaVersion,
		Contract:      WorkbenchContract(),
		ProjectRef:    projectRef,
	}

	configDir := continuityConfigDir(req.ConfigDir)
	registry, err := continuitybinding.LoadRegistry(configDir)
	if err != nil {
		return domain.WorkbenchContinuityProjection{}, err
	}

	// exact-by-id：id 唯一，不存在歧义分支；不做任何路径/scope 猜测。
	var binding *continuitybinding.Binding
	for index := range registry.Bindings {
		if registry.Bindings[index].BindingID == projectRef {
			binding = &registry.Bindings[index]
			break
		}
	}
	if binding == nil {
		envelope.Binding = domain.WorkbenchProjectionBinding{Status: "not_found", Ready: false}
		envelope.Freshness = domain.WorkbenchFreshnessFromEvidence(time.Time{}, req.TTLSeconds, now)
		envelope.Recovery = &domain.WorkbenchProjectionRecovery{
			Code:   "binding_not_found",
			Action: "pinax continue bind --vault <vault-alias> --scope <kind>:<id>",
		}
		return envelope, nil
	}

	status, err := s.ContinuityBindingStatus(ctx, ContinuityBindingStatusRequest{RepoPath: binding.CanonicalRepoRoot, ConfigDir: req.ConfigDir})
	if err != nil {
		return domain.WorkbenchContinuityProjection{}, err
	}
	envelope.Binding = domain.WorkbenchProjectionBinding{
		Status:        string(status.BindingStatus),
		Ready:         status.Ready,
		VaultRef:      status.VaultRef,
		Scope:         status.Scope,
		VaultResolved: status.VaultResolved,
		ScopeValid:    status.ScopeValid,
	}

	if !status.Ready {
		envelope.Freshness = domain.WorkbenchFreshnessFromEvidence(time.Time{}, req.TTLSeconds, now)
		code, action := workbenchBindingRecovery(string(status.BindingStatus), projectRef)
		envelope.Recovery = &domain.WorkbenchProjectionRecovery{Code: code, Action: action}
		return envelope, nil
	}

	resolved, err := s.ContinuityResolveBinding(ctx, ContinuityResolveRequest{RepoPath: binding.CanonicalRepoRoot, ConfigDir: req.ConfigDir})
	if err != nil {
		return domain.WorkbenchContinuityProjection{}, err
	}
	pack, err := s.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: resolved.VaultPath,
		Principal: agentprotocol.DefaultAdapterPrincipal("local-cli", "pinax-cli"),
		Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKind(binding.ScopeKind), ID: binding.ScopeID},
		RepoRoot:  binding.CanonicalRepoRoot,
	})
	if err != nil {
		return domain.WorkbenchContinuityProjection{}, err
	}
	envelope.ResumeCard = workbenchResumeCardPtr(pack)
	envelope.Freshness = domain.WorkbenchFreshnessFromEvidence(pack.Freshness, req.TTLSeconds, now)
	return envelope, nil
}

// workbenchBindingRecovery 给出每个非 ready 状态的稳定错误码与唯一恢复 action。
func workbenchBindingRecovery(status, projectRef string) (string, string) {
	switch status {
	case string(continuitybinding.StatusDisabled):
		return "binding_disabled", "pinax continue binding enable " + projectRef
	case string(continuitybinding.StatusInvalid):
		return "binding_invalid", fmt.Sprintf("pinax continue binding disable %s && pinax continue bind --vault <vault-alias> --scope <kind>:<id>", projectRef)
	default:
		return "binding_not_found", "pinax continue bind --vault <vault-alias> --scope <kind>:<id>"
	}
}

// workbenchResumeCard 把 ContinuityPack 投影为冻结的显式选字段卡（sections 计数化，
// 不透传未来 pack 字段）。
func workbenchResumeCardPtr(pack agentcontinuity.ContinuityPack) *domain.WorkbenchResumeCard {
	sections := make([]string, 0, len(pack.Sections))
	for _, section := range pack.Sections {
		sections = append(sections, fmt.Sprintf("%s:%d", section.Kind, len(section.Items)))
	}
	sort.Strings(sections)
	handoff := "none"
	if pack.HandoffID != "" {
		handoff = pack.HandoffID
	} else if pack.HandoffStatus != "" {
		handoff = string(pack.HandoffStatus)
	}
	return &domain.WorkbenchResumeCard{
		Objective:    pack.Objective,
		CurrentState: pack.CurrentState,
		Task:         pack.Task,
		Sections:     sections,
		Sources:      fmt.Sprintf("resolved:%d/%d", pack.SourceCoverage.Resolved, pack.SourceCoverage.Total),
		Handoff:      handoff,
		Conflicts:    len(pack.Conflicts),
	}
}
