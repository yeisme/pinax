package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// Reader session authorization (pinax-human-reading-api-v1 P02).
//
// Reader scopes live in their own session registry: an existing Agent temp
// token or token-store credential never grants reader access, and a reader
// bearer never widens legacy read/write scopes. The store is in-memory only;
// P05's supervised bootstrap mints sessions through it.

// ReaderSession is one authorized human reading session.
type ReaderSession struct {
	SessionID string
	BindingID string
	Nonce     string
	Scopes    []string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// HasScope reports whether the session grants a reader scope.
func (s *ReaderSession) HasScope(scope string) bool {
	for _, granted := range s.Scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

// Expired reports whether the session passed its deadline.
func (s *ReaderSession) Expired() bool {
	return !s.ExpiresAt.IsZero() && time.Now().UTC().After(s.ExpiresAt)
}

// ReaderSessionStore keeps bearer digests in process memory. Secrets are
// hashed at rest and never persisted.
type ReaderSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*ReaderSession // bearer digest -> session
	byID     map[string]string         // session id -> bearer digest
	secret   []byte                    // cursor signing secret
}

// NewReaderSessionStore creates an empty in-memory registry with a random
// cursor signing secret.
func NewReaderSessionStore() (*ReaderSessionStore, error) {
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		return nil, err
	}
	return &ReaderSessionStore{
		sessions: make(map[string]*ReaderSession),
		byID:     make(map[string]string),
		secret:   []byte(bearer),
	}, nil
}

func readerBearerDigest(bearer string) string {
	sum := sha256.Sum256([]byte(bearer))
	return hex.EncodeToString(sum[:])
}

// Create registers a session for a fresh bearer secret.
func (s *ReaderSessionStore) Create(session ReaderSession, bearer string) error {
	if strings.TrimSpace(bearer) == "" {
		return fmt.Errorf("reader bearer is required")
	}
	digest := readerBearerDigest(bearer)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[session.SessionID]; exists {
		return fmt.Errorf("reader session %q already exists", session.SessionID)
	}
	s.sessions[digest] = &session
	s.byID[session.SessionID] = digest
	return nil
}

// Verify resolves a bearer to its live session.
func (s *ReaderSessionStore) Verify(bearer string) (*ReaderSession, error) {
	digest := readerBearerDigest(bearer)
	s.mu.RLock()
	session, ok := s.sessions[digest]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("invalid reader session")
	}
	if session.Expired() {
		return nil, fmt.Errorf("reader session expired")
	}
	return session, nil
}

// Revoke removes a session by ID.
func (s *ReaderSessionStore) Revoke(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if digest, ok := s.byID[sessionID]; ok {
		delete(s.sessions, digest)
		delete(s.byID, sessionID)
	}
}

// CursorSecret returns the cursor signing key for this server process.
func (s *ReaderSessionStore) CursorSecret() []byte {
	return s.secret
}

type readerSessionContextKey struct{}

func (s *Server) readerSessionFromRequest(r *http.Request) *ReaderSession {
	session, _ := r.Context().Value(readerSessionContextKey{}).(*ReaderSession)
	return session
}
func (s *Server) readerSessionInfo(r *http.Request) app.ReaderSessionInfo {
	session := s.readerSessionFromRequest(r)
	if session == nil {
		return app.ReaderSessionInfo{}
	}
	return app.ReaderSessionInfo{SessionID: session.SessionID, BindingID: session.BindingID, Scopes: append([]string(nil), session.Scopes...)}
}

// requireReaderScope authorizes a reader request against the dedicated
// session registry. It is deliberately outside the legacy token scope map.
func (s *Server) requireReaderScope(w http.ResponseWriter, r *http.Request, scope string) *ReaderSession {
	session := s.readerSessionFromRequest(r)
	if session == nil {
		err := app.ReaderError("insufficient_scope", "reader surface requires an authorized human reader session", "")
		writeReaderError(w, r, "reader.notes", err, http.StatusForbidden)
		return nil
	}
	if !session.HasScope(scope) {
		err := app.ReaderError("insufficient_scope", "this reader endpoint requires the "+scope+" scope", "")
		writeReaderError(w, r, "reader.notes", err, http.StatusForbidden)
		return nil
	}
	return session
}

