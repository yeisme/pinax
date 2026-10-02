package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/transportcatalog"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Supervised writer surface (pinax-human-reading-write-v1 W02+).
//
// /v1/writer/* routes authorize ONLY through the supervised session
// registry — the same store as reader sessions, with independently granted
// writer scopes. Legacy tokens, temp tokens and MCP credentials never
// upgrade into writer scopes, and request fields never widen a session.
// Every write carries an operation identity so receipts stay recoverable
// through operation.show/reconcile.

// writerAuth guards /v1/writer/ paths with the supervised session registry.
func (s *Server) writerAuth(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "private, no-store")
	if s.readerSessions == nil {
		writeAuthError(w, "writer_disabled", "The supervised writer surface is not enabled on this server", http.StatusForbidden)
		return false
	}
	secret := extractBearerToken(r)
	if secret == "" {
		writeAuthError(w, "token_required", "Authorization Bearer supervised session token is required", http.StatusUnauthorized)
		return false
	}
	session, err := s.readerSessions.Verify(secret)
	if err != nil {
		writeAuthError(w, "invalid_token", "Supervised session authentication failed", http.StatusUnauthorized)
		return false
	}
	if !isLoopback(r) {
		writeAuthError(w, "loopback_required", "Writer routes only allow local access", http.StatusForbidden)
		return false
	}
	if s.readerRootPinned != nil {
		current, statErr := os.Stat(s.vault)
		if statErr != nil || !os.SameFile(current, s.readerRootPinned) {
			writeWriterError(w, r, "writer.capture", app.ReaderError("source_changed", "The authorized vault root changed; re-bind the session", ""), http.StatusConflict)
			return false
		}
	}
	*r = *r.WithContext(withReaderSession(r.Context(), session))
	return true
}

// requireWriterScope authorizes a writer request against the session's
// granted writer scopes. It is outside the legacy token scope map.
func (s *Server) requireWriterScope(w http.ResponseWriter, r *http.Request, command, scope string) *ReaderSession {
	session := s.readerSessionFromRequest(r)
	if session == nil {
		writeWriterError(w, r, command, app.ReaderError("insufficient_scope", "the writer surface requires an authorized supervised session", ""), http.StatusForbidden)
		return nil
	}
	if !session.HasScope(scope) {
		writeWriterError(w, r, command, app.ReaderError("insufficient_scope", "this endpoint requires the "+scope+" scope", ""), http.StatusForbidden)
		return nil
	}
	return session
}

// writeWriterError renders writer errors with the no-store policy; user
// content never appears in the error body or logs. Every error passes the
// frozen-table guard first.
func writeWriterError(w http.ResponseWriter, r *http.Request, command string, err *domain.CommandError, status int) {
	w.Header().Set("Cache-Control", "private, no-store")
	writeProjectionStatus(w, domain.NewErrorProjection(command, normalizeWriterError(command, err)), status)
}

// writerErrorStatus maps the frozen writer error table to HTTP statuses.
func writerErrorStatus(code string) int {
	switch code {
	case "insufficient_scope", "writer_disabled", "write_disabled":
		return http.StatusForbidden
	case "operation_identity_required", "writer_request_invalid", "approval_required", "authorization_required",
		"change_invalid", "body_invalid", "title_invalid", "action_unsupported",
		"organize_selection_invalid", "scoped_plan_override_forbidden", "organize_plan_schema_invalid":
		return http.StatusBadRequest
	case "revision_conflict", "idempotency_conflict", "preview_mismatch", "source_changed", "organize_plan_not_planned":
		return http.StatusConflict
	case "operation_store_unavailable", "outcome_unknown":
		return http.StatusServiceUnavailable
	case "plan_not_found":
		return http.StatusNotFound
	case "note_not_found":
		return http.StatusNotFound
	case "snapshot_required", "plan_stale":
		return http.StatusPreconditionFailed
	}
	return http.StatusServiceUnavailable
}

// writerCommandCapability maps writer commands to their frozen capability.
func writerCommandCapability(command string) string {
	switch command {
	case "writer.capture":
		return "writer.capture"
	case "writer.propose":
		return "writer.propose"
	case "writer.apply":
		return "writer.apply"
	case "writer.organize.plan":
		return "writer.organize.plan"
	case "writer.organize.apply":
		return "writer.organize.apply"
	}
	return ""
}

