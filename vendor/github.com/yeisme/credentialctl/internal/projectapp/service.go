// Package projectapp is the application service that ties the envelope store
// and crypto primitives into the mutation operations the CLI exposes: init,
// set/list/remove entry, rekey and unlock. It owns no domain semantics — entry
// payloads are opaque bytes provided by the caller.
package projectapp

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/yeisme/credentialctl/internal/projectcrypto"
	"github.com/yeisme/credentialctl/internal/projectstore"
)

// Service performs envelope mutations. It is stateless; each operation reloads
// the envelope from disk so concurrent CLI invocations see a consistent file.
type Service struct{}

// New returns the default service.
func New() *Service { return &Service{} }

// Init creates a new envelope at the resolved path. It generates a random DEK,
// wraps it with the passphrase, and writes only ciphertext + identity. The
// asset path is resolved under repoRoot with path containment.
func (Service) Init(repoRoot, relPath, project, repository, provider string, passphrase []byte) (string, *projectstore.Envelope, error) {
	if project == "" || repository == "" {
		return "", nil, fmt.Errorf("projectapp: project and repository required")
	}
	if len(passphrase) == 0 {
		return "", nil, fmt.Errorf("projectapp: passphrase required")
	}
	if provider == "" {
		provider = "passphrase-v1"
	}
	if provider != "passphrase-v1" {
		return "", nil, fmt.Errorf("projectapp: unsupported provider %q", provider)
	}
	path, err := projectstore.ResolveAssetPath(repoRoot, relPath)
	if err != nil {
		return "", nil, err
	}
	params := projectcrypto.MinimumParams()
	salt := projectcrypto.MustRandomSalt()
	binding := projectcrypto.Binding{
		Schema: projectstore.SchemaVersion, Project: project, Repository: repository,
	}
	wrapped, err := projectcrypto.WrapKey(passphrase, salt, params, binding)
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	env := &projectstore.Envelope{
		SchemaVersion: projectstore.SchemaVersion,
		Project:       project,
		Repository:    repository,
		Provider:      provider,
		KDF: projectstore.KDFProfile{
			Time: params.Time, Memory: params.Memory, Threads: params.Threads, KeyLen: params.KeyLen,
		},
		Salt:       hex.EncodeToString(salt),
		WrappedKey: encodeCiphertext(wrapped),
		Entries:    map[string]projectstore.EnvelopeEntry{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := projectstore.Save(path, env); err != nil {
		return "", nil, err
	}
	return path, env, nil
}

// SetEntry adds or replaces an entry. It unwraps the DEK, authenticates the
// new entry payload under its binding, re-wraps nothing (DEK unchanged) and
// saves. Replacing an entry with the same name rotates its identity so the
// caller can detect the change.
func (Service) SetEntry(repoRoot, relPath, name string, plaintext []byte, passphrase []byte) (*projectstore.Envelope, error) {
	path, env, err := load(repoRoot, relPath)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("projectapp: entry name required")
	}
	dek, _, err := unlockDEK(env, passphrase)
	if err != nil {
		return nil, err
	}
	defer projectcrypto.Wipe(dek)
	existing, replacing := env.Entries[name]
	kind, format, version := "", "", ""
	identity := shortIdentity(name, env)
	if replacing {
		// Preserve declared classification when updating in place.
		kind, format, version = existing.Kind, existing.Format, existing.Version
	}
	entryBinding := projectcrypto.Binding{
		Schema: env.SchemaVersion, Project: env.Project, Repository: env.Repository,
		EntryName: name, Identity: identity, Kind: kind, Format: format, Version: version,
	}
	ct, err := projectcrypto.EncryptEntry(dek, entryBinding, plaintext)
	if err != nil {
		return nil, err
	}
	env.Entries[name] = projectstore.EnvelopeEntry{
		Identity:   identity,
		Kind:       kind,
		Format:     format,
		Version:    version,
		Ciphertext: encodeCiphertext(ct),
	}
	env.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := projectstore.Save(path, env); err != nil {
		return nil, err
	}
	return env, nil
}

// SetEntryTyped is SetEntry with an explicit domain classification (kind,
// format, version). The classification participates in the AAD binding, so a
// ciphertext cannot be reused against a different declared format.
func (Service) SetEntryTyped(repoRoot, relPath, name, kind, format, version string, plaintext []byte, passphrase []byte) (*projectstore.Envelope, error) {
	path, env, err := load(repoRoot, relPath)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("projectapp: entry name required")
	}
	dek, _, err := unlockDEK(env, passphrase)
	if err != nil {
		return nil, err
	}
	defer projectcrypto.Wipe(dek)
	identity := shortIdentity(name, env)
	entryBinding := projectcrypto.Binding{
		Schema: env.SchemaVersion, Project: env.Project, Repository: env.Repository,
		EntryName: name, Identity: identity, Kind: kind, Format: format, Version: version,
	}
	ct, err := projectcrypto.EncryptEntry(dek, entryBinding, plaintext)
	if err != nil {
		return nil, err
	}
	env.Entries[name] = projectstore.EnvelopeEntry{
		Identity:   identity,
		Kind:       kind,
		Format:     format,
		Version:    version,
		Ciphertext: encodeCiphertext(ct),
	}
	env.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := projectstore.Save(path, env); err != nil {
		return nil, err
	}
	return env, nil
}

