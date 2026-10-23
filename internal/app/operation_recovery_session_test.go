package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

func recoverySessionFixture(t *testing.T) (string, *RecoverySession, localsession.RecoveryLookupGrant, operation.OperationRow) {
	t.Helper()
	root := t.TempDir()
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.CreateAccepted(t.Context(), operation.CreateRequest{OperationID: "op-recover", IdempotencyKey: "idem-recover", CapabilityID: "writer.daily.edit", BindingID: "rest.writer.daily.edit", PrincipalDigest: operation.IdentityDigest("old-session"), ScopeDigest: operation.IdentityDigest("old-scope"), RequestDigest: operation.IdentityDigest("original-request")})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := NewRecoverySession(root, "new-session", strings.Repeat("n", 32), "binding-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 6, 16, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	c := s.LookupContext()
	row := result.Operation
	g := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: c.SessionNonce, BindingID: c.BindingID, VaultIdentityDigest: c.VaultIdentityDigest, OperationID: row.OperationID, CapabilityID: row.CapabilityID, OperationBindingID: row.BindingID, PrincipalDigest: row.PrincipalDigest, ScopeDigest: row.ScopeDigest, RequestDigest: row.RequestDigest, Action: "lookup", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}
	return root, s, g, row
}
func recoveryFrame(t *testing.T, g localsession.RecoveryLookupGrant) []byte {
	t.Helper()
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOperationShowRecoverySessionOriginalIdentity(t *testing.T) {
	root, s, g, row := recoverySessionFixture(t)
	before := tagPlanTree(t, root)
	if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("unapproved lookup")
	}
	if err := s.AuthorizeLookup(recoveryFrame(t, g)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Lookup(t.Context(), "new-session", g.OperationID)
	if err != nil || !reflect.DeepEqual(got, operation.ViewFromRow(row)) {
		t.Fatal("not original receipt", err)
	}
	for _, id := range []string{"old-session", "foreign-session", ""} {
		if _, err := s.Lookup(t.Context(), id, g.OperationID); err == nil {
			t.Fatal("foreign session")
		}
	}
	if _, err := s.Lookup(t.Context(), "new-session", "other-operation"); err == nil {
		t.Fatal("grant widened")
	}
	if err := s.AuthorizeLookup(recoveryFrame(t, g)); err == nil {
		t.Fatal("grant replaced")
	}
	// A fresh ordinary session still cannot claim the old principal through show.
	access := WriterOperationAccess("new-session", g.BindingID, []string{WriterScopeEditor})
	if _, err := NewService().OperationShow(t.Context(), OperationRequest{VaultPath: root, OperationID: g.OperationID, Access: access.Access}); err == nil {
		t.Fatal("legacy scope isolation changed")
	}
	if !reflect.DeepEqual(before, tagPlanTree(t, root)) {
		t.Fatal("lookup changed source")
	}
	s.Revoke()
	if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("revoked session")
	}
	if err := s.AuthorizeLookup(recoveryFrame(t, g)); err == nil {
		t.Fatal("revoked reauthorization")
	}
	if s.LookupContext() != (localsession.RecoveryLookupContext{}) {
		t.Fatal("revoked context retained")
	}
}

func TestOperationShowRecoverySessionRejectsWrongRow(t *testing.T) {
	for _, field := range []string{"principal", "scope", "request", "capability", "operation"} {
		t.Run(field, func(t *testing.T) {
			root, s, g, _ := recoverySessionFixture(t)
			before := tagPlanTree(t, root)
			switch field {
			case "principal":
				g.PrincipalDigest = operation.IdentityDigest("other")
			case "scope":
				g.ScopeDigest = operation.IdentityDigest("other")
			case "request":
				g.RequestDigest = operation.IdentityDigest("other")
			case "capability":
				g.CapabilityID = "writer.apply"
				g.OperationBindingID = "rest.writer.apply"
			case "operation":
				g.OperationID = "op-other"
			}
			if err := s.AuthorizeLookup(recoveryFrame(t, g)); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
				t.Fatal("wrong original identity disclosed")
			}
			if !reflect.DeepEqual(before, tagPlanTree(t, root)) {
				t.Fatal("denied lookup wrote source")
			}
		})
	}
}

func TestOperationShowRecoverySessionExpiryAndCancellation(t *testing.T) {
	_, s, g, _ := recoverySessionFixture(t)
	if err := s.AuthorizeLookup(recoveryFrame(t, g)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Lookup(ctx, "new-session", g.OperationID); err != context.Canceled {
		t.Fatal("cancel ignored")
	}
	expiry, _ := time.Parse(time.RFC3339Nano, g.ExpiresAt)
	s.now = func() time.Time { return expiry }
	if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("expired grant")
	}
	s.now = func() time.Time { return expiry.Add(-time.Minute) }
	if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
		t.Fatal("expired grant revived after clock rollback")
	}
	if err := s.AuthorizeLookup(recoveryFrame(t, g)); err == nil {
		t.Fatal("expired session reauthorized")
	}
}

func TestOperationShowRecoverySessionRootBinding(t *testing.T) {
	for _, kind := range []string{"root", "ledger", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root, s, g, _ := recoverySessionFixture(t)
			if err := s.AuthorizeLookup(recoveryFrame(t, g)); err != nil {
				t.Fatal(err)
			}
			target := root
			if kind == "ledger" {
				target = filepath.Join(root, ".pinax/api/operations.sqlite")
			}
			if kind == "directory" {
				target = filepath.Join(root, ".pinax/api")
			}
			backup := target + "-old"
			if err := os.Rename(target, backup); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := os.Lstat(backup); err == nil {
					_ = os.RemoveAll(target)
					_ = os.Rename(backup, target)
				}
			})
			if kind == "ledger" {
				b, err := os.ReadFile(backup)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, b, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
				t.Fatal("filesystem identity replaced")
			}
			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(backup, target); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Lookup(t.Context(), "new-session", g.OperationID); err == nil {
				t.Fatal("restored path revived invalidated grant")
			}
		})
	}
	root, s, g, _ := recoverySessionFixture(t)
	another, err := NewRecoverySession(root, "new-session", g.SessionNonce, g.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	another.now = s.now
	if another.LookupContext().VaultIdentityDigest == g.VaultIdentityDigest {
		t.Fatal("binding reused")
	}
	if err := another.AuthorizeLookup(recoveryFrame(t, g)); err == nil {
		t.Fatal("grant crossed owner observation")
	}
	_, foreign, _, _ := recoverySessionFixture(t)
	foreign.now = s.now
	if err := foreign.AuthorizeLookup(recoveryFrame(t, g)); err == nil {
		t.Fatal("foreign vault grant")
	}
	link := filepath.Join(t.TempDir(), "linked-vault")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlink unsupported")
	}
	if _, err := NewRecoverySession(link, "new-session", g.SessionNonce, g.BindingID); err == nil {
		t.Fatal("root symlink followed")
	}
}

func TestOperationShowRecoverySessionZeroValue(t *testing.T) {
	s := &RecoverySession{}
	if err := s.AuthorizeLookup([]byte("{}")); err == nil {
		t.Fatal("zero value authorized")
	}
	if _, err := s.Lookup(t.Context(), "", "op"); err == nil {
		t.Fatal("zero value disclosed")
	}
	s.Revoke()
}
