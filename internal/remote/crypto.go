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

	"github.com/yeisme/pinax/internal/profile"
	"golang.org/x/crypto/pbkdf2"
)

const CryptoEnvelopeSchemaVersion = "pinax.cloud.envelope.v1"

const (
	keyDerivationSalt       = "capsa-sync-salt-v1"
	keyDerivationIterations = 100000
	keySize                 = 32
	manifestAssociatedData  = "pinax.cloud.manifest"
)

type CryptoKey struct {
	KeyID string
	key   []byte
}

type EncryptedEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Alg           string `json:"alg"`
	KeyID         string `json:"key_id"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
	PlainSHA256   string `json:"plain_sha256"`
}

func DeriveKey(secretRef string) (CryptoKey, error) {
	if secretRef == "" {
		return CryptoKey{}, fmt.Errorf("secret reference is required")
	}
	resolved, err := profile.ResolveSecretRef(secretRef)
	if err != nil {
		return CryptoKey{}, fmt.Errorf("resolve secret reference: %w", err)
	}
	if resolved == "" {
		return CryptoKey{}, fmt.Errorf("resolved secret is empty")
	}
	key := pbkdf2.Key([]byte(resolved), []byte(keyDerivationSalt), keyDerivationIterations, keySize, sha256.New)
	keyIDHash := sha256.Sum256(append([]byte("capsa-key-id\x00"), key...))
	return CryptoKey{KeyID: "key_" + hex.EncodeToString(keyIDHash[:])[:16], key: key}, nil
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

func DecryptBlob(key CryptoKey, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	return decryptBytes(key, envelope, aad)
}

func EncryptManifest(key CryptoKey, manifest Manifest) (EncryptedEnvelope, error) {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	return encryptBytes(key, payload, []byte(manifestAssociatedData))
}

func DecryptManifest(key CryptoKey, envelope EncryptedEnvelope) (Manifest, error) {
	var manifest Manifest
	payload, err := decryptBytes(key, envelope, []byte(manifestAssociatedData))
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

func decryptBytes(key CryptoKey, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	if err := validateEnvelope(envelope); err != nil {
		return nil, err
	}
	if envelope.KeyID != key.KeyID {
		return nil, fmt.Errorf("key ID mismatch: envelope=%s, key=%s", envelope.KeyID, key.KeyID)
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