// normalizeWriterError closes the frozen error contract: every error the
// writer surface emits must belong to the capability's frozen error table.
// An off-table downstream code is wrapped as writer_unavailable with the
// original code preserved in the hint — clients never see an undeclared
// code, and nothing is silently reclassified.
func normalizeWriterError(command string, err *domain.CommandError) *domain.CommandError {
	if err == nil {
		return nil
	}
	capabilityID := writerCommandCapability(command)
	if capabilityID == "" {
		return err
	}
	for _, definition := range transportcatalog.WriterCapabilityDefinitions() {
		if definition.ID != capabilityID {
			continue
		}
		for _, declared := range definition.Errors {
			if declared == err.Code {
				return err
			}
		}
		break
	}
	return &domain.CommandError{
		Code:    "writer_unavailable",
		Message: "Writer request failed",
		Hint:    "upstream code: " + err.Code,
	}
}

// writerCommandError flattens app/operation errors into the frozen writer
// vocabulary so every writer response carries a contract error code.
func writerCommandError(err error) (bool, *domain.CommandError) {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return true, commandErr
	}
	var operationErr *operation.Error
	if errors.As(err, &operationErr) {
		switch operationErr.Code {
		case operation.CodeIdentityRequired:
			return true, &domain.CommandError{Code: "operation_identity_required", Message: "Writer operation identity is required"}
		case operation.CodeIdempotencyConflict:
			return true, &domain.CommandError{Code: "idempotency_conflict", Message: "Idempotency identity conflicts with an existing operation"}
		case operation.CodeNotFound:
			return true, &domain.CommandError{Code: "operation_not_found", Message: "Operation was not found"}
		}
		return true, &domain.CommandError{Code: "operation_store_unavailable", Message: "Operation store failed"}
	}
	return false, nil
}

