package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/cloudclient"
	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/remote"
)

func rebaseManifestEntry(path, blobID string) remote.ManifestEntry {
	return remote.ManifestEntry{Path: path, PathHash: remote.PathHash(path), BlobID: blobID, SHA256: blobID, Size: int64(len(blobID))}
}

func rebaseManifest(entries ...remote.ManifestEntry) remote.Manifest {
	return remote.Manifest{SchemaVersion: remote.ManifestSchemaVersion, Entries: entries, EntryCount: len(entries)}
}

// TestPushAutoRebaseSuccess exercises the happy path: a stale base causes a
// revision conflict, the pull finds no content conflict, and the retry commit
// succeeds against the freshly pulled revision.
func TestPushAutoRebaseSuccess(t *testing.T) {
	base := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))
	local := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"), rebaseManifestEntry("notes/local.md", "blob_local"))
	pulled := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))

	var commits []string
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		commits = append(commits, baseRevision)
		if len(commits) == 1 {
			return cloudsync.CommitResult{}, cloudsync.ErrRevisionConflict
		}
		return cloudsync.CommitResult{RevisionID: "rev_rebased", RemoteWrite: true, ManifestBlobID: "manifest_rebased"}, nil
	}
	pull := func() (cloudRemoteSnapshot, error) {
		return cloudRemoteSnapshot{Manifest: pulled, RevisionID: "rev_remote", ManifestBlobID: "manifest_remote"}, nil
	}

	result, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, localManifest: local, baseManifest: base, baseRevision: "rev_stale", yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Rebased {
		t.Fatal("expected the commit to succeed on the rebase retry")
	}
	if result.Commit.RevisionID != "rev_rebased" {
		t.Fatalf("commit revision = %q, want rev_rebased", result.Commit.RevisionID)
	}
	if len(commits) != 2 {
		t.Fatalf("expected exactly one retry, commits=%v", commits)
	}
	if commits[0] != "rev_stale" || commits[1] != "rev_remote" {
		t.Fatalf("retry must commit against the pulled revision, commits=%v", commits)
	}
}

// TestPushAutoRebaseConflict exercises the conflict path: after pulling the
// remote head, the rebuilt plan detects a content conflict, so no retry happens
// and the conflict operations are returned for the caller to project.
func TestPushAutoRebaseConflict(t *testing.T) {
	base := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_base"))
	local := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_local"))
	pulled := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_remote"))

	commitCalls := 0
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		commitCalls++
		return cloudsync.CommitResult{}, cloudsync.ErrRevisionConflict
	}
	pull := func() (cloudRemoteSnapshot, error) {
		return cloudRemoteSnapshot{Manifest: pulled, RevisionID: "rev_remote"}, nil
	}

	result, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, localManifest: local, baseManifest: base, baseRevision: "rev_stale", yes: true})
	if err != nil {
		t.Fatalf("content conflict must be returned as conflicts, not an error: %v", err)
	}
	if len(result.Conflicts) == 0 {
		t.Fatal("expected at least one content conflict after rebase")
	}
	if result.Conflicts[0].Path != "notes/shared.md" {
		t.Fatalf("conflict path = %q, want notes/shared.md", result.Conflicts[0].Path)
	}
	if commitCalls != 1 {
		t.Fatalf("must not retry commit when content conflicts exist, calls=%d", commitCalls)
	}
}

