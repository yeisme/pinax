package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/internal/version/versiontest"
)

func TestWriterTagsHTTPUnknownDoesNotResume(t *testing.T) {
	root := readerHTTPVault(t)
	path := filepath.Join(root, "notes/alpha.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original = []byte(strings.Replace(string(original), "status: active", "tags: [old]\nstatus: active", 1))
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	sha := sha256.Sum256(original)
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{SessionID: "tags-unknown", BindingID: "binding", Scopes: []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata}, ExpiresAt: time.Now().Add(time.Hour)}, token); err != nil {
		t.Fatal(err)
	}
	backend := &versiontest.FakeBackend{SnapshotErr: errors.New("fixture snapshot failure")}
	server := httptest.NewServer(NewServerWithOptions(app.NewServiceWithVersionBackend(backend), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	post := func(route string, payload any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest("POST", server.URL+route, strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("unexpected status/cache policy", resp.StatusCode)
		}
		var envelope map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		return envelope
	}
	plan := writerData(t, post("/v1/writer/tags:plan", map[string]any{"operation_id": "unknown-http", "operation": "delete", "old_tag": "old", "new_tag": "", "targets": []any{map[string]any{"object_id": "note_alpha", "revision": "sha256:" + hex.EncodeToString(sha[:])}}}))
	envelope := post("/v1/writer/tags:apply", map[string]any{"plan": plan, "authorization": "reviewed_proposal"})
	assertWriterDataConformance(t, envelope, catalogschema.WriterTagsReceiptV1Alpha1)
	receipt := writerData(t, envelope)
	if receipt["status"] != "reconcile_required" || receipt["reconcile_required"] != true {
		t.Fatal("200 mistaken for completion")
	}
	backend.SnapshotErr = nil
	replay := writerData(t, post("/v1/writer/tags:lookup", map[string]any{"operation_id": "unknown-http"}))
	if !reflect.DeepEqual(receipt, replay) {
		t.Fatal("lookup rewrote unknown receipt")
	}
	replay = writerData(t, post("/v1/writer/tags:apply", map[string]any{"plan": plan, "authorization": "reviewed_proposal"}))
	if !reflect.DeepEqual(receipt, replay) {
		t.Fatal("replay resumed unknown operation")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatal("unknown replay changed source")
	}
}
