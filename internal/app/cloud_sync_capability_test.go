package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/cloudsync"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// writeCapabilityDeclaration writes a valid s3-direct repository-encrypted
// declaration carrying the given required capabilities.
func writeCapabilityDeclaration(t *testing.T, root string, capabilities []string) {
	t.Helper()
	capsYAML := ""
	if len(capabilities) > 0 {
		capsYAML = "\nrequires:\n  capabilities:\n"
		for _, c := range capabilities {
			capsYAML += "    - \"" + c + "\"\n"
		}
	}
	decl := "schema_version: \"" + pinaxremote.SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://bucket/main/\n" +
		"  s3:\n    bucket: bucket\n    endpoint: https://cos.example.com\n    region: ap-shanghai\n    credential_mode: repository-encrypted\n" +
		"workspace:\n  workspace_id: ws-cap\n" +
		"secrets:\n  credential_id: cred-1\n  encryption_key_id: enc-1\n" +
		capsYAML
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(pinaxremote.DeclarationPath(root), []byte(decl), 0o600); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
}

// TestSyncCapabilityGateRejectsUnsupportedCapability verifies task 6.8: a
// declaration requiring a capability this binary does not support fails closed.
func TestSyncCapabilityGateRejectsUnsupportedCapability(t *testing.T) {
	root := t.TempDir()
	writeCapabilityDeclaration(t, root, []string{"repository-encrypted-s3-v1", "unknown-cap-v9"})
	err := syncCapabilityGate(root)
	if !hasCommandCode(err, "sync_capability_unsupported") {
		t.Fatalf("expected sync_capability_unsupported, got %v", err)
	}
}

// TestSyncCapabilityGateAllowsSupportedCapabilities verifies the gate passes
// when every required capability is in the supported set.
func TestSyncCapabilityGateAllowsSupportedCapabilities(t *testing.T) {
	root := t.TempDir()
	writeCapabilityDeclaration(t, root, []string{"repository-encrypted-s3-v1", "capsa-remote-commit-v1", "pull-only-bootstrap-v1"})
	if err := syncCapabilityGate(root); err != nil {
		t.Fatalf("expected supported capabilities to pass, got %v", err)
	}
}

// TestSyncCapabilityGatePassesWithoutDeclaration verifies a device-profile
// runtime with no portable declaration is not blocked by the capability gate.
func TestSyncCapabilityGatePassesWithoutDeclaration(t *testing.T) {
	if err := syncCapabilityGate(t.TempDir()); err != nil {
		t.Fatalf("expected no gate error without a declaration, got %v", err)
	}
}

// TestDiffRemoteCheckedTrueWhenRemoteLoaded verifies task 6.8: a repository-
// encrypted diff with a valid unlock source reads the real remote head and
// reports remote_checked=true / diff_scope=remote-aware.
func TestDiffRemoteCheckedTrueWhenRemoteLoaded(t *testing.T) {
	svc := NewService()
	root, source := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	proj, err := svc.SyncDiff(context.Background(), SyncRequest{VaultPath: root, Target: "capsa", ProjectUnlockSource: source})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if proj.Facts["remote_checked"] != "true" {
		t.Fatalf("expected remote_checked=true, got %v", proj.Facts["remote_checked"])
	}
	if proj.Facts["diff_scope"] != "remote-aware" {
		t.Fatalf("expected diff_scope=remote-aware, got %v", proj.Facts["diff_scope"])
	}
}

// TestDiffRemoteCheckedFalseWhenCachedOnly verifies task 6.8: a repository-
// encrypted diff with no unlock source degrades to a cached comparison
// (fail-closed at credential resolution) and MUST mark remote_checked=false so
// the result cannot pass as a pre-backup check.
func TestDiffRemoteCheckedFalseWhenCachedOnly(t *testing.T) {
	svc := NewService()
	root, _ := bootstrapRepoEncryptedVault(t, svc, newS3NotFoundFake(t).URL)
	proj, err := svc.SyncDiff(context.Background(), SyncRequest{VaultPath: root, Target: "capsa"})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if proj.Facts["remote_checked"] != "false" {
		t.Fatalf("expected remote_checked=false, got %v", proj.Facts["remote_checked"])
	}
	if proj.Facts["diff_scope"] != "cached" {
		t.Fatalf("expected diff_scope=cached, got %v", proj.Facts["diff_scope"])
	}
}

