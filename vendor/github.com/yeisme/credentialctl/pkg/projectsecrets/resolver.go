package projectsecrets

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/yeisme/credentialctl/internal/projectcrypto"
	"github.com/yeisme/credentialctl/internal/projectstore"
)

// EnvelopeInfo is the redacted, plaintext-free view of a loaded envelope that
// the policy and grant are checked against. It carries identity, the digest and
// the set of entry names only.
type EnvelopeInfo struct {
	Project    string
	Repository string
	Provider   string
	Digest     string
	Entries    map[string]EntryInfo
}

// EntryInfo is the redacted view of one envelope entry.
type EntryInfo struct {
	Identity string
	Kind     string
	Format   string
	Version  string
}

// Policy gates whether a grant may resolve against an envelope.
type Policy interface {
	Approve(g Grant, env EnvelopeInfo) error
}

// Resolver resolves an approved grant against a repository envelope using an
// unlock source. It is safe for concurrent use; each Resolve call produces an
// independent Snapshot.
type Resolver struct {
	envPath string
	source  UnlockSource
	policy  Policy
}

// NewResolver binds an envelope path, an unlock source and a policy.
func NewResolver(envPath string, source UnlockSource, policy Policy) (*Resolver, error) {
	if envPath == "" {
		return nil, fmt.Errorf("projectsecrets: empty envelope path")
	}
	if source == nil {
		return nil, fmt.Errorf("projectsecrets: nil unlock source")
	}
	if policy == nil {
		policy = DenyAllPolicy{}
	}
	return &Resolver{envPath: envPath, source: source, policy: policy}, nil
}

// EnvelopeInfo loads the envelope and returns its redacted view without
// decrypting. Callers use it to build grants (digest, entry names).
func (r *Resolver) EnvelopeInfo() (EnvelopeInfo, error) {
	env, err := projectstore.Load(r.envPath)
	if err != nil {
		return EnvelopeInfo{}, mapStoreError(err)
	}
	return envelopeInfoFrom(env)
}

// Resolve validates the grant, unlocks the DEK and returns a Snapshot of the
// requested entries. It fails closed on any authentication, policy or digest
// mismatch without reading or comparing entry values.
func (r *Resolver) Resolve(ctx context.Context, g Grant) (*Snapshot, error) {
	env, err := projectstore.Load(r.envPath)
	if err != nil {
		return nil, mapStoreError(err)
	}
	info, err := envelopeInfoFrom(env)
	if err != nil {
		return nil, err
	}
	if err := r.policy.Approve(g, info); err != nil {
		return nil, err
	}
	if env.Provider != ProviderPassphraseV1 {
		return nil, ProviderUnsupportedError("provider " + env.Provider + " not supported")
	}
	// Envelope-level binding for DEK unwrap.
	salt, err := hex.DecodeString(env.Salt)
	if err != nil {
		return nil, AssetInvalidError("salt encoding")
	}
	params := projectcrypto.Params{
		Time:    env.KDF.Time,
		Memory:  env.KDF.Memory,
		Threads: env.KDF.Threads,
		KeyLen:  env.KDF.KeyLen,
	}
	if !params.AtOrAbove(projectcrypto.MinimumParams()) {
		// A repository that weakens the KDF below floor is rejected; the store
		// should never have written such a profile, but verify on read too.
		return nil, AssetInvalidError("kdf profile below minimum")
	}
	envBinding := projectcrypto.Binding{
		Schema: env.SchemaVersion, Project: env.Project, Repository: env.Repository,
	}
	wrapped, err := decodeCiphertext(env.WrappedKey)
	if err != nil {
		return nil, AssetInvalidError("wrapped key encoding")
	}
	secret, err := r.source.Secret(ctx)
	if err != nil {
		return nil, mapSourceError(err)
	}
	defer wipe(secret)
	dek, err := projectcrypto.UnwrapKey(secret, salt, params, envBinding, wrapped)
	if err != nil {
		return nil, UnlockFailedError("unlock failed")
	}
	// Decrypt each requested entry under its per-entry binding.
	decrypted := make(map[string][]byte, len(g.Entries))
	for _, name := range g.Entries {
		entry, ok := env.Entries[name]
		if !ok {
			projectcrypto.Wipe(dek)
			return nil, EntryMissingError("entry " + name)
		}
		ct, err := decodeCiphertext(entry.Ciphertext)
		if err != nil {
			projectcrypto.Wipe(dek)
			return nil, AssetInvalidError("entry " + name + " encoding")
		}
		binding := projectcrypto.Binding{
			Schema:     env.SchemaVersion,
			Project:    env.Project,
			Repository: env.Repository,
			EntryName:  name,
			Identity:   entry.Identity,
			Kind:       entry.Kind,
			Format:     entry.Format,
			Version:    entry.Version,
		}
		pt, err := projectcrypto.DecryptEntry(dek, binding, ct)
		if err != nil {
			projectcrypto.Wipe(dek)
			return nil, UnlockFailedError("unlock failed")
		}
		decrypted[name] = pt
	}
	projectcrypto.Wipe(dek)
	return &Snapshot{
		metadata: ResolutionMetadata{
			Project:    env.Project,
			Repository: env.Repository,
			Provider:   env.Provider,
			Digest:     info.Digest,
			Source:     r.source.Descriptor(),
			Entries:    append([]string(nil), g.Entries...),
		},
		entries: decrypted,
	}, nil
}

func envelopeInfoFrom(env *projectstore.Envelope) (EnvelopeInfo, error) {
	digest, err := env.Digest()
	if err != nil {
		return EnvelopeInfo{}, AssetInvalidError("digest")
	}
	entries := make(map[string]EntryInfo, len(env.Entries))
	for name, e := range env.Entries {
		entries[name] = EntryInfo{Identity: e.Identity, Kind: e.Kind, Format: e.Format, Version: e.Version}
	}
	return EnvelopeInfo{
		Project:    env.Project,
		Repository: env.Repository,
		Provider:   env.Provider,
		Digest:     digest,
		Entries:    entries,
	}, nil
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

func mapStoreError(err error) error {
	switch err {
	case projectstore.ErrAssetMissing:
		return AssetMissingError("envelope not found")
	case projectstore.ErrAssetInvalid:
		return AssetInvalidError("envelope invalid")
	case projectstore.ErrPathUnsafe:
		return PathUnsafeError("envelope path unsafe")
	default:
		return AssetInvalidError(err.Error())
	}
}

// DenyAllPolicy denies every grant. It is the safe default when no policy is
// configured so a Resolver never silently approves an unconfigured consumer.
type DenyAllPolicy struct{}

func (DenyAllPolicy) Approve(Grant, EnvelopeInfo) error {
	return GrantDeniedError("no policy configured")
}

// wipe best-effort zeroes a byte slice.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
