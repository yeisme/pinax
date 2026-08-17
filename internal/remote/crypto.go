package remote

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/yeisme/pinax/internal/profile"
	"github.com/yeisme/pinax/internal/syncwire"
	"golang.org/x/crypto/pbkdf2"
)

const CryptoEnvelopeSchemaVersion = syncwire.EnvelopeSchemaVersion

const (
	// keyDerivationSaltLegacy is the historical static global salt. It remains
	// in use only to derive the legacy read key so envelopes written before the
	// v2 derivation stay decryptable.
	keyDerivationSaltLegacy = "capsa-sync-salt-v1"
	// keyDerivationSaltV2Seed domains-separates the v2 salt derivation. The v2
	// salt is derived from the shared secret itself (not stored anywhere), so
	// every device with the same secret derives the same key while each secret
	// gets its own salt — defeating cross-secret precomputation tables without
	// any per-vault state to migrate.
	keyDerivationSaltV2Seed = "capsa-sync-salt-v2"
	// keyDerivationIterationsLegacy was below current OWASP guidance for
	// PBKDF2-SHA256; v2 raises it to 600k.
	keyDerivationIterationsLegacy = 100000
	keyDerivationIterationsV2     = 600000
	keySize                       = 32
	manifestAssociatedData        = "pinax.cloud.manifest"
)

// kdfIterationsOverride lets tests shrink PBKDF2 iteration counts; zero
// keeps the production constants. Atomic so -race stays clean when parallel
// tests derive keys while a TestMain sets it once.
var kdfIterationsOverride atomic.Int64

// kdfIterations returns the iteration count for a derivation, honoring the
// test-only override.
func kdfIterations(productionDefault int) int {
	if override := int(kdfIterationsOverride.Load()); override > 0 {
		return override
	}
	return productionDefault
}

// SetKeyDerivationIterationsForTesting is TEST-ONLY: it overrides the
// PBKDF2-SHA256 iteration counts for both the v2 and legacy derivations in
// this process. Never call it from non-test code — it would silently weaken
// every key derived here. Legitimate callers are _test.go files and TestMain
// functions only; TestKeyDerivationOverrideIsTestOnly enforces this by
// walking the module and failing if any production file references it.
func SetKeyDerivationIterationsForTesting(iterations int) {
	if iterations <= 0 {
		kdfIterationsOverride.Store(0)
		return
	}
	kdfIterationsOverride.Store(int64(iterations))
}

type CryptoKey struct {
	KeyID string
	key   []byte
}

// EncryptedEnvelope is the syncwire envelope; remote aliases it so the
// encryption helpers and the transport layer share one wire schema.
type EncryptedEnvelope = syncwire.Envelope

// deriveKeyFrom derives a key identifier and raw key from a derived key
// using the shared derivation recipe.
func deriveKeyFrom(resolved string, salt string, iterations int) CryptoKey {
	key := pbkdf2.Key([]byte(resolved), []byte(salt), iterations, keySize, sha256.New)
	keyIDHash := sha256.Sum256(append([]byte("capsa-key-id\x00"), key...))
	return CryptoKey{KeyID: "key_" + hex.EncodeToString(keyIDHash[:])[:16], key: key}
}

// DeriveKeyV2 derives the active v2 key: 600k PBKDF2-SHA256 iterations over
// the resolved secret and a salt derived from the secret itself. All devices
// sharing the secret derive the same key; distinct secrets get distinct
// salts, so one precomputed table cannot serve multiple users.
func DeriveKeyV2(secretRef string) (CryptoKey, error) {
	resolved, err := resolveSyncSecret(secretRef)
	if err != nil {
		return CryptoKey{}, err
	}
	salt := deriveV2Salt(resolved)
	return deriveKeyFrom(resolved, salt, kdfIterations(keyDerivationIterationsV2)), nil
}

func deriveV2Salt(resolved string) string {
	digest := sha256.Sum256(append([]byte(keyDerivationSaltV2Seed+"\x00"), []byte(resolved)...))
	return hex.EncodeToString(digest[:])
}

// DeriveKeyLegacy derives the pre-v2 key (static global salt, 100k iterations).
// It exists so envelopes written before the v2 derivation remain readable.
func DeriveKeyLegacy(secretRef string) (CryptoKey, error) {
	resolved, err := resolveSyncSecret(secretRef)
	if err != nil {
		return CryptoKey{}, err
	}
	return deriveKeyFrom(resolved, keyDerivationSaltLegacy, kdfIterations(keyDerivationIterationsLegacy)), nil
}

// DeriveKey derives the active v2 key.
func DeriveKey(secretRef string) (CryptoKey, error) {
	return DeriveKeyV2(secretRef)
}

func resolveSyncSecret(secretRef string) (string, error) {
	if secretRef == "" {
		return "", fmt.Errorf("secret reference is required")
	}
	resolved, err := profile.ResolveSecretRef(secretRef)
	if err != nil {
		return "", fmt.Errorf("resolve secret reference: %w", err)
	}
	if resolved == "" {
		return "", fmt.Errorf("resolved secret is empty")
	}
	return resolved, nil
}

