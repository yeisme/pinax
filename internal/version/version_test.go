package version

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestLocalBackendImplementsVersionBackendContract(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	var backend VersionBackend = NewLocalBackend()
	status, err := backend.Status(context.Background(), StatusRequest{Root: root})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Backend != "local" || !status.Capabilities.SnapshotSupported || status.Capabilities.ChangedPathsSupported || status.Capabilities.ReadAtRevision || status.Capabilities.DiffSupported {
		t.Fatalf("status = %#v", status)
	}

	snapshot, err := backend.Snapshot(context.Background(), SnapshotRequest{Root: root, Message: "checkpoint"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Backend != "local" || snapshot.SnapshotID == "" || snapshot.Files == 0 || snapshot.ContentHash == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.Evidence) == 0 || !strings.HasPrefix(snapshot.Evidence[0], ".pinax/version/snapshots/") {
		t.Fatalf("snapshot evidence = %#v", snapshot.Evidence)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(snapshot.Evidence[0]))); err != nil {
		t.Fatalf("snapshot evidence file missing: %v", err)
	}
}

func TestNoneBackendReportsNoHistoryCapabilities(t *testing.T) {
	var backend VersionBackend = NewNoneBackend()
	status, err := backend.Status(context.Background(), StatusRequest{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("none status: %v", err)
	}
	if status.Backend != "none" || status.WorktreeState != "unavailable" {
		t.Fatalf("none status identity = %#v", status)
	}
	if status.Capabilities.SnapshotSupported || status.Capabilities.ChangedPathsSupported || status.Capabilities.ReadAtRevision || status.Capabilities.DiffSupported {
		t.Fatalf("none capabilities = %#v", status.Capabilities)
	}
	if status.CurrentRevision != "" || status.LastSnapshotID != "" || status.LastSnapshotAt != "" {
		t.Fatalf("none status should not expose revision facts: %#v", status)
	}
}

func TestLocalBackendSnapshotRecordsLedgerIndexAndFileFacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "records"), 0o755); err != nil {
		t.Fatalf("mkdir records: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "records", "version.json"), []byte(`{"schema_version":"pinax.records.v1","last_seq":7,"updated_at":"2026-06-08T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write ledger version: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "records", "index-snapshot.json"), []byte(`{"schema_version":"pinax.index-snapshot.v1","snapshot_id":"idx_1","ledger_seq":7,"index_epoch":9,"created_at":"2026-06-08T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write index snapshot: %v", err)
	}

	snapshot, err := NewLocalBackend().Snapshot(context.Background(), SnapshotRequest{Root: root, Message: "evidence"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.LedgerSeq != 7 || snapshot.IndexEpoch != 9 {
		t.Fatalf("snapshot ledger/index facts = %#v", snapshot)
	}
	if len(snapshot.FileFacts) == 0 {
		t.Fatalf("snapshot missing file facts: %#v", snapshot)
	}
	var noteFact *ChangedPath
	for i := range snapshot.FileFacts {
		if snapshot.FileFacts[i].Path == "notes/a.md" {
			noteFact = &snapshot.FileFacts[i]
			break
		}
	}
	if noteFact == nil || noteFact.ContentHash == "" || noteFact.SizeBytes == 0 || noteFact.ModifiedUnix == 0 {
		t.Fatalf("note file fact = %#v", noteFact)
	}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(snapshot.Evidence[0])))
	if err != nil {
		t.Fatalf("read snapshot payload: %v", err)
	}
	for _, want := range []string{`"ledger_seq": 7`, `"index_epoch": 9`, `"file_facts"`, `"path": "notes/a.md"`, `"modified_unix"`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("snapshot payload missing %q:\n%s", want, payload)
		}
	}
}

func TestVersionBackendCapabilityGuardsReturnStableErrors(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	changed, err := NewLocalBackend().ChangedSince(ctx, ChangedSinceRequest{Root: root, SinceRevision: "local-0"})
	if err == nil || commandErrorCode(err) != "version_changed_paths_unavailable" {
		t.Fatalf("local changed-since error = %v, changed = %#v", err, changed)
	}
	file, err := NewLocalBackend().ReadFile(ctx, ReadFileRequest{Root: root, Path: "notes/a.md", Revision: "local-0"})
	if err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("local read-file error = %v, file = %#v", err, file)
	}
	diff, err := NewLocalBackend().DiffSummary(ctx, DiffSummaryRequest{Root: root, BaseRevision: "local-0", TargetRevision: "local-1"})
	if err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("local diff error = %v, diff = %#v", err, diff)
	}

	var backend VersionBackend = NewNoneBackend()
	if _, err := backend.Snapshot(ctx, SnapshotRequest{Root: root, Message: "checkpoint"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("none snapshot error = %v", err)
	}
	if _, err := backend.ChangedSince(ctx, ChangedSinceRequest{Root: root, SinceRevision: "local-0"}); err == nil || commandErrorCode(err) != "version_changed_paths_unavailable" {
		t.Fatalf("none changed-since error = %v", err)
	}
	if _, err := backend.ReadFile(ctx, ReadFileRequest{Root: root, Path: "notes/a.md", Revision: "local-0"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("none read-file error = %v", err)
	}
	if _, err := backend.DiffSummary(ctx, DiffSummaryRequest{Root: root, BaseRevision: "local-0", TargetRevision: "local-1"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("none diff error = %v", err)
	}
}

func TestGitBackendBoundaryAndCapabilityProbe(t *testing.T) {
	root := t.TempDir()
	probe := ProbeGitBackend(root)
	if probe.Name != "git" || probe.Active || probe.Capabilities.SnapshotSupported || probe.Capabilities.ChangedPathsSupported || probe.Capabilities.ReadAtRevision || probe.Capabilities.DiffSupported {
		t.Fatalf("git probe without repo = %#v", probe)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	probe = ProbeGitBackend(root)
	if probe.Name != "git" || !probe.Active || probe.Description == "" {
		t.Fatalf("git probe with repo = %#v", probe)
	}

	var backend VersionBackend = NewGitBackend()
	status, err := backend.Status(context.Background(), StatusRequest{Root: root})
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if status.Backend != "git" || status.WorktreeState != "available" {
		t.Fatalf("git status = %#v", status)
	}
	if _, err := backend.Snapshot(context.Background(), SnapshotRequest{Root: root, Message: "checkpoint"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("git snapshot error = %v", err)
	}
	if _, err := backend.ChangedSince(context.Background(), ChangedSinceRequest{Root: root, SinceRevision: "rev_0"}); err == nil || commandErrorCode(err) != "version_changed_paths_unavailable" {
		t.Fatalf("git changed-since error = %v", err)
	}
	if _, err := backend.ReadFile(context.Background(), ReadFileRequest{Root: root, Path: "notes/a.md", Revision: "rev_0"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("git read error = %v", err)
	}
	if _, err := backend.DiffSummary(context.Background(), DiffSummaryRequest{Root: root, BaseRevision: "rev_0", TargetRevision: "rev_1"}); err == nil || commandErrorCode(err) != "version_read_unavailable" {
		t.Fatalf("git diff error = %v", err)
	}
}

func TestHashReaderStreamsWithoutLargeSingleRead(t *testing.T) {
	const size = int64(2 << 20)
	reader := &boundedChunkReader{remaining: size, maxReadSize: 64 << 10}

	sum, gotSize, err := hashReader(reader)
	if err != nil {
		t.Fatalf("hash reader: %v", err)
	}
	if gotSize != size {
		t.Fatalf("size = %d, want %d", gotSize, size)
	}
	if sum != repeatedByteSHA256('x', size) {
		t.Fatalf("sum = %s", sum)
	}
	if reader.reads < 2 {
		t.Fatalf("reader was not streamed: reads=%d", reader.reads)
	}
}

type boundedChunkReader struct {
	remaining   int64
	maxReadSize int
	reads       int
}

func (r *boundedChunkReader) Read(p []byte) (int, error) {
	if len(p) > r.maxReadSize {
		return 0, fmt.Errorf("read buffer too large: %d", len(p))
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	r.reads++
	return len(p), nil
}

func repeatedByteSHA256(b byte, size int64) string {
	h := sha256.New()
	chunk := bytes.Repeat([]byte{b}, 32<<10)
	for size > 0 {
		write := int64(len(chunk))
		if write > size {
			write = size
		}
		_, _ = h.Write(chunk[:write])
		size -= write
	}
	return hex.EncodeToString(h.Sum(nil))
}

func commandErrorCode(err error) string {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Code
	}
	return ""
}
