package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/planningops"
	"github.com/yeisme/pinax/internal/domain"
)

// Plan operations: daily/weekly/monthly planning snapshot+decision, action draft
// generation, and snapshot/action persistence. Extracted from service.go to isolate
// the planning surface from the Service facade.

type PlanningRequest struct {
	VaultPath      string
	Period         string // daily, weekly, monthly
	WithTaskBridge bool
	TaskReview     bool
	DryRun         bool
	Yes            bool
	Save           bool
	FromPeriod     string // for plan actions --from
}

// PlanDaily 生成每日Plan。
func (s *Service) PlanDaily(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningDaily)
}

// PlanWeekly 生成每周Plan。
func (s *Service) PlanWeekly(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningWeekly)
}

// PlanMonthly 生成每月Plan。
func (s *Service) PlanMonthly(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningMonthly)
}

func (s *Service) planPeriod(ctx context.Context, req PlanningRequest, period domain.PlanningPeriod) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	// 生成 planning snapshot。
	now := currentTimeUTC()
	snapshot := domain.PlanningSnapshot{
		SchemaVersion: "pinax.planning.snapshot.v1",
		SnapshotID:    planningSnapshotID(root, string(period), now),
		Source:        "local",
		CapturedAt:    now.Format(time.RFC3339),
		Facts:         map[string]string{},
		Risks:         []domain.PlanningRisk{},
	}
	// 读取 vault planning context。
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	snapshot.Facts["notes"] = fmt.Sprint(len(facts))
	snapshot.Facts["source"] = "local"
	if board, rel, ok, err := latestProjectBoardSnapshot(root); err != nil {
		return errorProjection("plan."+string(period), err), err
	} else if ok {
		mergeProjectBoardPlanningFacts(&snapshot, board, rel)
	}
	maxCommitments := planningops.MaxCommitments(period)
	snapshot.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
	// 生成 decision。
	decision := domain.PlanningDecision{
		SchemaVersion: "pinax.planning.decision.v1",
		DecisionID:    planningDecisionID(root, string(period), now),
		Period:        period,
		Selected:      []string{},
		Deferred:      []string{},
		Reasons:       []domain.PlanningReason{},
		NextActions:   []domain.Action{},
		CreatedAt:     now.Format(time.RFC3339),
	}
	planningops.AddCapacityRisk(&snapshot, &decision, len(facts), maxCommitments)
	command := "plan." + string(period)
	if req.WithTaskBridge && period == domain.PlanningDaily {
		taskBridge, err := loadTaskBridgeDaily(ctx, now)
		if err != nil {
			return errorProjection(command, err), err
		}
		applyTaskBridgePlanning(&snapshot, &decision, taskBridge, maxCommitments)
	}
	if req.TaskReview && period == domain.PlanningDaily {
		return s.planDailyTaskReview(ctx, root, now, snapshot, decision, req.Yes)
	}
	targetNote := filepath.ToSlash(filepath.Join("daily", now.Format("2006-01-02")+".md"))
	if req.DryRun || !req.Yes {
		projection := domain.NewProjection(command, string(period)+" plan previewed.")
		projection.Facts["period"] = string(period)
		projection.Facts["dry_run"] = "true"
		projection.Facts["snapshot_id"] = snapshot.SnapshotID
		projection.Facts["decision_id"] = decision.DecisionID
		projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
		projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
		if req.WithTaskBridge && period == domain.PlanningDaily {
			projection.Facts["source"] = "taskbridge"
			projection.Facts["captured_at"] = snapshot.CapturedAt
			projection.Facts["target_note"] = targetNote
			projection.Facts["managed_block"] = planningDailyBlockName
			projection.Facts["selected_commitments"] = fmt.Sprint(len(decision.Selected))
			projection.Facts["taskbridge_tasks"] = snapshot.Facts["taskbridge_tasks"]
		}
		copyProjectBoardPlanningFacts(&projection, snapshot)
		projection.Data = map[string]any{"snapshot": snapshot, "decision": decision}
		applyCommand := fmt.Sprintf("pinax plan %s --vault %s --yes", string(period), shellQuote(root))
		if req.WithTaskBridge && period == domain.PlanningDaily {
			applyCommand = fmt.Sprintf("pinax plan daily --taskbridge --vault %s --yes", shellQuote(root))
		}
		projection.Actions = []domain.Action{
			{Name: "apply", Command: applyCommand},
		}
		return projection, nil
	}
	if req.WithTaskBridge && period == domain.PlanningDaily {
		markdown := renderTaskBridgeDailyMarkdown(snapshot, decision)
		dailyRel, err := writeDailyPlanningBlock(root, now, markdown)
		if err != nil {
			return errorProjection(command, err), err
		}
		snapshot.Facts["target_note"] = dailyRel
	}
	// 写入 snapshot。
	if req.Save {
		snapRel, err := savePlanningSnapshot(root, &snapshot)
		if err != nil {
			return errorProjection(command, err), err
		}
		snapshot.SavedPath = snapRel
	}
	_ = appendEvent(root, command, "success", map[string]string{"period": string(period), "snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection(command, string(period)+" plan generated.")
	projection.Facts["period"] = string(period)
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["decision_id"] = decision.DecisionID
	projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
	projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
	if req.WithTaskBridge && period == domain.PlanningDaily {
		projection.Facts["source"] = "taskbridge"
		projection.Facts["captured_at"] = snapshot.CapturedAt
		projection.Facts["target_note"] = snapshot.Facts["target_note"]
		projection.Facts["managed_block"] = planningDailyBlockName
		projection.Facts["selected_commitments"] = fmt.Sprint(len(decision.Selected))
		projection.Facts["taskbridge_tasks"] = snapshot.Facts["taskbridge_tasks"]
	}
	copyProjectBoardPlanningFacts(&projection, snapshot)
	if snapshot.SavedPath != "" {
		projection.Facts["saved_path"] = snapshot.SavedPath
		projection.Evidence = []string{snapshot.SavedPath}
	}
	projection.Data = map[string]any{"snapshot": snapshot, "decision": decision}
	projection.Actions = []domain.Action{
		{Name: "open", Command: fmt.Sprintf("pinax %s open --vault %s", string(period), shellQuote(root))},
	}
	return projection, nil
}

// PlanActions 生成 TaskBridge action file 草稿。
func (s *Service) PlanActions(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan.actions", err), err
	}
	now := currentTimeUTC()
	period := strings.TrimSpace(req.FromPeriod)
	if period == "" {
		period = "daily"
	}
	planningPeriod, err := planningops.ParsePeriod(period)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	preview, err := s.planPeriod(ctx, PlanningRequest{VaultPath: root, Period: period, WithTaskBridge: req.WithTaskBridge, DryRun: true}, planningPeriod)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	snapshot, decision, err := planningops.PreviewData(preview)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	draft := planningops.BuildActionDraft(period, snapshot, decision, now)
	if req.DryRun || !req.Save {
		projection := domain.NewProjection("plan.actions", "Action draft previewed.")
		projection.Facts["period"] = period
		projection.Facts["dry_run"] = "true"
		projection.Facts["action_id"] = draft.ActionID
		projection.Facts["source_decision"] = draft.SourceDecision
		projection.Facts["snapshot_id"] = draft.SourceSnapshot
		projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
		if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
			projection.Facts["source"] = "taskbridge"
		}
		projection.Data = map[string]any{"draft": draft}
		saveCommand := fmt.Sprintf("pinax plan actions --from %s --vault %s --save", period, shellQuote(root))
		if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
			saveCommand = fmt.Sprintf("pinax plan actions --from daily --taskbridge --vault %s --save", shellQuote(root))
		}
		projection.Actions = []domain.Action{
			{Name: "save", Command: saveCommand},
		}
		return projection, nil
	}
	// 保存 action draft。
	rel, err := savePlanningActionDraft(root, &draft)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	draft.SavedPath = rel
	_ = appendEvent(root, "plan.actions", "success", map[string]string{"action_id": draft.ActionID, "saved_path": rel})
	projection := domain.NewProjection("plan.actions", "Action draft saved.")
	projection.Facts["action_id"] = draft.ActionID
	projection.Facts["source_decision"] = draft.SourceDecision
	projection.Facts["snapshot_id"] = draft.SourceSnapshot
	projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
	projection.Facts["saved_path"] = rel
	if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
		projection.Facts["source"] = "taskbridge"
	}
	projection.Evidence = []string{rel}
	projection.Data = map[string]any{"draft": draft}
	projection.Actions = []domain.Action{
		{Name: "execute", Command: fmt.Sprintf("connectors task agent execute --action-file %s --dry-run", rel)},
	}
	return projection, nil
}

