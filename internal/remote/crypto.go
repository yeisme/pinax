package remote

import "github.com/yeisme/capsa"

const CryptoEnvelopeSchemaVersion = "pinax.cloud.envelope.v1"

type CryptoKey = capsa.CryptoKey

type EncryptedEnvelope = capsa.EncryptedEnvelope

func DeriveKey(secretRef string) (CryptoKey, error) {
	return capsa.DeriveKey(secretRef)
}

// KeyID resolves the secret reference and returns the stable key identifier for
// secretRef, or an empty string when the reference cannot be resolved.
func KeyID(secretRef string) string {
	id, err := capsa.KeyID(secretRef)
	if err != nil {
		return ""
	}
	return id
}

func EncryptBlob(key CryptoKey, plaintext, aad []byte) (EncryptedEnvelope, error) {
	return capsa.EncryptBlob(key, plaintext, aad)
}

func DecryptBlob(key CryptoKey, envelope EncryptedEnvelope, aad []byte) ([]byte, error) {
	return capsa.DecryptBlob(key, envelope, aad)
}

func EncryptManifest(key CryptoKey, manifest Manifest) (EncryptedEnvelope, error) {
	return capsa.EncryptManifest(key, manifest)
}

func DecryptManifest(key CryptoKey, envelope EncryptedEnvelope) (Manifest, error) {
	var manifest Manifest
	if err := capsa.DecryptManifest(key, envelope, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
