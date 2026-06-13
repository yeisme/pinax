package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type Capabilities = domain.VersionCapabilities

type Status = domain.VersionStatus

type Snapshot = domain.VersionSnapshot

type ChangedPath = domain.ChangedPath

type DiffSummary = domain.DiffSummary

type VersionedFile = domain.VersionedFile

type VersionBackend interface {
	Status(context.Context, StatusRequest) (Status, error)
	Snapshot(context.Context, SnapshotRequest) (Snapshot, error)
	ChangedSince(context.Context, ChangedSinceRequest) ([]ChangedPath, error)
	ReadFile(context.Context, ReadFileRequest) (VersionedFile, error)
	DiffSummary(context.Context, DiffSummaryRequest) (DiffSummary, error)
}

type StatusRequest struct {
	Root string
}

type SnapshotRequest struct {
	Root    string
	Message string
}

type ChangedSinceRequest struct {
	Root          string
	SinceRevision string
}

type ReadFileRequest struct {
	Root     string
	Path     string
	Revision string
}

type DiffSummaryRequest struct {
	Root           string
	BaseRevision   string
	TargetRevision string
}

type BackendInfo struct {
	Name         string       `json:"name"`
	Active       bool         `json:"active"`
	Description  string       `json:"description"`
	Capabilities Capabilities `json:"capabilities"`
}

type LocalBackend struct{}

func NewLocalBackend() LocalBackend {
	return LocalBackend{}
}

func AvailableBackends() []BackendInfo {
	return []BackendInfo{
		{Name: "local", Active: true, Description: "本地内容证据 backend", Capabilities: localCapabilities()},
		{Name: "none", Active: false, Description: "只报告无版本能力", Capabilities: noneCapabilities()},
	}
}

func (LocalBackend) Status(_ context.Context, req StatusRequest) (Status, error) {
	status := Status{Backend: "local", Capabilities: localCapabilities(), WorktreeState: "local"}
	latest, err := latestSnapshot(req.Root)
	if err != nil {
		return status, err
	}
	if latest != nil {
		status.LastSnapshotID = latest.SnapshotID
		status.LastSnapshotAt = latest.CreatedAt
		status.CurrentRevision = latest.SnapshotID
	}
	return status, nil
}

func (LocalBackend) Snapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	root := req.Root
	createdAt := time.Now().UTC().Format(time.RFC3339)
	files, bytes, contentHash, fileFacts, err := hashVault(ctx, root)
	if err != nil {
		return Snapshot{}, err
	}
	ledgerSeq, ledgerEvidence, err := readLedgerSeq(root)
	if err != nil {
		return Snapshot{}, err
	}
	indexEpoch, indexEvidence, err := readIndexEpoch(root)
	if err != nil {
		return Snapshot{}, err
	}
	snapshotID := "local-" + time.Now().UTC().Format("20060102T150405.000000000")
	rel := filepath.ToSlash(filepath.Join(".pinax", "version", "snapshots", snapshotID+".json"))
	evidence := []string{rel, filepath.ToSlash(filepath.Join(".pinax", "last_snapshot"))}
	if ledgerEvidence != "" {
		evidence = append(evidence, ledgerEvidence)
	}
	if indexEvidence != "" {
		evidence = append(evidence, indexEvidence)
	}
	snapshot := Snapshot{SnapshotID: snapshotID, Backend: "local", Message: req.Message, CreatedAt: createdAt, Files: files, Bytes: bytes, ContentHash: contentHash, LedgerSeq: ledgerSeq, IndexEpoch: indexEpoch, FileFacts: fileFacts, Evidence: evidence}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Snapshot{}, err
	}
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return Snapshot{}, err
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "last_snapshot"), []byte(createdAt+"\n"), 0o644); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (LocalBackend) ChangedSince(_ context.Context, req ChangedSinceRequest) ([]ChangedPath, error) {
	return nil, changedPathsUnavailableError("local", req.SinceRevision)
}

func (LocalBackend) ReadFile(_ context.Context, req ReadFileRequest) (VersionedFile, error) {
	return VersionedFile{}, readUnavailableError("local", req.Path, req.Revision)
}

func (LocalBackend) DiffSummary(_ context.Context, req DiffSummaryRequest) (DiffSummary, error) {
	return DiffSummary{}, readUnavailableError("local", "diff", req.BaseRevision+".."+req.TargetRevision)
}

type NoneBackend struct{}

func NewNoneBackend() NoneBackend {
	return NoneBackend{}
}

func (NoneBackend) Status(_ context.Context, _ StatusRequest) (Status, error) {
	return Status{Backend: "none", Capabilities: noneCapabilities(), WorktreeState: "unavailable"}, nil
}

func (NoneBackend) Snapshot(_ context.Context, _ SnapshotRequest) (Snapshot, error) {
	return Snapshot{}, &domain.CommandError{Code: domain.ErrorCodeVersionReadUnavailable, Message: "当前 version backend 不支持创建 snapshot", Hint: "切换到 local backend 后重试"}
}

func (NoneBackend) ChangedSince(_ context.Context, req ChangedSinceRequest) ([]ChangedPath, error) {
	return nil, changedPathsUnavailableError("none", req.SinceRevision)
}

func (NoneBackend) ReadFile(_ context.Context, req ReadFileRequest) (VersionedFile, error) {
	return VersionedFile{}, readUnavailableError("none", req.Path, req.Revision)
}

