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

const tombstonesRel = ".pinax/records/tombstones.json"

type TrashRequest struct {
	VaultPath string
	ObjectRef string
	DryRun    bool
	Hard      bool
	Yes       bool
}

type ProjectDeleteRequest struct {
	VaultPath string
	Project   string
	Yes       bool
}

type ProjectSubprojectDeleteRequest struct {
	VaultPath  string
	Project    string
	Subproject string
	Yes        bool
}

type remoteTrashDeleteMarker struct {
	ObjectKind  string
	ObjectID    string
	TombstoneID string
	DeletedAt   string
}

type remoteTrashDeleteResult struct {
	Applied  bool
	Conflict *domain.SyncConflictEntry
}

func (s *Service) TrashList(_ context.Context, req TrashRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("trash.list", err), err
	}
	tombstones, err := loadTrashTombstones(root)
	if err != nil {
		return errorProjection("trash.list", err), err
	}
	entries := make([]domain.Tombstone, 0, len(tombstones))
	for _, tombstone := range tombstones {
		if tombstone.RestoredAt != "" {
			continue
		}
		entries = append(entries, tombstone)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].DeletedAt > entries[j].DeletedAt })
	projection := domain.NewProjection("trash.list", "Trash entries listed.")
	projection.Facts["entries"] = fmt.Sprint(len(entries))
	projection.Facts["local_write"] = "false"
	projection.Facts["remote_write"] = "false"
	for i, entry := range entries {
		prefix := fmt.Sprintf("entry.%d.", i+1)
		projection.Facts[prefix+"object_id"] = trashObjectID(entry)
		projection.Facts[prefix+"object_kind"] = trashObjectKind(entry)
		projection.Facts[prefix+"deleted_at"] = entry.DeletedAt
		projection.Facts[prefix+"trash_path"] = entry.TrashPath
	}
	projection.Evidence = []string{tombstonesRel}
	projection.Data = map[string]any{"entries": entries}
	projection.Actions = []domain.Action{{Name: "restore", Command: fmt.Sprintf("pinax trash restore <object> --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) TrashRestore(_ context.Context, req TrashRequest) (domain.Projection, error) {
	root, tombstones, tombstone, err := resolveTrashTombstone(req.VaultPath, req.ObjectRef, "trash.restore")
	if err != nil {
		return errorProjection("trash.restore", err), err
	}
	objectID := trashObjectID(tombstone)
	switch trashObjectKind(tombstone) {
	case "project":
		if err := restoreProjectFromTrash(root, tombstone); err != nil {
			return errorProjection("trash.restore", err), err
		}
	case "subproject":
		if err := restoreSubprojectFromTrash(root, tombstone); err != nil {
			return errorProjection("trash.restore", err), err
		}
	default:
		err := &domain.CommandError{Code: "unsupported_trash_object", Message: "Trash object kind is not restorable yet", Hint: "Run pinax trash list --json"}
		return domain.NewErrorProjection("trash.restore", err), err
	}
	delete(tombstones, objectID)
	if err := saveTrashTombstones(root, tombstones); err != nil {
		return errorProjection("trash.restore", err), err
	}
	_ = appendEvent(root, "trash.restore", "success", map[string]string{"object_id": objectID})
	projection := domain.NewProjection("trash.restore", "Trash object restored.")
	projection.Facts["object_id"] = objectID
	projection.Facts["object_kind"] = trashObjectKind(tombstone)
	projection.Facts["local_write"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Facts["trash_path"] = tombstone.TrashPath
	projection.Evidence = []string{tombstonesRel, tombstone.TrashPath}
	projection.Data = map[string]any{"tombstone": tombstone}
	return projection, nil
}

func (s *Service) TrashPurge(_ context.Context, req TrashRequest) (domain.Projection, error) {
	root, tombstones, tombstone, err := resolveTrashTombstone(req.VaultPath, req.ObjectRef, "trash.purge")
	if err != nil {
		return errorProjection("trash.purge", err), err
	}
	if !req.DryRun && (!req.Hard || !req.Yes) {
		err := &domain.CommandError{Code: "approval_required", Message: "trash purge requires --hard --yes or --dry-run", Hint: "Review first with pinax trash purge <object> --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection("trash.purge", err), err
	}
	objectID := trashObjectID(tombstone)
	projection := domain.NewProjection("trash.purge", "Trash purge planned.")
	projection.Facts["object_id"] = objectID
	projection.Facts["object_kind"] = trashObjectKind(tombstone)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["hard"] = fmt.Sprint(req.Hard)
	projection.Facts["local_write"] = fmt.Sprint(!req.DryRun)
	projection.Facts["remote_write"] = "false"
	projection.Facts["trash_path"] = tombstone.TrashPath
	projection.Evidence = []string{tombstonesRel, tombstone.TrashPath}
	projection.Data = map[string]any{"tombstone": tombstone}
	if req.DryRun {
		projection.Actions = []domain.Action{{Name: "purge", Command: fmt.Sprintf("pinax trash purge %s --hard --yes --vault %s --json", shellQuote(objectID), shellQuote(root))}}
		return projection, nil
	}
	if tombstone.TrashPath != "" {
		trashPath, err := safeJoin(root, tombstone.TrashPath)
		if err != nil {
			return errorProjection("trash.purge", err), err
		}
		if err := os.RemoveAll(trashPath); err != nil {
			return errorProjection("trash.purge", err), err
		}
	}
	delete(tombstones, objectID)
	if err := saveTrashTombstones(root, tombstones); err != nil {
		return errorProjection("trash.purge", err), err
	}
	_ = appendEvent(root, "trash.purge", "success", map[string]string{"object_id": objectID, "hard": "true"})
	projection.Summary = "Trash object purged."
	return projection, nil
}

func (s *Service) ProjectDelete(_ context.Context, req ProjectDeleteRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "project delete requires --yes", Hint: "Rerun with --yes after confirming; restore is available through pinax trash restore"}
		return domain.NewErrorProjection("project.delete", err), err
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return errorProjection("project.delete", err), err
	}
	idx := -1
	var project domain.Project
	for i, item := range registry.Projects {
		if item.Slug == req.Project {
			idx = i
			project = item
			break
		}
	}
	if idx < 0 {
		notFound := projectNotFoundWithRestore(root, req.Project)
		return errorProjection("project.delete", notFound), notFound
	}
	trashRel, err := uniqueTrashDirRel(root, filepath.ToSlash(filepath.Join(".pinax", "trash", time.Now().UTC().Format("20060102"), "projects", project.Slug)))
	if err != nil {
		return errorProjection("project.delete", err), err
	}
	trashPath, err := safeJoin(root, trashRel)
	if err != nil {
		return errorProjection("project.delete", err), err
	}
	if err := os.MkdirAll(trashPath, 0o755); err != nil {
		return errorProjection("project.delete", err), err
	}
	registryBackupRel := filepath.ToSlash(filepath.Join(trashRel, "registry.json"))
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(registryBackupRel)), project); err != nil {
		return errorProjection("project.delete", err), err
	}
	contentBackupRel := ""
	if strings.TrimSpace(project.NotesPrefix) != "" {
		if moved, moveErr := moveIfExists(root, project.NotesPrefix, filepath.ToSlash(filepath.Join(trashRel, "content"))); moveErr != nil {
			return errorProjection("project.delete", moveErr), moveErr
		} else if moved {
			contentBackupRel = filepath.ToSlash(filepath.Join(trashRel, "content"))
		}
	}
	registry.Projects = append(registry.Projects[:idx], registry.Projects[idx+1:]...)
	if registry.CurrentProject == project.Slug {
		registry.CurrentProject = ""
		if len(registry.Projects) > 0 {
			registry.CurrentProject = registry.Projects[0].Slug
		}
	}
	if err := saveProjectRegistry(root, registry); err != nil {
		return errorProjection("project.delete", err), err
	}
	tombstone := domain.Tombstone{ObjectKind: "project", ObjectID: "project/" + project.Slug, TombstoneID: trashID("project/" + project.Slug), OldPath: project.NotesPrefix, Title: project.Name, TrashPath: trashRel, RegistryPath: registryBackupRel, RegistryFacts: map[string]any{"project": project, "content_path": contentBackupRel}, DeletedAt: time.Now().UTC().Format(time.RFC3339), Source: "project.delete", Evidence: []string{registryBackupRel, tombstonesRel}}
	if err := upsertTrashTombstone(root, tombstone); err != nil {
		return errorProjection("project.delete", err), err
	}
	_ = appendEvent(root, "project.delete", "success", map[string]string{"project": project.Slug, "trash_path": trashRel})
	projection := domain.NewProjection("project.delete", "Project moved to trash.")
	projection.Facts["project"] = project.Slug
	projection.Facts["object_id"] = tombstone.ObjectID
	projection.Facts["object_kind"] = tombstone.ObjectKind
	projection.Facts["trash_path"] = trashRel
	projection.Facts["current_project"] = registry.CurrentProject
	projection.Facts["local_write"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Evidence = []string{registryBackupRel, tombstonesRel, filepath.ToSlash(filepath.Join(".pinax", "projects.json"))}
	projection.Data = map[string]any{"project": project, "tombstone": tombstone, "registry": registry}
	projection.Actions = []domain.Action{{Name: "restore", Command: fmt.Sprintf("pinax trash restore project/%s --vault %s --json", shellQuote(project.Slug), shellQuote(root))}}
	return projection, nil
}

func (s *Service) ProjectSubprojectDelete(_ context.Context, req ProjectSubprojectDeleteRequest) (domain.Projection, error) {
	root, project, subproject, err := validateProjectWorkspaceRequest(ProjectWorkspaceRequest{VaultPath: req.VaultPath, Project: req.Project, Subproject: req.Subproject})
	if err != nil {
		return errorProjection("project.subproject.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "project subproject delete requires --yes", Hint: "Rerun with --yes after confirming"}
		return domain.NewErrorProjection("project.subproject.delete", err), err
	}
	workspace, err := loadProjectWorkspace(root, project.Slug, subproject)
	if err != nil {
		return errorProjection("project.subproject.delete", err), err
	}
	if workspaceHasContent(root, workspace.WorkspacePath) && !hasVersionSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "non-empty subproject delete requires a recent snapshot", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("before subproject delete"))}
		projection := domain.NewErrorProjection("project.subproject.delete", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		return projection, err
	}
	trashRel, err := uniqueTrashDirRel(root, filepath.ToSlash(filepath.Join(".pinax", "trash", time.Now().UTC().Format("20060102"), "subprojects", project.Slug, subproject)))
	if err != nil {
		return errorProjection("project.subproject.delete", err), err
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(trashRel)), 0o755); err != nil {
		return errorProjection("project.subproject.delete", err), err
	}
	workspaceBackupRel := filepath.ToSlash(filepath.Join(trashRel, "workspace"))
	if moved, moveErr := moveIfExists(root, workspace.WorkspacePath, workspaceBackupRel); moveErr != nil {
		return errorProjection("project.subproject.delete", moveErr), moveErr
	} else if !moved {
		workspaceBackupRel = ""
	}
	registryRel := projectWorkspaceRegistryRel(project.Slug, subproject)
	registryBackupRel := filepath.ToSlash(filepath.Join(trashRel, "registry.json"))
	if moved, moveErr := moveIfExists(root, registryRel, registryBackupRel); moveErr != nil {
		return errorProjection("project.subproject.delete", moveErr), moveErr
	} else if !moved {
		if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(registryBackupRel)), workspace); err != nil {
			return errorProjection("project.subproject.delete", err), err
		}
	}
	boardRel := projectBoardConfigRel(project.Slug, subproject)
	boardBackupRel := ""
	if moved, moveErr := moveIfExists(root, boardRel, filepath.ToSlash(filepath.Join(trashRel, "board.json"))); moveErr != nil {
		return errorProjection("project.subproject.delete", moveErr), moveErr
	} else if moved {
		boardBackupRel = filepath.ToSlash(filepath.Join(trashRel, "board.json"))
	}
	clearCurrentWorkspaceIfMatches(root, project.Slug, subproject)
	tombstone := domain.Tombstone{ObjectKind: "subproject", ObjectID: "subproject/" + project.Slug + "/" + subproject, TombstoneID: trashID("subproject/" + project.Slug + "/" + subproject), OldPath: workspace.WorkspacePath, Title: workspace.Title, TrashPath: trashRel, RegistryPath: registryBackupRel, RegistryFacts: map[string]any{"workspace": workspace, "workspace_backup": workspaceBackupRel, "board_backup": boardBackupRel}, DeletedAt: time.Now().UTC().Format(time.RFC3339), Source: "project.subproject.delete", Evidence: []string{registryBackupRel, tombstonesRel}}
	if err := upsertTrashTombstone(root, tombstone); err != nil {
		return errorProjection("project.subproject.delete", err), err
	}
	_ = appendEvent(root, "project.subproject.delete", "success", map[string]string{"project": project.Slug, "subproject": subproject, "trash_path": trashRel})
	projection := domain.NewProjection("project.subproject.delete", "Project subproject moved to trash.")
	projection.Facts["project"] = project.Slug
	projection.Facts["subproject"] = subproject
	projection.Facts["object_id"] = tombstone.ObjectID
	projection.Facts["object_kind"] = tombstone.ObjectKind
	projection.Facts["workspace_path"] = workspace.WorkspacePath
	projection.Facts["trash_path"] = trashRel
	projection.Facts["local_write"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Evidence = []string{registryBackupRel, tombstonesRel}
	projection.Data = map[string]any{"workspace": workspace, "tombstone": tombstone}
	projection.Actions = []domain.Action{{Name: "restore", Command: fmt.Sprintf("pinax trash restore subproject/%s/%s --vault %s --json", shellQuote(project.Slug), shellQuote(subproject), shellQuote(root))}}
	return projection, nil
}

