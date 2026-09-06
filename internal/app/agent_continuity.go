package app

import (
	"context"
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/memoryinbox"
)

// ContinuityRequest 描述一次 continuity compilation 的输入。
type ContinuityRequest struct {
	VaultPath  string
	Principal  agentprotocol.Principal
	Scope      agentprotocol.Scope
	Task       string
	Intent     string
	MaxItems   int
	MaxChars   int
	MaxSources int
	HandoffID  string
	// RepoRoot 是当前 binding 解析到的 canonical repository root（additive）。
	// 为空时 repository source 计为 missing；绝不允许访问绑定之外的路径。
	RepoRoot string
}

// AgentContinuity 编译 bounded continuity pack。
// 复用 agentcontinuity.Orchestrator，不直接访问 transport。
func (s *AgentMemoryService) AgentContinuity(ctx context.Context, req ContinuityRequest) (agentcontinuity.ContinuityPack, error) {
	ctx = ensureCtx(ctx)
	if err := req.Principal.Validate(); err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}
	if err := req.Scope.Validate(); err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}
	st, err := s.storeFor(req.VaultPath)
	if err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}

	budget := agentcontinuity.DefaultBudget()
	if req.MaxItems > 0 {
		budget.MaxItems = req.MaxItems
	}
	if req.MaxChars > 0 {
		budget.MaxChars = req.MaxChars
	}
	if req.MaxSources > 0 {
		budget.MaxSources = req.MaxSources
	}

	compiler := agentcontext.NewCompiler(st, s.policy)
	orch := agentcontinuity.NewOrchestrator(st, compiler)
	pack, err := orch.Compile(ctx, agentcontinuity.ContinuityRequest{
		SchemaVersion: agentcontinuity.ContinuitySchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		Intent:        req.Intent,
		Budget:        budget,
		HandoffID:     req.HandoffID,
	})
	if err != nil {
		return agentcontinuity.ContinuityPack{}, err
	}

	// 防御性检查：确保 pack 不泄漏完整 body
	if err := pack.AssertNoBody(500); err != nil {
		return agentcontinuity.ContinuityPack{}, fmt.Errorf("continuity pack body safety check failed: %w", err)
	}
	coverageResult := resolveContinuitySourceCoverage(ctx, req.VaultPath, pack.Sources, req.RepoRoot)
	pack.SourceCoverage = coverageResult.Coverage
	// repository evidence 的 observed time 参与 freshness：比 handoff 更新则抬高。
	if coverageResult.LatestObserved.After(pack.Freshness) {
		pack.Freshness = coverageResult.LatestObserved
	}

	// review attention（6.1）：只有会改变 objective/decision/blocker/
	// conflict/next action 的 pending item 才进入 inline 提示；其余留在
	// weekly review inbox。不影响 pack 的可信 section。
	attachReviewAttention(ctx, s, req, &pack)

	pack.RefreshDerived()
	return pack, nil
}

// continuitySourceResolution 是 coverage 解析结果（含最新 evidence 时间）。
type continuitySourceResolution struct {
	Coverage       agentcontinuity.SourceCoverage
	LatestObserved time.Time
}

func resolveContinuitySourceCoverage(ctx context.Context, vaultPath string, sources agentprotocol.SourceRefList, repoRoot string) continuitySourceResolution {
	coverage := agentcontinuity.SourceCoverage{Total: len(sources)}
	resolver := NewService()
	result := continuitySourceResolution{Coverage: coverage}
	for _, source := range sources {
		switch source.Kind {
		case agentprotocol.SourceKindNote, agentprotocol.SourceKindAsset:
			resolved, err := resolver.ResolveVaultObject(ctx, ResolverRequest{
				VaultPath: vaultPath,
				Query:     source.Ref,
				Scope:     "registered",
				Kind:      source.Kind,
			})
			if err != nil || resolved.Facts.Ambiguous || len(resolved.Candidates) != 1 {
				result.Coverage.Missing++
				continue
			}
			result.Coverage.Resolved++
		case agentprotocol.SourceKindRepository:
			// repository source 只允许在 binding canonical root 内解析；
			// 未绑定时计为 missing，不允许访问任意路径。
			if repoRoot == "" {
				result.Coverage.Missing++
				continue
			}
			resolution, err := ResolveRepositorySource(ctx, repoRoot, source)
			if err != nil {
				result.Coverage.Missing++
				continue
			}
			if resolution.ObservedAt.After(result.LatestObserved) {
				result.LatestObserved = resolution.ObservedAt
			}
			switch resolution.Status {
			case repoSourceResolved:
				result.Coverage.Resolved++
			case repoSourceStale:
				result.Coverage.Stale++
			case repoSourceAmbiguous:
				result.Coverage.Ambiguous++
			default:
				result.Coverage.Missing++
			}
		default:
			// unknown kind 按 unresolved 兼容（旧消费者不崩溃）。
			result.Coverage.Missing++
		}
	}
	return result
}

// attachReviewAttention 聚合当前 scope 的 inbox 并投影唯一 inline 提示。
// 失败（如 vault 无 inbox 状态）不阻塞 continuity 主路径，保持 partial 语义。
func attachReviewAttention(ctx context.Context, s *AgentMemoryService, req ContinuityRequest, pack *agentcontinuity.ContinuityPack) {
	inbox, err := s.MemoryInbox(ctx, InboxRequest{
		VaultPath: req.VaultPath,
		Scope:     req.Scope,
		Limit:     100,
	})
	if err != nil {
		// 不阻塞主路径，但 0 条 attention 此时是"未测量"：
		// 打 flag 让 RefreshDerived 写入 review_attention_unavailable。
		pack.ReviewAttentionUnavailable = true
		return
	}
	relevant := memoryinbox.RelevanceProjection(inbox.Items, pack.Objective, pack.CurrentState)
	if len(relevant) == 0 {
		return
	}
	pack.ReviewAttentionCount = len(relevant)
	first := relevant[0]
	pack.ReviewAttention = &agentcontinuity.ReviewAttention{
		ItemID:      first.Item.ItemID,
		Subject:     first.Item.Subject,
		ReasonCodes: first.ReasonCodes,
		Risk:        string(first.Item.Risk),
	}
}
