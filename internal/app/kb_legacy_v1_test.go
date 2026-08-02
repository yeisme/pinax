package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/semantic"
)

func TestKBDoctorReportsLegacyV1ReadonlyCompatibility(t *testing.T) {
	root := t.TempDir()
	writeAppLegacyV1Fixture(t, root, "legacy query")
	projection, err := NewService().KBDoctor(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, SidecarExecutable: filepath.Join(root, "missing-sidecar")})
	if err != nil {
		t.Fatalf("legacy doctor: %v", err)
	}
	if projection.Facts["protocol"] != semantic.LegacySidecarSchema || projection.Facts["profile"] != "legacy_v1_readonly" || projection.Facts["compatibility_status"] != "active" {
		t.Fatalf("legacy doctor facts = %#v", projection.Facts)
	}
	if projection.Facts["next_action"] != "rebuild_inferrum_v1" {
		t.Fatalf("legacy doctor next_action = %q, want rebuild_inferrum_v1", projection.Facts["next_action"])
	}
	for key, want := range map[string]string{"introduced_release": "N", "compatible_through_release": "N+1", "removal_eligible_release": "N+2"} {
		if projection.Facts[key] != want {
			t.Fatalf("legacy doctor fact %s=%q, want %q", key, projection.Facts[key], want)
		}
	}
}

func TestKBReviewOverviewReportsLegacyV1WithoutCallingSidecar(t *testing.T) {
	root := t.TempDir()
	writeAppLegacyV1Fixture(t, root, "legacy query")
	projection, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("legacy review overview: %v", err)
	}
	if projection.Facts["index_status"] != "legacy_v1_readonly" || projection.Facts["protocol"] != semantic.LegacySidecarSchema {
		t.Fatalf("legacy review facts = %#v", projection.Facts)
	}
	if projection.Facts["generation_status"] != "legacy_v1_readonly" {
		t.Fatalf("legacy generation status = %q", projection.Facts["generation_status"])
	}
	if projection.Facts["next_action"] != "rebuild_inferrum_v1" {
		t.Fatalf("legacy review next_action = %q, want rebuild_inferrum_v1", projection.Facts["next_action"])
	}
}

func TestKBSearchRequiresExplicitLegacyV1ReadonlyProfile(t *testing.T) {
	root := t.TempDir()
	writeAppLegacyV1Fixture(t, root, "legacy query")
	baseRequest := KBIndexRequest{VaultPath: root, Query: "legacy query", Backend: semantic.DefaultBackend, Provider: "fake", Model: semantic.FakeProviderModel, PermissionResolved: true, AllowedIDs: []string{"chunk-legacy"}}
	_, err := NewService().KBSearch(context.Background(), baseRequest)
	if err == nil || !strings.Contains(err.Error(), "kb_legacy_v1_readonly") {
		t.Fatalf("legacy search without profile error = %v, want kb_legacy_v1_readonly", err)
	}
	baseRequest.LegacyV1Readonly = true
	projection, err := NewService().KBSearch(context.Background(), baseRequest)
	if err != nil {
		t.Fatalf("legacy readonly search: %v", err)
	}
	if projection.Facts["protocol"] != semantic.LegacySidecarSchema || projection.Facts["matches"] != "1" {
		t.Fatalf("legacy search facts = %#v", projection.Facts)
	}
}

func TestKBContextRequiresExplicitLegacyV1ReadonlyProfile(t *testing.T) {
	root := t.TempDir()
	writeAppLegacyV1Fixture(t, root, "legacy query")
	baseRequest := KBIndexRequest{VaultPath: root, Query: "legacy query", Backend: semantic.DefaultBackend, Provider: "fake", Model: semantic.FakeProviderModel, PermissionResolved: true, AllowedIDs: []string{"chunk-legacy"}}
	_, err := NewService().KBContext(context.Background(), baseRequest)
	if err == nil || !strings.Contains(err.Error(), "kb_legacy_v1_readonly") {
		t.Fatalf("legacy context without profile error = %v, want kb_legacy_v1_readonly", err)
	}
	baseRequest.LegacyV1Readonly = true
	projection, err := NewService().KBContext(context.Background(), baseRequest)
	if err != nil {
		t.Fatalf("legacy readonly context: %v", err)
	}
	if projection.Facts["protocol"] != semantic.LegacySidecarSchema || projection.Facts["profile"] != "legacy_v1_readonly" || projection.Facts["matches"] != "1" {
		t.Fatalf("legacy context facts = %#v", projection.Facts)
	}
}

