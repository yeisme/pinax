package app

import (
	"context"
	"testing"
)

func TestMemoryServiceCaptureAndContext(t *testing.T) {
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if projection, err := svc.MemoryCapture(context.Background(), MemoryCaptureRequest{VaultPath: root, Type: "decision", Subject: "pinax", Object: "Use structured memory", Source: "openspec/changes/pinax-agent-memory-ledger/design.md"}); err != nil || projection.Command != "memory.capture" {
		t.Fatalf("capture projection=%#v err=%v", projection, err)
	}
	projection, err := svc.MemoryContext(context.Background(), MemoryRecallRequest{VaultPath: root, Query: "structured memory", Entity: "pinax", Limit: 4})
	if err != nil {
		t.Fatalf("memory context: %v", err)
	}
	if projection.Command != "memory.context" || projection.Facts["memory.matches"] != "1" || projection.Facts["memory.types"] != "decision" {
		t.Fatalf("context projection = %#v", projection)
	}
	data := projection.Data.(map[string]any)
	matches := data["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["recall_reason"] == "" {
		t.Fatalf("context matches = %#v", matches)
	}
}
