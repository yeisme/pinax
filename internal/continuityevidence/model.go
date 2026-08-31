// Package continuityevidence 实现 opt-in continuity dogfood evidence。
//
// 两张 additive GORM 表（挂在 agent memory sqlite DB 上，由本包独立
// AutoMigrate，旧 DB 自动升级，不回写既有表）：
//
//   - continuity_runs：一次 recorded continuation loop 的最小 receipt；
//   - continuity_feedback_events：append-only 用户 outcome / weekly review 事件。
//
// 证据不保存 task title、note title、prompt、正文、完整路径、provider payload
// 或私有工具参数；run/binding/scope 只存 opaque ID 或 bounded digest。
package continuityevidence

import (
	"context"
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"gorm.io/gorm"
)

// Outcome 是用户显式提交的四值 continuation 结果。
// 只能来自用户选择；Agent 不得按任务完成度、语气或工具日志推断。
type Outcome string

const (
	OutcomeTrusted      Outcome = "trusted"
	OutcomeCorrected    Outcome = "corrected"
	OutcomeWrongProject Outcome = "wrong_project"
	OutcomeInsufficient Outcome = "insufficient"
)

// ValidOutcome 校验 outcome 枚举。
func ValidOutcome(outcome string) bool {
	switch Outcome(outcome) {
	case OutcomeTrusted, OutcomeCorrected, OutcomeWrongProject, OutcomeInsufficient:
		return true
	}
	return false
}

// TaskClass 是 dogfood 覆盖的三类 Pinax 任务。
type TaskClass string

const (
	TaskClassImplementationDebugging TaskClass = "implementation_debugging"
	TaskClassProductSpecDocs         TaskClass = "product_spec_docs"
	TaskClassReleaseOperations       TaskClass = "release_operations"
)

// ValidTaskClass 校验 task class 枚举。
func ValidTaskClass(class string) bool {
	switch TaskClass(class) {
	case TaskClassImplementationDebugging, TaskClassProductSpecDocs, TaskClassReleaseOperations:
		return true
	}
	return false
}

// ContinuityRunRow 是一次 recorded continuation run 的最小 receipt。
// 无正文/path/prompt/provider 字段；warning codes bounded。
type ContinuityRunRow struct {
	RunID                   string     `gorm:"primaryKey;column:run_id" json:"run_id"`
	BindingDigest           string     `gorm:"index;column:binding_digest" json:"binding_digest,omitempty"`
	ScopeKind               string     `gorm:"column:scope_kind" json:"scope_kind"`
	ScopeIDDigest           string     `gorm:"column:scope_id_digest" json:"scope_id_digest"`
	Runtime                 string     `gorm:"index;column:runtime" json:"runtime"`
	TaskClass               string     `gorm:"index;column:task_class" json:"task_class"`
	StartedAt               time.Time  `gorm:"index;column:started_at" json:"started_at"`
	CheckpointedAt          *time.Time `gorm:"column:checkpointed_at" json:"checkpointed_at,omitempty"`
	HandoffStatus           string     `gorm:"column:handoff_status" json:"handoff_status"`
	SourceTotal             int        `gorm:"column:source_total" json:"source_total"`
	SourceResolved          int        `gorm:"column:source_resolved" json:"source_resolved"`
	SourceStale             int        `gorm:"column:source_stale" json:"source_stale"`
	SourceMissing           int        `gorm:"column:source_missing" json:"source_missing"`
	WarningCodes            string     `gorm:"column:warning_codes" json:"warning_codes,omitempty"` // comma-separated, bounded
	ProposalCount           int        `gorm:"column:proposal_count" json:"proposal_count"`
	SilentConfirmedWriteCnt int        `gorm:"column:silent_confirmed_write_count" json:"silent_confirmed_write_count"`
}

// TableName 固定 additive 表名。
func (ContinuityRunRow) TableName() string { return "continuity_runs" }

// ContinuityFeedbackEventRow 是 append-only feedback/weekly-review 事件。
// 用户改选 outcome 时追加 superseding event，旧 event 保留供本地审计。
type ContinuityFeedbackEventRow struct {
	FeedbackID           string    `gorm:"primaryKey;column:feedback_id" json:"feedback_id"`
	RunID                string    `gorm:"index;column:run_id" json:"run_id,omitempty"`
	EventKind            string    `gorm:"index;column:event_kind" json:"event_kind"` // outcome | weekly_review
	Outcome              string    `gorm:"column:outcome" json:"outcome,omitempty"`
	ReviewSeconds        *int      `gorm:"column:review_seconds" json:"review_seconds,omitempty"`
	SubmittedAt          time.Time `gorm:"index;column:submitted_at" json:"submitted_at"`
	SupersedesFeedbackID string    `gorm:"column:supersedes_feedback_id" json:"supersedes_feedback_id,omitempty"`
}

// TableName 固定 append-only 表名。
func (ContinuityFeedbackEventRow) TableName() string { return "continuity_feedback_events" }

// EventKind 枚举。
const (
	EventKindOutcome      = "outcome"
	EventKindWeeklyReview = "weekly_review"
)

// Store 是 continuity evidence 的 GORM repository。
// 只在显式 record/feedback/report 路径打开；默认 continue 不触发 migrate。
type Store struct {
	db *gorm.DB
}

