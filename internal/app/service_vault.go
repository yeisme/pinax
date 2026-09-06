package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/vaultops"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	"github.com/yeisme/pinax/internal/identity"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/vaultignore"
)

// Vault, project, storage, and repair operations: vault init/validate/ignore,
// project lifecycle, local/S3 storage profiles, vault stats/doctor, and the repair
// plan/apply workflow with its note-scan and issue helpers. Extracted from service.go
// to isolate the vault-maintenance surface from the rest of the Service facade.

func (s *Service) InitVault(_ context.Context, req InitVaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.init", err), err
	}
	config := filepath.Join(root, ".pinax", "config.yaml")
	if _, err := os.Stat(config); err == nil {
		commandErr := &domain.CommandError{Code: "vault_already_initialized", Message: "Pinax vault is already initialized", Hint: fmt.Sprintf("Run pinax vault validate --vault %s to check the current vault", shellQuote(root))}
		return errorProjection("vault.init", commandErr), commandErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorProjection("vault.init", err), err
	}
	if req.Title == "" {
		req.Title = filepath.Base(root)
	}
	for _, dir := range []string{filepath.Join(root, "notes"), filepath.Join(root, ".pinax")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errorProjection("vault.init", err), err
		}
	}
	content := fmt.Sprintf("schema_version: pinax.config.v1\ntitle: %q\n", req.Title)
	if err := os.WriteFile(config, []byte(content), 0o644); err != nil {
		return errorProjection("vault.init", err), err
	}
	if err := ensureDefaultIgnoreFiles(root); err != nil {
		return errorProjection("vault.init", err), err
	}
	if err := ensureEventLog(root); err != nil {
		return errorProjection("vault.init", err), err
	}
	appendEventWarned(root, "vault.init", "success", map[string]string{"title": req.Title})

	projection := domain.NewProjection("vault.init", "Pinax vault initialized.")
	projection.Facts["vault"] = root
	projection.Facts["title"] = req.Title
	projection.Evidence = []string{".pinax/config.yaml", ".pinaxignore", ".gitignore", ".pinax/events.jsonl"}
	projection.Actions = []domain.Action{{Name: "validate", Command: fmt.Sprintf("pinax vault validate --vault %s", shellQuote(root))}}
	return projection, nil
}

func ensureDefaultIgnoreFiles(root string) error {
	if err := writeFileIfMissing(filepath.Join(root, vaultignore.PinaxIgnoreName), vaultignore.DefaultPinaxIgnore(), 0o644); err != nil {
		return err
	}
	return writeFileIfMissing(filepath.Join(root, ".gitignore"), vaultignore.MetadataOnlyGitignoreBlock(), 0o644)
}

func writeFileIfMissing(path, body string, perm os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), perm)
}

func (s *Service) VaultIgnoreStatus(_ context.Context, req VaultIgnoreRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.ignore.status", err), err
	}
	stats, err := vaultIgnoreStats(root)
	if err != nil {
		return errorProjection("vault.ignore.status", err), err
	}
	projection := domain.NewProjection("vault.ignore.status", "Vault ignore status read.")
	addVaultIgnoreFacts(&projection, stats)
	projection.Data = stats
	return projection, nil
}

func (s *Service) VaultIgnorePlan(_ context.Context, req VaultIgnoreRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.ignore.plan", err), err
	}
	stats, err := vaultIgnoreStats(root)
	if err != nil {
		return errorProjection("vault.ignore.plan", err), err
	}
	ops := vaultIgnoreOperations(stats)
	projection := domain.NewProjection("vault.ignore.plan", "Vault ignore repair plan generated.")
	projection.Facts["writes"] = "false"
	projection.Facts["operations"] = fmt.Sprint(len(ops))
	addVaultIgnoreFacts(&projection, stats)
	if len(ops) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax vault ignore apply --vault %s --yes --json", shellQuote(root))}}
	}
	projection.Data = map[string]any{"operations": ops, "status": stats}
	return projection, nil
}

func (s *Service) VaultIgnoreApply(_ context.Context, req VaultIgnoreRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "vault ignore apply requires explicit approval", Hint: "Rerun with --yes after reviewing the ignore plan"}
		return domain.NewErrorProjection("vault.ignore.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.ignore.apply", err), err
	}
	stats, err := vaultIgnoreStats(root)
	if err != nil {
		return errorProjection("vault.ignore.apply", err), err
	}
	ops := vaultIgnoreOperations(stats)
	if stats.PinaxIgnore == "missing" {
		if err := os.WriteFile(filepath.Join(root, vaultignore.PinaxIgnoreName), []byte(vaultignore.DefaultPinaxIgnore()), 0o644); err != nil {
			return errorProjection("vault.ignore.apply", err), err
		}
	}
	if stats.GitMetadataOnly == "missing" {
		gitPath := filepath.Join(root, ".gitignore")
		existing, readErr := os.ReadFile(gitPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return errorProjection("vault.ignore.apply", readErr), readErr
		}
		updated := vaultignore.ApplyMetadataOnlyGitignore(string(existing))
		if err := os.WriteFile(gitPath, []byte(updated), 0o644); err != nil {
			return errorProjection("vault.ignore.apply", err), err
		}
	}
	appendEventWarned(root, "vault.ignore.apply", "success", map[string]string{"operations": fmt.Sprint(len(ops))})
	projection := domain.NewProjection("vault.ignore.apply", "Vault ignore configuration updated.")
	projection.Facts["local_write"] = "true"
	projection.Facts["operations"] = fmt.Sprint(len(ops))
	projection.Evidence = []string{".pinaxignore", ".gitignore", filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"operations": ops}
	return projection, nil
}

type vaultIgnoreStatus struct {
	PinaxIgnore     string `json:"pinaxignore"`
	GitMetadataOnly string `json:"git_metadata_only"`
	ContentFiles    int    `json:"content_files"`
	IgnoredFiles    int    `json:"ignored_files"`
	ContentBytes    int64  `json:"content_bytes"`
	BinaryFiles     int    `json:"binary_files"`
	ScriptFiles     int    `json:"script_files"`
}

