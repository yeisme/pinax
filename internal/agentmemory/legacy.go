package agentmemory

import (
	"context"
	"strings"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// DefaultLocalPrincipalID 是旧 memory rows 的默认 principal。
// 旧 ledger 没有 principal 概念，映射时统一归到 local owner。
const DefaultLocalPrincipalID = "local-owner"

// DefaultLocalScope 是旧 memory rows 的默认 scope。
// 旧 ledger 没有 scope 概念，映射时统一归到 workspace scope。
func DefaultLocalScope(workspaceID string) agentprotocol.Scope {
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = "default"
	}
	return agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: workspaceID}
}

// LegacyRecord 是旧 internal/memory.Record 的最小投影。
// 用于在不引入 import cycle 的情况下映射旧 row。
type LegacyRecord struct {
	ID           string
	Type         string
	Subject      string
	Predicate    string
	Object       string
	Body         string
	Status       string
	Confidence   string
	SourceURI    string
	SupersedesID string
}

// LegacyKindMap 将旧 Type 映射到新 MemoryKind。
// 未知 type 映射为 fact（向前兼容），不失败。
func LegacyKindMap(oldType string) agentprotocol.MemoryKind {
	switch strings.ToLower(strings.TrimSpace(oldType)) {
	case "fact":
		return agentprotocol.MemoryKindFact
	case "decision":
		return agentprotocol.MemoryKindDecision
	case "event":
		return agentprotocol.MemoryKindEvent
	case "task":
		return agentprotocol.MemoryKindTask
	case "preference":
		return agentprotocol.MemoryKindPreference
	case "procedure":
		return agentprotocol.MemoryKindProcedure
	case "failure":
		return agentprotocol.MemoryKindFailure
	default:
		return agentprotocol.MemoryKindFact
	}
}

// LegacyStateMap 将旧 Status 映射到新 LifecycleState。
func LegacyStateMap(oldStatus string) agentprotocol.LifecycleState {
	switch strings.ToLower(strings.TrimSpace(oldStatus)) {
	case "confirmed":
		return agentprotocol.LifecycleConfirmed
	case "superseded":
		return agentprotocol.LifecycleSuperseded
	case "expired":
		return agentprotocol.LifecycleExpired
	case "rejected":
		return agentprotocol.LifecycleRejected
	case "draft":
		return agentprotocol.LifecycleProposed
	default:
		return agentprotocol.LifecycleProposed
	}
}

// MapLegacyRecord 将一条旧 memory row 映射为新 protocol MemoryRecord。
// 不写回数据库——这只是 read mapping。
// legacyID 映射到 LegacyRecordID，保持双向追溯。
func MapLegacyRecord(rec LegacyRecord, workspaceID string) agentprotocol.MemoryRecord {
	scope := DefaultLocalScope(workspaceID)
	m := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            rec.ID, // 保持原 ID 不变
		Kind:          LegacyKindMap(rec.Type),
		Scope:         scope,
		State:         LegacyStateMap(rec.Status),
		Subject:       rec.Subject,
		Predicate:     rec.Predicate,
		Object:        rec.Object,
		CreatorID:     DefaultLocalPrincipalID,
		SupersedesID:  rec.SupersedesID,
	}
	if rec.SourceURI != "" {
		m.Sources = agentprotocol.SourceRefList{
			{Kind: "legacy", Ref: rec.SourceURI},
		}
	}
	return m
}

// EnsureLegacyViewed 将旧 records 的 protocol 视图映射为 list，供 read path 使用。
// 不静默 backfill 到 agent_memory_records 表。
func EnsureLegacyViewed(ctx context.Context, s *Store, records []LegacyRecord, workspaceID string) ([]agentprotocol.MemoryRecord, error) {
	_ = ctx
	_ = s
	result := make([]agentprotocol.MemoryRecord, 0, len(records))
	for _, rec := range records {
		result = append(result, MapLegacyRecord(rec, workspaceID))
	}
	return result, nil
}
