package agentcontinuity

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// Orchestrator 编译 bounded continuity pack。
// 它只编排 scope resolution、handoff selection、context compilation、
// source coverage、budget truncation 和 safe next actions。
// 不创建第二套 memory ranking、ledger 或 source resolver。
type Orchestrator struct {
	store    *agentmemory.Store
	compiler *agentcontext.Compiler
}

func handoffSources(handoff *agentmemory.AgentHandoffRow) agentprotocol.SourceRefList {
	if handoff == nil {
		return nil
	}
	return handoff.Sources
}

func mergeSources(groups ...agentprotocol.SourceRefList) agentprotocol.SourceRefList {
	seen := make(map[string]int)
	var merged agentprotocol.SourceRefList
	for _, group := range groups {
		for _, source := range group {
			key := source.Kind + "\x00" + source.Ref
			if idx, ok := seen[key]; ok {
				// 同 kind+ref 去重时保留更精确的变体：context pack 的无 span
				// 引用不得覆盖 handoff 的 rev: 钉定，否则 pinned 漂移检测会
				// 把 stale 误判成 resolved。
				if merged[idx].Span == "" && source.Span != "" {
					merged[idx] = source
				}
				continue
			}
			seen[key] = len(merged)
			merged = append(merged, source)
		}
	}
	return merged
}

// NewOrchestrator 构造 orchestrator。
func NewOrchestrator(store *agentmemory.Store, compiler *agentcontext.Compiler) *Orchestrator {
	return &Orchestrator{store: store, compiler: compiler}
}

// Compile 执行 continuity compilation 管线。
func (o *Orchestrator) Compile(ctx context.Context, req ContinuityRequest) (ContinuityPack, error) {
	if err := req.Validate(); err != nil {
		return ContinuityPack{}, err
	}

	// 阶段 1: handoff selection
	handoff, handoffStatus := o.selectHandoff(ctx, req)

	// 阶段 2: context compilation（复用 compiler）
	contextReq := agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Intent:        req.Intent,
		Task:          req.Task,
		Budget: agentprotocol.ContextBudget{
			MaxItems: req.Budget.MaxItems,
			MaxChars: req.Budget.MaxChars,
		},
	}
	pack, err := o.compiler.Compile(ctx, contextReq)
	if err != nil {
		// 降级：context 编译失败时返回 partial pack，不泄漏 raw error
		return o.degradedPack(req, handoff, handoffStatus), nil
	}

	// 阶段 3: section projection（ContextPack 用分桶字段，不是 flat Entries）
	sections := o.projectSections(pack, handoff, req.Budget)

	// 阶段 4: source coverage（SourceRef 没有 Status 字段，从 presence 推断）
	sources := mergeSources(pack.Sources, handoffSources(handoff))
	coverage := o.computeSourceCoverage(sources)

	// 阶段 5: budget truncation
	truncated := pack.Truncated
	if o.totalSectionChars(sections) > req.Budget.MaxChars {
		sections = o.truncateSections(sections, req.Budget.MaxChars)
		truncated = true
	}

	// 阶段 6: next actions
	nextActions := o.buildNextActions(req, pack, handoffStatus, o.handoffID(handoff))

	now := time.Now().UTC()
	// freshness 取支持本 pack 的真实 evidence 时间（handoff 创建时间），
	// 而不是本次响应生成时刻；生成时刻单独放 GeneratedAt（additive）。
	freshness := time.Time{}
	if handoff != nil && handoff.CreatedAt.After(freshness) {
		freshness = handoff.CreatedAt.UTC()
	}

	result := ContinuityPack{
		SchemaVersion:  ContinuitySchemaVersion,
		Principal:      req.Principal,
		Scope:          req.Scope,
		Task:           req.Task,
		Objective:      o.deriveObjective(req, handoff),
		CurrentState:   o.deriveCurrentState(handoff),
		Sections:       sections,
		Conflicts:      pack.Conflicts,
		Sources:        o.boundedSources(sources, req.Budget.MaxSources),
		SourceCoverage: coverage,
		HandoffStatus:  handoffStatus,
		HandoffID:      o.handoffID(handoff),
		Truncated:      truncated,
		Freshness:      freshness,
		GeneratedAt:    now,
		NextActions:    nextActions,
		Experimental:   true,
	}
	result.RefreshDerived()
	return result, nil
}

