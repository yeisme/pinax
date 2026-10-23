package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func (s *Server) handleReaderRecoveryLookup(w http.ResponseWriter, r *http.Request) {
	const command = "reader.recovery.lookup"
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	fail := func(code, message string, status int) {
		writeReaderError(w, r, command, app.ReaderError(code, message, ""), status)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		fail("reader_request_invalid", "Recovery lookup accepts GET only", 405)
		return
	}
	if s.readerSessions == nil || s.readerSessionFromRequest(r) == nil {
		fail("insufficient_scope", "Recovery requires a current supervised session", 403)
		return
	}
	invalid := func() { fail("reader_request_invalid", "Recovery lookup requires only one operation_id", 400) }
	if len(r.URL.RawQuery) > 2048 || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		invalid()
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		invalid()
		return
	}
	payload := map[string]any{}
	for key, values := range query {
		if len(values) != 1 || !utf8.ValidString(values[0]) {
			invalid()
			return
		}
		payload[key] = values[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRecoveryLookupRequestV1Alpha1, payload) != nil {
		invalid()
		return
	}
	view, err := s.readerSessions.LookupRecovery(r.Context(), extractBearerToken(r), query.Get("operation_id"))
	// Do not distinguish absent/foreign rows from missing private grants. Neither
	// denial nor 404 can establish that a mutation was never submitted.
	if err != nil {
		fail("recovery_lookup_unavailable", "Original operation could not be read under the current recovery authorization", 403)
		return
	}
	projection := domain.NewProjection(command, "Original operation evidence; lookup does not execute or confirm a mutation.")
	projection.Data = view
	encoded, err := json.Marshal(projection)
	if err != nil || len(encoded) > 128*1024 {
		fail("reader_unavailable", "Recovery response exceeds its wire budget", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}
