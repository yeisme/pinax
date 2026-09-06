package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// 合并缝测试：metadata plan --save --trust-fields 保存的 trust_fields 操作必须能被
// metadata apply --plan 消费（stale_after 目标值取 plan 内记录，不依赖 apply 期传参）。
func TestMetadataSavedPlanAppliesTrustFields(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(root, "notes", "auth.md")
	if err := os.WriteFile(notePath, []byte("---\nschema_version: pinax.note.v1\nnote_id: note_auth\ntitle: Auth\n---\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService()

	planProjection, err := svc.PlanMetadata(context.Background(), MetadataPlanRequest{VaultPath: root, Save: true, TrustFields: true, StaleAfter: "2030-01-01T00:00:00+00:00"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	planID := planProjection.Facts["plan_id"]
	if planID == "" {
		t.Fatalf("plan_id missing: %#v", planProjection.Facts)
	}
	found := false
	planData, _ := planProjection.Data.(map[string]any)
	ops, _ := planData["operations"].([]domain.PlanOperation)
	for _, op := range ops {
		if op.Kind == "trust_fields" {
			found = true
			if op.Target != "2030-01-01T00:00:00+00:00" {
				t.Fatalf("trust op target = %q, want stale_after value", op.Target)
			}
		}
	}
	if !found {
		t.Fatalf("trust_fields op missing from saved plan: %#v", planData["operations"])
	}

	// apply --plan 不带 --stale-after：值必须来自 plan。
	applied, err := svc.ApplyMetadata(context.Background(), ApplyRequest{VaultPath: root, PlanID: planID, Yes: true})
	if err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	if applied.Facts["applied_updates"] != "2" { // 1 metadata_update + 1 trust_fields
		t.Fatalf("applied_updates = %v (want 1): %#v", applied.Facts["applied_updates"], applied.Facts)
	}
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	if !strings.Contains(body, "generated:") || !strings.Contains(body, `stale_after: "2030-01-01T00:00:00+00:00"`) {
		t.Fatalf("trust fields missing after saved-plan apply:\n%s", body)
	}
	if !strings.Contains(body, "body\n") {
		t.Fatalf("note body corrupted:\n%s", body)
	}
}
