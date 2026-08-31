package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/continuityevidence"
)

// RateValue 是带 not_measured 语义的比率（导出 continuityevidence 同名类型）。
type RateValue = continuityevidence.RateValue

// ContinuityReport 是六周 dogfood 报告（导出类型别名，保持 app 为 CLI 唯一入口）。
type ContinuityReport = continuityevidence.Report

// continuityEvidenceStoreFor 打开 vault 的 continuity evidence store。
// 该 store 只在显式 record/feedback/report 路径打开，AutoMigrate 幂等。
func (s *AgentMemoryService) continuityEvidenceStoreFor(vaultPath string) (*continuityevidence.Store, error) {
	st, err := s.storeFor(vaultPath)
	if err != nil {
		return nil, err
	}
	return continuityevidence.Open(st.DB())
}

// ContinuityRunRecord 描述一次 recorded continuation run 的最小输入。
type ContinuityRunRecord struct {
	VaultPath      string
	BindingDigest  string
	Scope          agentprotocol.Scope
	Runtime        string
	TaskClass      string
	HandoffStatus  string
	SourceTotal    int
	SourceResolved int
	SourceStale    int
	SourceMissing  int
	WarningCodes   []string
	ProposalCount  int
}

// ContinuityRecordRun 创建 opt-in run receipt。默认 continue 不调用本函数。
// 枚举校验在写入前完成；scope ID 只存 bounded digest，不存原文。
func (s *AgentMemoryService) ContinuityRecordRun(ctx context.Context, req ContinuityRunRecord) (string, error) {
	ctx = ensureCtx(ctx)
	store, err := s.continuityEvidenceStoreFor(req.VaultPath)
	if err != nil {
		return "", fmt.Errorf("open continuity evidence store: %w", err)
	}
	runID := "crun_" + continuityScopeDigest(req.VaultPath+req.Scope.ID+req.Runtime+req.TaskClass+nowUTC().Format(time.RFC3339Nano))
	err = store.CreateRun(ctx, continuityevidence.ContinuityRunRow{
		RunID:          runID,
		BindingDigest:  req.BindingDigest,
		ScopeKind:      string(req.Scope.Kind),
		ScopeIDDigest:  continuityScopeDigest(req.Scope.ID),
		Runtime:        req.Runtime,
		TaskClass:      req.TaskClass,
		StartedAt:      nowUTC(),
		HandoffStatus:  req.HandoffStatus,
		SourceTotal:    req.SourceTotal,
		SourceResolved: req.SourceResolved,
		SourceStale:    req.SourceStale,
		SourceMissing:  req.SourceMissing,
		WarningCodes:   strings.Join(req.WarningCodes, ","),
		ProposalCount:  req.ProposalCount,
		// silent_confirmed_write_count 由 canonical lifecycle receipt 计算；
		// 本 slice 的唯一写入路径（checkpoint/review approve）都不产生 silent
		// confirm，因此恒为 0。未来 lifecycle 检测到未经 review 的 confirm
		// 时由 service 填入非零值。
		SilentConfirmedWriteCnt: 0,
	})
	if err != nil {
		return "", err
	}
	return runID, nil
}

// ContinuityFeedbackRequest 描述一次用户四值 outcome 或 weekly review 提交。
type ContinuityFeedbackRequest struct {
	VaultPath        string
	RunID            string
	Outcome          string
	ReviewSeconds    int
	ReviewSecondsSet bool
	WeeklyReview     bool
}

// ContinuityFeedbackResult 是 feedback 提交结果。
type ContinuityFeedbackResult struct {
	FeedbackID string `json:"feedback_id"`
	EventKind  string `json:"event_kind"`
	Outcome    string `json:"outcome,omitempty"`
	Supersedes string `json:"supersedes,omitempty"`
}

