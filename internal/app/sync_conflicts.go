package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type SyncConflictsListRequest struct {
	VaultPath string
}

type SyncConflictFileRequest struct {
	VaultPath string
	File      string
}

type SyncConflictResolveRequest struct {
	VaultPath  string
	File       string
	KeepLocal  bool
	KeepRemote bool
	MergedPath string
	Yes        bool
}

func (s *Service) SyncConflictsList(_ context.Context, req SyncConflictsListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.conflicts.list", err), err
	}
	conflicts, err := scanSyncConflicts(root)
	if err != nil {
		return errorProjection("sync.conflicts.list", err), err
	}
	projection := domain.NewProjection("sync.conflicts.list", "Sync conflict queue scanned.")
	projection.Facts["conflicts"] = fmt.Sprint(len(conflicts))
	addSyncConflictFacts(&projection, conflicts)
	projection.Data = map[string]any{"conflicts": conflicts}
	projection.Actions = syncConflictActions(root, conflicts)
	return projection, nil
}

func (s *Service) SyncConflictsShow(_ context.Context, req SyncConflictFileRequest) (domain.Projection, error) {
	return s.syncConflictInspect("sync.conflicts.show", req, true)
}

func (s *Service) SyncConflictsDiff(_ context.Context, req SyncConflictFileRequest) (domain.Projection, error) {
	return s.syncConflictInspect("sync.conflicts.diff", req, false)
}

func (s *Service) syncConflictInspect(command string, req SyncConflictFileRequest, includeBodies bool) (domain.Projection, error) {
	root, entry, err := resolveSyncConflictEntry(req.VaultPath, req.File)
	if err != nil {
		return errorProjection(command, err), err
	}
	mainBody, conflictBody, err := readSyncConflictBodies(root, entry)
	if err != nil {
		return errorProjection(command, err), err
	}
	detail := domain.SyncConflictDetail{Conflict: entry, Diff: buildSyncConflictDiff(entry.MainPath, entry.File, mainBody, conflictBody)}
	if includeBodies {
		detail.MainBody = string(mainBody)
		detail.Body = string(conflictBody)
	}
	projection := domain.NewProjection(command, "Sync conflict inspected.")
	projection.Facts["conflict_file"] = entry.File
	projection.Facts["main_path"] = entry.MainPath
	projection.Facts["conflicts"] = "1"
	projection.Data = map[string]any{"conflict": detail.Conflict, "diff": detail.Diff}
	if includeBodies {
		projection.Data = map[string]any{"conflict": detail.Conflict, "main_body": detail.MainBody, "body": detail.Body}
	}
	actions := syncConflictActions(root, []domain.SyncConflictEntry{entry})
	if includeBodies && len(actions) == 3 {
		projection.Actions = []domain.Action{actions[1], actions[2], actions[0]}
	} else {
		projection.Actions = actions
	}
	return projection, nil
}

