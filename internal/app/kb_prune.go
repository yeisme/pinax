package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

type KBGenerationPruneRequest struct {
	VaultPath string
	Keep      int
	DryRun    bool
	Yes       bool
}

type KBGenerationPruneItem struct {
	GenerationID string `json:"generation_id"`
	CreatedAt    string `json:"created_at"`
	Bytes        uint64 `json:"bytes"`
}

type KBGenerationPrunePlan struct {
	Keep                     int                     `json:"keep"`
	ProtectedGenerationIDs   []string                `json:"protected_generation_ids"`
	PreservedCandidates      []KBGenerationPruneItem `json:"preserved_candidates"`
	DeleteCandidates         []KBGenerationPruneItem `json:"delete_candidates"`
	EvidenceDeleteCandidates []string                `json:"evidence_delete_candidates"`
}

// PlanKBGenerationPrune returns a deterministic plan. Active and previous are
// always protected; Keep controls only additional non-active candidates.
func PlanKBGenerationPrune(root string, keep int) (KBGenerationPrunePlan, error) {
	if keep < 0 {
		keep = 0
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return KBGenerationPrunePlan{}, err
	}
	protected := make([]string, 0, 2)
	protectedSet := make(map[string]struct{}, 2)
	for _, ref := range []*KBActivationRef{descriptor.Active, descriptor.Previous} {
		if ref == nil {
			continue
		}
		if _, exists := protectedSet[ref.GenerationID]; exists {
			continue
		}
		protectedSet[ref.GenerationID] = struct{}{}
		protected = append(protected, ref.GenerationID)
	}
	generationRoot := filepath.Join(kbRoot(root), "generations")
	entries, err := os.ReadDir(generationRoot)
	if errors.Is(err, os.ErrNotExist) {
		return KBGenerationPrunePlan{Keep: keep, ProtectedGenerationIDs: protected}, nil
	}
	if err != nil {
		return KBGenerationPrunePlan{}, err
	}
	candidates := make([]KBGenerationPruneItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validKBToken(entry.Name()) {
			continue
		}
		if _, isProtected := protectedSet[entry.Name()]; isProtected {
			continue
		}
		manifest, readErr := ReadKBGenerationManifest(root, entry.Name())
		if readErr != nil {
			// Unknown or malformed directories are preserved. Pruning must not
			// turn a corrupted candidate into silent data loss.
			continue
		}
		bytes, sizeErr := kbDirectoryBytes(filepath.Join(generationRoot, entry.Name()))
		if sizeErr != nil {
			return KBGenerationPrunePlan{}, sizeErr
		}
		candidates = append(candidates, KBGenerationPruneItem{GenerationID: entry.Name(), CreatedAt: manifest.CreatedAt, Bytes: bytes})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, candidates[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339, candidates[j].CreatedAt)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		return candidates[i].GenerationID > candidates[j].GenerationID
	})
	plan := KBGenerationPrunePlan{Keep: keep, ProtectedGenerationIDs: protected}
	if keep > len(candidates) {
		keep = len(candidates)
	}
	plan.PreservedCandidates = append(plan.PreservedCandidates, candidates[:keep]...)
	plan.DeleteCandidates = append(plan.DeleteCandidates, candidates[keep:]...)
	deleteSet := make(map[string]struct{}, len(plan.DeleteCandidates))
	for _, item := range plan.DeleteCandidates {
		deleteSet[item.GenerationID] = struct{}{}
	}
	plan.EvidenceDeleteCandidates = planEvidenceDeletes(root, deleteSet)
	return plan, nil
}

func planEvidenceDeletes(root string, generationIDs map[string]struct{}) []string {
	if len(generationIDs) == 0 {
		return nil
	}
	dir := filepath.Join(kbRoot(root), "evaluations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !validKBToken(entry.Name()) {
			continue
		}
		receipt, readErr := ReadKBEvaluationReceipt(root, entry.Name())
		if readErr != nil {
			continue
		}
		if _, ok := generationIDs[receipt.GenerationID]; !ok {
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Join(".pinax", "kb", "evaluations", entry.Name())))
	}
	sort.Strings(paths)
	return paths
}