func vaultIgnoreStats(root string) (vaultIgnoreStatus, error) {
	status := vaultIgnoreStatus{PinaxIgnore: "present", GitMetadataOnly: "present"}
	if _, err := os.Stat(filepath.Join(root, vaultignore.PinaxIgnoreName)); errors.Is(err, os.ErrNotExist) {
		status.PinaxIgnore = "missing"
	} else if err != nil {
		return status, err
	}
	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if errors.Is(err, os.ErrNotExist) {
		status.GitMetadataOnly = "missing"
	} else if err != nil {
		return status, err
	} else if !strings.Contains(string(gitignore), "# BEGIN PINAX METADATA-ONLY") || !strings.Contains(string(gitignore), "# END PINAX METADATA-ONLY") {
		status.GitMetadataOnly = "missing"
	}
	matcher, err := vaultignore.Load(root)
	if err != nil {
		return status, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if matcher.Ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || matcher.Ignored(rel, false) {
			status.IgnoredFiles++
			return nil
		}
		status.ContentFiles++
		status.ContentBytes += info.Size()
		if strings.HasPrefix(rel, "scripts/") || info.Mode().Perm()&0o111 != 0 {
			status.ScriptFiles++
			return nil
		}
		if !isManifestText(rel) {
			status.BinaryFiles++
		}
		return nil
	})
	return status, err
}

func vaultIgnoreOperations(status vaultIgnoreStatus) []domain.PlanOperation {
	ops := make([]domain.PlanOperation, 0, 2)
	if status.PinaxIgnore == "missing" {
		ops = append(ops, domain.PlanOperation{Kind: "write_pinaxignore", Path: ".pinaxignore", Reason: "Create Pinax content ignore rules", Status: "planned"})
	}
	if status.GitMetadataOnly == "missing" {
		ops = append(ops, domain.PlanOperation{Kind: "patch_gitignore", Path: ".gitignore", Reason: "Add Pinax metadata-only Git ignore block", Status: "planned"})
	}
	return ops
}

func addVaultIgnoreFacts(projection *domain.Projection, status vaultIgnoreStatus) {
	projection.Facts["pinaxignore"] = status.PinaxIgnore
	projection.Facts["git_metadata_only"] = status.GitMetadataOnly
	projection.Facts["content_files"] = fmt.Sprint(status.ContentFiles)
	projection.Facts["ignored_files"] = fmt.Sprint(status.IgnoredFiles)
	projection.Facts["content_bytes"] = fmt.Sprint(status.ContentBytes)
	projection.Facts["binary_files"] = fmt.Sprint(status.BinaryFiles)
	projection.Facts["script_files"] = fmt.Sprint(status.ScriptFiles)
}

func (s *Service) ValidateVault(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.validate", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("vault.validate", err), err
	}
	notes = ordinaryNotes(notes)
	issues := make([]domain.Issue, 0)
	for _, required := range []string{".pinax/config.yaml", ".pinax/events.jsonl"} {
		if _, err := os.Stat(filepath.Join(root, required)); err != nil {
			issues = append(issues, domain.Issue{Code: "missing_asset", Path: required, Message: "Missing Pinax machine asset"})
		}
	}
	for _, note := range notes {
		if note.ID == "" {
			issues = append(issues, domain.Issue{Code: "missing_note_id", Path: note.Path, Message: "Missing note_id"})
		}
	}
	issues = append(issues, validateProjectBoardAssets(root)...)
	projection := domain.NewProjection("vault.validate", "Vault validation completed.")
	projection.Facts["vault"] = root
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Data = map[string]any{"issues": issues}
	if len(issues) > 0 {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "metadata_plan", Command: fmt.Sprintf("pinax metadata plan --vault %s", shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "note_list", Command: fmt.Sprintf("pinax note list --vault %s", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) CreateProject(_ context.Context, req ProjectRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.create", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("project.create", err), err
	}
	if err := validateProjectSlug(req.Slug); err != nil {
		return errorProjection("project.create", err), err
	}
	if req.Name == "" {
		req.Name = req.Slug
	}
	if req.NotesPrefix == "" {
		req.NotesPrefix = filepath.ToSlash(filepath.Join("notes", req.Slug))
	}
	if err := validateProjectPrefix(req.NotesPrefix); err != nil {
		return errorProjection("project.create", err), err
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return errorProjection("project.create", err), err
	}
	projectObjectID, err := s.allocateObjectID(identity.KindProject, root, req.Slug)
	if err != nil {
		return errorProjection("project.create", err), err
	}
	project := domain.Project{ObjectID: projectObjectID, Slug: req.Slug, Name: req.Name, Description: req.Description, NotesPrefix: filepath.ToSlash(req.NotesPrefix), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	for i, existing := range registry.Projects {
		if existing.Slug != req.Slug {
			continue
		}
		if existing.Name != project.Name || existing.Description != project.Description || existing.NotesPrefix != project.NotesPrefix {
			err := &domain.CommandError{Code: "project_conflict", Message: "Project slug already exists with a different definition", Hint: "Choose another slug, or inspect pinax project list first"}
			return domain.NewErrorProjection("project.create", err), err
		}
		project.CreatedAt = existing.CreatedAt
		if strings.TrimSpace(existing.ObjectID) != "" {
			project.ObjectID = existing.ObjectID
		}
		registry.Projects[i] = project
		return saveProjectRegistryProjection(root, registry, project, false)
	}
	registry.Projects = append(registry.Projects, project)
	if registry.CurrentProject == "" {
		registry.CurrentProject = project.Slug
	}
	return saveProjectRegistryProjection(root, registry, project, true)
}

func (s *Service) ListProjects(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.list", err), err
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return errorProjection("project.list", err), err
	}
	projection := domain.NewProjection("project.list", "Project list read.")
	projection.Facts["vault"] = root
	projection.Facts["projects"] = fmt.Sprint(len(registry.Projects))
	if registry.CurrentProject != "" {
		projection.Facts["current_project"] = registry.CurrentProject
	}
	for i, project := range registry.Projects {
		prefix := fmt.Sprintf("project.%d.", i+1)
		projection.Facts[prefix+"slug"] = project.Slug
		projection.Facts[prefix+"name"] = project.Name
		projection.Facts[prefix+"notes_prefix"] = project.NotesPrefix
		if project.Description != "" {
			projection.Facts[prefix+"description"] = project.Description
		}
		if project.CreatedAt != "" {
			projection.Facts[prefix+"created_at"] = project.CreatedAt
		}
	}
	projection.Data = map[string]any{"registry": registry}
	projection.Actions = []domain.Action{{Name: "create", Command: fmt.Sprintf("pinax project create <slug> --vault %s", shellQuote(root))}}
	return projection, nil
}

func (s *Service) ProjectShow(_ context.Context, req ProjectRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.show", err), err
	}
	project, err := findProject(root, req.Slug)
	if err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok && commandErr.Code == "project_not_found" {
			restoreErr := projectNotFoundWithRestore(root, req.Slug)
			projection := domain.NewErrorProjection("project.show", restoreErr)
			if restoreErr.Hint != commandErr.Hint {
				projection.Actions = []domain.Action{{Name: "restore", Command: restoreErr.Hint}}
			}
			return projection, restoreErr
		}
		return errorProjection("project.show", err), err
	}
	projection := domain.NewProjection("project.show", "Project read.")
	projection.Facts["project"] = project.Slug
	projection.Facts["name"] = project.Name
	projection.Facts["notes_prefix"] = project.NotesPrefix
	if project.Description != "" {
		projection.Facts["description"] = project.Description
	}
	projection.Data = map[string]any{"project": project}
	projection.Actions = []domain.Action{{Name: "subprojects", Command: fmt.Sprintf("pinax project subproject list %s --vault %s --json", shellQuote(project.Slug), shellQuote(root))}}
	return projection, nil
}

