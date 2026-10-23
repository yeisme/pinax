package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterDailyEditHTTP(t *testing.T) {
	root := readerHTTPVault(t)
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{app.WriterScopeEditor, app.ReaderScopeBody, app.ReaderScopeMetadata}
	if err = sessions.Create(ReaderSession{SessionID: "edit-http", BindingID: "edit-binding", Scopes: scopes, ExpiresAt: time.Now().Add(time.Hour)}, bearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	status, envelope := writerPost(t, server, bearer, "/v1/writer/journals/daily:propose", `{"date":"2026-08-20","timezone":"UTC","operation_id":"create-http"}`)
	if status != 200 {
		t.Fatal("create proposal failed", status)
	}
	created := writerData(t, envelope)
	status, envelope = writerPost(t, server, bearer, "/v1/writer/journals/daily:apply", encode(map[string]any{"proposal": created, "authorization": "reviewed_proposal"}))
	if status != 200 {
		t.Fatal("create failed", status)
	}
	revision := writerData(t, envelope)["revision_after"].(string)
	id := created["object_id"].(string)
	query := url.Values{"date": {"2026-08-20"}, "timezone": {"UTC"}, "object_id": {id}, "revision": {revision}}
	sourcePath := "/v1/writer/journals/daily/editor-source?" + query.Encode()
	get := func(token, path string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("cacheable")
		}
		var out map[string]any
		if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, out
	}
	status, envelope = get(bearer, sourcePath)
	if status != 200 {
		t.Fatal("daily source failed", status)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterEditorSourceV1Alpha1)
	before := writerData(t, envelope)["body"]
	payload := map[string]any{"date": "2026-08-20", "timezone": "UTC", "object_id": id, "revision": revision, "operation_id": "edit-original", "body": "\nExact HTTP revised daily.  \r\n"}
	status, envelope = writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:propose", encode(payload))
	if status != 200 {
		t.Fatal("edit proposal failed", status)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterDailyEditProposalV1Alpha1)
	proposal := writerData(t, envelope)
	preview := proposal["preview"].(map[string]any)
	if preview["before"] != before || preview["after"] != payload["body"] {
		t.Fatal("diff mismatch")
	}
	tampered := map[string]any{}
	for key, value := range proposal {
		tampered[key] = value
	}
	tampered["date"] = "2026-08-21"
	status, _ = writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:apply", encode(map[string]any{"proposal": tampered, "authorization": "reviewed_proposal"}))
	if status != 409 {
		t.Fatal("HMAC tampering admitted", status)
	}
	apply := encode(map[string]any{"proposal": proposal, "authorization": "reviewed_proposal"})
	status, envelope = writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:apply", apply)
	if status != 200 {
		t.Fatal("edit apply failed", status)
	}
	assertWriterDataConformance(t, envelope, catalogschema.WriterApplyReceiptV1Alpha1)
	receipt := writerData(t, envelope)
	if receipt["status"] != "succeeded" || receipt["capability_id"] != "writer.daily.edit" || receipt["request_digest"] != proposal["preview_digest"] {
		t.Fatal("wrong edit receipt")
	}
	status, replay := writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:apply", apply)
	if status != 200 || !reflect.DeepEqual(receipt, writerData(t, replay)) {
		t.Fatal("receipt replay changed")
	}
	query.Set("revision", receipt["revision_after"].(string))
	status, envelope = get(bearer, "/v1/writer/journals/daily/editor-source?"+query.Encode())
	if status != 200 || writerData(t, envelope)["body"] != payload["body"] {
		t.Fatal("edited source did not read back", status)
	}
	if status, _ = get(bearer, sourcePath); status != 409 {
		t.Fatal("stale source accepted", status)
	}
	if status, _ = get(bearer, sourcePath+"&date=2026-08-20"); status != 400 {
		t.Fatal("duplicate query admitted")
	}
	payload["force"] = true
	status, _ = writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:propose", encode(payload))
	if status != 400 {
		t.Fatal("unknown write field admitted")
	}
	for i := range scopes {
		token, _ := app.GenerateReaderBearer()
		reduced := append([]string{}, scopes[:i]...)
		reduced = append(reduced, scopes[i+1:]...)
		if err = sessions.Create(ReaderSession{SessionID: "reduced-" + scopes[i], BindingID: "edit-binding", Scopes: reduced, ExpiresAt: time.Now().Add(time.Hour)}, token); err != nil {
			t.Fatal(err)
		}
		if status, _ = get(token, sourcePath); status != 403 {
			t.Fatal("missing source scope admitted", status)
		}
		if status, _ = writerPost(t, server, token, "/v1/writer/journals/daily/edit:apply", apply); status != 403 {
			t.Fatal("missing apply scope admitted", status)
		}
	}
	sessions.Revoke("edit-http")
	status, _ = writerPost(t, server, bearer, "/v1/writer/journals/daily/edit:apply", apply)
	if status != 401 {
		t.Fatal("revoked replay admitted", status)
	}
}
