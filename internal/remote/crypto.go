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

	"golang.org/x/crypto/pbkdf2"
)

const CryptoEnvelopeSchemaVersion = "pinax.cloud.envelope.v1"

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
		return CryptoKey{}, fmt.Errorf("secret ref required")
	}
	// Using a static salt for deterministic key derivation since secretRef is our master secret
	salt := []byte("pinax-cloud-sync-salt-v1")
	key := pbkdf2.Key([]byte(secretRef), salt, 100000, 32, sha256.New)
	keyIDHash := sha256.Sum256(append([]byte("pinax-cloud-key-id\x00"), key...))
	return CryptoKey{KeyID: "key_" + hex.EncodeToString(keyIDHash[:])[:16], key: key}, nil
}

func EncryptBlob(key CryptoKey, plaintext, aad []byte) (EncryptedEnvelope, error) {
	return encryptBytes(key, plaintext, aad)
}

func DecryptBlob(key CryptoKey, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	return decryptBytes(key, envelope, aad)
}

func EncryptManifest(key CryptoKey, manifest Manifest) (EncryptedEnvelope, error) {
	b, err := json.Marshal(manifest)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	return encryptBytes(key, b, []byte("pinax.cloud.manifest"))
}

func DecryptManifest(key CryptoKey, envelope EncryptedEnvelope) (Manifest, error) {
	b, err := decryptBytes(key, envelope, []byte("pinax.cloud.manifest"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(b, &manifest); err != nil {
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
	return EncryptedEnvelope{SchemaVersion: CryptoEnvelopeSchemaVersion, Alg: "AES-256-GCM", KeyID: key.KeyID, Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext), PlainSHA256: hex.EncodeToString(plainHash[:])}, nil
}

func decryptBytes(key CryptoKey, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	if envelope.Alg != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported envelope alg %q", envelope.Alg)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	plainHash := sha256.Sum256(plaintext)
	if got := hex.EncodeToString(plainHash[:]); envelope.PlainSHA256 != "" && got != envelope.PlainSHA256 {
		return nil, fmt.Errorf("plaintext hash mismatch")
	}
	return plaintext, nil
}