func restoreProjectFromTrash(root string, tombstone domain.Tombstone) error {
	var project domain.Project
	if err := readJSONAsset(root, tombstone.RegistryPath, &project); err != nil {
		return err
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return err
	}
	for _, existing := range registry.Projects {
		if existing.Slug == project.Slug {
			return &domain.CommandError{Code: "restore_conflict", Message: "active project already exists", Hint: "Rename or delete the active project before restoring"}
		}
	}
	if contentPath, _ := tombstone.RegistryFacts["content_path"].(string); contentPath != "" {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(project.NotesPrefix))); err == nil {
			return &domain.CommandError{Code: "restore_conflict", Message: "project content path already exists", Hint: "Move the active content path before restoring"}
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(contentPath))); err == nil {
			if err := moveIfExistsRequired(root, contentPath, project.NotesPrefix); err != nil {
				return err
			}
		}
	}
	registry.Projects = append(registry.Projects, project)
	if registry.CurrentProject == "" {
		registry.CurrentProject = project.Slug
	}
	return saveProjectRegistry(root, registry)
}

func restoreSubprojectFromTrash(root string, tombstone domain.Tombstone) error {
	var workspace domain.ProjectWorkspace
	if err := readJSONAsset(root, tombstone.RegistryPath, &workspace); err != nil {
		return err
	}
	if _, err := loadProjectWorkspace(root, workspace.Project, workspace.Subproject); err == nil {
		return &domain.CommandError{Code: "restore_conflict", Message: "active subproject already exists", Hint: "Delete or rename the active subproject before restoring"}
	}
	workspaceBackup, _ := tombstone.RegistryFacts["workspace_backup"].(string)
	if workspaceBackup != "" {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(workspace.WorkspacePath))); err == nil {
			return &domain.CommandError{Code: "restore_conflict", Message: "workspace path already exists", Hint: "Move the active workspace path before restoring"}
		}
		if err := moveIfExistsRequired(root, workspaceBackup, workspace.WorkspacePath); err != nil {
			return err
		}
	}
	if err := saveProjectWorkspace(root, workspace); err != nil {
		return err
	}
	boardBackup, _ := tombstone.RegistryFacts["board_backup"].(string)
	if boardBackup != "" {
		boardRel := projectBoardConfigRel(workspace.Project, workspace.Subproject)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(boardRel))); err == nil {
			return &domain.CommandError{Code: "restore_conflict", Message: "board config already exists", Hint: "Move the active board config before restoring"}
		}
		if err := moveIfExistsRequired(root, boardBackup, boardRel); err != nil {
			return err
		}
	}
	return nil
}

