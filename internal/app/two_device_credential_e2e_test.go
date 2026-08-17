package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
)

// TestTwoDeviceRepositoryCredentialContinuity simulates the cross-device
// repository-encrypted credential flow (pinax-passphrase-s3-bootstrap task 6.2):
//
//	Device A: credential init + set a typed S3 bundle, then "commit" the
//	          ciphertext (copy .pinax/project-secrets.yaml to device B's vault).
//	Device B: "clone" the fixture (receive the ciphertext), bootstrap with the
//	          passphrase, and resolve the same S3 credential.
//
// The credential identity (access key, envelope digest, project/repository)
// MUST be identical on both devices — the bundle survived the clone. Plaintext
// never left device A's secure stdin and device B's unlock boundary; only the
// ciphertext was "transferred". The actual note/revision pull is handled by the
// sync engine (capsa SDK transport) and is out of scope for this credential-
// continuity e2e.
func TestTwoDeviceRepositoryCredentialContinuity(t *testing.T) {
	t.Parallel()
	deviceA := t.TempDir()
	deviceB := t.TempDir()

	// --- Device A: create + commit the ciphertext bundle. ---
	store := projectsecrets.NewStore()
	if _, err := store.Init(deviceA, ".pinax/project-secrets.yaml", "pinax", "yeisme-notes", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("device A init: %v", err)
	}
	sharedAKID := "AKIDTWODEVICE"
	sharedSK := "SKTWODEVICE"
	payload := []byte(`{"access_key_id":"` + sharedAKID + `","secret_access_key":"` + sharedSK + `"}`)
	if _, err := store.SetEntry(deviceA, ".pinax/project-secrets.yaml", "tencent-cos-pinax", "credential", "s3_credentials.v1", "1", payload, []byte(testPassphrase)); err != nil {
		t.Fatalf("device A set: %v", err)
	}
	// Record device A's resolved identity.
	resolverA := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	providerA, snapA, err := resolverA.Resolve(context.Background(), deviceA, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("device A resolve: %v", err)
	}
	credsA, _ := providerA.Retrieve(context.Background())
	resolverInfoA, _ := projectsecrets.NewResolver(filepath.Join(deviceA, ".pinax/project-secrets.yaml"), projectsecrets.StaticSource([]byte(testPassphrase)), projectsecrets.DenyAllPolicy{})
	infoA, _ := resolverInfoA.EnvelopeInfo()
	func() { _ = snapA.Close() }()

	// --- "Clone": copy only the ciphertext asset to device B. ---
	src := filepath.Join(deviceA, ".pinax", "project-secrets.yaml")
	dstDir := filepath.Join(deviceB, ".pinax")
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		t.Fatalf("device B mkdir: %v", err)
	}
	ciphertext, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	// Assert the transferred artifact carries NO plaintext.
	if string(ciphertext) != "" {
		for _, secret := range []string{sharedAKID, sharedSK, testPassphrase} {
			if contains(ciphertext, secret) {
				t.Fatalf("ciphertext artifact leaked plaintext %q", secret)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dstDir, "project-secrets.yaml"), ciphertext, 0o600); err != nil {
		t.Fatalf("device B write ciphertext: %v", err)
	}

	// --- Device B: bootstrap with the passphrase + resolve. ---
	resolverB := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	providerB, snapB, err := resolverB.Resolve(context.Background(), deviceB, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("device B resolve after clone: %v", err)
	}
	defer func() { _ = snapB.Close() }()
	credsB, _ := providerB.Retrieve(context.Background())
	resolverInfoB, _ := projectsecrets.NewResolver(filepath.Join(deviceB, ".pinax/project-secrets.yaml"), projectsecrets.StaticSource([]byte(testPassphrase)), projectsecrets.DenyAllPolicy{})
	infoB, _ := resolverInfoB.EnvelopeInfo()

	// Identity continuity: same access key, secret, digest, project, repository.
	if credsA.AccessKeyID != credsB.AccessKeyID || credsA.AccessKeyID != sharedAKID {
		t.Fatalf("access key continuity broken: A=%q B=%q want=%q", credsA.AccessKeyID, credsB.AccessKeyID, sharedAKID)
	}
	if credsA.SecretAccessKey != credsB.SecretAccessKey || credsA.SecretAccessKey != sharedSK {
		t.Fatalf("secret key continuity broken: A=%q B=%q", credsA.SecretAccessKey, credsB.SecretAccessKey)
	}
	if infoA.Digest != infoB.Digest {
		t.Fatalf("envelope digest changed across clone: A=%q B=%q", infoA.Digest, infoB.Digest)
	}
	if infoA.Project != infoB.Project || infoA.Repository != infoB.Repository {
		t.Fatalf("project/repository changed across clone: %+v vs %+v", infoA, infoB)
	}
	// remote_write was never attempted in this credential-continuity e2e.
}

// TestTwoDeviceWrongPassphraseFailsOnClone verifies device B cannot unlock the
// cloned ciphertext with a wrong passphrase (fail-closed, no fallback).
func TestTwoDeviceWrongPassphraseFailsOnClone(t *testing.T) {
	t.Parallel()
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	store := projectsecrets.NewStore()
	if _, err := store.Init(deviceA, ".pinax/project-secrets.yaml", "pinax", "yeisme-notes", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("device A init: %v", err)
	}
	if _, err := store.SetEntry(deviceA, ".pinax/project-secrets.yaml", "k", "credential", "s3_credentials.v1", "1",
		[]byte(`{"access_key_id":"AK","secret_access_key":"SK"}`), []byte(testPassphrase)); err != nil {
		t.Fatalf("device A set: %v", err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(deviceA, ".pinax", "project-secrets.yaml"))
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(deviceB, ".pinax"), 0o700); err != nil {
		t.Fatalf("device B mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deviceB, ".pinax", "project-secrets.yaml"), ciphertext, 0o600); err != nil {
		t.Fatalf("device B write: %v", err)
	}

	r := NewSyncCredentialResolver("pinax", "yeisme-notes", "k")
	_, _, err = r.Resolve(context.Background(), deviceB, projectsecrets.StaticSource([]byte("wrong-passphrase-from-device-b")))
	if err == nil {
		t.Fatal("device B should fail closed on wrong passphrase")
	}
}

func contains(haystack []byte, needle string) bool {
	return len(needle) > 0 && indexOfBytes(haystack, []byte(needle)) >= 0
}

func indexOfBytes(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
