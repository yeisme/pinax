package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

const folderRenameCapabilityID = "folder.rename"

type folderOperationResult struct {
	SourcePath     string `json:"source_path"`
	TargetPath     string `json:"target_path"`
	SnapshotID     string `json:"snapshot_id"`
	RevisionBefore string `json:"revision_before"`
	RevisionAfter  string `json:"revision_after,omitempty"`
	UpdatedNotes   string `json:"updated_notes,omitempty"`
}

type folderOperationReceipt struct {
	SourcePath     string `json:"source_path"`
	TargetPath     string `json:"target_path"`
	SnapshotID     string `json:"snapshot_id"`
	RevisionBefore string `json:"revision_before"`
	RevisionAfter  string `json:"revision_after"`
	Proof          string `json:"proof"`
}

func folderRenameRevision(root, sourcePath, targetPath string) (string, string, error) {
	vaultHash, err := versionVaultHash(root)
	if err != nil {
		return "", "", err
	}
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return "", "", err
	}
	snapshotID := latestVersionSnapshotID(root)
	digest, err := operation.CanonicalDigest(map[string]any{
		"capability_id": folderRenameCapabilityID,
		"source_path":   sourcePath, "target_path": targetPath,
		"snapshot_id": snapshotID, "vault_hash": vaultHash, "folder_registry": registry,
	})
	return digest, snapshotID, err
}