func resolveTrashTombstone(vaultPath, objectRef, command string) (string, map[string]domain.Tombstone, domain.Tombstone, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", nil, domain.Tombstone{}, err
	}
	objectID := normalizeTrashObjectRef(objectRef)
	if objectID == "" {
		return "", nil, domain.Tombstone{}, &domain.CommandError{Code: "trash_object_required", Message: command + " requires an object", Hint: "Run pinax trash list --json"}
	}
	tombstones, err := loadTrashTombstones(root)
	if err != nil {
		return "", nil, domain.Tombstone{}, err
	}
	tombstone, ok := tombstones[objectID]
	if !ok || tombstone.RestoredAt != "" {
		return "", nil, domain.Tombstone{}, &domain.CommandError{Code: "trash_object_not_found", Message: "Trash object not found", Hint: "Run pinax trash list --json"}
	}
	return root, tombstones, tombstone, nil
}

func loadTrashTombstones(root string) (map[string]domain.Tombstone, error) {
	tombstones := map[string]domain.Tombstone{}
	path := filepath.Join(root, filepath.FromSlash(tombstonesRel))
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return tombstones, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(payload)) == "" {
		return tombstones, nil
	}
	if err := json.Unmarshal(payload, &tombstones); err != nil {
		return nil, err
	}
	return tombstones, nil
}