func writeWriterProjection(w http.ResponseWriter, command string, projection domain.Projection, err error) {
	w.Header().Set("Cache-Control", "private, no-store")
	if err != nil {
		if known, commandErr := writerCommandError(err); known {
			commandErr = normalizeWriterError(command, commandErr)
			status := writerErrorStatus(commandErr.Code)
			if projection.Error != nil && projection.Command == command {
				// Serve the app-layer projection but with the normalized,
				// frozen-table error code.
				projection.Error = commandErr
				writeProjectionStatus(w, projection, status)
				return
			}
			writeProjectionStatus(w, domain.NewErrorProjection(command, commandErr), status)
			return
		}
		writeProjectionStatus(w, domain.NewErrorProjection(command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Writer request failed"}), http.StatusServiceUnavailable)
		return
	}
	writeProjectionStatus(w, projection, http.StatusOK)
}

// writerCaptureBody is the closed writer.capture request payload; decoded
// strictly and validated against the frozen schema before projecting.
type writerCaptureBody struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
	Slug  string   `json:"slug"`
	Dir   string   `json:"dir"`
}

const writerBodyMaxBytes int64 = 128 * 1024

// handleWriterCapture serves POST /v1/writer/capture: the supervised
// projection of inbox.capture. The writer.capture scope is the host gate;
// the caller must supply X-Pinax-Operation-ID and Idempotency-Key so every
// capture lands in the operation ledger with a recoverable receipt.
func (s *Server) handleWriterCapture(w http.ResponseWriter, r *http.Request) {
	const command = "writer.capture"
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Writer routes accept POST only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	session := s.requireWriterScope(w, r, command, app.WriterScopeCapture)
	if session == nil {
		return
	}
	body := writerCaptureBody{}
	if r.Body != nil {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, writerBodyMaxBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil && err != io.EOF {
			writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer capture body must be a closed JSON object"}, http.StatusBadRequest)
			return
		}
	}
	// Trim BEFORE schema validation: a whitespace-only title must fail the
	// frozen minLength here, never leak downstream as title_required.
	body.Title = strings.TrimSpace(body.Title)
	body.Slug = strings.TrimSpace(body.Slug)
	body.Dir = strings.TrimSpace(body.Dir)
	payloadFields := map[string]any{
		"title": body.Title, "body": body.Body,
		"slug": body.Slug, "dir": body.Dir,
	}
	if body.Tags != nil {
		payloadFields["tags"] = body.Tags
	}
	payload, err := json.Marshal(payloadFields)
	if err == nil {
		var value any
		if json.Unmarshal(payload, &value) == nil {
			if schemaErr := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterCaptureRequestV1Alpha1, value); schemaErr != nil {
				writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer capture payload violates the frozen contract", Hint: schemaErr.Error()}, http.StatusBadRequest)
				return
			}
		}
	}
	operationID := strings.TrimSpace(r.Header.Get("X-Pinax-Operation-ID"))
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if operationID == "" || idempotencyKey == "" {
		writeWriterError(w, r, command, &domain.CommandError{
			Code: "operation_identity_required", Message: "Writer capture requires X-Pinax-Operation-ID and Idempotency-Key",
			Hint: "Reuse the same identity to recover the receipt; never retry with a fresh identity",
		}, http.StatusBadRequest)
		return
	}
	identity := app.WriterOperationAccess(session.SessionID, session.BindingID, session.Scopes)
	identity.OperationID = operationID
	identity.IdempotencyKey = idempotencyKey
	projection, err := s.service.WriterCapture(r.Context(), app.WriterCaptureRequest{
		VaultPath: s.vault, Title: body.Title, Body: body.Body,
		Tags: body.Tags, Slug: body.Slug, Dir: body.Dir,
	}, identity)
	if err != nil {
		writeWriterProjection(w, command, projection, err)
		return
	}
	// Serve the receipt in the frozen capture-receipt shape: a closed
	// whitelist of the frozen schema fields only (never the raw operation
	// view, whose request_digest/result/receipt fields are outside the
	// capture contract), validated against the schema before serving.
	view := map[string]any{}
	if data, isMap := projection.Data.(map[string]any); isMap {
		if encoded, marshalErr := json.Marshal(data["operation"]); marshalErr == nil {
			_ = json.Unmarshal(encoded, &view)
		}
		for _, key := range []string{"note_id", "path", "record_event_id"} {
			if value, present := data[key]; present {
				view[key] = value
			}
		}
	}
	if projection.Facts["resource_ref"] != "" {
		view["resource_ref"] = projection.Facts["resource_ref"]
	}
	receipt := map[string]any{"schema_version": catalogschema.WriterCaptureReceiptV1Alpha1}
	for _, field := range []string{
		"operation_id", "capability_id", "binding_id", "status",
		"retryable", "replay_safe", "reconcile_required",
		"note_id", "path", "record_event_id", "revision_after",
		"resource_ref", "error", "created_at", "updated_at", "accepted_at",
	} {
		if value, present := view[field]; present {
			receipt[field] = value
		}
	}
	if receiptValue, marshalErr := json.Marshal(receipt); marshalErr == nil {
		var decoded any
		if json.Unmarshal(receiptValue, &decoded) == nil {
			if schemaErr := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterCaptureReceiptV1Alpha1, decoded); schemaErr != nil {
				writeWriterError(w, r, command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Writer capture receipt violated the frozen contract", Hint: schemaErr.Error()}, http.StatusInternalServerError)
				return
			}
		}
	}
	response := domain.NewProjection(command, "Writer capture applied.")
	for _, fact := range []string{"operation_id", "operation_status", "idempotent_replay", "reconcile_required", "receipt_ref", "resource_ref"} {
		if value, present := projection.Facts[fact]; present {
			response.Facts[fact] = value
		}
	}
	response.Data = receipt
	writeWriterProjection(w, command, response, nil)
}

// decodeWriterJSON reads a bounded writer request body and validates it
// against the frozen schema before any projection runs.
func decodeWriterJSON(w http.ResponseWriter, r *http.Request, command, schemaID string) (map[string]any, bool) {
	if r.Body == nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer request body is required"}, http.StatusBadRequest)
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, writerBodyMaxBytes))
	if err != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer request body exceeds the bounded size"}, http.StatusBadRequest)
		return nil, false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer request body must be valid JSON"}, http.StatusBadRequest)
		return nil, false
	}
	if err := catalogschema.ValidatePayload(catalogschema.Default(), schemaID, value); err != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer request violates the frozen contract", Hint: err.Error()}, http.StatusBadRequest)
		return nil, false
	}
	document, _ := value.(map[string]any)
	return document, true
}

// writerChangeFromPayload extracts the validated change object.
func writerChangeFromPayload(document map[string]any) (app.CollaborationChange, error) {
	encoded, err := json.Marshal(document["change"])
	if err != nil {
		return app.CollaborationChange{}, err
	}
	var change app.CollaborationChange
	if err := json.Unmarshal(encoded, &change); err != nil {
		return app.CollaborationChange{}, err
	}
	return change, nil
}