// RefreshDerived 重新计算 FreshnessStatus/EvidenceStatus/PackStatus、
// WarningCodes 和 RecommendedNextAction。
//
// 状态与排序规则（deterministic，中文注释）：
//  1. warning 顺序固定：handoff_missing → decision_conflict → source_missing →
//     source_stale → source_ambiguous → context_degraded → review_attention。conflict/source
//     warning 是风险信号，不被 budget 截断，任何 renderer 不得丢弃。
//  2. evidence_status：total=0 → not_measured；存在任何 unresolved →
//     partial；全部 resolved → resolved。
//  3. freshness_status：无 evidence → not_measured；最新 evidence 超出
//     FreshnessPolicy（14 天）→ stale；否则 fresh。生成时刻不参与判断。
//  4. pack_status：handoff 缺失、存在 conflict 或 evidence 非 resolved
//     → partial；其余 ready。partial 不丢弃可信 section。
//  5. recommended_next_action 只取一个：handoff 缺失优先给 checkpoint
//     action（context-only resume 的恢复路径），其次 conflict、source、
//     review attention，最后回退到第一个 drill-down action。
func (p *ContinuityPack) RefreshDerived() {
	// warning codes（固定顺序）
	p.WarningCodes = nil
	if p.HandoffStatus == HandoffStatusMissing {
		p.WarningCodes = append(p.WarningCodes, WarningHandoffMissing)
	}
	if len(p.Conflicts) > 0 {
		p.WarningCodes = append(p.WarningCodes, WarningDecisionConflict)
	}
	if p.SourceCoverage.Missing > 0 {
		p.WarningCodes = append(p.WarningCodes, WarningSourceMissing)
	}
	if p.SourceCoverage.Stale > 0 {
		p.WarningCodes = append(p.WarningCodes, WarningSourceStale)
	}
	if p.SourceCoverage.Ambiguous > 0 {
		p.WarningCodes = append(p.WarningCodes, WarningSourceAmbiguous)
	}
	if p.HandoffStatus == HandoffStatusDegraded {
		p.WarningCodes = append(p.WarningCodes, WarningContextDegraded)
	}
	if p.ReviewAttentionCount > 0 && p.ReviewAttention != nil {
		p.WarningCodes = append(p.WarningCodes, WarningReviewAttention)
	}
	if p.ReviewAttentionUnavailable {
		p.WarningCodes = append(p.WarningCodes, WarningReviewAttentionUnavailable)
	}

	// evidence status
	switch {
	case p.SourceCoverage.Total == 0:
		p.EvidenceStatus = EvidenceStatusNotMeasured
	case p.SourceCoverage.Missing > 0 || p.SourceCoverage.Stale > 0 || p.SourceCoverage.Ambiguous > 0:
		p.EvidenceStatus = EvidenceStatusPartial
	default:
		p.EvidenceStatus = EvidenceStatusResolved
	}

	// freshness status
	switch {
	case p.Freshness.IsZero():
		p.FreshnessStatus = FreshnessStatusNotMeasured
	case time.Since(p.Freshness) > FreshnessPolicy:
		p.FreshnessStatus = FreshnessStatusStale
	default:
		p.FreshnessStatus = FreshnessStatusFresh
	}

	// pack status
	if p.HandoffStatus == HandoffStatusMissing || len(p.Conflicts) > 0 || p.EvidenceStatus != EvidenceStatusResolved {
		p.PackStatus = PackStatusPartial
	} else {
		p.PackStatus = PackStatusReady
	}

	// 单 next action（预算外，固定优先级）
	p.RecommendedNextAction = p.selectRecommendedNextAction()
}

