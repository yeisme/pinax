package app

import (
	"context"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
)

const testPassphrase = "pinax-test-passphrase-0123"

func writeEnvelope(t *testing.T, repo string) {
	t.Helper()
	store := projectsecrets.NewStore()
	if _, err := store.Init(repo, ".pinax/project-secrets.yaml", "pinax", "yeisme-notes", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("init envelope: %v", err)
	}
	payload := []byte(`{"access_key_id":"AKIDPINAX","secret_access_key":"SKPINAX","session_token":"SESSIONTOK"}`)
	if _, err := store.SetEntry(repo, ".pinax/project-secrets.yaml", "tencent-cos-pinax", "credential", "s3_credentials.v1", "1", payload, []byte(testPassphrase)); err != nil {
		t.Fatalf("set entry: %v", err)
	}
}

func TestSyncCredentialResolverProducesProvider(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeEnvelope(t, repo)
	r := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	provider, snap, err := r.Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer func() { _ = snap.Close() }()
	creds, err := provider.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if creds.AccessKeyID != "AKIDPINAX" {
		t.Fatalf("access key: got %q want AKIDPINAX", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "SKPINAX" {
		t.Fatalf("secret key: got %q want SKPINAX", creds.SecretAccessKey)
	}
	if creds.SessionToken != "SESSIONTOK" {
		t.Fatalf("session token: got %q want SESSIONTOK", creds.SessionToken)
	}
}

func TestSyncCredentialResolverWrongPassphraseFails(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeEnvelope(t, repo)
	r := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	_, _, err := r.Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte("wrong-pass")))
	if err == nil {
		t.Fatal("wrong passphrase should fail closed")
	}
}

func TestSyncCredentialResolverMissingEnvelope(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	_, _, err := r.Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err == nil {
		t.Fatal("missing envelope should fail")
	}
}

func TestSyncCredentialResolverClosesSnapshot(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeEnvelope(t, repo)
	r := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax")
	_, snap, err := r.Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	snap.Close() //nolint:errcheck // best-effort wipe in test
	// After Close, the snapshot no longer exposes the entry.
	if _, ok := snap.Entry("tencent-cos-pinax"); ok {
		t.Fatal("snapshot still readable after Close")
	}
}
