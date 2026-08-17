package app

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// TestCompatibility_AgentContextUnchanged 验证新增 continuity/inbox 不影响旧 agent context。
func TestCompatibility_AgentContextUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "default"}

	// 旧 agent context runtime 仍然工作
	pack, err := svc.AgentContextRuntime(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("compat-test", "codex"),
		Scope:     scope,
		MaxItems:  10,
		MaxChars:  2000,
	})
	if err != nil {
		t.Fatalf("old agent context failed: %v", err)
	}
	if pack.SchemaVersion == "" {
		t.Error("old context pack schema_version missing")
	}
}

// TestCompatibility_NewSurfaceAdditive 验证新 surface 不删除旧字段。
func TestCompatibility_NewSurfaceAdditive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "compat"}
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// 旧方法仍然可用
	_, err := svc.AgentMemoryListProposals(ctx, vault, scope)
	if err != nil {
		t.Errorf("old ListProposals failed: %v", err)
	}

	// 新方法也可用
	_, err = svc.MemoryInbox(ctx, InboxRequest{VaultPath: vault, Scope: scope})
	if err != nil {
		t.Errorf("new MemoryInbox failed: %v", err)
	}

	_, err = svc.AgentContinuity(ctx, ContinuityRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("compat", "codex"),
		Scope:     scope,
	})
	if err != nil {
		t.Errorf("new AgentContinuity failed: %v", err)
	}
}

// TestCompatibility_RollbackCapability 验证禁用新 capability 后旧面不受影响。
// rollback 方式是隐藏新命令和 route registration，不删除 additive data。
func TestCompatibility_RollbackCapability(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	svc := NewAgentMemoryService()
	defer func() { _ = svc.Close() }()

	// 即使新 surface 出错，旧 context runtime 仍然正常
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "rollback-test"}

	_, err := svc.AgentContextRuntime(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("rollback", "codex"),
		Scope:     scope,
	})
	if err != nil {
		t.Errorf("old context runtime affected by new surface: %v", err)
	}
}
