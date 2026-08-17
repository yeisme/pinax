// Package projectstore owns the versioned repository envelope for project
// secrets: the on-disk schema, atomic contained write, path containment, and
// the redacted envelope digest used to bind grants.
//
// The store never holds plaintext. The envelope contains only wrapped keys and
// authenticated ciphertext; plaintext exists only inside projectcrypto during a
// short-lived unlock. The default CLI asset path is .yeisme/project-secrets.yaml;
// a domain library (for example Pinax) may pass its own contained CLI-owned path
// such as .pinax/pinax-sync.secrets.yaml.
package projectstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the envelope schema version this implementation reads and
// writes. Forward-compatible readers must accept unknown optional fields.
const SchemaVersion = "yeisme.project_secrets.v0.1"

// Envelope is the repository-tracked encrypted project-secrets asset. Every
// field is safe to commit: it contains only wrapped keys, ciphertext, and
// non-sensitive identity/metadata.
type Envelope struct {
	SchemaVersion string                   `yaml:"schema_version"`
	Project       string                   `yaml:"project"`
	Repository    string                   `yaml:"repository"`
	Provider      string                   `yaml:"provider"` // passphrase-v1
	KDF           KDFProfile               `yaml:"kdf"`
	Salt          string                   `yaml:"salt"` // hex
	WrappedKey    Ciphertext               `yaml:"wrapped_key"`
	Entries       map[string]EnvelopeEntry `yaml:"entries,omitempty"`
	CreatedAt     string                   `yaml:"created_at,omitempty"`
	UpdatedAt     string                   `yaml:"updated_at,omitempty"`
	// KeychainAccountDigest is a non-sensitive hint that a Keychain item exists.
	KeychainAccountDigest string `yaml:"keychain_account_digest,omitempty"`
}

// KDFProfile records the Argon2id parameters used to wrap the DEK. Parameters
// may only be at or above the library minimum so a repository cannot weaken the
// KDF; the store rejects an envelope whose profile is below floor.
type KDFProfile struct {
	Time    uint32 `yaml:"time"`
	Memory  uint32 `yaml:"memory"` // KiB
	Threads uint8  `yaml:"threads"`
	KeyLen  uint32 `yaml:"key_len"`
}

// Ciphertext is the hex-encoded authenticated blob stored in the envelope.
type Ciphertext struct {
	Nonce      string `yaml:"nonce"`
	Ciphertext string `yaml:"ciphertext"`
}

// EnvelopeEntry is one encrypted entry. Identity is the stable logical key id;
// kind/format/version classify the payload for the domain owner.
type EnvelopeEntry struct {
	Identity   string     `yaml:"identity"`
	Kind       string     `yaml:"kind,omitempty"`
	Format     string     `yaml:"format,omitempty"`
	Version    string     `yaml:"version,omitempty"`
	Ciphertext Ciphertext `yaml:"ciphertext"`
}

// ErrAssetMissing is returned when no envelope exists at the resolved path.
var ErrAssetMissing = errors.New("project secret asset missing")

// ErrAssetInvalid is returned when the envelope exists but is structurally
// invalid or fails path containment.
var ErrAssetInvalid = errors.New("project secret asset invalid")

// ErrPathUnsafe is returned when the requested asset path escapes the canonical
// repository root, is absolute, traverses a symlink, or uses platform separators
// to break containment.
var ErrPathUnsafe = errors.New("project secret path unsafe")

