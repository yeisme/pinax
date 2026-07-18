package mcpserver

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestAgentMemoryMCPToolsAreListedAndReadonly(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	tools, err := server.Handle(ctx, Request{ID: 40, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, name := range []string{"pinax.agent.context", "pinax.agent.memory_recall", "pinax.agent.handoff_read"} {
		if _, ok := toolByName(tools.Tools, name); !ok {
			t.Fatalf("missing %s in tools list: %#v", name, tools.Tools)
		}
	}

	// Existing brain tools must still be present (compatibility)
	for _, name := range []string{"pinax.brain.context", "pinax.brain.answer"} {
		if _, ok := toolByName(tools.Tools, name); !ok {
			t.Fatalf("existing brain tool %s must be preserved", name)
		}
	}
}

func TestAgentMemoryMCPContextToolReturnsBoundedPack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	resp, err := server.Handle(ctx, Request{
		ID:     41,
		Method: "tools/call",
		Params: map[string]any{
			"name":      "pinax.agent.context",
			"arguments": map[string]any{"workspace": "default"},
		},
	})
	if err != nil {
		t.Fatalf("agent context call: %v", err)
	}
	result := resp.Result
	if result["status"] != "success" {
		t.Fatalf("agent context status = %#v", result["status"])
	}
	if result["body_exposure"] != "bounded_projection" {
		t.Fatalf("body_exposure = %#v", result["body_exposure"])
	}
}

func TestAgentMemoryMCPRecallToolReturnsBoundedResults(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	resp, err := server.Handle(ctx, Request{
		ID:     42,
		Method: "tools/call",
		Params: map[string]any{
			"name":      "pinax.agent.memory_recall",
			"arguments": map[string]any{"workspace": "default"},
		},
	})
	if err != nil {
		t.Fatalf("agent recall call: %v", err)
	}
	result := resp.Result
	if result["status"] != "success" {
		t.Fatalf("agent recall status = %#v", result["status"])
	}
	if result["body_exposure"] != "bounded_projection" {
		t.Fatalf("body_exposure = %#v", result["body_exposure"])
	}
}

func TestAgentMemoryMCPWriteToolNotPresentByDefault(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	tools, err := server.Handle(ctx, Request{ID: 43, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	// Proposal/feedback write tools must NOT appear by default (capability disabled)
	for _, name := range []string{"pinax.agent.memory_propose", "pinax.agent.feedback_add"} {
		if _, ok := toolByName(tools.Tools, name); ok {
			t.Fatalf("write tool %s should not be listed by default", name)
		}
	}
}