// selectRecommendedNextAction 按固定优先级选择一个推荐动作。
func (p ContinuityPack) selectRecommendedNextAction() *agentprotocol.NextAction {
	scopeStr := fmt.Sprintf("%s:%s", p.Scope.Kind, p.Scope.ID)
	if p.HandoffStatus == HandoffStatusMissing {
		return &agentprotocol.NextAction{
			Name:    "Create a continuity checkpoint",
			Command: fmt.Sprintf("pinax continue checkpoint --scope %s --objective \"<bounded objective>\"", scopeStr),
			Reason:  "no handoff found for this scope; context-only resume",
		}
	}
	if len(p.Conflicts) > 0 {
		return &agentprotocol.NextAction{
			Name:    "Resolve memory conflicts",
			Command: fmt.Sprintf("pinax review --scope %s", scopeStr),
			Reason:  "conflicting decisions exist in this scope",
		}
	}
	if p.SourceCoverage.Missing > 0 || p.SourceCoverage.Stale > 0 || p.SourceCoverage.Ambiguous > 0 {
		if p.HandoffID != "" {
			return &agentprotocol.NextAction{
				Name:    "Inspect source evidence",
				Command: fmt.Sprintf("pinax agent handoff show %s", p.HandoffID),
				Reason:  "at least one supporting source is stale or missing",
			}
		}
		return &agentprotocol.NextAction{
			Name:    "Inspect source evidence",
			Command: "pinax continue status --repo .",
			Reason:  "at least one supporting source is stale or missing",
		}
	}
	if p.ReviewAttentionCount > 0 && p.ReviewAttention != nil {
		return &agentprotocol.NextAction{
			Name:    "Review pending memory item",
			Command: fmt.Sprintf("pinax review --scope %s", scopeStr),
			Reason:  "a pending review item may change the next action",
		}
	}
	for _, action := range p.NextActions {
		copied := action
		return &copied
	}
	return &agentprotocol.NextAction{
		Name:    "Get full context pack",
		Command: fmt.Sprintf("pinax agent context --scope %s", scopeStr),
	}
}

// selectHandoff 按显式 ID 或 scope 自动选择最近可消费 handoff。
func (o *Orchestrator) selectHandoff(ctx context.Context, req ContinuityRequest) (*agentmemory.AgentHandoffRow, HandoffStatus) {
	handoffs, err := o.store.ListHandoffs(ctx, req.Scope)
	if err != nil || len(handoffs) == 0 {
		return nil, HandoffStatusMissing
	}

	// 显式指定 ID 时查找匹配
	if req.HandoffID != "" {
		for i := range handoffs {
			if handoffs[i].HandoffID == req.HandoffID {
				return &handoffs[i], HandoffStatusExplicit
			}
		}
		return nil, HandoffStatusMissing
	}

	// 自动选择最近一条（ListHandoffs 已按 created_at DESC 排序）
	return &handoffs[0], HandoffStatusConsumed
}

// projectSections 将 ContextPack 的分桶 entries 投影为产品 sections。
func (o *Orchestrator) projectSections(pack agentprotocol.ContextPack, handoff *agentmemory.AgentHandoffRow, budget ContinuityBudget) []ContinuitySection {
	type bucket struct {
		kind    string
		title   string
		entries []agentprotocol.ContextEntry
	}

	buckets := []bucket{
		{"decision", "Key decisions", pack.Decisions},
		{"preference", "Preferences", pack.Preferences},
		{"open_task", "Open tasks", pack.OpenTasks},
		{"failed_attempt", "Failed attempts", pack.FailedAttempts},
		{"procedure", "Procedures", pack.Procedures},
		{"fact", "Key facts", pack.Facts},
	}

	maxPerSection := budget.MaxItems
	if maxPerSection < 1 {
		maxPerSection = 1
	}

	var sections []ContinuitySection
	for _, b := range buckets {
		if len(b.entries) == 0 {
			continue
		}
		items := make([]string, 0, len(b.entries))
		for _, e := range b.entries {
			// 优先使用 subject，其次 summary，最后 preview
			text := e.Subject
			if text == "" {
				text = e.Summary
			}
			if text == "" {
				text = e.Preview
			}
			if text != "" {
				items = append(items, text)
			}
		}
		if len(items) == 0 {
			continue
		}
		section := ContinuitySection{Kind: b.kind, Title: b.title, Items: items}
		if len(items) > maxPerSection {
			section.Truncated = true
			section.Omitted = len(items) - maxPerSection
			section.Items = items[:maxPerSection]
		}
		sections = append(sections, section)
	}

	// 如果有 handoff-derived blockers/completed work，追加 sections
	o.appendHandoffSections(&sections, handoff, budget)

	return sections
}

