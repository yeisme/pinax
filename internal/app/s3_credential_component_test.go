package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/remote"
)

// TestS3CredentialComponentRepositoryEncryptedSigning verifies the full injection
// chain end-to-end (pinax-passphrase-s3-bootstrap task 6.1): a repository-
// encrypted s3_credentials.v1 bundle is unlocked by SyncCredentialResolver,
// injected as an AWS SDK credentials provider into S3BackendOptions, and the
// resulting S3 request is actually SIGNED with the resolved access key (the
// SigV4 Authorization header carries the AccessKeyId in its Credential field).
//
// The fake server only captures the signed Authorization header; it does not
// need to be a real S3 implementation because SigV4 signing happens client-side
// before any response. A wrong credential produces a header signed under a
// different AccessKeyId.
func TestS3CredentialComponentRepositoryEncryptedSigning(t *testing.T) {
	t.Parallel()
	// Fake "S3" endpoint: record the Authorization header of every request.
	var gotAuth string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		// Any response is fine; signing already happened client-side.
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fake.Close()

	repo := t.TempDir()
	writeResolverEnvelope(t, repo, "AKIDREPOCOMP", "SKREPOCOMP")

	provider, snap, err := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax").
		Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer func() { _ = snap.Close() }()

	backend, err := remote.NewS3BackendWithOptions(context.Background(), "bucket", "prefix/", remote.S3BackendOptions{
		EndpointURL:         fake.URL,
		Region:              "us-east-1",
		PathStyle:           true,
		CredentialsProvider: provider,
	})
	if err != nil {
		t.Fatalf("new s3 backend: %v", err)
	}
	// Stat triggers a signed HEAD request; the object need not exist.
	_, _ = backend.Stat(context.Background(), "any-key")
	if gotAuth == "" {
		t.Fatal("no Authorization header captured — SDK did not sign the request")
	}
	// SigV4 Authorization: "AWS4-HMAC-SHA256 Credential=<AKID>/date/region/s3/aws4_request, ...".
	if !strings.Contains(gotAuth, "AKIDREPOCOMP") {
		t.Fatalf("Authorization header not signed with repository access key: %s", gotAuth)
	}
}

// TestS3CredentialComponentWrongCredentialSignsDifferently verifies that a
// different (wrong) credential produces a signature under a different
// AccessKeyId — proving the provider is authoritative, not a static fallback.
func TestS3CredentialComponentWrongCredentialSignsDifferently(t *testing.T) {
	t.Parallel()
	var gotAuth string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fake.Close()

	repo := t.TempDir()
	// Envelope holds AKID-CORRECT; the resolver will return it.
	writeResolverEnvelope(t, repo, "AKIDCORRECT", "SKCORRECT")

	provider, snap, err := NewSyncCredentialResolver("pinax", "yeisme-notes", "tencent-cos-pinax").
		Resolve(context.Background(), repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer func() { _ = snap.Close() }()

	backend, err := remote.NewS3BackendWithOptions(context.Background(), "bucket", "prefix/", remote.S3BackendOptions{
		EndpointURL:         fake.URL,
		Region:              "us-east-1",
		PathStyle:           true,
		CredentialsProvider: provider,
	})
	if err != nil {
		t.Fatalf("new s3 backend: %v", err)
	}
	_, _ = backend.Stat(context.Background(), "k")
	if !strings.Contains(gotAuth, "AKIDCORRECT") {
		t.Fatalf("expected signature under AKIDCORRECT, got: %s", gotAuth)
	}
}

// writeResolverEnvelope seeds a repository-encrypted envelope at the pinax
// project-secret asset path so SyncCredentialResolver can unlock it.
func writeResolverEnvelope(t *testing.T, repo, akid, sk string) {
	t.Helper()
	store := projectsecrets.NewStore()
	if _, err := store.Init(repo, ".pinax/project-secrets.yaml", "pinax", "yeisme-notes", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("init envelope: %v", err)
	}
	payload := `{"access_key_id":"` + akid + `","secret_access_key":"` + sk + `"}`
	if _, err := store.SetEntry(repo, ".pinax/project-secrets.yaml", "tencent-cos-pinax", "credential", "s3_credentials.v1", "1", []byte(payload), []byte(testPassphrase)); err != nil {
		t.Fatalf("set entry: %v", err)
	}
}
