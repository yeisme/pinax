package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

// proof_loop.go 实现 pinax proof loop run 编排：把已有的 capture/retrieve/diagnose/
// plan/snapshot/apply 服务串成一条可调用、可审计的本地 agent 工作流，产出单一 projection
// 携带 proof_loop_run_id、有序 stage facts、saved plan paths、snapshot 与 next actions。
//
// 默认 preview（只读）；--apply --yes 才在 fresh snapshot 后执行已批准的 repair/organize
// apply 路径。manual-review-only 操作保持为 next action，不自动 apply。

// ProofLoopRunRequest drives the proof loop run orchestration.
type ProofLoopRunRequest struct {
	VaultPath string
	Apply     bool
	Yes       bool
}

// ProofLoopRun 编排本地 proof loop。它复用内部 service，不 shell out 到子命令。
// 每个阶段把有界事实汇入一个 projection；preview 不写 vault，apply 路径先 fresh snapshot
// 再执行已批准的 repair/organize apply。
func (s *Service) ProofLoopRun(ctx context.Context, req ProofLoopRunRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("proof.loop.run", err), err
	}
	if req.Apply && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "proof loop run --apply requires --yes", Hint: "Review the preview output, then rerun with --apply --yes"}
		return domain.NewErrorProjection("proof.loop.run", err), err
	}

	runID := "proof_loop_" + time.Now().UTC().Format("20060102T150405Z")
	mode := "preview"
	if req.Apply && req.Yes {
		mode = "apply"
	}
	projection := domain.NewProjection("proof.loop.run", "Proof loop "+mode+" completed.")
	projection.Facts["proof_loop_run_id"] = runID
	projection.Facts["mode"] = mode
	projection.Facts["vault"] = root
	evidence := []string{}

	// Stage 1-2: Capture + Diagnose — vault stats 与 doctor issue 摘要。
	stats, statsErr := s.VaultStats(ctx, VaultStatsRequest{VaultPath: root})
	if statsErr != nil {
		return errorProjection("proof.loop.run", statsErr), statsErr
	}
	captureFacts := stageFacts(stats, "capture")
	for k, v := range captureFacts {
		projection.Facts["capture."+k] = v
	}

	doctor, doctorErr := s.VaultDoctor(ctx, VaultDoctorRequest{VaultPath: root})
	if doctorErr != nil {
		return errorProjection("proof.loop.run", doctorErr), doctorErr
	}
	projection.Facts["diagnose.status"] = doctor.Facts["status"]
	projection.Facts["diagnose.issues"] = doctor.Facts["issues"]
	if doctor.Facts["issues"] == "" {
		projection.Facts["diagnose.issues"] = "0"
	}

	// Stage 3: Plan — 生成并保存 repair + organize plan（只读，不 apply）。
	repairPlan, repairErr := s.PlanRepair(ctx, RepairPlanRequest{VaultPath: root, Save: true})
	if repairErr != nil {
		return errorProjection("proof.loop.run", repairErr), repairErr
	}
	if rp := repairPlan.Facts["plan_id"]; rp != "" {
		projection.Facts["plan.repair_plan_id"] = rp
	}
	if rp := repairPlan.Facts["saved_path"]; rp != "" {
		evidence = append(evidence, rp)
	}
	organizePlanID, organizePath, organizeErr := s.saveOrganizePlanForRun(root)
	if organizeErr == nil && organizePlanID != "" {
		projection.Facts["plan.organize_plan_id"] = organizePlanID
		evidence = append(evidence, organizePath)
	}

	// Stage 4: Snapshot — apply 路径需要 fresh snapshot；preview 只提示。
	if req.Apply && req.Yes {
		snap, snapErr := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "proof loop pre-apply"})
		if snapErr != nil {
			projection := errorProjection("proof.loop.run", snapErr)
			projection.Facts["proof_loop_run_id"] = runID
			return projection, snapErr
		}
		projection.Facts["apply.snapshot"] = "true"
		if ev := snap.Evidence; len(ev) > 0 {
			evidence = append(evidence, ev[0])
		}

		// Stage 5: Apply — 仅执行已批准的 repair/organize apply；manual-review-only 自动跳过。
		if rp := repairPlan.Facts["plan_id"]; rp != "" {
			repairApply, applyErr := s.ApplyRepair(ctx, RepairApplyRequest{VaultPath: root, PlanID: rp, Yes: true})
			if applyErr != nil {
				projection.Facts["apply.repair"] = "failed"
				projection.Facts["apply.repair_error"] = applyErr.Error()
			} else {
				projection.Facts["apply.repair"] = repairApply.Facts["applied"]
				if repairApply.Facts["applied"] == "0" && repairApply.Facts["skipped"] != "" {
					projection.Facts["apply.repair"] = "skipped"
				}
			}
		}
		if organizePlanID != "" {
			orgApply, applyErr := s.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, PlanID: organizePlanID, Yes: true})
			if applyErr != nil {
				projection.Facts["apply.organize"] = "failed"
				projection.Facts["apply.organize_error"] = applyErr.Error()
			} else if orgApply.Facts["applied"] != "" {
				projection.Facts["apply.organize"] = orgApply.Facts["applied"]
			}
		}
	}

	// 汇总 evidence 与 next actions（preview 与 apply 都给）。
	if len(evidence) > 0 {
		projection.Evidence = dedupeEvidence(evidence)
	}
	projection.Evidence = append(projection.Evidence, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl")))
	projection.Actions = proofLoopNextActions(root, req.Apply && req.Yes, repairPlan.Facts["plan_id"], organizePlanID)
	projection.Data = map[string]any{
		"proof_loop_run_id": runID,
		"mode":              mode,
		"stages":            []string{"capture", "diagnose", "plan", "snapshot", "apply"},
		"diagnose":          doctor.Data,
		"repair_plan_id":    repairPlan.Facts["plan_id"],
		"organize_plan_id":  organizePlanID,
	}
	return projection, nil
}

// saveOrganizePlanForRun 构造并保存一份 organize plan 供 proof loop run 使用，返回 plan id 与保存路径。
func (s *Service) saveOrganizePlanForRun(root string) (string, string, error) {
	plan, err := buildOrganizePlan(root)
	if err != nil {
		return "", "", err
	}
	if err := saveOrganizePlan(root, &plan); err != nil {
		return "", "", err
	}
	return plan.PlanID, plan.SavedPath, nil
}

// stageFacts 从子阶段 projection 提取事实并加上前缀。
func stageFacts(p domain.Projection, prefix string) map[string]string {
	out := map[string]string{}
	for k, v := range p.Facts {
		out[k] = v
	}
	return out
}

// proofLoopNextActions 给出 proof loop run 之后的 next action 集合。
func proofLoopNextActions(root string, applied bool, repairPlanID, organizePlanID string) []domain.Action {
	actions := []domain.Action{}
	if !applied {
		actions = append(actions, domain.Action{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("proof loop pre-apply"))})
		if repairPlanID != "" {
			actions = append(actions, domain.Action{Name: "repair_apply", Command: fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(repairPlanID))})
		}
		if organizePlanID != "" {
			actions = append(actions, domain.Action{Name: "organize_apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(organizePlanID))})
		}
	}
	actions = append(actions, domain.Action{Name: "doctor", Command: fmt.Sprintf("pinax vault doctor --vault %s", shellQuote(root))})
	return actions
}

func dedupeEvidence(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