func (s *Service) KBPruneGenerations(_ context.Context, req KBGenerationPruneRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.generations.prune", err), err
	}
	if legacy, legacyErr := semantic.DetectLegacyV1(root); legacyErr != nil {
		return errorProjection("kb.generations.prune", legacyErr), legacyErr
	} else if legacy.Present {
		descriptor, descriptorErr := ReadKBActivationDescriptor(root)
		if descriptorErr != nil {
			return errorProjection("kb.generations.prune", descriptorErr), descriptorErr
		}
		if descriptor.Active == nil {
			err := &domain.CommandError{Code: "kb_legacy_v1_readonly", Message: "Legacy v1 KB projection is read-only until an Inferrum v1 generation is active", Hint: "Run a normal pinax kb rebuild, evaluate the Inferrum v1 candidate, and activate it before pruning generations"}
			return domain.NewErrorProjection("kb.generations.prune", err), err
		}
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "KB generation prune requires --yes", Hint: "Run with --dry-run first, then add --yes after reviewing delete candidates"}
		return domain.NewErrorProjection("kb.generations.prune", err), err
	}
	plan, err := PlanKBGenerationPrune(root, req.Keep)
	if err != nil {
		return errorProjection("kb.generations.prune", err), err
	}
	deleted := 0
	evidenceDeleted := 0
	if !req.DryRun {
		lockPath := filepath.Join(kbRoot(root), "activation.lock")
		if err := acquireKBActivationLock(lockPath); err != nil {
			return commandErrorProjection("kb.generations.prune", err)
		}
		defer func() { _ = os.Remove(lockPath) }()
		// Re-plan under the mutation lock so a concurrent activation cannot
		// make a formerly-candidate generation active between planning/apply.
		plan, err = PlanKBGenerationPrune(root, req.Keep)
		if err != nil {
			return errorProjection("kb.generations.prune", err), err
		}
		for _, item := range plan.DeleteCandidates {
			path := filepath.Join(kbRoot(root), "generations", item.GenerationID)
			if err := os.RemoveAll(path); err != nil {
				return errorProjection("kb.generations.prune", err), err
			}
			deleted++
		}
		for _, rel := range plan.EvidenceDeleteCandidates {
			path, joinErr := safeJoin(root, rel)
			if joinErr != nil {
				return errorProjection("kb.generations.prune", joinErr), joinErr
			}
			if err := os.RemoveAll(path); err != nil {
				return errorProjection("kb.generations.prune", err), err
			}
			evidenceDeleted++
		}
	}
	projection := domain.NewProjection("kb.generations.prune", "KB generation prune plan generated.")
	if !req.DryRun {
		projection.Summary = "KB generation candidates pruned."
	}
	projection.Facts["keep"] = fmt.Sprint(plan.Keep)
	projection.Facts["candidates"] = fmt.Sprint(len(plan.PreservedCandidates) + len(plan.DeleteCandidates))
	projection.Facts["delete_candidates"] = fmt.Sprint(len(plan.DeleteCandidates))
	projection.Facts["deleted"] = fmt.Sprint(deleted)
	projection.Facts["evidence_delete_candidates"] = fmt.Sprint(len(plan.EvidenceDeleteCandidates))
	projection.Facts["evidence_deleted"] = fmt.Sprint(evidenceDeleted)
	projection.Facts["preserved_active"] = fmt.Sprint(hasGenerationID(plan.ProtectedGenerationIDs, activeGenerationID(root)))
	projection.Facts["preserved_previous"] = fmt.Sprint(hasPreviousGeneration(root, plan.ProtectedGenerationIDs))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	for _, item := range plan.DeleteCandidates {
		projection.Evidence = append(projection.Evidence, filepath.ToSlash(filepath.Join(".pinax", "kb", "generations", item.GenerationID)))
	}
	projection.Data = map[string]any{"plan": plan, "deleted": deleted, "evidence_deleted": evidenceDeleted, "dry_run": req.DryRun}
	return projection, nil
}

func activeGenerationID(root string) string {
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil || descriptor.Active == nil {
		return ""
	}
	return descriptor.Active.GenerationID
}

func hasPreviousGeneration(root string, ids []string) bool {
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil || descriptor.Previous == nil {
		return false
	}
	return hasGenerationID(ids, descriptor.Previous.GenerationID)
}

func hasGenerationID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
