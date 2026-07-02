package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const folderRegistrySchemaVersion = "pinax.folders.v1"

type FolderListRequest struct {
	VaultPath    string
	Purpose      string
	Under        string
	IncludeEmpty bool
	Depth        int
}

type FolderRequest struct {
	VaultPath string
	Path      string
}

type FolderOperationRequest struct {
	VaultPath       string
	Path            string
	TargetPath      string
	TargetParent    string
	Purpose         string
	DryRun          bool
	Yes             bool
	EmptyOnly       bool
	RequireSnapshot bool
}

type FolderRepairRequest struct {
	VaultPath string
	Plan      bool
}

func (s *Service) ListFolders(_ context.Context, req FolderListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.list", err), err
	}
	purpose, purposeErr := normalizeFolderListPurpose(req.Purpose)
	if purposeErr != nil {
		return domain.NewErrorProjection("folder.list", purposeErr), purposeErr
	}
	under := ""
	if strings.TrimSpace(req.Under) != "" {
		var pathErr *domain.CommandError
		under, pathErr = validateVaultFolderPath(req.Under)
		if pathErr != nil {
			return domain.NewErrorProjection("folder.list", pathErr), pathErr
		}
	}
	collectDepth := req.Depth
	if under != "" {
		collectDepth = 0
	}
	folders, err := collectFolders(root, req.IncludeEmpty, collectDepth)
	if err != nil {
		return errorProjection("folder.list", err), err
	}
	filtered := make([]domain.FolderInfo, 0, len(folders))
	for _, folder := range folders {
		if folderPurposeMatches(purpose, folder.Purpose) && folderUnderPathMatches(folder.Path, under, req.Depth) {
			filtered = append(filtered, folder)
		}
	}
	projection := domain.NewProjection("folder.list", "Folders listed.")
	projection.Facts["folders"] = fmt.Sprint(len(filtered))
	projection.Facts["filter.purpose"] = purpose
	projection.Facts["include_empty"] = fmt.Sprint(req.IncludeEmpty)
	if under != "" {
		projection.Facts["filter.under"] = under
	}
	if req.Depth > 0 {
		projection.Facts["depth"] = fmt.Sprint(req.Depth)
	}
	projection.Data = map[string]any{"folders": filtered, "purpose": purpose, "under": under, "include_empty": req.IncludeEmpty}
	return projection, nil
}

func (s *Service) ShowFolder(_ context.Context, req FolderRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.show", err), err
	}
	folderPath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.show", pathErr), pathErr
	}
	folders, err := collectFolders(root, true, 0)
	if err != nil {
		return errorProjection("folder.show", err), err
	}
	for _, folder := range folders {
		if folder.Path == folderPath {
			children := immediateChildFolders(folders, folderPath)
			descendants := descendantFolderCount(folders, folderPath)
			projection := domain.NewProjection("folder.show", "Folder details read.")
			setFolderFacts(projection.Facts, folder)
			projection.Facts["child_folders"] = fmt.Sprint(len(children))
			projection.Facts["descendant_folders"] = fmt.Sprint(descendants)
			projection.Data = map[string]any{"folder": folder, "children": children, "child_count": len(children), "descendant_count": descendants}
			return projection, nil
		}
	}
	err = &domain.CommandError{Code: "folder_not_found", Message: "Folder not found", Hint: "Run pinax folder list --include-empty --vault <vault> to view folders"}
	cmdErr := err.(*domain.CommandError)
	return domain.NewErrorProjection("folder.show", cmdErr), err
}