// handleWriterPropose serves POST /v1/writer/proposals: prepare an edit
// proposal with an exact preview. No vault write happens here — propose is
// readonly under the writer.editor scope.
func (s *Server) handleWriterPropose(w http.ResponseWriter, r *http.Request) {
	const command = "writer.propose"
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Writer routes accept POST only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	if s.requireWriterScope(w, r, command, app.WriterScopeEditor) == nil {
		return
	}
	document, ok := decodeWriterJSON(w, r, command, catalogschema.WriterProposeRequestV1Alpha1)
	if !ok {
		return
	}
	change, err := writerChangeFromPayload(document)
	if err != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer change could not be decoded"}, http.StatusBadRequest)
		return
	}
	preview, err := s.service.WriterPropose(r.Context(), s.vault, change)
	if err != nil {
		writeWriterProjection(w, command, domain.Projection{}, err)
		return
	}
	projection := domain.NewProjection(command, "Writer proposal prepared.")
	projection.Facts["preview_digest"] = preview.Digest
	projection.Facts["path"] = preview.Path
	projection.Facts["state"] = preview.State
	projection.Data = map[string]any{
		"schema_version": catalogschema.WriterProposalPreviewV1Alpha1,
		"change":         preview.Change, "preview_digest": preview.Digest,
		"path": preview.Path, "before": preview.Before, "after": preview.After,
		"state": preview.State,
	}
	writeWriterProjection(w, command, projection, nil)
}

// handleWriterApply serves POST /v1/writer/proposals:apply: save an
// UNCHANGED preview under the writer session identity. authorization is a
// caller assertion (explicit_instruction or reviewed_proposal), never a
// verified host approval; the host gate stays the writer.editor scope.
// The operation identity is change.operation_id — the only identity a
// recovery query may use.
func (s *Server) handleWriterApply(w http.ResponseWriter, r *http.Request) {
	const command = "writer.apply"
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Writer routes accept POST only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	session := s.requireWriterScope(w, r, command, app.WriterScopeEditor)
	if session == nil {
		return
	}
	document, ok := decodeWriterJSON(w, r, command, catalogschema.WriterApplyRequestV1Alpha1)
	if !ok {
		return
	}
	change, err := writerChangeFromPayload(document)
	if err != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Writer change could not be decoded"}, http.StatusBadRequest)
		return
	}
	digest, _ := document["preview_digest"].(string)
	authorization, _ := document["authorization"].(string)
	identity := app.WriterOperationAccess(session.SessionID, session.BindingID, session.Scopes)
	view, err := s.service.WriterApply(r.Context(), s.vault, change, digest, authorization, identity)
	if err != nil {
		writeWriterProjection(w, command, domain.Projection{}, err)
		return
	}
	projection := domain.NewProjection(command, "Writer proposal applied.")
	projection.Facts["operation_id"] = view.OperationID
	projection.Facts["operation_status"] = string(view.Status)
	projection.Facts["reconcile_required"] = strconv.FormatBool(view.ReconcileRequired)
	projection.Facts["receipt_ref"] = view.ReceiptRef
	projection.Facts["resource_ref"] = view.ResourceRef
	projection.Facts["revision_after"] = view.RevisionAfter
	// Frozen apply-receipt shape: the operation view re-versioned for the
	// writer contract so typed clients validate schema versions; the served
	// data is validated against the frozen schema before leaving the server.
	receipt := map[string]any{}
	if encoded, marshalErr := json.Marshal(view); marshalErr == nil {
		_ = json.Unmarshal(encoded, &receipt)
	}
	receipt["schema_version"] = catalogschema.WriterApplyReceiptV1Alpha1
	if receiptValue, marshalErr := json.Marshal(receipt); marshalErr == nil {
		var decoded any
		if json.Unmarshal(receiptValue, &decoded) == nil {
			if schemaErr := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterApplyReceiptV1Alpha1, decoded); schemaErr != nil {
				writeWriterError(w, r, command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Writer apply receipt violated the frozen contract", Hint: schemaErr.Error()}, http.StatusInternalServerError)
				return
			}
		}
	}
	projection.Data = receipt
	writeWriterProjection(w, command, projection, nil)
}

