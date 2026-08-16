package remote

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCryptoEnvelopeRoundTripAndDoesNotLeakPlaintext(t *testing.T) {
	keys, err := DeriveKeychain("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	plain := []byte("# Alpha\nsecret local body\n")
	envelope, err := EncryptBlob(keys.Active, plain, []byte(PathHash("notes/alpha.md")))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	for _, forbidden := range []string{"secret local body", "notes/alpha.md", "cloud-token", "Authorization"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("encrypted envelope leaked %q:\n%s", forbidden, encoded)
		}
	}
	got, err := DecryptBlob(keys, envelope, []byte(PathHash("notes/alpha.md")))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("decrypt = %q", got)
	}
}

func TestCryptoManifestEnvelope(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, root+"/notes/alpha.md", "# Alpha\nsecret local body\n")
	manifest, err := BuildManifest(root)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	keys, err := DeriveKeychain("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	envelope, err := EncryptManifest(keys.Active, manifest)
	if err != nil {
		t.Fatalf("encrypt manifest: %v", err)
	}
	encoded, _ := json.Marshal(envelope)
	if strings.Contains(string(encoded), "secret local body") || strings.Contains(string(encoded), "notes/alpha.md") {
		t.Fatalf("manifest envelope leaked local data:\n%s", encoded)
	}
	decoded, err := DecryptManifest(keys, envelope)
	if err != nil {
		t.Fatalf("decrypt manifest: %v", err)
	}
	if decoded.EntryCount != manifest.EntryCount || decoded.Entries[0].BlobID != manifest.Entries[0].BlobID {
		t.Fatalf("decoded manifest = %#v want %#v", decoded, manifest)
	}
}

const deriveKeyAWSCredentialsFixture = `[test-profile]
aws_access_key_id = AKIATEST
aws_secret_access_key = raw-shared-secret
`

func writeDeriveKeyCredentialsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/credentials"
	if err := os.WriteFile(path, []byte(deriveKeyAWSCredentialsFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestDeriveKeyProfileEquivalentToPlain(t *testing.T) {
	path := writeDeriveKeyCredentialsFixture(t)
	orig := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	_ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	defer func() { _ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", orig) }()

	// profile:// must resolve to the raw secret BEFORE PBKDF2, so it derives the
	// same key as plain:<raw-secret>. The literal ref string must never be used.
	profileKey, err := DeriveKey("profile://test-profile")
	if err != nil {
		t.Fatalf("derive profile key: %v", err)
	}
	plainKey, err := DeriveKey("plain:raw-shared-secret")
	if err != nil {
		t.Fatalf("derive plain key: %v", err)
	}
	if profileKey.KeyID != plainKey.KeyID {
		t.Fatalf("profile key %q != plain key %q (raw ref was used as key material?)", profileKey.KeyID, plainKey.KeyID)
	}
	// Sanity: the literal ref must NOT match (proves resolution happened).
	rawKey, err := DeriveKey("plain:profile://test-profile")
	if err != nil {
		t.Fatalf("derive raw-literal key: %v", err)
	}
	if profileKey.KeyID == rawKey.KeyID {
		t.Fatalf("profile key matched the raw ref derivation — resolution did not run")
	}
}

func TestDeriveKeyFailsClosedOnUnresolvableRef(t *testing.T) {
	// An unset env var must error, never fall back to the literal ref string.
	orig := os.Getenv("PINAX_DERIVE_KEY_UNSET")
	_ = os.Unsetenv("PINAX_DERIVE_KEY_UNSET")
	defer func() { _ = os.Setenv("PINAX_DERIVE_KEY_UNSET", orig) }()

	if _, err := DeriveKey("env://PINAX_DERIVE_KEY_UNSET"); err == nil {
		t.Fatal("expected DeriveKey to fail closed on unresolvable env ref")
	}
}

func TestDeriveKeyBareStringStillResolves(t *testing.T) {
	// A bare string (no scheme) is returned as-is by ResolveSecretRef, so legacy
	// callers passing a raw secret still derive a key.
	key, err := DeriveKey("legacy-raw-secret")
	if err != nil {
		t.Fatalf("derive bare key: %v", err)
	}
	if key.KeyID == "" {
		t.Fatal("expected non-empty key id for bare secret")
	}
}

func TestKeyIDMatchesDeriveKeyAndEmptyOnError(t *testing.T) {
	key, err := DeriveKey("plain:shared")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if id := KeyID("plain:shared"); id != key.KeyID {
		t.Fatalf("KeyID = %q, want %q", id, key.KeyID)
	}
	orig := os.Getenv("PINAX_DERIVE_KEY_UNSET_KID")
	_ = os.Unsetenv("PINAX_DERIVE_KEY_UNSET_KID")
	defer func() { _ = os.Setenv("PINAX_DERIVE_KEY_UNSET_KID", orig) }()
	if id := KeyID("env://PINAX_DERIVE_KEY_UNSET_KID"); id != "" {
		t.Fatalf("KeyID on unresolvable ref = %q, want empty", id)
	}
}

// TestKeychainDecryptsLegacyEnvelopes pins the migration guarantee: envelopes
// written under the pre-v2 derivation stay readable through the keychain even
// after the vault provisions a per-vault v2 salt.
func TestKeychainDecryptsLegacyEnvelopes(t *testing.T) {
	writeDeriveKeyCredentialsFixture(t)
	legacy, err := DeriveKeyLegacy("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive legacy: %v", err)
	}
	keys, err := DeriveKeychain("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive keychain: %v", err)
	}
	if legacy.KeyID == keys.Active.KeyID {
		t.Fatal("legacy and v2 derivations unexpectedly produced the same key id")
	}
	plain := []byte("legacy envelope body")
	envelope, err := EncryptBlob(legacy, plain, []byte("blob_legacy"))
	if err != nil {
		t.Fatalf("encrypt legacy: %v", err)
	}
	got, err := DecryptBlob(keys, envelope, []byte("blob_legacy"))
	if err != nil || string(got) != string(plain) {
		t.Fatalf("legacy decrypt via keychain = %q err=%v", got, err)
	}
	// An envelope under a foreign key id must still fail closed.
	other, err := DeriveKeychain("op://pinax/other-token")
	if err != nil {
		t.Fatalf("derive other: %v", err)
	}
	foreign, err := EncryptBlob(other.Active, plain, []byte("blob_foreign"))
	if err != nil {
		t.Fatalf("encrypt foreign: %v", err)
	}
	if _, err := DecryptBlob(keys, foreign, []byte("blob_foreign")); err == nil {
		t.Fatal("foreign key id unexpectedly decrypted")
	}
}