func (s *Service) CreateFolder(_ context.Context, req FolderOperationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.create", err), err
	}
	folderPath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.create", pathErr), pathErr
	}
	purpose, purposeErr := normalizeFolderPurpose(req.Purpose, domain.FolderPurposeGeneric)
	if purposeErr != nil {
		return domain.NewErrorProjection("folder.create", purposeErr), purposeErr
	}
	projection := domain.NewProjection("folder.create", "Folder create plan generated.")
	projection.Facts["folder_path"] = folderPath
	projection.Facts["purpose"] = string(purpose)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Facts["managed_status"] = string(domain.ManagedStatusManaged)
	projection.Data = map[string]any{"plan": domain.FolderOperationPlan{Operation: "create", Path: folderPath, Purpose: purpose, DryRun: req.DryRun, Writes: !req.DryRun, Effects: []domain.PlanOperation{{Kind: "mkdir", Path: folderPath, Reason: "Create directory inside the vault", Status: plannedStatus(req.DryRun)}}}}
	if req.DryRun {
		return projection, nil
	}

	target, err := safeJoin(root, folderPath)
	if err != nil {
		return errorProjection("folder.create", err), err
	}
	created := false
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.IsDir() {
			err := &domain.CommandError{Code: "folder_path_conflict", Message: "Target path exists and is not a directory", Hint: "Choose another directory path or handle the existing file first"}
			return domain.NewErrorProjection("folder.create", err), err
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return errorProjection("folder.create", err), err
		}
		created = true
	} else {
		return errorProjection("folder.create", statErr), statErr
	}

	registry, err := loadFolderRegistry(root)
	if err != nil {
		return errorProjection("folder.create", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	upsertFolderRecord(&registry, domain.FolderRecord{Path: folderPath, Purpose: purpose, ManagedStatus: domain.ManagedStatusManaged, CreatedAt: now, UpdatedAt: now})
	if err := saveFolderRegistry(root, registry); err != nil {
		return errorProjection("folder.create", err), err
	}
	projection.Summary = "Folder created."
	projection.Facts["created"] = fmt.Sprint(created)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}
	_ = appendEvent(root, "folder.created", "success", map[string]string{"folder_path": folderPath, "purpose": string(purpose), "created": fmt.Sprint(created)})
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) RenameFolder(_ context.Context, req FolderOperationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.rename", err), err
	}
	sourcePath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.rename", pathErr), pathErr
	}
	targetPath, pathErr := validateVaultFolderPath(req.TargetPath)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.rename", pathErr), pathErr
	}
	return applyFolderMove(root, "folder.rename", sourcePath, targetPath, req.DryRun, req.Yes, req.RequireSnapshot, "renamed")
}

func (s *Service) MoveFolder(_ context.Context, req FolderOperationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.move", err), err
	}
	sourcePath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.move", pathErr), pathErr
	}
	targetParent, pathErr := validateVaultFolderPath(req.TargetParent)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.move", pathErr), pathErr
	}
	if _, ok, err := findFolderInfo(root, targetParent); err != nil {
		return errorProjection("folder.move", err), err
	} else if !ok {
		err := &domain.CommandError{Code: "folder_not_found", Message: "Target parent folder not found", Hint: "Run pinax folder create <target-parent> --vault <vault> first"}
		return domain.NewErrorProjection("folder.move", err), err
	}
	targetPath := filepath.ToSlash(filepath.Join(targetParent, filepath.Base(sourcePath)))
	return applyFolderMove(root, "folder.move", sourcePath, targetPath, req.DryRun, req.Yes, req.RequireSnapshot, "moved")
}

