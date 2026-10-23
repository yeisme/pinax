package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func (s *Server) handleWriterEditorSource(w http.ResponseWriter, r *http.Request) {
	const command = "writer.editor_source"
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeWriterError(w, r, command, &domain.CommandError{Code: "method_not_allowed", Message: "Editor source accepts GET only"}, http.StatusMethodNotAllowed)
		return
	}
	if !s.writerAuth(w, r) {
		return
	}
	if s.requireWriterScope(w, r, command, app.WriterScopeEditor) == nil || s.requireWriterScope(w, r, command, app.ReaderScopeBody) == nil {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/writer/notes/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "editor-source" {
		writeWriterError(w, r, command, &domain.CommandError{Code: "note_not_found", Message: "Editor source route was not found"}, http.StatusNotFound)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["revision"]) != 1 {
		writeWriterError(w, r, command, &domain.CommandError{Code: "writer_request_invalid", Message: "Editor source accepts one revision query parameter"}, http.StatusBadRequest)
		return
	}
	source, err := s.service.WriterEditorSource(r.Context(), app.WriterEditorSourceRequest{
		VaultPath: s.vault, ObjectID: parts[0], Revision: query.Get("revision"), Session: s.readerSessionInfo(r),
	})
	if err != nil {
		if known, commandErr := writerCommandError(err); known {
			status := writerErrorStatus(commandErr.Code)
			switch commandErr.Code {
			case "body_too_large":
				status = http.StatusRequestEntityTooLarge
			case "unsafe_path":
				status = http.StatusForbidden
			case "note_identity_ambiguous":
				status = http.StatusConflict
			}
			writeWriterError(w, r, command, commandErr, status)
			return
		}
	}
	projection := domain.NewProjection(command, "Editor source read at the requested revision.")
	projection.Data = source
	writeWriterProjection(w, command, projection, err)
}
