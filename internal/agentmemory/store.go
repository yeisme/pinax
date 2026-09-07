package agentmemory

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
	"github.com/yeisme/pinax/internal/sqlitedsn"
	"gorm.io/gorm"
)

// Store 是 Agent memory runtime 的持久化仓库。
// 它管理自己的 `agent_memory.sqlite` 数据库，不直接修改旧 `ledger.sqlite`。
// 旧 memory rows 通过 LegacyView 映射读取，不静默 backfill。
type Store struct {
	db *gorm.DB
}

// Open 打开或创建指定 vault root 下的 agent memory store。
func Open(root string) (*Store, error) {
	dir := filepath.Join(root, ".pinax", "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent memory dir: %w", err)
	}
	db, err := sqlitedsn.Open(filepath.Join(dir, "agent_memory.sqlite"))
	if err != nil {
		return nil, fmt.Errorf("open agent memory store: %w", err)
	}
	s := &Store{db: db}
	if err := s.autoMigrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// OpenDB 从已有 gorm.DB 构造 store（用于测试和共享连接）。
func OpenDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.autoMigrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) autoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&AgentMemoryRow{},
		&AgentSourceRow{},
		&AgentConflictRow{},
		&AgentPrincipalRow{},
		&AgentProposalRow{},
		&AgentProposalSourceRow{},
		&AgentHandoffRow{},
		&AgentHandoffSourceRow{},
		&AgentFeedbackRow{},
	)
}

// DB 返回底层 gorm.DB（用于高级查询和测试）。
func (s *Store) DB() *gorm.DB { return s.db }

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// SaveMemory 在事务中写入 memory record 及其 sources 和 conflicts。
func (s *Store) SaveMemory(ctx context.Context, m agentprotocol.MemoryRecord) error {
	if err := m.Validate(); err != nil {
		return err
	}
	row := FromProtocol(m)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return fmt.Errorf("save memory row: %w", err)
		}
		// 替换 sources
		if err := tx.Where("memory_id = ?", m.ID).Delete(&AgentSourceRow{}).Error; err != nil {
			return fmt.Errorf("clear sources: %w", err)
		}
		for _, src := range m.Sources {
			srcRow := AgentSourceRow{
				ID:        sourceID(m.ID, src),
				MemoryID:  m.ID,
				Kind:      src.Kind,
				Ref:       src.Ref,
				Label:     src.Label,
				Span:      src.Span,
				CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&srcRow).Error; err != nil {
				return fmt.Errorf("save source: %w", err)
			}
		}
		// 替换 conflicts
		if err := tx.Where("memory_id = ?", m.ID).Delete(&AgentConflictRow{}).Error; err != nil {
			return fmt.Errorf("clear conflicts: %w", err)
		}
		for _, cid := range m.ConflictsWith {
			cRow := AgentConflictRow{
				ID:            conflictID(m.ID, cid),
				MemoryID:      m.ID,
				ConflictsWith: cid,
				CreatedAt:     time.Now().UTC(),
			}
			if err := tx.Create(&cRow).Error; err != nil {
				return fmt.Errorf("save conflict: %w", err)
			}
		}
		return nil
	})
}

// GetMemory 读取单个 memory record（含 sources 和 conflicts）。
func (s *Store) GetMemory(ctx context.Context, id string) (agentprotocol.MemoryRecord, error) {
	var row AgentMemoryRow
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return agentprotocol.MemoryRecord{}, fmt.Errorf("get memory %s: %w", id, err)
	}
	sources, err := s.getSources(ctx, id)
	if err != nil {
		return agentprotocol.MemoryRecord{}, err
	}
	conflicts, err := s.getConflicts(ctx, id)
	if err != nil {
		return agentprotocol.MemoryRecord{}, err
	}
	return row.ToProtocol(sources, conflicts), nil
}

