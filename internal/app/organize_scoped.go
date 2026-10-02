package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
)

type ScopedOrganizeRequest struct {
	VaultPath string
	NoteRefs  []string
	Save      bool
}

func scopedOrganizeError(code string) error {
	return &domain.CommandError{Code: code, Message: "Selected organization requires an unchanged, unambiguous selection and unoccupied targets", Hint: "Inspect the original plan, then generate and review a new plan with explicit --note selections"}
}

func selectedOrganizeNotes(root string, refs []string) ([]domain.Note, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return nil, scopedOrganizeError("organize_selection_invalid")
	}
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	selected := map[string]domain.Note{}
	for _, ref := range refs {
		if ref == "" || ref != strings.TrimSpace(ref) || filepath.IsAbs(ref) || strings.Contains(ref, "\\") || filepath.ToSlash(filepath.Clean(ref)) != ref || ref == ".." || strings.HasPrefix(ref, "../") {
			return nil, scopedOrganizeError("organize_selection_invalid")
		}
		matches := []domain.Note{}
		for _, n := range notes {
			if n.ID == ref || n.Path == ref || strings.TrimPrefix(n.Path, "notes/") == ref {
				matches = append(matches, n)
			}
		}
		if len(matches) != 1 {
			return nil, scopedOrganizeError("organize_selection_invalid")
		}
		n := matches[0]
		if identity.Classify(n.ID) != identity.IDClassCanonical {
			return nil, &domain.CommandError{Code: "organize_identity_required", Message: "Selected notes require canonical identity", Hint: "Run pinax record adopt --plan on the vault owner; planning never adopts notes"}
		}
		count := 0
		for _, other := range notes {
			if other.ID == n.ID {
				count++
			}
		}
		reservedRoot := !strings.Contains(n.Path, "/") && !isOrganizeRootNoteCandidate(n.Path)
		if count != 1 || reservedRoot || isSystemIndexNote(n) || isSystemJournalNote(n) || n.Status == "archived" || n.Status == "discarded" || n.Status == "system" {
			return nil, scopedOrganizeError("organize_selection_invalid")
		}
		if err := collaborationSafePath(root, n.Path); err != nil {
			return nil, err
		}
		selected[n.ID] = n
	}
	if len(selected) > 20 {
		return nil, scopedOrganizeError("organize_selection_invalid")
	}
	out := make([]domain.Note, 0, len(selected))
	for _, n := range selected {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func scopedOrganizeCommand(root string, selection []domain.OrganizeSelection) string {
	cmd := "pinax organize plan --vault " + shellQuote(root)
	for _, item := range selection {
		cmd += " --note " + shellQuote(item.ObjectID)
	}
	return cmd
}

// PlanScopedOrganize reuses the full-vault rule engine before restricting writes.
func (s *Service) PlanScopedOrganize(ctx context.Context, req ScopedOrganizeRequest) (domain.Projection, error) {
	const command = "organize.plan"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	selected, err := selectedOrganizeNotes(root, req.NoteRefs)
	if err != nil {
		return errorProjection(command, err), err
	}
	plan, err := buildOrganizePlan(root, true)
	if err != nil {
		return errorProjection(command, err), err
	}
	bindings, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return errorProjection(command, err), err
	}
	paths := map[string]bool{}
	for _, n := range selected {
		b := bindings[n.Path]
		plan.Selection = append(plan.Selection, domain.OrganizeSelection{ObjectID: n.ID, ObservedPath: n.Path, ExpectedRecordVersion: b.ExpectedRecordVersion, ExpectedContentRevision: b.ExpectedContentRevision})
		paths[n.Path] = true
	}
	operations := []domain.OrganizeOperation{}
	for _, op := range plan.Operations {
		if paths[op.Path] {
			operations = append(operations, op)
		}
	}
	plan.Operations = operations
	plan.SchemaVersion = domain.ScopedOrganizePlanSchema
	plan.PlanID += "-" + shortSyncManifestDigest(strings.Join(req.NoteRefs, "\x00"))
	plan.SourceCommand = scopedOrganizeCommand(root, plan.Selection)
	plan.SourceFacts = map[string]string{"selected": fmt.Sprint(len(selected))}
	for _, item := range plan.Selection {
		plan.SourceFacts[item.ObjectID] = item.ExpectedContentRevision.Hash
	}
	// Bind operation identity to the scoped plan, never a broader saved plan.
	for i := range plan.Operations {
		op := &plan.Operations[i]
		op.OperationID = organizeOperationID(plan.PlanID, domain.PlanOperation{Kind: op.Kind, Path: op.Path, Target: op.Target})
	}
	if req.Save {
		if err := saveScopedOrganizePlan(root, &plan); err != nil {
			return errorProjection(command, err), err
		}
	}
	p := domain.NewProjection(command, "Selected organization preview; only explicitly selected objects may change.")
	p.Facts["schema_version"], p.Facts["plan_id"] = plan.SchemaVersion, plan.PlanID
	p.Facts["selected"], p.Facts["operations"] = fmt.Sprint(len(plan.Selection)), fmt.Sprint(len(plan.Operations))
	p.Facts["local_write"], p.Facts["remote_write"] = fmt.Sprint(req.Save), "false"
	p.Data = plan
	if req.Save {
		p.Facts["saved_path"] = plan.SavedPath
		p.Actions = []domain.Action{{Name: "inspect", Command: "pinax pipeline show " + shellQuote(plan.PlanID) + " --vault " + shellQuote(root)}}
	} else {
		p.Actions = []domain.Action{{Name: "save", Command: plan.SourceCommand + " --save"}}
	}
	return p, nil
}

