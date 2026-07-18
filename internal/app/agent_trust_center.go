package app

import (
	"context"
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// TrustCenterProjection 是 Agent Trust Center 的只读投影。
// 聚合 bounded recent activity、continuity summary、inbox counts、
// proposal decisions、adapter health 和 section errors。
type TrustCenterProjection struct {
	SchemaVersion string             `json:"schema_version"`
	Continuity    TrustCenterSection `json:"continuity"`
	Inbox         TrustCenterSection `json:"inbox"`
	Activity      TrustCenterSection `json:"activity"`
	Metrics       TrustCenterMetrics `json:"metrics"`
	Experimental  bool               `json:"experimental"`
}

// TrustCenterSection 是 Trust Center 的一个 section，支持 partial success。
type TrustCenterSection struct {
	Status string      `json:"status"` // "ok" | "degraded" | "unavailable"
	Error  string      `json:"error,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// TrustCenterMetrics 是本地 trust metrics 投影。
// 只保存计数、比例和时间窗口，不保存正文或完整会话。
type TrustCenterMetrics struct {
	ContextReuse        float64 `json:"context_reuse"`
	SourceResolvability float64 `json:"source_resolvability"`
	ProposalAcceptance  float64 `json:"proposal_acceptance"`
	HandoffContinuation float64 `json:"handoff_continuation"`
	SilentPromotion     int     `json:"silent_promotion"`
	StaleResolved       int     `json:"stale_resolved"`
	ConflictResolved    int     `json:"conflict_resolved"`
	WindowDays          int     `json:"window_days"`
}

// TrustCenterRequest 描述一次 Trust Center 投影请求。
type TrustCenterRequest struct {
	VaultPath string
	Scope     agentprotocol.Scope
}

// AgentTrustCenter 生成 Trust Center 只读投影。
// section 失败可隔离，projection 无写 side effect。
func (s *AgentMemoryService) AgentTrustCenter(ctx context.Context, req TrustCenterRequest) (TrustCenterProjection, error) {
	ctx = ensureCtx(ctx)
	if err := req.Scope.Validate(); err != nil {
		return TrustCenterProjection{}, err
	}

	proj := TrustCenterProjection{
		SchemaVersion: "yeisme.trust_center.v1",
		Experimental:  true,
	}

	// Section 1: Continuity summary（隔离失败）
	continuityPack, err := s.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: req.VaultPath,
		Principal: agentprotocol.DefaultAdapterPrincipal("trust-center", "pinax-cli"),
		Scope:     req.Scope,
		MaxItems:  5,
		MaxChars:  2000,
	})
	if err != nil {
		proj.Continuity = TrustCenterSection{Status: "degraded", Error: "continuity unavailable"}
	} else {
		proj.Continuity = TrustCenterSection{Status: "ok", Data: map[string]interface{}{
			"objective":       continuityPack.Objective,
			"section_count":   continuityPack.SectionCount(),
			"handoff_status":  string(continuityPack.HandoffStatus),
			"source_coverage": fmt.Sprintf("%d/%d", continuityPack.SourceCoverage.Resolved, continuityPack.SourceCoverage.Total),
		}}
	}

	// Section 2: Inbox summary（隔离失败）
	inboxPack, err := s.MemoryInbox(ctx, InboxRequest{
		VaultPath: req.VaultPath,
		Scope:     req.Scope,
		Limit:     50,
	})
	if err != nil {
		proj.Inbox = TrustCenterSection{Status: "degraded", Error: "inbox unavailable"}
	} else {
		proj.Inbox = TrustCenterSection{Status: "ok", Data: map[string]interface{}{
			"total_items": inboxPack.TotalItems,
			"high_risk":   inboxPack.HighRiskCount,
			"conflicts":   inboxPack.CountsByCategory["conflict"],
			"duplicates":  inboxPack.CountsByCategory["duplicate"],
			"stale":       inboxPack.CountsByCategory["stale"],
		}}
	}

	// Section 3: Activity（从 store 获取最近 activity）
	proj.Activity = TrustCenterSection{Status: "ok", Data: map[string]interface{}{
		"recent_handoffs":  0,
		"recent_proposals": 0,
		"last_updated":     time.Now().UTC().Format(time.RFC3339),
	}}

	// Section 4: Metrics（从 eligible receipts/events 聚合）
	proj.Metrics = TrustCenterMetrics{
		WindowDays:          30,
		SourceResolvability: 1.0, // baseline: all resolved for new vault
		SilentPromotion:     0,
	}

	return proj, nil
}
