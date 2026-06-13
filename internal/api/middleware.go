package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// AuthMode defines the authentication mode for the API server.
type AuthMode int

const (
	AuthModeUnset     AuthMode = iota // zero value: no auth (tests)
	AuthModeTemp                      // default: temp token in memory
	AuthModeTokenFile                 // --token-file: long-lived tokens from file
	AuthModeNone                      // --no-auth: no auth, loopback only
)

// RouteInfo describes a route's group and method for scope checking.
type RouteInfo struct {
	Group    string
	Method   string
	Action   string
	Readonly bool
}

// routeGroupMap maps URL path patterns to route groups.
var routeGroupMap = map[string]RouteInfo{
	"/":                       {Group: "capabilities", Method: "GET", Readonly: true},
	"/v1/capabilities":        {Group: "capabilities", Method: "GET", Readonly: true},
	"/v1/folders":             {Group: "folders", Method: "GET", Readonly: true},
	"/v1/folders/":            {Group: "folders", Method: "GET", Readonly: false},
	"/v1/folders:repair-plan": {Group: "folders", Method: "POST", Action: "folder.repair", Readonly: true},
	"/v1/inbox":               {Group: "inbox", Method: "GET", Readonly: true},
	"/v1/inbox:capture":       {Group: "inbox", Method: "POST", Action: "inbox.capture", Readonly: false},
	"/v1/inbox/":              {Group: "inbox", Method: "GET", Readonly: false},
	"/v1/drafts":              {Group: "drafts", Method: "GET", Readonly: true},
	"/v1/drafts/":             {Group: "drafts", Method: "GET", Readonly: false},
	"/v1/notes/":              {Group: "notes", Method: "GET", Readonly: true},
	"/v1/project-items/":      {Group: "projects", Method: "POST", Readonly: true},
	"/v1/projects/":           {Group: "projects", Method: "GET", Readonly: true},
}

type auditStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *auditStatusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *auditStatusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func (s *Server) lookupRequestRouteInfo(r *http.Request) (RouteInfo, bool) {
	if r.URL.Path == "/v1/rpc" {
		return s.lookupRPCRequestRouteInfo(r)
	}
	return lookupRouteInfo(r.URL.Path, r.Method)
}

func (s *Server) lookupRPCRequestRouteInfo(r *http.Request) (RouteInfo, bool) {
	info := RouteInfo{Group: "rpc", Method: http.MethodPost, Readonly: true}
	if r.Method != http.MethodPost || r.Body == nil {
		return info, true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRPCBodyBytes+1))
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || int64(len(body)) > maxRPCBodyBytes {
		return info, true
	}
	var req HTTPRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return info, true
	}
	route, ok := app.FindRemoteRPCMethod(req.Method)
	if !ok {
		return info, true
	}
	return RouteInfo{Group: rpcRouteGroup(route), Method: http.MethodPost, Action: route.Command, Readonly: route.Readonly}, true
}

func lookupRouteInfo(path string, method string) (RouteInfo, bool) {
	// Exact match first
	if info, ok := routeGroupMap[path]; ok {
		if path == "/v1/drafts" && method == http.MethodPost {
			info.Readonly = false
			info.Action = "draft.create"
		}
		return info, true
	}
	// Try prefix matches for parameterized routes
	if strings.HasPrefix(path, "/v1/inbox/") {
		info := routeGroupMap["/v1/inbox/"]
		info.Readonly = method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
		info.Action = inboxRouteAction(path)
		return info, true
	}
	if strings.HasPrefix(path, "/v1/drafts/") {
		info := routeGroupMap["/v1/drafts/"]
		info.Readonly = method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
		info.Action = draftRouteAction(path)
		return info, true
	}
	if strings.HasPrefix(path, "/v1/project-items/") {
		return routeGroupMap["/v1/project-items/"], true
	}
	if strings.HasPrefix(path, "/v1/projects/") {
		return routeGroupMap["/v1/projects/"], true
	}
	if strings.HasPrefix(path, "/v1/folders/") {
		return routeGroupMap["/v1/folders/"], true
	}
	if strings.HasPrefix(path, "/v1/notes/") {
		return routeGroupMap["/v1/notes/"], true
	}
	return RouteInfo{}, false
}