// TestPushAutoRebaseRetryExhausted exercises the no-loop guarantee: the retry
// commit also fails with a revision conflict, so the ORIGINAL conflict error is
// surfaced and exactly one retry was attempted.
func TestPushAutoRebaseRetryExhausted(t *testing.T) {
	base := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))
	local := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"), rebaseManifestEntry("notes/local.md", "blob_local"))
	pulled := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))

	var commits []string
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		commits = append(commits, baseRevision)
		return cloudsync.CommitResult{}, cloudsync.ErrRevisionConflict
	}
	pull := func() (cloudRemoteSnapshot, error) {
		return cloudRemoteSnapshot{Manifest: pulled, RevisionID: "rev_remote"}, nil
	}

	result, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, localManifest: local, baseManifest: base, baseRevision: "rev_stale", yes: true})
	if err == nil {
		t.Fatal("expected the original revision conflict error after an exhausted retry")
	}
	if !isCloudRevisionConflict(err) {
		t.Fatalf("expected a revision conflict error, got %v", err)
	}
	if result.Rebased {
		t.Fatal("must not report rebase success on an exhausted retry")
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("must not report conflicts on an exhausted retry, got %d", len(result.Conflicts))
	}
	if len(commits) != 2 {
		t.Fatalf("expected exactly one retry, commits=%v", commits)
	}
}

// TestPushAutoRebaseNoYesSurfacesConflict ensures auto-rebase only runs with
// --yes: without it, the revision conflict error is returned directly and the
// remote is never pulled.
func TestPushAutoRebaseNoYesSurfacesConflict(t *testing.T) {
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		return cloudsync.CommitResult{}, cloudsync.ErrRevisionConflict
	}
	pullCalls := 0
	pull := func() (cloudRemoteSnapshot, error) {
		pullCalls++
		return cloudRemoteSnapshot{}, nil
	}

	_, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, baseRevision: "rev_stale", yes: false})
	if !isCloudRevisionConflict(err) {
		t.Fatalf("expected revision conflict surfaced directly without --yes, got %v", err)
	}
	if pullCalls != 0 {
		t.Fatalf("must not pull remote without --yes, pulls=%d", pullCalls)
	}
}

// TestPushAutoRebasePullFailureSurfacesOriginalConflict ensures a failed pull
// does not swallow the original revision conflict.
func TestPushAutoRebasePullFailureSurfacesOriginalConflict(t *testing.T) {
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		return cloudsync.CommitResult{}, cloudsync.ErrRevisionConflict
	}
	pull := func() (cloudRemoteSnapshot, error) {
		return cloudRemoteSnapshot{}, errors.New("transport_unavailable")
	}

	_, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, baseRevision: "rev_stale", yes: true})
	if !isCloudRevisionConflict(err) {
		t.Fatalf("expected original conflict surfaced on pull failure, got %v", err)
	}
}

// TestPushAutoRebaseNonConflictErrorUnchanged ensures a non-conflict commit
// error is propagated verbatim without triggering a rebase.
func TestPushAutoRebaseNonConflictErrorUnchanged(t *testing.T) {
	other := errors.New("transport_unavailable")
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		return cloudsync.CommitResult{}, other
	}
	pullCalls := 0
	pull := func() (cloudRemoteSnapshot, error) {
		pullCalls++
		return cloudRemoteSnapshot{}, nil
	}

	_, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, baseRevision: "rev_stale", yes: true})
	if !errors.Is(err, other) {
		t.Fatalf("expected the non-conflict error propagated, got %v", err)
	}
	if pullCalls != 0 {
		t.Fatalf("must not pull on a non-conflict error, pulls=%d", pullCalls)
	}
}

