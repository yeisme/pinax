package pinaxclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

func TestReaderSessionRecoveryClientRealHTTP(t *testing.T) {
	root := t.TempDir()
	ledger, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	req := operation.CreateRequest{OperationID: "op-sdk", IdempotencyKey: "key-sdk", CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", PrincipalDigest: operation.IdentityDigest("old-session"), ScopeDigest: operation.IdentityDigest("old-scope"), RequestDigest: operation.IdentityDigest("request")}
	if _, err := ledger.CreateAccepted(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("b", 64)
	if err := sessions.Create(api.ReaderSession{SessionID: "new-sdk-session", Nonce: strings.Repeat("n", 32), BindingID: "binding", Scopes: []string{"reader.metadata"}}, token); err != nil {
		t.Fatal(err)
	}
	binding, err := sessions.PrepareRecovery("new-sdk-session", root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewServerWithOptions(app.NewService(), root, api.ServerOptions{ReaderSessions: sessions}).Handler())
	defer server.Close()
	client := writerClient(t, server.URL, token)
	handle := ReaderRecoveryHandle{OperationID: req.OperationID, CapabilityID: req.CapabilityID, BindingID: req.BindingID, RequestDigest: req.RequestDigest}
	if _, err := client.ReaderRecoveryLookup(t.Context(), handle); err == nil {
		t.Fatal("unapproved read succeeded")
	}
	now := time.Now().UTC()
	grant := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: binding.SessionNonce, BindingID: binding.BindingID, VaultIdentityDigest: binding.VaultIdentityDigest, OperationID: req.OperationID, CapabilityID: req.CapabilityID, OperationBindingID: req.BindingID, PrincipalDigest: req.PrincipalDigest, ScopeDigest: req.ScopeDigest, RequestDigest: req.RequestDigest, Action: "lookup", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}
	raw, _ := json.Marshal(grant)
	if err := sessions.AuthorizeRecovery("new-sdk-session", raw); err != nil {
		t.Fatal(err)
	}
	view, err := client.ReaderRecoveryLookup(t.Context(), handle)
	if err != nil || view.Status != "accepted" || view.OperationID != req.OperationID {
		t.Fatal("original accepted row unavailable", err)
	}
	ledger, err = operation.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.StartApplying(t.Context(), req.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	view, err = client.ReaderRecoveryLookup(t.Context(), handle)
	if err != nil || view.Status != "applying" || !view.ReconcileRequired {
		t.Fatal("applying fact lost", err)
	}
	ledger, err = operation.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CompleteSucceeded(t.Context(), req.OperationID, operation.Outcome{}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	view, err = client.ReaderRecoveryLookup(t.Context(), handle)
	if err != nil || view.Status != "succeeded" {
		t.Fatal("original terminal fact unavailable", err)
	}
	sessions.Revoke("new-sdk-session")
	if _, err := client.ReaderRecoveryLookup(t.Context(), handle); err == nil {
		t.Fatal("revoked lookup accepted")
	}
}

func recoveryClientFixture() (Operation, ReaderRecoveryHandle) {
	digest := "sha256:" + strings.Repeat("a", 64)
	stamp := "2026-10-06T00:00:00Z"
	v := Operation{SchemaVersion: OperationSchemaV1, OperationID: "op-original", CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", RequestDigest: digest, Status: "accepted", CreatedAt: stamp, UpdatedAt: stamp, AcceptedAt: stamp}
	return v, ReaderRecoveryHandle{OperationID: v.OperationID, CapabilityID: v.CapabilityID, BindingID: v.BindingID, RequestDigest: digest}
}

func TestReaderSessionRecoveryClientRejectsUnsafeEvidence(t *testing.T) {
	modes := []string{"valid", "applying", "reconcile_required", "succeeded", "failed", "wrong-operation", "wrong-capability", "wrong-binding", "wrong-digest", "schema", "unknown", "timestamp", "flags", "action", "completion", "idempotency", "extra", "command", "result-budget", "receipt-budget", "wire", "smaller-limit", "redirect", "unavailable"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			view, handle := recoveryClientFixture()
			command := "reader.recovery.lookup"
			switch mode {
			case "applying", "reconcile_required":
				view.Status = mode
				view.ApplyingAt = view.AcceptedAt
				view.ReconcileRequired = true
				view.RequiredAction = "inspect_and_reconcile"
			case "succeeded", "failed":
				view.Status = mode
				view.ApplyingAt = view.AcceptedAt
				view.CompletedAt = view.AcceptedAt
			case "wrong-operation":
				view.OperationID = "foreign"
			case "wrong-capability":
				view.CapabilityID = "writer.apply"
			case "wrong-binding":
				view.BindingID = "rest.writer.apply"
			case "wrong-digest":
				view.RequestDigest = "sha256:" + strings.Repeat("b", 64)
			case "schema":
				view.SchemaVersion = "future"
			case "unknown":
				view.Status = "unknown"
			case "timestamp":
				view.AcceptedAt = "today"
			case "flags":
				view.ReconcileRequired = true
			case "action":
				view.RequiredAction = "retry"
			case "completion":
				view.CompletedAt = view.AcceptedAt
			case "idempotency":
				view.IdempotencyKey = "old-key"
			case "command":
				command = "operation.show"
			case "result-budget":
				view.Result = json.RawMessage(`"` + strings.Repeat("x", 64*1024) + `"`)
			case "receipt-budget":
				view.Receipt = json.RawMessage(`"` + strings.Repeat("x", 16*1024) + `"`)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/v1/reader/recovery/operation" || len(r.URL.Query()) != 1 || r.URL.Query().Get("operation_id") != handle.OperationID {
					t.Error("SDK sent non-minimal query")
				}
				if mode == "redirect" {
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(302)
					return
				}
				if mode == "unavailable" {
					w.WriteHeader(503)
					return
				}
				if mode == "wire" {
					_, _ = w.Write([]byte(strings.Repeat("x", 128*1024+1)))
					return
				}
				encoded, _ := json.Marshal(view)
				var data map[string]any
				_ = json.Unmarshal(encoded, &data)
				if mode == "extra" {
					data["principal_digest"] = "private"
				}
				p := domain.NewProjection(command, "Original evidence")
				p.Data = data
				_ = json.NewEncoder(w).Encode(p)
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "token")
			if mode == "smaller-limit" {
				client.maxResponseBytes = 64
			}
			got, err := client.ReaderRecoveryLookup(t.Context(), handle)
			valid := mode == "valid" || mode == "applying" || mode == "reconcile_required" || mode == "succeeded" || mode == "failed"
			if valid {
				if err != nil || got.Status != view.Status {
					t.Fatal("valid state rejected", err)
				}
			} else if err == nil {
				t.Fatal("unsafe evidence accepted")
			}
			if calls != 1 {
				t.Fatal("automatic retry or redirect")
			}
			for _, mutate := range []func(*ReaderRecoveryHandle){func(h *ReaderRecoveryHandle) { h.OperationID = "../op" }, func(h *ReaderRecoveryHandle) { h.RequestDigest = "" }, func(h *ReaderRecoveryHandle) { h.CapabilityID = "writer.tags.item" }, func(h *ReaderRecoveryHandle) { h.BindingID = "rest.other" }} {
				bad := handle
				mutate(&bad)
				if _, err := client.ReaderRecoveryLookup(t.Context(), bad); err == nil {
					t.Fatal("invalid handle accepted")
				}
			}
			if calls != 1 {
				t.Fatal("invalid handle reached network")
			}
		})
	}
}
