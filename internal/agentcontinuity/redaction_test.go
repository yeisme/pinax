package agentcontinuity

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/agentcontext"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// forbiddenPatterns 是所有 projection 中不应出现的敏感模式。
var forbiddenPatterns = []string{
	"Authorization:", "Cookie:", "Bearer ", "api_key=", "password=",
	"-----BEGIN", "PRIVATE KEY", "raw_prompt", "provider_payload",
	"chain-of-thought", "<system>", "session_id=",
}

// scanForForbidden 扫描字符串中的敏感模式。
func scanForForbidden(t *testing.T, label, content string) {
	t.Helper()
	for _, pat := range forbiddenPatterns {
		if strings.Contains(strings.ToLower(content), strings.ToLower(pat)) {
			t.Errorf("%s contains forbidden pattern %q", label, pat)
		}
	}
}

func TestContinuityPack_NoSecretLeak(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	st, err := agentmemory.Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "redaction-test"}

	// 保存一条 memory，其 subject 包含看似敏感的内容
	mem := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "secret-test-mem",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         scope,
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       "API endpoint for authentication",
		Summary:       "The auth endpoint uses Bearer tokens",
		CreatorID:     "test-agent",
		Sources:       agentprotocol.SourceRefList{{Kind: "note", Ref: "auth-note"}},
	}
	if err := st.SaveMemory(ctx, mem); err != nil {
		t.Fatal(err)
	}

	compiler := agentcontext.NewCompiler(st, agentmemory.DefaultPolicy())
	orch := NewOrchestrator(st, compiler)
	pack, err := orch.Compile(ctx, ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("test", "codex"),
		Scope:         scope,
		Budget:        DefaultBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 扫描 sections 不包含 forbidden patterns
	for _, section := range pack.Sections {
		for _, item := range section.Items {
			scanForForbidden(t, "section item", item)
		}
	}

	// 扫描 objective 和 current state
	scanForForbidden(t, "objective", pack.Objective)
	scanForForbidden(t, "current_state", pack.CurrentState)
}

func TestContinuityPack_NoRawPrompt(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	st, err := agentmemory.Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "prompt-test"}

	// 保存一条 memory，其内容模拟 raw prompt（不应该出现在 projection 中）
	mem := agentprotocol.MemoryRecord{
		SchemaVersion: agentprotocol.SchemaVersion,
		ID:            "prompt-leak-test",
		Kind:          agentprotocol.MemoryKindFact,
		Scope:         scope,
		State:         agentprotocol.LifecycleConfirmed,
		Subject:       "System configuration",
		Summary:       "Configured with system prompt and chain-of-thought reasoning",
		CreatorID:     "test-agent",
	}
	if err := st.SaveMemory(ctx, mem); err != nil {
		t.Fatal(err)
	}

	compiler := agentcontext.NewCompiler(st, agentmemory.DefaultPolicy())
	orch := NewOrchestrator(st, compiler)
	pack, err := orch.Compile(ctx, ContinuityRequest{
		SchemaVersion: ContinuitySchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("test", "codex"),
		Scope:         scope,
		Budget:        DefaultBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, section := range pack.Sections {
		for _, item := range section.Items {
			scanForForbidden(t, "section item", item)
		}
	}
}

func TestContinuityPack_AssertNoBody_Contract(t *testing.T) {
	// 合同检查：continuity pack 必须通过 body safety check
	pack := ContinuityPack{
		SchemaVersion: ContinuitySchemaVersion,
		Sections: []ContinuitySection{
			{Kind: "fact", Items: []string{"short bounded item"}},
		},
	}
	if err := pack.AssertNoBody(500); err != nil {
		t.Errorf("valid pack failed body check: %v", err)
	}

	// 长 item 应该被拦截
	longItem := strings.Repeat("x", 600)
	pack2 := ContinuityPack{
		Sections: []ContinuitySection{
			{Kind: "fact", Items: []string{longItem}},
		},
	}
	if err := pack2.AssertNoBody(500); err == nil {
		t.Error("expected body leak detection for long item")
	}
}
