package store

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/credentialctl/pkg/credentials"
)

// Service binds a backend, the registry and the allowlist, implementing the
// operations behind the CLI and the credentials.Resolver contract for
// consumers. It never logs or returns secrets except through Resolve, and only
// to a caller whose (consumer, capability) is allowlisted.
type Service struct {
	backend   Backend
	registry  *RegistryStore
	allowlist credentials.Allowlist
	now       func() time.Time
	mu        sync.RWMutex
}

// NewService builds a service over the given backend, registry and allowlist.
func NewService(backend Backend, registry *RegistryStore, allowlist credentials.Allowlist) *Service {
	return &Service{backend: backend, registry: registry, allowlist: allowlist, now: time.Now}
}

// Backend exposes the active backend for privileged local operations
// (currently doctor --probe, which must read the secret to validate it over the
// network). It is never used to log or echo secrets.
func (s *Service) Backend() Backend { return s.backend }

// SetWithNow is like Set but allows injecting a clock (tests).
func (s *Service) SetWithNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Set stores a new credential and records a redacted registry entry. The secret
// goes to the backend; the entry stores only a redacted digest. On a registry
// write failure the orphaned secret is best-effort removed.
func (s *Service) Set(ref credentials.Ref, secret []byte, consumers, capabilities []string) (credentials.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ref.Validate(); err != nil {
		return credentials.Entry{}, err
	}
	if len(secret) == 0 {
		return credentials.Entry{}, errors.New("empty secret")
	}
	if err := s.backend.Put(ref.Key(), secret); err != nil {
		return credentials.Entry{}, err
	}
	entry := s.buildEntry(ref, secret, consumers, capabilities)
	if err := s.putEntry(entry); err != nil {
		_ = s.backend.Delete(ref.Key())
		return credentials.Entry{}, err
	}
	return entry, nil
}

// Rotate replaces the secret with a new revision; the previous secret bytes are
// overwritten so the old revision is no longer readable. The allowlist is
// preserved from the prior entry.
func (s *Service) Rotate(ref credentials.Ref, secret []byte) (credentials.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(secret) == 0 {
		return credentials.Entry{}, errors.New("empty secret")
	}
	entries, err := s.registry.Load()
	if err != nil {
		return credentials.Entry{}, err
	}
	prev, ok := entries[ref.Key()]
	if !ok {
		return credentials.Entry{}, credentials.ErrNotFound
	}
	if err := s.backend.Put(ref.Key(), secret); err != nil {
		return credentials.Entry{}, err
	}
	entry := s.buildEntry(ref, secret, prev.AllowedConsumers, prev.AllowedCapabilities)
	if err := s.putEntry(entry); err != nil {
		return credentials.Entry{}, err
	}
	return entry, nil
}

// Remove deletes the secret and its registry entry.
func (s *Service) Remove(ref credentials.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.backend.Delete(ref.Key()); err != nil {
		return err
	}
	entries, err := s.registry.Load()
	if err != nil {
		return err
	}
	delete(entries, ref.Key())
	return s.registry.Save(entries)
}

// Entry returns the redacted registry entry for ref, if present.
func (s *Service) Entry(ref credentials.Ref) (credentials.Entry, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entryUnlocked(ref)
}

func (s *Service) entryUnlocked(ref credentials.Ref) (credentials.Entry, bool, error) {
	entries, err := s.registry.Load()
	if err != nil {
		return credentials.Entry{}, false, err
	}
	e, ok := entries[ref.Key()]
	return e, ok, nil
}

// List returns all redacted registry entries, ordered by ref key.
func (s *Service) List() ([]credentials.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := s.registry.Load()
	if err != nil {
		return nil, err
	}
	out := make([]credentials.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.Key() < out[j].Ref.Key() })
	return out, nil
}

// Resolve implements credentials.Resolver. It enforces the consumer/capability
// allowlist and only returns a secret for an allowed pairing that exists
// locally.
func (s *Service) Resolve(consumer, capability string, ref credentials.Ref) (credentials.Resolution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok, err := s.entryUnlocked(ref)
	if err != nil {
		return credentials.Resolution{}, err
	}
	if !ok {
		return credentials.Resolution{}, credentials.ErrNotFound
	}
	return s.resolveWithGrantUnlocked(credentials.CredentialUseGrant{
		Ref: ref, Consumer: consumer, Capability: capability, Owner: credentials.OwnerCredentialctl,
		Operation: "resolve", Profile: ref.Account, Revision: entry.Revision,
		Project: consumer, User: "current-os-user", Audience: consumer,
		ExpiresAt: s.now().UTC().Add(time.Minute),
	})
}

// ResolveWithGrant resolves one credential under an explicit experimental
// scoped policy claim. V0.1 trusts the current OS user; these strings are not
// process authentication and the shared broker is therefore always denied.
func (s *Service) ResolveWithGrant(grant credentials.CredentialUseGrant) (credentials.Resolution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resolveWithGrantUnlocked(grant)
}