func (s *Service) AdoptFolder(_ context.Context, req FolderOperationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.adopt", err), err
	}
	folderPath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.adopt", pathErr), pathErr
	}
	purpose, purposeErr := normalizeFolderPurpose(req.Purpose, domain.FolderPurposeGeneric)
	if purposeErr != nil {
		return domain.NewErrorProjection("folder.adopt", purposeErr), purposeErr
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "folder adopt requires --yes", Hint: "Preview first with pinax folder adopt <path> --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection("folder.adopt", err), err
	}
	target, err := safeJoin(root, folderPath)
	if err != nil {
		return errorProjection("folder.adopt", err), err
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = &domain.CommandError{Code: "folder_path_conflict", Message: "Target path exists and is not a directory", Hint: "Choose a directory path inside the vault"}
		}
		if errors.Is(statErr, os.ErrNotExist) {
			statErr = &domain.CommandError{Code: "folder_not_found", Message: "Folder to adopt not found", Hint: "Create it first with pinax folder create, or confirm the path exists"}
		}
		return errorProjection("folder.adopt", statErr), statErr
	}
	projection := domain.NewProjection("folder.adopt", "Folder adopt plan generated.")
	projection.Facts["folder_path"] = folderPath
	projection.Facts["purpose"] = string(purpose)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Facts["managed_status"] = string(domain.ManagedStatusManaged)
	projection.Data = map[string]any{"plan": domain.FolderOperationPlan{Operation: "adopt", Path: folderPath, Purpose: purpose, DryRun: req.DryRun, Writes: !req.DryRun, Effects: []domain.PlanOperation{{Kind: "registry.update", Path: filepath.ToSlash(filepath.Join(".pinax", "folders.json")), Reason: "Adopt existing directory", Status: plannedStatus(req.DryRun)}}}}
	if req.DryRun {
		return projection, nil
	}
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return errorProjection("folder.adopt", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	upsertFolderRecord(&registry, domain.FolderRecord{Path: folderPath, Purpose: purpose, ManagedStatus: domain.ManagedStatusManaged, CreatedAt: now, UpdatedAt: now})
	if err := saveFolderRegistry(root, registry); err != nil {
		return errorProjection("folder.adopt", err), err
	}
	projection.Summary = "Folder adopted."
	projection.Facts["adopted"] = "true"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}
	_ = appendEvent(root, "folder.adopted", "success", map[string]string{"folder_path": folderPath, "purpose": string(purpose)})
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) DeleteFolder(_ context.Context, req FolderOperationRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.delete", err), err
	}
	folderPath, pathErr := validateVaultFolderPath(req.Path)
	if pathErr != nil {
		return domain.NewErrorProjection("folder.delete", pathErr), pathErr
	}
	if !req.EmptyOnly {
		err := &domain.CommandError{Code: "empty_only_required", Message: "folder delete currently requires --empty-only", Hint: "Pinax currently only deletes empty directories; move the contents first, then retry"}
		return domain.NewErrorProjection("folder.delete", err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "folder delete requires --yes", Hint: "Preview first with pinax folder delete <path> --empty-only --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection("folder.delete", err), err
	}
	if req.RequireSnapshot && !req.DryRun && !hasVersionSnapshot(root) {
		return folderSnapshotRequiredProjection("folder.delete", root)
	}
	target, err := safeJoin(root, folderPath)
	if err != nil {
		return errorProjection("folder.delete", err), err
	}
	exists := false
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.IsDir() {
			err := &domain.CommandError{Code: "folder_path_conflict", Message: "Target path exists and is not a directory", Hint: "Choose a directory path inside the vault"}
			return domain.NewErrorProjection("folder.delete", err), err
		}
		exists = true
		if !dirIsEmpty(target) {
			err := &domain.CommandError{Code: "folder_not_empty", Message: "Folder is not empty and cannot be deleted with empty-only", Hint: "Move or delete folder contents, then retry pinax folder delete --empty-only"}
			return domain.NewErrorProjection("folder.delete", err), err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errorProjection("folder.delete", statErr), statErr
	}
	_, known, err := findFolderInfo(root, folderPath)
	if err != nil {
		return errorProjection("folder.delete", err), err
	}
	if !exists && !known {
		err := &domain.CommandError{Code: "folder_not_found", Message: "Folder not found", Hint: "Run pinax folder list --include-empty --vault <vault> to view folders"}
		return domain.NewErrorProjection("folder.delete", err), err
	}
	projection := domain.NewProjection("folder.delete", "Folder delete plan generated.")
	projection.Facts["folder_path"] = folderPath
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Facts["empty_only"] = fmt.Sprint(req.EmptyOnly)
	projection.Facts["requires_snapshot"] = "true"
	projection.Data = map[string]any{"plan": domain.FolderOperationPlan{Operation: "delete", Path: folderPath, DryRun: req.DryRun, Writes: !req.DryRun, Effects: []domain.PlanOperation{{Kind: "rmdir", Path: folderPath, Reason: "Delete empty directory", Status: plannedStatus(req.DryRun)}}}}
	if req.DryRun {
		return projection, nil
	}
	if exists {
		if err := os.Remove(target); err != nil {
			return errorProjection("folder.delete", err), err
		}
	}
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return errorProjection("folder.delete", err), err
	}
	removedRegistry := removeFolderRecords(&registry, folderPath)
	if err := saveFolderRegistry(root, registry); err != nil {
		return errorProjection("folder.delete", err), err
	}
	projection.Summary = "Folder deleted."
	projection.Facts["deleted"] = fmt.Sprint(exists)
	projection.Facts["removed_registry"] = fmt.Sprint(removedRegistry)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}
	_ = appendEvent(root, "folder.deleted", "success", map[string]string{"folder_path": folderPath, "deleted": fmt.Sprint(exists)})
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) RepairFolders(_ context.Context, req FolderRepairRequest) (domain.Projection, error) {
	if !req.Plan {
		err := &domain.CommandError{Code: "plan_required", Message: "folder repair currently requires --plan", Hint: "Use pinax folder repair --plan --vault <vault>"}
		return domain.NewErrorProjection("folder.repair", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("folder.repair", err), err
	}
	folders, err := collectFolders(root, true, 0)
	if err != nil {
		return errorProjection("folder.repair", err), err
	}
	type folderRepairIssue struct {
		Code      string `json:"code"`
		Path      string `json:"path"`
		Message   string `json:"message"`
		Operation string `json:"operation"`
	}
	issues := []folderRepairIssue{}
	operations := []domain.PlanOperation{}
	for _, folder := range folders {
		if folder.ManagedStatus == domain.ManagedStatusManaged && !folder.Exists {
			issues = append(issues, folderRepairIssue{Code: "managed_folder_missing", Path: folder.Path, Message: "Managed directory in registry does not exist", Operation: "registry.remove"})
			operations = append(operations, domain.PlanOperation{Kind: "registry.remove", Path: folder.Path, Reason: "Managed directory is missing", Status: "manual_review", Evidence: []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}})
		}
		if folder.ManagedStatus == domain.ManagedStatusAdoptable && folder.Exists {
			issues = append(issues, folderRepairIssue{Code: "folder_adoptable", Path: folder.Path, Message: "Directory exists but is not adopted by the folder registry", Operation: "folder.adopt"})
			operations = append(operations, domain.PlanOperation{Kind: "folder.adopt", Path: folder.Path, Reason: "Normalize folder metadata and hooks", Status: "manual_review"})
		}
	}
	projection := domain.NewProjection("folder.repair", "Folder repair plan generated.")
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["planned"] = fmt.Sprint(len(operations))
	projection.Facts["writes"] = "false"
	projection.Facts["dry_run"] = "true"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}
	projection.Data = map[string]any{"issues": issues, "plan": domain.FolderOperationPlan{Operation: "repair", DryRun: true, Writes: false, Effects: operations}}
	return projection, nil
}

