package api

import (
	"encoding/json"
	"net/http"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) handleReaderDailyDocument(w http.ResponseWriter, r *http.Request) {
	const command = "reader.daily.document"
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil || s.requireReaderScope(w, r, app.ReaderScopeBody) == nil {
		return
	}
	query, ok := readerNavigationQuery(w, r, command, catalogschema.ReaderDailyDocumentRequestV1Alpha1)
	if !ok {
		return
	}
	projection, err := s.service.ReaderDailyDocument(r.Context(), app.ReaderDailyDocumentRequest{Date: query.Get("date"), Timezone: query.Get("timezone"), Document: app.ReaderDocumentRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), ObjectID: query.Get("object_id"), Revision: query.Get("revision"), Cursor: query.Get("cursor"), Anchor: query.Get("anchor"), OutlineCursor: query.Get("outline_cursor"), CursorSecret: s.readerCursorSecret()}})
	if err != nil {
		writeReaderProjection(w, command, projection, err, http.StatusBadRequest)
		return
	}
	encoded, err := json.Marshal(projection)
	if err != nil || len(encoded) > catalogschema.ReaderDocumentResponseMaxBytes {
		budgetErr := app.ReaderError("document_budget_exceeded", "The daily document exceeds the 1 MiB response budget", "")
		failure := domain.NewErrorProjection(command, budgetErr)
		failure.Facts["revision"] = projection.Facts["revision"]
		failure.Facts["source_size_bytes"] = projection.Facts["source_size_bytes"]
		w.Header().Set("Cache-Control", "private, no-store")
		writeProjectionStatus(w, failure, http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}