func (s *Service) SwitchProject(_ context.Context, req ProjectRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("project.switch", err), err
	}
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return errorProjection("project.switch", err), err
	}
	var project domain.Project
	found := false
	for _, item := range registry.Projects {
		if item.Slug == req.Slug {
			project = item
			found = true
			break
		}
	}
	if !found {
		err := &domain.CommandError{Code: "project_not_found", Message: "Project not found", Hint: "Run pinax project list to view available projects"}
		return domain.NewErrorProjection("project.switch", err), err
	}
	registry.CurrentProject = req.Slug
	if err := saveProjectRegistry(root, registry); err != nil {
		return errorProjection("project.switch", err), err
	}
	appendEventWarned(root, "project.switch", "success", map[string]string{"project": req.Slug})
	projection := domain.NewProjection("project.switch", "Current project switched.")
	projection.Facts["project"] = project.Slug
	projection.Facts["notes_prefix"] = project.NotesPrefix
	projection.Actions = []domain.Action{{Name: "note_list", Command: fmt.Sprintf("pinax note list --vault %s", shellQuote(root))}}
	return projection, nil
}

func (s *Service) SetLocalStorage(_ context.Context, req StorageRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.set_local", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("storage.set_local", err), err
	}
	storageRoot := req.Root
	if storageRoot == "" {
		storageRoot = root
	}
	profile := domain.StorageProfile{SchemaVersion: "pinax.storage.v1", Backend: "local", Local: &domain.LocalStorage{Root: storageRoot}}
	if err := saveStorageProfile(root, profile); err != nil {
		return errorProjection("storage.set_local", err), err
	}
	appendEventWarned(root, "storage.set_local", "success", map[string]string{"backend": "local"})
	return storageProjection("storage.set_local", "Local storage backend configured.", profile), nil
}

func (s *Service) SetS3Storage(_ context.Context, req StorageRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.set_s3", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("storage.set_s3", err), err
	}
	if strings.TrimSpace(req.Bucket) == "" || strings.TrimSpace(req.Region) == "" {
		err := &domain.CommandError{Code: "s3_config_incomplete", Message: "S3 backend requires bucket and region", Hint: "Rerun pinax storage set-s3 --bucket <bucket> --region <region>"}
		return domain.NewErrorProjection("storage.set_s3", err), err
	}
	profile := domain.StorageProfile{SchemaVersion: "pinax.storage.v1", Backend: "s3", S3: &domain.S3Storage{Bucket: req.Bucket, Region: req.Region, Prefix: req.Prefix, Endpoint: req.Endpoint, Profile: req.Profile}}
	if err := saveStorageProfile(root, profile); err != nil {
		return errorProjection("storage.set_s3", err), err
	}
	appendEventWarned(root, "storage.set_s3", "success", map[string]string{"backend": "s3", "bucket": req.Bucket, "region": req.Region})
	projection := storageProjection("storage.set_s3", "S3 storage backend configured.", profile)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax storage doctor --vault %s", shellQuote(root))}}
	return projection, nil
}

func (s *Service) StorageStatus(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.status", err), err
	}
	profile, err := loadStorageProfile(root)
	if err != nil {
		return errorProjection("storage.status", err), err
	}
	return storageProjection("storage.status", "Storage backend status read.", profile), nil
}

func (s *Service) StorageDoctor(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("storage.doctor", err), err
	}
	profile, err := loadStorageProfile(root)
	if err != nil {
		return errorProjection("storage.doctor", err), err
	}
	projection := storageProjection("storage.doctor", "Storage backend diagnostics completed.", profile)
	issues := make([]domain.Issue, 0)
	if profile.Backend == "s3" {
		if profile.S3 == nil || profile.S3.Bucket == "" {
			issues = append(issues, domain.Issue{Code: "missing_bucket", Path: ".pinax/storage.json", Message: "Missing S3 bucket"})
		}
		if profile.S3 == nil || profile.S3.Region == "" {
			issues = append(issues, domain.Issue{Code: "missing_region", Path: ".pinax/storage.json", Message: "Missing S3 region"})
		}
	}
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Data = map[string]any{"storage": profile, "issues": issues, "network_checked": false}
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	return projection, nil
}

func (s *Service) VaultStats(_ context.Context, req VaultStatsRequest) (domain.Projection, error) {
	started := time.Now()
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.stats", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("vault.stats", err), err
	}
	facts = ordinaryNoteFacts(facts)
	stats := vaultops.Stats(root, toVaultOpsFacts(facts), time.Since(started))
	projection := domain.NewProjection("vault.stats", "Vault statistics generated.")
	projection.Facts["vault"] = root
	projection.Facts["notes"] = fmt.Sprint(stats.NoteCount)
	projection.Facts["tags"] = fmt.Sprint(stats.TagCount)
	projection.Facts["frontmatter_coverage"] = fmt.Sprint(stats.FrontmatterCoverage)
	projection.Facts["recent_updates"] = fmt.Sprint(stats.RecentUpdates)
	projection.Facts["index_status"] = stats.IndexStatus
	projection.Facts["scan_duration_ms"] = fmt.Sprint(stats.ScanDurationMillis)
	if stats.IndexStatus != "fresh" {
		projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
	}
	projection.Data = stats
	return projection, nil
}

