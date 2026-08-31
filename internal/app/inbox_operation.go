package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/records"
)

const inboxCaptureCapabilityID = "inbox.capture"

type RemoteOperationIdentity struct {
	OperationID    string
	IdempotencyKey string
	BindingID      string
	Access         OperationAccess
}

type inboxOperationResult struct {
	Path          string `json:"path,omitempty"`
	PlannedPath   string `json:"planned_path,omitempty"`
	NoteID        string `json:"note_id,omitempty"`
	RecordEventID string `json:"record_event_id,omitempty"`
	LedgerSeq     string `json:"ledger_seq,omitempty"`
}

type inboxOperationReceipt struct {
	NoteID        string `json:"note_id"`
	Path          string `json:"path"`
	RecordEventID string `json:"record_event_id,omitempty"`
	Proof         string `json:"proof"`
}

// InboxCaptureRemote adds owner-side operation recovery only when both
// operation identity fields are present. Requests from legacy clients that
// send neither field continue through the original compatibility path.
func (s *Service) InboxCaptureRemote(ctx context.Context, req CreateNoteRequest, identity RemoteOperationIdentity) (domain.Projection, error) {
	if req.DryRun {
		projection, err := s.InboxCapture(ctx, req)
		projection.Facts["operation_status"] = "planned"
		projection.Facts["remote_write"] = "false"
		projection.Facts["idempotent_replay"] = "false"
		return projection, err
	}

	operationID := strings.TrimSpace(identity.OperationID)
	idempotencyKey := strings.TrimSpace(identity.IdempotencyKey)
	if operationID == "" && idempotencyKey == "" {
		return s.InboxCapture(ctx, req)
	}

	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(inboxCaptureCapabilityID, err), err
	}
	req.VaultPath = root
	principalDigest := strings.TrimSpace(identity.Access.PrincipalDigest)
	scopeDigest := strings.TrimSpace(identity.Access.ScopeDigest)
	if identity.Access.OwnerLocal {
		principalDigest = operation.IdentityDigest("pinax-owner", "local")
		scopeDigest = operation.IdentityDigest("pinax-vault", root)
	}
	requestDigest, err := operation.CanonicalDigest(map[string]any{
		"capability_id": inboxCaptureCapabilityID,
		"binding_id":    strings.TrimSpace(identity.BindingID),
		"title":         strings.TrimSpace(req.Title),
		"body":          req.Body,
		"tags":          append([]string(nil), req.Tags...),
		"slug":          strings.TrimSpace(req.Slug),
	})
	if err != nil {
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}
	createRequest := operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: idempotencyKey,
		CapabilityID: inboxCaptureCapabilityID, BindingID: strings.TrimSpace(identity.BindingID),
		PrincipalDigest: principalDigest, ScopeDigest: scopeDigest, RequestDigest: requestDigest,
	}
	// Validate before Open so partial identity input never creates a ledger.
	if err := operation.ValidateCreateRequest(createRequest); err != nil {
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}
	store, err := operation.Open(root)
	if err != nil {
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}
	defer func() { _ = store.Close() }()

	created, err := store.CreateAccepted(ctx, createRequest)
	if err != nil {
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}
	if created.Replay && created.Operation.Status != operation.StatusAccepted {
		return inboxProjectionFromOperation(created.Operation, true)
	}

	planRequest := req
	planRequest.DryRun = true
	plan, planErr := s.InboxCapture(ctx, planRequest)
	if planErr != nil {
		_, applyingErr := store.StartApplying(ctx, operationID)
		if applyingErr != nil {
			if current, getErr := store.Get(ctx, operationID); getErr == nil {
				return inboxProjectionFromOperation(current, true)
			}
			return operationErrorProjection(inboxCaptureCapabilityID, applyingErr)
		}
		row, completeErr := store.CompleteFailed(ctx, operationID, operation.Outcome{
			ErrorCode: domain.ErrorCode(planErr), ErrorMessage: "Inbox capture validation failed",
			Retryable: false, ReplaySafe: true,
		})
		if completeErr == nil {
			addOperationFacts(&plan, row, false)
		}
		return plan, planErr
	}
	prepared := inboxResultFromProjection(plan)
	preparedJSON, err := json.Marshal(prepared)
	if err != nil {
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}
	applying, err := store.StartApplyingWithOutcome(ctx, operationID, operation.Outcome{Result: preparedJSON})
	if err != nil {
		if operation.IsCode(err, operation.CodeIllegalTransition) {
			current, getErr := store.Get(ctx, operationID)
			if getErr == nil {
				return inboxProjectionFromOperation(current, true)
			}
		}
		return operationErrorProjection(inboxCaptureCapabilityID, err)
	}

	applyRequest := req
	applyRequest.PlannedPath = prepared.PlannedPath
	applyRequest.ObjectID = prepared.NoteID
	applyRequest.RecordIdempotencyKey = inboxRecordIdempotency(operationID)
	applyRequest.RecordEvidence = []string{"operation_id=" + operationID}
	projection, applyErr := s.InboxCapture(ctx, applyRequest)
	if applyErr != nil {
		row, reconcileErr := store.RequireReconcile(ctx, operationID, operation.Outcome{
			Result: preparedJSON, ErrorCode: string(operation.CodeReconcileRequired),
			ErrorMessage: "Inbox capture outcome requires evidence inspection",
		})
		if reconcileErr == nil {
			addOperationFacts(&projection, row, false)
		} else {
			addOperationFacts(&projection, applying, false)
		}
		return projection, applyErr
	}

	result := inboxResultFromProjection(projection)
	resultJSON, _ := json.Marshal(result)
	receipt := inboxOperationReceipt{NoteID: result.NoteID, Path: result.Path, RecordEventID: result.RecordEventID, Proof: "record_event"}
	receiptJSON, _ := json.Marshal(receipt)
	outcome := operation.Outcome{
		Result: resultJSON, Receipt: receiptJSON,
		ReceiptRef:    "record:" + result.RecordEventID,
		ResourceRef:   "pinax://note/" + result.NoteID,
		RevisionAfter: "record:" + result.RecordEventID,
	}
	completed, err := store.CompleteSucceeded(ctx, operationID, outcome)
	if err != nil {
		if row, reconcileErr := store.RequireReconcile(ctx, operationID, operation.Outcome{
			Result: resultJSON, Receipt: receiptJSON, ReceiptRef: outcome.ReceiptRef,
			ResourceRef: outcome.ResourceRef, RevisionAfter: outcome.RevisionAfter,
			ErrorCode: string(operation.CodeReconcileRequired), ErrorMessage: "Inbox capture completion requires reconciliation",
		}); reconcileErr == nil {
			addOperationFacts(&projection, row, false)
		} else {
			addOperationFacts(&projection, applying, false)
		}
		return operationStoreFailureProjection(inboxCaptureCapabilityID, projection, operationID)
	}
	addOperationFacts(&projection, completed, false)
	return projection, nil
}