func (s *Service) SyncConflictsResolve(_ context.Context, req SyncConflictResolveRequest) (domain.Projection, error) {
	root, entry, err := resolveSyncConflictEntry(req.VaultPath, req.File)
	if err != nil {
		return errorProjection("sync.conflicts.resolve", err), err
	}
	if !req.Yes {
		commandErr := &domain.CommandError{Code: "approval_required", Message: "sync conflict resolve requires --yes", Hint: "Review pinax sync conflicts diff " + shellQuote(entry.File) + " first, then rerun resolve with --yes"}
		projection := domain.NewErrorProjection("sync.conflicts.resolve", commandErr)
		projection.Facts["conflict_file"] = entry.File
		projection.Facts["main_path"] = entry.MainPath
		projection.Actions = syncConflictActions(root, []domain.SyncConflictEntry{entry})
		return projection, commandErr
	}
	resolution, strategyErr := syncConflictResolution(req)
	if strategyErr != nil {
		projection := domain.NewErrorProjection("sync.conflicts.resolve", strategyErr)
		projection.Facts["conflict_file"] = entry.File
		projection.Facts["main_path"] = entry.MainPath
		projection.Actions = syncConflictActions(root, []domain.SyncConflictEntry{entry})
		return projection, strategyErr
	}
	conflictPath, err := safeJoin(root, entry.File)
	if err != nil {
		return errorProjection("sync.conflicts.resolve", err), err
	}
	mainPath, err := safeJoin(root, entry.MainPath)
	if err != nil {
		return errorProjection("sync.conflicts.resolve", err), err
	}
	switch resolution {
	case "keep_local":
		if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
		if err := os.Rename(conflictPath, mainPath); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
	case "keep_remote":
		if err := os.Remove(conflictPath); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
	case "merged":
		mergedContent, err := os.ReadFile(req.MergedPath)
		if err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
		if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
		if err := os.WriteFile(mainPath, mergedContent, 0o644); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
		if err := os.Remove(conflictPath); err != nil {
			return errorProjection("sync.conflicts.resolve", err), err
		}
	}
	receipt, err := writeSyncConflictResolutionReceipt(root, entry, resolution)
	if err != nil {
		return errorProjection("sync.conflicts.resolve", err), err
	}
	_ = appendEvent(root, "sync.conflict.resolve", "success", map[string]string{"conflict_file": entry.File, "main_path": entry.MainPath, "resolution": resolution, "receipt_path": receipt.ReceiptPath})
	projection := domain.NewProjection("sync.conflicts.resolve", "Sync conflict resolved.")
	projection.Facts["conflict_file"] = entry.File
	projection.Facts["main_path"] = entry.MainPath
	projection.Facts["resolution"] = resolution
	projection.Facts["resolved"] = "true"
	projection.Facts["receipt_path"] = receipt.ReceiptPath
	projection.Evidence = []string{receipt.ReceiptPath, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"receipt": receipt}
	projection.Actions = []domain.Action{{Name: "list", Command: fmt.Sprintf("pinax sync conflicts list --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func scanSyncConflicts(root string) ([]domain.SyncConflictEntry, error) {
	conflicts := []domain.SyncConflictEntry{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".conflict.md") {
			return nil
		}
		mainRel, err := mainPathForSyncConflict(rel)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		conflicts = append(conflicts, domain.SyncConflictEntry{File: rel, MainPath: mainRel, Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].File < conflicts[j].File })
	return conflicts, nil
}

func resolveSyncConflictEntry(vaultPath, file string) (string, domain.SyncConflictEntry, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.SyncConflictEntry{}, err
	}
	rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(file)))
	if rel == "" || rel == "." {
		return "", domain.SyncConflictEntry{}, &domain.CommandError{Code: "conflict_file_required", Message: "sync conflict file is required", Hint: "Run pinax sync conflicts list --vault " + shellQuote(root) + " --json"}
	}
	if !strings.HasSuffix(rel, ".conflict.md") {
		return "", domain.SyncConflictEntry{}, &domain.CommandError{Code: "invalid_conflict_file", Message: "sync conflict file must end with .conflict.md", Hint: "Run pinax sync conflicts list --vault " + shellQuote(root) + " --json"}
	}
	mainRel, err := mainPathForSyncConflict(rel)
	if err != nil {
		return "", domain.SyncConflictEntry{}, err
	}
	conflictPath, err := safeJoin(root, rel)
	if err != nil {
		return "", domain.SyncConflictEntry{}, err
	}
	info, err := os.Stat(conflictPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", domain.SyncConflictEntry{}, &domain.CommandError{Code: "conflict_not_found", Message: "sync conflict file not found", Hint: "Run pinax sync conflicts list --vault " + shellQuote(root) + " --json"}
		}
		return "", domain.SyncConflictEntry{}, err
	}
	if info.IsDir() {
		return "", domain.SyncConflictEntry{}, &domain.CommandError{Code: "invalid_conflict_file", Message: "sync conflict path is a directory", Hint: "Run pinax sync conflicts list --vault " + shellQuote(root) + " --json"}
	}
	return root, domain.SyncConflictEntry{File: rel, MainPath: mainRel, Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339)}, nil
}

func mainPathForSyncConflict(rel string) (string, error) {
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if rel == "" || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "Path escapes the vault boundary"}
	}
	if !strings.HasSuffix(rel, ".conflict.md") {
		return "", &domain.CommandError{Code: "invalid_conflict_file", Message: "sync conflict file must end with .conflict.md"}
	}
	stem := strings.TrimSuffix(rel, ".conflict.md")
	idx := strings.LastIndex(stem, ".")
	if idx <= strings.LastIndex(stem, "/") {
		return "", &domain.CommandError{Code: "invalid_conflict_file", Message: "sync conflict file is missing the timestamp segment"}
	}
	return stem[:idx] + ".md", nil
}

func readSyncConflictBodies(root string, entry domain.SyncConflictEntry) ([]byte, []byte, error) {
	conflictPath, err := safeJoin(root, entry.File)
	if err != nil {
		return nil, nil, err
	}
	mainPath, err := safeJoin(root, entry.MainPath)
	if err != nil {
		return nil, nil, err
	}
	conflictBody, err := os.ReadFile(conflictPath)
	if err != nil {
		return nil, nil, err
	}
	mainBody, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	return mainBody, conflictBody, nil
}

