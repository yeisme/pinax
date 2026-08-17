package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

// bootstrapRepoEncryptedVault seeds a repository-encrypted declaration + envelope
// pointed at an S3 fake endpoint, then runs the staged bootstrap so the device
// runtime compiles into repository-encrypted mode. It returns the vault root and
// the static unlock source used for the envelope. The fake endpoint returns S3
// 404 for every request (empty remote), mirroring the bootstrap-transaction
// fixture, so pull is a no-op and push gets past credential resolution before
// failing at the remote.
func bootstrapRepoEncryptedVault(t *testing.T, svc *Service, endpoint string) (string, projectsecrets.UnlockSource) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	seedRepoEncryptedRepo(t, root, endpoint)
	if _, err := svc.SyncRepoBootstrap(context.Background(), SyncRepoRuntimeRequest{
		VaultPath:           root,
		DeviceID:            "dev1",
		Yes:                 true,
		ProjectUnlockSource: projectsecrets.StaticSource([]byte(testPassphrase)),
		Pull:                true,
	}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return root, projectsecrets.StaticSource([]byte(testPassphrase))
}

func newS3NotFoundFake(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCloudTransportFailsClosedRepoEncryptedWithoutSource verifies task 6.7: in
// repository-encrypted credential mode, when no unlock source is supplied the
// transport MUST fail closed with sync_repo_unlock_required instead of falling
// back to the device-local shared AWS profile / default credential chain.
func TestCloudTransportFailsClosedRepoEncryptedWithoutSource(t *testing.T) {
	t.Parallel()
	state := pinaxcloud.State{Config: pinaxcloud.Config{
		Endpoint: "s3://bucket/prefix/", WorkspaceID: "ws-x", BackendKind: "s3-direct",
		S3: &pinaxcloud.S3Config{Bucket: "bucket", Prefix: "prefix/", Endpoint: "https://cos.example.com", Region: "us-east-1", CredentialMode: pinaxcloud.CredentialModeRepositoryEncrypted},
	}}
	_, _, err := cloudTransportForStateWithCredential(context.Background(), state, "", nil)
	if !hasCommandCode(err, "sync_repo_unlock_required") {
		t.Fatalf("expected sync_repo_unlock_required fail-closed, got %v", err)
	}
}

// TestSyncPushRepoEncryptedFailsClosedWithoutSource verifies the fail-closed
// behavior end-to-end through SyncPush: a repository-encrypted vault with no
// project unlock source must refuse to push rather than silently using another
// local AWS account or the default credential chain.
func TestSyncPushRepoEncryptedFailsClosedWithoutSource(t *testing.T) {
	svc := NewService()
	root, _ := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	writeFile(t, filepath.Join(root, "notes", "pushed.md"), "# Pushed\n")
	_, err := svc.SyncPush(context.Background(), SyncRequest{VaultPath: root, Target: "capsa", Yes: true})
	if !hasCommandCode(err, "sync_repo_unlock_required") {
		t.Fatalf("expected sync_repo_unlock_required, got %v", err)
	}
}

// TestDaemonCredentialGateWiredIntoLoop verifies task 6.7: the daemon loop MUST
// carry a credential gate so that a repository-encrypted vault with no
// non-interactive unlock source enters a degraded state and emits a
// credential_degraded event instead of attempting remote writes.
func TestDaemonCredentialGateWiredIntoLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := NewService()
	root, _ := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	_, _ = svc.SyncDaemonRun(ctx, SyncDaemonRequest{VaultPath: root, Target: "capsa", Yes: true, Once: true, PollInterval: time.Hour, SyncTimeout: 5 * time.Second})
	events, err := syncdaemon.NewRepository(root).ReadEvents(50)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, event := range events {
		if event.Type == "credential_degraded" {
			return
		}
	}
	t.Fatalf("expected credential_degraded event when repo-encrypted daemon has no unlock source: %#v", events)
}

// TestDaemonCredentialGateAllowsWithUnlockSource verifies the gate does not
// block a repository-encrypted daemon when a non-interactive unlock source is
// supplied, and that the executor threads the source into the real push.
func TestDaemonCredentialGateAllowsWithUnlockSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := NewService()
	root, source := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	writeFile(t, filepath.Join(root, "notes", "pushed.md"), "# Pushed\n")
	_, _ = svc.SyncDaemonRun(ctx, SyncDaemonRequest{VaultPath: root, Target: "capsa", Yes: true, Once: true, PollInterval: time.Hour, SyncTimeout: 5 * time.Second, ProjectUnlockSource: source})
	events, err := syncdaemon.NewRepository(root).ReadEvents(50)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, event := range events {
		if event.Type == "credential_degraded" {
			t.Fatalf("gate blocked despite a non-interactive unlock source: %#v", event)
		}
	}
}

// TestDaemonExecutorPassesUnlockCredentialOnPush verifies the daemon executor
// threads the unlock source into SyncPush: with a source the push resolves the
// typed credential and proceeds past credential resolution (it may still fail
// at the remote), whereas without threading it would fail closed.
func TestDaemonExecutorPassesUnlockCredentialOnPush(t *testing.T) {
	svc := NewService()
	root, source := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	writeFile(t, filepath.Join(root, "notes", "pushed.md"), "# Pushed\n")
	executor := cloudDaemonExecutor{s: svc, root: root, target: "capsa", unlockSource: source}
	_, err := executor.Push(context.Background())
	if hasCommandCode(err, "sync_repo_unlock_required") {
		t.Fatalf("executor push failed closed at credential resolution; source not threaded: %v", err)
	}
}
