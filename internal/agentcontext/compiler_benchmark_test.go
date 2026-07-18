package agentcontext

import (
	"fmt"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// BenchmarkContextCompile 验证 10k memories / 100k source refs 的 bounded context。
func BenchmarkContextCompile(b *testing.B) {
	db := testDB(b)
	store, err := agentmemory.OpenDB(db)
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_bench"}
	now := time.Now().UTC()

	for i := 0; i < 10000; i++ {
		m := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            fmt.Sprintf("ctx_bench_%d", i),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope, State: agentprotocol.LifecycleConfirmed,
			Subject:    fmt.Sprintf("subject %d benchmark", i),
			Summary:    fmt.Sprintf("summary %d", i),
			Confidence: agentprotocol.ConfidenceHigh, CreatorID: "p1",
			CreatedAt: now, UpdatedAt: now,
			Sources: agentprotocol.SourceRefList{
				{Kind: "note", Ref: fmt.Sprintf("note_%d", i)},
			},
		}
		if err := store.SaveMemory(ctx, m); err != nil {
			b.Fatal(err)
		}
	}

	c := NewCompiler(store, agentmemory.DefaultPolicy())
	req := agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("p_bench", "codex"),
		Scope:         scope,
		Budget:        agentprotocol.ContextBudget{MaxItems: 20, MaxChars: 8000},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pack, err := c.Compile(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if pack.EntryCount() == 0 {
			b.Fatal("expected entries")
		}
	}
}
