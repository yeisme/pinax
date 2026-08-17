package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/identity"
	"github.com/yeisme/pinax/internal/markdownnote"
	"github.com/yeisme/pinax/internal/records"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

type IdentityMigrationRequest struct {
	VaultPath string
	Save      bool
}

func (s *Service) RecordIdentityAudit(ctx context.Context, req IdentityMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.identity.audit", err), err
	}
	report, _, err := s.inspectIdentityMigration(ctx, root, false)
	if err != nil {
		return errorProjection("record.identity.audit", err), err
	}
	projection := domain.NewProjection("record.identity.audit", "Identity audit completed.")
	setIdentityAuditFacts(&projection, report)
	projection.Facts["writes"] = "false"
	projection.Data = report
	if len(report.Issues) > 0 {
		projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax record identity plan --vault %s --json", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) RecordIdentityPlan(ctx context.Context, req IdentityMigrationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.identity.plan", err), err
	}
	report, operations, err := s.inspectIdentityMigration(ctx, root, true)
	if err != nil {
		return errorProjection("record.identity.plan", err), err
	}
	plan := domain.IdentityMigrationPlan{
		SchemaVersion: domain.IdentityMigrationPlanSchemaVersion,
		PlanID:        identityMigrationPlanID(report, operations),
		CreatedAt:     s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00"),
		VaultPath:     root,
		Issues:        report.Issues,
		Operations:    operations,
	}
	if req.Save {
		plan.SavedPath = filepath.ToSlash(filepath.Join(".pinax", "records", "identity-migrations", plan.PlanID+".json"))
		if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(plan.SavedPath)), plan); err != nil {
			return errorProjection("record.identity.plan", err), err
		}
	}
	projection := domain.NewProjection("record.identity.plan", "Identity migration plan generated.")
	setIdentityAuditFacts(&projection, report)
	projection.Facts["operations"] = fmt.Sprint(len(operations))
	projection.Facts["writes"] = fmt.Sprint(req.Save)
	projection.Facts["plan_id"] = plan.PlanID
	if plan.SavedPath != "" {
		projection.Facts["saved_path"] = plan.SavedPath
		projection.Evidence = []string{plan.SavedPath}
	}
	projection.Data = plan
	return projection, nil
}

