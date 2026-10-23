package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) handleWriterDailyEditorSource(w http.ResponseWriter, r *http.Request) {
	const command = "writer.daily.editor_source"
	if r.Method != http.MethodGet {
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Daily source accepts GET only"}, 405)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	for _, scope := range []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata} {
		if s.requireWriterScope(w, r, command, scope) == nil {
			return
		}
	}
	invalid := func() {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Daily source requires a closed identity query"}, 400)
	}
	if len(r.URL.RawQuery) > 8192 {
		invalid()
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		invalid()
		return
	}
	payload := map[string]any{}
	for key, list := range query {
		if len(list) != 1 {
			invalid()
			return
		}
		payload[key] = list[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyEditorSourceRequestV1Alpha1, payload) != nil {
		invalid()
		return
	}
	source, err := s.service.WriterDailyEditorSource(r.Context(), app.WriterDailyEditorSourceRequest{Date: query.Get("date"), Timezone: query.Get("timezone"), Source: app.WriterEditorSourceRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), ObjectID: query.Get("object_id"), Revision: query.Get("revision")}})
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	projection := domain.NewProjection(command, "Daily editor source read without mutation.")
	projection.Data = source
	writeWriterProjection(w, command, projection, nil)
}

func (s *Server) handleWriterDailyEditPropose(w http.ResponseWriter, r *http.Request) {
	const command = "writer.daily.edit.propose"
	req, payload, ok := s.dailyWriterRequest(w, r, command, catalogschema.WriterDailyEditProposeRequestV1Alpha1)
	if !ok {
		return
	}
	req.Date = stringField(payload, "date")
	req.Timezone = stringField(payload, "timezone")
	req.OperationID = stringField(payload, "operation_id")
	proposal, err := s.service.WriterDailyEditPropose(r.Context(), app.WriterDailyEditRequest{Daily: req, ObjectID: stringField(payload, "object_id"), Revision: stringField(payload, "revision"), Body: stringField(payload, "body")})
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	projection := domain.NewProjection(command, "Daily edit proposal prepared without mutation.")
	projection.Data = proposal
	writeWriterProjection(w, command, projection, nil)
}

func (s *Server) handleWriterDailyEditApply(w http.ResponseWriter, r *http.Request) {
	const command = "writer.daily.edit"
	req, payload, ok := s.dailyWriterRequest(w, r, command, catalogschema.WriterDailyEditApplyRequestV1Alpha1)
	if !ok {
		return
	}
	encoded, err := json.Marshal(payload["proposal"])
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	var proposal app.WriterDailyEditProposal
	if err = json.Unmarshal(encoded, &proposal); err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	req.Date = proposal.Date
	req.Timezone = proposal.Timezone
	req.OperationID = proposal.Preview.Change.OperationID
	view, err := s.service.WriterDailyEditApply(r.Context(), req, proposal, stringField(payload, "authorization"))
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	encoded, err = json.Marshal(view)
	if err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	receipt := map[string]any{}
	if err = json.Unmarshal(encoded, &receipt); err != nil {
		writeDailyWriterFailure(w, r, command, err)
		return
	}
	receipt["schema_version"] = catalogschema.WriterApplyReceiptV1Alpha1
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterApplyReceiptV1Alpha1, receipt) != nil {
		writeDailyWriterFailure(w, r, command, &domain.CommandError{Code: "operation_store_unavailable", Message: "Daily edit receipt violated its schema"})
		return
	}
	projection := domain.NewProjection(command, "Daily edit operation returned.")
	projection.Data = receipt
	writeWriterProjection(w, command, projection, nil)
}