// ContinuityFeedback 追加 append-only feedback event。
// 缺失 run、invalid enum 都不写；重复提交追加 superseding event。
func (s *AgentMemoryService) ContinuityFeedback(ctx context.Context, req ContinuityFeedbackRequest) (ContinuityFeedbackResult, error) {
	ctx = ensureCtx(ctx)

	if req.WeeklyReview {
		// weekly review 事件独立于 outcome：允许空 inbox 记录 0 秒。
		if strings.TrimSpace(req.RunID) != "" || strings.TrimSpace(req.Outcome) != "" {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"--weekly-review cannot be combined with --run or --outcome")
		}
		if !req.ReviewSecondsSet || req.ReviewSeconds < 0 {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"--weekly-review requires --review-seconds <non-negative integer>")
		}
	} else {
		if strings.TrimSpace(req.RunID) == "" {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "--run <run-id> is required")
		}
		if strings.TrimSpace(req.Outcome) == "" {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"--outcome <trusted|corrected|wrong_project|insufficient> is required")
		}
		if !continuityevidence.ValidOutcome(req.Outcome) {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"unknown outcome: "+req.Outcome)
		}
		if req.ReviewSecondsSet && req.ReviewSeconds < 0 {
			return ContinuityFeedbackResult{}, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed,
				"review_seconds must be a non-negative integer")
		}
	}

	store, err := s.continuityEvidenceStoreFor(req.VaultPath)
	if err != nil {
		return ContinuityFeedbackResult{}, fmt.Errorf("open continuity evidence store: %w", err)
	}
	feedbackID := "cfb_" + continuityScopeDigest(nowUTC().Format(time.RFC3339Nano)+req.RunID+req.Outcome)

	if req.WeeklyReview {
		event := continuityevidence.ContinuityFeedbackEventRow{
			FeedbackID:    feedbackID,
			EventKind:     continuityevidence.EventKindWeeklyReview,
			ReviewSeconds: intPtr(req.ReviewSeconds),
			SubmittedAt:   nowUTC(),
		}
		if err := store.AppendFeedback(ctx, event); err != nil {
			return ContinuityFeedbackResult{}, err
		}
		return ContinuityFeedbackResult{FeedbackID: feedbackID, EventKind: event.EventKind}, nil
	}

	if _, err := store.GetRun(ctx, req.RunID); err != nil {
		return ContinuityFeedbackResult{}, agentprotocol.NewStableError("continuity_run_not_found",
			"recorded run not found: "+req.RunID)
	}

	// 查找同一 run 的最新 outcome event 作为 supersede 目标（保留审计链）。
	supersedes := ""
	existing, err := store.ListFeedbackSince(ctx, time.Time{})
	if err != nil {
		return ContinuityFeedbackResult{}, err
	}
	for _, event := range existing {
		if event.RunID == req.RunID && event.EventKind == continuityevidence.EventKindOutcome {
			supersedes = event.FeedbackID
		}
	}
	event := continuityevidence.ContinuityFeedbackEventRow{
		FeedbackID:           feedbackID,
		RunID:                req.RunID,
		EventKind:            continuityevidence.EventKindOutcome,
		Outcome:              req.Outcome,
		SubmittedAt:          nowUTC(),
		SupersedesFeedbackID: supersedes,
	}
	if req.ReviewSecondsSet && req.ReviewSeconds > 0 {
		event.ReviewSeconds = intPtr(req.ReviewSeconds)
	}
	if err := store.AppendFeedback(ctx, event); err != nil {
		return ContinuityFeedbackResult{}, err
	}
	return ContinuityFeedbackResult{
		FeedbackID: feedbackID,
		EventKind:  event.EventKind,
		Outcome:    event.Outcome,
		Supersedes: supersedes,
	}, nil
}

// ContinuityReportRequest 描述一次报告生成。
type ContinuityReportRequest struct {
	VaultPath string
	Since     string
	Now       time.Time
}

// ContinuityReport 聚合窗口内 receipt 并执行预注册 Go gates（CLI-authored）。
func (s *AgentMemoryService) ContinuityReport(ctx context.Context, req ContinuityReportRequest) (continuityevidence.Report, error) {
	ctx = ensureCtx(ctx)
	since, err := parseReportSince(req.Since)
	if err != nil {
		return continuityevidence.Report{}, err
	}
	now := req.Now
	if now.IsZero() {
		now = nowUTC()
	}
	store, err := s.continuityEvidenceStoreFor(req.VaultPath)
	if err != nil {
		return continuityevidence.Report{}, fmt.Errorf("open continuity evidence store: %w", err)
	}
	windowStart := now.Add(-since)
	runs, err := store.ListRunsBetween(ctx, windowStart, now)
	if err != nil {
		return continuityevidence.Report{}, err
	}
	events, err := store.ListFeedbackBetween(ctx, windowStart, now)
	if err != nil {
		return continuityevidence.Report{}, err
	}
	return continuityevidence.BuildReport(windowStart, now, runs, events), nil
}

// continuityCheckpointLinkStore 验证 optional run link 属于同一 vault/scope。
// 该检查在 handoff 写入前完成，避免未知或跨 scope run 只被输出回显。
func (s *AgentMemoryService) continuityCheckpointLinkStore(ctx context.Context, vaultPath string, scope agentprotocol.Scope, runID string) (*continuityevidence.Store, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	store, err := s.continuityEvidenceStoreFor(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("open continuity evidence store: %w", err)
	}
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return nil, agentprotocol.NewStableError("continuity_run_not_found", "recorded run not found: "+runID)
	}
	if run.ScopeKind != string(scope.Kind) || run.ScopeIDDigest != continuityScopeDigest(scope.ID) {
		return nil, agentprotocol.NewStableError(agentprotocol.ErrCodeInvalidScope, "recorded run belongs to a different scope")
	}
	return store, nil
}

// parseReportSince 解析 6w/30d/24h/90m 形式的窗口。
// w/d 是 Pinax 窗口惯例，其余委托 time.ParseDuration。
func parseReportSince(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "6w"
	}
	if strings.HasSuffix(value, "w") || strings.HasSuffix(value, "d") {
		n, err := strconv.Atoi(strings.TrimRight(value, "wd"))
		if err != nil || n <= 0 {
			return 0, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "invalid --since window: "+value)
		}
		if strings.HasSuffix(value, "w") {
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "invalid --since window: "+value)
	}
	return parsed, nil
}

func intPtr(value int) *int { return &value }

// continuityScopeDigest 计算 opaque 字符串的 bounded sha256 digest（16 hex chars）。
// scope ID / run seed 只存 digest，不把原始字符串写进 evidence 表。
func continuityScopeDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
