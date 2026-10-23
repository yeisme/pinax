package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

func TestReaderSessionStoreIdentityIsolation(t *testing.T) {
	store, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	original := ReaderSession{SessionID: "session-one", BindingID: "binding-one", Nonce: strings.Repeat("n", 32), Scopes: []string{"reader.metadata"}}
	if err := store.Create(original, "fixture-bearer"); err != nil {
		t.Fatal(err)
	}
	original.Scopes[0] = "writer.editor"
	verified, err := store.Verify("fixture-bearer")
	if err != nil {
		t.Fatal(err)
	}
	if verified.HasScope("writer.editor") {
		t.Fatal("caller mutated stored scopes")
	}
	verified.SessionID = "forged"
	verified.Scopes[0] = "writer.editor"
	again, err := store.Verify("fixture-bearer")
	if err != nil || again.SessionID != "session-one" || again.HasScope("writer.editor") {
		t.Fatal("verified snapshot mutated owner registry")
	}
	if err := store.Create(ReaderSession{SessionID: "session-two"}, "fixture-bearer"); err == nil {
		t.Fatal("bearer collision overwrote session")
	}
	store.Revoke("session-two")
	if _, err := store.Verify("fixture-bearer"); err != nil {
		t.Fatal("unregistered ID revoked original session")
	}
	store.Revoke("session-one")
	if _, err := store.Verify("fixture-bearer"); err == nil {
		t.Fatal("revoked bearer accepted")
	}
}

func recoveryStoreFixture(t *testing.T) (*ReaderSessionStore, string, localsession.RecoveryLookupGrant) {
	t.Helper()
	root := t.TempDir()
	ledger, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	old := operation.CreateRequest{OperationID: "op-old", IdempotencyKey: "key-old", CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", PrincipalDigest: operation.IdentityDigest("old-session"), ScopeDigest: operation.IdentityDigest("old-scope"), RequestDigest: operation.IdentityDigest("old-request")}
	if _, err := ledger.CreateAccepted(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ReaderSession{SessionID: "new-session", BindingID: "binding", Nonce: strings.Repeat("n", 32), Scopes: []string{"reader.metadata"}}, "new-bearer"); err != nil {
		t.Fatal(err)
	}
	binding, err := store.PrepareRecovery("new-session", root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	grant := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: binding.SessionNonce, BindingID: binding.BindingID, VaultIdentityDigest: binding.VaultIdentityDigest, OperationID: old.OperationID, CapabilityID: old.CapabilityID, OperationBindingID: old.BindingID, PrincipalDigest: old.PrincipalDigest, ScopeDigest: old.ScopeDigest, RequestDigest: old.RequestDigest, Action: "lookup", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}
	return store, root, grant
}

func TestReaderSessionRecoveryRegistryLifecycle(t *testing.T) {
	store, root, g := recoveryStoreFixture(t)
	before := snapshotFileTree(t, root)
	defer func() {
		if !reflect.DeepEqual(before, snapshotFileTree(t, root)) {
			t.Fatal("recovery lifecycle changed vault")
		}
	}()
	if _, err := store.LookupRecovery(t.Context(), "new-bearer", g.OperationID); err == nil {
		t.Fatal("no private approval")
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AuthorizeRecovery("new-session", raw); err != nil {
		t.Fatal(err)
	}
	view, err := store.LookupRecovery(t.Context(), "new-bearer", g.OperationID)
	if err != nil || view.OperationID != g.OperationID || view.RequestDigest != g.RequestDigest || view.Status != operation.StatusAccepted {
		t.Fatal("original receipt unavailable", err)
	}
	if _, err := store.LookupRecovery(t.Context(), "wrong-bearer", g.OperationID); err == nil {
		t.Fatal("foreign bearer")
	}
	if _, err := store.LookupRecovery(t.Context(), "new-bearer", "other"); err == nil {
		t.Fatal("grant widened")
	}
	if _, err := store.PrepareRecovery("new-session", root); err == nil {
		t.Fatal("binding replaced without new session")
	}
	if _, err := store.PrepareRecovery("missing", root); err == nil {
		t.Fatal("unregistered session prepared")
	}
	if err := store.AuthorizeRecovery("missing", raw); err == nil {
		t.Fatal("unregistered session authorized")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.LookupRecovery(ctx, "new-bearer", g.OperationID); err != context.Canceled {
		t.Fatal("cancel ignored")
	}
	// Keep the internal object to prove revocation reaches the owner grant store,
	// not just the bearer map. It is never exposed to a production consumer.
	recovery := store.recoveries["new-session"]
	store.Revoke("new-session")
	if _, err := store.LookupRecovery(t.Context(), "new-bearer", g.OperationID); err == nil {
		t.Fatal("revoked bearer")
	}
	if _, err := recovery.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("owner recovery survived registry revocation")
	}
	if len(store.recoveries) != 0 {
		t.Fatal("revoked recovery retained")
	}
	if err := store.Create(ReaderSession{SessionID: "new-session", BindingID: "binding", Nonce: g.SessionNonce}, "replacement-bearer"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LookupRecovery(t.Context(), "replacement-bearer", g.OperationID); err == nil {
		t.Fatal("reused ID inherited old grant")
	}
}

func TestReaderSessionRecoveryRegistryExpiry(t *testing.T) {
	store, _, g := recoveryStoreFixture(t)
	raw, _ := json.Marshal(g)
	if err := store.AuthorizeRecovery("new-session", raw); err != nil {
		t.Fatal(err)
	}
	recovery := store.recoveries["new-session"]
	store.mu.Lock()
	store.sessions[store.byID["new-session"]].ExpiresAt = time.Now().Add(-time.Second)
	store.mu.Unlock()
	if _, err := store.LookupRecovery(t.Context(), "new-bearer", g.OperationID); err == nil {
		t.Fatal("expired bearer recovery")
	}
	if _, err := recovery.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("expired owner grant remained active")
	}
	if err := store.AuthorizeRecovery("new-session", raw); err == nil {
		t.Fatal("expired session authorized")
	}
}

func TestReaderSessionRecoveryVerifyExpiryRevokesOwner(t *testing.T) {
	store, _, g := recoveryStoreFixture(t)
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AuthorizeRecovery("new-session", raw); err != nil {
		t.Fatal(err)
	}
	recovery := store.recoveries["new-session"]
	store.mu.Lock()
	store.sessions[store.byID["new-session"]].ExpiresAt = time.Now().Add(-time.Second)
	store.mu.Unlock()
	if _, err := store.Verify("new-bearer"); err == nil {
		t.Fatal("expired verify accepted")
	}
	if _, err := recovery.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("verify expiry left owner grants live")
	}
}
