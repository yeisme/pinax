package api

import (
	"context"
	"errors"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/pkg/localsession"
)

func recoveryRegistryDenied() error { return errors.New("recovery_lookup_denied") }

// liveRecoverySessionLocked is called with the registry write lock held.
func (s *ReaderSessionStore) liveRecoverySessionLocked(id string) *ReaderSession {
	digest, ok := s.byID[id]
	if !ok {
		return nil
	}
	session := s.sessions[digest]
	if session == nil || session.SessionID != id {
		return nil
	}
	if session.Expired() {
		s.revokeLocked(id)
		return nil
	}
	return session
}

// PrepareRecovery is a TRUSTED HOST integration point, not an HTTP action.
// The root comes from the independently approved vault selection, not a query
// or renderer path. A session gets only one owner filesystem observation.
func (s *ReaderSessionStore) PrepareRecovery(sessionID, root string) (localsession.RecoveryLookupContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.liveRecoverySessionLocked(sessionID)
	if session == nil || s.recoveries[sessionID] != nil {
		return localsession.RecoveryLookupContext{}, recoveryRegistryDenied()
	}
	recovery, err := app.NewRecoverySession(root, session.SessionID, session.Nonce, session.BindingID)
	if err != nil {
		return localsession.RecoveryLookupContext{}, err
	}
	if session.Expired() {
		recovery.Revoke()
		s.revokeLocked(sessionID)
		return localsession.RecoveryLookupContext{}, recoveryRegistryDenied()
	}
	s.recoveries[sessionID] = recovery
	return recovery.LookupContext(), nil
}

// AuthorizeRecovery receives an explicitly approved private host frame. It
// must never be called with HTTP/renderer-supplied authority or old digests.
// The current bearer alone does not permit installation of recovery grants.
func (s *ReaderSessionStore) AuthorizeRecovery(sessionID string, frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.liveRecoverySessionLocked(sessionID)
	recovery := s.recoveries[sessionID]
	if session == nil || recovery == nil {
		return recoveryRegistryDenied()
	}
	return recovery.AuthorizeLookup(frame)
}

// LookupRecovery revalidates the actual current bearer, not a cached session
// snapshot. Lock order is registry -> owner recovery; neither owner nor its
// snapshot reader calls back into this registry. Revoke waits for an admitted
// lookup, and no new lookup can pass after Revoke returns.
func (s *ReaderSessionStore) LookupRecovery(ctx context.Context, bearer, operationID string) (operation.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := operation.View{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	session := s.sessions[readerBearerDigest(bearer)]
	if session == nil {
		return empty, recoveryRegistryDenied()
	}
	session = s.liveRecoverySessionLocked(session.SessionID)
	if session == nil {
		return empty, recoveryRegistryDenied()
	}
	recovery := s.recoveries[session.SessionID]
	if recovery == nil {
		return empty, recoveryRegistryDenied()
	}
	view, err := recovery.Lookup(ctx, session.SessionID, operationID)
	if session.Expired() {
		s.revokeLocked(session.SessionID)
		return empty, recoveryRegistryDenied()
	}
	return view, err
}
