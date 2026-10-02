package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
)

// TestReaderDocumentBudget413CarriesRecoverableDetails proves the transport
// serves the app-layer budget projection as-is: the public 413 response
// carries the machine-recoverable facts (fixed revision, true source size)
// and the source-export action instead of a bare error envelope.
func TestReaderDocumentBudget413CarriesRecoverableDetails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("a", 8*1024*1024+64)
	if err := os.WriteFile(filepath.Join(root, "notes", "huge.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_huge\ntitle: Huge\nstatus: active\n---\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewService().IndexRefresh(t.Context(), app.IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _ := app.GenerateReaderBearer()
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-413", BindingID: "binding-413",
		Scopes:    []string{app.ReaderScopeMetadata, app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()

	// First request without an explicit revision: the error must still carry
	// the RESOLVED revision, the true source size, and the export action.
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/reader/notes/note_huge/document", nil)
	request.Header.Set("Authorization", "Bearer "+bearer)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var envelope struct {
		Facts   map[string]string `json:"facts"`
		Actions []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		} `json:"actions"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "document_too_large" {
		t.Fatalf("error = %#v", envelope.Error)
	}
	if revision := envelope.Facts["revision"]; !strings.HasPrefix(revision, "sha256:") {
		t.Fatalf("revision fact = %#v", envelope.Facts)
	}
	if size := envelope.Facts["source_size_bytes"]; size == "" || size == "0" {
		t.Fatalf("source_size_bytes fact = %#v", envelope.Facts)
	}
	found := false
	for _, action := range envelope.Actions {
		if action.Name == "source_export" && strings.Contains(action.Command, "/v1/reader/notes/note_huge/source?revision=sha256:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("source_export action missing: %#v", envelope.Actions)
	}
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache control = %q", response.Header.Get("Cache-Control"))
	}
}