func (s *Service) VaultDoctor(_ context.Context, req VaultDoctorRequest) (domain.Projection, error) {
	started := time.Now()
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("vault.doctor", err), err
	}
	if req.StaleAfter <= 0 {
		req.StaleAfter = 90 * 24 * time.Hour
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("vault.doctor", err), err
	}
	stats := vaultops.Stats(root, toVaultOpsFacts(facts), time.Since(started))
	facts = ordinaryNoteFacts(facts)
	issues := VaultHealthService{}.Issues(root, facts, stats, req.StaleAfter)
	issues = append(issues, projectTrashLifecycleIssues(root)...)
	report := domain.VaultDoctorReport{VaultPath: root, Issues: issues, Counts: countIssuesBySeverity(issues), Stats: stats}
	projection := domain.NewProjection("vault.doctor", "Vault health check completed.")
	projection.Facts["vault"] = root
	projection.Facts["issues.total"] = fmt.Sprint(len(issues))
	for severity, count := range report.Counts {
		projection.Facts["issues."+severity] = fmt.Sprint(count)
	}
	projection.Data = report
	if len(issues) > 0 {
		projection.Status = "partial"
		projection.Actions = nextActionsFromIssues(issues)
	} else {
		projection.Actions = []domain.Action{{Name: "stats", Command: fmt.Sprintf("pinax vault stats --vault %s", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) PlanRepair(ctx context.Context, req RepairPlanRequest) (domain.Projection, error) {
	started := time.Now()
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("repair.plan", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("repair.plan", err), err
	}
	elapsed := time.Since(started)
	facts = ordinaryNoteFacts(facts)
	stats := vaultops.Stats(root, toVaultOpsFacts(facts), elapsed)
	issues := VaultHealthService{}.Issues(root, facts, stats, 90*24*time.Hour)
	issues = append(issues, projectTrashLifecycleIssues(root)...)
	issues = append(issues, assetAndVersionRepairIssues(root, issues)...)
	plan := buildRepairPlan(root, facts, stats, issues, elapsed)
	if err := bindRepairPlanObjects(ctx, root, &plan); err != nil {
		return errorProjection("repair.plan", err), err
	}
	if req.Save {
		if err := saveRepairPlan(root, &plan); err != nil {
			return errorProjection("repair.plan", err), err
		}
	}
	projection := domain.NewProjection("repair.plan", "Repair plan generated.")
	projection.Facts["vault"] = root
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations.total"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["operations.automatic"] = fmt.Sprint(countRepairOperations(plan.Operations, "automatic"))
	projection.Facts["operations.manual_review"] = fmt.Sprint(countRepairOperations(plan.Operations, "manual_review"))
	projection.Facts["skipped_issues"] = fmt.Sprint(len(plan.SkippedIssues))
	projection.Facts["scan_duration_ms"] = fmt.Sprint(plan.ScanDurationMillis)
	if plan.SavedPath != "" {
		projection.Facts["saved_path"] = plan.SavedPath
		projection.Evidence = []string{plan.SavedPath}
	}
	projection.Data = plan
	if len(plan.Operations) > 0 || len(plan.SkippedIssues) > 0 {
		projection.Status = "partial"
		if plan.SavedPath != "" {
			projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(plan.PlanID))}}
		} else {
			projection.Actions = []domain.Action{{Name: "save", Command: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}}
		}
	} else {
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax vault doctor --vault %s", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ApplyRepair(ctx context.Context, req RepairApplyRequest) (domain.Projection, error) {
	tracker := newPipelineStageTracker(domain.PipelineKindRepair, strings.TrimSpace(req.PlanID), "", "apply")
	projection, err := s.applyRepair(ctx, req, tracker)
	tracker.finish(&projection, err, pipelineStageCounts(projection, "applied", "skipped"))
	return projection, err
}

func (s *Service) applyRepair(ctx context.Context, req RepairApplyRequest, tracker *pipelineStageTracker) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "repair apply requires --yes", Hint: "Run pinax repair plan --save first, then add --yes after confirming"}
		return domain.NewErrorProjection("repair.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("repair.apply", err), err
	}
	if strings.TrimSpace(req.PlanID) == "" {
		err := &domain.CommandError{Code: "plan_required", Message: "repair apply requires --plan", Hint: "pinax repair apply --vault <vault> --plan <plan_id> --yes"}
		return domain.NewErrorProjection("repair.apply", err), err
	}
	plan, err := loadRepairPlan(root, req.PlanID)
	if err != nil {
		return errorProjection("repair.apply", err), err
	}
	tracker.begin()
	staleOverridden := false
	if err := ensureRepairPlanFresh(ctx, root, &plan); err != nil {
		if domain.ErrorCode(err) == "plan_stale" && req.AllowStale {
			// --allow-stale 逃生门：跳过 freshness 守卫，投影附 warning。
			staleOverridden = true
		} else {
			projection := errorProjection("repair.apply", err)
			projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}}
			projection.Data = map[string]any{"plan_id": plan.PlanID}
			return projection, err
		}
	}
	beforeBindingsByPath, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return errorProjection("repair.apply", err), err
	}
	beforeBindings := managedBindingsByObject(beforeBindingsByPath)
	snapshotID := ""
	requiresSnapshot := repairPlanRequiresSnapshot(plan)
	if req.SnapshotMessage != "" && requiresSnapshot {
		if _, err := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: req.SnapshotMessage}); err != nil {
			return errorProjection("repair.apply", err), err
		}
		snapshotID = filepath.ToSlash(filepath.Join(".pinax", "last_snapshot"))
	}
	if requiresSnapshot && !gitstore.HasSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "Applying a repair plan requires an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before repair"))}
		projection := domain.NewErrorProjection("repair.apply", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		projection.Data = map[string]any{"plan_id": plan.PlanID}
		return projection, err
	}
	applied := make([]domain.RepairOperation, 0)
	skipped := make([]domain.RepairOperation, 0)
	changedPaths := make([]string, 0)
	for _, op := range plan.Operations {
		if op.Mode != "automatic" {
			op.Status = "skipped"
			skipped = append(skipped, op)
			appendEventWarned(root, "repair.apply", "skipped", map[string]string{"plan_id": plan.PlanID, "operation_id": op.OperationID, "kind": op.Kind, "reason": "manual_review"})
			continue
		}
		if err := s.applyRepairOperation(ctx, root, op); err != nil {
			return errorProjection("repair.apply", err), err
		}
		op.Status = "applied"
		applied = append(applied, op)
		if op.Path != "" {
			changedPaths = append(changedPaths, op.Path)
		}
		appendEventWarned(root, "repair.apply", "success", map[string]string{"plan_id": plan.PlanID, "operation_id": op.OperationID, "kind": op.Kind})
	}
	projection := domain.NewProjection("repair.apply", "Repair plan applied.")
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations.total"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["applied"] = fmt.Sprint(len(applied))
	projection.Facts["skipped"] = fmt.Sprint(len(skipped))
	if staleOverridden {
		projection.Facts["allow_stale"] = "true"
		projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{Code: "plan_stale_overridden", Message: "stale plan applied via --allow-stale", Hint: "vault facts changed after this plan was generated"})
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	receipt, receiptErr := writeObjectApplyReceipt(ctx, root, "repair.apply", plan.PlanID, snapshotID, beforeBindings, changedPaths)
	if receiptErr != nil {
		return errorProjection("repair.apply", receiptErr), receiptErr
	}
	addApplyReceiptProjection(&projection, receipt)
	projection.Data = map[string]any{"plan_id": plan.PlanID, "results": applied, "skipped": skipped, "receipt": receipt}
	return projection, nil
}

