package planningops

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func MaxCommitments(period domain.PlanningPeriod) int {
	switch period {
	case domain.PlanningWeekly:
		return 7
	case domain.PlanningMonthly:
		return 15
	default:
		return 3
	}
}

func AddCapacityRisk(snapshot *domain.PlanningSnapshot, decision *domain.PlanningDecision, notes, maxCommitments int) {
	if notes <= maxCommitments*5 {
		return
	}
	snapshot.Risks = append(snapshot.Risks, domain.PlanningRisk{Code: "OVER_CAPACITY", Message: "vault note count may exceed planning capacity", Evidence: []string{fmt.Sprintf("notes=%d max_commitments=%d", notes, maxCommitments)}})
	decision.Reasons = append(decision.Reasons, domain.PlanningReason{Kind: "capacity", Summary: fmt.Sprintf("vault has %d notes; prioritize %d items", notes, maxCommitments)})
}

func ParsePeriod(value string) (domain.PlanningPeriod, error) {
	switch domain.PlanningPeriod(strings.TrimSpace(value)) {
	case domain.PlanningDaily:
		return domain.PlanningDaily, nil
	case domain.PlanningWeekly:
		return domain.PlanningWeekly, nil
	case domain.PlanningMonthly:
		return domain.PlanningMonthly, nil
	default:
		return "", &domain.CommandError{Code: "invalid_planning_period", Message: "Unsupported planning period", Hint: "Use daily, weekly, or monthly"}
	}
}

func PreviewData(projection domain.Projection) (domain.PlanningSnapshot, domain.PlanningDecision, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "Planning preview data is missing"}
	}
	snapshot, ok := data["snapshot"].(domain.PlanningSnapshot)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "Planning snapshot data is missing"}
	}
	decision, ok := data["decision"].(domain.PlanningDecision)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "Planning decision data is missing"}
	}
	return snapshot, decision, nil
}

func BuildActionDraft(period string, snapshot domain.PlanningSnapshot, decision domain.PlanningDecision, now time.Time) domain.PlanningActionDraft {
	draftID := actionIDFromRefs(period, snapshot.SnapshotID, decision.DecisionID, now)
	draft := domain.PlanningActionDraft{SchemaVersion: "taskbridge.actions.v1", ActionID: draftID, SourcePeriod: period, SourceDecision: decision.DecisionID, SourceSnapshot: snapshot.SnapshotID, RequiresConfirmation: false, Tasks: []domain.ActionDraftTask{}, EvidenceRefs: []string{"snapshot:" + snapshot.SnapshotID, "decision:" + decision.DecisionID}, CreatedAt: now.Format(time.RFC3339)}
	reason := actionReason(decision)
	for i, taskID := range decision.Deferred {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		draft.Tasks = append(draft.Tasks, domain.ActionDraftTask{ActionID: taskActionID(draftID, taskID, i), TaskID: taskID, Kind: "defer", Reason: reason, RequiresConfirmation: true})
	}
	draft.RequiresConfirmation = len(draft.Tasks) > 0
	return draft
}

func actionReason(decision domain.PlanningDecision) string {
	for _, reason := range decision.Reasons {
		if strings.TrimSpace(reason.Summary) != "" {
			return reason.Summary
		}
	}
	return "Planning recommendations should be confirmed by TaskBridge before writing tasks."
}

func actionIDFromRefs(period, snapshotID, decisionID string, t time.Time) string {
	h := sha1.Sum([]byte(period + "\x00" + snapshotID + "\x00" + decisionID + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_act_" + hex.EncodeToString(h[:])[:16]
}

func taskActionID(draftID, taskID string, index int) string {
	h := sha1.Sum([]byte(draftID + "\x00" + taskID + "\x00" + fmt.Sprint(index)))
	return "act_" + hex.EncodeToString(h[:])[:16]
}