func (s *Service) resolveWithGrantUnlocked(grant credentials.CredentialUseGrant) (credentials.Resolution, error) {
	if err := grant.Ref.Validate(); err != nil ||
		grant.Owner != credentials.OwnerCredentialctl ||
		strings.TrimSpace(grant.Consumer) == "" || strings.TrimSpace(grant.Capability) == "" ||
		grant.Operation != "resolve" ||
		strings.TrimSpace(grant.Project) == "" || strings.TrimSpace(grant.User) == "" ||
		grant.Audience != grant.Consumer || strings.TrimSpace(grant.Revision) == "" ||
		grant.ExpiresAt.IsZero() || !grant.ExpiresAt.After(s.now().UTC()) ||
		isBrokerClaim(grant.Consumer) || isBrokerClaim(grant.Owner) || isBrokerClaim(grant.Audience) {
		return credentials.Resolution{}, credentials.ErrNotAllowed
	}
	entry, ok, err := s.entryUnlocked(grant.Ref)
	if err != nil {
		return credentials.Resolution{}, err
	}
	if !ok {
		return credentials.Resolution{}, credentials.ErrNotFound
	}
	if entry.Disabled || entry.Revision != grant.Revision || !s.allowlist.Allows(grant.Consumer, grant.Capability) ||
		!contains(entry.AllowedConsumers, grant.Consumer) || !contains(entry.AllowedCapabilities, grant.Capability) {
		return credentials.Resolution{}, credentials.ErrNotAllowed
	}
	secret, err := s.backend.Get(grant.Ref.Key())
	if err != nil {
		return credentials.Resolution{}, err
	}
	return credentials.Resolution{
		Ref:      grant.Ref,
		Backend:  s.backend.Name(),
		Revision: entry.Revision,
		Digest:   credentials.DigestSecret(secret),
		Secret:   secret,
	}, nil
}

// SetEnabled changes only credentialctl's registry entry. It never migrates or
// modifies another project's private credential store.
func (s *Service) SetEnabled(ref credentials.Ref, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.registry.Load()
	if err != nil {
		return err
	}
	entry, ok := entries[ref.Key()]
	if !ok {
		return credentials.ErrNotFound
	}
	entry.Disabled = !enabled
	entry.UpdatedAt = s.now().UTC()
	entries[ref.Key()] = entry
	return s.registry.Save(entries)
}

// Readiness creates a secret-free, network-free local projection.
func (s *Service) Readiness(ref credentials.Ref) credentials.Readiness {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := credentials.Readiness{
		SpecVersion: credentials.ReadinessSpecVersion,
		Ref:         ref, State: credentials.ReadinessMissing, Source: "local_store",
		Capabilities: []string{}, Owner: credentials.OwnerCredentialctl,
	}
	if err := ref.Validate(); err != nil {
		r.State = credentials.ReadinessCorrupt
		return r
	}
	entry, ok, err := s.entryUnlocked(ref)
	if err != nil {
		if errors.Is(err, credentials.ErrRegistryCorrupt) {
			r.State = credentials.ReadinessCorrupt
		} else {
			r.State = credentials.ReadinessUnavailable
		}
		return r
	}
	if !ok {
		r.Action = &credentials.RemediationAction{
			Name:    "configure",
			Command: "credentialctl set " + ref.Key() + " --preset local-ai --json",
		}
		return r
	}
	r.Backend = entry.Backend
	r.Revision = entry.Revision
	r.RedactedDigest = entry.RedactedDigest
	r.Capabilities = append([]string(nil), entry.AllowedCapabilities...)
	sort.Strings(r.Capabilities)
	if entry.Disabled {
		r.State = credentials.ReadinessDisabled
		r.Action = &credentials.RemediationAction{Name: "enable", Command: "credentialctl enable " + ref.Key() + " --json"}
		return r
	}
	secret, err := s.backend.Get(ref.Key())
	if err != nil {
		r.State = credentials.ReadinessUnavailable
		return r
	}
	// 本地状态检查只确认 backend 中的值存在；立即清零临时副本，绝不输出或联网。
	for i := range secret {
		secret[i] = 0
	}
	r.Configured = true
	r.State = credentials.ReadinessConfigured
	return r
}

func isBrokerClaim(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "broker" || strings.Contains(s, "runtime-broker") || strings.Contains(s, "client-runtime")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *Service) buildEntry(ref credentials.Ref, secret []byte, consumers, capabilities []string) credentials.Entry {
	return credentials.Entry{
		Ref:                 ref,
		Provider:            ref.Provider,
		Account:             ref.Account,
		Backend:             s.backend.Name(),
		Revision:            s.newRevision(),
		AllowedConsumers:    dedup(consumers),
		AllowedCapabilities: dedup(capabilities),
		UpdatedAt:           s.now().UTC(),
		RedactedDigest:      credentials.DigestSecret(secret),
	}
}

func (s *Service) newRevision() string {
	// nanosecond timestamp keeps rotations distinct even within one second
	return "rev-" + strconv.FormatInt(s.now().UTC().UnixNano(), 36)
}

func (s *Service) putEntry(entry credentials.Entry) error {
	entries, err := s.registry.Load()
	if err != nil {
		return err
	}
	entries[entry.Ref.Key()] = entry
	return s.registry.Save(entries)
}

func dedup(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