// RemoveEntry deletes an entry. Missing entry is a no-op success.
func (Service) RemoveEntry(repoRoot, relPath, name string, passphrase []byte) (*projectstore.Envelope, error) {
	path, env, err := load(repoRoot, relPath)
	if err != nil {
		return nil, err
	}
	// Unlock to authorize the mutation (fail closed without passphrase).
	if _, _, err := unlockDEK(env, passphrase); err != nil {
		return nil, err
	}
	if _, ok := env.Entries[name]; !ok {
		return env, nil
	}
	delete(env.Entries, name)
	env.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := projectstore.Save(path, env); err != nil {
		return nil, err
	}
	return env, nil
}

// Rekey rewraps the existing DEK with a new passphrase. Entry plaintexts and
// identities are unchanged because the DEK does not change. The previous asset
// remains usable until the atomic rename commits the new envelope.
func (Service) Rekey(repoRoot, relPath string, oldPassphrase, newPassphrase []byte) (*projectstore.Envelope, error) {
	path, env, err := load(repoRoot, relPath)
	if err != nil {
		return nil, err
	}
	if len(newPassphrase) == 0 {
		return nil, fmt.Errorf("projectapp: new passphrase required")
	}
	dek, binding, err := unlockDEK(env, passphraseBytes(oldPassphrase))
	if err != nil {
		return nil, err
	}
	defer projectcrypto.Wipe(dek)
	// Fresh salt + same params; rewrap the SAME DEK under the new passphrase.
	newSalt := projectcrypto.MustRandomSalt()
	params := projectcrypto.Params{
		Time: env.KDF.Time, Memory: env.KDF.Memory, Threads: env.KDF.Threads, KeyLen: env.KDF.KeyLen,
	}
	if !params.AtOrAbove(projectcrypto.MinimumParams()) {
		return nil, fmt.Errorf("projectapp: envelope kdf below minimum")
	}
	wrapped, err := projectcrypto.RewrapKey(newPassphrase, newSalt, params, binding, dek)
	if err != nil {
		return nil, err
	}
	env.Salt = hex.EncodeToString(newSalt)
	env.WrappedKey = encodeCiphertext(wrapped)
	env.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	// Identities and entry ciphertext stay byte-identical; only the wrapping changed.
	if err := projectstore.Save(path, env); err != nil {
		return nil, err
	}
	return env, nil
}

// List returns redacted entry metadata sorted by name. No plaintext is touched.
func (Service) List(repoRoot, relPath string) ([]EntryMetadata, *projectstore.Envelope, error) {
	_, env, err := load(repoRoot, relPath)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(env.Entries))
	for n := range env.Entries {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]EntryMetadata, 0, len(names))
	for _, n := range names {
		e := env.Entries[n]
		out = append(out, EntryMetadata{
			Name: n, Identity: e.Identity, Kind: e.Kind, Format: e.Format, Version: e.Version,
		})
	}
	return out, env, nil
}