func applyFolderMove(root, command, sourcePath, targetPath string, dryRun, yes, requireSnapshot bool, appliedFact string) (domain.Projection, error) {
	if sourcePath == targetPath || strings.HasPrefix(targetPath, sourcePath+"/") {
		err := &domain.CommandError{Code: "invalid_folder_target", Message: "Folder target path is invalid", Hint: "Choose a different directory and do not move it under itself"}
		return domain.NewErrorProjection(command, err), err
	}
	if !dryRun && !yes {
		err := &domain.CommandError{Code: "approval_required", Message: "folder write requires --yes", Hint: fmt.Sprintf("Preview first with pinax %s %s %s --dry-run --vault <vault> --json", strings.ReplaceAll(command, ".", " "), sourcePath, targetPath)}
		return domain.NewErrorProjection(command, err), err
	}
	if requireSnapshot && !dryRun && !hasVersionSnapshot(root) {
		return folderSnapshotRequiredProjection(command, root)
	}
	sourceInfo, known, err := findFolderInfo(root, sourcePath)
	if err != nil {
		return errorProjection(command, err), err
	}
	source, err := safeJoin(root, sourcePath)
	if err != nil {
		return errorProjection(command, err), err
	}
	target, err := safeJoin(root, targetPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	sourceExists := false
	if info, statErr := os.Stat(source); statErr == nil {
		if !info.IsDir() {
			err := &domain.CommandError{Code: "folder_path_conflict", Message: "Source path exists and is not a directory", Hint: "Choose a directory path inside the vault"}
			return domain.NewErrorProjection(command, err), err
		}
		sourceExists = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errorProjection(command, statErr), statErr
	}
	if !sourceExists && !known {
		err := &domain.CommandError{Code: "folder_not_found", Message: "Folder not found", Hint: "Run pinax folder list --include-empty --vault <vault> to view folders"}
		return domain.NewErrorProjection(command, err), err
	}
	if _, err := os.Stat(target); err == nil {
		err := &domain.CommandError{Code: "folder_path_conflict", Message: "Target folder already exists", Hint: "Choose another target path or handle the existing directory first"}
		return domain.NewErrorProjection(command, err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Folder move plan generated.")
	if command == "folder.rename" {
		projection.Summary = "Folder rename plan generated."
	}
	projection.Facts["folder_path"] = sourcePath
	projection.Facts["target_path"] = targetPath
	projection.Facts["dry_run"] = fmt.Sprint(dryRun)
	projection.Facts["writes"] = fmt.Sprint(!dryRun)
	projection.Facts["requires_snapshot"] = "true"
	if sourceInfo.Purpose != "" {
		projection.Facts["purpose"] = string(sourceInfo.Purpose)
	}
	projection.Data = map[string]any{"plan": domain.FolderOperationPlan{Operation: strings.TrimPrefix(command, "folder."), Path: sourcePath, Target: targetPath, Purpose: sourceInfo.Purpose, DryRun: dryRun, Writes: !dryRun, Effects: []domain.PlanOperation{{Kind: "rename", Path: sourcePath, Target: targetPath, Reason: "Move directory inside the vault", Status: plannedStatus(dryRun)}}}}
	if dryRun {
		return projection, nil
	}
	updatedNotes, err := rewriteMovedFolderNoteMetadata(root, sourcePath, targetPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if sourceExists {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection(command, err), err
		}
		if err := os.Rename(source, target); err != nil {
			return errorProjection(command, err), err
		}
	}
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	rewriteFolderRecordPaths(&registry, sourcePath, targetPath, sourceInfo.Purpose)
	if err := saveFolderRegistry(root, registry); err != nil {
		return errorProjection(command, err), err
	}
	projection.Summary = "Folder moved."
	if command == "folder.rename" {
		projection.Summary = "Folder renamed."
	}
	projection.Facts[appliedFact] = "true"
	projection.Facts["updated_notes"] = fmt.Sprint(updatedNotes)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "folders.json"))}
	_ = appendEvent(root, command, "success", map[string]string{"folder_path": sourcePath, "target_path": targetPath})
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func folderSnapshotRequiredProjection(command, root string) (domain.Projection, error) {
	err := &domain.CommandError{Code: "snapshot_required", Message: "High-risk folder changes require an explicit version snapshot", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before folder change"))}
	projection := domain.NewErrorProjection(command, err)
	projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
	return projection, err
}

func rewriteMovedFolderNoteMetadata(root, sourcePath, targetPath string) (int, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return 0, err
	}
	updatedNotes := 0
	for _, note := range notes {
		if !folderContainsPath(sourcePath, note.Path) {
			continue
		}
		nextFolder, ok := movedNoteFolderValue(note.Folder, sourcePath, targetPath)
		if !ok {
			continue
		}
		path, err := safeJoin(root, note.Path)
		if err != nil {
			return updatedNotes, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return updatedNotes, err
		}
		meta, _ := splitFrontmatter(string(content))
		meta["folder"] = nextFolder
		meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		updated, _ := patchFrontmatterFields(string(content), meta)
		if err := commitNoteContent(path, path, updated); err != nil {
			return updatedNotes, err
		}
		updatedNotes++
	}
	return updatedNotes, nil
}

func movedNoteFolderValue(current, sourcePath, targetPath string) (string, bool) {
	current = strings.Trim(filepath.ToSlash(current), "/")
	if current == "" {
		return "", false
	}
	if current == sourcePath {
		return targetPath, true
	}
	if strings.HasPrefix(current, sourcePath+"/") {
		return targetPath + strings.TrimPrefix(current, sourcePath), true
	}
	return "", false
}

func collectFolders(root string, includeEmpty bool, maxDepth int) ([]domain.FolderInfo, error) {
	registry, err := loadFolderRegistry(root)
	if err != nil {
		return nil, err
	}
	folders := map[string]domain.FolderInfo{}
	for _, record := range registry.Folders {
		if record.Path == "" {
			continue
		}
		folders[record.Path] = domain.FolderInfo{Path: record.Path, Purpose: record.Purpose, ManagedStatus: record.ManagedStatus, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Depth: folderDepth(record.Path)}
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		if shouldSkipVaultWalkDir(entry.Name()) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		folderPath := filepath.ToSlash(rel)
		if maxDepth > 0 && folderDepth(folderPath) > maxDepth {
			return filepath.SkipDir
		}
		info := folders[folderPath]
		info.Path = folderPath
		info.Exists = true
		info.Empty = dirIsEmpty(path)
		info.Depth = folderDepth(folderPath)
		if info.ManagedStatus == "" {
			info.ManagedStatus = domain.ManagedStatusAdoptable
		}
		folders[folderPath] = info
		return nil
	}); err != nil {
		return nil, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	for _, note := range notes {
		for path, info := range folders {
			if folderContainsPath(path, note.Path) {
				info.NoteCount++
				folders[path] = info
			}
		}
	}
	assetCounts, err := countAssetsByFolder(root)
	if err != nil {
		return nil, err
	}
	for path, count := range assetCounts {
		info := folders[path]
		if info.Path == "" {
			info.Path = path
			info.Depth = folderDepth(path)
			info.ManagedStatus = domain.ManagedStatusAdoptable
		}
		info.AssetCount += count
		folders[path] = info
	}
	result := make([]domain.FolderInfo, 0, len(folders))
	for _, info := range folders {
		if info.Path == "" {
			continue
		}
		if !info.Exists {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(info.Path))); err == nil {
				info.Exists = true
			}
		}
		if info.ManagedStatus == "" {
			info.ManagedStatus = domain.ManagedStatusAdoptable
		}
		if info.Purpose == "" {
			info.Purpose = inferFolderPurpose(info)
		}
		if !includeEmpty && info.NoteCount == 0 && info.AssetCount == 0 {
			continue
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func loadFolderRegistry(root string) (domain.FolderRegistry, error) {
	registry := domain.FolderRegistry{SchemaVersion: folderRegistrySchemaVersion, Folders: []domain.FolderRecord{}}
	b, err := os.ReadFile(folderRegistryPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = folderRegistrySchemaVersion
	}
	cleaned := make([]domain.FolderRecord, 0, len(registry.Folders))
	for _, record := range registry.Folders {
		path, pathErr := validateVaultFolderPath(record.Path)
		if pathErr != nil {
			continue
		}
		purpose, purposeErr := normalizeFolderPurpose(string(record.Purpose), domain.FolderPurposeGeneric)
		if purposeErr != nil {
			purpose = domain.FolderPurposeGeneric
		}
		record.Path = path
		record.Purpose = purpose
		if record.ManagedStatus == "" {
			record.ManagedStatus = domain.ManagedStatusManaged
		}
		cleaned = append(cleaned, record)
	}
	registry.Folders = cleaned
	return registry, nil
}

func saveFolderRegistry(root string, registry domain.FolderRegistry) error {
	registry.SchemaVersion = folderRegistrySchemaVersion
	sort.Slice(registry.Folders, func(i, j int) bool { return registry.Folders[i].Path < registry.Folders[j].Path })
	if err := os.MkdirAll(filepath.Dir(folderRegistryPath(root)), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(folderRegistryPath(root), b, 0o644)
}

func folderRegistryPath(root string) string {
	return filepath.Join(root, ".pinax", "folders.json")
}

func upsertFolderRecord(registry *domain.FolderRegistry, next domain.FolderRecord) {
	for i, current := range registry.Folders {
		if current.Path != next.Path {
			continue
		}
		if current.CreatedAt != "" {
			next.CreatedAt = current.CreatedAt
		}
		registry.Folders[i] = next
		return
	}
	registry.Folders = append(registry.Folders, next)
}

func findFolderInfo(root, folderPath string) (domain.FolderInfo, bool, error) {
	folders, err := collectFolders(root, true, 0)
	if err != nil {
		return domain.FolderInfo{}, false, err
	}
	for _, folder := range folders {
		if folder.Path == folderPath {
			return folder, true, nil
		}
	}
	return domain.FolderInfo{}, false, nil
}

func rewriteFolderRecordPaths(registry *domain.FolderRegistry, sourcePath, targetPath string, fallbackPurpose domain.FolderPurpose) {
	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for i, current := range registry.Folders {
		if current.Path != sourcePath && !strings.HasPrefix(current.Path, sourcePath+"/") {
			continue
		}
		suffix := strings.TrimPrefix(current.Path, sourcePath)
		current.Path = targetPath + suffix
		current.UpdatedAt = now
		if current.ManagedStatus == "" {
			current.ManagedStatus = domain.ManagedStatusManaged
		}
		if current.Purpose == "" {
			current.Purpose = fallbackPurpose
		}
		registry.Folders[i] = current
		found = true
	}
	if !found {
		if fallbackPurpose == "" {
			fallbackPurpose = domain.FolderPurposeGeneric
		}
		upsertFolderRecord(registry, domain.FolderRecord{Path: targetPath, Purpose: fallbackPurpose, ManagedStatus: domain.ManagedStatusManaged, CreatedAt: now, UpdatedAt: now})
	}
}

func removeFolderRecords(registry *domain.FolderRegistry, folderPath string) int {
	kept := registry.Folders[:0]
	removed := 0
	for _, record := range registry.Folders {
		if record.Path == folderPath || strings.HasPrefix(record.Path, folderPath+"/") {
			removed++
			continue
		}
		kept = append(kept, record)
	}
	registry.Folders = kept
	return removed
}

func validateVaultFolderPath(raw string) (string, *domain.CommandError) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &domain.CommandError{Code: "folder_path_required", Message: "folder path cannot be empty", Hint: "pinax folder create <path> --vault <vault>"}
	}
	if filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", unsafeFolderPathError()
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", unsafeFolderPathError()
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." || part == ".pinax" || part == ".git" {
			return "", unsafeFolderPathError()
		}
	}
	return clean, nil
}

