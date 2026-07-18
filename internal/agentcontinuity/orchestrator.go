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
	sections := o.projectSections(pack, req.Budget)

	// 阶段 4: source coverage（SourceRef 没有 Status 字段，从 presence 推断）
	coverage := o.computeSourceCoverage(pack)

	// 阶段 5: budget truncation
	truncated := pack.Truncated
	if o.totalSectionChars(sections) > req.Budget.MaxChars {
		sections = o.truncateSections(sections, req.Budget.MaxChars)
		truncated = true
	}

	// 阶段 6: next actions
	nextActions := o.buildNextActions(req, pack, handoffStatus)

	result := ContinuityPack{
		SchemaVersion:  ContinuitySchemaVersion,
		Principal:      req.Principal,
		Scope:          req.Scope,
		Task:           req.Task,
		Objective:      o.deriveObjective(req, handoff),
		CurrentState:   o.deriveCurrentState(handoff),
		Sections:       sections,
		Conflicts:      pack.Conflicts,
		Sources:        o.boundedSources(pack, req.Budget.MaxSources),
		SourceCoverage: coverage,
		HandoffStatus:  handoffStatus,
		HandoffID:      o.handoffID(handoff),
		Truncated:      truncated,
		Freshness:      time.Now().UTC(),
		NextActions:    nextActions,
		Experimental:   true,
	}
	return result, nil
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
func (o *Orchestrator) projectSections(pack agentprotocol.ContextPack, budget ContinuityBudget) []ContinuitySection {
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
	o.appendHandoffSections(&sections, budget)

	return sections
}

// appendHandoffSections 从最近的 handoff 提取 blockers 和 completed work。
// 这是产品投影，不复制完整 handoff body。
func (o *Orchestrator) appendHandoffSections(sections *[]ContinuitySection, budget ContinuityBudget) {
	// handoff sections 由 app service 在 Compile 之外追加
	// 这里只预留扩展点
}

// computeSourceCoverage 从 context pack 的 sources 计算解析情况。
// SourceRef 没有 Status 字段，因此 Total = len(sources)，Resolved = Total（全部存在即解析）。
func (o *Orchestrator) computeSourceCoverage(pack agentprotocol.ContextPack) SourceCoverage {
	total := len(pack.Sources)
	return SourceCoverage{Total: total, Resolved: total}
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
func (o *Orchestrator) buildNextActions(req ContinuityRequest, pack agentprotocol.ContextPack, hs HandoffStatus) []agentprotocol.NextAction {
	var actions []agentprotocol.NextAction

	scopeStr := fmt.Sprintf("%s:%s", req.Scope.Kind, req.Scope.ID)

	if hs == HandoffStatusConsumed || hs == HandoffStatusExplicit {
		actions = append(actions, agentprotocol.NextAction{
			Name:    "View handoff details",
			Command: fmt.Sprintf("pinax agent handoff show --scope %s", scopeStr),
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
func (o *Orchestrator) degradedPack(req ContinuityRequest, handoff *agentmemory.AgentHandoffRow, hs HandoffStatus) ContinuityPack {
	pack := ContinuityPack{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     req.Principal,
		Scope:         req.Scope,
		Task:          req.Task,
		HandoffStatus: hs,
		Experimental:  true,
		Freshness:     time.Now().UTC(),
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
	}
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

func (o *Orchestrator) boundedSources(pack agentprotocol.ContextPack, maxSources int) agentprotocol.SourceRefList {
	if maxSources <= 0 || len(pack.Sources) <= maxSources {
		return pack.Sources
	}
	// SourceRef 没有 Status，按 Kind 优先级截断（note > receipt > task > other）
	sorted := make(agentprotocol.SourceRefList, len(pack.Sources))
	copy(sorted, pack.Sources)
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
		obj := pack.Objective
		if len(obj) > 80 {
			obj = obj[:77] + "..."
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