func (s *Service) inspectIdentityMigration(ctx context.Context, root string, allocate bool) (domain.IdentityAuditReport, []domain.IdentityMigrationOperation, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return domain.IdentityAuditReport{}, nil, err
	}
	notes = ordinaryNotes(notes)
	report := domain.IdentityAuditReport{SchemaVersion: domain.IdentityAuditSchemaVersion, VaultPath: root, Notes: len(notes), Issues: []domain.IdentityIssue{}}
	state, replayErr := records.NewService(root).ReplayReadOnly(ctx)
	if replayErr != nil {
		if domain.ErrorCode(replayErr) != "record_path_collision" {
			return domain.IdentityAuditReport{}, nil, replayErr
		}
		report.PathCollisions++
		report.Issues = append(report.Issues, domain.IdentityIssue{Code: "record_path_collision", Severity: "error", Message: "Ledger contains multiple active objects at one path."})
		state = domain.LedgerState{Records: map[string]domain.NoteRecord{}}
	}
	byID := map[string][]domain.Note{}
	authoritativeByPath := map[string]string{}
	for _, note := range notes {
		id := strings.TrimSpace(note.ID)
		if id != "" {
			byID[id] = append(byID[id], note)
		}
		switch identity.Classify(id) {
		case identity.IDClassCanonical:
			report.Canonical++
		case identity.IDClassLegacy:
			report.Legacy++
		default:
			report.Missing++
			report.Issues = append(report.Issues, domain.IdentityIssue{Code: "identity_missing", Severity: "warning", LegacyID: id, Path: note.Path, Message: "Note has no canonical object identity."})
		}
		if recordID := recordIDByPath(state, note.Path); recordID != "" {
			authoritativeByPath[note.Path] = recordID
			if recordID != id {
				report.Conflicts++
				report.Issues = append(report.Issues, domain.IdentityIssue{Code: "record_frontmatter_mismatch", Severity: "error", ObjectID: recordID, LegacyID: id, Path: note.Path, Message: "Ledger identity differs from note frontmatter."})
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	duplicatePaths := map[string]bool{}
	for _, id := range ids {
		paths := notePaths(byID[id])
		if identity.Classify(id) == identity.IDClassLegacy {
			report.Issues = append(report.Issues, domain.IdentityIssue{Code: "identity_legacy", Severity: "warning", LegacyID: id, Path: paths[0], RelatedPaths: paths, Message: "Legacy note identity requires migration to UUIDv7."})
		}
		if len(paths) > 1 {
			report.Duplicates++
			report.Issues = append(report.Issues, domain.IdentityIssue{Code: "identity_duplicate", Severity: "error", ObjectID: id, LegacyID: id, Path: paths[0], RelatedPaths: paths, Message: "Multiple notes share one identity."})
			for _, path := range paths[1:] {
				duplicatePaths[path] = true
			}
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Path == report.Issues[j].Path {
			return report.Issues[i].Code < report.Issues[j].Code
		}
		return report.Issues[i].Path < report.Issues[j].Path
	})
	operations := []domain.IdentityMigrationOperation{}
	if allocate {
		for _, note := range notes {
			currentID := strings.TrimSpace(note.ID)
			toID := authoritativeByPath[note.Path]
			kind := "repair_frontmatter_mirror"
			reason := "Repair the portable frontmatter mirror from authoritative ledger identity."
			if toID == "" || toID == currentID {
				if identity.Classify(currentID) == identity.IDClassCanonical && !duplicatePaths[note.Path] {
					continue
				}
				allocated, allocateErr := s.allocateObjectID(identity.KindNote, root, "identity-migration:"+note.Path)
				if allocateErr != nil {
					return domain.IdentityAuditReport{}, nil, allocateErr
				}
				toID = allocated
				kind = "assign_object_id"
				reason = "Replace missing, legacy or duplicate note identity with canonical UUIDv7."
			}
			operations = append(operations, domain.IdentityMigrationOperation{OperationID: fmt.Sprintf("identity_%03d", len(operations)+1), Kind: kind, ObjectKind: "note", Path: note.Path, FromID: currentID, ToObjectID: toID, Status: "planned", Reason: reason})
		}
	}
	return report, operations, nil
}

func setIdentityAuditFacts(projection *domain.Projection, report domain.IdentityAuditReport) {
	projection.Facts["notes"] = fmt.Sprint(report.Notes)
	projection.Facts["canonical"] = fmt.Sprint(report.Canonical)
	projection.Facts["legacy"] = fmt.Sprint(report.Legacy)
	projection.Facts["missing"] = fmt.Sprint(report.Missing)
	projection.Facts["duplicates"] = fmt.Sprint(report.Duplicates)
	projection.Facts["conflicts"] = fmt.Sprint(report.Conflicts)
	projection.Facts["path_collisions"] = fmt.Sprint(report.PathCollisions)
	projection.Facts["issues"] = fmt.Sprint(len(report.Issues))
}

func notePaths(notes []domain.Note) []string {
	paths := make([]string, 0, len(notes))
	for _, note := range notes {
		paths = append(paths, note.Path)
	}
	sort.Strings(paths)
	return paths
}

func identityMigrationPlanID(report domain.IdentityAuditReport, operations []domain.IdentityMigrationOperation) string {
	var parts []string
	for _, issue := range report.Issues {
		parts = append(parts, issue.Code+":"+issue.Path+":"+issue.LegacyID+":"+issue.ObjectID)
	}
	for _, operation := range operations {
		parts = append(parts, operation.Path+":"+operation.FromID+":"+operation.ToObjectID)
	}
	digest := strings.TrimPrefix(hashString(strings.Join(parts, "\n")), "sha256:")
	return "identity_plan_" + digest[:12]
}

type IdentityMigrationApplyRequest struct {
	VaultPath string
	PlanID    string
	Yes       bool
	Resume    bool
}

func (s *Service) RecordIdentityApply(ctx context.Context, req IdentityMigrationApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "identity migration apply requires --yes", Hint: "Review a saved plan, then rerun with --yes"}
		return domain.NewErrorProjection("record.identity.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("record.identity.apply", err), err
	}
	plan, err := loadIdentityMigrationPlan(root, req.PlanID)
	if err != nil {
		return errorProjection("record.identity.apply", err), err
	}
	receipt, receiptExists := loadIdentityMigrationReceipt(root, plan.PlanID)
	if receiptExists && receipt.Status == "completed" {
		return completedIdentityMigrationProjection(plan, receipt), nil
	}
	if receiptExists && !req.Resume {
		err := &domain.CommandError{Code: "migration_incomplete", Message: "identity migration has an incomplete receipt", Hint: fmt.Sprintf("pinax record identity apply --vault %s --plan %s --yes --resume", shellQuote(root), shellQuote(plan.PlanID))}
		projection := domain.NewErrorProjection("record.identity.apply", err)
		projection.Facts["receipt"] = receipt.SavedPath
		return projection, err
	}
	if err := ensureIdentityMigrationPlanFresh(root, plan); err != nil {
		return errorProjection("record.identity.apply", err), err
	}
	now := s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00")
	if !receiptExists {
		receipt = domain.IdentityMigrationReceipt{SchemaVersion: domain.IdentityMigrationReceiptSchemaVersion, PlanID: plan.PlanID, Status: "running", Phase: "snapshot", StartedAt: now, UpdatedAt: now, CompletedPaths: []string{}, RestoreCommands: identityMigrationRestoreCommands(root, plan), SavedPath: identityMigrationReceiptPath(plan.PlanID)}
		snapshot, snapshotErr := s.versionBackend.Snapshot(ctx, versionSnapshotRequest(root, plan.PlanID))
		if snapshotErr != nil {
			return failIdentityMigration(root, &receipt, "snapshot_failed", snapshotErr)
		}
		receipt.SnapshotID = snapshot.SnapshotID
		receipt.Phase = "frontmatter"
		receipt.UpdatedAt = s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00")
		if err := saveIdentityMigrationReceipt(root, receipt); err != nil {
			return errorProjection("record.identity.apply", err), err
		}
	}
	completed := make(map[string]bool, len(receipt.CompletedPaths))
	for _, path := range receipt.CompletedPaths {
		completed[path] = true
	}
	for _, operation := range plan.Operations {
		if completed[operation.Path] {
			continue
		}
		if err := applyIdentityMigrationOperation(ctx, root, operation); err != nil {
			return failIdentityMigration(root, &receipt, "frontmatter_or_ledger_failed", err)
		}
		receipt.CompletedPaths = append(receipt.CompletedPaths, operation.Path)
		sort.Strings(receipt.CompletedPaths)
		receipt.Phase = "applying"
		receipt.UpdatedAt = s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00")
		if err := saveIdentityMigrationReceipt(root, receipt); err != nil {
			return errorProjection("record.identity.apply", err), err
		}
	}
	receipt.Status = "completed"
	receipt.Phase = "completed"
	receipt.UpdatedAt = s.currentTimeUTC().Format("2006-01-02T15:04:05Z07:00")
	receipt.ErrorCode = ""
	receipt.ErrorMessage = ""
	if err := saveIdentityMigrationReceipt(root, receipt); err != nil {
		return errorProjection("record.identity.apply", err), err
	}
	return completedIdentityMigrationProjection(plan, receipt), nil
}

func applyIdentityMigrationOperation(ctx context.Context, root string, operation domain.IdentityMigrationOperation) error {
	path, err := safeJoin(root, operation.Path)
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	patched, _ := patchFrontmatterFields(string(payload), map[string]string{"note_id": operation.ToObjectID})
	if patched != string(payload) {
		if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
			return err
		}
	}
	note := parseNote(operation.Path, patched)
	aliases := []string{}
	if strings.TrimSpace(operation.FromID) != "" && operation.FromID != operation.ToObjectID {
		aliases = append(aliases, operation.FromID)
	}
	_, err = records.NewService(root).AppendEvent(ctx, domain.RecordEvent{Kind: domain.RecordEventNoteIdentityMigrated, IdempotencyKey: "identity-migrate:" + operation.OperationID + ":" + operation.ToObjectID, ObjectID: operation.ToObjectID, ObjectKind: operation.ObjectKind, CurrentPath: operation.Path, LegacyAliases: aliases, Title: note.Title, ContentRevision: domain.ContentRevision{Hash: hashString(note.Title + "\x00" + note.Body), Size: int64(len(note.Body))}, Evidence: []string{"source=identity_migration"}})
	return err
}