// RenameFolderRemote enables operation recovery only for callers that send
// both identity values. Legacy callers that send neither retain the direct
// route behavior and are not silently reinterpreted.
func (s *Service) RenameFolderRemote(ctx context.Context, req FolderOperationRequest, identity RemoteOperationIdentity) (domain.Projection, error) {
	if req.DryRun {
		projection, err := s.RenameFolder(ctx, req)
		projection.Facts["operation_status"] = "planned"
		projection.Facts["remote_write"] = "false"
		projection.Facts["idempotent_replay"] = "false"
		return projection, err
	}
	operationID := strings.TrimSpace(identity.OperationID)
	idempotencyKey := strings.TrimSpace(identity.IdempotencyKey)
	if operationID == "" && idempotencyKey == "" {
		return s.RenameFolder(ctx, req)
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(folderRenameCapabilityID, err), err
	}
	req.VaultPath = root
	if strings.TrimSpace(req.ExpectedRevision) == "" {
		commandErr := &domain.CommandError{Code: "expected_revision_required", Message: "Remote folder rename requires expected_revision", Hint: "Run folder rename with --dry-run and reuse revision_before"}
		return domain.NewErrorProjection(folderRenameCapabilityID, commandErr), commandErr
	}
	sourcePath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection(folderRenameCapabilityID, pathErr), pathErr
	}
	targetPath, pathErr := validateVaultFolderPath(req.TargetPath)
	if pathErr != nil {
		return domain.NewErrorProjection(folderRenameCapabilityID, pathErr), pathErr
	}
	req.Path = sourcePath
	req.TargetPath = targetPath
	principalDigest := strings.TrimSpace(identity.Access.PrincipalDigest)
	scopeDigest := strings.TrimSpace(identity.Access.ScopeDigest)
	if identity.Access.OwnerLocal {
		principalDigest = operation.IdentityDigest("pinax-owner", "local")
		scopeDigest = operation.IdentityDigest("pinax-vault", root)
	}
	requestDigest, err := operation.CanonicalDigest(map[string]any{
		"capability_id": folderRenameCapabilityID, "binding_id": strings.TrimSpace(identity.BindingID),
		"source_path": sourcePath, "target_path": targetPath,
		"expected_revision": strings.TrimSpace(req.ExpectedRevision),
	})
	if err != nil {
		return operationErrorProjection(folderRenameCapabilityID, err)
	}
	createRequest := operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: idempotencyKey,
		CapabilityID: folderRenameCapabilityID, BindingID: strings.TrimSpace(identity.BindingID),
		PrincipalDigest: principalDigest, ScopeDigest: scopeDigest, RequestDigest: requestDigest,
		RevisionBefore: strings.TrimSpace(req.ExpectedRevision),
	}
	if err := operation.ValidateCreateRequest(createRequest); err != nil {
		return operationErrorProjection(folderRenameCapabilityID, err)
	}
	if existingStore, existingErr := operation.OpenExisting(root); existingErr == nil {
		existing, found, findErr := existingStore.FindReplay(ctx, createRequest)
		_ = existingStore.Close()
		if findErr != nil {
			return operationErrorProjection(folderRenameCapabilityID, findErr)
		}
		if found && existing.Operation.Status != operation.StatusAccepted {
			return folderProjectionFromOperation(existing.Operation, true)
		}
	} else if !operation.IsCode(existingErr, operation.CodeNotFound) {
		return operationErrorProjection(folderRenameCapabilityID, existingErr)
	}
	snapshotID := latestVersionSnapshotID(root)
	if snapshotID == "" {
		return folderSnapshotRequiredProjection(folderRenameCapabilityID, root)
	}
	fresh, _, freshnessErr := pinaxversion.InspectSnapshotFreshness(ctx, root, snapshotID)
	if freshnessErr != nil {
		return errorProjection(folderRenameCapabilityID, freshnessErr), freshnessErr
	}
	if !fresh {
		return folderFreshSnapshotRequiredProjection(root)
	}
	store, err := operation.Open(root)
	if err != nil {
		return operationErrorProjection(folderRenameCapabilityID, err)
	}
	defer func() { _ = store.Close() }()
	created, err := store.CreateAccepted(ctx, createRequest)
	if err != nil {
		return operationErrorProjection(folderRenameCapabilityID, err)
	}
	if created.Replay && created.Operation.Status != operation.StatusAccepted {
		return folderProjectionFromOperation(created.Operation, true)
	}

	planRequest := req
	planRequest.DryRun = true
	planRequest.Yes = false
	plan, planErr := s.RenameFolder(ctx, planRequest)
	if planErr != nil {
		_, applyingErr := store.StartApplying(ctx, operationID)
		if applyingErr != nil {
			if current, getErr := store.Get(ctx, operationID); getErr == nil {
				return folderProjectionFromOperation(current, true)
			}
			return operationErrorProjection(folderRenameCapabilityID, applyingErr)
		}
		failed, completeErr := store.CompleteFailed(ctx, operationID, operation.Outcome{
			ErrorCode: domain.ErrorCode(planErr), ErrorMessage: "Folder rename preflight failed",
			Retryable: domain.ErrorCode(planErr) == "revision_conflict", ReplaySafe: true,
		})
		if completeErr == nil {
			addOperationFacts(&plan, failed, false)
		}
		return plan, planErr
	}
	prepared := folderResultFromProjection(plan)
	preparedJSON, _ := json.Marshal(prepared)
	applying, err := store.StartApplyingWithOutcome(ctx, operationID, operation.Outcome{Result: preparedJSON})
	if err != nil {
		if operation.IsCode(err, operation.CodeIllegalTransition) {
			if current, getErr := store.Get(ctx, operationID); getErr == nil {
				return folderProjectionFromOperation(current, true)
			}
		}
		return operationErrorProjection(folderRenameCapabilityID, err)
	}

	projection, applyErr := s.RenameFolder(ctx, req)
	if applyErr != nil {
		if domain.ErrorCode(applyErr) == "revision_conflict" {
			failed, completeErr := store.CompleteFailed(ctx, operationID, operation.Outcome{
				Result: preparedJSON, ErrorCode: "revision_conflict", ErrorMessage: "Folder rename revision changed before apply",
				Retryable: true, ReplaySafe: true,
			})
			if completeErr == nil {
				addOperationFacts(&projection, failed, false)
			}
			return projection, applyErr
		}
		row, reconcileErr := store.RequireReconcile(ctx, operationID, operation.Outcome{
			Result: preparedJSON, ErrorCode: string(operation.CodeReconcileRequired),
			ErrorMessage: "Folder rename outcome requires evidence inspection",
		})
		if reconcileErr == nil {
			addOperationFacts(&projection, row, false)
		} else {
			addOperationFacts(&projection, applying, false)
		}
		return projection, applyErr
	}
	result := folderResultFromProjection(projection)
	if result.RevisionAfter == "" {
		row, reconcileErr := store.RequireReconcile(ctx, operationID, operation.Outcome{
			Result: preparedJSON, ErrorCode: string(operation.CodeReconcileRequired),
			ErrorMessage: "Folder rename revision proof requires reconciliation",
		})
		if reconcileErr == nil {
			addOperationFacts(&projection, row, false)
		} else {
			addOperationFacts(&projection, applying, false)
		}
		return projection, nil
	}
	resultJSON, _ := json.Marshal(result)
	receipt := folderOperationReceipt{
		SourcePath: result.SourcePath, TargetPath: result.TargetPath, SnapshotID: result.SnapshotID,
		RevisionBefore: result.RevisionBefore, RevisionAfter: result.RevisionAfter, Proof: "filesystem_and_registry",
	}
	receiptJSON, _ := json.Marshal(receipt)
	outcome := operation.Outcome{
		Result: resultJSON, Receipt: receiptJSON,
		ReceiptRef: folderReceiptRef(result.RevisionAfter), ResourceRef: "pinax://folder/" + result.TargetPath,
		RevisionAfter: result.RevisionAfter,
	}
	completed, err := store.CompleteSucceeded(ctx, operationID, outcome)
	if err != nil {
		if row, reconcileErr := store.RequireReconcile(ctx, operationID, operation.Outcome{
			Result: resultJSON, Receipt: receiptJSON, ReceiptRef: outcome.ReceiptRef,
			ResourceRef: outcome.ResourceRef, RevisionAfter: outcome.RevisionAfter,
			ErrorCode: string(operation.CodeReconcileRequired), ErrorMessage: "Folder rename completion requires reconciliation",
		}); reconcileErr == nil {
			addOperationFacts(&projection, row, false)
		} else {
			addOperationFacts(&projection, applying, false)
		}
		return operationStoreFailureProjection(folderRenameCapabilityID, projection, operationID)
	}
	addOperationFacts(&projection, completed, false)
	return projection, nil
}