// ListMemories 按 scope 列出 memory（可选 state 过滤）。
func (s *Store) ListMemories(ctx context.Context, scope agentprotocol.Scope, states ...agentprotocol.LifecycleState) ([]agentprotocol.MemoryRecord, error) {
	q := s.db.WithContext(ctx).Where("scope_kind = ? AND scope_id = ?", scope.Kind, scope.ID)
	if len(states) > 0 {
		stateStrs := make([]string, len(states))
		for i, st := range states {
			stateStrs[i] = string(st)
		}
		q = q.Where("state IN ?", stateStrs)
	}
	var rows []AgentMemoryRow
	if err := q.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	result := make([]agentprotocol.MemoryRecord, 0, len(rows))
	for _, row := range rows {
		sources, err := s.getSources(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		conflicts, err := s.getConflicts(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, row.ToProtocol(sources, conflicts))
	}
	return result, nil
}

func (s *Store) getSources(ctx context.Context, memoryID string) ([]agentprotocol.SourceRef, error) {
	var rows []AgentSourceRow
	if err := s.db.WithContext(ctx).Where("memory_id = ?", memoryID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get sources: %w", err)
	}
	refs := make([]agentprotocol.SourceRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, agentprotocol.SourceRef{Kind: r.Kind, Ref: r.Ref, Label: r.Label, Span: r.Span})
	}
	return refs, nil
}

func (s *Store) getConflicts(ctx context.Context, memoryID string) ([]string, error) {
	var rows []AgentConflictRow
	if err := s.db.WithContext(ctx).Where("memory_id = ?", memoryID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get conflicts: %w", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ConflictsWith)
	}
	return ids, nil
}

// SavePrincipal 注册或更新 principal。
func (s *Store) SavePrincipal(ctx context.Context, p agentprotocol.Principal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	caps := make([]string, 0, len(p.Capabilities))
	for _, c := range p.Capabilities {
		caps = append(caps, string(c))
	}
	row := AgentPrincipalRow{
		PrincipalID:  p.PrincipalID,
		Runtime:      p.Runtime,
		AgentID:      p.AgentID,
		OwnerID:      p.OwnerID,
		WorkspaceID:  p.WorkspaceID,
		Trust:        string(p.Trust),
		Capabilities: strings.Join(caps, ","),
		UpdatedAt:    time.Now().UTC(),
	}
	var existing AgentPrincipalRow
	err := s.db.WithContext(ctx).First(&existing, "principal_id = ?", p.PrincipalID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.CreatedAt = time.Now().UTC()
		return s.db.WithContext(ctx).Create(&row).Error
	}
	if err != nil {
		return err
	}
	row.CreatedAt = existing.CreatedAt
	return s.db.WithContext(ctx).Save(&row).Error
}

// SaveProposal 保存 proposal。
func (s *Store) SaveProposal(ctx context.Context, p agentprotocol.Proposal, status agentprotocol.ProposalStatus, reviewReason agentprotocol.ProposalStatusReason) error {
	row := AgentProposalRow{
		ProposalID:     p.ProposalID,
		PrincipalID:    p.Principal.PrincipalID,
		ScopeKind:      string(p.Scope.Kind),
		ScopeID:        p.Scope.ID,
		Kind:           string(p.Kind),
		Subject:        p.Subject,
		Summary:        p.Summary,
		Object:         p.Object,
		RequestedState: string(p.RequestedState),
		Risk:           string(p.Risk),
		Reason:         p.Reason,
		Status:         string(status),
		ReviewReason:   string(reviewReason),
		CreatedAt:      p.CreatedAt,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return fmt.Errorf("save proposal row: %w", err)
		}
		if err := tx.Where("proposal_id = ?", p.ProposalID).Delete(&AgentProposalSourceRow{}).Error; err != nil {
			return fmt.Errorf("clear proposal sources: %w", err)
		}
		for _, source := range p.Sources {
			sourceRow := AgentProposalSourceRow{
				ID:         sourceID(p.ProposalID, source),
				ProposalID: p.ProposalID,
				Kind:       source.Kind,
				Ref:        source.Ref,
				Label:      source.Label,
				Span:       source.Span,
				CreatedAt:  time.Now().UTC(),
			}
			if err := tx.Create(&sourceRow).Error; err != nil {
				return fmt.Errorf("save proposal source: %w", err)
			}
		}
		return nil
	})
}

