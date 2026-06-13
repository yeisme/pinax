package remote

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCryptoEnvelopeRoundTripAndDoesNotLeakPlaintext(t *testing.T) {
	key, err := DeriveKey("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	plain := []byte("# Alpha\nsecret local body\n")
	envelope, err := EncryptBlob(key, plain, []byte(PathHash("notes/alpha.md")))
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
	got, err := DecryptBlob(key, envelope, []byte(PathHash("notes/alpha.md")))
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
	key, err := DeriveKey("op://pinax/cloud-token")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	envelope, err := EncryptManifest(key, manifest)
	if err != nil {
		t.Fatalf("encrypt manifest: %v", err)
	}
	encoded, _ := json.Marshal(envelope)
	if strings.Contains(string(encoded), "secret local body") || strings.Contains(string(encoded), "notes/alpha.md") {
		t.Fatalf("manifest envelope leaked local data:\n%s", encoded)
	}
	decoded, err := DecryptManifest(key, envelope)
	if err != nil {
		t.Fatalf("decrypt manifest: %v", err)
	}
	if decoded.EntryCount != manifest.EntryCount || decoded.Entries[0].BlobID != manifest.Entries[0].BlobID {
		t.Fatalf("decoded manifest = %#v want %#v", decoded, manifest)
	}
}