func inboxRecordIdempotency(operationID string) string {
	return "operation:" + strings.TrimSpace(operationID)
}

func inboxResultFromProjection(projection domain.Projection) inboxOperationResult {
	path := projection.Facts["path"]
	plannedPath := projection.Facts["planned_path"]
	if path == "" {
		path = plannedPath
	}
	if plannedPath == "" {
		plannedPath = path
	}
	return inboxOperationResult{
		Path: path, PlannedPath: plannedPath, NoteID: projection.Facts["note_id"],
		RecordEventID: projection.Facts["record_event_id"], LedgerSeq: projection.Facts["ledger_seq"],
	}
}

func inboxProjectionFromOperation(row operation.OperationRow, replay bool) (domain.Projection, error) {
	projection := domain.NewProjection(inboxCaptureCapabilityID, "Inbox capture operation is "+string(row.Status)+".")
	var result inboxOperationResult
	if len(row.ResultJSON) > 0 {
		_ = json.Unmarshal(row.ResultJSON, &result)
	}
	for key, value := range map[string]string{
		"path": result.Path, "planned_path": result.PlannedPath, "note_id": result.NoteID,
		"record_event_id": result.RecordEventID, "ledger_seq": result.LedgerSeq,
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
		commandErr := &domain.CommandError{Code: code, Message: "Inbox capture operation failed", Hint: "Inspect the operation before creating a new operation identity"}
		projection.Status = "failed"
		projection.Error = commandErr
		return projection, commandErr
	}
	return projection, nil
}

