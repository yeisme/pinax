package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Reader document route (pinax-human-reading-api-v1 P06): serves the paged
// human-reading projection for one note revision over the shared reader
// session authorization.

// Reader relations, assets and source export routes
// (pinax-human-reading-api-v1 P04). All responses are no-store, nosniff and
// loopback-gated by the reader session branch in authMiddleware.

// readerRange is one validated byte window.
type readerRange struct {
	Start int64
	End   int64 // inclusive
}

// parseReaderRange validates a Range header against size and the per-route
// maximum. Out-of-bounds or oversized windows return reader_range_invalid.
func parseReaderRange(header string, size int64, maxWindow int64) (*readerRange, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, true
	}
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return nil, false
	}
	spec := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	startPart, endPart, hasEnd := strings.Cut(spec, "-")
	if strings.TrimSpace(startPart) == "" {
		// Suffix ranges are not needed by the reading surface; reject rather
		// than guess an interpretation the consumer did not send.
		return nil, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startPart), 10, 64)
	if err != nil || start < 0 || start >= size {
		return nil, false
	}
	end := size - 1
	if hasEnd && strings.TrimSpace(endPart) != "" {
		end, err = strconv.ParseInt(strings.TrimSpace(endPart), 10, 64)
		if err != nil || end < start || end >= size {
			return nil, false
		}
	}
	if end-start+1 > maxWindow {
		return nil, false
	}
	return &readerRange{Start: start, End: end}, true
}

func readerRangeInvalid(w http.ResponseWriter, r *http.Request, command, hint string) {
	w.Header().Set("Cache-Control", "private, no-store")
	err := app.ReaderError("reader_range_invalid", "The requested range is out of bounds or exceeds the per-request byte budget", hint)
	writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusRequestedRangeNotSatisfiable)
}

// readerDangerousMime lists declared attachment types that can execute or
// render as active content in a consumer; nosniff alone does not make them
// safe downloads.
var readerDangerousMime = map[string]bool{
	"text/html":               true,
	"application/xhtml+xml":   true,
	"image/svg+xml":           true,
	"application/xml":         true,
	"text/xml":                true,
	"application/javascript":  true,
	"text/javascript":         true,
	"application/ecmascript":  true,
	"application/x-httpd-php": true,
	"text/css":                true,
}

// readerSafeContentType forces a neutral octet-stream download for
// dangerous declared types; everything else serves its declared type.
func readerSafeContentType(mime string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if readerDangerousMime[normalized] {
		return "application/octet-stream", true
	}
	return mime, false
}

// streamReaderContent writes (a slice of) the resolved content with
// nosniff, no-store and explicit Content-Range semantics.
func streamReaderContent(w http.ResponseWriter, r *http.Request, command string, content io.ReadSeeker, mime string, size int64, digest string, maxWindow int64) {
	window, ok := parseReaderRange(r.Header.Get("Range"), size, maxWindow)
	if !ok {
		readerRangeInvalid(w, r, command, fmt.Sprintf("total_size=%d", size))
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The content digest rides along as ETag so typed consumers recover
	// the full identity of what they are reading, windowed or not.
	if digest != "" {
		w.Header().Set("ETag", digest)
	}
	if contentType, forced := readerSafeContentType(mime); forced {
		// Executable-capable declared types never render in the consumer:
		// the bytes download as a neutral attachment instead.
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", "attachment")
	} else {
		w.Header().Set("Content-Type", mime)
	}
	if window == nil {
		if size > maxWindow {
			// No Range only serves content that fits one window; larger
			// content requires explicit chunked ranges.
			err := app.ReaderError("reader_range_invalid", "Content larger than the per-request budget requires a Range request", fmt.Sprintf("total_size=%d", size))
			writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content)
		return
	}
	if _, err := content.Seek(window.Start, io.SeekStart); err != nil {
		writeReaderError(w, r, command, app.ReaderError("reader_unavailable", "The content could not be read", ""), http.StatusServiceUnavailable)
		return
	}
	length := window.End - window.Start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", window.Start, window.End, size))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, io.LimitReader(content, length))
}

func (s *Server) handleReaderNoteItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/reader/notes/")
	objectID, action, ok := strings.Cut(rest, "/")
	if !ok || objectID == "" {
		writeReaderError(w, r, "reader.notes", app.ReaderError("reader_request_invalid", "Unknown reader note route", ""), http.StatusNotFound)
		return
	}
	switch action {
	case "composed-document":
		s.handleReaderRichPage(w, r, objectID, true)
	case "rich-document":
		s.handleReaderRichDocument(w, r, objectID)
	case "document":
		s.handleReaderDocument(w, r, objectID)
	case "links":
		s.handleReaderLinks(w, r, objectID)
	case "source":
		s.handleReaderSourceExport(w, r, objectID)
	default:
		writeReaderError(w, r, "reader.notes", app.ReaderError("reader_request_invalid", "Unknown reader note route", ""), http.StatusNotFound)
	}
}

