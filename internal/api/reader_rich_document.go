package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) handleReaderRichDocument(w http.ResponseWriter, r *http.Request, objectID string) {
	s.handleReaderRichPage(w, r, objectID, false)
}

func (s *Server) handleReaderRichPage(w http.ResponseWriter, r *http.Request, objectID string, composed bool) {
	command := "reader.rich_document"
	requestSchema := catalogschema.ReaderRichDocumentRequestV1Alpha1
	read := s.service.ReaderRichDocument
	if composed {
		command = "reader.composed_document"
		requestSchema = catalogschema.ReaderComposedRequestV1Alpha1
		read = s.service.ReaderComposedDocument
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeReaderError(w, r, command, app.ReaderError("reader_request_invalid", "Rich documents accept GET only", ""), 405)
		return
	}
	session := s.readerSessionFromRequest(r)
	if session == nil || !session.HasScope(app.ReaderScopeBody) {
		writeReaderError(w, r, command, app.ReaderError("insufficient_scope", "Rich documents require reader.body", ""), 403)
		return
	}
	invalid := func() {
		writeReaderError(w, r, command, app.ReaderError("reader_request_invalid", "Rich documents require a closed fixed-revision query", ""), 400)
	}
	if len(r.URL.RawQuery) > 8192 || !utf8.ValidString(objectID) {
		invalid()
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		invalid()
		return
	}
	payload := map[string]any{"object_id": objectID}
	for key, values := range query {
		if key == "object_id" || len(values) != 1 || !utf8.ValidString(values[0]) {
			invalid()
			return
		}
		payload[key] = values[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), requestSchema, payload) != nil {
		invalid()
		return
	}
	projection, err := read(r.Context(), app.ReaderDocumentRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), ObjectID: objectID, Revision: query.Get("revision"), Cursor: query.Get("cursor"), Anchor: query.Get("anchor"), OutlineCursor: query.Get("outline_cursor"), CursorSecret: s.readerCursorSecret()})
	if err != nil {
		writeReaderProjection(w, command, projection, err, 400)
		return
	}
	encoded, err := json.Marshal(projection)
	if err != nil || len(encoded) > catalogschema.ReaderDocumentResponseMaxBytes {
		writeReaderError(w, r, command, app.ReaderError("document_budget_exceeded", "Rich response exceeds its wire budget", ""), 413)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}
