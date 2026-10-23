package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestReaderSessionRecoveryHTTP(t *testing.T) {
	sessions, root, g := recoveryStoreFixture(t)
	before := snapshotFileTree(t, root)
	defer func() {
		if !reflect.DeepEqual(before, snapshotFileTree(t, root)) {
			t.Fatal("HTTP recovery wrote vault")
		}
	}()
	server := NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions, ExposeGroups: []string{"reader"}})
	handler := server.Handler()
	get := func(method, query, bearer, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1/v1/reader/recovery/operation"+query, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:32000"
		r.Header.Set("Authorization", "Bearer "+bearer)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := get("GET", "?operation_id="+g.OperationID, "new-bearer", ""); w.Code != 403 {
		t.Fatalf("unapproved status %d", w.Code)
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.AuthorizeRecovery("new-session", raw); err != nil {
		t.Fatal(err)
	}
	w := get("GET", "?operation_id="+g.OperationID, "new-bearer", "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("lookup status %d", w.Code)
	}
	var envelope struct {
		Command string         `json:"command"`
		Data    operation.View `json:"data"`
	}
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Command != "reader.recovery.lookup" || envelope.Data.Status != operation.StatusAccepted || envelope.Data.RequestDigest != g.RequestDigest {
		t.Fatal("not original accepted evidence")
	}
	encoded, _ := json.Marshal(envelope.Data)
	var data any
	if json.Unmarshal(encoded, &data) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.OperationV1, data) != nil {
		t.Fatal("response contract mismatch")
	}
	for _, secret := range []string{g.PrincipalDigest, g.ScopeDigest, g.VaultIdentityDigest, root, "new-bearer"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private identity leaked")
		}
	}
	for _, query := range []string{"", "?", "?operation_id=", "?operation_id=../bad", "?operation_id=op-old&operation_id=op-old", "?operation_id=op-old&principal_digest=forged", "?operation_id=%ff", "?operation_id=%zz", "?operation_id=" + strings.Repeat("x", 2049)} {
		if w := get("GET", query, "new-bearer", ""); w.Code != 400 {
			t.Fatalf("invalid query status %d", w.Code)
		}
	}
	if w := get("GET", "?operation_id=op-old", "new-bearer", "{}"); w.Code != 400 {
		t.Fatal("GET body accepted")
	}
	if w := get("POST", "?operation_id=op-old", "new-bearer", string(raw)); w.Code != 405 {
		t.Fatal("HTTP grant installation accepted")
	}
	if w := get("GET", "?operation_id=foreign", "new-bearer", ""); w.Code != 403 {
		t.Fatal("foreign receipt disclosed")
	}
	if w := get("GET", "?operation_id=op-old", "foreign-bearer", ""); w.Code != 401 {
		t.Fatal("foreign bearer accepted")
	}
	sessions.Revoke("new-session")
	if w := get(http.MethodGet, "?operation_id=op-old", "new-bearer", ""); w.Code != 401 {
		t.Fatal("revoked bearer accepted")
	}
}