// appendHandoffSections 从最近的 handoff 提取 blockers 和 completed work。
// 这是产品投影，不复制完整 handoff body。
func (o *Orchestrator) appendHandoffSections(sections *[]ContinuitySection, handoff *agentmemory.AgentHandoffRow, budget ContinuityBudget) {
	if handoff == nil {
		return
	}
	for _, section := range []struct {
		kind  string
		title string
		value string
	}{
		{kind: "handoff_decision", title: "Handoff decisions", value: handoff.Decisions},
		{kind: "completed_work", title: "Completed work", value: handoff.CompletedWork},
		{kind: "blocker", title: "Blockers", value: handoff.Blockers},
		{kind: "verification", title: "Verification", value: handoff.Verification},
		{kind: "follow_up", title: "Follow-ups", value: handoff.FollowUps},
	} {
		items := splitBoundedLines(section.value, budget.MaxItems)
		if len(items) == 0 {
			continue
		}
		*sections = append(*sections, ContinuitySection{Kind: section.kind, Title: section.title, Items: items})
	}
}

// computeSourceCoverage 从 context pack 的 sources 计算解析情况。
// SourceRef 没有 Status 字段，因此 Total = len(sources)，Resolved = Total（全部存在即解析）。
func (o *Orchestrator) computeSourceCoverage(sources agentprotocol.SourceRefList) SourceCoverage {
	total := len(sources)
	return SourceCoverage{Total: total, Resolved: total}
}

func splitBoundedLines(value string, maxItems int) []string {
	if maxItems < 1 {
		maxItems = 1
	}
	var items []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
		if len(items) == maxItems {
			break
		}
	}
	return items
}

// truncateSections 按 budget 确定性截断 sections。
func (o *Orchestrator) truncateSections(sections []ContinuitySection, maxChars int) []ContinuitySection {
	charBudget := maxChars
	for i := range sections {
		kept := make([]string, 0, len(sections[i].Items))
		for _, item := range sections[i].Items {
			if charBudget-len(item) < 0 {
				break
			}
			charBudget -= len(item)
			kept = append(kept, item)
		}
		if len(kept) < len(sections[i].Items) {
			sections[i].Omitted += len(sections[i].Items) - len(kept)
			sections[i].Truncated = true
		}
		sections[i].Items = kept
	}
	return sections
}

// buildNextActions 生成安全 drill-down 操作建议。
func (o *Orchestrator) buildNextActions(req ContinuityRequest, pack agentprotocol.ContextPack, hs HandoffStatus, handoffID string) []agentprotocol.NextAction {
	var actions []agentprotocol.NextAction

	scopeStr := fmt.Sprintf("%s:%s", req.Scope.Kind, req.Scope.ID)

	if hs == HandoffStatusConsumed || hs == HandoffStatusExplicit {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "View handoff details",
			Command: fmt.Sprintf("pinax agent handoff show %s", handoffID),
		})
	}

	if pack.EntryCount() > 0 {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "Review memory proposals",
			Command: fmt.Sprintf("pinax review --scope %s", scopeStr),
		})
	}

	if len(pack.Conflicts) > 0 {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "Resolve memory conflicts",
			Command: fmt.Sprintf("pinax review --scope %s", scopeStr),
		})
	}

	actions = append(actions, agentprotocol.NextAction{
		Name:    "Get full context pack",
		Command: fmt.Sprintf("pinax agent context --scope %s", scopeStr),
	})

	return actions
}

