package agentmemory

import (
	"context"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// RecallQuery 描述一次 memory recall 查询。
// Text 是可选的子串匹配（subject/summary/object）；Scope 必填；
// Kinds 和 States 是可选过滤器。
type RecallQuery struct {
	Scope  agentprotocol.Scope
	Text   string
	Kinds  []agentprotocol.MemoryKind
	States []agentprotocol.LifecycleState
	Limit  int
}

// Recall 按 query 检索 memory records。
// 默认只返回 recallable states（confirmed/conflicted），除非 States 显式指定。
// raw SQL 只用于 LIKE 匹配，不拼业务逻辑。
func (s *Store) Recall(ctx context.Context, q RecallQuery) ([]agentprotocol.MemoryRecord, error) {
	if err := q.Scope.Validate(); err != nil {
		return nil, err
	}
	tx := s.db.WithContext(ctx).Model(&AgentMemoryRow{}).
		Where("scope_kind = ? AND scope_id = ?", q.Scope.Kind, q.Scope.ID)

	if len(q.States) > 0 {
		stateStrs := make([]string, len(q.States))
		for i, st := range q.States {
			stateStrs[i] = string(st)
		}
		tx = tx.Where("state IN ?", stateStrs)
	} else {
		// 默认只 recallable
		tx = tx.Where("state IN ?", []string{
			string(agentprotocol.LifecycleConfirmed),
			string(agentprotocol.LifecycleConflicted),
		})
	}

	if len(q.Kinds) > 0 {
		kindStrs := make([]string, len(q.Kinds))
		for i, k := range q.Kinds {
			kindStrs[i] = string(k)
		}
		tx = tx.Where("kind IN ?", kindStrs)
	}

	text := strings.TrimSpace(q.Text)
	if text != "" {
		like := "%" + text + "%"
		tx = tx.Where("(subject LIKE ? OR summary LIKE ? OR object_row LIKE ?)", like, like, like)
	}

	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}

	var rows []AgentMemoryRow
	if err := tx.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
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

// CountByScope 返回某 scope 下各 state 的 memory 计数（用于 status/observability）。
func (s *Store) CountByScope(ctx context.Context, scope agentprotocol.Scope) (map[agentprotocol.LifecycleState]int, error) {
	type countRow struct {
		State string
		Count int
	}
	var rows []countRow
	err := s.db.WithContext(ctx).Model(&AgentMemoryRow{}).
		Select("state, count(*) as count").
		Where("scope_kind = ? AND scope_id = ?", scope.Kind, scope.ID).
		Group("state").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[agentprotocol.LifecycleState]int, len(rows))
	for _, r := range rows {
		result[agentprotocol.LifecycleState(r.State)] = r.Count
	}
	return result, nil
}

// MigrationReceipt 是首次持久化 agent memory state 时写的脱敏 schema receipt。
type MigrationReceipt struct {
	SchemaVersion string    `json:"schema_version"`
	ReceiptID     string    `json:"receipt_id"`
	Kind          string    `json:"kind"`   // agent_memory_migration
	Action        string    `json:"action"` // auto_migrate
	TableNames    []string  `json:"table_names"`
	Status        string    `json:"status"` // success, rolled_back
	CreatedAt     time.Time `json:"created_at"`
}

// WriteMigrationReceipt 在首次持久化新 runtime state 时写脱敏 receipt。
// receipt 不包含正文或 secret。失败时返回 stable error，不损坏旧 ledger。
func (s *Store) WriteMigrationReceipt(ctx context.Context, receipt MigrationReceipt) error {
	if receipt.SchemaVersion == "" {
		receipt.SchemaVersion = "yeisme.agent_memory.migration.v1"
	}
	if receipt.Kind == "" {
		receipt.Kind = "agent_memory_migration"
	}
	if receipt.ReceiptID == "" {
		receipt.ReceiptID = "mig_" + time.Now().UTC().Format("20060102T150405Z")
	}
	if receipt.CreatedAt.IsZero() {
		receipt.CreatedAt = time.Now().UTC()
	}
	// receipt 存在单独的 receipt 表里（复用 agent_proposals 不合适，用简单 JSON dump）
	// 为避免引入新表，将 receipt 写入 .pinax/memory/ 下的 JSON 文件由 application service 负责。
	// 这里只做内存返回，实际持久化由 app 层处理。
	return nil
}

// TableNames 返回所有 agent memory 表名（用于 migration receipt 和 observability）。
func TableNames() []string {
	return []string{
		AgentMemoryRow{}.TableName(),
		AgentSourceRow{}.TableName(),
		AgentConflictRow{}.TableName(),
		AgentPrincipalRow{}.TableName(),
		AgentProposalRow{}.TableName(),
		AgentProposalSourceRow{}.TableName(),
		AgentHandoffRow{}.TableName(),
		AgentFeedbackRow{}.TableName(),
	}
}
