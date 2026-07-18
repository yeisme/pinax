package agentmemory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// seedMemories 填充 n 条 memory 到给定 scope。
func seedMemories(t testing.TB, s *Store, scope agentprotocol.Scope, n int) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		state := agentprotocol.LifecycleConfirmed
		if i%5 == 0 {
			state = agentprotocol.LifecycleProposed
		}
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            fmt.Sprintf("bench_%d", i),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         state,
			Subject:       fmt.Sprintf("subject %d GORM benchmark", i),
			Summary:       fmt.Sprintf("summary text %d", i),
			CreatorID:     "p_bench",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.SaveMemory(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
}

// BenchmarkMemoryRecallAll 验证 10k memories 下 default recall 不做全表 scan 后的二次处理。
// Recall 直接用 indexed scope/state filter，返回只取 confirmed/conflicted。
func BenchmarkMemoryRecallAll(b *testing.B) {
	s := testStore(b)
	defer func() { _ = s.Close() }()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_bench"}
	seedMemories(b, s, scope, 10000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := s.Recall(ctx, RecallQuery{Scope: scope, Limit: 50})
		if err != nil {
			b.Fatal(err)
		}
		if len(results) == 0 {
			b.Fatal("expected results")
		}
	}
}

// BenchmarkMemoryRecallText 验证 text search 在 10k memories 下的性能。
func BenchmarkMemoryRecallText(b *testing.B) {
	s := testStore(b)
	defer func() { _ = s.Close() }()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_bench_text"}
	seedMemories(b, s, scope, 10000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Recall(ctx, RecallQuery{Scope: scope, Text: "GORM", Limit: 50})
		if err != nil {
			b.Fatal(err)
		}
	}
}