// CryptoKeys is the decryption keychain: the active write key plus legacy
// read keys. Envelopes are decrypted with the key whose KeyID matches the
// envelope, so a vault migrated to the v2 derivation keeps reading blobs that
// were written under the legacy derivation until they are re-pushed.
type CryptoKeys struct {
	Active CryptoKey
	Legacy []CryptoKey
}

// DeriveKeychain derives the active v2 key plus the legacy fallback in one
// pass.
func DeriveKeychain(secretRef string) (CryptoKeys, error) {
	active, err := DeriveKeyV2(secretRef)
	if err != nil {
		return CryptoKeys{}, err
	}
	keys := CryptoKeys{Active: active}
	if legacy, legacyErr := DeriveKeyLegacy(secretRef); legacyErr == nil && legacy.KeyID != active.KeyID {
		keys.Legacy = append(keys.Legacy, legacy)
	}
	return keys, nil
}

// For returns the key matching keyID, preferring the active key.
func (k CryptoKeys) For(keyID string) (CryptoKey, bool) {
	if k.Active.KeyID != "" && k.Active.KeyID == keyID {
		return k.Active, true
	}
	for _, legacy := range k.Legacy {
		if legacy.KeyID == keyID {
			return legacy, true
		}
	}
	return CryptoKey{}, false
}

// KeyID resolves the secret reference and returns the stable key identifier for
// secretRef, or an empty string when the reference cannot be resolved.
func KeyID(secretRef string) string {
	key, err := DeriveKey(secretRef)
	if err != nil {
		return ""
	}
	return key.KeyID
}

func EncryptBlob(key CryptoKey, plaintext, aad []byte) (EncryptedEnvelope, error) {
	return encryptBytes(key, plaintext, aad)
}

func DecryptBlob(keys CryptoKeys, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	return decryptBytes(keys, envelope, aad)
}

func EncryptManifest(key CryptoKey, manifest Manifest) (EncryptedEnvelope, error) {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	return encryptBytes(key, payload, []byte(manifestAssociatedData))
}

func DecryptManifest(keys CryptoKeys, envelope EncryptedEnvelope) (Manifest, error) {
	var manifest Manifest
	payload, err := decryptBytes(keys, envelope, []byte(manifestAssociatedData))
	if err != nil {
		return Manifest{}, err
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func encryptBytes(key CryptoKey, plaintext, aad []byte) (EncryptedEnvelope, error) {
	block, err := aes.NewCipher(key.key)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedEnvelope{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	plainHash := sha256.Sum256(plaintext)
	return EncryptedEnvelope{
		SchemaVersion: CryptoEnvelopeSchemaVersion,
		Alg:           "AES-256-GCM",
		KeyID:         key.KeyID,
		Nonce:         base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:    base64.StdEncoding.EncodeToString(ciphertext),
		PlainSHA256:   hex.EncodeToString(plainHash[:]),
	}, nil
}

func decryptBytes(keys CryptoKeys, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	if err := validateEnvelope(envelope); err != nil {
		return nil, err
	}
	key, ok := keys.For(envelope.KeyID)
	if !ok {
		return nil, fmt.Errorf("key ID mismatch: envelope=%s, active=%s", envelope.KeyID, keys.Active.KeyID)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	plainHash := sha256.Sum256(plaintext)
	if hex.EncodeToString(plainHash[:]) != envelope.PlainSHA256 {
		return nil, fmt.Errorf("plaintext hash mismatch")
	}
	return plaintext, nil
}

func validateEnvelope(envelope EncryptedEnvelope) error {
	if envelope.SchemaVersion != CryptoEnvelopeSchemaVersion {
		return fmt.Errorf("invalid schema version: %s", envelope.SchemaVersion)
	}
	if envelope.Alg != "AES-256-GCM" {
		return fmt.Errorf("unsupported algorithm: %s", envelope.Alg)
	}
	if envelope.KeyID == "" || envelope.Nonce == "" || envelope.Ciphertext == "" || envelope.PlainSHA256 == "" {
		return fmt.Errorf("encrypted envelope is incomplete")
	}
	return nil
}

// SyncKeyVersions reports the active (v2) and legacy (v1) key identifiers for
// a secret reference, so `pinax sync keys` can classify a remote envelope as
// v2, legacy, or foreign without decrypting it.
func SyncKeyVersions(secretRef string) (active, legacy string, err error) {
	activeKey, err := DeriveKeyV2(secretRef)
	if err != nil {
		return "", "", err
	}
	legacyKey, err := DeriveKeyLegacy(secretRef)
	if err != nil {
		return "", "", err
	}
	return activeKey.KeyID, legacyKey.KeyID, nil
}

// ClassifyKeyID labels a remote envelope KeyID against the vault's derivations.
func ClassifyKeyID(keyID, active, legacy string) string {
	switch keyID {
	case "", active:
		return "v2"
	case legacy:
		return "legacy"
	default:
		return "unknown"
	}
}