// PlanSnapshot 生成Plan快照。
func (s *Service) PlanSnapshot(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	now := time.Now().UTC()
	snapshot := domain.PlanningSnapshot{
		SchemaVersion: "pinax.planning.snapshot.v1",
		SnapshotID:    planningSnapshotID(root, "manual", now),
		Source:        "local",
		CapturedAt:    now.Format(time.RFC3339),
		Facts:         map[string]string{},
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	snapshot.Facts["notes"] = fmt.Sprint(len(facts))
	snapRel, err := savePlanningSnapshot(root, &snapshot)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	snapshot.SavedPath = snapRel
	_ = appendEvent(root, "plan.snapshot", "success", map[string]string{"snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection("plan.snapshot", "Planning snapshot saved.")
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["saved_path"] = snapRel
	projection.Data = map[string]any{"snapshot": snapshot}
	projection.Actions = []domain.Action{
		{Name: "plan", Command: fmt.Sprintf("pinax plan daily --vault %s", shellQuote(root))},
	}
	return projection, nil
}

func planningSnapshotID(root, period string, t time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + period + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_snap_" + hex.EncodeToString(h[:])[:16]
}

func planningDecisionID(root, period string, t time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + period + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_dec_" + hex.EncodeToString(h[:])[:16]
}

func savePlanningSnapshot(root string, snapshot *domain.PlanningSnapshot) (string, error) {
	dir, err := safeJoin(root, ".pinax/planning/snapshots")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "snapshots", snapshot.SnapshotID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	snapshot.SavedPath = rel
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	return rel, os.WriteFile(path, payload, 0o644)
}

func savePlanningActionDraft(root string, draft *domain.PlanningActionDraft) (string, error) {
	dir, err := safeJoin(root, ".pinax/planning/actions")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "actions", draft.ActionID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	draft.SavedPath = rel
	payload, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	return rel, os.WriteFile(path, payload, 0o644)
}
