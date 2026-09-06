package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type BrainMaintenanceRequest struct {
	VaultPath string
	DryRun    bool
	SavePlan  bool
}

func (s *Service) BrainMaintenancePlan(_ context.Context, req BrainMaintenanceRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("brain.maintenance_plan", err), err
	}
	planID := "brain-maintenance-" + time.Now().UTC().Format("20060102T150405Z")
	operations := []domain.AgentBrainMaintenanceOperation{
		{Kind: "stale_memory", Risk: "low", Status: "candidate", Evidence: []string{"memory ledger freshness not inspected in preview"}, NextAction: domain.Action{Name: "memory_context", Command: "pinax memory context <task> --vault <vault> --agent"}},
		{Kind: "duplicate_memory", Risk: "low", Status: "candidate", Evidence: []string{"duplicate detection requires reviewable memory refs"}, NextAction: domain.Action{Name: "memory_recall", Command: "pinax memory recall <query> --vault <vault> --json"}},
		{Kind: "citation_repair", Risk: "medium", Status: "candidate", Evidence: []string{"broken or stale citations must be repaired through proof loop"}, NextAction: domain.Action{Name: "proof_loop", Command: fmt.Sprintf("pinax proof loop run --vault %s --json", shellQuote(root))}},
	}
	// 信任维护规则：stale 且 human 分级的 note 产出重新验证候选（仅候选，不自动写）。
	if reverify, err := brainReverifyStaleHumanOperations(root); err == nil {
		operations = append(operations, reverify...)
	}
	plan := domain.AgentBrainMaintenancePlan{
		SchemaVersion: domain.AgentBrainMaintenancePlanSchemaVersion,
		PlanID:        planID,
		BodyExposure:  "none",
		Writes:        false,
		Operations:    operations,
		NextActions:   []domain.Action{{Name: "proof_loop", Command: fmt.Sprintf("pinax proof loop run --vault %s --json", shellQuote(root))}},
	}

	projection := domain.NewProjection("brain.maintenance_plan", "Agent Brain maintenance plan preview generated.")
	projection.Facts["schema_version"] = domain.AgentBrainMaintenancePlanSchemaVersion
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
	if reverifyCount := countBrainOperations(plan.Operations, "reverify_stale_human"); reverifyCount > 0 {
		projection.Facts["reverify_candidates"] = fmt.Sprint(reverifyCount)
	}
	projection.Facts["writes"] = "false"
	projection.Facts["body_exposure"] = plan.BodyExposure
	projection.Actions = plan.NextActions
	if req.SavePlan {
		relPath := filepath.ToSlash(filepath.Join(".pinax", "brain-maintenance-plans", plan.PlanID+".json"))
		absPath := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			cmdErr := &domain.CommandError{Code: "brain_maintenance_plan_save_failed", Message: "Could not create maintenance plan directory", Hint: "Check vault permissions"}
			return domain.NewErrorProjection("brain.maintenance_plan", cmdErr), cmdErr
		}
		body, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			cmdErr := &domain.CommandError{Code: "brain_maintenance_plan_save_failed", Message: err.Error(), Hint: "Retry the command"}
			return domain.NewErrorProjection("brain.maintenance_plan", cmdErr), cmdErr
		}
		if err := atomicWriteFile(absPath, append(body, '\n'), 0o600); err != nil {
			cmdErr := &domain.CommandError{Code: "brain_maintenance_plan_save_failed", Message: "Could not write maintenance plan", Hint: "Check vault permissions"}
			return domain.NewErrorProjection("brain.maintenance_plan", cmdErr), cmdErr
		}
		plan.SavedPath = relPath
		projection.Facts["saved"] = "true"
		projection.Facts["plan_path"] = relPath
		projection.Evidence = []string{relPath}
	} else {
		projection.Facts["saved"] = "false"
	}
	projection.Data = plan
	return projection, nil
}

// brainReverifyStaleHumanOperations 扫描 vault，为 stale 且 human 分级的 note 产出
// 重新验证候选（仅候选）。扫描失败时静默跳过（维持 preview 只读语义）。
func brainReverifyStaleHumanOperations(root string) ([]domain.AgentBrainMaintenanceOperation, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	evidence := make([]string, 0)
	firstPath := ""
	for _, note := range notes {
		if domain.TrustTierOf(note.Trust) != domain.TrustTierHuman {
			continue
		}
		if domain.FreshnessOf(note.Trust, now) != domain.FreshnessStale {
			continue
		}
		if len(evidence) < 5 {
			evidence = append(evidence, note.Path+" stale_after="+strings.TrimSpace(note.Trust.StaleAfter))
		}
		if firstPath == "" {
			firstPath = note.Path
		}
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	nextAction := domain.Action{Name: "note_verify", Command: fmt.Sprintf("pinax note verify %s --vault %s", shellQuote(firstPath), shellQuote(root))}
	return []domain.AgentBrainMaintenanceOperation{{Kind: "reverify_stale_human", Risk: "low", Status: "candidate", Evidence: evidence, NextAction: nextAction}}, nil
}

func countBrainOperations(operations []domain.AgentBrainMaintenanceOperation, kind string) int {
	count := 0
	for _, operation := range operations {
		if operation.Kind == kind {
			count++
		}
	}
	return count
}
