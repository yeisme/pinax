package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) tagsWriterRequest(w http.ResponseWriter, r *http.Request, command, schemaID string) (map[string]any, bool) {
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Tag operations accept POST only"}, 405)
		return nil, false
	}
	if !s.writerAuth(w, r) {
		return nil, false
	}
	for _, scope := range []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata} {
		if s.requireWriterScope(w, r, command, scope) == nil {
			return nil, false
		}
	}
	invalid := func() (map[string]any, bool) {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Tag operation requires bounded closed JSON without query fields"}, 400)
		return nil, false
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return invalid()
	}
	limit := int64(128 * 1024)
	if schemaID == catalogschema.WriterTagsLookupRequestV1Alpha1 {
		limit = 2048
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || int64(len(raw)) > limit || !validTagsJSON(raw) {
		return invalid()
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), schemaID, payload) != nil {
		return invalid()
	}
	return payload, true
}

func (s *Server) handleWriterTagsPlan(w http.ResponseWriter, r *http.Request) {
	const command = "writer.tags.plan"
	payload, ok := s.tagsWriterRequest(w, r, command, catalogschema.WriterTagsPlanRequestV1Alpha1)
	if !ok {
		return
	}
	raw, _ := json.Marshal(payload["targets"])
	var targets []app.WriterTagTarget
	if json.Unmarshal(raw, &targets) != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Invalid tag targets"}, 400)
		return
	}
	plan, err := s.service.WriterTagsPlan(r.Context(), app.WriterTagsPlanRequest{Writer: app.WriterTagsRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), OperationID: stringField(payload, "operation_id"), PreviewSecret: s.readerCursorSecret()}, Operation: stringField(payload, "operation"), OldTag: stringField(payload, "old_tag"), NewTag: stringField(payload, "new_tag"), Targets: targets})
	if err != nil {
		writeTagsWriterFailure(w, r, command, err)
		return
	}
	projection := domain.NewProjection(command, "Tag plan prepared without mutation.")
	projection.Data = plan
	writeWriterProjection(w, command, projection, nil)
}

func (s *Server) handleWriterTagsApply(w http.ResponseWriter, r *http.Request) {
	const command = "writer.tags.apply"
	payload, ok := s.tagsWriterRequest(w, r, command, catalogschema.WriterTagsApplyRequestV1Alpha1)
	if !ok {
		return
	}
	raw, _ := json.Marshal(payload["plan"])
	var plan app.WriterTagsPlan
	if json.Unmarshal(raw, &plan) != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Invalid tag plan"}, 400)
		return
	}
	view, err := s.service.WriterTagsApply(r.Context(), app.WriterTagsRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), OperationID: plan.OperationID, PreviewSecret: s.readerCursorSecret()}, plan, stringField(payload, "authorization"))
	if err != nil {
		writeTagsWriterFailure(w, r, command, err)
		return
	}
	writeTagsReceipt(w, r, command, view)
}

func (s *Server) handleWriterTagsLookup(w http.ResponseWriter, r *http.Request) {
	const command = "writer.tags.lookup"
	payload, ok := s.tagsWriterRequest(w, r, command, catalogschema.WriterTagsLookupRequestV1Alpha1)
	if !ok {
		return
	}
	view, err := s.service.WriterTagsLookup(r.Context(), app.WriterTagsLookupRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), OperationID: stringField(payload, "operation_id")})
	if err != nil {
		writeTagsWriterFailure(w, r, command, err)
		return
	}
	writeTagsReceipt(w, r, command, view)
}

func writeTagsWriterFailure(w http.ResponseWriter, r *http.Request, command string, err error) {
	if known, e := writerCommandError(err); known && e.Code == "operation_not_found" {
		writeWriterError(w, r, command, e, 404)
		return
	}
	writeDailyWriterFailure(w, r, command, err)
}

func writeTagsReceipt(w http.ResponseWriter, r *http.Request, command string, view operation.View) {
	raw, err := json.Marshal(view)
	receipt := map[string]any{}
	if err == nil {
		err = json.Unmarshal(raw, &receipt)
	}
	receipt["schema_version"] = catalogschema.WriterTagsReceiptV1Alpha1
	if err != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterTagsReceiptV1Alpha1, receipt) != nil {
		writeWriterError(w, r, command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Tag receipt violated its contract; inspect the original operation"}, 503)
		return
	}
	projection := domain.NewProjection(command, "Original tag operation returned; inspect its status before treating it as complete.")
	projection.Data = receipt
	writeWriterProjection(w, command, projection, nil)
}