func (s *Service) ListRepairPlans(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("repair.list", err), err
	}
	plans, err := listRepairPlans(root)
	if err != nil {
		return errorProjection("repair.list", err), err
	}
	projection := domain.NewProjection("repair.list", "Repair plans read.")
	projection.Facts["plans"] = fmt.Sprint(len(plans))
	projection.Data = map[string]any{"plans": plans}
	if len(plans) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax repair apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(plans[0].PlanID))}}
	}
	return projection, nil
}

func (s *Service) applyRepairOperation(ctx context.Context, root string, op domain.RepairOperation) error {
	switch op.Kind {
	case "metadata_patch", "tags_patch":
		return s.applyRepairMetadataPatch(ctx, root, op.Path)
	case "archive_status_patch":
		return applyRepairFrontmatterPatch(root, op.Path, map[string]string{"status": "archived"})
	case "index_rebuild":
		_, err := s.RebuildIndex(ctx, VaultRequest{VaultPath: root})
		return err
	default:
		return nil
	}
}

type VaultHealthService struct{}

func (VaultHealthService) Issues(root string, facts []noteFact, stats domain.VaultStats, staleAfter time.Duration) []domain.VaultIssue {
	return buildVaultIssues(root, facts, stats, staleAfter)
}

type noteFact struct {
	note           domain.Note
	meta           map[string]string
	rel            string
	modTime        time.Time
	size           int64
	hasFrontmatter bool
}

func toVaultOpsFacts(facts []noteFact) []vaultops.Fact {
	out := make([]vaultops.Fact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, vaultops.Fact{Note: fact.note, Meta: fact.meta, Rel: fact.rel, ModTime: fact.modTime, Size: fact.size, HasFrontmatter: fact.hasFrontmatter})
	}
	return out
}

func scanNoteFacts(root string) ([]noteFact, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, err
	}
	facts := make([]noteFact, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		meta, body := splitFrontmatter(string(content))
		if !isPinaxNoteFrontmatter(meta) {
			return nil
		}
		rel = filepath.ToSlash(rel)
		note := parseNote(rel, string(content))
		if isSystemIndexNote(note) {
			return nil
		}
		if note.UpdatedAt == "" {
			note.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		// 这里保留 frontmatter 是否真实存在的事实，避免把文件名推导出的 title 误判为机器 metadata。
		facts = append(facts, noteFact{note: note, meta: meta, rel: rel, modTime: info.ModTime(), size: info.Size(), hasFrontmatter: strings.HasPrefix(string(content), "---\n") && body != string(content)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].rel < facts[j].rel })
	return facts, nil
}

func buildVaultIssues(root string, facts []noteFact, stats domain.VaultStats, staleAfter time.Duration) []domain.VaultIssue {
	issues := make([]domain.VaultIssue, 0)
	titles := map[string][]noteFact{}
	notes := notesFromFacts(facts)
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	for _, fact := range facts {
		titles[strings.ToLower(strings.TrimSpace(fact.note.Title))] = append(titles[strings.ToLower(strings.TrimSpace(fact.note.Title))], fact)
	}
	for _, fact := range facts {
		nextMetadata := []domain.Action{{Name: "metadata_plan", Command: fmt.Sprintf("pinax metadata plan --vault %s", shellQuote(root))}}
		if fact.note.Title == "" || strings.TrimSuffix(filepath.Base(fact.rel), filepath.Ext(fact.rel)) == fact.note.Title && !strings.Contains(fact.note.Body, "# ") && fact.meta["title"] == "" {
			issues = append(issues, vaultIssue("missing_title", "warning", fact, "Note is missing an explicit title", []string{"Both frontmatter.title and an H1 heading are missing"}, nextMetadata))
		}
		if len(fact.note.Tags) == 0 {
			issues = append(issues, vaultIssue("missing_tags", "info", fact, "Note is missing tags", []string{"frontmatter.tags is empty"}, nextMetadata))
		}
		if fact.meta["schema_version"] != "pinax.note.v1" || fact.meta["note_id"] == "" {
			issues = append(issues, vaultIssue("missing_pinax_metadata", "warning", fact, "Note is missing Pinax metadata", []string{"Requires schema_version=pinax.note.v1 and note_id"}, nextMetadata))
		}
		if strings.TrimSpace(fact.note.Body) == "" {
			issues = append(issues, vaultIssue("empty_note", "warning", fact, "Note body is empty", []string{"No body after frontmatter"}, []domain.Action{{Name: "edit", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(fact.rel), shellQuote(root))}}))
		}
		if time.Since(fact.modTime) > staleAfter {
			issues = append(issues, vaultIssue("stale_note", "info", fact, "Note has not been updated for a long time", []string{fmt.Sprintf("mtime=%s", fact.modTime.UTC().Format(time.RFC3339))}, []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(fact.rel), shellQuote(root))}}))
		}
		for _, link := range outgoing[fact.rel] {
			issues = append(issues, linkIssueForFact(root, fact, link)...)
		}
		if len(outgoing[fact.rel]) == 0 && len(incoming[fact.rel]) == 0 {
			issues = append(issues, vaultIssue("orphan_note", "info", fact, "Note has no bidirectional links", []string{"title=" + fact.note.Title, "graph=incoming:0,outgoing:0"}, []domain.Action{{Name: "organize_plan", Command: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root))}}))
		}
		cleanRel := filepath.ToSlash(filepath.Clean(fact.rel))
		if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.HasPrefix(cleanRel, ".pinax/") || filepath.IsAbs(fact.rel) {
			issues = append(issues, vaultIssue("path_anomaly", "error", fact, "Note path is unusual", []string{fact.rel}, nil))
		}
	}
	for _, group := range titles {
		if len(group) <= 1 || strings.TrimSpace(group[0].note.Title) == "" {
			continue
		}
		for _, fact := range group {
			issues = append(issues, vaultIssue("duplicate_title", "warning", fact, "Duplicate title exists", []string{"title=" + fact.note.Title}, []domain.Action{{Name: "organize_plan", Command: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root))}}))
		}
	}
	if stats.IndexStatus != "fresh" {
		issues = append(issues, domain.VaultIssue{Code: "index_stale", Severity: "warning", Message: "Local index is missing or stale", Evidence: []string{"index_status=" + stats.IndexStatus}, NextActions: []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}})
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity == issues[j].Severity {
			return issues[i].Code < issues[j].Code
		}
		return severityRank(issues[i].Severity) > severityRank(issues[j].Severity)
	})
	return issues
}

