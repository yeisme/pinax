package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// CachePolicy defines caching parameters for a route.
type CachePolicy struct {
	MaxAge int    // seconds
	Scope  string // "public" or "private"
}

// defaultCachePolicies defines default cache policies by route pattern.
var defaultCachePolicies = map[string]CachePolicy{
	"/v1/capabilities":    {MaxAge: 300, Scope: "public"},
	"/v1/monitor/runs":    {MaxAge: 5, Scope: "private"},
	"/v1/monitor/runs/":   {MaxAge: 30, Scope: "private"},
	"/v1/monitor/summary": {MaxAge: 5, Scope: "private"},
	"/v1/folders":         {MaxAge: 60, Scope: "private"},
	"/v1/folders/":        {MaxAge: 60, Scope: "private"},
	"/v1/notes/":          {MaxAge: 30, Scope: "private"},
	"/v1/inbox":           {MaxAge: 10, Scope: "private"},
	"/v1/inbox/":          {MaxAge: 10, Scope: "private"},
	"/v1/drafts":          {MaxAge: 10, Scope: "private"},
	"/v1/drafts/":         {MaxAge: 10, Scope: "private"},
	"/v1/projects/":       {MaxAge: 30, Scope: "private"},
}

// cacheMiddleware adds Cache-Control and ETag support for GET requests.
func (s *Server) cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only cache GET requests
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		// Wrap response to capture body for ETag computation.
		// This wrapper delays WriteHeader calls so we can override with 304.
		cw := &captureWriter{ResponseWriter: w, ifNoneMatch: r.Header.Get("If-None-Match")}

		next.ServeHTTP(cw, r)

		// If body was JSON and we computed an ETag, check conditional request
		if cw.etag != "" {
			if cw.ifNoneMatch == cw.etag {
				w.Header().Set("ETag", cw.etag)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			// Apply cache headers for the normal response
			policy := lookupCachePolicy(r.URL.Path)
			if policy != nil {
				var cacheControl string
				if policy.Scope == "private" {
					cacheControl = "private, max-age=" + itoa(policy.MaxAge)
				} else {
					cacheControl = "public, max-age=" + itoa(policy.MaxAge)
				}
				w.Header().Set("Cache-Control", cacheControl)
				w.Header().Set("ETag", cw.etag)
			}
		}

		// Flush buffered output to real writer
		cw.flushTo(w)
	})
}

// captureWriter buffers the response body to compute ETag before sending.
type captureWriter struct {
	http.ResponseWriter
	statusCode  int
	body        []byte
	ifNoneMatch string
	etag        string
	headerSent  bool
}

func (w *captureWriter) WriteHeader(code int) {
	w.statusCode = code
	// Don't send to underlying writer yet — we may want to override with 304
}

func (w *captureWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		w.body = append(w.body, b...)
		h := sha256.Sum256(w.body)
		w.etag = `"` + hex.EncodeToString(h[:]) + `"`
		return len(b), nil
	}
	// Non-JSON: pass through immediately
	return w.ResponseWriter.Write(b)
}

// flushTo sends the buffered response to the real ResponseWriter.
func (w *captureWriter) flushTo(dest http.ResponseWriter) {
	if w.headerSent {
		return
	}
	w.headerSent = true
	// Copy headers from capture writer to destination
	for k, vv := range w.Header() {
		for _, v := range vv {
			dest.Header().Add(k, v)
		}
	}
	if w.statusCode != 0 {
		dest.WriteHeader(w.statusCode)
	}
	if len(w.body) > 0 {
		_, _ = dest.Write(w.body)
	}
}

// lookupCachePolicy finds the cache policy for a path.
func lookupCachePolicy(path string) *CachePolicy {
	// Exact match
	if p, ok := defaultCachePolicies[path]; ok {
		return &p
	}
	// Prefix match for parameterized routes
	for pattern, policy := range defaultCachePolicies {
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
			return &policy
		}
	}
	return nil
}

// itoa converts int to string without importing strconv for this small use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
