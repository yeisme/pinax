package api

import (
	"encoding/json"
	"net/http"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) dailyWriterRequest(w http.ResponseWriter, r *http.Request, command, schemaID string) (app.WriterDailyRequest, map[string]any, bool) {
	var req app.WriterDailyRequest
	if r.Method != http.MethodPost {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Daily writer accepts POST only"}, http.StatusMethodNotAllowed)
		return req, nil, false
	}
	if !s.writerAuth(w, r) {
		return req, nil, false
	}
	for _, scope := range []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata} {
		if s.requireWriterScope(w, r, command, scope) == nil {
			return req, nil, false
		}
	}
	if r.URL.RawQuery != "" {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Daily writer does not accept query fields"}, http.StatusBadRequest)
		return req, nil, false
	}
	payload, ok := decodeWriterJSON(w, r, command, schemaID)
	if !ok {
		return req, nil, false
	}
	return app.WriterDailyRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), PreviewSecret: s.readerCursorSecret()}, payload, true
}

func writeDailyWriterFailure(w http.ResponseWriter, r *http.Request, command string, err error) {
	if known, commandErr := writerCommandError(err); known {
		status := writerErrorStatus(commandErr.Code)
		switch commandErr.Code {
		case "reader_request_invalid":
			status = http.StatusBadRequest
		case "daily_exists", "lock_held", "note_identity_ambiguous":
			status = http.StatusConflict
		case "template_unsupported", "reader_unsupported", "frontmatter_unsupported":
			status = http.StatusUnprocessableEntity
		case "body_too_large":
			status = http.StatusRequestEntityTooLarge
		case "unsafe_path":
			status = http.StatusForbidden
		}
		writeWriterError(w, r, command, commandErr, status)
		return
	}
	writeWriterProjection(w, command, domain.Projection{}, err)
}

func (s *Server) handleWriterDailyPropose(w http.ResponseWriter, r *http.Request) {
	const command = "writer.daily.propose"
	req, payload, ok := s.dailyWriterRequest(w, r, command, catalogschema.WriterDailyProposeRequestV1Alpha1)
	if !ok {
		return
	}
	req.OperationID = stringField(payload, "operation_id")
	req.Date = stringField(payload, "date")
	req.Timezone = stringField(payload, "timezone")
	preview, err := s.service.WriterDailyPropose(r.Context(), req)
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	projection := domain.NewProjection(command, "Daily creation proposal prepared without mutation.")
	projection.Data = preview
	writeWriterProjection(w, command, projection, nil)
}

func (s *Server) handleWriterDailyApply(w http.ResponseWriter, r *http.Request) {
	const command = "writer.daily.apply"
	req, payload, ok := s.dailyWriterRequest(w, r, command, catalogschema.WriterDailyApplyRequestV1Alpha1)
	if !ok {
		return
	}
	encoded, err := json.Marshal(payload["proposal"])
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	var preview app.WriterDailyProposal
	if err := json.Unmarshal(encoded, &preview); err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	req.OperationID = preview.OperationID
	req.Date = preview.Date
	req.Timezone = preview.Timezone
	view, err := s.service.WriterDailyApply(r.Context(), req, preview, stringField(payload, "authorization"))
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	receipt := map[string]any{}
	encoded, err = json.Marshal(view)
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	receipt["schema_version"] = catalogschema.WriterApplyReceiptV1Alpha1
	if err := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterApplyReceiptV1Alpha1, receipt); err != nil {
		writeDailyWriterFailure(w, r, command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Daily receipt violated its schema"})
		return
	}
	projection := domain.NewProjection(command, "Daily creation operation returned.")
	projection.Data = receipt
	writeWriterProjection(w, command, projection, nil)
}