func unsafeFolderPathError() *domain.CommandError {
	return &domain.CommandError{Code: "unsafe_folder_path", Message: "folder path must be a safe relative directory inside the vault", Hint: "Use a path like spaces/research, notes/work, or assets/images"}
}

func normalizeFolderListPurpose(raw string) (string, *domain.CommandError) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "all", nil
	}
	if value == "all" || value == string(domain.FolderPurposeNotes) || value == string(domain.FolderPurposeAssets) || value == string(domain.FolderPurposeGeneric) {
		return value, nil
	}
	return "", &domain.CommandError{Code: "invalid_folder_purpose", Message: "Unknown folder purpose", Hint: "Use notes, assets, generic, or all"}
}

func normalizeFolderPurpose(raw string, fallback domain.FolderPurpose) (domain.FolderPurpose, *domain.CommandError) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return fallback, nil
	}
	switch domain.FolderPurpose(value) {
	case domain.FolderPurposeNotes, domain.FolderPurposeAssets, domain.FolderPurposeGeneric:
		return domain.FolderPurpose(value), nil
	default:
		return "", &domain.CommandError{Code: "invalid_folder_purpose", Message: "Unknown folder purpose", Hint: "Use notes, assets, or generic"}
	}
}

func folderPurposeMatches(filter string, purpose domain.FolderPurpose) bool {
	return filter == "" || filter == "all" || filter == string(purpose)
}

