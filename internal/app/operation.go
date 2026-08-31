package app

import (
	"context"
	"errors"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
)

type OperationAccess struct {
	PrincipalDigest string
	ScopeDigest     string
	OwnerLocal      bool
}

type OperationRequest struct {
	VaultPath   string
	OperationID string
	Access      OperationAccess
}

func (s *Service) OperationShow(ctx context.Context, request OperationRequest) (domain.Projection, error) {
	row, err := loadOperation(ctx, request)
	if err != nil {
		return operationErrorProjection("operation.show", err)
	}
	return operationProjection("operation.show", operation.ViewFromRow(row)), nil
}

func (s *Service) OperationReconcile(ctx context.Context, request OperationRequest) (domain.Projection, error) {
	row, err := loadOperation(ctx, request)
	if err != nil {
		return operationErrorProjection("operation.reconcile", err)
	}
	if operation.StatusTerminal(row.Status) {
		return operationProjection("operation.reconcile", operation.ViewFromRow(row)), nil
	}
	evidence, err := s.inboxReconcileEvidence(ctx, request.VaultPath, row)
	if err == nil && row.CapabilityID == folderRenameCapabilityID {
		evidence, err = s.folderReconcileEvidence(ctx, request.VaultPath, row)
	}
	if err != nil {
		return operationErrorProjection("operation.reconcile", err)
	}
	decision, err := operation.DecideReconcile(row, evidence)
	if err != nil {
		return operationErrorProjection("operation.reconcile", err)
	}
	store, err := operation.OpenExisting(request.VaultPath)
	if err != nil {
		return operationErrorProjection("operation.reconcile", err)
	}
	defer func() { _ = store.Close() }()
	row, err = store.ApplyReconcileDecision(ctx, request.OperationID, decision)
	if err != nil {
		return operationErrorProjection("operation.reconcile", err)
	}
	projection := operationProjection("operation.reconcile", operation.ViewFromRow(row))
	if decision.NextAction != "" {
		projection.Actions = append(projection.Actions, domain.Action{Name: "inspect_evidence", Command: "pinax operation show " + request.OperationID + " --json"})
	}
	return projection, nil
}

func loadOperation(ctx context.Context, request OperationRequest) (operation.OperationRow, error) {
	if strings.TrimSpace(request.VaultPath) == "" || strings.TrimSpace(request.OperationID) == "" {
		return operation.OperationRow{}, &operation.Error{Code: operation.CodeNotFound, Message: "Operation was not found"}
	}
	store, err := operation.OpenExisting(request.VaultPath)
	if err != nil {
		return operation.OperationRow{}, err
	}
	defer func() { _ = store.Close() }()
	if request.Access.OwnerLocal {
		return store.Get(ctx, request.OperationID)
	}
	if request.Access.PrincipalDigest == "" || request.Access.ScopeDigest == "" {
		return operation.OperationRow{}, &operation.Error{Code: operation.CodeNotFound, Message: "Operation was not found"}
	}
	return store.GetAuthorized(ctx, request.OperationID, request.Access.PrincipalDigest, request.Access.ScopeDigest)
}

func operationProjection(command string, view operation.View) domain.Projection {
	status := "success"
	summary := "Operation status is " + string(view.Status) + "."
	projection := domain.NewProjection(command, summary)
	projection.Status = status
	projection.Facts["schema_version"] = view.SchemaVersion
	projection.Facts["operation_id"] = view.OperationID
	projection.Facts["capability_id"] = view.CapabilityID
	projection.Facts["binding_id"] = view.BindingID
	projection.Facts["operation_status"] = string(view.Status)
	projection.Facts["retryable"] = boolString(view.Retryable)
	projection.Facts["replay_safe"] = boolString(view.ReplaySafe)
	projection.Facts["reconcile_required"] = boolString(view.ReconcileRequired)
	if view.ReceiptRef != "" {
		projection.Facts["receipt_ref"] = view.ReceiptRef
	}
	if view.ResourceRef != "" {
		projection.Facts["resource_ref"] = view.ResourceRef
	}
	if view.RevisionBefore != "" {
		projection.Facts["revision_before"] = view.RevisionBefore
	}
	if view.RevisionAfter != "" {
		projection.Facts["revision_after"] = view.RevisionAfter
	}
	projection.Data = map[string]any{"operation": view}
	return projection
}

func operationErrorProjection(command string, err error) (domain.Projection, error) {
	code := "operation_store_unavailable"
	message := "Operation store is unavailable"
	hint := "Retry after the owner operation store is available"
	var operationErr *operation.Error
	if errors.As(err, &operationErr) {
		switch operationErr.Code {
		case operation.CodeIdentityRequired:
			code = "operation_identity_required"
			message = "Operation identity and idempotency key are required"
			hint = "Provide both operation_id and idempotency_key"
		case operation.CodeIdempotencyConflict:
			code = "idempotency_conflict"
			message = "Idempotency identity conflicts with an existing operation"
			hint = "Reuse the original request or create a fresh operation identity"
		case operation.CodeNotFound:
			code = "operation_not_found"
			message = "Operation was not found"
			hint = "Check the operation ID and authenticated owner scope"
		case operation.CodeIllegalTransition:
			code = "reconcile_required"
			message = "Operation cannot be reconciled from its current state"
			hint = "Inspect the operation status before retrying reconciliation"
		case operation.CodeEvidenceConflict:
			code = "reconcile_required"
			message = "Operation reconciliation evidence conflicts"
			hint = "Inspect receipt and revision evidence without replaying the mutation"
		}
	}
	commandErr := &domain.CommandError{Code: code, Message: message, Hint: hint}
	return domain.NewErrorProjection(command, commandErr), commandErr
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