func notesFromFacts(facts []noteFact) []domain.Note {
	notes := make([]domain.Note, 0, len(facts))
	for _, fact := range facts {
		notes = append(notes, fact.note)
	}
	return notes
}
func assetAndVersionRepairIssues(root string, baseIssues []domain.VaultIssue) []domain.VaultIssue {
	issues := make([]domain.VaultIssue, 0)
	action := []domain.Action{{Name: "asset_repair_plan", Command: fmt.Sprintf("pinax asset repair --plan --vault %s --json", shellQuote(root))}}
	verify, err := pinaxassets.Verify(root)
	if err == nil {
		for _, result := range verify.Results {
			switch result.Status {
			case "missing":
				issues = append(issues, assetVaultIssue("asset_missing", "error", result.Asset.Path, "Asset manifest points to a missing file", assetEvidence(result.Asset, "status=missing"), action))
			case "changed":
				issues = append(issues, assetVaultIssue("asset_hash_changed", "warning", result.Asset.Path, "Asset content hash does not match the manifest", assetEvidence(result.Asset, "status=changed", "actual_sha256="+result.SHA256), action))
			}
		}
	}
	links, _, linkErr := noteindex.ListAssetLinks(root)
	if linkErr == nil {
		linked := map[string]bool{}
		for _, link := range links {
			if link.Status == "resolved" {
				linked[link.AssetPath] = true
			}
			if link.Status == "missing" {
				issues = append(issues, assetVaultIssue("dangling_asset_link", "warning", link.AssetPath, "Note attachment reference points to a missing asset", []string{"source=" + link.SourcePath, "raw=" + link.RawReference, fmt.Sprintf("line=%d", link.Line), "status=" + link.Status}, action))
			}
		}
		if manifest, err := pinaxassets.Load(root); err == nil {
			for _, asset := range manifest.Assets {
				if !linked[asset.Path] {
					issues = append(issues, assetVaultIssue("orphan_manifest_entry", "info", asset.Path, "Asset manifest entry has no resolved note reference", assetEvidence(asset, "linked_notes=0"), action))
				}
			}
		}
	}
	if len(baseIssues)+len(issues) > 0 && versionEvidenceMissing(root) {
		issues = append(issues, domain.VaultIssue{Code: "version_evidence_missing", Severity: "warning", Message: "Current vault lacks version snapshot evidence", Evidence: []string{"snapshot=missing"}, NextActions: []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before repair"))}}})
	}
	return issues
}

func assetVaultIssue(code, severity, path, message string, evidence []string, actions []domain.Action) domain.VaultIssue {
	return domain.VaultIssue{Code: code, Severity: severity, Path: path, Message: message, Evidence: evidence, NextActions: actions}
}

func projectTrashLifecycleIssues(root string) []domain.VaultIssue {
	issues := []domain.VaultIssue{}
	registry, err := loadProjectRegistry(root)
	if err == nil {
		for _, project := range registry.Projects {
			workspaces, listErr := listProjectWorkspaces(root, project.Slug)
			if listErr != nil {
				issues = append(issues, domain.VaultIssue{Code: "project_workspace_registry_unreadable", Severity: "warning", Path: filepath.ToSlash(filepath.Join(".pinax", "project-workspaces", project.Slug)), Message: "Project workspace registry could not be read", Evidence: []string{"project=" + project.Slug, "error=" + listErr.Error()}, NextActions: []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}})
				continue
			}
			for _, workspace := range workspaces {
				if strings.TrimSpace(workspace.WorkspacePath) == "" {
					continue
				}
				workspacePath, pathErr := safeJoin(root, workspace.WorkspacePath)
				if pathErr != nil {
					issues = append(issues, domain.VaultIssue{Code: "project_workspace_path_invalid", Severity: "warning", Path: workspace.WorkspacePath, Message: "Project workspace path is invalid", Evidence: []string{"project=" + project.Slug, "subproject=" + workspace.Subproject}, NextActions: []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}})
					continue
				}
				if info, statErr := os.Stat(workspacePath); statErr != nil || !info.IsDir() {
					evidence := []string{"project=" + project.Slug, "subproject=" + workspace.Subproject, "workspace_path=" + workspace.WorkspacePath}
					if statErr != nil {
						evidence = append(evidence, "error="+statErr.Error())
					}
					issues = append(issues, domain.VaultIssue{Code: "project_workspace_missing", Severity: "warning", Path: workspace.WorkspacePath, Message: "Active project workspace path is missing", Evidence: evidence, NextActions: []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}})
				}
			}
		}
	}
	tombstones, err := loadTrashTombstones(root)
	if err != nil {
		issues = append(issues, domain.VaultIssue{Code: "trash_tombstones_unreadable", Severity: "warning", Path: tombstonesRel, Message: "Trash tombstones could not be read", Evidence: []string{"error=" + err.Error()}, NextActions: []domain.Action{{Name: "trash_list", Command: fmt.Sprintf("pinax trash list --vault %s --json", shellQuote(root))}}})
		return issues
	}
	for objectID, tombstone := range tombstones {
		if tombstone.RestoredAt != "" || strings.TrimSpace(tombstone.TrashPath) == "" {
			continue
		}
		trashPath, pathErr := safeJoin(root, tombstone.TrashPath)
		evidence := []string{"object_id=" + defaultString(trashObjectID(tombstone), objectID), "object_kind=" + trashObjectKind(tombstone), "trash_path=" + tombstone.TrashPath}
		if pathErr != nil {
			evidence = append(evidence, "error="+pathErr.Error())
			issues = append(issues, domain.VaultIssue{Code: "trash_backup_missing", Severity: "warning", Path: tombstone.TrashPath, Message: "Trash tombstone points to an invalid backup path", Evidence: evidence, NextActions: []domain.Action{{Name: "trash_list", Command: fmt.Sprintf("pinax trash list --vault %s --json", shellQuote(root))}}})
			continue
		}
		if _, statErr := os.Stat(trashPath); statErr != nil {
			evidence = append(evidence, "error="+statErr.Error())
			issues = append(issues, domain.VaultIssue{Code: "trash_backup_missing", Severity: "warning", Path: tombstone.TrashPath, Message: "Trash tombstone backup is missing", Evidence: evidence, NextActions: []domain.Action{{Name: "trash_list", Command: fmt.Sprintf("pinax trash list --vault %s --json", shellQuote(root))}}})
		}
	}
	return issues
}

