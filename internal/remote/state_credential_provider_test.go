package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// TestGetStoreWithCredentialProviderSignsWithInjectedCredential verifies the
// repository-encrypted transport path (pinax-passphrase-s3-bootstrap pull
// execution): State.GetStoreWithCredentialProvider builds an S3 backend whose
// requests are SIGNED with the explicitly injected credentials provider, not the
// device-local shared profile chain. The SigV4 Authorization header carries the
// injected AccessKeyId.
func TestGetStoreWithCredentialProviderSignsWithInjectedCredential(t *testing.T) {
	var gotAuth string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fake.Close()

	state := State{Config: Config{
		Endpoint:    fake.URL,
		WorkspaceID: "ws-x",
		BackendKind: "s3-direct",
		S3: &S3Config{
			Bucket:   "bucket",
			Prefix:   "prefix/",
			Endpoint: fake.URL,
			Region:   "us-east-1",
		},
	}}
	provider := credentials.NewStaticCredentialsProvider("AKIDSTATE", "SKSTATE", "")
	store, err := state.GetStoreWithCredentialProvider(context.Background(), provider)
	if err != nil {
		t.Fatalf("get store: %v", err)
	}
	_, _ = store.Stat(context.Background(), "any-key")
	if gotAuth == "" {
		t.Fatal("no Authorization header captured")
	}
	if !strings.Contains(gotAuth, "AKIDSTATE") {
		t.Fatalf("request not signed with injected credential: %s", gotAuth)
	}
}

// TestGetStoreWithCredentialProviderNilFallsBack verifies that a nil provider
// (device-profile mode) falls back to the default endpoint-based store path.
func TestGetStoreWithCredentialProviderNilFallsBack(t *testing.T) {
	state := State{Config: Config{
		Endpoint:    "s3://bucket/prefix/",
		WorkspaceID: "ws-x",
		BackendKind: "s3-direct",
	}}
	store, err := state.GetStoreWithCredentialProvider(context.Background(), nil)
	if err != nil {
		t.Fatalf("get store with nil provider: %v", err)
	}
	if store == nil {
		t.Fatal("nil store returned")
	}
}

// TestGetStoreWithCredentialProviderNoS3ConfigFallsBack verifies that when the
// state has no S3 config (e.g. server backend), the provider is ignored and the
// endpoint-based store is used.
func TestGetStoreWithCredentialProviderNoS3ConfigFallsBack(t *testing.T) {
	state := State{Config: Config{Endpoint: "s3://bucket/prefix/", WorkspaceID: "ws-x"}}
	store, err := state.GetStoreWithCredentialProvider(context.Background(), credentials.NewStaticCredentialsProvider("a", "b", ""))
	if err != nil {
		t.Fatalf("get store: %v", err)
	}
	if store == nil {
		t.Fatal("nil store")
	}
}

var _ aws.CredentialsProvider = (aws.CredentialsProvider)(nil)
