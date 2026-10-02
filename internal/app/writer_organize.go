package app

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// Supervised writer organize projection (pinax-human-reading-write-v1 W04).
//
// The human session projects the existing organize plan/apply semantics:
// plan generates and SAVES a plan (the host reviews plan_id before any
// apply), apply consumes the plan id as-is under the approval_and_snapshot
// gate. The host gate is the writer.organizer scope; the projection sets
// Yes so the CLI approval flag cannot be smuggled through request fields,
// and always snapshots before applying.

// WriterOrganizePlan generates a saved organize plan for the human session.
// note_refs present scopes the plan to exactly those objects; omitted means
// the whole-vault plan. Either way the plan is saved under its plan_id so
// apply can reference it — a preview-only surface would be unusable for the
// host confirmation flow.
func (s *Service) WriterOrganizePlan(ctx context.Context, root string, noteRefs []string) (domain.Projection, error) {
	refs := make([]string, 0, len(noteRefs))
	for _, ref := range noteRefs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			// An EXPLICIT selection with blank entries is invalid: dropping
			// them would silently widen the plan to the whole vault.
			return errorProjection("writer.organize.plan", scopedOrganizeError("organize_selection_invalid")), scopedOrganizeError("organize_selection_invalid")
		}
		refs = append(refs, trimmed)
	}
	if len(refs) > 0 {
		return s.PlanScopedOrganize(ctx, ScopedOrganizeRequest{VaultPath: root, NoteRefs: refs, Save: true})
	}
	return s.SuggestOrganize(ctx, OrganizeSuggestRequest{VaultPath: root, Save: true})
}

// WriterOrganizeApply applies a saved plan id as-is. The approval gate is
// the writer.organizer scope (never a request field), and the projection
// always requests a snapshot first: an empty snapshot message defaults to
// one derived from the plan id so no apply path can skip the snapshot gate.
// Consumption is explicit on BOTH plan shapes: scoped plans are guarded by
// the scoped machinery (status stays "planned" until consumed), and
// legacy whole-vault plans get the same writer-layer guard — an applied or
// partial plan re-fails with organize_plan_not_planned instead of silently
// re-planning, and the consumed status is persisted back to the plan file.
func (s *Service) WriterOrganizeApply(ctx context.Context, root, planID, snapshotMessage string, allowStale bool) (domain.Projection, error) {
	const command = "writer.organize.apply"
	planID = strings.TrimSpace(planID)
	if planID == "" {
		err := scopedOrganizeError("plan_not_found")
		return errorProjection(command, err), err
	}
	plan, loadErr := loadOrganizePlan(root, planID)
	if loadErr != nil {
		if errors.Is(loadErr, fs.ErrNotExist) {
			notFound := &domain.CommandError{
				Code: "plan_not_found", Message: "Organize plan was not found",
				Hint: "Plan with the given id is not saved under .pinax/organize-plans",
			}
			return domain.NewErrorProjection(command, notFound), notFound
		}
		// Corrupted plan JSON and other raw load errors flow through the
		// frozen-table guard at the HTTP boundary (writer_unavailable).
		return errorProjection(command, loadErr), loadErr
	}
	if plan.SchemaVersion != domain.ScopedOrganizePlanSchema && plan.Status != "planned" {
		consumed := scopedOrganizeError("organize_plan_not_planned")
		projection := errorProjection(command, consumed)
		projection.Data = map[string]any{"plan_id": plan.PlanID, "status": plan.Status}
		return projection, consumed
	}
	if strings.TrimSpace(snapshotMessage) == "" {
		snapshotMessage = "Before writer organize " + planID
	}
	projection, err := s.ApplyOrganize(ctx, ApplyRequest{
		VaultPath: root, PlanID: planID, Yes: true,
		SnapshotMessage: snapshotMessage, AllowStale: allowStale,
	})
	if plan.SchemaVersion == domain.ScopedOrganizePlanSchema {
		// The scoped machinery owns its own consumption state.
		if projection.Command == "organize.apply" {
			projection.Command = command
		}
		return projection, err
	}
	// Legacy whole-vault plan: persist the consumption outcome so a replay
	// re-fails explicitly on the guard above.
	consumed := plan
	if err == nil {
		consumed.Status = "applied"
	} else if applied := projection.Facts["applied"]; applied != "" && applied != "0" {
		consumed.Status = "partial"
	} else {
		consumed.Status = "failed"
	}
	_ = saveOrganizePlan(root, &consumed)
	if projection.Command == "organize.apply" {
		projection.Command = command
	}
	return projection, err
}