func addOperationFacts(projection *domain.Projection, row operation.OperationRow, replay bool) {
	if projection == nil {
		return
	}
	projection.Facts["operation_id"] = row.OperationID
	projection.Facts["operation_status"] = string(row.Status)
	projection.Facts["idempotent_replay"] = boolString(replay)
	projection.Facts["reconcile_required"] = boolString(row.Status == operation.StatusApplying || row.Status == operation.StatusReconcileRequired)
	if row.ReceiptRef != "" {
		projection.Facts["receipt_ref"] = row.ReceiptRef
	}
	if row.ResourceRef != "" {
		projection.Facts["resource_ref"] = row.ResourceRef
	}
	data, ok := projection.Data.(map[string]any)
	if !ok || data == nil {
		data = map[string]any{}
	}
	data["operation"] = operation.ViewFromRow(row)
	projection.Data = data
}

func operationStoreFailureProjection(command string, existing domain.Projection, operationID string) (domain.Projection, error) {
	commandErr := &domain.CommandError{Code: "operation_store_unavailable", Message: "Operation completion could not be persisted", Hint: "Run pinax operation reconcile " + operationID + " --json; do not repeat the mutation"}
	existing.Command = command
	existing.Status = "failed"
	existing.Error = commandErr
	existing.Facts["operation_id"] = operationID
	existing.Facts["reconcile_required"] = "true"
	return existing, commandErr
}

func (s *Service) inboxReconcileEvidence(ctx context.Context, root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	if row.CapabilityID != inboxCaptureCapabilityID {
		return operation.ReconcileEvidence{}, nil
	}
	var prepared inboxOperationResult
	if len(row.ResultJSON) == 0 || json.Unmarshal(row.ResultJSON, &prepared) != nil || prepared.NoteID == "" || prepared.PlannedPath == "" {
		return operation.ReconcileEvidence{}, nil
	}
	event, found, err := records.NewService(root).FindEventByIdempotency(ctx, inboxRecordIdempotency(row.OperationID))
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	if found && event.NoteID == prepared.NoteID && filepath.ToSlash(event.Path) == filepath.ToSlash(prepared.PlannedPath) {
		result := inboxOperationResult{Path: event.Path, PlannedPath: event.Path, NoteID: event.NoteID, RecordEventID: event.EventID, LedgerSeq: fmt.Sprint(event.Seq)}
		resultJSON, _ := json.Marshal(result)
		receiptJSON, _ := json.Marshal(inboxOperationReceipt{NoteID: event.NoteID, Path: event.Path, RecordEventID: event.EventID, Proof: "record_event"})
		return operation.ReconcileEvidence{
			Applied: true, ProofRef: "record:" + event.EventID, Result: resultJSON, Receipt: receiptJSON,
			ReceiptRef: "record:" + event.EventID, ResourceRef: "pinax://note/" + event.NoteID,
			RevisionAfter: "record:" + event.EventID,
		}, nil
	}
	path, err := safeJoin(root, prepared.PlannedPath)
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return operation.ReconcileEvidence{}, nil
	}
	if err != nil {
		return operation.ReconcileEvidence{}, err
	}
	note := parseNote(prepared.PlannedPath, string(content))
	if note.ID != prepared.NoteID {
		return operation.ReconcileEvidence{}, nil
	}
	revision := hashString(string(content))
	result := inboxOperationResult{Path: prepared.PlannedPath, PlannedPath: prepared.PlannedPath, NoteID: prepared.NoteID}
	resultJSON, _ := json.Marshal(result)
	receiptJSON, _ := json.Marshal(inboxOperationReceipt{NoteID: prepared.NoteID, Path: prepared.PlannedPath, Proof: "frontmatter_object_id"})
	return operation.ReconcileEvidence{
		Applied: true, ProofRef: revision, Result: resultJSON, Receipt: receiptJSON,
		ReceiptRef:  "content:" + strings.TrimPrefix(revision, "sha256:"),
		ResourceRef: "pinax://note/" + prepared.NoteID, RevisionAfter: revision,
	}, nil
}