// Verify confirms the passphrase unwraps the envelope DEK. It touches no entry
// plaintext and returns the fail-closed sentinel on any authentication failure.
// The CLI `unlock` command uses this to prove the source actually works.
func (Service) Verify(repoRoot, relPath string, passphrase []byte) error {
	_, env, err := load(repoRoot, relPath)
	if err != nil {
		return err
	}
	dek, _, err := unlockDEK(env, passphrase)
	if err != nil {
		return err
	}
	projectcrypto.Wipe(dek)
	return nil
}

// EntryMetadata is the redacted view of one entry.
type EntryMetadata struct {
	Name     string `json:"name"`
	Identity string `json:"identity"`
	Kind     string `json:"kind"`
	Format   string `json:"format"`
	Version  string `json:"version"`
}

func load(repoRoot, relPath string) (string, *projectstore.Envelope, error) {
	path, err := projectstore.ResolveAssetPath(repoRoot, relPath)
	if err != nil {
		return "", nil, err
	}
	env, err := projectstore.Load(path)
	if err != nil {
		return "", nil, err
	}
	// omitempty drops an empty entries map on save, so Load yields nil; keep the
	// map usable for mutators that assign into it.
	if env.Entries == nil {
		env.Entries = map[string]projectstore.EnvelopeEntry{}
	}
	return path, env, nil
}

func unlockDEK(env *projectstore.Envelope, passphrase []byte) ([]byte, projectcrypto.Binding, error) {
	salt, err := hex.DecodeString(env.Salt)
	if err != nil {
		return nil, projectcrypto.Binding{}, fmt.Errorf("projectapp: salt decode: %w", err)
	}
	params := projectcrypto.Params{
		Time: env.KDF.Time, Memory: env.KDF.Memory, Threads: env.KDF.Threads, KeyLen: env.KDF.KeyLen,
	}
	if !params.AtOrAbove(projectcrypto.MinimumParams()) {
		return nil, projectcrypto.Binding{}, fmt.Errorf("projectapp: kdf below minimum")
	}
	binding := projectcrypto.Binding{
		Schema: env.SchemaVersion, Project: env.Project, Repository: env.Repository,
	}
	wrapped, err := decodeCiphertext(env.WrappedKey)
	if err != nil {
		return nil, projectcrypto.Binding{}, fmt.Errorf("projectapp: wrapped key decode: %w", err)
	}
	dek, err := projectcrypto.UnwrapKey(passphrase, salt, params, binding, wrapped)
	if err != nil {
		return nil, projectcrypto.Binding{}, err
	}
	return dek, binding, nil
}

func encodeCiphertext(c projectcrypto.Ciphertext) projectstore.Ciphertext {
	return projectstore.Ciphertext{
		Nonce:      hex.EncodeToString(c.Nonce),
		Ciphertext: hex.EncodeToString(c.Ciphertext),
	}
}

func decodeCiphertext(c projectstore.Ciphertext) (projectcrypto.Ciphertext, error) {
	nonce, err := hex.DecodeString(c.Nonce)
	if err != nil {
		return projectcrypto.Ciphertext{}, err
	}
	ct, err := hex.DecodeString(c.Ciphertext)
	if err != nil {
		return projectcrypto.Ciphertext{}, err
	}
	return projectcrypto.Ciphertext{Nonce: nonce, Ciphertext: ct}, nil
}

// shortIdentity derives a stable, non-sensitive identity for an entry from its
// name. It is NOT the secret; it only labels the logical key so the AAD binding
// can distinguish entries without storing plaintext.
func shortIdentity(name string, env *projectstore.Envelope) string {
	if existing, ok := env.Entries[name]; ok && existing.Identity != "" {
		return existing.Identity
	}
	return name
}

// passphraseBytes is a no-op identity kept for symmetry with future token types.
func passphraseBytes(b []byte) []byte { return b }
