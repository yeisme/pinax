package app

import (
	"context"
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

// TestCloudTransportForStateWithCredentialResolvesAndInjects verifies the
// app-layer transport wiring: in repository-encrypted mode with an unlock
// source, cloudTransportForStateWithCredential resolves the typed bundle and
// returns a non-nil transport + non-nil snapshot (caller closes after the run).
// The actual SigV4 signing under the injected provider is verified in the
// remote-layer test (TestGetStoreWithCredentialProviderSignsWithInjectedCredential).
func TestCloudTransportForStateWithCredentialResolvesAndInjects(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	store := projectsecrets.NewStore()
	if _, err := store.Init(repo, ".pinax/project-secrets.yaml", "pinax", "ws-x", "passphrase-v1", []byte(testPassphrase)); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := store.SetEntry(repo, ".pinax/project-secrets.yaml", "default", "credential", "s3_credentials.v1", "1",
		[]byte(`{"access_key_id":"AKIDTRANSPORT","secret_access_key":"SKTRANSPORT"}`), []byte(testPassphrase)); err != nil {
		t.Fatalf("set entry: %v", err)
	}

	state := pinaxcloud.State{Config: pinaxcloud.Config{
		Endpoint: "s3://b/p/", WorkspaceID: "ws-x", BackendKind: "s3-direct",
		S3: &pinaxcloud.S3Config{Bucket: "b", Prefix: "p/", Endpoint: "https://example.com", Region: "us-east-1", CredentialMode: pinaxcloud.CredentialModeRepositoryEncrypted},
	}}
	transport, snap, err := cloudTransportForStateWithCredential(context.Background(), state, repo, projectsecrets.StaticSource([]byte(testPassphrase)))
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if transport == nil {
		t.Fatal("nil transport")
	}
	if snap == nil {
		t.Fatal("snapshot should be returned for caller to close")
	}
	func() { _ = snap.Close() }()
}

// TestCloudTransportForStateWithCredentialFallsBackForDeviceProfile verifies
// device-profile mode (or nil source) falls back to the default transport and
// returns a nil snapshot (no credential resolution).
func TestCloudTransportForStateWithCredentialFallsBackForDeviceProfile(t *testing.T) {
	t.Parallel()
	state := pinaxcloud.State{Config: pinaxcloud.Config{
		Endpoint: "s3://bucket/prefix/", WorkspaceID: "ws-x", BackendKind: "s3-direct",
		S3: &pinaxcloud.S3Config{Bucket: "bucket", Prefix: "prefix/", CredentialMode: pinaxcloud.CredentialModeDeviceProfile},
	}}
	transport, snap, err := cloudTransportForStateWithCredential(context.Background(), state, "", nil)
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if transport == nil {
		t.Fatal("nil transport")
	}
	if snap != nil {
		t.Fatal("snapshot should be nil for device-profile fallback")
	}
}