// TestPushAutoRebaseServerConflictError verifies the server-transport flavor of
// revision conflict (cloudclient REVISION_CONFLICT) also triggers auto-rebase,
// so the fix covers both object-store and Pinax Cloud transports.
func TestPushAutoRebaseServerConflictError(t *testing.T) {
	base := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))
	local := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"), rebaseManifestEntry("notes/local.md", "blob_local"))
	pulled := rebaseManifest(rebaseManifestEntry("notes/shared.md", "blob_shared"))

	calls := 0
	commit := func(baseRevision string) (cloudsync.CommitResult, error) {
		calls++
		if calls == 1 {
			return cloudsync.CommitResult{}, &cloudclient.Error{Code: cloudclient.CodeRevisionConflict, Message: "base revision mismatch", Retryable: true}
		}
		return cloudsync.CommitResult{RevisionID: "rev_ok", RemoteWrite: true}, nil
	}
	pull := func() (cloudRemoteSnapshot, error) {
		return cloudRemoteSnapshot{Manifest: pulled, RevisionID: "rev_remote"}, nil
	}

	result, err := runCloudPushRebase(cloudRebasePlan{commit: commit, pull: pull, localManifest: local, baseManifest: base, baseRevision: "rev_stale", yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Rebased {
		t.Fatal("expected rebase retry for server-transport conflict")
	}
}

// cloudPushAutoRebaseIntegrationFixture wires two devices against a shared
// file:// object store and is shared by the success/conflict integration tests.
func cloudPushAutoRebaseIntegrationFixture(t *testing.T) (svc *Service, deviceA, deviceB string) {
	t.Helper()
	ctx := context.Background()
	store := t.TempDir()
	deviceA = filepath.Join(t.TempDir(), "device-a")
	deviceB = filepath.Join(t.TempDir(), "device-b")
	svc = NewService()
	for _, root := range []string{deviceA, deviceB} {
		if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
			t.Fatalf("init %s: %v", root, err)
		}
	}
	for _, req := range []CloudLoginRequest{
		{VaultPath: deviceA, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "laptop", SecretRef: "test-secret"},
		{VaultPath: deviceB, Endpoint: "file://" + store, WorkspaceID: "ws", DeviceID: "desktop", SecretRef: "test-secret"},
	} {
		if _, err := svc.CloudLogin(ctx, req); err != nil {
			t.Fatalf("cloud login: %v", err)
		}
	}
	return svc, deviceA, deviceB
}

// TestPushAutoRebaseIntegrationSuccess proves the real end-to-end wiring: with
// a stale cached base, a push --yes pulls the advanced remote head, finds no
// content conflict, and retries the commit successfully.
func TestPushAutoRebaseIntegrationSuccess(t *testing.T) {
	ctx := context.Background()
	svc, deviceA, deviceB := cloudPushAutoRebaseIntegrationFixture(t)

	writeFile(t, filepath.Join(deviceA, "notes", "shared.md"), "# shared\noriginal\n")
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device A push rev_1: %v", err)
	}
	if _, err := svc.SyncPull(ctx, SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device B pull rev_1: %v", err)
	}
	// Advance the remote head with a disjoint change.
	writeFile(t, filepath.Join(deviceA, "notes", "only-a.md"), "# only A\n")
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device A push rev_2: %v", err)
	}
	// B's cached base is now stale; a disjoint local change must auto-rebase.
	writeFile(t, filepath.Join(deviceB, "notes", "only-b.md"), "# only B\n")
	proj, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("device B push auto-rebase: %v", err)
	}
	if proj.Facts["revision_id"] == "" {
		t.Fatalf("expected a committed revision_id after rebase, facts=%#v", proj.Facts)
	}
}

// TestPushAutoRebaseIntegrationConflict proves the real end-to-end conflict
// path: both devices modify the same file, B's stale-base push auto-rebases,
// detects the content conflict, and surfaces conflict_required.
func TestPushAutoRebaseIntegrationConflict(t *testing.T) {
	ctx := context.Background()
	svc, deviceA, deviceB := cloudPushAutoRebaseIntegrationFixture(t)

	writeFile(t, filepath.Join(deviceA, "notes", "contended.md"), "# contended\noriginal\n")
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device A push rev_1: %v", err)
	}
	if _, err := svc.SyncPull(ctx, SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device B pull rev_1: %v", err)
	}
	// A modifies the shared file and advances the remote head.
	writeFile(t, filepath.Join(deviceA, "notes", "contended.md"), "# contended\nchanged by A\n")
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatalf("device A push rev_2: %v", err)
	}
	// B independently modifies the SAME file; the stale-base push must surface a
	// content conflict after the auto-rebase pull rather than clobbering A.
	writeFile(t, filepath.Join(deviceB, "notes", "contended.md"), "# contended\nchanged by B\n")
	proj, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if !hasCommandCode(err, "conflict_required") {
		t.Fatalf("expected conflict_required after rebase, err=%v proj=%#v", err, proj)
	}
}
