// Package agentcontext 实现 permission-first bounded context compilation。
//
// Compiler 分阶段执行：
// 1. capability/permission 过滤
// 2. scope 继承
// 3. lifecycle/source visibility
// 4. ranking merge
// 5. budget truncation
// 6. safe next actions
//
// 输出 ContextPack 不包含完整 note body；preview 受 budget 约束。
package agentcontext

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// Compiler 编译 bounded context pack。
type Compiler struct {
	store  *agentmemory.Store
	policy agentmemory.Policy
}

// NewCompiler 构造 compiler。
func NewCompiler(store *agentmemory.Store, policy agentmemory.Policy) *Compiler {
	return &Compiler{store: store, policy: policy}
}

// Compile 执行 context compilation 管线。
func (c *Compiler) Compile(ctx context.Context, req agentprotocol.ContextRequest) (agentprotocol.ContextPack, error) {
	if err := req.Validate(); err != nil {
		return agentprotocol.ContextPack{}, err
	}

	// 阶段 1: permission check
	if !req.Principal.HasCapability(agentprotocol.CapabilityRead) {
		return agentprotocol.ContextPack{}, agentprotocol.NewStableError(
			agentprotocol.ErrCodeInsufficientScope,
			"principal lacks read capability",
		).WithDetail("principal_id", req.Principal.PrincipalID)
	}

	// 阶段 2-3: scope + lifecycle/source visibility 过滤
	query := agentmemory.RecallQuery{
		Scope: req.Scope,
		Kinds: req.KindFilter,
		Limit: 0,
	}
	if req.Budget.MaxItems > 0 {
		query.Limit = req.Budget.MaxItems * 3
	}
	memories, err := c.store.Recall(ctx, query)
	if err != nil {
		return agentprotocol.ContextPack{}, err
	}

	return c.CompileMemories(ctx, req, memories)
}

// CompileMemories 对已检索的 memories 执行 ranking/budget/next-action 管线。
// 用于合并 legacy + new memories 后的编译，避免重复查询。
func (c *Compiler) CompileMemories(_ context.Context, req agentprotocol.ContextRequest, memories []agentprotocol.MemoryRecord) (agentprotocol.ContextPack, error) {
	// 阶段 4: ranking + merge
	ranked := c.rankAndMerge(memories, req)

	// 阶段 5: budget truncation
	pack := c.applyBudget(ranked, req)

	// 阶段 6: safe next actions
	pack.NextActions = c.buildNextActions(req, pack)

	return pack, nil
}

// rankedEntry 是带分数的 context entry 候选。
type rankedEntry struct {
	entry agentprotocol.ContextEntry
	score float64
}

// rankAndMerge 按 confidence/freshness/kind/entity 匹配排序。
func (c *Compiler) rankAndMerge(memories []agentprotocol.MemoryRecord, req agentprotocol.ContextRequest) []rankedEntry {
	now := time.Now().UTC()
	entities := make(map[string]bool, len(req.Entities))
	for _, e := range req.Entities {
		entities[strings.ToLower(e)] = true
	}

	ranked := make([]rankedEntry, 0, len(memories))
	for _, m := range memories {
		entry := toContextEntry(m)
		score := scoreMemory(m, req, entities, now)
		entry.ScoreReason = explainScore(score, m)
		ranked = append(ranked, rankedEntry{entry: entry, score: score})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].entry.MemoryID < ranked[j].entry.MemoryID
	})
	return ranked
}

// toContextEntry 将 MemoryRecord 转换为 bounded ContextEntry（不含完整 body）。
func toContextEntry(m agentprotocol.MemoryRecord) agentprotocol.ContextEntry {
	preview := m.Summary
	if preview == "" {
		preview = m.Object
	}
	// bounded preview: 最多 200 chars
	if len(preview) > 200 {
		preview = preview[:197] + "..."
	}
	return agentprotocol.ContextEntry{
		MemoryID:   m.ID,
		Kind:       m.Kind,
		Subject:    m.Subject,
		Summary:    m.Summary,
		Preview:    preview,
		Confidence: m.Confidence,
		Sources:    m.Sources,
		State:      m.State,
	}
}

