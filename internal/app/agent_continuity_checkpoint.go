package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// checkpoint 各 section 的 item/字符上限。
// 上限存在的根本原因：防止 checkpoint 退化为 transcript dump——
// 超限必须失败，而不是截断后静默写入可能含私密内容的手册。
const (
	checkpointMaxObjectiveChars  = 300
	checkpointMaxStateChars      = 500
	checkpointMaxSectionItems    = 12
	checkpointMaxSectionItemChar = 200
	checkpointMaxSources         = 10
	checkpointMaxDurableItems    = 5
	checkpointMaxDurableSubject  = 120
	checkpointMaxDurableSummary  = 300
)

// DurableCandidate 描述一个值得长期保留的 decision/preference/lesson。
// 它只能进入 proposal service（proposed 状态），绝不直接 confirmed。
type DurableCandidate struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Summary string `json:"summary"`
}

// ContinuityCheckpointRequest 描述一次 bounded checkpoint。
// RunID 可选关联 recorded run；本结构故意不接收 transcript 文件、
// raw prompt、provider payload 或自由 JSON dump（无对应字段）。
type ContinuityCheckpointRequest struct {
	VaultPath         string
	Principal         agentprotocol.Principal
	Scope             agentprotocol.Scope
	Objective         string
	CurrentState      string
	Decisions         []string
	CompletedWork     []string
	Blockers          []string
	Verification      []string
	FollowUps         []string
	Sources           agentprotocol.SourceRefList
	DurableCandidates []DurableCandidate
	ToRuntime         string
	RunID             string
}

// ContinuityCheckpointResult 是 checkpoint 的 bounded 结果。
type ContinuityCheckpointResult struct {
	HandoffID   string   `json:"handoff_id"`
	ProposalIDs []string `json:"proposal_ids,omitempty"`
	RunID       string   `json:"run_id,omitempty"`
	Scope       string   `json:"scope"`
	// ConfirmedCreated 恒为 0：checkpoint 只走 proposal/review。
	ConfirmedCreated int `json:"confirmed_created"`
}

// validateCheckpointCaps 在写入前校验 item/char caps。
// 任何超限返回 validation_failed（bounded error），不产生任何写入。
func validateCheckpointCaps(req ContinuityCheckpointRequest) error {
	if strings.TrimSpace(req.Objective) == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "objective is required")
	}
	if len(req.Objective) > checkpointMaxObjectiveChars {
		return capError("objective", checkpointMaxObjectiveChars)
	}
	if len(req.CurrentState) > checkpointMaxStateChars {
		return capError("current_state", checkpointMaxStateChars)
	}
	sections := []struct {
		name  string
		items []string
	}{
		{"decisions", req.Decisions},
		{"completed_work", req.CompletedWork},
		{"blockers", req.Blockers},
		{"verification", req.Verification},
		{"follow_ups", req.FollowUps},
	}
	for _, section := range sections {
		if len(section.items) > checkpointMaxSectionItems {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("%s exceeds item cap %d", section.name, checkpointMaxSectionItems))
		}
		for i, item := range section.items {
			if len(item) > checkpointMaxSectionItemChar {
				return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
					fmt.Sprintf("%s[%d] exceeds character cap %d", section.name, i, checkpointMaxSectionItemChar))
			}
		}
	}
	if len(req.Sources) > checkpointMaxSources {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
			fmt.Sprintf("sources exceeds cap %d", checkpointMaxSources))
	}
	if err := req.Sources.Validate(); err != nil {
		// Sources.Validate 会把 StableError 包一层；这里统一成 validation_failed，
		// 保证调用方拿到稳定错误码而不是 wrap 链。
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "sources invalid: "+err.Error())
	}
	if len(req.DurableCandidates) > checkpointMaxDurableItems {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
			fmt.Sprintf("durable candidates exceeds cap %d", checkpointMaxDurableItems))
	}
	for i, candidate := range req.DurableCandidates {
		kind := agentprotocol.MemoryKind(strings.TrimSpace(candidate.Kind))
		if !validCheckpointMemoryKind(kind) {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("durable_candidates[%d] has unknown kind %q", i, candidate.Kind))
		}
		if strings.TrimSpace(candidate.Subject) == "" {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("durable_candidates[%d] subject is required", i))
		}
		if strings.TrimSpace(candidate.Summary) == "" {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("durable_candidates[%d] summary is required", i))
		}
		if len(candidate.Subject) > checkpointMaxDurableSubject || len(candidate.Summary) > checkpointMaxDurableSummary {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("durable_candidates[%d] exceeds character caps", i))
		}
	}
	return nil
}