func syncConflictResolution(req SyncConflictResolveRequest) (string, *domain.CommandError) {
	strategies := 0
	resolution := ""
	if req.KeepLocal {
		strategies++
		resolution = "keep_local"
	}
	if req.KeepRemote {
		strategies++
		resolution = "keep_remote"
	}
	if strings.TrimSpace(req.MergedPath) != "" {
		strategies++
		resolution = "merged"
	}
	if strategies == 0 {
		return "", &domain.CommandError{Code: "conflict_resolution_required", Message: "choose exactly one conflict resolution strategy", Hint: "Use --keep-local, --keep-remote, or --merged <file> with --yes"}
	}
	if strategies > 1 {
		return "", &domain.CommandError{Code: "conflict_resolution_conflict", Message: "only one conflict resolution strategy is allowed", Hint: "Choose one of --keep-local, --keep-remote, or --merged <file>"}
	}
	return resolution, nil
}

func writeSyncConflictResolutionReceipt(root string, entry domain.SyncConflictEntry, resolution string) (domain.SyncConflictResolutionReceipt, error) {
	rel := filepath.ToSlash(filepath.Join(".pinax", "sync-conflicts", "receipts", time.Now().UTC().Format("20060102150405.000000000")+".json"))
	receipt := domain.SyncConflictResolutionReceipt{SchemaVersion: "pinax.sync_conflict_resolution.v1", Command: "sync.conflicts.resolve", Status: "resolved", ConflictFile: entry.File, MainPath: entry.MainPath, Resolution: resolution, ReceiptPath: rel, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	return receipt, writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), receipt)
}

func buildSyncConflictDiff(mainRel, conflictRel string, mainBody, conflictBody []byte) string {
	mainLines := strings.Split(strings.TrimRight(string(mainBody), "\n"), "\n")
	conflictLines := strings.Split(strings.TrimRight(string(conflictBody), "\n"), "\n")
	if len(mainLines) == 1 && mainLines[0] == "" {
		mainLines = nil
	}
	if len(conflictLines) == 1 && conflictLines[0] == "" {
		conflictLines = nil
	}
	var b strings.Builder
	b.WriteString("--- ")
	b.WriteString(mainRel)
	b.WriteByte('\n')
	b.WriteString("+++ ")
	b.WriteString(conflictRel)
	b.WriteByte('\n')
	maxLines := len(mainLines)
	if len(conflictLines) > maxLines {
		maxLines = len(conflictLines)
	}
	for i := 0; i < maxLines; i++ {
		var left, right string
		if i < len(mainLines) {
			left = mainLines[i]
		}
		if i < len(conflictLines) {
			right = conflictLines[i]
		}
		switch {
		case i >= len(mainLines):
			b.WriteString("+ ")
			b.WriteString(right)
			b.WriteByte('\n')
		case i >= len(conflictLines):
			b.WriteString("- ")
			b.WriteString(left)
			b.WriteByte('\n')
		case left == right:
			b.WriteString("  ")
			b.WriteString(left)
			b.WriteByte('\n')
		default:
			b.WriteString("- ")
			b.WriteString(left)
			b.WriteByte('\n')
			b.WriteString("+ ")
			b.WriteString(right)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func addSyncConflictFacts(projection *domain.Projection, conflicts []domain.SyncConflictEntry) {
	for i, conflict := range conflicts {
		prefix := fmt.Sprintf("conflict.%d.", i+1)
		projection.Facts[prefix+"file"] = conflict.File
		projection.Facts[prefix+"main_path"] = conflict.MainPath
	}
}

func syncConflictActions(root string, conflicts []domain.SyncConflictEntry) []domain.Action {
	actions := []domain.Action{{Name: "list", Command: fmt.Sprintf("pinax sync conflicts list --vault %s --json", shellQuote(root))}}
	file := "<file>"
	if len(conflicts) > 0 && strings.TrimSpace(conflicts[0].File) != "" {
		file = conflicts[0].File
	}
	diffFile := shellQuote(file)
	resolveFile := shellQuote(file)
	if file == "<file>" {
		diffFile = file
		resolveFile = file
	}
	actions = append(actions,
		domain.Action{Name: "diff", Command: fmt.Sprintf("pinax sync conflicts diff %s --vault %s", diffFile, shellQuote(root))},
		domain.Action{Name: "resolve", Command: fmt.Sprintf("pinax sync conflicts resolve %s --keep-remote --vault %s --yes", resolveFile, shellQuote(root))},
	)
	return actions
}