func assetEvidence(asset domain.Asset, extra ...string) []string {
	evidence := []string{"asset_id=" + asset.ID, "path=" + asset.Path, "media_type=" + asset.MediaType}
	if asset.SHA256 != "" {
		evidence = append(evidence, "manifest_sha256="+asset.SHA256)
	}
	return append(evidence, extra...)
}

func versionEvidenceMissing(root string) bool {
	if gitstore.HasSnapshot(root) {
		return false
	}
	snapshots, err := loadVersionSnapshots(root, 1)
	return err == nil && len(snapshots) == 0
}

func linkIssueForFact(root string, fact noteFact, link domain.NoteLink) []domain.VaultIssue {
	switch {
	case link.Status == string(domain.LinkStatusBroken) || link.Broken:
		return []domain.VaultIssue{vaultIssue("broken_link", "warning", fact, "Note has broken links", linkEvidence(link), []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}})}
	case link.Status == string(domain.LinkStatusAmbiguous):
		return []domain.VaultIssue{vaultIssue("ambiguous_link", "warning", fact, "Note link target has multiple candidates", linkEvidence(link), []domain.Action{{Name: "organize_plan", Command: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root))}})}
	default:
		return nil
	}
}

func linkEvidence(link domain.NoteLink) []string {
	evidence := []string{"status=" + link.Status, "kind=" + link.Kind, "target=" + link.Target}
	if link.TargetRaw != "" {
		evidence = append(evidence, "raw="+link.TargetRaw)
	}
	if link.Line > 0 {
		evidence = append(evidence, fmt.Sprintf("line=%d", link.Line))
	}
	if link.Evidence != "" {
		evidence = append(evidence, "resolver="+link.Evidence)
	}
	for _, candidate := range link.Candidates {
		parts := []string{candidate.Path}
		if candidate.Title != "" {
			parts = append(parts, candidate.Title)
		}
		if candidate.NoteID != "" {
			parts = append(parts, candidate.NoteID)
		}
		evidence = append(evidence, "candidate="+strings.Join(parts, ":"))
	}
	return evidence
}

func evidenceValue(evidence []string, key string) string {
	prefix := key + "="
	for _, item := range evidence {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func buildRepairPlan(root string, facts []noteFact, stats domain.VaultStats, issues []domain.VaultIssue, elapsed time.Duration) domain.RepairPlan {
	created := time.Now().UTC()
	planID := repairPlanID(root, issues, created)
	plan := domain.RepairPlan{
		SchemaVersion:      "pinax.repair_plan.v1",
		PlanID:             planID,
		CreatedAt:          created.Format(time.RFC3339),
		ExpiresAt:          created.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		VaultRoot:          root,
		SourceCommand:      fmt.Sprintf("pinax vault doctor --vault %s", shellQuote(root)),
		SourceFacts:        repairSourceFacts(facts, stats),
		IssueSnapshot:      issues,
		Operations:         make([]domain.RepairOperation, 0, len(issues)),
		SkippedIssues:      make([]domain.VaultIssue, 0),
		Status:             "planned",
		ScanDurationMillis: elapsed.Milliseconds(),
	}
	for _, issue := range issues {
		op, ok := repairOperationForIssue(planID, issue)
		if ok {
			plan.Operations = append(plan.Operations, op)
			continue
		}
		plan.SkippedIssues = append(plan.SkippedIssues, issue)
	}
	return plan
}

func repairOperationForIssue(planID string, issue domain.VaultIssue) (domain.RepairOperation, bool) {
	op := domain.RepairOperation{
		OperationID: repairOperationID(planID, issue),
		Path:        issue.Path,
		Target:      evidenceValue(issue.Evidence, "target"),
		NoteID:      issue.NoteID,
		IssueCode:   issue.Code,
		Reason:      issue.Message,
		Status:      "planned",
		Evidence:    issue.Evidence,
	}
	switch issue.Code {
	case "missing_pinax_metadata":
		op.Kind = "metadata_patch"
		op.Mode = "automatic"
		op.Risk = "low"
	case "missing_tags":
		op.Kind = "tags_patch"
		op.Mode = "automatic"
		op.Risk = "low"
	case "index_stale", "index_missing":
		op.Kind = "index_rebuild"
		op.Mode = "automatic"
		op.Risk = "low"
	case "stale_note":
		op.Kind = "archive_status_patch"
		op.Mode = "automatic"
		op.Risk = "medium"
	case "broken_link":
		op.Kind = "link_resolution"
		op.Mode = "manual_review"
		op.Risk = "review"
	case "ambiguous_link":
		op.Kind = "link_rewrite"
		op.Mode = "manual_review"
		op.Risk = "review"
	case "orphan_note":
		op.Kind = "orphan_review"
		op.Target = evidenceValue(issue.Evidence, "title")
		op.Mode = "manual_review"
		op.Risk = "review"
	case "duplicate_title", "empty_note", "missing_title":
		op.Kind = "manual_review"
		op.Mode = "manual_review"
		op.Risk = "review"
	case "asset_missing", "asset_hash_changed", "orphan_manifest_entry", "dangling_asset_link", "version_evidence_missing", "trash_backup_missing", "trash_tombstones_unreadable", "project_workspace_registry_unreadable", "project_workspace_path_invalid", "project_workspace_missing":
		op.Kind = issue.Code
		op.Mode = "manual_review"
		op.Risk = "review"
	default:
		return domain.RepairOperation{}, false
	}
	return op, true
}
func repairPlanRequiresSnapshot(plan domain.RepairPlan) bool {
	for _, op := range plan.Operations {
		if op.Mode != "automatic" {
			continue
		}
		if op.Kind == "index_rebuild" {
			continue
		}
		return true
	}
	return false
}

func repairSourceFacts(facts []noteFact, stats domain.VaultStats) map[string]string {
	source := map[string]string{
		"notes":                fmt.Sprint(len(facts)),
		"index_status":         stats.IndexStatus,
		"frontmatter_coverage": fmt.Sprint(stats.FrontmatterCoverage),
	}
	for _, fact := range facts {
		path := "note." + fact.rel
		source[path+".mtime"] = fact.modTime.UTC().Format(time.RFC3339Nano)
		source[path+".size"] = fmt.Sprint(fact.size)
		source[path+".sha1"] = noteFactHash(fact)
	}
	return source
}

func noteFactHash(fact noteFact) string {
	h := sha1.Sum([]byte(fact.note.Title + "\x00" + fact.note.Body + "\x00" + strings.Join(fact.note.Tags, ",")))
	return hex.EncodeToString(h[:])
}

func repairPlanID(root string, issues []domain.VaultIssue, created time.Time) string {
	parts := []string{root, created.Format(time.RFC3339Nano)}
	for _, issue := range issues {
		parts = append(parts, issue.Code, issue.Path, issue.NoteID)
	}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "repair-" + hex.EncodeToString(h[:])[:12]
}

func repairOperationID(planID string, issue domain.VaultIssue) string {
	h := sha1.Sum([]byte(planID + "\x00" + issue.Code + "\x00" + issue.Path + "\x00" + issue.NoteID))
	return "op-" + hex.EncodeToString(h[:])[:12]
}

func countRepairOperations(ops []domain.RepairOperation, mode string) int {
	count := 0
	for _, op := range ops {
		if op.Mode == mode {
			count++
		}
	}
	return count
}

func saveRepairPlan(root string, plan *domain.RepairPlan) error {
	dir, err := safeJoin(root, ".pinax/repair-plans")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "repair-plans", plan.PlanID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	plan.SavedPath = rel
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func loadRepairPlan(root, planRef string) (domain.RepairPlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.RepairPlan{}, &domain.CommandError{Code: "plan_required", Message: "repair plan id cannot be empty", Hint: "Run pinax repair plan --save to generate a plan"}
	}
	rel := planRef
	if !strings.Contains(planRef, "/") && !strings.HasSuffix(planRef, ".json") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "repair-plans", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.RepairPlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.RepairPlan{}, err
	}
	var plan domain.RepairPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.RepairPlan{}, err
	}
	if plan.SchemaVersion != "pinax.repair_plan.v1" {
		return domain.RepairPlan{}, &domain.CommandError{Code: "repair_plan_schema_invalid", Message: "repair plan schema is not supported", Hint: "Rerun pinax repair plan --save"}
	}
	return plan, nil
}