// handleWriterOrganizePlan serves POST /v1/writer/organize/plan: generate
// and SAVE an organize plan under the writer.organizer scope. note_refs
// scopes the plan to exactly those objects; omitted means the whole vault.
// Planning never mutates note content — the plan id is what the host
// reviews before apply.
func (s *Server) handleWriterOrganizePlan(w http.ResponseWriter, r *http.Request) {
	const command = "writer.organize.plan"
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Writer routes accept POST only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	if s.requireWriterScope(w, r, command, app.WriterScopeOrganizer) == nil {
		return
	}
	document, ok := decodeWriterJSON(w, r, command, catalogschema.WriterOrganizePlanRequestV1Alpha1)
	if !ok {
		return
	}
	noteRefs := make([]string, 0)
	if raw, present := document["note_refs"].([]any); present {
		for _, value := range raw {
			if text, isString := value.(string); isString {
				noteRefs = append(noteRefs, text)
			}
		}
	}
	projection, err := s.service.WriterOrganizePlan(r.Context(), s.vault, noteRefs)
	if err != nil {
		writeWriterProjection(w, command, domain.Projection{}, err)
		return
	}
	plan, isPlan := projection.Data.(domain.OrganizePlan)
	if !isPlan {
		writeWriterError(w, r, command, &domain.CommandError{Code: "plan_save_failed", Message: "Writer organize plan could not be generated"}, http.StatusInternalServerError)
		return
	}
	operations := make([]map[string]any, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		operations = append(operations, map[string]any{
			"operation_id": op.OperationID, "object_id": op.ObjectID,
			"kind": op.Kind, "mode": op.Mode, "risk": op.Risk, "path": op.Path,
		})
	}
	response := domain.NewProjection(command, "Writer organize plan saved for host review.")
	response.Facts["plan_id"] = plan.PlanID
	response.Facts["operation_count"] = strconv.Itoa(len(plan.Operations))
	response.Facts["saved_path"] = plan.SavedPath
	response.Data = map[string]any{
		"schema_version": catalogschema.WriterOrganizePlanV1Alpha1,
		"plan_id":        plan.PlanID, "status": plan.Status,
		"created_at": plan.CreatedAt, "expires_at": plan.ExpiresAt,
		"operation_count": len(plan.Operations), "operations": operations,
	}
	writeWriterProjection(w, command, response, nil)
}

// handleWriterOrganizeApply serves POST /v1/writer/organize/apply: apply a
// saved plan id AS-IS under the approval_and_snapshot gate. The host gate
// is the writer.organizer scope; the snapshot runs before any move, and
// the plan id doubles as the recovery identity (re-apply of a consumed
// plan fails with organize_plan_not_planned instead of a silent replay).
func (s *Server) handleWriterOrganizeApply(w http.ResponseWriter, r *http.Request) {
	const command = "writer.organize.apply"
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Writer routes accept POST only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	if s.requireWriterScope(w, r, command, app.WriterScopeOrganizer) == nil {
		return
	}
	document, ok := decodeWriterJSON(w, r, command, catalogschema.WriterOrganizeApplyRequestV1Alpha1)
	if !ok {
		return
	}
	planID, _ := document["plan_id"].(string)
	snapshotMessage, _ := document["snapshot_message"].(string)
	allowStale := false
	if value, present := document["allow_stale"].(bool); present {
		allowStale = value
	}
	projection, err := s.service.WriterOrganizeApply(r.Context(), s.vault, planID, snapshotMessage, allowStale)
	if err != nil {
		writeWriterProjection(w, command, domain.Projection{}, err)
		return
	}
	status := "succeeded"
	switch projection.Status {
	case "partial":
		status = "reconcile_required"
	case "failed":
		status = "failed"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	receipt := map[string]any{
		"schema_version": catalogschema.WriterOrganizeApplyReceiptV1Alpha1,
		"operation_id":   projection.Facts["plan_id"], "plan_id": projection.Facts["plan_id"],
		"status": status, "retryable": false, "replay_safe": true,
		"reconcile_required": status == "reconcile_required",
		"snapshot_id":        projection.Facts["snapshot_id"],
		"capability_id":      "writer.organize.apply", "binding_id": "rest.writer.organize.apply",
		"created_at": now, "updated_at": now, "accepted_at": now,
	}
	response := domain.NewProjection(command, "Writer organize apply finished.")
	response.Facts["plan_id"] = projection.Facts["plan_id"]
	response.Facts["snapshot_id"] = projection.Facts["snapshot_id"]
	response.Facts["operation_status"] = status
	response.Data = receipt
	writeWriterProjection(w, command, response, nil)
}