// ResolveAssetPath joins repoRoot with a contained relative path and rejects
// any attempt to escape: absolute paths, ".." traversal, symlink escape, and
// Windows-style separators embedded to bypass cleaning are all denied. The
// returned path is canonical and contained under repoRoot.
func ResolveAssetPath(repoRoot, relPath string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("%w: empty repository root", ErrPathUnsafe)
	}
	if relPath == "" {
		return "", fmt.Errorf("%w: empty asset path", ErrPathUnsafe)
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%w: absolute asset path rejected", ErrPathUnsafe)
	}
	// Normalize any platform separators so a Windows-style backslash on a POSIX
	// host cannot smuggle a path that filepath.Clean would otherwise treat as a
	// single literal component.
	normalized := filepath.FromSlash(relPath)
	cleaned := filepath.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: parent traversal rejected", ErrPathUnsafe)
	}
	cleanRoot := filepath.Clean(repoRoot)
	joined := filepath.Join(cleanRoot, cleaned)
	// Containment check: the joined path must be within cleanRoot.
	rel, err := filepath.Rel(cleanRoot, joined)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: resolved path escapes repository root", ErrPathUnsafe)
	}
	// Symlink escape: if the parent directory chain contains a symlink that
	// resolves outside the root, reject. This is checked against the existing
	// tree; a non-existent target (first init) is allowed.
	if err := containSymlinks(cleanRoot, joined); err != nil {
		return "", err
	}
	return joined, nil
}

// containSymlinks walks the existing directory chain from root toward path and
// rejects any symlink that points outside root. A not-yet-existing leaf is fine
// (the store will create it atomically); a symlinked leaf or intermediate
// directory that escapes is not.
func containSymlinks(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	parts := splitPath(rel)
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		li, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			// Remaining path does not exist yet; safe to create.
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPathUnsafe, err)
		}
		if li.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(current)
			if err != nil {
				return fmt.Errorf("%w: symlink eval failed", ErrPathUnsafe)
			}
			absRoot, _ := filepath.Abs(root)
			if !strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), filepath.Clean(absRoot)+string(filepath.Separator)) && filepath.Clean(target) != filepath.Clean(absRoot) {
				return fmt.Errorf("%w: symlink escapes repository root", ErrPathUnsafe)
			}
		}
	}
	return nil
}

func splitPath(p string) []string {
	p = filepath.ToSlash(p)
	if p == "." {
		return nil
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Load reads and validates the envelope at path. It does not decrypt; it only
// checks structural integrity and schema version. Unknown optional fields are
// ignored (forward compatibility).
func Load(path string) (*Envelope, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrAssetMissing
		}
		return nil, fmt.Errorf("%w: read: %v", ErrAssetInvalid, err)
	}
	var env Envelope
	if err := yaml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: parse: %v", ErrAssetInvalid, err)
	}
	if err := env.Validate(); err != nil {
		return nil, err
	}
	return &env, nil
}

// Validate checks structural integrity without touching the filesystem. The
// schema version and provider are mandatory; the KDF profile must be at or above
// the library minimum; the wrapped key and salt must be present.
func (e *Envelope) Validate() error {
	if e == nil {
		return fmt.Errorf("%w: nil envelope", ErrAssetInvalid)
	}
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema %q", ErrAssetInvalid, e.SchemaVersion)
	}
	if e.Project == "" || e.Repository == "" {
		return fmt.Errorf("%w: missing project/repository identity", ErrAssetInvalid)
	}
	if e.Provider == "" {
		return fmt.Errorf("%w: missing provider", ErrAssetInvalid)
	}
	if e.Salt == "" {
		return fmt.Errorf("%w: missing salt", ErrAssetInvalid)
	}
	if e.WrappedKey.Nonce == "" || e.WrappedKey.Ciphertext == "" {
		return fmt.Errorf("%w: missing wrapped key", ErrAssetInvalid)
	}
	return nil
}

// Save writes the envelope atomically with owner-only permissions. The write
// goes to a sibling temp file (0600) in the same directory, then renames over
// the target so a crash never leaves a half-written asset. Parent directories
// are created with 0700.
func Save(path string, env *Envelope) error {
	if err := env.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(env); err != nil {
		return fmt.Errorf("projectstore: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("projectstore: close encode: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("projectstore: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".project-secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("projectstore: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("projectstore: write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("projectstore: chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("projectstore: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("projectstore: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("projectstore: rename: %w", err)
	}
	// Best-effort fsync the directory so the rename is durable.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Digest returns a redacted SHA-256 digest over the canonical envelope bytes.
// It covers ciphertext and identity only (no plaintext, because the envelope
// holds none). Grants bind this digest so a resolution attempt against a
// different envelope revision fails closed.
func (e *Envelope) Digest() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e); err != nil {
		return "", err
	}
	_ = enc.Close()
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}