func folderFreshSnapshotRequiredProjection(root string) (domain.Projection, error) {
	err := &domain.CommandError{
		Code:    "snapshot_required",
		Message: "Remote folder rename requires a fresh version snapshot matching current vault content",
		Hint:    fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("fresh snapshot before remote folder rename")),
	}
	return domain.NewErrorProjection(folderRenameCapabilityID, err), err
}

func folderResultFromProjection(projection domain.Projection) folderOperationResult {
	return folderOperationResult{
		SourcePath: projection.Facts["folder_path"], TargetPath: projection.Facts["target_path"],
		SnapshotID: projection.Facts["snapshot_id"], RevisionBefore: projection.Facts["revision_before"],
		RevisionAfter: projection.Facts["revision_after"], UpdatedNotes: projection.Facts["updated_notes"],
	}
}

func folderReceiptRef(revision string) string {
	return "folder-revision:" + strings.TrimPrefix(strings.TrimSpace(revision), "sha256:")
}

func folderProjectionFromOperation(row operation.OperationRow, replay bool) (domain.Projection, error) {
	projection := domain.NewProjection(folderRenameCapabilityID, "Folder rename operation is "+string(row.Status)+".")
	var result folderOperationResult
	if len(row.ResultJSON) > 0 {
		_ = json.Unmarshal(row.ResultJSON, &result)
	}
	for key, value := range map[string]string{
		"folder_path": result.SourcePath, "target_path": result.TargetPath, "snapshot_id": result.SnapshotID,
		"revision_before": result.RevisionBefore, "revision_after": result.RevisionAfter, "updated_notes": result.UpdatedNotes,
	} {
		if value != "" {
			projection.Facts[key] = value
		}
	}
	addOperationFacts(&projection, row, replay)
	projection.Data = map[string]any{"operation": operation.ViewFromRow(row)}
	if row.Status == operation.StatusFailed {
		code := strings.TrimSpace(row.ErrorCode)
		if code == "" {
			code = "operation_failed"
		}
		commandErr := &domain.CommandError{Code: code, Message: "Folder rename operation failed", Hint: "Inspect the operation before creating a new operation identity"}
		projection.Status = "failed"
		projection.Error = commandErr
		return projection, commandErr
	}
	return projection, nil
}

func (s *Service) folderReconcileEvidence(_ context.Context, root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	if row.CapabilityID != folderRenameCapabilityID {
		return operation.ReconcileEvidence{}, nil
	}
	var prepared folderOperationResult
	if len(row.ResultJSON) == 0 || json.Unmarshal(row.ResultJSON, &prepared) != nil || prepared.SourcePath == "" || prepared.TargetPath == "" || prepared.SnapshotID == "" || prepared.RevisionBefore == "" {
		return operation.ReconcileEvidence{}, nil
	}
	source, err := safeJoin(root, prepared.SourcePath)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	target, err := safeJoin(root, prepared.TargetPath)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	_, sourceErr := os.Stat(source)
	targetInfo, targetErr := os.Stat(target)
	if !errors.Is(sourceErr, os.ErrNotExist) || targetErr != nil || !targetInfo.IsDir() {
		return operation.ReconcileEvidence{}, nil
	}
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	targetRegistered := false
	sourceRegistered := false
	for _, record := range registry.Folders {
		if record.Path == prepared.TargetPath || strings.HasPrefix(record.Path, prepared.TargetPath+"/") {
			targetRegistered = true
		}
		if record.Path == prepared.SourcePath || strings.HasPrefix(record.Path, prepared.SourcePath+"/") {
			sourceRegistered = true
		}
	}
	if !targetRegistered || sourceRegistered {
		return operation.ReconcileEvidence{}, nil
	}
	revisionAfter, _, err := folderRenameRevision(root, prepared.SourcePath, prepared.TargetPath)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	result := prepared
	result.RevisionAfter = revisionAfter
	resultJSON, _ := json.Marshal(result)
	receiptJSON, _ := json.Marshal(folderOperationReceipt{
		SourcePath: prepared.SourcePath, TargetPath: prepared.TargetPath, SnapshotID: prepared.SnapshotID,
		RevisionBefore: prepared.RevisionBefore, RevisionAfter: revisionAfter, Proof: "filesystem_and_registry",
	})
	return operation.ReconcileEvidence{
		Applied: true, ProofRef: revisionAfter, Result: resultJSON, Receipt: receiptJSON,
		ReceiptRef: folderReceiptRef(revisionAfter), ResourceRef: "pinax://folder/" + prepared.TargetPath,
		RevisionAfter: revisionAfter,
	}, nil
}