// scoreMemory 计算单条 memory 的 context fitness 分数。
func scoreMemory(m agentprotocol.MemoryRecord, req agentprotocol.ContextRequest, entities map[string]bool, now time.Time) float64 {
	score := 0.0

	// confidence weight
	switch m.Confidence {
	case agentprotocol.ConfidenceVerified:
		score += 40
	case agentprotocol.ConfidenceHigh:
		score += 30
	case agentprotocol.ConfidenceMedium:
		score += 20
	case agentprotocol.ConfidenceLow:
		score += 10
	}

	// kind preference: decisions and facts rank higher for context
	switch m.Kind {
	case agentprotocol.MemoryKindDecision:
		score += 15
	case agentprotocol.MemoryKindFact:
		score += 10
	case agentprotocol.MemoryKindProcedure:
		score += 8
	case agentprotocol.MemoryKindPreference:
		score += 5
	}

	// entity match boost
	subjLower := strings.ToLower(m.Subject)
	for entity := range entities {
		if strings.Contains(subjLower, entity) {
			score += 25
			break
		}
	}

	// task match boost
	if req.Task != "" && strings.Contains(subjLower, strings.ToLower(req.Task)) {
		score += 20
	}

	// freshness decay (7-day half-life)
	if !m.UpdatedAt.IsZero() {
		days := now.Sub(m.UpdatedAt).Hours() / 24
		if days > 0 {
			score -= days * 2 // 每天扣 2 分
		}
	}

	// conflicted penalty (conflict 仍可见但排后)
	if m.State == agentprotocol.LifecycleConflicted {
		score -= 5
	}

	return score
}

// explainScore 生成人类可读的 scoring reason（不引用 body）。
func explainScore(score float64, m agentprotocol.MemoryRecord) string {
	parts := []string{}
	parts = append(parts, "confidence="+string(m.Confidence))
	parts = append(parts, "kind="+string(m.Kind))
	if score > 50 {
		parts = append(parts, "high_relevance")
	}
	return strings.Join(parts, ";")
}

// applyBudget 按 max_items/max_chars 确定性截断，设 truncated=true。
func (c *Compiler) applyBudget(ranked []rankedEntry, req agentprotocol.ContextRequest) agentprotocol.ContextPack {
	pack := agentprotocol.ContextPack{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
	}

	maxItems := req.Budget.MaxItems
	if maxItems <= 0 {
		maxItems = 20 // 默认上限
	}
	maxChars := req.Budget.MaxChars
	if maxChars <= 0 {
		maxChars = 8000 // 默认上限
	}

	totalChars := 0
	count := 0
	truncated := false

	for _, r := range ranked {
		if count >= maxItems {
			truncated = true
			break
		}
		entryChars := len(r.entry.Preview) + len(r.entry.Summary) + len(r.entry.Subject)
		if totalChars+entryChars > maxChars && count > 0 {
			truncated = true
			break
		}
		totalChars += entryChars
		c.appendEntry(&pack, r.entry)
		count++
	}

	pack.Truncated = truncated
	return pack
}

// appendEntry 按 kind 分桶。
func (c *Compiler) appendEntry(pack *agentprotocol.ContextPack, e agentprotocol.ContextEntry) {
	switch e.Kind {
	case agentprotocol.MemoryKindFact:
		pack.Facts = append(pack.Facts, e)
	case agentprotocol.MemoryKindDecision:
		pack.Decisions = append(pack.Decisions, e)
	case agentprotocol.MemoryKindPreference:
		pack.Preferences = append(pack.Preferences, e)
	case agentprotocol.MemoryKindProcedure:
		pack.Procedures = append(pack.Procedures, e)
	case agentprotocol.MemoryKindTask:
		if e.State == agentprotocol.LifecycleConfirmed {
			pack.OpenTasks = append(pack.OpenTasks, e)
		}
	case agentprotocol.MemoryKindFailure:
		pack.FailedAttempts = append(pack.FailedAttempts, e)
	}
}

// buildNextActions 生成安全 drill-down 操作建议。
func (c *Compiler) buildNextActions(req agentprotocol.ContextRequest, pack agentprotocol.ContextPack) []agentprotocol.NextAction {
	actions := []agentprotocol.NextAction{}
	if pack.Truncated {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "narrow_scope",
			Command: "pinax agent context --scope " + string(req.Scope.Kind) + ":" + req.Scope.ID + " --kind-filter <specific>",
			Reason:  "context was truncated; narrow kind or scope for more focused results",
		})
	}
	if len(pack.Conflicts) > 0 {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "resolve_conflicts",
			Command: "pinax agent memory resolve --scope " + req.Scope.ID,
			Reason:  "conflicting memories detected; resolve to establish authoritative record",
		})
	}
	return actions
}
