package agentcontinuity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// BenchmarkContinuityCompile 验证 continuity compilation 在 10k memories 下
// 不会产生每请求全 vault scan。
func BenchmarkContinuityCompile(b *testing.B) {
	ctx := context.Background()
	vault := b.TempDir()
	st, err := agentmemory.Open(vault)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "bench"}

	// 预填充 10k memories
	for i := 0; i < 10000; i++ {
		mem := agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            fmt.Sprintf("bench-mem-%d", i),
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         agentprotocol.LifecycleConfirmed,
			Subject:       fmt.Sprintf("Fact number %d", i),
			CreatorID:     "bench-agent",
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
		if err := st.SaveMemory(ctx, mem); err != nil {
			b.Fatal(err)
		}
	}

	compiler := agentcontext.NewCompiler(st, agentmemory.DefaultPolicy())
	orch := NewOrchestrator(st, compiler)
	req := ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("bench", "codex"),
		Scope:         scope,
		Budget:        DefaultBudget(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := orch.Compile(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}
