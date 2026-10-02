package app

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
)

// WriterOrganizePersistTerminal persists the terminal consumption state.
// It is a var so fault-injection tests can make exactly the terminal save
// fail (after the writes landed); production always uses saveOrganizePlan.
var WriterOrganizePersistTerminal = saveOrganizePlan

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
	if strings.TrimSpace(snapshotMessage) == "" {
		snapshotMessage = "Before writer organize " + planID
	}
	if plan.SchemaVersion == domain.ScopedOrganizePlanSchema {
		// The scoped machinery owns its own lock, in-lock reload and
		// consumption state.
		projection, err := s.ApplyOrganize(ctx, ApplyRequest{
			VaultPath: root, PlanID: planID, Yes: true,
			SnapshotMessage: snapshotMessage, AllowStale: allowStale,
		})
		if projection.Command == "organize.apply" {
			projection.Command = command
		}
		return projection, err
	}
	// Legacy whole-vault plan: enforce the same consumption discipline as
	// the scoped machinery at the writer layer — one owner lock, an in-lock
	// reload of the consumption state, an "applying" marker persisted
	// BEFORE any write (a crash mid-apply leaves a non-planned plan that a
	// replay explicitly rejects), and a terminal state whose persistence
	// failure is surfaced instead of silently returning success.
	lock, lockErr := syncdaemon.AcquireOperationLock(root, "organize.apply")
	if lockErr != nil {
		return errorProjection(command, lockErr), lockErr
	}
	defer lock.Release()
	plan, loadErr = loadOrganizePlan(root, planID)
	if loadErr != nil {
		if errors.Is(loadErr, fs.ErrNotExist) {
			notFound := &domain.CommandError{Code: "plan_not_found", Message: "Organize plan was not found"}
			return domain.NewErrorProjection(command, notFound), notFound
		}
		return errorProjection(command, loadErr), loadErr
	}
	if plan.Status != "planned" {
		consumed := scopedOrganizeError("organize_plan_not_planned")
		projection := errorProjection(command, consumed)
		projection.Data = map[string]any{"plan_id": plan.PlanID, "status": plan.Status}
		return projection, consumed
	}
	plan.Status = "applying"
	if saveErr := saveOrganizePlan(root, &plan); saveErr != nil {
		// Nothing has been written yet: fail closed before any mutation.
		saveFailed := &domain.CommandError{
			Code: "plan_save_failed", Message: "Writer organize could not persist the applying state",
			Hint: "No writes were performed; inspect the plan file and re-issue the same plan id",
		}
		return domain.NewErrorProjection(command, saveFailed), saveFailed
	}
	projection, err := s.ApplyOrganize(ctx, ApplyRequest{
		VaultPath: root, PlanID: planID, Yes: true,
		SnapshotMessage: snapshotMessage, AllowStale: allowStale,
	})
	applied := 0
	if raw, present := projection.Facts["applied"]; present {
		applied, _ = strconv.Atoi(raw)
	}
	if err == nil {
		plan.Status = "applied"
	} else if applied > 0 {
		plan.Status = "partial"
	} else {
		plan.Status = "failed"
	}
	if projection.Command == "organize.apply" {
		projection.Command = command
	}
	if saveErr := WriterOrganizePersistTerminal(root, &plan); saveErr != nil {
		// The writes may have landed but the consumption record could not
		// be persisted: never return plain success — set the projection
		// error explicitly so the HTTP boundary serves THIS outcome (with
		// all recovery facts) under reconcile_required, overriding any
		// stale apply error from below.
		reconcile := &domain.CommandError{
			Code: "reconcile_required", Message: "Writer organize finished but the plan consumption state could not be persisted",
			Hint: "Inspect the plan id and the apply receipt before any re-issue; never re-issue blindly",
		}
		projection.Facts["plan_consumption"] = "unconfirmed"
		projection.Facts["plan_status_intended"] = plan.Status
		projection.Error = reconcile
		return projection, reconcile
	}
	return projection, err
}
