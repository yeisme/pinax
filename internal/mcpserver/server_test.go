package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestReadonlyMCPListsAndCallsTools(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	tools, err := server.Handle(ctx, Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if !containsTool(tools.Tools, "pinax.search") || containsTool(tools.Tools, "pinax.organize.apply") {
		t.Fatalf("tools = %#v", tools.Tools)
	}

	resources, err := server.Handle(ctx, Request{ID: 2, Method: "resources/list"})
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if !containsResource(resources.Resources, "pinax://vault/current") {
		t.Fatalf("resources = %#v", resources.Resources)
	}

	writeMCPFixture(t, root, "notes/pinax.md", "# Pinax MCP\n\n只读查询。\n")
	search, err := server.Handle(ctx, Request{ID: 3, Method: "tools/call", Params: map[string]any{"name": "pinax.search", "arguments": map[string]any{"query": "只读"}}})
	if err != nil {
		t.Fatalf("tools/call search: %v", err)
	}
	if search.Result == nil || search.Result["status"] != "success" {
		t.Fatalf("search result = %#v", search.Result)
	}

	if _, err := server.Handle(ctx, Request{ID: 4, Method: "tools/call", Params: map[string]any{"name": "pinax.organize.apply"}}); err == nil {
		t.Fatalf("write tool unexpectedly succeeded")
	}
}

func writeMCPFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func containsTool(tools []Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func containsResource(resources []Resource, uri string) bool {
	for _, resource := range resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}
