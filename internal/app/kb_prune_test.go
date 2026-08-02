package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestPlanKBGenerationPrunePreservesActivePreviousAndNewest(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"gen-active", "gen-previous", "gen-newest", "gen-old"} {
		manifest := validKBGenerationManifest()
		manifest.GenerationID = id
		manifest.CreatedAt = map[string]string{
			"gen-active":   "2026-08-02T00:00:00Z",
			"gen-previous": "2026-08-01T00:00:00Z",
			"gen-newest":   "2026-08-03T00:00:00Z",
			"gen-old":      "2026-07-01T00:00:00Z",
		}[id]
		if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
			t.Fatalf("write %s manifest: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(root, ".pinax", "kb", "generations", id, "payload.bin"), []byte(id), 0o600); err != nil {
			t.Fatalf("write %s payload: %v", id, err)
		}
	}
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      1,
		Active:        validKBActivationRef("gen-active"),
		Previous:      validKBActivationRef("gen-previous"),
		ActivatedAt:   "2026-08-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("write activation: %v", err)
	}
	plan, err := PlanKBGenerationPrune(root, 1)
	if err != nil {
		t.Fatalf("plan prune: %v", err)
	}
	if !containsPruneString(plan.ProtectedGenerationIDs, "gen-active") || !containsPruneString(plan.ProtectedGenerationIDs, "gen-previous") {
		t.Fatalf("protected ids = %#v", plan.ProtectedGenerationIDs)
	}
	if len(plan.DeleteCandidates) != 1 || plan.DeleteCandidates[0].GenerationID != "gen-old" {
		t.Fatalf("delete candidates = %#v, want gen-old only", plan.DeleteCandidates)
	}
}

func TestKBPruneGenerationsRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	writePruneGeneration(t, root, "gen-active", "2026-08-02T00:00:00Z")
	writePruneGeneration(t, root, "gen-old", "2026-07-01T00:00:00Z")
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      1,
		Active:        validKBActivationRef("gen-active"),
		ActivatedAt:   "2026-08-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("write activation: %v", err)
	}
	_, err := NewService().KBPruneGenerations(context.Background(), KBGenerationPruneRequest{VaultPath: root, Keep: 0})
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "approval_required" {
		t.Fatalf("prune without confirmation error = %v, want approval_required", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations", "gen-old")); statErr != nil {
		t.Fatalf("candidate should remain after rejected prune: %v", statErr)
	}
}

func TestKBPruneGenerationsDeletesOnlyUnprotectedCandidates(t *testing.T) {
	root := t.TempDir()
	writePruneGeneration(t, root, "gen-active", "2026-08-02T00:00:00Z")
	writePruneGeneration(t, root, "gen-previous", "2026-08-01T00:00:00Z")
	writePruneGeneration(t, root, "gen-old", "2026-07-01T00:00:00Z")
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      1,
		Active:        validKBActivationRef("gen-active"),
		Previous:      validKBActivationRef("gen-previous"),
		ActivatedAt:   "2026-08-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("write activation: %v", err)
	}
	projection, err := NewService().KBPruneGenerations(context.Background(), KBGenerationPruneRequest{VaultPath: root, Keep: 0, Yes: true})
	if err != nil {
		t.Fatalf("prune generations: %v", err)
	}
	if projection.Facts["deleted"] != "1" || projection.Facts["preserved_active"] != "true" || projection.Facts["preserved_previous"] != "true" {
		t.Fatalf("prune projection = %#v", projection.Facts)
	}
	for _, id := range []string{"gen-active", "gen-previous"} {
		if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations", id)); statErr != nil {
			t.Fatalf("protected generation %s removed: %v", id, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations", "gen-old")); !os.IsNotExist(statErr) {
		t.Fatalf("old generation should be removed, stat err=%v", statErr)
	}
}

func writePruneGeneration(t *testing.T, root, id, createdAt string) {
	t.Helper()
	manifest := validKBGenerationManifest()
	manifest.GenerationID = id
	manifest.CreatedAt = createdAt
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write %s manifest: %v", id, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "kb", "generations", id, "payload.bin"), []byte(id), 0o600); err != nil {
		t.Fatalf("write %s payload: %v", id, err)
	}
}

func containsPruneString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