func saveScopedOrganizePlan(root string, plan *domain.OrganizePlan) error {
	if !paneRefPattern.MatchString(plan.PlanID) {
		return scopedOrganizeError("organize_plan_schema_invalid")
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "organize-plans", plan.PlanID+".json"))
	if err := collaborationSafePath(root, rel); err != nil {
		return err
	}
	plan.SavedPath = rel
	return writeJSONAsset(filepath.Join(root, rel), plan)
}

func validateScopedOrganizePlan(ctx context.Context, root string, plan *domain.OrganizePlan) error {
	if plan.Status != "planned" {
		return scopedOrganizeError("organize_plan_not_planned")
	}
	if plan.SchemaVersion != domain.ScopedOrganizePlanSchema || plan.SnapshotID != "" || len(plan.OperationResults) > 0 {
		return scopedOrganizeError("organize_selection_invalid")
	}
	if plan.VaultRoot != root || len(plan.Selection) == 0 || len(plan.Selection) > 20 {
		return scopedOrganizeError("organize_selection_invalid")
	}
	expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		return scopedOrganizeError("plan_stale")
	}
	refs := make([]string, 0, len(plan.Selection))
	seen := map[string]domain.OrganizeSelection{}
	for _, item := range plan.Selection {
		if _, ok := seen[item.ObjectID]; ok || item.ExpectedContentRevision.Hash == "" {
			return scopedOrganizeError("organize_selection_invalid")
		}
		seen[item.ObjectID] = item
		refs = append(refs, item.ObjectID)
	}
	if _, err := selectedOrganizeNotes(root, refs); err != nil {
		return scopedOrganizeError("plan_stale")
	}
	current := map[string]string{}
	for _, item := range plan.Selection {
		path, err := resolveBoundPlanObject(ctx, root, item.ObjectID, item.ObservedPath, item.ExpectedContentRevision)
		if err != nil {
			return err
		}
		current[item.ObjectID] = path
	}
	targets, operationIDs := map[string]bool{}, map[string]bool{}
	for i := range plan.Operations {
		op := &plan.Operations[i]
		item, ok := seen[op.ObjectID]
		if !ok || operationIDs[op.OperationID] || op.OperationID == "" || (op.Path != item.ObservedPath && op.Path != current[op.ObjectID]) || op.ObservedPath != item.ObservedPath || op.ExpectedContentRevision.Hash != item.ExpectedContentRevision.Hash || op.ExpectedContentRevision.Size != item.ExpectedContentRevision.Size {
			return scopedOrganizeError("organize_selection_invalid")
		}
		if op.Mode != "automatic" && op.Mode != "manual_review" {
			return scopedOrganizeError("organize_selection_invalid")
		}
		operationIDs[op.OperationID] = true
		op.Path = current[op.ObjectID]
		if op.Mode == "manual_review" || op.Status != "planned" {
			continue
		}
		switch op.Kind {
		case "tag_patch":
			if _, err := normalizeTagsForWrite(strings.Split(op.Target, ",")); err != nil {
				return err
			}
		case "status_patch":
			if op.Target != "active" {
				return scopedOrganizeError("organize_selection_invalid")
			}
		case "move":
			if targets[op.Target] {
				return scopedOrganizeError("note_path_conflict")
			}
			targets[op.Target] = true
			if err := scopedMoveTarget(root, op.Target); err != nil {
				return err
			}
		default:
			return scopedOrganizeError("organize_selection_invalid")
		}
	}
	return nil
}