// GetProposalSources 返回 proposal 的 bounded refs；不读取或返回 proposal
// 正文。顺序按写入时间稳定。
func (s *Store) GetProposalSources(ctx context.Context, proposalID string) (agentprotocol.SourceRefList, error) {
	var rows []AgentProposalSourceRow
	if err := s.db.WithContext(ctx).Where("proposal_id = ?", proposalID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get proposal sources: %w", err)
	}
	sources := make(agentprotocol.SourceRefList, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, agentprotocol.SourceRef{Kind: row.Kind, Ref: row.Ref, Label: row.Label, Span: row.Span})
	}
	return sources, nil
}

// UpdateProposalStatus 更新 proposal 的 review 结果。
func (s *Store) UpdateProposalStatus(ctx context.Context, proposalID string, status agentprotocol.ProposalStatus, reviewReason agentprotocol.ProposalStatusReason, resultingMemoryID string) error {
	updates := map[string]any{
		"status":              string(status),
		"review_reason":       string(reviewReason),
		"resulting_memory_id": resultingMemoryID,
		"reviewed_at":         time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Model(&AgentProposalRow{}).Where("proposal_id = ?", proposalID).Updates(updates).Error
}

// UpdateProposalStatusFrom 是带状态前置条件的原子 review 结果更新：
// 只有 proposal 仍处于 from 集合之一时才写入。RowsAffected != 1 说明
// proposal 已被并发 action 消费（approve 与 reject 竞态），返回 false，
// 调用方必须以 stale action 失败，不得假成功。
func (s *Store) UpdateProposalStatusFrom(ctx context.Context, proposalID string, from []agentprotocol.ProposalStatus, status agentprotocol.ProposalStatus, reviewReason agentprotocol.ProposalStatusReason, resultingMemoryID string) (bool, error) {
	fromValues := make([]string, 0, len(from))
	for _, value := range from {
		fromValues = append(fromValues, string(value))
	}
	updates := map[string]any{
		"status":              string(status),
		"review_reason":       string(reviewReason),
		"resulting_memory_id": resultingMemoryID,
		"reviewed_at":         time.Now().UTC(),
	}
	result := s.db.WithContext(ctx).Model(&AgentProposalRow{}).
		Where("proposal_id = ?", proposalID).
		Where("status IN ?", fromValues).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ListProposals 按 scope 列出 proposal。
func (s *Store) ListProposals(ctx context.Context, scope agentprotocol.Scope) ([]AgentProposalRow, error) {
	var rows []AgentProposalRow
	if err := s.db.WithContext(ctx).Where("scope_kind = ? AND scope_id = ?", scope.Kind, scope.ID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetHandoff 按 ID 读取一条 handoff 及其 source refs。
func (s *Store) GetHandoff(ctx context.Context, handoffID string) (AgentHandoffRow, error) {
	var row AgentHandoffRow
	if err := s.db.WithContext(ctx).Where("handoff_id = ?", handoffID).First(&row).Error; err != nil {
		return AgentHandoffRow{}, err
	}
	var sourceRows []AgentHandoffSourceRow
	if err := s.db.WithContext(ctx).Where("handoff_id = ?", handoffID).Order("created_at ASC").Find(&sourceRows).Error; err != nil {
		return AgentHandoffRow{}, err
	}
	for _, sourceRow := range sourceRows {
		row.Sources = append(row.Sources, agentprotocol.SourceRef{
			Kind: sourceRow.Kind, Ref: sourceRow.Ref, Label: sourceRow.Label, Span: sourceRow.Span,
		})
	}
	return row, nil
}

// SaveHandoff 保存 handoff。
func (s *Store) SaveHandoff(ctx context.Context, h agentprotocol.Handoff) error {
	row := AgentHandoffRow{
		HandoffID:               h.HandoffID,
		FromPrincipal:           h.FromPrincipal.PrincipalID,
		ToPrincipal:             h.ToPrincipal.PrincipalID,
		ScopeKind:               string(h.Scope.Kind),
		ScopeID:                 h.Scope.ID,
		Objective:               h.Objective,
		CurrentState:            h.CurrentState,
		Decisions:               strings.Join(h.Decisions, "\n"),
		CompletedWork:           strings.Join(h.CompletedWork, "\n"),
		Blockers:                strings.Join(h.Blockers, "\n"),
		Verification:            strings.Join(h.Verification, "\n"),
		FollowUps:               strings.Join(h.FollowUps, "\n"),
		RequestedNextCapability: h.RequestedNextCapability,
		CreatedAt:               h.CreatedAt,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("save handoff row: %w", err)
		}
		for _, src := range h.Sources {
			sourceRow := AgentHandoffSourceRow{
				ID:        sourceID(h.HandoffID, src),
				HandoffID: h.HandoffID,
				Kind:      src.Kind,
				Ref:       src.Ref,
				Label:     src.Label,
				Span:      src.Span,
				CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&sourceRow).Error; err != nil {
				return fmt.Errorf("save handoff source: %w", err)
			}
		}
		return nil
	})
}

// ListHandoffs 按 scope 列出 handoff。
func (s *Store) ListHandoffs(ctx context.Context, scope agentprotocol.Scope) ([]AgentHandoffRow, error) {
	var rows []AgentHandoffRow
	if err := s.db.WithContext(ctx).Where("scope_kind = ? AND scope_id = ?", scope.Kind, scope.ID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return rows, nil
	}
	handoffIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		handoffIDs = append(handoffIDs, row.HandoffID)
	}
	var sourceRows []AgentHandoffSourceRow
	if err := s.db.WithContext(ctx).Where("handoff_id IN ?", handoffIDs).Order("created_at ASC").Find(&sourceRows).Error; err != nil {
		return nil, err
	}
	sourcesByHandoff := make(map[string]agentprotocol.SourceRefList, len(rows))
	for _, sourceRow := range sourceRows {
		sourcesByHandoff[sourceRow.HandoffID] = append(sourcesByHandoff[sourceRow.HandoffID], agentprotocol.SourceRef{
			Kind: sourceRow.Kind, Ref: sourceRow.Ref, Label: sourceRow.Label, Span: sourceRow.Span,
		})
	}
	for i := range rows {
		rows[i].Sources = sourcesByHandoff[rows[i].HandoffID]
	}
	return rows, nil
}

// SaveFeedback 保存 feedback。
func (s *Store) SaveFeedback(ctx context.Context, f agentprotocol.Feedback) error {
	if err := f.Validate(); err != nil {
		return err
	}
	row := AgentFeedbackRow{
		FeedbackID:        f.FeedbackID,
		PrincipalID:       f.Principal.PrincipalID,
		ScopeKind:         string(f.Scope.Kind),
		ScopeID:           f.Scope.ID,
		Kind:              string(f.Kind),
		MemoryID:          f.MemoryID,
		ContextRequestRef: f.ContextRequestRef,
		Comment:           f.Comment,
		CreatedAt:         f.CreatedAt,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

// ListFeedback 按 scope 列出 feedback。
func (s *Store) ListFeedback(ctx context.Context, scope agentprotocol.Scope) ([]AgentFeedbackRow, error) {
	var rows []AgentFeedbackRow
	if err := s.db.WithContext(ctx).Where("scope_kind = ? AND scope_id = ?", scope.Kind, scope.ID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// sourceID 生成 source row 的确定性 ID。
func sourceID(memoryID string, src agentprotocol.SourceRef) string {
	h := sha1.Sum([]byte(memoryID + "|" + src.Kind + "|" + src.Ref))
	return "src_" + hex.EncodeToString(h[:])
}

// conflictID 生成 conflict row 的确定性 ID。
func conflictID(memoryID, otherID string) string {
	h := sha1.Sum([]byte(memoryID + "|" + otherID))
	return "conf_" + hex.EncodeToString(h[:])
}