func ensureIdentityMigrationPlanFresh(root string, plan domain.IdentityMigrationPlan) error {
	for _, operation := range plan.Operations {
		path, err := safeJoin(root, operation.Path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return &domain.CommandError{Code: "plan_stale", Message: "identity migration plan path no longer exists", Hint: "Generate and save a fresh identity migration plan"}
		}
		meta, _, _ := markdownnote.ParseFrontmatter(payload)
		currentID := strings.TrimSpace(meta["note_id"])
		if currentID == operation.ToObjectID {
			continue
		}
		if currentID != strings.TrimSpace(operation.FromID) {
			return &domain.CommandError{Code: "plan_stale", Message: "identity migration plan does not match current frontmatter", Hint: "Run pinax record identity plan --save again"}
		}
	}
	return nil
}

func loadIdentityMigrationPlan(root, planRef string) (domain.IdentityMigrationPlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.IdentityMigrationPlan{}, &domain.CommandError{Code: "plan_required", Message: "identity migration apply requires a saved plan", Hint: "Run pinax record identity plan --save"}
	}
	rel := planRef
	if !strings.Contains(planRef, "/") && !strings.HasSuffix(planRef, ".json") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "records", "identity-migrations", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.IdentityMigrationPlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.IdentityMigrationPlan{}, &domain.CommandError{Code: "identity_migration_plan_not_found", Message: "saved identity migration plan was not found", Hint: "Run pinax record identity plan --save"}
	}
	var plan domain.IdentityMigrationPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.IdentityMigrationPlan{}, err
	}
	if plan.SchemaVersion != domain.IdentityMigrationPlanSchemaVersion {
		return domain.IdentityMigrationPlan{}, &domain.CommandError{Code: "identity_migration_plan_schema_invalid", Message: "identity migration plan schema is not supported"}
	}
	return plan, nil
}