func scopedMoveTarget(root, target string) error {
	if target == "" || filepath.IsAbs(target) || filepath.ToSlash(filepath.Clean(target)) != target {
		return scopedOrganizeError("organize_selection_invalid")
	}
	if err := collaborationSafePath(root, target); err != nil {
		return err
	}
	path, err := safeJoin(root, target)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		return scopedOrganizeError("note_path_conflict")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// No overwrite fallback: unsupported hard links leave the source untouched.
func moveScopedNote(root, from, to string) error {
	if err := scopedMoveTarget(root, to); err != nil {
		return err
	}
	if err := collaborationSafePath(root, from); err != nil {
		return err
	}
	source, err := safeJoin(root, from)
	if err != nil {
		return err
	}
	target, err := safeJoin(root, to)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Link(source, target); err != nil {
		return scopedOrganizeError("organize_move_failed")
	}
	// If unlink fails, retain both names and report an uncertain partial result.
	if err := os.Remove(source); err != nil {
		return scopedOrganizeError("organize_move_incomplete")
	}
	return nil
}

func (s *Service) applyScopedOrganize(ctx context.Context, root string, req ApplyRequest, plan domain.OrganizePlan) (domain.Projection, error) {
	const command = "organize.apply"
	if req.AllowStale {
		err := scopedOrganizeError("scoped_plan_override_forbidden")
		return errorProjection(command, err), err
	}
	if err := validateScopedOrganizePlan(ctx, root, &plan); err != nil {
		return errorProjection(command, err), err
	}
	for _, rel := range []string{".pinax/sync/operation.lock", ".pinax/records", ".pinax/receipts", ".pinax/organize-plans", ".pinax/version/snapshots", ".pinax/version/objects", ".pinax/last_snapshot"} {
		if err := collaborationSafePath(root, rel); err != nil {
			return errorProjection(command, err), err
		}
	}
	lock, err := syncdaemon.AcquireOperationLock(root, command)
	if err != nil {
		return errorProjection(command, err), err
	}
	defer lock.Release()
	// Reload under the shared owner lock so two processes cannot consume one plan.
	plan, err = loadOrganizePlan(root, req.PlanID)
	if err != nil {
		return errorProjection(command, err), err
	}
	if err = validateScopedOrganizePlan(ctx, root, &plan); err != nil {
		return errorProjection(command, err), err
	}
	beforeByPath, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return errorProjection(command, err), err
	}
	before := map[string]managedObjectPlanBinding{}
	for _, item := range plan.Selection {
		for _, b := range beforeByPath {
			if b.ObjectID == item.ObjectID {
				b.ExpectedContentRevision = item.ExpectedContentRevision
				before[item.ObjectID] = b
			}
		}
	}
	snapshot, err := s.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: defaultString(req.SnapshotMessage, "Before selected organization "+plan.PlanID)})
	if err != nil {
		return errorProjection(command, err), err
	}
	plan.SnapshotID = snapshot.Facts["snapshot_id"]
	if plan.SnapshotID == "" {
		err = scopedOrganizeError("snapshot_required")
		return errorProjection(command, err), err
	}
	plan.Status = "applying"
	for _, op := range plan.Operations {
		plan.OperationResults = append(plan.OperationResults, domain.OrganizeOperationResult{OperationID: op.OperationID, ObjectID: op.ObjectID, Status: "pending"})
	}
	if err = saveScopedOrganizePlan(root, &plan); err != nil {
		return errorProjection(command, err), err
	}
	current := map[string]managedObjectPlanBinding{}
	for k, v := range before {
		current[k] = v
	}
	changed := []string{}
	applied := 0
	for i, op := range plan.Operations {
		result := &plan.OperationResults[i]
		if op.Mode == "manual_review" || op.Status != "planned" {
			result.Status = "skipped"
			continue
		}
		b := current[op.ObjectID]
		path, runErr := resolveBoundPlanObject(ctx, root, b.ObjectID, b.ObservedPath, b.ExpectedContentRevision)
		if runErr == nil {
			runErr = ctx.Err()
		}
		if runErr == nil {
			runErr = collaborationSafePath(root, path)
		}
		if runErr == nil {
			op.Path = path
			// Include possible writes even when the filesystem outcome is ambiguous.
			changed = append(changed, path)
			if op.Kind == "move" {
				changed = append(changed, op.Target)
				runErr = moveScopedNote(root, path, op.Target)
				if runErr == nil {
					b.ObservedPath = op.Target
					content, readErr := os.ReadFile(filepath.Join(root, op.Target))
					runErr = readErr
					if runErr == nil {
						note := parseNote(op.Target, string(content))
						if note.ID != op.ObjectID {
							runErr = scopedOrganizeError("plan_stale")
						} else {
							_, runErr = appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, plan.PlanID+":"+op.OperationID, note, path)
						}
					}
				}
			} else {
				runErr = applyOrganizeMetadataOperation(root, domain.PlanOperation{Kind: op.Kind, Path: path, Target: op.Target})
			}
		}
		if runErr == nil {
			b.ExpectedContentRevision, runErr = fileContentRevision(root, b.ObservedPath)
			current[op.ObjectID] = b
		}
		if runErr != nil {
			result.Status, result.ErrorCode = "failed", defaultString(domain.ErrorCode(runErr), "organize_operation_failed")
			if op.Kind == "move" && b.ObservedPath == op.Target {
				result.Status = "reconcile_required"
			}
			if domain.ErrorCode(runErr) == "organize_move_incomplete" {
				result.Status = "reconcile_required"
			}
			err = runErr
			break
		}
		result.Status = "applied"
		applied++
		if err = saveScopedOrganizePlan(root, &plan); err != nil {
			break
		}
	}
	plan.Status = "applied"
	if err != nil {
		plan.Status = "failed"
		if len(changed) > 0 {
			plan.Status = "partial"
		}
	}
	p := domain.NewProjection(command, "Selected organization finished; inspect the original plan before further changes.")
	p.Facts["plan_id"], p.Facts["snapshot_id"], p.Facts["applied"] = plan.PlanID, plan.SnapshotID, fmt.Sprint(applied)
	p.Facts["local_write"], p.Facts["remote_write"] = "true", "false"
	p.Actions = []domain.Action{{Name: "inspect", Command: "pinax pipeline show " + shellQuote(plan.PlanID) + " --vault " + shellQuote(root)}, {Name: "history", Command: "pinax version history --vault " + shellQuote(root)}}
	receipt, receiptErr := writeObjectApplyReceiptOutcome(context.WithoutCancel(ctx), root, command, plan.PlanID, plan.SnapshotID, before, changed, plan.Status, plan.OperationResults)
	if err != nil {
		p.Facts["changed_paths_status"] = "potential"
	}
	if receiptErr == nil {
		addApplyReceiptProjection(&p, receipt)
	} else if plan.Status == "applied" {
		plan.Status = "partial"
	}
	stateErr := saveScopedOrganizePlan(root, &plan)
	if receiptErr != nil || stateErr != nil {
		if stateErr != nil && plan.Status == "applied" {
			plan.Status = "partial"
		}
		err = scopedOrganizeError("organize_evidence_incomplete")
		p.Facts["evidence_complete"] = "false"
	} else {
		p.Facts["evidence_complete"] = "true"
	}
	if err != nil {
		p.Status = "failed"
		if len(changed) > 0 {
			p.Status = "partial"
		}
		p.Error = &domain.CommandError{Code: defaultString(domain.ErrorCode(err), "organize_operation_failed"), Message: "Organization did not complete cleanly; inspect the original plan and snapshot, do not retry it"}
	}
	p.Facts["index_status"] = "not_refreshed"
	p.Actions = append(p.Actions, domain.Action{Name: "refresh_index", Command: "pinax index refresh --vault " + shellQuote(root)})
	p.Data = map[string]any{"plan_id": plan.PlanID, "status": plan.Status, "operation_results": plan.OperationResults, "receipt": receipt}
	return p, err
}
