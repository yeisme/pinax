package api

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestRPCAgentContextRoute(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.Context",
		Params: map[string]any{"workspace": "default"},
	})
	if err != nil {
		t.Fatalf("agent context RPC: %v", err)
	}
	if projection.Command != "agent.context" {
		t.Errorf("command = %s", projection.Command)
	}
	if projection.Facts["schema_version"] == "" {
		t.Error("expected schema_version fact")
	}
}

func TestRPCAgentMemoryRecallRoute(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	projection, err := dispatcher.Call(ctx, RPCRequest{
		Method: "Pinax.Agent.Memory.Recall",
		Params: map[string]any{"workspace": "default"},
	})
	if err != nil {
		t.Fatalf("agent recall RPC: %v", err)
	}
	if projection.Command != "agent.memory.recall" {
		t.Errorf("command = %s", projection.Command)
	}
	if projection.Facts["count"] == "" {
		t.Error("expected count fact")
	}
}

func TestRPCAgentCapabilityDiscoveryVisible(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Vault"}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewRPCDispatcher(svc, vault)

	// Unknown agent method should mention capability/routes
	_, err := dispatcher.Call(ctx, RPCRequest{Method: "Pinax.Agent.Unknown"})
	if err == nil {
		t.Fatal("unknown agent method should fail")
	}
}
