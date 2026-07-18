package remote

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretEnvelope is the repository-tracked encrypted secrets asset
// (pinax-sync.secrets.yaml). Plaintext values are available only during an
// authenticated runtime unlock; the ciphertext is safe to commit.
type SecretEnvelope struct {
	SchemaVersion string                 `json:"schema_version" yaml:"schema_version"`
	Provider      string                 `json:"provider" yaml:"provider"`
	Secrets       map[string]SecretEntry `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

// SecretEntry is one logical identity's encrypted material. Identity is the
// stable logical key id; ciphertext is provider-specific and opaque.
type SecretEntry struct {
	Identity   string `json:"identity" yaml:"identity"`
	Ciphertext string `json:"ciphertext" yaml:"ciphertext"`
}

// SecretEntryKind classifies a stored secret so the unlock path and doctor can
// distinguish credential vs encryption-key material. Stored as opaque metadata
// in the envelope; unknown kinds degrade rather than fail-open.
type SecretEntryKind string

const (
	SecretKindCredential    SecretEntryKind = "credential"
	SecretKindEncryptionKey SecretEntryKind = "encryption_key"
)

// SecretMetadata is the redacted, safe-to-show view of an entry.
type SecretMetadata struct {
	Name     string `json:"name" yaml:"name"`
	Kind     string `json:"kind" yaml:"kind"`
	Identity string `json:"identity" yaml:"identity"`
	Provider string `json:"provider" yaml:"provider"`
}

// UnlockProvider resolves a SecretEnvelope to plaintext name→value pairs using
// a device-local identity. Implementations must fail-closed on any missing or
// wrong identity and never log plaintext. The interface is provider-neutral so
// the first version can ship a deterministic fake + env provider and later
// swap in age/keychain without changing the CLI contract.
type UnlockProvider interface {
	// Name is the stable provider identifier written into the envelope.
	Name() string
	// Unlock decrypts the envelope into plaintext values. Plaintext MUST NOT
	// escape the caller; it lives only in the unlocking runtime.
	Unlock(env SecretEnvelope) (map[string]string, error)
	// Lock encrypts a single plaintext value under the given identity, returning
	// an entry suitable for SecretEnvelope.Secrets.
	Lock(name string, plaintext string, identity string, kind SecretEntryKind) (SecretEntry, error)
}

// LoadSecretEnvelope reads the secrets asset. A missing file is reported via
// ErrSecretsAssetMissing so init/doctor can distinguish absent from corrupt.
func LoadSecretEnvelope(root string) (SecretEnvelope, error) {
	b, err := os.ReadFile(SecretsAssetPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SecretEnvelope{}, ErrSecretsAssetMissing
		}
		return SecretEnvelope{}, err
	}
	var env SecretEnvelope
	if err := yaml.Unmarshal(b, &env); err != nil {
		return SecretEnvelope{}, fmt.Errorf("parse secrets asset: %w", err)
	}
	if env.SchemaVersion != "" && env.SchemaVersion != SyncSecretsSchemaVersion {
		return SecretEnvelope{}, &SecretEnvelopeError{Code: "unsupported_secrets_schema", Message: fmt.Sprintf("unsupported secrets schema version: %s", env.SchemaVersion)}
	}
	return env, nil
}

// ErrSecretsAssetMissing is returned when no pinax-sync.secrets.yaml exists.
var ErrSecretsAssetMissing = errors.New("sync secrets asset not found")

// SaveSecretEnvelope persists the envelope with restrictive permissions. It
// never writes plaintext; entries carry only ciphertext.
func SaveSecretEnvelope(root string, env SecretEnvelope) error {
	env.SchemaVersion = SyncSecretsSchemaVersion
	if env.Provider == "" {
		return &SecretEnvelopeError{Code: "missing_provider", Message: "secrets asset provider is required"}
	}
	for name, entry := range env.Secrets {
		if strings.TrimSpace(entry.Ciphertext) == "" {
			return &SecretEnvelopeError{Code: "missing_ciphertext", Message: fmt.Sprintf("secret %q has no ciphertext", name)}
		}
		if plaintextSensitivePattern.MatchString(name) {
			return &SecretEnvelopeError{Code: "plaintext_sensitive_name", Message: fmt.Sprintf("secret name %q looks plaintext-sensitive", name)}
		}
	}
	path := SecretsAssetPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secrets directory: %w", err)
	}
	data, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal secrets asset: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// SecretEnvelopeError is the stable error type for envelope operations.
type SecretEnvelopeError struct {
	Code    string
	Message string
}

func (e *SecretEnvelopeError) Error() string { return e.Message }

// --- Fake provider (deterministic, for first version + tests) ---

// FakeUnlockProvider is a deterministic AES-GCM provider keyed by a passphrase
// resolved from PINAX_SYNC_FAKE_KEY (or a fixed test key). It exists so the
// declaration layer is testable end-to-end without an age/keychain dependency.
// Production deployments must register a reviewed provider; doctor flags fake.
type FakeUnlockProvider struct {
	KeyResolver func() (string, error)
}

func (p FakeUnlockProvider) Name() string { return "fake" }

func (p FakeUnlockProvider) resolveKey() ([]byte, error) {
	var key string
	if p.KeyResolver != nil {
		k, err := p.KeyResolver()
		if err != nil {
			return nil, err
		}
		key = k
	}
	if strings.TrimSpace(key) == "" {
		key = os.Getenv("PINAX_SYNC_FAKE_KEY")
	}
	if strings.TrimSpace(key) == "" {
		return nil, &SecretEnvelopeError{Code: "sync_repo_unlock_required", Message: "no fake unlock key available; set PINAX_SYNC_FAKE_KEY or provide an identity"}
	}
	sum := sha256.Sum256([]byte(key))
	return sum[:], nil
}

func (p FakeUnlockProvider) Unlock(env SecretEnvelope) (map[string]string, error) {
	if env.Provider != "" && env.Provider != "fake" {
		return nil, &SecretEnvelopeError{Code: "provider_mismatch", Message: fmt.Sprintf("envelope provider %q does not match unlock provider fake", env.Provider)}
	}
	key, err := p.resolveKey()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(env.Secrets))
	for name, entry := range env.Secrets {
		pt, err := aesGCMDecrypt(key, entry.Ciphertext)
		if err != nil {
			return nil, &SecretEnvelopeError{Code: "decrypt_failed", Message: fmt.Sprintf("decrypt secret %q failed: %v", name, err)}
		}
		out[name] = pt
	}
	return out, nil
}

func (p FakeUnlockProvider) Lock(name, plaintext, identity string, kind SecretEntryKind) (SecretEntry, error) {
	key, err := p.resolveKey()
	if err != nil {
		return SecretEntry{}, err
	}
	ct, err := aesGCMEncrypt(key, plaintext)
	if err != nil {
		return SecretEntry{}, &SecretEnvelopeError{Code: "encrypt_failed", Message: fmt.Sprintf("encrypt secret %q failed: %v", name, err)}
	}
	return SecretEntry{Identity: identity, Ciphertext: ct}, nil
}

func aesGCMEncrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func aesGCMDecrypt(key []byte, ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// --- Env provider (CI / one-shot bootstrap compatibility) ---

// EnvUnlockProvider resolves secret values from environment variables referenced
// by identity (e.g. identity "personal-sync-key" resolves from
// PINAX_SYNC_SECRET_PERSONAL_SYNC_KEY). It keeps the no-plaintext-in-repo
// invariant while supporting ephemeral CI bootstrap without a keychain.
type EnvUnlockProvider struct{}

func (EnvUnlockProvider) Name() string { return "env" }

func (EnvUnlockProvider) envName(identity string) string {
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, strings.ToUpper(identity))
	return "PINAX_SYNC_SECRET_" + sanitized
}

func (p EnvUnlockProvider) Unlock(env SecretEnvelope) (map[string]string, error) {
	if env.Provider != "" && env.Provider != "env" {
		return nil, &SecretEnvelopeError{Code: "provider_mismatch", Message: fmt.Sprintf("envelope provider %q does not match unlock provider env", env.Provider)}
	}
	out := make(map[string]string, len(env.Secrets))
	for name, entry := range env.Secrets {
		val := os.Getenv(p.envName(entry.Identity))
		if strings.TrimSpace(val) == "" {
			return nil, &SecretEnvelopeError{Code: "sync_repo_unlock_required", Message: fmt.Sprintf("environment variable %s is not set for secret %q", p.envName(entry.Identity), name)}
		}
		out[name] = val
	}
	return out, nil
}

func (p EnvUnlockProvider) Lock(name, plaintext, identity string, kind SecretEntryKind) (SecretEntry, error) {
	// Env provider does not persist ciphertext; it records the identity so a
	// future unlock knows which variable to read. This keeps the asset honest
	// about its provider while remaining commit-safe (no plaintext stored).
	return SecretEntry{Identity: identity, Ciphertext: "env://" + p.envName(identity)}, nil
}

// ResolveUnlockProvider selects the provider for an envelope. Unknown providers
// fail-closed; the caller surfaces a runnable recovery command.
func ResolveUnlockProvider(providerName string) (UnlockProvider, error) {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "", "fake":
		return FakeUnlockProvider{}, nil
	case "env":
		return EnvUnlockProvider{}, nil
	default:
		return nil, &SecretEnvelopeError{Code: "unsupported_provider", Message: fmt.Sprintf("unsupported unlock provider: %q", providerName)}
	}
}

// HexDigest returns a short stable hex digest of arbitrary bytes, used for the
// source marker's declaration_digest.
func HexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

// MarshalForDigest serializes a SyncConfig canonically so equal configs produce
// equal digests regardless of map ordering or whitespace.
func MarshalForDigest(c SyncConfig) []byte {
	c = c.Normalized()
	b, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	return b
}