func (NoneBackend) DiffSummary(_ context.Context, req DiffSummaryRequest) (DiffSummary, error) {
	return DiffSummary{}, readUnavailableError("none", "diff", req.BaseRevision+".."+req.TargetRevision)
}

func changedPathsUnavailableError(backend, _ string) error {
	hint := "backend " + backend + " 不支持 changed-since；可运行 pinax version status 查看能力"
	return &domain.CommandError{Code: domain.ErrorCodeVersionChangedPathsUnavailable, Message: "当前 version backend 不支持 changed paths 查询", Hint: hint}
}

func readUnavailableError(backend, path, _ string) error {
	target := "vault file"
	if strings.TrimSpace(path) == "diff" {
		target = "diff"
	}
	hint := "backend " + backend + " 不支持读取历史 " + target + "；可运行 pinax version status 查看能力"
	return &domain.CommandError{Code: domain.ErrorCodeVersionReadUnavailable, Message: "当前 version backend 不支持历史读取", Hint: hint}
}

// GitBackend reserves the pure Go adapter boundary. It does not shell out to system git.
type GitBackend struct{}

func NewGitBackend() GitBackend {
	return GitBackend{}
}

func ProbeGitBackend(root string) BackendInfo {
	info := BackendInfo{Name: "git", Description: "预留的 pure Go Git backend adapter", Capabilities: noneCapabilities()}
	if stat, err := os.Stat(filepath.Join(root, ".git")); err == nil && (stat.IsDir() || stat.Mode().IsRegular()) {
		info.Active = true
	}
	return info
}

func (GitBackend) Status(_ context.Context, req StatusRequest) (Status, error) {
	probe := ProbeGitBackend(req.Root)
	worktreeState := "unavailable"
	if probe.Active {
		worktreeState = "available"
	}
	return Status{Backend: "git", Capabilities: probe.Capabilities, WorktreeState: worktreeState}, nil
}

func (GitBackend) Snapshot(_ context.Context, _ SnapshotRequest) (Snapshot, error) {
	return Snapshot{}, readUnavailableError("git", "snapshot", "")
}

func (GitBackend) ChangedSince(_ context.Context, req ChangedSinceRequest) ([]ChangedPath, error) {
	return nil, changedPathsUnavailableError("git", req.SinceRevision)
}

func (GitBackend) ReadFile(_ context.Context, req ReadFileRequest) (VersionedFile, error) {
	return VersionedFile{}, readUnavailableError("git", req.Path, req.Revision)
}

func (GitBackend) DiffSummary(_ context.Context, req DiffSummaryRequest) (DiffSummary, error) {
	return DiffSummary{}, readUnavailableError("git", "diff", req.BaseRevision+".."+req.TargetRevision)
}

func noneCapabilities() Capabilities {
	return Capabilities{}
}

func localCapabilities() Capabilities {
	return Capabilities{SnapshotSupported: true, ChangedPathsSupported: false, ReadAtRevision: false, DiffSupported: false}
}
func latestSnapshot(root string) (*Snapshot, error) {
	dir := filepath.Join(root, ".pinax", "version", "snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)
	payload, err := os.ReadFile(filepath.Join(dir, names[len(names)-1]))
	if err != nil {
		return nil, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func readLedgerSeq(root string) (uint64, string, error) {
	rel := filepath.ToSlash(filepath.Join(".pinax", "records", "version.json"))
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", nil
		}
		return 0, "", err
	}
	var version domain.LedgerVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return 0, "", err
	}
	return version.LastSeq, rel, nil
}

func readIndexEpoch(root string) (uint64, string, error) {
	rel := filepath.ToSlash(filepath.Join(".pinax", "records", "index-snapshot.json"))
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", nil
		}
		return 0, "", err
	}
	var snapshot domain.IndexSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return 0, "", err
	}
	return snapshot.IndexEpoch, rel, nil
}

func hashVault(ctx context.Context, root string) (int, int64, string, []ChangedPath, error) {
	type fileHash struct {
		path     string
		sum      string
		size     int64
		modified int64
	}
	files := []fileHash{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || rel == filepath.ToSlash(filepath.Join(".pinax", "version", "snapshots")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".pinax/version/snapshots/") || rel == ".pinax/last_snapshot" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		sum, size, err := hashFile(path)
		if err != nil {
			return err
		}
		files = append(files, fileHash{path: rel, sum: sum, size: size, modified: info.ModTime().Unix()})
		return nil
	}); err != nil {
		return 0, 0, "", nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	facts := make([]ChangedPath, 0, len(files))
	var total int64
	for _, file := range files {
		_, _ = io.WriteString(h, file.path)
		_, _ = io.WriteString(h, "\x00")
		_, _ = io.WriteString(h, file.sum)
		_, _ = io.WriteString(h, "\n")
		total += file.size
		facts = append(facts, ChangedPath{Path: file.path, ObjectKind: versionObjectKind(file.path), ContentHash: file.sum, SizeBytes: file.size, ModifiedUnix: file.modified})
	}
	return len(files), total, hex.EncodeToString(h.Sum(nil)), facts, nil
}

func versionObjectKind(rel string) domain.VaultObjectKind {
	lower := strings.ToLower(rel)
	if strings.HasSuffix(lower, ".md") {
		return domain.VaultObjectKindNote
	}
	if strings.HasPrefix(rel, "assets/") || strings.HasPrefix(rel, "attachments/") {
		return domain.VaultObjectKindAsset
	}
	return domain.VaultObjectKindFile
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	return hashReader(f)
}

func hashReader(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