// writeReaderError renders reader errors with the no-store policy. Query
// text and snippets never appear in the error body or logs.
func writeReaderError(w http.ResponseWriter, r *http.Request, command string, err *domain.CommandError, status int) {
	w.Header().Set("Cache-Control", "private, no-store")
	writeProjectionStatus(w, domain.NewErrorProjection(command, err), status)
}

func writeReaderProjection(w http.ResponseWriter, command string, projection domain.Projection, err error, errorStatus int) {
	w.Header().Set("Cache-Control", "private, no-store")
	if err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok {
			status := errorStatus
			switch commandErr.Code {
			case "index_unavailable", "reader_unavailable":
				status = http.StatusServiceUnavailable
			case "reader_cursor_invalid", "reader_request_invalid":
				status = http.StatusBadRequest
			case "reader_cursor_expired":
				status = http.StatusGone
			case "insufficient_scope", "reader_disabled":
				status = http.StatusForbidden
			case "revision_unavailable", "source_changed":
				status = http.StatusConflict
			case "reader_object_not_found":
				status = http.StatusNotFound
			case "document_too_large", "document_budget_exceeded":
				status = http.StatusRequestEntityTooLarge
			case "reader_range_invalid":
				status = http.StatusRequestedRangeNotSatisfiable
			case "reader_unsupported":
				status = http.StatusUnprocessableEntity
			}
			// Serve the app-layer error projection as-is: it carries the
			// machine-recoverable Facts and Actions (revision, sizes,
			// source-export action) that rebuilding would drop.
			if projection.Error != nil && projection.Command == command {
				writeProjectionStatus(w, projection, status)
				return
			}
			writeProjectionStatus(w, domain.NewErrorProjection(command, commandErr), status)
			return
		}
		writeProjectionStatus(w, domain.NewErrorProjection(command, &domain.CommandError{Code: "reader_unavailable", Message: "Reader request failed"}), http.StatusServiceUnavailable)
		return
	}
	writeProjection(w, projection, nil)
}

// handleReaderSource serves GET /v1/reader/source.
func (s *Server) handleReaderSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.source", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil {
		return
	}
	projection, err := s.service.ReaderSource(r.Context(), app.ReaderSourceRequest{
		VaultPath: s.vault,
		Session:   s.readerSessionInfo(r),
	})
	writeReaderProjection(w, "reader.source", projection, err, http.StatusBadRequest)
}

// handleReaderNotes serves GET /v1/reader/notes.
func (s *Server) handleReaderNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.notes", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeMetadata) == nil {
		return
	}
	query := r.URL.Query()
	pageQuery, queryErr := parseReaderPageQuery(query, []string{"updated_desc", "created_desc", "title_asc"}, "updated_desc")
	if queryErr != nil {
		writeReaderError(w, r, "reader.notes", queryErr, http.StatusBadRequest)
		return
	}
	projection, err := s.service.ReaderNotes(r.Context(), app.ReaderNotesRequest{
		VaultPath:    s.vault,
		Session:      s.readerSessionInfo(r),
		Query:        pageQuery,
		CursorSecret: s.readerCursorSecret(),
	})
	writeReaderProjection(w, "reader.notes", projection, err, http.StatusBadRequest)
}

// handleReaderSearch serves GET /v1/reader/search. The q parameter is read
// once and never logged or echoed into facts.
func (s *Server) handleReaderSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeReaderError(w, r, "reader.search", app.ReaderError("reader_request_invalid", "reader routes only support GET", ""), http.StatusMethodNotAllowed)
		return
	}
	if s.requireReaderScope(w, r, app.ReaderScopeBody) == nil {
		return
	}
	query := r.URL.Query()
	pageQuery, queryErr := parseReaderPageQuery(query, []string{"relevance_desc"}, "relevance_desc")
	if queryErr != nil {
		writeReaderError(w, r, "reader.search", queryErr, http.StatusBadRequest)
		return
	}
	projection, err := s.service.ReaderSearch(r.Context(), app.ReaderSearchRequest{
		VaultPath:    s.vault,
		Session:      s.readerSessionInfo(r),
		QueryText:    strings.TrimSpace(query.Get("q")),
		Query:        pageQuery,
		CursorSecret: s.readerCursorSecret(),
	})
	writeReaderProjection(w, "reader.search", projection, err, http.StatusBadRequest)
}