// handleReaderDocument serves GET /v1/reader/notes/{object_id}/document.
func (s *Server) handleReaderDocument(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.document", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeBody) == nil {
		return
	}
	query := r.URL.Query()
	projection, err := s.service.ReaderDocument(r.Context(), app.ReaderDocumentRequest{
		VaultPath:     s.vault,
		Session:       s.readerSessionInfo(r),
		ObjectID:      objectID,
		Revision:      query.Get("revision"),
		Cursor:        query.Get("cursor"),
		Anchor:        query.Get("anchor"),
		OutlineCursor: query.Get("outline_cursor"),
		CursorSecret:  s.readerCursorSecret(),
	})
	if err != nil {
		writeReaderProjection(w, "reader.document", projection, err, http.StatusBadRequest)
		return
	}
	// The full document response (envelope included) is capped at 1 MiB: a
	// page that would exceed the wire budget fails explicitly instead of
	// shipping an oversized body. The 413 carries the machine-recoverable
	// details the contract requires: fixed revision, source size and the
	// source-export action.
	encoded, marshalErr := json.Marshal(projection)
	if marshalErr != nil || len(encoded) > catalogschema.ReaderDocumentResponseMaxBytes {
		budgetErr := app.ReaderError("document_budget_exceeded", "The document page exceeds the 1 MiB response budget", "Use the explicit source export with ranged windows")
		errorProjection := domain.NewErrorProjection("reader.document", budgetErr)
		// Machine-recoverable details come from the resolved projection
		// facts: the pinned revision (never the raw query) and the true
		// markdown source size (not the JSON envelope size).
		resolvedRevision := projection.Facts["revision"]
		if resolvedRevision == "" {
			resolvedRevision = projection.Facts["current_revision"]
		}
		errorProjection.Facts["revision"] = resolvedRevision
		errorProjection.Facts["source_size_bytes"] = projection.Facts["source_size_bytes"]
		errorProjection.Actions = []domain.Action{{Name: "source_export", Command: "GET /v1/reader/notes/" + objectID + "/source?revision=" + resolvedRevision}}
		w.Header().Set("Cache-Control", "private, no-store")
		writeProjectionStatus(w, errorProjection, http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

// handleReaderLinks serves GET /v1/reader/notes/{object_id}/links.
func (s *Server) handleReaderLinks(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.links", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeRelations) == nil {
		return
	}
	query := r.URL.Query()
	projection, err := s.service.ReaderLinks(r.Context(), app.ReaderLinksRequest{
		VaultPath:    s.vault,
		Session:      s.readerSessionInfo(r),
		ObjectID:     objectID,
		Revision:     query.Get("revision"),
		Direction:    query.Get("direction"),
		Limit:        readerLimitQuery(query.Get("limit")),
		Cursor:       query.Get("cursor"),
		CursorSecret: s.readerCursorSecret(),
	})
	writeReaderProjection(w, "reader.links", projection, err, http.StatusBadRequest)
}

// handleReaderSourceExport serves GET /v1/reader/notes/{object_id}/source
// with explicit Range windows of at most 1 MiB.
func (s *Server) handleReaderSourceExport(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.source_export", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeBody) == nil {
		return
	}
	projection, content, err := s.service.ReaderSourceExport(r.Context(), app.ReaderSourceExportRequest{
		VaultPath: s.vault,
		Session:   s.readerSessionInfo(r),
		ObjectID:  objectID,
		Revision:  r.URL.Query().Get("revision"),
	})
	if err != nil {
		writeReaderProjection(w, "reader.source_export", projection, err, http.StatusBadRequest)
		return
	}
	if closer, ok := content.ReadSeeker.(io.Closer); ok && content.ReadSeeker != nil {
		// File-backed export streams hold an open handle until the
		// response is written.
		defer func() { _ = closer.Close() }()
	}
	streamReaderContent(w, r, "reader.source_export", content.ReadSeeker, content.MediaType, content.SizeBytes, content.Digest, int64(catalogschema.ReaderSourceExportRangeMaxBytes))
}

// handleReaderAssetContent serves GET /v1/reader/assets/{asset_ref}/content
// with note/revision binding and Range windows of at most 8 MiB.
func (s *Server) handleReaderAssetContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.asset", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeAssets) == nil {
		return
	}
	assetRef := strings.TrimPrefix(r.URL.Path, "/v1/reader/assets/")
	assetRef = strings.TrimSuffix(assetRef, "/content")
	if assetRef == "" {
		writeReaderError(w, r, "reader.asset", app.ReaderError("reader_request_invalid", "asset_ref is required", ""), http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	projection, content, err := s.service.ReaderAsset(r.Context(), app.ReaderAssetRequest{
		VaultPath:    s.vault,
		Session:      s.readerSessionInfo(r),
		AssetRef:     assetRef,
		NoteID:       query.Get("note_id"),
		NoteRevision: query.Get("note_revision"),
	})
	if err != nil {
		if closer, ok := content.ReadSeeker.(io.Closer); ok && content.ReadSeeker != nil {
			_ = closer.Close()
		}
		writeReaderProjection(w, "reader.asset", projection, err, http.StatusBadRequest)
		return
	}
	defer func() {
		if closer, ok := content.ReadSeeker.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	streamReaderContent(w, r, "reader.asset", content.ReadSeeker, content.Mime, content.SizeBytes, content.Digest, int64(catalogschema.ReaderAssetRangeMaxBytes))
}

func readerLimitQuery(raw string) int {
	if raw == "" {
		return 0 // omitted: the app layer applies the default
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 200 {
		return -1 // invalid (including explicit 0): rejected downstream
	}
	return limit
}
