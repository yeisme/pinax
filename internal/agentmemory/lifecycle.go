package agentmemory

import (
	"context"
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// LifecycleService 校验和执行 memory lifecycle 转换。
// 非法转换返回稳定 code；conflict 不静默覆盖。
type LifecycleService struct {
	store *Store
}

// NewLifecycleService 构造 lifecycle service。
func NewLifecycleService(s *Store) *LifecycleService {
	return &LifecycleService{store: s}
}

// ValidateTransition 检查从 from 到 to 的转换是否合法。
// 返回 nil 表示合法，否则返回 StableError。
func (ls *LifecycleService) ValidateTransition(from, to agentprotocol.LifecycleState) error {
	if !agentprotocol.ValidTransition(from, to) {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeValidationFailed,
			fmt.Sprintf("invalid lifecycle transition: %s → %s", from, to),
		)
	}
	return nil
}

// ConfirmProposed 将一条 proposed memory 转为 confirmed。
// 要求 source 非空（unsourced memory 不能自动 confirm）。
func (ls *LifecycleService) ConfirmProposed(ctx context.Context, memoryID string, now time.Time) (agentprotocol.MemoryRecord, error) {
	m, err := ls.store.GetMemory(ctx, memoryID)
	if err != nil {
		return agentprotocol.MemoryRecord{}, err
	}
	if err := ls.ValidateTransition(m.State, agentprotocol.LifecycleConfirmed); err != nil {
		return agentprotocol.MemoryRecord{}, err
	}
	// unsourced memory 不能 confirm（除非是 owner 显式操作，由 policy 层检查）
	if m.Sources.IsEmpty() {
		return agentprotocol.MemoryRecord{}, agentprotocol.NewStableError(
			agentprotocol.ErrCodeValidationFailed,
			"cannot confirm memory without sources",
		).WithDetail("memory_id", memoryID)
	}
	m.State = agentprotocol.LifecycleConfirmed
	m.UpdatedAt = now
	if err := ls.store.SaveMemory(ctx, m); err != nil {
		return agentprotocol.MemoryRecord{}, err
	}
	return m, nil
}

// Supersede 将一条 confirmed memory 标记为 superseded，由 replacement 接替。
// 旧 record 不被删除；replacement.SupersedesID 指向旧 record。
func (ls *LifecycleService) Supersede(ctx context.Context, oldID, replacementID string, now time.Time) error {
	old, err := ls.store.GetMemory(ctx, oldID)
	if err != nil {
		return err
	}
	if err := ls.ValidateTransition(old.State, agentprotocol.LifecycleSuperseded); err != nil {
		return err
	}
	old.State = agentprotocol.LifecycleSuperseded
	old.UpdatedAt = now
	if err := ls.store.SaveMemory(ctx, old); err != nil {
		return err
	}
	// replacement 的 SupersedesID 指向 old
	replacement, err := ls.store.GetMemory(ctx, replacementID)
	if err != nil {
		return err
	}
	replacement.SupersedesID = oldID
	replacement.UpdatedAt = now
	return ls.store.SaveMemory(ctx, replacement)
}

// Expire 将一条 memory 标记为 expired（基于 ExpiresAt 或显式操作）。
func (ls *LifecycleService) Expire(ctx context.Context, memoryID string, now time.Time) error {
	m, err := ls.store.GetMemory(ctx, memoryID)
	if err != nil {
		return err
	}
	if err := ls.ValidateTransition(m.State, agentprotocol.LifecycleExpired); err != nil {
		return err
	}
	m.State = agentprotocol.LifecycleExpired
	m.UpdatedAt = now
	return ls.store.SaveMemory(ctx, m)
}

// MarkConflicted 将一组 confirmed memory 标记为互相冲突。
// 冲突 memory 保持可见（recallable），不静默覆盖。
func (ls *LifecycleService) MarkConflicted(ctx context.Context, memoryIDs []string, reason string, now time.Time) error {
	if len(memoryIDs) < 2 {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeValidationFailed,
			"conflict requires at least 2 memories",
		)
	}
	for _, id := range memoryIDs {
		m, err := ls.store.GetMemory(ctx, id)
		if err != nil {
			return err
		}
		if err := ls.ValidateTransition(m.State, agentprotocol.LifecycleConflicted); err != nil {
			return err
		}
		m.State = agentprotocol.LifecycleConflicted
		m.UpdatedAt = now
		// 记录冲突关系
		for _, otherID := range memoryIDs {
			if otherID != id {
				m.ConflictsWith = append(m.ConflictsWith, otherID)
			}
		}
		if err := ls.store.SaveMemory(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// ResolveConflict 将一条 conflicted memory 转回 confirmed，
// 同时将其余冲突方标记为 superseded。
func (ls *LifecycleService) ResolveConflict(ctx context.Context, winnerID string, now time.Time) error {
	winner, err := ls.store.GetMemory(ctx, winnerID)
	if err != nil {
		return err
	}
	if winner.State != agentprotocol.LifecycleConflicted {
		return agentprotocol.NewStableError(
			agentprotocol.ErrCodeValidationFailed,
			fmt.Sprintf("memory %s is not conflicted (state=%s)", winnerID, winner.State),
		)
	}
	// winner → confirmed
	if err := ls.ValidateTransition(winner.State, agentprotocol.LifecycleConfirmed); err != nil {
		return err
	}
	winner.State = agentprotocol.LifecycleConfirmed
	winner.UpdatedAt = now
	if err := ls.store.SaveMemory(ctx, winner); err != nil {
		return err
	}
	// 其余冲突方 → superseded
	for _, otherID := range winner.ConflictsWith {
		other, err := ls.store.GetMemory(ctx, otherID)
		if err != nil {
			continue // 对方可能已被处理
		}
		if other.State == agentprotocol.LifecycleConflicted {
			if err := ls.ValidateTransition(other.State, agentprotocol.LifecycleSuperseded); err == nil {
				other.State = agentprotocol.LifecycleSuperseded
				other.UpdatedAt = now
				_ = ls.store.SaveMemory(ctx, other)
			}
		}
	}
	return nil
}

// RollbackSupersede 将一条 superseded memory 恢复为 confirmed（显式回滚）。
func (ls *LifecycleService) RollbackSupersede(ctx context.Context, supersededID string, now time.Time) error {
	m, err := ls.store.GetMemory(ctx, supersededID)
	if err != nil {
		return err
	}
	if err := ls.ValidateTransition(m.State, agentprotocol.LifecycleConfirmed); err != nil {
		return err
	}
	m.State = agentprotocol.LifecycleConfirmed
	m.SupersedesID = ""
	m.UpdatedAt = now
	return ls.store.SaveMemory(ctx, m)
}