// parseReaderPageQuery reads shared pagination controls with strict
// validation: unknown or out-of-range values are rejected, never guessed.
func parseReaderPageQuery(query url.Values, allowedSorts []string, defaultSort string) (app.ReaderPageQuery, *domain.CommandError) {
	invalid := func(message string) *domain.CommandError {
		return app.ReaderError("reader_request_invalid", message, "")
	}
	page := app.ReaderPageQuery{
		ProjectID: query.Get("project_id"),
		Status:    query.Get("status"),
		Sort:      query.Get("sort"),
		Cursor:    query.Get("cursor"),
		Tags:      query["tag"],
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return page, invalid("limit must be an integer")
		}
		// An EXPLICIT limit (including 0) must be within the frozen 1-200
		// range; only omission falls back to the default.
		if limit < 1 || limit > 200 {
			return page, invalid("limit must be between 1 and 200")
		}
		page.Limit = limit
	}
	// Reuse the app-layer normalizer so all callers share one rule set.
	normalized, err := app.NormalizeReaderPageQuery(page, allowedSorts, defaultSort)
	if err != nil {
		return page, err
	}
	return normalized, nil
}

func (s *Server) readerCursorSecret() []byte {
	if s.readerSessions == nil {
		return nil
	}
	return s.readerSessions.CursorSecret()
}

// readerAuth guards /v1/reader/ paths: bearer from the reader session
// registry only. Legacy tokens, temp tokens and MCP credentials cannot
// upgrade into reader scopes.
func (s *Server) readerAuth(w http.ResponseWriter, r *http.Request) bool {
	// Reader-surface responses — including authentication failures — carry
	// the reader cache policy; the auth middleware sits outside the cache
	// layer, so the header is set here at the reader boundary.
	w.Header().Set("Cache-Control", "private, no-store")
	if s.readerSessions == nil {
		// Reader capability not enabled on this server: explicit failure,
		// never a fallback to the legacy token check below.
		writeAuthError(w, "reader_disabled", "The human reading surface is not enabled on this server", http.StatusForbidden)
		return false
	}
	secret := extractBearerToken(r)
	if secret == "" {
		writeAuthError(w, "token_required", "Authorization Bearer reader session token is required", http.StatusUnauthorized)
		return false
	}
	session, err := s.readerSessions.Verify(secret)
	if err != nil {
		writeAuthError(w, "invalid_token", "Reader session authentication failed", http.StatusUnauthorized)
		return false
	}
	// Loopback only: the reading surface is a local-first contract.
	if !isLoopback(r) {
		writeAuthError(w, "loopback_required", "Reader routes only allow local access", http.StatusForbidden)
		return false
	}
	// Root identity re-verification: if the authorized vault directory was
	// replaced (mount swap, rename+recreate), the old session must not keep
	// reading the new content — fail with 409 source_changed and require a
	// fresh binding instead.
	if s.readerRootPinned != nil {
		current, statErr := os.Stat(s.vault)
		if statErr != nil || !os.SameFile(current, s.readerRootPinned) {
			writeReaderError(w, r, "reader.notes", app.ReaderError("source_changed", "The authorized vault root changed; re-bind the reading session", "Restart the supervised session against the intended vault"), http.StatusConflict)
			return false
		}
	}
	*r = *r.WithContext(withReaderSession(r.Context(), session))
	return true
}

func withReaderSession(ctx context.Context, session *ReaderSession) context.Context {
	return context.WithValue(ctx, readerSessionContextKey{}, session)
}