// fakeReadBack is a focused remoteReadBack fake for durable-commit tests.
type fakeReadBack struct {
	head        cloudsync.Head
	headErr     error
	manifestErr error
}

func (f fakeReadBack) CurrentHead(_ context.Context, _ string) (cloudsync.Head, error) {
	return f.head, f.headErr
}
func (f fakeReadBack) GetManifest(_ context.Context, _ string) (cloudsync.Envelope, error) {
	return cloudsync.Envelope{}, f.manifestErr
}

// TestVerifyDurableCommitSuccess verifies the read-back confirms a committed
// revision observable on the remote.
func TestVerifyDurableCommitSuccess(t *testing.T) {
	result := cloudsync.CommitResult{RevisionID: "rev_1", ManifestBlobID: "manifest_1"}
	rb := fakeReadBack{head: cloudsync.Head{CurrentRevision: "rev_1"}}
	if !verifyDurableCommit(context.Background(), rb, "ws", result) {
		t.Fatal("expected durable commit when head matches committed revision")
	}
}

// TestVerifyDurableCommitFailureHeadMismatch verifies the spec failure re-check:
// when the read-back head does not match the committed revision the commit is
// not durable (blob/commit may have succeeded but the revision is unobservable).
func TestVerifyDurableCommitFailureHeadMismatch(t *testing.T) {
	result := cloudsync.CommitResult{RevisionID: "rev_1", ManifestBlobID: "manifest_1"}
	rb := fakeReadBack{head: cloudsync.Head{CurrentRevision: "rev_other"}}
	if verifyDurableCommit(context.Background(), rb, "ws", result) {
		t.Fatal("expected non-durable commit on head mismatch")
	}
}

// TestVerifyDurableCommitFailureManifestUnreadable verifies a commit whose
// manifest cannot be read back is not durable.
func TestVerifyDurableCommitFailureManifestUnreadable(t *testing.T) {
	result := cloudsync.CommitResult{RevisionID: "rev_1", ManifestBlobID: "manifest_1"}
	rb := fakeReadBack{head: cloudsync.Head{CurrentRevision: "rev_1"}, manifestErr: errors.New("missing manifest")}
	if verifyDurableCommit(context.Background(), rb, "ws", result) {
		t.Fatal("expected non-durable commit when manifest is unreadable")
	}
}

// TestPushUpToDateFactWhenNoChanges verifies task 6.8: after a successful push, a
// second push with no local changes and an unchanged remote reports
// up_to_date=true with remote_checked=true and remote_write=false —
// distinguishable from a blocked, failed, or dry-run push.
func TestPushUpToDateFactWhenNoChanges(t *testing.T) {
	ctx := context.Background()
	svc, deviceA, _ := cloudPushAutoRebaseIntegrationFixture(t)
	writeFile(t, filepath.Join(deviceA, "notes", "note.md"), "# Note\n")
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	proj, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("second push (no changes): %v", err)
	}
	if proj.Facts["up_to_date"] != "true" {
		t.Fatalf("expected up_to_date=true, got %v (facts: %#v)", proj.Facts["up_to_date"], proj.Facts)
	}
	if proj.Facts["remote_checked"] != "true" {
		t.Fatalf("expected remote_checked=true, got %v", proj.Facts["remote_checked"])
	}
	if proj.Facts["remote_write"] != "false" {
		t.Fatalf("expected remote_write=false on up-to-date push, got %v", proj.Facts["remote_write"])
	}
}
