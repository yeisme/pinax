package versiontest

import (
	"context"
	"errors"
	"testing"

	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func TestFakeBackendImplementsVersionBackendAndCapturesRequests(t *testing.T) {
	wantErr := errors.New("boom")
	fake := &FakeBackend{
		StatusResult:       pinaxversion.Status{Backend: "fake", WorktreeState: "clean"},
		SnapshotResult:     pinaxversion.Snapshot{SnapshotID: "snap_1", Backend: "fake"},
		ChangedSinceResult: []pinaxversion.ChangedPath{{Path: "notes/a.md", ChangeKind: "modified"}},
		ReadFileResult:     pinaxversion.VersionedFile{Path: "notes/a.md", Revision: "rev_1", Backend: "fake", Content: "# A\n"},
		DiffSummaryResult:  pinaxversion.DiffSummary{BaseRevision: "rev_0", TargetRevision: "rev_1", FilesChanged: 1},
	}
	var backend pinaxversion.VersionBackend = fake

	if status, err := backend.Status(context.Background(), pinaxversion.StatusRequest{Root: "/vault"}); err != nil || status.Backend != "fake" {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	if fake.LastStatusRequest.Root != "/vault" {
		t.Fatalf("status request = %#v", fake.LastStatusRequest)
	}
	if snapshot, err := backend.Snapshot(context.Background(), pinaxversion.SnapshotRequest{Root: "/vault", Message: "checkpoint"}); err != nil || snapshot.SnapshotID != "snap_1" {
		t.Fatalf("snapshot = %#v, err = %v", snapshot, err)
	}
	if fake.LastSnapshotRequest.Message != "checkpoint" {
		t.Fatalf("snapshot request = %#v", fake.LastSnapshotRequest)
	}
	if changed, err := backend.ChangedSince(context.Background(), pinaxversion.ChangedSinceRequest{Root: "/vault", SinceRevision: "rev_0"}); err != nil || len(changed) != 1 || changed[0].Path != "notes/a.md" {
		t.Fatalf("changed = %#v, err = %v", changed, err)
	}
	if fake.LastChangedSinceRequest.SinceRevision != "rev_0" {
		t.Fatalf("changed request = %#v", fake.LastChangedSinceRequest)
	}
	if file, err := backend.ReadFile(context.Background(), pinaxversion.ReadFileRequest{Root: "/vault", Path: "notes/a.md", Revision: "rev_1"}); err != nil || file.Content != "# A\n" {
		t.Fatalf("file = %#v, err = %v", file, err)
	}
	if fake.LastReadFileRequest.Path != "notes/a.md" {
		t.Fatalf("read request = %#v", fake.LastReadFileRequest)
	}
	if diff, err := backend.DiffSummary(context.Background(), pinaxversion.DiffSummaryRequest{Root: "/vault", BaseRevision: "rev_0", TargetRevision: "rev_1"}); err != nil || diff.FilesChanged != 1 {
		t.Fatalf("diff = %#v, err = %v", diff, err)
	}
	if fake.LastDiffSummaryRequest.TargetRevision != "rev_1" {
		t.Fatalf("diff request = %#v", fake.LastDiffSummaryRequest)
	}

	fake.ReadFileErr = wantErr
	if _, err := backend.ReadFile(context.Background(), pinaxversion.ReadFileRequest{Root: "/vault", Path: "notes/b.md", Revision: "rev_2"}); !errors.Is(err, wantErr) {
		t.Fatalf("read injected err = %v", err)
	}
}
