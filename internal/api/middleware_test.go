package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func newAuthTestServer(t *testing.T, mode AuthMode) (*Server, string) {
	t.Helper()
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	opts := ServerOptions{AuthMode: mode}
	s := NewServerWithOptions(svc, root, opts)
	secret := s.tempSecret
	return s, secret
}

func TestAuthMiddleware_ZeroMode_PassesThrough(t *testing.T) {
	s, _ := newAuthTestServer(t, 0)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_TempMode_RequiresToken(t *testing.T) {
	s, _ := newAuthTestServer(t, AuthModeTemp)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "token_required") {
		t.Fatalf("expected token_required error, got: %s", res.Body.String())
	}
}

func TestAuthMiddleware_TempMode_ValidToken(t *testing.T) {
	s, secret := newAuthTestServer(t, AuthModeTemp)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_TempMode_InvalidToken(t *testing.T) {
	s, _ := newAuthTestServer(t, AuthModeTemp)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid token, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "invalid_token") {
		t.Fatalf("expected invalid_token, got: %s", res.Body.String())
	}
}

func TestAuthMiddleware_NoneMode_LoopbackPasses(t *testing.T) {
	s, _ := newAuthTestServer(t, AuthModeNone)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for loopback, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_NoneMode_NonLoopbackBlocked(t *testing.T) {
	s, _ := newAuthTestServer(t, AuthModeNone)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "loopback_required") {
		t.Fatalf("expected loopback_required, got: %s", res.Body.String())
	}
}

func TestAuthMiddleware_ScopeCheck_ReadScopeAllowsGET(t *testing.T) {
	s, secret := newAuthTestServer(t, AuthModeTemp)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_ReadOnlyPOSTUsesRouteReadonlyMetadata(t *testing.T) {
	s, _ := newAuthTestServer(t, AuthModeTemp)
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("read-only", map[TokenScope]ScopeTarget{ScopeRead: {}}, "", "test")
	if err := store.Create(rec); err != nil {
		t.Fatalf("create token: %v", err)
	}
	s.tokenStore = store

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/folders:repair-plan", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	s.Handler().ServeHTTP(res, req)
	if res.Code == http.StatusForbidden && strings.Contains(res.Body.String(), "insufficient_scope") {
		t.Fatalf("read-only POST route required write scope: %d %s", res.Code, res.Body.String())
	}
}
func TestAuthMiddleware_ReadScopeAllowsInboxAndDraftItemGET(t *testing.T) {
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAPIFixture(t, filepath.Join(root, "inbox", "inbox-one.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_inbox_one\ntitle: Inbox One\nstatus: inbox\nkind: inbox\n---\n\nbody\n")
	writeAPIFixture(t, filepath.Join(root, "drafts", "draft-one.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_draft_one\ntitle: Draft One\nstatus: draft\nkind: draft\n---\n\nbody\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("reader", map[TokenScope]ScopeTarget{ScopeRead: {}}, "", "test")
	if err := store.Create(rec); err != nil {
		t.Fatalf("create token: %v", err)
	}
	s := NewServerWithOptions(svc, root, ServerOptions{AuthMode: AuthModeTemp})
	s.tokenStore = store

	for _, path := range []string{"/v1/inbox/note_inbox_one", "/v1/drafts/note_draft_one"} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		s.Handler().ServeHTTP(res, req)
		if res.Code == http.StatusForbidden && strings.Contains(res.Body.String(), "insufficient_scope") {
			t.Fatalf("read-only item GET required write scope for %s: %d %s", path, res.Code, res.Body.String())
		}
	}
}

func TestAuthMiddleware_ActionScopeRestrictsRoutes(t *testing.T) {
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	s := NewServerWithOptions(svc, root, ServerOptions{AuthMode: AuthModeTemp, AllowWrite: true})
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("draft-action", map[TokenScope]ScopeTarget{ScopeWrite: {Groups: []string{"drafts"}, Actions: []string{"draft.promote"}}}, "", "test")
	if err := store.Create(rec); err != nil {
		t.Fatalf("create token: %v", err)
	}
	s.tokenStore = store

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/drafts/note_missing:discard?yes=true", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), "insufficient_scope") {
		t.Fatalf("draft discard should be blocked by action scope, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_HiddenRouteReturnsNotFoundBeforeAuth(t *testing.T) {
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	s := NewServerWithOptions(svc, root, ServerOptions{AuthMode: AuthModeTemp, HideGroups: []string{"folders"}})

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/folders", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound || !strings.Contains(res.Body.String(), "route_not_found") {
		t.Fatalf("hidden route should return route_not_found before auth, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddleware_ExposeGroups(t *testing.T) {
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	opts := ServerOptions{
		AuthMode:     0,
		ExposeGroups: []string{"capabilities"},
	}
	s := NewServerWithOptions(svc, root, opts)

	// Capabilities should be available
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for exposed group, got %d", res.Code)
	}

	// Folders should be hidden (404)
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/folders", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for hidden group, got %d", res.Code)
	}
}

func TestAuthMiddleware_HideGroups(t *testing.T) {
	ctx := backgroundContext(t)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	opts := ServerOptions{
		AuthMode:   0,
		HideGroups: []string{"folders"},
	}
	s := NewServerWithOptions(svc, root, opts)

	// Folders should be hidden
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/folders", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for hidden group, got %d", res.Code)
	}

	// Capabilities should still be available
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for non-hidden group, got %d", res.Code)
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty", "", ""},
		{"no prefix", "Token abc", ""},
		{"valid", "Bearer abc123", "abc123"},
		{"bearer lowercase", "bearer abc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(r)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		remote string
		want   bool
	}{
		{"", true}, // httptest default empty
		{"127.0.0.1:1234", true},
		{"[::1]:1234", true},
		{"10.0.0.1:1234", false},
		{"192.168.1.1:80", false},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if got := isLoopback(r); got != tt.want {
				t.Fatalf("isLoopback(%q) = %v, want %v", tt.remote, got, tt.want)
			}
		})
	}
}

func TestLookupRouteGroup(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v1/capabilities", "capabilities"},
		{"/v1/folders", "folders"},
		{"/v1/folders/", "folders"},
		{"/v1/notes/note-001", "notes"},
		{"/v1/inbox", "inbox"},
		{"/v1/inbox/item-001", "inbox"},
		{"/v1/drafts", "drafts"},
		{"/v1/drafts/item-001", "drafts"},
		{"/v1/projects/proj-001", "projects"},
		{"/v1/project-items/item-001", "projects"},
		{"/v1/unknown", ""},
		{"/v2/something", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := lookupRouteGroup(tt.path)
			if got != tt.want {
				t.Fatalf("lookupRouteGroup(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRequiredScopeForMethod(t *testing.T) {
	if requiredScopeForMethod(http.MethodGet) != ScopeRead {
		t.Fatal("GET should require read scope")
	}
	if requiredScopeForMethod(http.MethodHead) != ScopeRead {
		t.Fatal("HEAD should require read scope")
	}
	if requiredScopeForMethod(http.MethodPost) != ScopeWrite {
		t.Fatal("POST should require write scope")
	}
	if requiredScopeForMethod(http.MethodPut) != ScopeWrite {
		t.Fatal("PUT should require write scope")
	}
	if requiredScopeForMethod(http.MethodDelete) != ScopeWrite {
		t.Fatal("DELETE should require write scope")
	}
}

func backgroundContext(t *testing.T) context.Context {
	t.Helper()
	return t.Context()
}
