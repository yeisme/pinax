package localsession_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/pkg/localsession"
)

func ready() localsession.Event {
	readonly := true
	return localsession.Event{SchemaVersion: localsession.EventVersion, Type: "ready", SessionID: "rss_fixture", SessionNonce: strings.Repeat("n", 32), BindingID: "binding-fixture", Address: "http://127.0.0.1:12345", ContractDigest: "sha256:" + strings.Repeat("a", 64), Readonly: &readonly}
}
func TestReaderSessionPublicTypesMatchOwnerSchema(t *testing.T) {
	event := ready()
	frame := localsession.Bootstrap{SchemaVersion: localsession.BootstrapVersion, SessionNonce: event.SessionNonce, Bearer: strings.Repeat("b", 64), BindingID: event.BindingID, VaultRoot: "/fixture/vault", Scopes: []string{"reader.metadata"}, Readonly: true}
	for _, tc := range []struct {
		id    string
		value any
	}{{schema.LocalSessionBootstrapV1Alpha1, frame}, {schema.LocalSessionEventV1Alpha1, event}} {
		encoded, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if json.Unmarshal(encoded, &decoded) != nil {
			t.Fatal("invalid fixture")
		}
		if err := schema.ValidatePayload(schema.Default(), tc.id, decoded); err != nil {
			t.Fatal("public type drifted from owner schema")
		}
	}
	if localsession.MaxFrameBytes != schema.ReaderSessionBootstrapMaxFrame {
		t.Fatal("frame budget drift")
	}
	for _, scope := range []string{app.WriterScopeCapture, app.WriterScopeEditor, app.WriterScopeOrganizer, app.ReaderScopeBody, "writer.unknown"} {
		frame.Scopes = []string{scope}
		if frame.HasWriterScope() != app.IsWriterScope(scope) {
			t.Fatal("writer vocabulary drift")
		}
	}
}
func TestReaderSessionDecodeReadyStrictHostBoundary(t *testing.T) {
	expected := ready()
	encode := func(e localsession.Event) []byte {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	valid := encode(expected)
	if _, err := localsession.DecodeReady(valid, expected.SessionNonce, expected.BindingID, expected.ContractDigest, true); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"http://localhost:12345", "http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:01234", "https://127.0.0.1:12345", "http://user@127.0.0.1:12345", "http://127.0.0.1:12345/", "http://127.0.0.1:12345?", "http://127.0.0.1:12345/#x", "http://10.0.0.1:12345", "http://[::1]:12345"} {
		e := expected
		e.Address = address
		if _, err := localsession.DecodeReady(encode(e), expected.SessionNonce, expected.BindingID, expected.ContractDigest, true); err == nil {
			t.Fatal("unsafe address accepted")
		}
	}
	for _, mutate := range []func(*localsession.Event){func(e *localsession.Event) { e.SessionNonce = "wrong" }, func(e *localsession.Event) { e.BindingID = "wrong" }, func(e *localsession.Event) { e.ContractDigest = "sha256:" + strings.Repeat("b", 64) }, func(e *localsession.Event) { e.Readonly = nil }, func(e *localsession.Event) { f := false; e.Readonly = &f }, func(e *localsession.Event) { e.Type = "error"; e.Message = "private fixture diagnostic" }, func(e *localsession.Event) { e.SchemaVersion = "future" }, func(e *localsession.Event) { e.SessionID = "" }} {
		e := expected
		mutate(&e)
		_, err := localsession.DecodeReady(encode(e), expected.SessionNonce, expected.BindingID, expected.ContractDigest, true)
		if err == nil || err.Error() != "session_ready_invalid" {
			t.Fatal("invalid ready accepted or raw error leaked")
		}
	}
	for _, frame := range [][]byte{append(append([]byte(nil), valid...), []byte(" {}")...), []byte(strings.Repeat("x", localsession.MaxFrameBytes+1)), []byte(`{"type":"ready",` + string(valid[1:])), []byte(strings.Replace(string(valid), `"type":"ready"`, `"type":"ready","bearer":"secret"`, 1)), []byte(strings.Replace(string(valid), `"type":"ready"`, `"type":"ready","Type":"ready"`, 1)), []byte(strings.Replace(string(valid), `"type":"ready"`, `"type":"ready","message":null`, 1))} {
		if _, err := localsession.DecodeReady(frame, expected.SessionNonce, expected.BindingID, expected.ContractDigest, true); err == nil {
			t.Fatal("ambiguous or oversized frame accepted")
		}
	}
}