// degradedPack 在 context 编译失败时返回 bounded partial pack。
// freshness 保持零值（not_measured），生成时刻单独记录。
func (o *Orchestrator) degradedPack(req ContinuityRequest, handoff *agentmemory.AgentHandoffRow, hs HandoffStatus) ContinuityPack {
	pack := ContinuityPack{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		HandoffStatus: hs,
		Experimental:  true,
		GeneratedAt:   time.Now().UTC(),
		NextActions: []agentprotocol.NextAction{
			{
				Name:    "Retry context compilation",
				Command: fmt.Sprintf("pinax continue --scope %s:%s", req.Scope.Kind, req.Scope.ID),
			},
		},
	}
	if handoff != nil {
		pack.Objective = handoff.Objective
		pack.CurrentState = handoff.CurrentState
		pack.HandoffID = handoff.HandoffID
		pack.Freshness = handoff.CreatedAt.UTC()
	}
	pack.WarningCodes = append(pack.WarningCodes, WarningContextDegraded)
	pack.RefreshDerived()
	return pack
}

func (o *Orchestrator) deriveObjective(req ContinuityRequest, handoff *agentmemory.AgentHandoffRow) string {
	if handoff != nil && handoff.Objective != "" {
		return handoff.Objective
	}
	if req.Task != "" {
		return req.Task
	}
	return ""
}

func (o *Orchestrator) deriveCurrentState(handoff *agentmemory.AgentHandoffRow) string {
	if handoff == nil {
		return ""
	}
	return handoff.CurrentState
}

func (o *Orchestrator) handoffID(handoff *agentmemory.AgentHandoffRow) string {
	if handoff == nil {
		return ""
	}
	return handoff.HandoffID
}

func (o *Orchestrator) boundedSources(sources agentprotocol.SourceRefList, maxSources int) agentprotocol.SourceRefList {
	if maxSources <= 0 || len(sources) <= maxSources {
		return sources
	}
	// SourceRef 没有 Status，按 Kind 优先级截断（note > receipt > task > other）
	sorted := make(agentprotocol.SourceRefList, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sourceKindPriority(sorted[i].Kind) > sourceKindPriority(sorted[j].Kind)
	})
	return sorted[:maxSources]
}

func sourceKindPriority(kind string) int {
	switch kind {
	case "note":
		return 5
	case "receipt":
		return 4
	case "task":
		return 3
	case "asset":
		return 2
	}
	return 1
}

func (o *Orchestrator) totalSectionChars(sections []ContinuitySection) int {
	total := 0
	for _, s := range sections {
		for _, item := range s.Items {
			total += len(item)
		}
	}
	return total
}

// SummaryLine 返回 pack 的一行摘要（用于 human output）。
func SummaryLine(pack ContinuityPack) string {
	parts := []string{}
	if pack.Objective != "" {
		objRunes := []rune(pack.Objective)
		obj := pack.Objective
		if len(objRunes) > 80 {
			obj = string(objRunes[:77]) + "..."
		}
		parts = append(parts, fmt.Sprintf("objective: %s", obj))
	}
	parts = append(parts, fmt.Sprintf("%d items", pack.SectionCount()))
	parts = append(parts, fmt.Sprintf("handoff: %s", pack.HandoffStatus))
	if pack.SourceCoverage.Total > 0 {
		parts = append(parts, fmt.Sprintf("sources: %d/%d resolved", pack.SourceCoverage.Resolved, pack.SourceCoverage.Total))
	}
	if pack.Truncated {
		parts = append(parts, "truncated")
	}
	return strings.Join(parts, ", ")
}