func listRepairPlans(root string) ([]domain.RepairPlan, error) {
	dir, err := safeJoin(root, ".pinax/repair-plans")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.RepairPlan{}, nil
	}
	if err != nil {
		return nil, err
	}
	plans := make([]domain.RepairPlan, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		plan, err := loadRepairPlan(root, filepath.ToSlash(filepath.Join(".pinax", "repair-plans", entry.Name())))
		if err != nil {
			continue
		}
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].CreatedAt > plans[j].CreatedAt })
	return plans, nil
}

func ensureRepairPlanFresh(ctx context.Context, root string, plan *domain.RepairPlan) error {
	if plan == nil {
		return &domain.CommandError{Code: "plan_required", Message: "repair plan is required", Hint: "Rerun pinax repair plan --save"}
	}
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "repair_plan_not_planned", Message: "repair plan status is not applicable", Hint: "Rerun pinax repair plan --save"}
	}
	if plan.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
		if err == nil && time.Now().UTC().After(expires) {
			return &domain.CommandError{Code: "plan_stale", Message: "repair plan has expired", Hint: "pinax repair plan --vault <vault> --save"}
		}
	}
	objectBound, err := rebaseRepairPlanObjects(ctx, root, plan)
	if err != nil {
		return err
	}
	if objectBound {
		return nil
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return err
	}
	stats := vaultops.Stats(root, toVaultOpsFacts(facts), 0)
	facts = ordinaryNoteFacts(facts)
	current := repairSourceFacts(facts, stats)
	for key, want := range plan.SourceFacts {
		if got := current[key]; got != want {
			return &domain.CommandError{Code: "plan_stale", Message: "repair plan does not match current vault facts", Hint: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}
		}
	}
	return nil
}

func (s *Service) applyRepairMetadataPatch(ctx context.Context, root, rel string) error {
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	note := parseNote(filepath.ToSlash(rel), string(content))
	if strings.TrimSpace(note.ID) == "" {
		objectID, err := s.allocateObjectID(identity.KindNote, root, rel)
		if err != nil {
			return err
		}
		note.ID = objectID
	}
	updated := ensureFrontmatter(note, string(content))
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	parsed := parseNote(filepath.ToSlash(rel), updated)
	_, err = appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "repair.metadata:"+parsed.ID+":"+rel, parsed, "")
	return err
}

func applyRepairFrontmatterPatch(root, rel string, fields map[string]string) error {
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, _ := patchFrontmatterFields(string(content), fields)
	return atomicWriteFile(path, []byte(updated), 0o644)
}

func vaultIssue(code, severity string, fact noteFact, message string, evidence []string, actions []domain.Action) domain.VaultIssue {
	return domain.VaultIssue{Code: code, Severity: severity, Path: fact.rel, NoteID: fact.note.ID, Message: message, Evidence: evidence, NextActions: actions}
}

func noteAllTags(note domain.Note) []string {
	seen := map[string]bool{}
	for _, tag := range note.Tags {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag != "" {
			seen[tag] = true
		}
	}
	for _, match := range vaultInlineTagPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) > 2 && match[2] != "" {
			seen[match[2]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func wikiLinksInBody(body string) []string {
	seen := map[string]bool{}
	for _, match := range vaultWikiLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			target := strings.TrimSpace(match[1])
			if target != "" {
				seen[target] = true
			}
		}
	}
	links := make([]string, 0, len(seen))
	for link := range seen {
		links = append(links, link)
	}
	sort.Strings(links)
	return links
}

func countIssuesBySeverity(issues []domain.VaultIssue) map[string]int {
	counts := map[string]int{"error": 0, "warning": 0, "info": 0}
	for _, issue := range issues {
		counts[issue.Severity]++
	}
	return counts
}

func nextActionsFromIssues(issues []domain.VaultIssue) []domain.Action {
	seen := map[string]bool{}
	actions := make([]domain.Action, 0)
	for _, issue := range issues {
		for _, action := range issue.NextActions {
			key := action.Name + "\x00" + action.Command
			if seen[key] {
				continue
			}
			seen[key] = true
			actions = append(actions, action)
			if len(actions) >= 3 {
				return actions
			}
		}
	}
	return actions
}

func severityRank(severity string) int {
	switch severity {
	case "error":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

var vaultInlineTagPattern = regexp.MustCompile(`(^|\s)#([\pL\pN_/-]+)`)
var vaultWikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
var vaultMarkdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
