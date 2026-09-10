package inputintake

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

//go:embed upload.html
var pageHTML []byte

func ParsePath(p string) (string, string, bool) {
	if !strings.HasPrefix(p, Prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(p, Prefix), "/")
	if len(parts) > 2 || !validID(parts[0]) {
		return "", "", false
	}
	op := ""
	if len(parts) == 2 {
		op = parts[1]
	}
	return parts[0], op, true
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	id, op, ok := ParsePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u, _ := url.Parse(s.origin)
	// Do not derive authority from forwarded headers or arbitrary Host values.
	if r.Host != u.Host || r.Header.Get("Authorization") != "" {
		s.fail(w, ErrDenied)
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != s.origin {
		s.fail(w, ErrDenied)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" && r.Method != "GET" {
		s.fail(w, ErrDenied)
		return
	}
	if r.Method == http.MethodGet && op == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(strings.NewReplacer("./PLACEHOLDER", id+"/app.js", "./STYLESHEET", id+"/style.css").Replace(string(pageHTML))))
		return
	}
	if r.Method == http.MethodGet && op == "style.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(pageCSS)
		return
	}
	if r.Method == http.MethodGet && op == "app.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(pageJS)
		return
	}
	if r.Method == http.MethodPost && op == "exchange" {
		var input struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &input) {
			return
		}
		secret, expires, e := s.Exchange(r.Context(), id, input.Token)
		if e != nil {
			s.fail(w, e)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "input_session", Value: secret, Path: strings.TrimRight(strings.TrimPrefix(s.opt.BaseURL, s.origin), "/") + Prefix + id, Secure: u.Scheme == "https", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expires})
		s.reply(w, map[string]any{"exchanged": true})
		return
	}
	secret := r.Header.Get(GrantHeader)
	browser := false
	if cookie, e := r.Cookie("input_session"); e == nil {
		if secret != "" {
			s.fail(w, ErrDenied)
			return
		}
		secret = cookie.Value
		browser = true
	}
	// Browser writes require exact Origin. Native HTTP clients use the grant header.
	if browser && r.Method != http.MethodGet && origin != s.origin {
		s.fail(w, ErrDenied)
		return
	}
	who, e := s.authenticate(r.Context(), id, secret, browser)
	if e != nil {
		s.fail(w, e)
		return
	}
	var result View
	switch {
	case op == "status" && r.Method == http.MethodGet:
		result, e = s.Status(r.Context(), who, id)
	case op == "file" && r.Method == http.MethodPost:
		var f File
		if !decode(w, r, &f) {
			return
		}
		result, e = s.Bind(r.Context(), who, id, f)
	case op == "content" && r.Method == http.MethodPut:
		// Bound blocked socket reads to less than the durable operation lease.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(9 * time.Minute))
		result, e = s.Put(r.Context(), who, id, r.ContentLength, http.MaxBytesReader(w, r.Body, s.opt.MaxBytes))
	case op == "complete" && r.Method == http.MethodPost:
		result, e = s.Complete(r.Context(), who, id)
	case op == "abort" && r.Method == http.MethodPost:
		result, e = s.Abort(r.Context(), who, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	s.reply(w, result)
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid input JSON", 400)
		return false
	}
	return true
}
func (s *Service) reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Service) fail(w http.ResponseWriter, e error) {
	code := 400
	if e == ErrDenied || e == ErrAdmission {
		code = 403
	} else if e == ErrExpired {
		code = 410
	} else if e == ErrConflict {
		code = 409
	} else if e == ErrDisabled {
		code = 503
	} else if e == ErrCapacity {
		code = 507
	} else if e == ErrStorage {
		code = 503
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": safeError(e).Error()})
}

// CheckTransientLink validates owner-produced links before MCP transport delivery.
func CheckTransientLink(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	if u.RawPath != "" || path.Clean(u.Path) != u.Path {
		return false
	}
	index := strings.LastIndex(u.Path, Prefix)
	if index < 0 {
		return false
	}
	_, op, ok := ParsePath(u.Path[index:])
	if !ok || op != "" {
		return false
	}
	parts := strings.Split(u.Fragment, "=")
	if len(parts) != 2 || (parts[0] != "page" && parts[0] != "grant") || len(parts[1]) != 64 {
		return false
	}
	return isHex(parts[1])
}
func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ReadCapability is safe for resource output; it contains no credentials.
func (s *Service) ReadCapability(_ context.Context) any { return s.Capabilities() }
