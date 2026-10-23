package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterTagsHTTP(t *testing.T) {
	root := readerHTTPVault(t)
	path := filepath.Join(root, "notes/alpha.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "status: active", "tags: [old]\nstatus: active", 1))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	revision := "sha256:" + hex.EncodeToString(digest[:])
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata}
	add := func(id string, scopes []string) string {
		token, err := app.GenerateReaderBearer()
		if err != nil {
			t.Fatal(err)
		}
		if err := sessions.Create(ReaderSession{SessionID: id, BindingID: "tag-binding", Scopes: scopes, ExpiresAt: time.Now().Add(time.Hour)}, token); err != nil {
			t.Fatal(err)
		}
		return token
	}
	token := add("tags-http", scopes)
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	request := map[string]any{"operation_id": "tags-http-original", "operation": "rename", "old_tag": "old", "new_tag": "new/tag", "targets": []any{map[string]any{"object_id": "note_alpha", "revision": revision}}}
	status, envelope := writerPost(t, server, token, "/v1/writer/tags:plan", encode(request))
	if status != 200 {
		t.Fatalf("plan %d %v", status, envelope)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterTagsPlanV1Alpha1)
	plan := writerData(t, envelope)
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("plan wrote source")
	}
	apply := encode(map[string]any{"plan": plan, "authorization": "reviewed_proposal"})
	bad := map[string]any{}
	for k, v := range plan {
		bad[k] = v
	}
	bad["new_tag"] = "tampered"
	if status, _ := writerPost(t, server, token, "/v1/writer/tags:apply", encode(map[string]any{"plan": bad, "authorization": "reviewed_proposal"})); status != 409 {
		t.Fatal("tamper admitted", status)
	}
	status, envelope = writerPost(t, server, token, "/v1/writer/tags:apply", apply)
	if status != 200 {
		t.Fatalf("apply %d %v", status, envelope)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterTagsReceiptV1Alpha1)
	receipt := writerData(t, envelope)
	if receipt["status"] != "succeeded" {
		t.Fatal("not completed", receipt)
	}
	lookup := `{"operation_id":"tags-http-original"}`
	status, replayed := writerPost(t, server, token, "/v1/writer/tags:lookup", lookup)
	if status != 200 || !reflect.DeepEqual(receipt, writerData(t, replayed)) {
		t.Fatal("original receipt changed", status)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "tags: [new/tag]") || !strings.Contains(string(after), "Alpha contains") {
		t.Fatal("real source not updated")
	}
	for _, route := range []string{"plan", "apply", "lookup"} {
		payload := lookup
		if route == "plan" {
			payload = encode(request)
		}
		if route == "apply" {
			payload = apply
		}
		for _, query := range []string{"?", "?x=1"} {
			if status, _ := writerPost(t, server, token, "/v1/writer/tags:"+route+query, payload); status != 400 {
				t.Fatal("query admitted", route, status)
			}
		}
		for i := range scopes {
			reduced := append([]string{}, scopes[:i]...)
			reduced = append(reduced, scopes[i+1:]...)
			weak := add(route+scopes[i], reduced)
			if status, _ := writerPost(t, server, weak, "/v1/writer/tags:"+route, payload); status != 403 {
				t.Fatal("missing scope admitted", route, status)
			}
		}
	}
	for _, payload := range []string{`{"operation_id":"tags-http-original","operation_id":"other"}`, `{"operation_id":"tags-http-original","\u006fperation_id":"other"}`, `{"operation_id":"\ud800"}`, `{"operation_id":"tags-http-original","session_id":"injected"}`, lookup + "{}", strings.Repeat(" ", 2049) + lookup} {
		if status, _ := writerPost(t, server, token, "/v1/writer/tags:lookup", payload); status != 400 {
			t.Fatal("invalid JSON admitted", status)
		}
	}
	foreign := add("foreign", scopes)
	if status, _ := writerPost(t, server, foreign, "/v1/writer/tags:lookup", lookup); status != 404 {
		t.Fatal("foreign receipt exposed", status)
	}
	sessions.Revoke("tags-http")
	if status, _ := writerPost(t, server, token, "/v1/writer/tags:lookup", lookup); status != 401 {
		t.Fatal("revoked lookup admitted", status)
	}
}

func TestWriterTagsHTTPStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"nested":{"x":1,"x":2}}`, `{"x":"\ud800"}`, `{"x":"\udc00"}`, string([]byte{'"', 0xff, '"'}), strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34)} {
		if validTagsJSON([]byte(raw)) {
			t.Fatal("unsafe JSON accepted")
		}
	}
	for _, raw := range []string{`{"x":"\ud83d\ude00"}`, `{"x":"\\ud800"}`, `{"x":"escaped \" quote"}`, `{"x":[{"a":1},{"a":2}]}`} {
		if !validTagsJSON([]byte(raw)) {
			t.Fatal("valid JSON refused", raw)
		}
	}
}
