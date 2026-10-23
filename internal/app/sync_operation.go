package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
)

// syncOperationLocked runs under the vault operation lock, including recovery.
// Only bounded facts are retained; content, credentials and provider payloads
// never enter the operation ledger. A failed/unknown execution is never replayed.
func (s *Service) syncOperationLocked(ctx context.Context, root, command string, identity RemoteOperationIdentity, input any, apply func(string) (domain.Projection, error)) (domain.Projection, error) {
	if identity.OperationID == "" && identity.IdempotencyKey == "" {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return operationErrorProjection(command, err)
		}
		identity.OperationID = "sync_" + hex.EncodeToString(nonce[:])
		identity.IdempotencyKey = identity.OperationID
	}
	if identity.BindingID == "" {
		identity.BindingID = "cli." + command
		identity.Access.OwnerLocal = true
	}
	if identity.Access.OwnerLocal {
		identity.Access.PrincipalDigest = operation.IdentityDigest("pinax-owner", "local")
		identity.Access.ScopeDigest = operation.IdentityDigest("pinax-vault", root)
	}
	digest, err := operation.CanonicalDigest(input)
	if err != nil {
		return operationErrorProjection(command, err)
	}
	req := operation.CreateRequest{OperationID: identity.OperationID, IdempotencyKey: identity.IdempotencyKey,
		CapabilityID: command, BindingID: identity.BindingID, RequestDigest: digest,
		PrincipalDigest: identity.Access.PrincipalDigest, ScopeDigest: identity.Access.ScopeDigest}
	if err := operation.ValidateCreateRequest(req); err != nil {
		return operationErrorProjection(command, err)
	}
	if err := collaborationSafePath(root, ".pinax/api/operations.sqlite"); err != nil {
		return operationErrorProjection(command, err)
	}
	store, err := operation.Open(root)
	if err != nil {
		return operationErrorProjection(command, err)
	}
	defer func() { _ = store.Close() }()
	created, err := store.CreateAccepted(ctx, req)
	if err != nil {
		return operationErrorProjection(command, err)
	}
	if created.Replay && created.Operation.Status != operation.StatusAccepted {
		return syncProjectionFromOperation(created.Operation, true)
	}
	if _, err := store.StartApplying(ctx, req.OperationID); err != nil {
		return operationErrorProjection(command, err)
	}
	projection, applyErr := apply(req.OperationID)
	// Preserve durable completion even if the caller disconnected after commit.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	result, _ := json.Marshal(syncOperationFacts(projection.Facts))
	outcome := operation.Outcome{Result: result, RevisionAfter: projection.Facts["revision_id"]}
	if runID := projection.Facts["run_id"]; runID != "" {
		outcome.ReceiptRef = "sync:" + runID
	} else if projection.Facts["resolved"] == "true" {
		outcome.ReceiptRef = "sync-conflict:" + req.OperationID
	}
	var row operation.OperationRow
	if applyErr == nil && projection.Status == "success" && projection.Facts["reconcile_required"] != "true" {
		row, err = store.CompleteSucceeded(finishCtx, req.OperationID, outcome)
	} else {
		outcome.ErrorCode = "reconcile_required"
		outcome.ErrorMessage = "Inspect the original sync receipt; do not replay the mutation"
		row, err = store.RequireReconcile(finishCtx, req.OperationID, outcome)
	}
	if err != nil {
		return operationStoreFailureProjection(command, projection, req.OperationID)
	}
	addOperationFacts(&projection, row, false)
	return projection, applyErr
}

func syncOperationFacts(facts map[string]string) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"run_id", "target", "revision_id", "base_revision", "remote_revision", "remote_write", "local_write", "resolved", "resolution", "up_to_date", "reconcile_required"} {
		if value, ok := facts[key]; ok {
			result[key] = value
		}
	}
	return result
}

func syncProjectionFromOperation(row operation.OperationRow, replay bool) (domain.Projection, error) {
	projection := domain.NewProjection(row.CapabilityID, "Sync operation is "+string(row.Status)+".")
	if len(row.ResultJSON) > 0 {
		_ = json.Unmarshal(row.ResultJSON, &projection.Facts)
	}
	if projection.Facts == nil {
		projection.Facts = map[string]string{}
	}
	addOperationFacts(&projection, row, replay)
	if row.Status != operation.StatusSucceeded {
		commandErr := &domain.CommandError{Code: "reconcile_required", Message: "Sync outcome requires inspection", Hint: "Use operation.show or operation.reconcile with the original operation ID; do not replay"}
		projection.Status, projection.Error = "failed", commandErr
		return projection, commandErr
	}
	return projection, nil
}

func syncOperationRunID(operationID string) string {
	return "sync_op_" + strings.TrimPrefix(operation.IdentityDigest(operationID), "sha256:")
}

func isSyncOperation(command string) bool {
	return command == "sync.push" || command == "sync.pull" || command == "sync.conflicts.resolve"
}

func syncReconcileEvidence(root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	if row.CapabilityID == "sync.conflicts.resolve" {
		return syncConflictReconcileEvidence(root, row)
	}
	record, err := findSyncRunReceipt(root, syncOperationRunID(row.OperationID))
	if domain.ErrorCode(err) == "sync_run_not_found" {
		return operation.ReconcileEvidence{}, nil
	}
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	receipt := record.Receipt
	if receipt.OperationID != row.OperationID || receipt.Command != row.CapabilityID || receipt.Status != "success" || receipt.Error != nil {
		return operation.ReconcileEvidence{}, nil
	}
	facts := map[string]string{"run_id": receipt.RunID, "target": receipt.Target, "revision_id": receipt.RevisionID,
		"remote_write": boolString(receipt.RemoteWrite), "local_write": boolString(receipt.LocalWrite)}
	result, _ := json.Marshal(facts)
	return operation.ReconcileEvidence{Applied: true, ReceiptRef: "sync:" + receipt.RunID, Result: result, RevisionAfter: receipt.RevisionID}, nil
}

func syncConflictOperationReceiptPath(operationID string) string {
	return ".pinax/sync-conflicts/receipts/" + syncOperationRunID(operationID) + ".json"
}

func syncConflictReconcileEvidence(root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	rel := syncConflictOperationReceiptPath(row.OperationID)
	if err := collaborationSafePath(root, rel); err != nil {
		return operation.ReconcileEvidence{}, err
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return operation.ReconcileEvidence{}, nil
	}
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	var receipt domain.SyncConflictResolutionReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return operation.ReconcileEvidence{}, err
	}
	if receipt.OperationID != row.OperationID || receipt.Status != "resolved" || receipt.Command != row.CapabilityID {
		return operation.ReconcileEvidence{}, nil
	}
	result, _ := json.Marshal(map[string]string{"resolved": "true", "resolution": receipt.Resolution, "remote_write": "false"})
	return operation.ReconcileEvidence{Applied: true, ReceiptRef: "sync-conflict:" + row.OperationID, Result: result}, nil
}