func validCheckpointMemoryKind(kind agentprotocol.MemoryKind) bool {
	switch kind {
	case agentprotocol.MemoryKindFact,
		agentprotocol.MemoryKindDecision,
		agentprotocol.MemoryKindPreference,
		agentprotocol.MemoryKindProcedure,
		agentprotocol.MemoryKindEvent,
		agentprotocol.MemoryKindTask,
		agentprotocol.MemoryKindFailure:
		return true
	default:
		return false
	}
}

func capError(field string, cap int) *agentprotocol.StableError {
	return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
		fmt.Sprintf("%s exceeds character cap %d", field, cap))
}

// ContinuityCheckpoint 创建绑定感知的 bounded handoff。
//
// 状态机与补偿边界（中文注释）：
//  1. 写入前完成全部 caps/scope/principal 校验；超限即整体失败，零写入。
//  2. handoff 通过 canonical AgentHandoffCreate 创建，是 durable 记录。
//  3. durable candidates 逐个走 canonical proposal service，只产生 proposed
//     memory。若某 proposal 失败（如重复/权限），checkpoint 不回滚已创建的
//     handoff（handoff 本身是合法记录），而是返回 stable error 并在 details
//     中携带 handoff_id，调用方可针对失败 candidate 单独重试。
//  4. confirmed memory count 不变：本函数没有任何 confirm/approve 路径。
func (s *AgentMemoryService) ContinuityCheckpoint(ctx context.Context, req ContinuityCheckpointRequest) (ContinuityCheckpointResult, error) {
	ctx = ensureCtx(ctx)
	if err := req.Principal.Validate(); err != nil {
		return ContinuityCheckpointResult{}, fmt.Errorf("principal: %w", err)
	}
	if err := req.Scope.Validate(); err != nil {
		return ContinuityCheckpointResult{}, fmt.Errorf("scope: %w", err)
	}
	if err := validateCheckpointCaps(req); err != nil {
		return ContinuityCheckpointResult{}, err
	}
	linkStore, err := s.continuityCheckpointLinkStore(ctx, req.VaultPath, req.Scope, req.RunID)
	if err != nil {
		return ContinuityCheckpointResult{}, err
	}

	toRuntime := req.ToRuntime
	if toRuntime == "" {
		toRuntime = "target-agent"
	}
	handoffID, err := s.AgentHandoffCreate(ctx, AgentHandoffCreateRequest{
		VaultPath:     req.VaultPath,
		From:          req.Principal,
		To:            agentprotocol.DefaultAdapterPrincipal("checkpoint-target", toRuntime),
		Scope:         req.Scope,
		Objective:     req.Objective,
		CurrentState:  req.CurrentState,
		Decisions:     req.Decisions,
		CompletedWork: req.CompletedWork,
		Blockers:      req.Blockers,
		Verification:  req.Verification,
		FollowUps:     req.FollowUps,
		Sources:       req.Sources,
	})
	if err != nil {
		return ContinuityCheckpointResult{}, err
	}

	result := ContinuityCheckpointResult{
		HandoffID:        handoffID,
		RunID:            req.RunID,
		Scope:            fmt.Sprintf("%s:%s", req.Scope.Kind, req.Scope.ID),
		ConfirmedCreated: 0,
	}
	for _, candidate := range req.DurableCandidates {
		facts, _, proposeErr := s.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
			VaultPath: req.VaultPath,
			Principal: req.Principal,
			Scope:     req.Scope,
			Kind:      agentprotocol.MemoryKind(candidate.Kind),
			Subject:   candidate.Subject,
			Summary:   candidate.Summary,
		})
		if proposeErr != nil {
			return result, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				fmt.Sprintf("durable candidate proposal failed: %s", proposeErr.Error())).
				WithDetail("handoff_id", handoffID)
		}
		result.ProposalIDs = append(result.ProposalIDs, facts.ProposalID)
	}
	if linkStore != nil {
		if err := linkStore.MarkCheckpoint(ctx, req.RunID, nowUTC(), len(result.ProposalIDs)); err != nil {
			return result, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"checkpoint was saved but the run link could not be recorded").
				WithDetail("handoff_id", handoffID).
				WithDetail("continuity_run_id", req.RunID)
		}
	}
	return result, nil
}