func identityMigrationReceiptPath(planID string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "records", "identity-migrations", "receipts", planID+".json"))
}

func loadIdentityMigrationReceipt(root, planID string) (domain.IdentityMigrationReceipt, bool) {
	path, err := safeJoin(root, identityMigrationReceiptPath(planID))
	if err != nil {
		return domain.IdentityMigrationReceipt{}, false
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.IdentityMigrationReceipt{}, false
	}
	var receipt domain.IdentityMigrationReceipt
	if json.Unmarshal(payload, &receipt) != nil || receipt.SchemaVersion != domain.IdentityMigrationReceiptSchemaVersion {
		return domain.IdentityMigrationReceipt{}, false
	}
	return receipt, true
}

func saveIdentityMigrationReceipt(root string, receipt domain.IdentityMigrationReceipt) error {
	return writeJSONAsset(filepath.Join(root, filepath.FromSlash(receipt.SavedPath)), receipt)
}

func failIdentityMigration(root string, receipt *domain.IdentityMigrationReceipt, code string, err error) (domain.Projection, error) {
	receipt.Status = "failed"
	receipt.ErrorCode = code
	receipt.ErrorMessage = err.Error()
	receipt.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	_ = saveIdentityMigrationReceipt(root, *receipt)
	commandErr := &domain.CommandError{Code: code, Message: "identity migration did not complete", Hint: fmt.Sprintf("Resume with pinax record identity apply --vault %s --plan %s --yes --resume", shellQuote(root), shellQuote(receipt.PlanID))}
	projection := domain.NewErrorProjection("record.identity.apply", commandErr)
	projection.Facts["receipt"] = receipt.SavedPath
	return projection, commandErr
}

func completedIdentityMigrationProjection(plan domain.IdentityMigrationPlan, receipt domain.IdentityMigrationReceipt) domain.Projection {
	projection := domain.NewProjection("record.identity.apply", "Identity migration completed.")
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["status"] = receipt.Status
	projection.Facts["applied"] = fmt.Sprint(len(receipt.CompletedPaths))
	projection.Facts["snapshot_id"] = receipt.SnapshotID
	projection.Facts["receipt"] = receipt.SavedPath
	projection.Evidence = []string{receipt.SavedPath}
	projection.Actions = []domain.Action{{Name: "audit", Command: fmt.Sprintf("pinax record identity audit --vault %s --json", shellQuote(plan.VaultPath))}}
	projection.Data = receipt
	return projection
}

func identityMigrationRestoreCommands(root string, plan domain.IdentityMigrationPlan) []string {
	commands := make([]string, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		commands = append(commands, fmt.Sprintf("pinax version restore %s --vault %s --revision <snapshot_id> --plan --json", shellQuote(operation.Path), shellQuote(root)))
	}
	return commands
}

func versionSnapshotRequest(root, planID string) pinaxversion.SnapshotRequest {
	return pinaxversion.SnapshotRequest{Root: root, Message: "Before identity migration " + planID}
}
