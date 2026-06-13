package versiontest

import (
	"context"

	pinaxversion "github.com/yeisme/pinax/internal/version"
)

// FakeBackend is a reusable test double for app, index, record, and repair tests.
type FakeBackend struct {
	StatusResult        pinaxversion.Status
	StatusErr           error
	LastStatusRequest   pinaxversion.StatusRequest
	SnapshotResult      pinaxversion.Snapshot
	SnapshotErr         error
	LastSnapshotRequest pinaxversion.SnapshotRequest

	ChangedSinceResult      []pinaxversion.ChangedPath
	ChangedSinceErr         error
	LastChangedSinceRequest pinaxversion.ChangedSinceRequest

	ReadFileResult      pinaxversion.VersionedFile
	ReadFileErr         error
	LastReadFileRequest pinaxversion.ReadFileRequest

	DiffSummaryResult      pinaxversion.DiffSummary
	DiffSummaryErr         error
	LastDiffSummaryRequest pinaxversion.DiffSummaryRequest
}

func (f *FakeBackend) Status(_ context.Context, req pinaxversion.StatusRequest) (pinaxversion.Status, error) {
	f.LastStatusRequest = req
	return f.StatusResult, f.StatusErr
}

func (f *FakeBackend) Snapshot(_ context.Context, req pinaxversion.SnapshotRequest) (pinaxversion.Snapshot, error) {
	f.LastSnapshotRequest = req
	return f.SnapshotResult, f.SnapshotErr
}

func (f *FakeBackend) ChangedSince(_ context.Context, req pinaxversion.ChangedSinceRequest) ([]pinaxversion.ChangedPath, error) {
	f.LastChangedSinceRequest = req
	return append([]pinaxversion.ChangedPath(nil), f.ChangedSinceResult...), f.ChangedSinceErr
}

func (f *FakeBackend) ReadFile(_ context.Context, req pinaxversion.ReadFileRequest) (pinaxversion.VersionedFile, error) {
	f.LastReadFileRequest = req
	return f.ReadFileResult, f.ReadFileErr
}

func (f *FakeBackend) DiffSummary(_ context.Context, req pinaxversion.DiffSummaryRequest) (pinaxversion.DiffSummary, error) {
	f.LastDiffSummaryRequest = req
	return f.DiffSummaryResult, f.DiffSummaryErr
}
