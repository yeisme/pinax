package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// New navigation queries are closed and reject duplicate values. Validate the
// original wire values before converting optional integer fields to Go zeros.
func readerNavigationQuery(w http.ResponseWriter, r *http.Request, command, schemaID string) (url.Values, bool) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeReaderError(w, r, command, app.ReaderError("reader_request_invalid", "Reader navigation accepts GET only", ""), http.StatusMethodNotAllowed)
		return nil, false
	}
	invalid := func() (url.Values, bool) {
		writeReaderError(w, r, command, app.ReaderError("reader_request_invalid", "Reader navigation query violates its schema", ""), http.StatusBadRequest)
		return nil, false
	}
	if len(r.URL.RawQuery) > 8192 {
		return invalid()
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return invalid()
	}
	payload := map[string]any{}
	for key, values := range query {
		if len(values) != 1 {
			return invalid()
		}
		if key == "limit" || key == "hops" {
			value, err := strconv.Atoi(values[0])
			if err != nil {
				return invalid()
			}
			payload[key] = float64(value)
		} else {
			payload[key] = values[0]
		}
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), schemaID, payload) != nil {
		return invalid()
	}
	return query, true
}

func (s *Server) handleReaderTags(w http.ResponseWriter, r *http.Request) {
	const command = "reader.tags"
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil {
		return
	}
	query, ok := readerNavigationQuery(w, r, command, catalogschema.ReaderTagsRequestV1Alpha1)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	page, err := s.service.ReaderTags(r.Context(), app.ReaderTagsRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), Limit: limit, Cursor: query.Get("cursor"), CursorSecret: s.readerCursorSecret()})
	projection := domain.NewProjection(command, "Reader tag page composed.")
	projection.Data = page
	writeReaderProjection(w, command, projection, err, http.StatusBadRequest)
}

func (s *Server) handleReaderGraph(w http.ResponseWriter, r *http.Request) {
	const command = "reader.graph"
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil || s.requireReaderScope(w, r, app.ReaderScopeRelations) == nil {
		return
	}
	query, ok := readerNavigationQuery(w, r, command, catalogschema.ReaderGraphRequestV1Alpha1)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	hops, _ := strconv.Atoi(query.Get("hops"))
	page, err := s.service.ReaderGraph(r.Context(), app.ReaderGraphRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), Mode: query.Get("mode"), ObjectID: query.Get("object_id"), Revision: query.Get("revision"), Hops: hops, Limit: limit, Cursor: query.Get("cursor"), CursorSecret: s.readerCursorSecret()})
	projection := domain.NewProjection(command, "Reader graph page composed.")
	projection.Data = page
	writeReaderProjection(w, command, projection, err, http.StatusBadRequest)
}

func (s *Server) handleReaderDaily(w http.ResponseWriter, r *http.Request) {
	const command = "reader.daily"
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil {
		return
	}
	query, ok := readerNavigationQuery(w, r, command, catalogschema.ReaderDailyRequestV1Alpha1)
	if !ok {
		return
	}
	page, err := s.service.ReaderDaily(r.Context(), app.ReaderDailyRequest{VaultPath: s.vault, Session: s.readerSessionInfo(r), Date: query.Get("date"), Timezone: query.Get("timezone")})
	projection := domain.NewProjection(command, "Daily journal lookup completed without mutation.")
	projection.Data = page
	writeReaderProjection(w, command, projection, err, http.StatusBadRequest)
}
