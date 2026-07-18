package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharePublishedScopeHandlerServesStaticAndBoundedAPI(t *testing.T) {
	outDir := t.TempDir()
	writeAppFixture(t, filepath.Join(outDir, "index.html"), "<html>published</html>")
	writeAppFixture(t, filepath.Join(outDir, "pinax-data", "search-index.json"), `{"entries":[{"id":"note_public","title":"Public","path":"notes/public/","tags":["public"],"kind":"concept","body":"PRIVATE BODY MUST NOT LEAK"}]}`)

	server := httptest.NewServer(sharePublishedHandler(outDir))
	defer server.Close()

	webResp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = webResp.Body.Close() }()
	if webResp.StatusCode != http.StatusOK {
		t.Fatalf("web status = %d", webResp.StatusCode)
	}

	apiResp, err := http.Get(server.URL + "/api/share/notes")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = apiResp.Body.Close() }()
	var payload map[string]any
	if err := json.NewDecoder(apiResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode api payload: %v", err)
	}
	body := mustJSON(t, payload)
	if !strings.Contains(body, "note_public") || !strings.Contains(body, "Public") {
		t.Fatalf("api payload missing bounded note fields: %s", body)
	}
	for _, forbidden := range []string{"PRIVATE BODY MUST NOT LEAK", "body", ".pinax"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("api payload leaked %q: %s", forbidden, body)
		}
	}
}

func TestShareVaultReadonlyScopeRequiresTokenAndReturnsBoundedNotes(t *testing.T) {
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "private.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_private\ntitle: Private\nkind: concept\nstatus: active\ntags: [team]\n---\n\nPRIVATE BODY MUST NOT LEAK")
	server := httptest.NewServer(shareVaultReadonlyHandler(root, "share-token"))
	defer server.Close()

	unauthorized, err := http.Get(server.URL + "/api/share/notes")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unauthorized.Body.Close() }()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/share/notes", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer share-token")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode vault-readonly payload: %v", err)
	}
	body := mustJSON(t, payload)
	if !strings.Contains(body, "note_private") || !strings.Contains(body, "Private") || !strings.Contains(body, "card") {
		t.Fatalf("vault-readonly payload missing bounded note fields: %s", body)
	}
	for _, forbidden := range []string{"PRIVATE BODY MUST NOT LEAK", "body", "share-token", ".pinax"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("vault-readonly payload leaked %q: %s", forbidden, body)
		}
	}

	postRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/share/notes", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	postRequest.Header.Set("Authorization", "Bearer share-token")
	postRequest.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = postResp.Body.Close() }()
	if postResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("mutation status = %d", postResp.StatusCode)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
