package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

// RecoverySession is an in-memory owner object for a NEW authenticated session.
// Its constructor and AuthorizeLookup are trusted-host-only integration points,
// not HTTP request handlers. No renderer/request field proves host approval.
// It cannot apply, reconcile, change ledger principals or read note bodies.
type RecoverySession struct {
	mu        sync.Mutex
	sessionID string
	root      string
	binding   localsession.RecoveryLookupContext
	files     map[string]os.FileInfo
	grants    map[string]localsession.RecoveryLookupGrant
	revoked   bool
	now       func() time.Time
}

func recoveryDenied() error { return errors.New("recovery_lookup_denied") }

// NewRecoverySession pins every ancestor, owner directory and ledger file.
// The random vault binding identifies this observed filesystem selection, not
// a pathname hash or a persistent vault ID. A fresh process needs a fresh grant.
// The caller must obtain sessionID/nonce/binding from trusted session state.
func NewRecoverySession(root, sessionID, nonce, binding string) (*RecoverySession, error) {
	if root == "" || !filepath.IsAbs(root) || sessionID == "" || len(sessionID) > 256 {
		return nil, recoveryDenied()
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, recoveryDenied()
	}
	root = filepath.Clean(root)
	paths := []string{}
	for p := root; ; p = filepath.Dir(p) {
		paths = append(paths, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	paths = append(paths, filepath.Join(root, ".pinax"), filepath.Join(root, ".pinax/api"), filepath.Join(root, ".pinax/api/operations.sqlite"))
	files := map[string]os.FileInfo{}
	for i, p := range paths {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (i == len(paths)-1 && !info.Mode().IsRegular()) || (i != len(paths)-1 && !info.IsDir()) {
			return nil, recoveryDenied()
		}
		files[p] = info
	}
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, recoveryDenied()
	}
	state := &RecoverySession{sessionID: sessionID, root: root, binding: localsession.RecoveryLookupContext{SessionNonce: nonce, BindingID: binding, VaultIdentityDigest: "sha256:" + hex.EncodeToString(entropy)}, files: files, grants: map[string]localsession.RecoveryLookupGrant{}, now: time.Now}
	// Reuse the contract's identity vocabulary without issuing a grant.
	probe := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: nonce, BindingID: binding, VaultIdentityDigest: state.binding.VaultIdentityDigest, OperationID: "probe", CapabilityID: "writer.apply", OperationBindingID: "rest.writer.apply", PrincipalDigest: state.binding.VaultIdentityDigest, ScopeDigest: state.binding.VaultIdentityDigest, RequestDigest: state.binding.VaultIdentityDigest, Action: "lookup"}
	now := state.now().UTC()
	probe.IssuedAt = now.Format(time.RFC3339Nano)
	probe.ExpiresAt = now.Add(time.Minute).Format(time.RFC3339Nano)
	if !probe.ValidFor(state.binding, now) {
		return nil, recoveryDenied()
	}
	return state, nil
}

// LookupContext is private host metadata; never forward it to a renderer or
// treat it as permission. AuthorizeLookup still requires explicit host approval.
func (s *RecoverySession) LookupContext() localsession.RecoveryLookupContext {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked || s.now == nil {
		return localsession.RecoveryLookupContext{}
	}
	return s.binding
}

func (s *RecoverySession) current() bool {
	if s.revoked || s.now == nil || len(s.files) == 0 {
		return false
	}
	for path, before := range s.files {
		after, err := os.Lstat(path)
		if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
			s.revoked = true
			clear(s.grants)
			return false
		}
	}
	return true
}

// AuthorizeLookup installs one explicit, independently approved private host
// instruction. NEVER expose this method as an HTTP/body-driven permission API.
// A grant cannot be replaced/extended in place, and a session holds at most 32.
func (s *RecoverySession) AuthorizeLookup(frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current() {
		return recoveryDenied()
	}
	g, err := localsession.DecodeRecoveryLookupGrant(frame, s.binding, s.now())
	if err != nil {
		return recoveryDenied()
	}
	if _, exists := s.grants[g.OperationID]; exists || len(s.grants) >= 32 {
		return recoveryDenied()
	}
	s.grants[g.OperationID] = g
	return nil
}

// Revoke serializes against lookups and discards every grant. It is permanent;
// reauthorization requires a new session and new explicit host instructions.
func (s *RecoverySession) Revoke() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked = true
	clear(s.grants)
}

// Lookup accepts ONLY the current authenticated session identity and operation
// ID. The old principal/scope come from the private installed grant, never the
// caller. A receipt is historical evidence, not current source authorization.
func (s *RecoverySession) Lookup(ctx context.Context, sessionID, operationID string) (operation.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := operation.View{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if sessionID != s.sessionID || !s.current() {
		return empty, recoveryDenied()
	}
	g, ok := s.grants[operationID]
	if !ok {
		return empty, recoveryDenied()
	}
	if !g.ValidFor(s.binding, s.now()) {
		s.revoked = true
		clear(s.grants)
		return empty, recoveryDenied()
	}
	store, cleanup, err := operation.OpenReadSnapshot(s.root)
	if err != nil {
		return empty, err
	}
	defer cleanup()
	row, err := store.GetAuthorized(ctx, g.OperationID, g.PrincipalDigest, g.ScopeDigest)
	if err != nil {
		return empty, err
	}
	if row.OperationID != g.OperationID || row.CapabilityID != g.CapabilityID || row.BindingID != g.OperationBindingID || row.RequestDigest != g.RequestDigest {
		return empty, recoveryDenied()
	}
	if !s.current() || !g.ValidFor(s.binding, s.now()) {
		s.revoked = true
		clear(s.grants)
		return empty, recoveryDenied()
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return operation.ViewFromRow(row), nil
}