func saveTrashTombstones(root string, tombstones map[string]domain.Tombstone) error {
	return writeJSONAsset(filepath.Join(root, filepath.FromSlash(tombstonesRel)), tombstones)
}

func upsertTrashTombstone(root string, tombstone domain.Tombstone) error {
	tombstones, err := loadTrashTombstones(root)
	if err != nil {
		return err
	}
	tombstones[trashObjectID(tombstone)] = tombstone
	return saveTrashTombstones(root, tombstones)
}

func trashObjectID(tombstone domain.Tombstone) string {
	if tombstone.ObjectID != "" {
		return tombstone.ObjectID
	}
	if tombstone.NoteID != "" {
		return "note/" + tombstone.NoteID
	}
	return tombstone.OldPath
}

func trashObjectKind(tombstone domain.Tombstone) string {
	if tombstone.ObjectKind != "" {
		return tombstone.ObjectKind
	}
	return "note"
}

func trashID(objectID string) string {
	return "trash_" + strings.TrimPrefix(stableNoteID(objectID), "note_")
}

func normalizeTrashObjectRef(ref string) string {
	ref = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ref)))
	if ref == "." || strings.HasPrefix(ref, "../") || strings.HasPrefix(ref, "/") {
		return ""
	}
	return ref
}

func uniqueTrashDirRel(root, base string) (string, error) {
	candidate := filepath.ToSlash(base)
	for i := 2; ; i++ {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func moveIfExists(root, fromRel, toRel string) (bool, error) {
	from, err := safeJoin(root, fromRel)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	to, err := safeJoin(root, toRel)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(to); err == nil {
		return false, &domain.CommandError{Code: "trash_path_conflict", Message: "trash path already exists", Hint: "Run pinax trash list --json and retry"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return false, err
	}
	return true, os.Rename(from, to)
}

func moveIfExistsRequired(root, fromRel, toRel string) error {
	moved, err := moveIfExists(root, fromRel, toRel)
	if err != nil {
		return err
	}
	if !moved {
		return &domain.CommandError{Code: "trash_backup_missing", Message: "trash backup is missing", Hint: "Run pinax trash list --json to inspect recoverable entries"}
	}
	return nil
}

func readJSONAsset(root, rel string, target any) error {
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func workspaceHasContent(root, rel string) bool {
	path, err := safeJoin(root, rel)
	if err != nil {
		return true
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	found := false
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || found {
			return nil
		}
		if !entry.IsDir() {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func clearCurrentWorkspaceIfMatches(root, project, subproject string) {
	path := filepath.Join(root, ".pinax", "workspaces", "current.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var current domain.CurrentWorkspace
	if json.Unmarshal(payload, &current) != nil {
		return
	}
	if current.Project == project && current.Subproject == subproject {
		_ = os.Remove(path)
	}
}

func projectNotFoundWithRestore(root, slug string) *domain.CommandError {
	err := &domain.CommandError{Code: "project_not_found", Message: "Project not found", Hint: "Run pinax project list to view available projects"}
	if tombstones, loadErr := loadTrashTombstones(root); loadErr == nil {
		if _, ok := tombstones["project/"+slug]; ok {
			err.Hint = fmt.Sprintf("pinax trash restore project/%s --vault %s --json", shellQuote(slug), shellQuote(root))
		}
	}
	return err
}

func applyRemoteTrashDelete(root string, marker remoteTrashDeleteMarker) (remoteTrashDeleteResult, error) {
	objectID := normalizeTrashObjectRef(marker.ObjectID)
	if objectID == "" {
		return remoteTrashDeleteResult{}, nil
	}
	objectKind := strings.TrimSpace(marker.ObjectKind)
	if objectKind == "" {
		objectKind = strings.SplitN(objectID, "/", 2)[0]
	}
	switch objectKind {
	case "project":
		return applyRemoteProjectDelete(root, objectID, marker)
	case "subproject":
		return applyRemoteSubprojectDelete(root, objectID, marker)
	default:
		return remoteTrashDeleteResult{}, nil
	}
}

func applyRemoteProjectDelete(root, objectID string, marker remoteTrashDeleteMarker) (remoteTrashDeleteResult, error) {
	slug, ok := strings.CutPrefix(objectID, "project/")
	if !ok || validateProjectSlug(slug) != nil {
		return remoteTrashDeleteResult{}, nil
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return remoteTrashDeleteResult{}, err
	}
	idx := -1
	var project domain.Project
	for i, item := range registry.Projects {
		if item.Slug == slug {
			idx = i
			project = item
			break
		}
	}
	if idx < 0 {
		return remoteTrashDeleteResult{}, nil
	}
	trashRel, err := uniqueTrashDirRel(root, filepath.ToSlash(filepath.Join(".pinax", "trash", time.Now().UTC().Format("20060102"), "sync", "projects", project.Slug)))
	if err != nil {
		return remoteTrashDeleteResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(trashRel)), 0o755); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	registryBackupRel := filepath.ToSlash(filepath.Join(trashRel, "registry.json"))
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(registryBackupRel)), project); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	contentBackupRel := ""
	hadLocalContent := strings.TrimSpace(project.NotesPrefix) != "" && workspaceHasContent(root, project.NotesPrefix)
	if strings.TrimSpace(project.NotesPrefix) != "" {
		if moved, moveErr := moveIfExists(root, project.NotesPrefix, filepath.ToSlash(filepath.Join(trashRel, "content"))); moveErr != nil {
			return remoteTrashDeleteResult{}, moveErr
		} else if moved {
			contentBackupRel = filepath.ToSlash(filepath.Join(trashRel, "content"))
		}
	}
	registry.Projects = append(registry.Projects[:idx], registry.Projects[idx+1:]...)
	if registry.CurrentProject == project.Slug {
		registry.CurrentProject = ""
		if len(registry.Projects) > 0 {
			registry.CurrentProject = registry.Projects[0].Slug
		}
	}
	if err := saveProjectRegistry(root, registry); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	tombstone := domain.Tombstone{ObjectKind: "project", ObjectID: objectID, TombstoneID: remoteTombstoneID(marker, objectID), OldPath: project.NotesPrefix, Title: project.Name, TrashPath: trashRel, RegistryPath: registryBackupRel, RegistryFacts: map[string]any{"project": project, "content_path": contentBackupRel}, DeletedAt: remoteDeletedAt(marker), Source: "sync.pull.delete", Evidence: []string{registryBackupRel, tombstonesRel}}
	if err := upsertTrashTombstone(root, tombstone); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	_ = appendEvent(root, "sync.pull.delete", "success", map[string]string{"object_id": objectID, "trash_path": trashRel})
	result := remoteTrashDeleteResult{Applied: true}
	if hadLocalContent && contentBackupRel != "" {
		result.Conflict = &domain.SyncConflictEntry{File: contentBackupRel, MainPath: objectID}
	}
	return result, nil
}

func applyRemoteSubprojectDelete(root, objectID string, marker remoteTrashDeleteMarker) (remoteTrashDeleteResult, error) {
	parts := strings.Split(objectID, "/")
	if len(parts) != 3 || parts[0] != "subproject" || validateProjectSlug(parts[1]) != nil || validateSubprojectSlugValue(parts[2]) != nil {
		return remoteTrashDeleteResult{}, nil
	}
	projectSlug, subproject := parts[1], parts[2]
	workspace, err := loadProjectWorkspace(root, projectSlug, subproject)
	if err != nil {
		return remoteTrashDeleteResult{}, nil
	}
	trashRel, err := uniqueTrashDirRel(root, filepath.ToSlash(filepath.Join(".pinax", "trash", time.Now().UTC().Format("20060102"), "sync", "subprojects", projectSlug, subproject)))
	if err != nil {
		return remoteTrashDeleteResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(trashRel)), 0o755); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	workspaceBackupRel := filepath.ToSlash(filepath.Join(trashRel, "workspace"))
	hadLocalContent := workspaceHasContent(root, workspace.WorkspacePath)
	if moved, moveErr := moveIfExists(root, workspace.WorkspacePath, workspaceBackupRel); moveErr != nil {
		return remoteTrashDeleteResult{}, moveErr
	} else if !moved {
		workspaceBackupRel = ""
	}
	registryRel := projectWorkspaceRegistryRel(projectSlug, subproject)
	registryBackupRel := filepath.ToSlash(filepath.Join(trashRel, "registry.json"))
	if moved, moveErr := moveIfExists(root, registryRel, registryBackupRel); moveErr != nil {
		return remoteTrashDeleteResult{}, moveErr
	} else if !moved {
		if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(registryBackupRel)), workspace); err != nil {
			return remoteTrashDeleteResult{}, err
		}
	}
	boardRel := projectBoardConfigRel(projectSlug, subproject)
	boardBackupRel := ""
	if moved, moveErr := moveIfExists(root, boardRel, filepath.ToSlash(filepath.Join(trashRel, "board.json"))); moveErr != nil {
		return remoteTrashDeleteResult{}, moveErr
	} else if moved {
		boardBackupRel = filepath.ToSlash(filepath.Join(trashRel, "board.json"))
	}
	clearCurrentWorkspaceIfMatches(root, projectSlug, subproject)
	tombstone := domain.Tombstone{ObjectKind: "subproject", ObjectID: objectID, TombstoneID: remoteTombstoneID(marker, objectID), OldPath: workspace.WorkspacePath, Title: workspace.Title, TrashPath: trashRel, RegistryPath: registryBackupRel, RegistryFacts: map[string]any{"workspace": workspace, "workspace_backup": workspaceBackupRel, "board_backup": boardBackupRel}, DeletedAt: remoteDeletedAt(marker), Source: "sync.pull.delete", Evidence: []string{registryBackupRel, tombstonesRel}}
	if err := upsertTrashTombstone(root, tombstone); err != nil {
		return remoteTrashDeleteResult{}, err
	}
	_ = appendEvent(root, "sync.pull.delete", "success", map[string]string{"object_id": objectID, "trash_path": trashRel})
	result := remoteTrashDeleteResult{Applied: true}
	if hadLocalContent && workspaceBackupRel != "" {
		result.Conflict = &domain.SyncConflictEntry{File: workspaceBackupRel, MainPath: objectID}
	}
	return result, nil
}

func validateSubprojectSlugValue(slug string) *domain.CommandError {
	_, err := validateSubprojectSlug(slug)
	return err
}

func remoteTombstoneID(marker remoteTrashDeleteMarker, objectID string) string {
	if strings.TrimSpace(marker.TombstoneID) != "" {
		return strings.TrimSpace(marker.TombstoneID)
	}
	return trashID(objectID)
}

func remoteDeletedAt(marker remoteTrashDeleteMarker) string {
	if strings.TrimSpace(marker.DeletedAt) != "" {
		return strings.TrimSpace(marker.DeletedAt)
	}
	return time.Now().UTC().Format(time.RFC3339)
}
