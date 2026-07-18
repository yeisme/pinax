package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	plan := domain.AgentBrainMaintenancePlan{
		SchemaVersion: domain.AgentBrainMaintenancePlanSchemaVersion,
		PlanID:        planID,
		BodyExposure:  "none",
		Writes:        false,
		Operations: []domain.AgentBrainMaintenanceOperation{
			{Kind: "stale_memory", Risk: "low", Status: "candidate", Evidence: []string{"memory ledger freshness not inspected in preview"}, NextAction: domain.Action{Name: "memory_context", Command: "pinax memory context <task> --vault <vault> --agent"}},
			{Kind: "duplicate_memory", Risk: "low", Status: "candidate", Evidence: []string{"duplicate detection requires reviewable memory refs"}, NextAction: domain.Action{Name: "memory_recall", Command: "pinax memory recall <query> --vault <vault> --json"}},
			{Kind: "citation_repair", Risk: "medium", Status: "candidate", Evidence: []string{"broken or stale citations must be repaired through proof loop"}, NextAction: domain.Action{Name: "proof_loop", Command: fmt.Sprintf("pinax proof loop run --vault %s --json", shellQuote(root))}},
		},
		NextActions: []domain.Action{{Name: "proof_loop", Command: fmt.Sprintf("pinax proof loop run --vault %s --json", shellQuote(root))}},
	}

	projection := domain.NewProjection("brain.maintenance_plan", "Agent Brain maintenance plan preview generated.")
	projection.Facts["schema_version"] = domain.AgentBrainMaintenancePlanSchemaVersion
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
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
		if err := os.WriteFile(absPath, append(body, '\n'), 0o600); err != nil {
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