func folderUnderPathMatches(path, under string, maxRelativeDepth int) bool {
	if under == "" {
		return true
	}
	if path != under && !strings.HasPrefix(path, under+"/") {
		return false
	}
	if maxRelativeDepth <= 0 {
		return true
	}
	return folderDepth(path)-folderDepth(under) <= maxRelativeDepth
}

func immediateChildFolders(folders []domain.FolderInfo, parent string) []domain.FolderInfo {
	children := make([]domain.FolderInfo, 0)
	for _, folder := range folders {
		if folder.Path == parent || !strings.HasPrefix(folder.Path, parent+"/") {
			continue
		}
		if folderDepth(folder.Path)-folderDepth(parent) == 1 {
			children = append(children, folder)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return children
}

func descendantFolderCount(folders []domain.FolderInfo, parent string) int {
	count := 0
	for _, folder := range folders {
		if folder.Path != parent && strings.HasPrefix(folder.Path, parent+"/") {
			count++
		}
	}
	return count
}

func inferFolderPurpose(info domain.FolderInfo) domain.FolderPurpose {
	if info.NoteCount > 0 || info.Path == "notes" || strings.HasPrefix(info.Path, "notes/") {
		return domain.FolderPurposeNotes
	}
	if info.AssetCount > 0 || info.Path == "assets" || strings.HasPrefix(info.Path, "assets/") {
		return domain.FolderPurposeAssets
	}
	return domain.FolderPurposeGeneric
}

func folderDepth(path string) int {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func folderContainsPath(folderPath, objectPath string) bool {
	folderPath = strings.Trim(filepath.ToSlash(folderPath), "/")
	objectPath = strings.Trim(filepath.ToSlash(objectPath), "/")
	return objectPath == folderPath || strings.HasPrefix(objectPath, folderPath+"/")
}

func dirIsEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func countAssetsByFolder(root string) (map[string]int, error) {
	counts := map[string]int{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		folderPath := filepath.ToSlash(rel)
		if folderPath == "." {
			return nil
		}
		counts[folderPath]++
		return nil
	})
	return counts, err
}

func setFolderFacts(facts map[string]string, folder domain.FolderInfo) {
	facts["folder_path"] = folder.Path
	facts["purpose"] = string(folder.Purpose)
	facts["managed_status"] = string(folder.ManagedStatus)
	facts["exists"] = fmt.Sprint(folder.Exists)
	facts["empty"] = fmt.Sprint(folder.Empty)
	facts["note_count"] = fmt.Sprint(folder.NoteCount)
	facts["asset_count"] = fmt.Sprint(folder.AssetCount)
}

func plannedStatus(dryRun bool) string {
	if dryRun {
		return "planned"
	}
	return "applied"
}
