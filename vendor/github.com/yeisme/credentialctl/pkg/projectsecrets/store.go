package projectsecrets

import (
	"github.com/yeisme/credentialctl/internal/projectapp"
	"github.com/yeisme/credentialctl/internal/projectstore"
)

// StoreResult is the redacted outcome of a Store mutation. It carries only
// ciphertext-safe metadata: project/repository identity, entry count and the
// envelope digest. It never includes plaintext, ciphertext values or salts.
type StoreResult struct {
	Project    string
	Repository string
	Provider   string
	EntryCount int
	Digest     string
	Entries    []EntryMetadata
}

func newStoreResult(env *projectstore.Envelope, entries []projectapp.EntryMetadata) StoreResult {
	digest, _ := env.Digest()
	return StoreResult{
		Project:    env.Project,
		Repository: env.Repository,
		Provider:   env.Provider,
		EntryCount: len(env.Entries),
		Digest:     digest,
		Entries:    entries,
	}
}

// Store is the public mutation facade over the envelope: it lets a Go domain
// owner (for example Pinax) initialize a repository envelope and add, replace,
// remove or rekey entries without copying crypto or going through the CLI.
type Store struct {
	svc *projectapp.Service
}

// NewStore returns the default mutation store.
func NewStore() *Store { return &Store{svc: projectapp.New()} }

// Init creates a new envelope at repoRoot/relPath with the given project and
// repository identity. provider defaults to passphrase-v1 when empty.
func (s *Store) Init(repoRoot, relPath, project, repository, provider string, passphrase []byte) (StoreResult, error) {
	_, env, err := s.svc.Init(repoRoot, relPath, project, repository, provider, passphrase)
	if err != nil {
		return StoreResult{}, err
	}
	return newStoreResult(env, nil), nil
}

// SetEntry adds or replaces an entry with a typed domain classification. The
// classification participates in the AAD binding so a ciphertext cannot be
// replayed against a different declared format.
func (s *Store) SetEntry(repoRoot, relPath, name, kind, format, version string, plaintext, passphrase []byte) (StoreResult, error) {
	env, err := s.svc.SetEntryTyped(repoRoot, relPath, name, kind, format, version, plaintext, passphrase)
	if err != nil {
		return StoreResult{}, err
	}
	return newStoreResult(env, nil), nil
}

// RemoveEntry deletes one entry (no-op if absent). Passphrase authorizes the
// mutation.
func (s *Store) RemoveEntry(repoRoot, relPath, name string, passphrase []byte) (StoreResult, error) {
	env, err := s.svc.RemoveEntry(repoRoot, relPath, name, passphrase)
	if err != nil {
		return StoreResult{}, err
	}
	return newStoreResult(env, nil), nil
}

// Rekey rewraps the existing DEK with a new passphrase; entry plaintexts and
// identities are unchanged.
func (s *Store) Rekey(repoRoot, relPath string, oldPassphrase, newPassphrase []byte) (StoreResult, error) {
	env, err := s.svc.Rekey(repoRoot, relPath, oldPassphrase, newPassphrase)
	if err != nil {
		return StoreResult{}, err
	}
	return newStoreResult(env, nil), nil
}

// List returns redacted entry metadata sorted by name.
func (s *Store) List(repoRoot, relPath string) (StoreResult, error) {
	entries, env, err := s.svc.List(repoRoot, relPath)
	if err != nil {
		return StoreResult{}, err
	}
	return newStoreResult(env, entries), nil
}

// Verify confirms the passphrase unwraps the envelope DEK.
func (s *Store) Verify(repoRoot, relPath string, passphrase []byte) error {
	return s.svc.Verify(repoRoot, relPath, passphrase)
}

// EntryMetadata is re-exported so consumers do not need the internal package.
type EntryMetadata = projectapp.EntryMetadata