// Open 在既有 agent memory DB 连接上 migrate additive 表。
// 迁移幂等（GORM AutoMigrate），旧 DB 无需人工操作。
func Open(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("continuity evidence store requires an open db")
	}
	if err := db.AutoMigrate(&ContinuityRunRow{}, &ContinuityFeedbackEventRow{}); err != nil {
		return nil, fmt.Errorf("migrate continuity evidence tables: %w", err)
	}
	return &Store{db: db}, nil
}

// CreateRun 写入一条 run receipt。枚举字段必须在调用前校验（fail before write）。
func (s *Store) CreateRun(ctx context.Context, run ContinuityRunRow) error {
	if run.RunID == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "run_id is required")
	}
	if run.Runtime == "" || run.TaskClass == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "runtime and task_class are required for a recorded run")
	}
	if !ValidTaskClass(run.TaskClass) {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "unknown task_class: "+run.TaskClass)
	}
	return s.db.WithContext(ctx).Create(&run).Error
}

// GetRun 读取一条 run receipt。
func (s *Store) GetRun(ctx context.Context, runID string) (ContinuityRunRow, error) {
	var row ContinuityRunRow
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).First(&row).Error; err != nil {
		return ContinuityRunRow{}, err
	}
	return row, nil
}

// MarkCheckpoint 把显式 checkpoint link 写回既有 run receipt。
// 单条 GORM UPDATE 同时记录 checkpoint 时间与实际 proposal 数；run 不存在时
// 返回 record-not-found，调用方不得把未关联 handoff 伪装成 completed link。
func (s *Store) MarkCheckpoint(ctx context.Context, runID string, checkpointedAt time.Time, proposalCount int) error {
	if runID == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "run_id is required")
	}
	if proposalCount < 0 {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "proposal_count must be non-negative")
	}
	result := s.db.WithContext(ctx).
		Model(&ContinuityRunRow{}).
		Where(&ContinuityRunRow{RunID: runID}).
		Select("CheckpointedAt", "ProposalCount").
		Updates(&ContinuityRunRow{CheckpointedAt: &checkpointedAt, ProposalCount: proposalCount})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AppendFeedback 追加一条 feedback event（outcome 或 weekly_review）。
// supersession 通过 SupersedesFeedbackID 链表达；不原地覆盖。
func (s *Store) AppendFeedback(ctx context.Context, event ContinuityFeedbackEventRow) error {
	if event.FeedbackID == "" {
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "feedback_id is required")
	}
	switch event.EventKind {
	case EventKindOutcome:
		if event.RunID == "" {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "run_id is required for outcome feedback")
		}
		if !ValidOutcome(event.Outcome) {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "unknown outcome: "+event.Outcome)
		}
	case EventKindWeeklyReview:
		if event.ReviewSeconds == nil || *event.ReviewSeconds < 0 {
			return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "review_seconds must be a non-negative integer")
		}
	default:
		return agentprotocol.NewStableError(agentprotocol.ErrCodeValidationFailed, "unknown event_kind: "+event.EventKind)
	}
	return s.db.WithContext(ctx).Create(&event).Error
}

// LatestOutcome 按 run 折叠 append-only 事件，取每个 run 最新有效 outcome。
// 折叠规则确定性：按 submitted_at 升序回放，supersede 链中最后一条胜出；
// 相同时间戳按 feedback_id 字典序保证稳定。
func LatestOutcome(events []ContinuityFeedbackEventRow) map[string]Outcome {
	byRun := map[string][]ContinuityFeedbackEventRow{}
	for _, event := range events {
		if event.EventKind != EventKindOutcome {
			continue
		}
		byRun[event.RunID] = append(byRun[event.RunID], event)
	}
	latest := map[string]Outcome{}
	for runID, runEvents := range byRun {
		best := runEvents[0]
		for _, event := range runEvents[1:] {
			if event.SubmittedAt.After(best.SubmittedAt) ||
				(event.SubmittedAt.Equal(best.SubmittedAt) && event.FeedbackID > best.FeedbackID) {
				best = event
			}
		}
		latest[runID] = Outcome(best.Outcome)
	}
	return latest
}

// ListRunsSince 返回窗口内的 run receipt。
func (s *Store) ListRunsSince(ctx context.Context, since time.Time) ([]ContinuityRunRow, error) {
	var rows []ContinuityRunRow
	if err := s.db.WithContext(ctx).Where("started_at >= ?", since).Order("started_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListRunsBetween 返回闭区间 [since, until] 内的 run receipt。
func (s *Store) ListRunsBetween(ctx context.Context, since, until time.Time) ([]ContinuityRunRow, error) {
	var rows []ContinuityRunRow
	if err := s.db.WithContext(ctx).
		Where("started_at >= ? AND started_at <= ?", since, until).
		Order("started_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListFeedbackSince 返回窗口内提交的 feedback event。
func (s *Store) ListFeedbackSince(ctx context.Context, since time.Time) ([]ContinuityFeedbackEventRow, error) {
	var rows []ContinuityFeedbackEventRow
	if err := s.db.WithContext(ctx).Where("submitted_at >= ?", since).Order("submitted_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListFeedbackBetween 返回闭区间 [since, until] 内提交的 feedback event。
func (s *Store) ListFeedbackBetween(ctx context.Context, since, until time.Time) ([]ContinuityFeedbackEventRow, error) {
	var rows []ContinuityFeedbackEventRow
	if err := s.db.WithContext(ctx).
		Where("submitted_at >= ? AND submitted_at <= ?", since, until).
		Order("submitted_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
