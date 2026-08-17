package api

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestRPCAgentContinuityRoute(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.Continuity",
		Params: map[string]any{"scope": "project:test", "task": "prepare release"},
	})
	if err != nil {
		t.Fatalf("agent continuity RPC: %v", err)
	}
	if projection.Command != "agent.continuity" {
		t.Errorf("command = %s, want agent.continuity", projection.Command)
	}
	if projection.Facts["schema_version"] == "" {
		t.Error("expected schema_version fact")
	}
	if projection.Facts["experimental"] != "true" {
		t.Error("expected experimental=true")
	}
}

func TestRPCAgentInboxRoute(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.Inbox",
		Params: map[string]any{"scope": "project:test"},
	})
	if err != nil {
		t.Fatalf("agent inbox RPC: %v", err)
	}
	if projection.Command != "agent.inbox" {
		t.Errorf("command = %s, want agent.inbox", projection.Command)
	}
	if projection.Facts["experimental"] != "true" {
		t.Error("expected experimental=true")
	}
}

func TestRPCAgentTrustCenterRoute(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.TrustCenter",
		Params: map[string]any{"scope": "project:test"},
	})
	if err != nil {
		t.Fatalf("agent trust center RPC: %v", err)
	}
	if projection.Command != "agent.trust_center" {
		t.Errorf("command = %s, want agent.trust_center", projection.Command)
	}
	if projection.Facts["experimental"] != "true" {
		t.Error("expected experimental=true")
	}
}

func TestRPCAgentContinuityRoute_NoOldRouteChanged(t *testing.T) {
	t.Parallel()
	// 验证新 RPC 路由不影响旧路由
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	// Old agent context route still works
	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.Context",
		Params: map[string]any{"workspace": "default"},
	})
	if err != nil {
		t.Fatalf("old agent context RPC: %v", err)
	}
	if projection.Command != "agent.context" {
		t.Errorf("old command changed: %s", projection.Command)
	}
}
