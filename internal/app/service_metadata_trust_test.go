package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func writeTrustMetadataFixture(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{
		filepath.Join(root, "notes", "alpha.md"),
		filepath.Join(root, "notes", "beta.md"),
	}
	body1 := "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\ntags: [auth]\ncreated_at: 2026-08-01T08:00:00+00:00\nupdated_at: 2026-08-01T08:00:00+00:00\n---\n\n# Alpha\n\nauth design body\n"
	body2 := "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\ntags: [auth]\ngenerated:\n  by: agent:pinax/0.8.0\n  at: 2026-08-01T08:00:00+00:00\ncreated_at: 2026-08-02T08:00:00+00:00\nupdated_at: 2026-08-02T08:00:00+00:00\n---\n\n# Beta\n\nauth runbook body\n"
	if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(paths[0], []byte(body1), 0o644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := os.WriteFile(paths[1], []byte(body2), 0o644); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	return paths
}

func TestMetadataPlanDefaultExcludesTrustFields(t *testing.T) {
	t.Parallel()
	svc := NewService()
	root := t.TempDir()
	writeTrustMetadataFixture(t, root)

	projection, err := svc.PlanMetadata(context.Background(), MetadataPlanRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, op := range projection.Data.(map[string]any)["operations"].([]domain.PlanOperation) {
		if op.Kind == "trust_fields" {
			t.Fatalf("default plan must not include trust_fields: %#v", op)
		}
	}
}

func TestMetadataPlanAndApplyTrustFields(t *testing.T) {
	t.Parallel()
	svc := NewService()
	root := t.TempDir()
	paths := writeTrustMetadataFixture(t, root)

	projection, err := svc.PlanMetadata(context.Background(), MetadataPlanRequest{VaultPath: root, TrustFields: true, StaleAfter: "2026-12-01T00:00:00+00:00"})
	if err != nil {
		t.Fatalf("plan trust fields: %v", err)
	}
	operations := projection.Data.(map[string]any)["operations"].([]domain.PlanOperation)
	trustOps := map[string]int{}
	for _, op := range operations {
		if op.Kind != "trust_fields" {
			continue
		}
		trustOps[op.Path]++
	}
	// alpha 缺 generated+stale_after；beta 已有 generated 但缺 stale_after。
	if len(trustOps) != 2 || trustOps["notes/alpha.md"] != 1 || trustOps["notes/beta.md"] != 1 {
		t.Fatalf("trust_fields ops = %#v, want one op for alpha and beta each", trustOps)
	}

	if _, err := svc.ApplyMetadata(context.Background(), ApplyRequest{VaultPath: root, Yes: false, TrustFields: true}); err == nil {
		t.Fatalf("apply without --yes must fail")
	}
	applied, err := svc.ApplyMetadata(context.Background(), ApplyRequest{VaultPath: root, Yes: true, TrustFields: true, StaleAfter: "2026-12-01T00:00:00+00:00", AgentVersion: "0.9.0"})
	if err != nil {
		t.Fatalf("apply trust fields: %v", err)
	}
	if applied.Facts["trust_fields_updates"] != "2" {
		t.Fatalf("trust_fields_updates = %q", applied.Facts["trust_fields_updates"])
	}

	alphaBody, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read alpha: %v", err)
	}
	signals, err := domain.ParseTrustSignals(alphaBody)
	if err != nil {
		t.Fatalf("re-parse alpha: %v", err)
	}
	if !signals.HasGenerated || signals.Generated.By != "agent:pinax/0.9.0" || signals.Generated.At != "2026-08-01T08:00:00+00:00" {
		t.Fatalf("alpha generated = %#v", signals.Generated)
	}
	if signals.StaleAfter != "2026-12-01T00:00:00+00:00" {
		t.Fatalf("alpha stale_after = %q", signals.StaleAfter)
	}
	if !strings.Contains(string(alphaBody), "# Alpha") {
		t.Fatalf("alpha body lost: %s", alphaBody)
	}

	// beta 已有 generated：只有 stale_after 被回填，generated 保持不变。
	betaBody, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatalf("read beta: %v", err)
	}
	betaSignals, err := domain.ParseTrustSignals(betaBody)
	if err != nil {
		t.Fatalf("re-parse beta: %v", err)
	}
	if betaSignals.Generated.By != "agent:pinax/0.8.0" {
		t.Fatalf("beta generated must stay unchanged: %#v", betaSignals.Generated)
	}
	if betaSignals.StaleAfter != "2026-12-01T00:00:00+00:00" {
		t.Fatalf("beta stale_after = %q", betaSignals.StaleAfter)
	}

	// 再次 apply 幂等：无新写入。
	again, err := svc.ApplyMetadata(context.Background(), ApplyRequest{VaultPath: root, Yes: true, TrustFields: true, StaleAfter: "2026-12-01T00:00:00+00:00"})
	if err != nil {
		t.Fatalf("apply again: %v", err)
	}
	if again.Facts["trust_fields_updates"] != "0" {
		t.Fatalf("second trust_fields_updates = %q, want 0", again.Facts["trust_fields_updates"])
	}
}

func TestMetadataPlanTrustFieldsRejectsInvalidStaleAfter(t *testing.T) {
	t.Parallel()
	svc := NewService()
	root := t.TempDir()
	writeTrustMetadataFixture(t, root)
	projection, err := svc.PlanMetadata(context.Background(), MetadataPlanRequest{VaultPath: root, TrustFields: true, StaleAfter: "2026-12-01"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, op := range projection.Data.(map[string]any)["operations"].([]domain.PlanOperation) {
		if op.Kind == "trust_fields" {
			t.Fatalf("invalid stale_after must yield no trust_fields ops")
		}
	}
}