func inboxRouteAction(path string) string {
	if _, action, ok := strings.Cut(strings.TrimPrefix(path, "/v1/inbox/"), ":"); ok {
		return "inbox." + action
	}
	return ""
}

func draftRouteAction(path string) string {
	if _, action, ok := strings.Cut(strings.TrimPrefix(path, "/v1/drafts/"), ":"); ok {
		return "draft." + action
	}
	return ""
}

func lookupRouteGroup(path string) string {
	info, ok := lookupRouteInfo(path, http.MethodGet)
	if !ok {
		return ""
	}
	return info.Group
}

func requiredScopeForMethod(method string) TokenScope {
	return requiredScopeForRoute(method, RouteInfo{})
}

func requiredScopeForRoute(method string, route RouteInfo) TokenScope {
	if route.Readonly {
		return ScopeRead
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ScopeRead
	default:
		return ScopeWrite
	}
}

// authMiddleware validates tokens and enforces scope requirements.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, knownRoute := s.lookupRequestRouteInfo(r)
		group := info.Group
		if knownRoute && !s.isGroupExposed(group) {
			s.handleRouteNotFound(w, r)
			return
		}
		// No auth configured (AuthModeUnset, used by tests): pass through
		if s.authMode == AuthModeUnset {
			next.ServeHTTP(w, r)
			return
		}

		// No-auth mode: only check loopback
		if s.authMode == AuthModeNone {
			if !isLoopback(r) {
				s.writeAudit("no-auth", r.Method, r.URL.Path, "", group, http.StatusForbidden)
				writeAuthError(w, "loopback_required", "No-auth mode only allows local access", http.StatusForbidden)
				return
			}
			recorder := &auditStatusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			s.writeAudit("no-auth", r.Method, r.URL.Path, "", group, status)
			return
		}

		// Extract Bearer token
		secret := extractBearerToken(r)
		if secret == "" {
			s.writeAudit("", r.Method, r.URL.Path, "", group, http.StatusUnauthorized)
			writeAuthError(w, "token_required", "Authorization Bearer token is required", http.StatusUnauthorized)
			return
		}

		// Verify token
		record, err := s.tokenStore.Verify(secret)
		if err != nil {
			code := "invalid_token"
			status := http.StatusUnauthorized
			if strings.Contains(err.Error(), "expired") {
				code = "token_expired"
			}
			s.writeAudit("", r.Method, r.URL.Path, "", group, status)
			writeAuthError(w, code, "Authentication failed: "+err.Error(), status)
			return
		}

		// Check scope
		required := requiredScopeForRoute(r.Method, info)
		if !HasScopeForAction(record, required, group, info.Action) {
			s.writeAudit(record.ID, r.Method, r.URL.Path, string(required), group, http.StatusForbidden)
			writeAuthError(w, "insufficient_scope", "token scope is insufficient: requires "+string(required)+" for "+group, http.StatusForbidden)
			return
		}

		recorder := &auditStatusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		s.writeAudit(record.ID, r.Method, r.URL.Path, string(required), group, status)
	})
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimPrefix(auth, prefix)
}

// isLoopback checks if the request comes from a loopback address.
func isLoopback(r *http.Request) bool {
	if r.RemoteAddr == "" {
		return true // httptest recorder, treat as loopback
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

// writeAuthError writes a JSON error response for auth failures.
func writeAuthError(w http.ResponseWriter, code, message string, status int) {
	proj := domain.NewErrorProjection("api.auth", &domain.CommandError{
		Code:    code,
		Message: message,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(proj)
}