func TestKBRebuildTargetsInferrumV1WhenLegacyProjectionExists(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "notes"))
	writeReviewTruthNote(t, root)
	writeAppLegacyV1Fixture(t, root, "legacy query")
	projection, err := NewService().KBRebuild(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, Provider: "fake", Model: semantic.FakeProviderModel, SidecarExecutable: writeLegacyAwareFakeSidecar(t)})
	if err != nil {
		t.Fatalf("rebuild over legacy projection: %v", err)
	}
	if projection.Facts["protocol"] != semantic.SidecarSchema || projection.Facts["generation_status"] != string(KBGenerationStatusReady) {
		t.Fatalf("rebuild facts = %#v", projection.Facts)
	}
}

func TestKBRefreshTargetsInferrumV1WhenLegacyProjectionExists(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "notes"))
	writeReviewTruthNote(t, root)
	writeAppLegacyV1Fixture(t, root, "legacy query")
	projection, err := NewService().KBRefresh(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, Provider: "fake", Model: semantic.FakeProviderModel, SidecarExecutable: writeLegacyAwareFakeSidecar(t)})
	if err != nil {
		t.Fatalf("refresh over legacy projection: %v", err)
	}
	if projection.Command != "kb.refresh" || projection.Facts["protocol"] != semantic.SidecarSchema || projection.Facts["generation_status"] != string(KBGenerationStatusReady) {
		t.Fatalf("refresh facts = %#v", projection.Facts)
	}
}

func TestKBPruneRejectsLegacyOnlyProjection(t *testing.T) {
	root := t.TempDir()
	writeAppLegacyV1Fixture(t, root, "legacy query")
	projection, err := NewService().KBPruneGenerations(context.Background(), KBGenerationPruneRequest{VaultPath: root, Keep: 0, DryRun: false, Yes: true})
	if err == nil || projection.Error == nil || projection.Error.Code != "kb_legacy_v1_readonly" {
		t.Fatalf("legacy prune error = %v projection=%#v, want kb_legacy_v1_readonly", err, projection)
	}
}

func writeAppLegacyV1Fixture(t *testing.T, root, query string) {
	t.Helper()
	store := filepath.Join(root, ".pinax", "kb", "lancedb")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("mkdir legacy store: %v", err)
	}
	payload := map[string]any{"schema_version": semantic.LegacySidecarSchema, "backend": semantic.DefaultBackend, "provider": "fake", "model": semantic.FakeProviderModel, "embedding_dim": 32, "indexed_at": "2026-08-02T00:00:00Z"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "metadata.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}
	vector, err := (semantic.FakeProvider{ModelName: semantic.FakeProviderModel}).Embed(context.Background(), query)
	if err != nil {
		t.Fatalf("embed legacy fixture: %v", err)
	}
	chunk := semantic.Chunk{ChunkID: "chunk-legacy", NoteID: "legacy-note", VaultPath: "notes/legacy.md", Title: "Legacy", HeadingPath: "Legacy", Preview: "legacy bounded preview", EmbeddingModel: semantic.FakeProviderModel, EmbeddingDim: len(vector), Provider: "fake", Backend: semantic.DefaultBackend, Vector: vector, IndexedAt: "2026-08-02T00:00:00Z"}
	chunkPayload, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal legacy chunk: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "chunks.jsonl"), append(chunkPayload, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy chunks: %v", err)
	}
}

func writeLegacyAwareFakeSidecar(t *testing.T) string {
	t.Helper()
	return writeStagingFakeSidecar(t)
}
