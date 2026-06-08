package app

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	noteindex "github.com/yeisme/pinax/internal/index"
	notesearch "github.com/yeisme/pinax/internal/search"
)

type Service struct{}

type InitVaultRequest struct {
	VaultPath string
	Title     string
}

type VaultRequest struct {
	VaultPath string
}

type VaultStatsRequest struct {
	VaultPath string
}

type VaultDoctorRequest struct {
	VaultPath  string
	StaleAfter time.Duration
}

type RepairPlanRequest struct {
	VaultPath string
	Save      bool
}

type RepairApplyRequest struct {
	VaultPath       string
	PlanID          string
	Yes             bool
	SnapshotMessage string
}

type OrganizeSuggestRequest struct {
	VaultPath string
	Save      bool
}

type DailyRequest struct {
	VaultPath string
	Editor    string
	Body      string
	Date      string
	Prev      bool
	Next      bool
}

type InboxTriageRequest struct {
	VaultPath string
	NoteRef   string
	Group     string
	Folder    string
	Kind      string
	Status    string
}

type ViewRequest struct {
	VaultPath     string
	Name          string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	Sort          string
	Limit         int
	CreatedAfter  string
	UpdatedBefore string
	Yes           bool
}

type SearchRequest struct {
	VaultPath     string
	Query         string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedAfter  string
	LinkTarget    string
	HasAttachment bool
	Limit         int
	Sort          string
	AllowStale    bool
}

type CreateNoteRequest struct {
	VaultPath  string
	Title      string
	Project    string
	Folder     string
	Kind       string
	Tags       []string
	Template   string
	Vars       map[string]string
	Body       string
	SourcePath string
	StdinBody  string
	Dir        string
	Slug       string
	Status     string
	DryRun     bool
}

type TemplateRequest struct {
	VaultPath  string
	Name       string
	Title      string
	Project    string
	Tags       []string
	SourcePath string
	Body       string
	UseStdin   bool
	Vars       map[string]string
	Yes        bool
	Overwrite  bool
}

type ShowNoteRequest struct {
	VaultPath string
	NoteRef   string
}

type NoteLinkRequest struct {
	VaultPath string
	NoteRef   string
}

type NoteAttachRequest struct {
	VaultPath  string
	NoteRef    string
	SourcePath string
}

type ImportMarkdownRequest struct {
	VaultPath string
	Source    string
	Group     string
	Folder    string
	Kind      string
	Status    string
	Tags      []string
	Conflict  string
	DryRun    bool
	Yes       bool
}

type ExportMarkdownRequest struct {
	VaultPath string
	OutputDir string
	Tags      []string
	Group     string
	Folder    string
	Kind      string
	Status    string
}

type NoteListRequest struct {
	VaultPath     string
	Tags          []string
	Project       string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedBefore string
	Recent        bool
	Limit         int
	Sort          string
	PathPrefix    string
}

type NoteMutationRequest struct {
	VaultPath string
	NoteRef   string
	Title     string
	TargetDir string
}

type NoteDeleteRequest struct {
	VaultPath string
	NoteRef   string
	Yes       bool
	Hard      bool
}

type NoteTagRequest struct {
	VaultPath string
	NoteRef   string
	Operation string
	Tags      []string
}

type NoteEditRequest struct {
	VaultPath string
	NoteRef   string
	Editor    string
}

type ProjectRequest struct {
	VaultPath   string
	Slug        string
	Name        string
	Description string
	NotesPrefix string
}

type StorageRequest struct {
	VaultPath string
	Root      string
	Bucket    string
	Region    string
	Prefix    string
	Endpoint  string
	Profile   string
}

type ApplyRequest struct {
	VaultPath       string
	PlanID          string
	Yes             bool
	SnapshotMessage string
}

type SyncRequest struct {
	VaultPath string
	Target    string
	Yes       bool
}

func NewService() *Service { return &Service{} }

func (s *Service) InitVault(_ context.Context, req InitVaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
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
	config := filepath.Join(root, ".pinax", "config.yaml")
	if _, err := os.Stat(config); errors.Is(err, os.ErrNotExist) {
		content := fmt.Sprintf("schema_version: pinax.config.v1\ntitle: %q\n", req.Title)
		if err := os.WriteFile(config, []byte(content), 0o644); err != nil {
			return errorProjection("vault.init", err), err
		}
	}
	if err := ensureEventLog(root); err != nil {
		return errorProjection("vault.init", err), err
	}
	_ = appendEvent(root, "vault.init", "success", map[string]string{"title": req.Title})

	projection := domain.NewProjection("vault.init", "Pinax vault 已初始化。")
	projection.Facts["vault"] = root
	projection.Facts["title"] = req.Title
	projection.Actions = []domain.Action{{Name: "validate", Command: fmt.Sprintf("pinax validate --vault %s", shellQuote(root))}}
	return projection, nil
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
	issues := make([]domain.Issue, 0)
	for _, required := range []string{".pinax/config.yaml", ".pinax/events.jsonl"} {
		if _, err := os.Stat(filepath.Join(root, required)); err != nil {
			issues = append(issues, domain.Issue{Code: "missing_asset", Path: required, Message: "缺少 Pinax 机器资产"})
		}
	}
	for _, note := range notes {
		if note.ID == "" {
			issues = append(issues, domain.Issue{Code: "missing_note_id", Path: note.Path, Message: "缺少 note_id"})
		}
	}
	projection := domain.NewProjection("vault.validate", "Vault 校验完成。")
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
	project := domain.Project{Slug: req.Slug, Name: req.Name, Description: req.Description, NotesPrefix: filepath.ToSlash(req.NotesPrefix), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	for i, existing := range registry.Projects {
		if existing.Slug != req.Slug {
			continue
		}
		if existing.Name != project.Name || existing.Description != project.Description || existing.NotesPrefix != project.NotesPrefix {
			err := &domain.CommandError{Code: "project_conflict", Message: "项目 slug 已存在但定义不同", Hint: "换一个 slug，或先查看 pinax project list"}
			return domain.NewErrorProjection("project.create", err), err
		}
		project.CreatedAt = existing.CreatedAt
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
	projection := domain.NewProjection("project.list", "项目列表已读取。")
	projection.Facts["vault"] = root
	projection.Facts["projects"] = fmt.Sprint(len(registry.Projects))
	if registry.CurrentProject != "" {
		projection.Facts["current_project"] = registry.CurrentProject
	}
	projection.Data = map[string]any{"registry": registry}
	projection.Actions = []domain.Action{{Name: "create", Command: fmt.Sprintf("pinax project create <slug> --vault %s", shellQuote(root))}}
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
		err := &domain.CommandError{Code: "project_not_found", Message: "未找到项目", Hint: "运行 pinax project list 查看可用项目"}
		return domain.NewErrorProjection("project.switch", err), err
	}
	registry.CurrentProject = req.Slug
	if err := saveProjectRegistry(root, registry); err != nil {
		return errorProjection("project.switch", err), err
	}
	_ = appendEvent(root, "project.switch", "success", map[string]string{"project": req.Slug})
	projection := domain.NewProjection("project.switch", "当前项目已切换。")
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
	_ = appendEvent(root, "storage.set_local", "success", map[string]string{"backend": "local"})
	return storageProjection("storage.set_local", "本地 storage backend 已配置。", profile), nil
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
		err := &domain.CommandError{Code: "s3_config_incomplete", Message: "S3 backend 需要 bucket 和 region", Hint: "重新运行 pinax storage set-s3 --bucket <bucket> --region <region>"}
		return domain.NewErrorProjection("storage.set_s3", err), err
	}
	profile := domain.StorageProfile{SchemaVersion: "pinax.storage.v1", Backend: "s3", S3: &domain.S3Storage{Bucket: req.Bucket, Region: req.Region, Prefix: req.Prefix, Endpoint: req.Endpoint, Profile: req.Profile}}
	if err := saveStorageProfile(root, profile); err != nil {
		return errorProjection("storage.set_s3", err), err
	}
	_ = appendEvent(root, "storage.set_s3", "success", map[string]string{"backend": "s3", "bucket": req.Bucket, "region": req.Region})
	projection := storageProjection("storage.set_s3", "S3 storage backend 已配置。", profile)
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
	return storageProjection("storage.status", "Storage backend 状态已读取。", profile), nil
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
	projection := storageProjection("storage.doctor", "Storage backend 诊断完成。", profile)
	issues := make([]domain.Issue, 0)
	if profile.Backend == "s3" {
		if profile.S3 == nil || profile.S3.Bucket == "" {
			issues = append(issues, domain.Issue{Code: "missing_bucket", Path: ".pinax/storage.json", Message: "缺少 S3 bucket"})
		}
		if profile.S3 == nil || profile.S3.Region == "" {
			issues = append(issues, domain.Issue{Code: "missing_region", Path: ".pinax/storage.json", Message: "缺少 S3 region"})
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
	stats := VaultAnalyticsService{}.Stats(root, facts, time.Since(started))
	projection := domain.NewProjection("vault.stats", "Vault 统计已生成。")
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
	stats := VaultAnalyticsService{}.Stats(root, facts, time.Since(started))
	issues := VaultHealthService{}.Issues(root, facts, stats, req.StaleAfter)
	report := domain.VaultDoctorReport{VaultPath: root, Issues: issues, Counts: countIssuesBySeverity(issues), Stats: stats}
	projection := domain.NewProjection("vault.doctor", "Vault 健康检查完成。")
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
		projection.Actions = []domain.Action{{Name: "stats", Command: fmt.Sprintf("pinax stats --vault %s", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) PlanRepair(_ context.Context, req RepairPlanRequest) (domain.Projection, error) {
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
	stats := VaultAnalyticsService{}.Stats(root, facts, elapsed)
	issues := VaultHealthService{}.Issues(root, facts, stats, 90*24*time.Hour)
	plan := buildRepairPlan(root, facts, stats, issues, elapsed)
	if req.Save {
		if err := saveRepairPlan(root, &plan); err != nil {
			return errorProjection("repair.plan", err), err
		}
	}
	projection := domain.NewProjection("repair.plan", "Repair 计划已生成。")
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
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax doctor --vault %s", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ApplyRepair(ctx context.Context, req RepairApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "repair apply 需要 --yes", Hint: "先运行 pinax repair plan --save，确认后追加 --yes"}
		return domain.NewErrorProjection("repair.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("repair.apply", err), err
	}
	if strings.TrimSpace(req.PlanID) == "" {
		err := &domain.CommandError{Code: "plan_required", Message: "repair apply 需要 --plan", Hint: "pinax repair apply --vault <vault> --plan <plan_id> --yes"}
		return domain.NewErrorProjection("repair.apply", err), err
	}
	plan, err := loadRepairPlan(root, req.PlanID)
	if err != nil {
		return errorProjection("repair.apply", err), err
	}
	if err := ensureRepairPlanFresh(root, plan); err != nil {
		projection := errorProjection("repair.apply", err)
		projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}}
		projection.Data = map[string]any{"plan_id": plan.PlanID}
		return projection, err
	}
	if req.SnapshotMessage != "" {
		if _, err := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: req.SnapshotMessage}); err != nil {
			return errorProjection("repair.apply", err), err
		}
	}
	if !gitstore.HasSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "应用 repair 计划前需要显式 Git snapshot", Hint: fmt.Sprintf("pinax git snapshot --vault %s --message %s", shellQuote(root), shellQuote("repair 前快照"))}
		projection := domain.NewErrorProjection("repair.apply", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		projection.Data = map[string]any{"plan_id": plan.PlanID}
		return projection, err
	}
	applied := make([]domain.RepairOperation, 0)
	skipped := make([]domain.RepairOperation, 0)
	for _, op := range plan.Operations {
		if op.Mode != "automatic" {
			op.Status = "skipped"
			skipped = append(skipped, op)
			_ = appendEvent(root, "repair.apply", "skipped", map[string]string{"plan_id": plan.PlanID, "operation_id": op.OperationID, "kind": op.Kind, "reason": "manual_review"})
			continue
		}
		if err := s.applyRepairOperation(ctx, root, op); err != nil {
			return errorProjection("repair.apply", err), err
		}
		op.Status = "applied"
		applied = append(applied, op)
		_ = appendEvent(root, "repair.apply", "success", map[string]string{"plan_id": plan.PlanID, "operation_id": op.OperationID, "kind": op.Kind})
	}
	projection := domain.NewProjection("repair.apply", "Repair 计划已应用。")
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations.total"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["applied"] = fmt.Sprint(len(applied))
	projection.Facts["skipped"] = fmt.Sprint(len(skipped))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"plan_id": plan.PlanID, "results": applied, "skipped": skipped}
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
	projection := domain.NewProjection("repair.list", "Repair plans 已读取。")
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
		return applyRepairMetadataPatch(root, op.Path)
	case "archive_status_patch":
		return applyRepairFrontmatterPatch(root, op.Path, map[string]string{"status": "archived"})
	case "index_rebuild":
		_, err := s.RebuildIndex(ctx, VaultRequest{VaultPath: root})
		return err
	default:
		return nil
	}
}

type VaultAnalyticsService struct{}

func (VaultAnalyticsService) Stats(root string, facts []noteFact, elapsed time.Duration) domain.VaultStats {
	return buildVaultStats(root, facts, elapsed)
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

func buildVaultStats(root string, facts []noteFact, elapsed time.Duration) domain.VaultStats {
	dirs := map[string]int{}
	tags := map[string]bool{}
	frontmatterReady := 0
	recentUpdates := 0
	notes := make([]domain.NoteStat, 0, len(facts))
	for _, fact := range facts {
		dir := filepath.ToSlash(filepath.Dir(fact.rel))
		if dir == "." {
			dir = "/"
		}
		dirs[dir]++
		for _, tag := range noteAllTags(fact.note) {
			tags[tag] = true
		}
		if fact.meta["schema_version"] == "pinax.note.v1" && fact.meta["note_id"] != "" {
			frontmatterReady++
		}
		if time.Since(fact.modTime) <= 7*24*time.Hour {
			recentUpdates++
		}
		notes = append(notes, domain.NoteStat{ID: fact.note.ID, Title: fact.note.Title, Path: fact.rel, Tags: fact.note.Tags, HasFrontmatter: fact.hasFrontmatter, UpdatedAt: fact.modTime.UTC().Format(time.RFC3339), SizeBytes: fact.size})
	}
	coverage := 0
	if len(facts) > 0 {
		coverage = frontmatterReady * 100 / len(facts)
	}
	status, indexPath := indexStatus(root, facts)
	return domain.VaultStats{VaultPath: root, NoteCount: len(facts), TagCount: len(tags), DirectoryCounts: dirs, FrontmatterCoverage: coverage, RecentUpdates: recentUpdates, ScanDurationMillis: elapsed.Milliseconds(), IndexStatus: status, IndexPath: indexPath, Notes: notes}
}

func buildVaultIssues(root string, facts []noteFact, stats domain.VaultStats, staleAfter time.Duration) []domain.VaultIssue {
	issues := make([]domain.VaultIssue, 0)
	titles := map[string][]noteFact{}
	incoming := map[string]int{}
	for _, fact := range facts {
		titles[strings.ToLower(strings.TrimSpace(fact.note.Title))] = append(titles[strings.ToLower(strings.TrimSpace(fact.note.Title))], fact)
		for _, target := range wikiLinksInBody(fact.note.Body) {
			incoming[strings.ToLower(target)]++
		}
	}
	for _, fact := range facts {
		nextMetadata := []domain.Action{{Name: "metadata_plan", Command: fmt.Sprintf("pinax metadata plan --vault %s", shellQuote(root))}}
		if fact.note.Title == "" || strings.TrimSuffix(filepath.Base(fact.rel), filepath.Ext(fact.rel)) == fact.note.Title && !strings.Contains(fact.note.Body, "# ") && fact.meta["title"] == "" {
			issues = append(issues, vaultIssue("missing_title", "warning", fact, "笔记缺少明确标题", []string{"frontmatter.title 和一级标题均缺失"}, nextMetadata))
		}
		if len(fact.note.Tags) == 0 {
			issues = append(issues, vaultIssue("missing_tags", "info", fact, "笔记缺少标签", []string{"frontmatter.tags 为空"}, nextMetadata))
		}
		if fact.meta["schema_version"] != "pinax.note.v1" || fact.meta["note_id"] == "" {
			issues = append(issues, vaultIssue("missing_pinax_metadata", "warning", fact, "笔记缺少 Pinax metadata", []string{"需要 schema_version=pinax.note.v1 和 note_id"}, nextMetadata))
		}
		if strings.TrimSpace(fact.note.Body) == "" {
			issues = append(issues, vaultIssue("empty_note", "warning", fact, "笔记正文为空", []string{"frontmatter 后没有正文"}, []domain.Action{{Name: "edit", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(fact.rel), shellQuote(root))}}))
		}
		if time.Since(fact.modTime) > staleAfter {
			issues = append(issues, vaultIssue("stale_note", "info", fact, "笔记长期未更新", []string{fmt.Sprintf("mtime=%s", fact.modTime.UTC().Format(time.RFC3339))}, []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(fact.rel), shellQuote(root))}}))
		}
		// orphan_note 关注图谱维护性：既没有指向其它 wiki link，也没有被其它笔记通过标题引用。
		if len(wikiLinksInBody(fact.note.Body)) == 0 && incoming[strings.ToLower(fact.note.Title)] == 0 {
			issues = append(issues, vaultIssue("orphan_note", "info", fact, "笔记没有双链关系", []string{"没有 wiki link 入边或出边"}, []domain.Action{{Name: "search", Command: fmt.Sprintf("pinax search %s --vault %s", shellQuote(fact.note.Title), shellQuote(root))}}))
		}
		cleanRel := filepath.ToSlash(filepath.Clean(fact.rel))
		if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.HasPrefix(cleanRel, ".pinax/") || filepath.IsAbs(fact.rel) {
			issues = append(issues, vaultIssue("path_anomaly", "error", fact, "笔记路径异常", []string{fact.rel}, nil))
		}
	}
	for _, group := range titles {
		if len(group) <= 1 || strings.TrimSpace(group[0].note.Title) == "" {
			continue
		}
		for _, fact := range group {
			issues = append(issues, vaultIssue("duplicate_title", "warning", fact, "存在重复标题", []string{"title=" + fact.note.Title}, []domain.Action{{Name: "organize_plan", Command: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root))}}))
		}
	}
	if stats.IndexStatus != "fresh" {
		issues = append(issues, domain.VaultIssue{Code: "index_stale", Severity: "warning", Message: "本地索引缺失或过期", Evidence: []string{"index_status=" + stats.IndexStatus}, NextActions: []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}})
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity == issues[j].Severity {
			return issues[i].Code < issues[j].Code
		}
		return severityRank(issues[i].Severity) > severityRank(issues[j].Severity)
	})
	return issues
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
		SourceCommand:      fmt.Sprintf("pinax doctor --vault %s", shellQuote(root)),
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
	case "duplicate_title", "empty_note", "orphan_note", "missing_title":
		op.Kind = "manual_review"
		op.Mode = "manual_review"
		op.Risk = "review"
	default:
		return domain.RepairOperation{}, false
	}
	return op, true
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
		return domain.RepairPlan{}, &domain.CommandError{Code: "plan_required", Message: "repair plan id 不能为空", Hint: "运行 pinax repair plan --save 生成计划"}
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
		return domain.RepairPlan{}, &domain.CommandError{Code: "repair_plan_schema_invalid", Message: "repair plan schema 不受支持", Hint: "重新运行 pinax repair plan --save"}
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

func ensureRepairPlanFresh(root string, plan domain.RepairPlan) error {
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "repair_plan_not_planned", Message: "repair plan 状态不可应用", Hint: "重新运行 pinax repair plan --save"}
	}
	if plan.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
		if err == nil && time.Now().UTC().After(expires) {
			return &domain.CommandError{Code: "plan_stale", Message: "repair plan 已过期", Hint: "pinax repair plan --vault <vault> --save"}
		}
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return err
	}
	stats := VaultAnalyticsService{}.Stats(root, facts, 0)
	current := repairSourceFacts(facts, stats)
	for key, want := range plan.SourceFacts {
		if got := current[key]; got != want {
			return &domain.CommandError{Code: "plan_stale", Message: "repair plan 与当前 vault facts 不一致", Hint: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}
		}
	}
	return nil
}

func applyRepairMetadataPatch(root, rel string) error {
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	note := parseNote(filepath.ToSlash(rel), string(content))
	updated := ensureFrontmatter(note, string(content))
	return os.WriteFile(path, []byte(updated), 0o644)
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
	return os.WriteFile(path, []byte(updated), 0o644)
}

func vaultIssue(code, severity string, fact noteFact, message string, evidence []string, actions []domain.Action) domain.VaultIssue {
	return domain.VaultIssue{Code: code, Severity: severity, Path: fact.rel, NoteID: fact.note.ID, Message: message, Evidence: evidence, NextActions: actions}
}

func indexStatus(root string, facts []noteFact) (string, string) {
	path := filepath.Join(root, ".pinax", "index.sqlite")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	}
	if err != nil {
		return "unreadable", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	}
	for _, fact := range facts {
		if fact.modTime.After(info.ModTime()) {
			return "stale", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
		}
	}
	return "fresh", filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
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

type noteLinkGraph struct {
	notes    []domain.Note
	outgoing map[string][]domain.NoteLink
	incoming map[string][]domain.NoteLink
}

func buildNoteLinkGraph(root string) (noteLinkGraph, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return noteLinkGraph{}, err
	}
	byTitle := map[string]domain.Note{}
	byPath := map[string]domain.Note{}
	for _, note := range notes {
		byTitle[strings.ToLower(note.Title)] = note
		byPath[note.Path] = note
	}
	graph := noteLinkGraph{notes: notes, outgoing: map[string][]domain.NoteLink{}, incoming: map[string][]domain.NoteLink{}}
	for _, note := range notes {
		for _, link := range noteGraphLinks(note, byTitle, byPath) {
			graph.outgoing[note.Path] = append(graph.outgoing[note.Path], link)
			if link.TargetPath != "" && !link.Broken {
				graph.incoming[link.TargetPath] = append(graph.incoming[link.TargetPath], link)
			}
		}
	}
	for path := range graph.outgoing {
		sortNoteLinks(graph.outgoing[path])
	}
	for path := range graph.incoming {
		sortNoteLinks(graph.incoming[path])
	}
	return graph, nil
}

func noteGraphLinks(note domain.Note, byTitle map[string]domain.Note, byPath map[string]domain.Note) []domain.NoteLink {
	links := make([]domain.NoteLink, 0)
	seen := map[string]bool{}
	for _, rawTarget := range wikiLinksInBody(note.Body) {
		target := normalizeWikiLinkTarget(rawTarget)
		if target == "" {
			continue
		}
		resolved := byTitle[strings.ToLower(target)]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, Kind: "wiki", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetPath = resolved.Path
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.Target
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	for _, rawTarget := range markdownLinksInBody(note.Body) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || !strings.EqualFold(filepath.Ext(target), ".md") {
			continue
		}
		targetPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(note.Path), target)))
		resolved := byPath[targetPath]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, TargetPath: targetPath, Kind: "markdown", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.TargetPath
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	return links
}

func markdownLinksInBody(body string) []string {
	links := make([]string, 0)
	seen := map[string]bool{}
	for _, match := range vaultMarkdownLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		links = append(links, target)
	}
	sort.Strings(links)
	return links
}

func normalizeWikiLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if before, _, ok := strings.Cut(target, "|"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func normalizeMarkdownLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
		return ""
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func sortNoteLinks(links []domain.NoteLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourcePath == links[j].SourcePath {
			return links[i].Target < links[j].Target
		}
		return links[i].SourcePath < links[j].SourcePath
	})
}

func countResolvedLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if !link.Broken {
			count++
		}
	}
	return count
}

func countBrokenLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if link.Broken {
			count++
		}
	}
	return count
}

func uniqueAttachmentRel(root string, note domain.Note, filename string) (string, error) {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || filename == string(os.PathSeparator) {
		return "", &domain.CommandError{Code: "attachment_filename_invalid", Message: "附件文件名无效", Hint: "传入带文件名的源文件路径"}
	}
	owner := strings.TrimSpace(note.ID)
	if owner == "" {
		owner = stableNoteID(note.Path)
	}
	base := filepath.ToSlash(filepath.Join("attachments", owner))
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)
	for i := 0; i < 1000; i++ {
		candidateName := filename
		if i > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", stem, i+1, ext)
		}
		rel := filepath.ToSlash(filepath.Join(base, candidateName))
		path, err := safeJoin(root, rel)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return rel, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", &domain.CommandError{Code: "attachment_name_conflict", Message: "附件文件名冲突过多", Hint: "换一个源文件名后重试"}
}

func copyFile(source, target string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o644)
}

func markdownAttachmentReference(notePath, attachmentRel string) string {
	rel, err := filepath.Rel(filepath.Dir(filepath.FromSlash(notePath)), filepath.FromSlash(attachmentRel))
	if err != nil {
		rel = filepath.FromSlash(attachmentRel)
	}
	rel = filepath.ToSlash(rel)
	label := filepath.Base(attachmentRel)
	if attachmentMediaType(attachmentRel) == "image" {
		return fmt.Sprintf("![%s](%s)", label, rel)
	}
	return fmt.Sprintf("[%s](%s)", label, rel)
}

func noteAttachmentsFromBody(root string, note domain.Note) []domain.NoteAttachment {
	attachments := make([]domain.NoteAttachment, 0)
	for _, rawTarget := range markdownLinksInBody(note.Body) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || strings.EqualFold(filepath.Ext(target), ".md") {
			continue
		}
		targetRel, err := resolveVaultReferenceRel(note.Path, target)
		if err != nil {
			attachments = append(attachments, domain.NoteAttachment{NotePath: note.Path, ReferenceText: target, TargetPath: target, MediaType: attachmentMediaType(target), Exists: false})
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(targetRel))
		_, statErr := os.Stat(abs)
		attachments = append(attachments, domain.NoteAttachment{NotePath: note.Path, ReferenceText: target, TargetPath: targetRel, MediaType: attachmentMediaType(targetRel), Exists: statErr == nil})
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].TargetPath < attachments[j].TargetPath })
	return attachments
}

func resolveVaultReferenceRel(notePath, target string) (string, error) {
	target = filepath.ToSlash(strings.TrimSpace(target))
	if strings.HasPrefix(target, "/") {
		target = strings.TrimLeft(target, "/")
	}
	baseDir := filepath.Dir(filepath.ToSlash(notePath))
	clean := filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, target)))
	if strings.HasPrefix(target, "attachments/") || strings.HasPrefix(target, "notes/") {
		clean = filepath.ToSlash(filepath.Clean(target))
	}
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "附件引用越过 vault 边界"}
	}
	return clean, nil
}

func attachmentMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".pdf", ".doc", ".docx", ".txt":
		return "document"
	case ".mp3", ".wav", ".ogg":
		return "audio"
	case ".mp4", ".mov", ".webm":
		return "video"
	default:
		return "file"
	}
}

func countMissingAttachments(attachments []domain.NoteAttachment) int {
	count := 0
	for _, attachment := range attachments {
		if !attachment.Exists {
			count++
		}
	}
	return count
}

func planMarkdownImport(root, source string, req ImportMarkdownRequest) ([]domain.ImportPlan, error) {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &domain.CommandError{Code: "import_source_missing", Message: "导入源不存在", Hint: "检查 Markdown 文件或目录路径"}
	}
	if err != nil {
		return nil, err
	}
	sources := []string{}
	if info.IsDir() {
		if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".md") {
				sources = append(sources, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else if strings.EqualFold(filepath.Ext(source), ".md") {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	plans := make([]domain.ImportPlan, 0, len(sources))
	used := map[string]bool{}
	for _, item := range sources {
		targetRel, err := importTargetRel(source, item, info.IsDir(), req)
		if err != nil {
			return nil, err
		}
		plan := domain.ImportPlan{SourcePath: item, TargetPath: targetRel, Status: "write"}
		if used[targetRel] || fileExistsPath(root, targetRel) {
			plan.Conflict = "exists"
			switch strings.TrimSpace(req.Conflict) {
			case "rename":
				plan.TargetPath, err = uniqueImportRel(root, targetRel, used)
				if err != nil {
					return nil, err
				}
				plan.Status = "rename"
			case "overwrite":
				plan.Status = "overwrite"
			case "skip", "":
				plan.Status = "skip"
			default:
				return nil, &domain.CommandError{Code: "invalid_import_conflict", Message: "未知导入冲突策略", Hint: "使用 --conflict skip、rename 或 overwrite"}
			}
		}
		used[plan.TargetPath] = true
		plans = append(plans, plan)
	}
	return plans, nil
}

func importTargetRel(sourceRoot, sourceFile string, sourceIsDir bool, req ImportMarkdownRequest) (string, error) {
	name := filepath.Base(sourceFile)
	if sourceIsDir {
		rel, err := filepath.Rel(sourceRoot, sourceFile)
		if err != nil {
			return "", err
		}
		name = filepath.ToSlash(rel)
	}
	base := "notes"
	if strings.TrimSpace(req.Group) != "" {
		base = filepath.ToSlash(filepath.Join(base, strings.TrimSpace(req.Group)))
	}
	if strings.TrimSpace(req.Folder) != "" {
		folder, err := validateOptionalNoteFolder(req.Folder)
		if err != nil {
			return "", err
		}
		base = filepath.ToSlash(filepath.Join(base, folder))
	}
	return validateNoteDir(filepath.ToSlash(filepath.Join(base, name)))
}

func uniqueImportRel(root, targetRel string, used map[string]bool) (string, error) {
	dir := filepath.Dir(targetRel)
	base := filepath.Base(targetRel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	ext := filepath.Ext(base)
	for i := 2; i < 1000; i++ {
		candidate := filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext)))
		if !used[candidate] && !fileExistsPath(root, candidate) {
			return candidate, nil
		}
	}
	return "", &domain.CommandError{Code: "import_name_conflict", Message: "导入文件名冲突过多", Hint: "换一个目标分组或文件名后重试"}
}

func fileExistsPath(root, rel string) bool {
	path, err := safeJoin(root, rel)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func countImportPlans(plans []domain.ImportPlan, status string) int {
	count := 0
	for _, plan := range plans {
		if plan.Status == status {
			count++
		}
	}
	return count
}

func writeReceipt(root, kind string, payload map[string]any) (string, error) {
	now := time.Now().UTC()
	rel := filepath.ToSlash(filepath.Join(".pinax", "receipts", fmt.Sprintf("%s-%s.json", kind, now.Format("20060102T150405Z"))))
	payload["schema_version"] = "pinax.receipt.v1"
	payload["kind"] = kind
	payload["created_at"] = now.Format(time.RFC3339)
	return rel, writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), payload)
}

func copyVaultFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o644)
}

func (s *Service) ListNotes(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	return s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath})
}

func (s *Service) ListNotesQuery(_ context.Context, req NoteListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.list", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("note.list", err), err
	}
	notes := make([]domain.Note, 0, len(facts))
	for _, fact := range facts {
		note := fact.note
		if !noteMatchesQuery(note, req) {
			continue
		}
		notes = append(notes, note)
	}
	sortNotes(notes, req)
	total := len(notes)
	if req.Limit > 0 && len(notes) > req.Limit {
		notes = notes[:req.Limit]
	}
	projection := domain.NewProjection("note.list", "已列出本地笔记。")
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["count"] = fmt.Sprint(len(notes))
	projection.Facts["total"] = fmt.Sprint(total)
	projection.Facts["returned"] = fmt.Sprint(len(notes))
	if req.Recent {
		projection.Facts["recent"] = "true"
	}
	if req.Recent || req.Sort == "" || req.Sort == "updated" {
		projection.Facts["sort"] = "updated"
	} else {
		projection.Facts["sort"] = req.Sort
	}
	if len(req.Tags) > 0 {
		projection.Facts["filter.tag"] = strings.Join(req.Tags, ",")
	}
	if req.Project != "" {
		projection.Facts["filter.project"] = req.Project
	}
	if req.Group != "" {
		projection.Facts["filter.group"] = req.Group
	}
	if req.Folder != "" {
		projection.Facts["filter.folder"] = req.Folder
	}
	if req.Kind != "" {
		projection.Facts["filter.kind"] = req.Kind
	}
	if req.Status != "" {
		projection.Facts["filter.status"] = req.Status
	}
	if req.CreatedAfter != "" {
		projection.Facts["filter.created_after"] = req.CreatedAfter
	}
	if req.UpdatedBefore != "" {
		projection.Facts["filter.updated_before"] = req.UpdatedBefore
	}
	if req.PathPrefix != "" {
		projection.Facts["filter.path_prefix"] = req.PathPrefix
	}
	projection.Data = map[string]any{"notes": notes, "filters": req, "total": total, "returned": len(notes)}
	return projection, nil
}

func (s *Service) ListDimension(_ context.Context, req VaultRequest, dimension string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(dimension+".list", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(dimension+".list", err), err
	}
	counts := map[string]int{}
	for _, note := range notes {
		for _, value := range noteDimensionValues(note, dimension) {
			counts[value]++
		}
	}
	items := make([]domain.DimensionCount, 0, len(counts))
	for value, count := range counts {
		items = append(items, domain.DimensionCount{Value: value, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Value < items[j].Value
		}
		return items[i].Count > items[j].Count
	})
	projection := domain.NewProjection(dimension+".list", "组织视图已列出。")
	projection.Facts["dimension"] = dimension
	projection.Facts["dimensions"] = fmt.Sprint(len(items))
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Data = map[string]any{"dimension": dimension, "items": items}
	return projection, nil
}

func (s *Service) SaveView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.save", err), err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "view_name_required", Message: "view save 需要名称", Hint: "pinax view save <name> --vault <vault>"}
		return domain.NewErrorProjection("view.save", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.save", err), err
	}
	view := domain.SavedView{Name: name, Tags: cleanTags(req.Tags), Group: strings.TrimSpace(req.Group), Folder: strings.TrimSpace(req.Folder), Kind: strings.TrimSpace(req.Kind), Status: strings.TrimSpace(req.Status), Sort: normalizedListSort(req.Sort), Limit: req.Limit, CreatedAfter: req.CreatedAfter, UpdatedBefore: req.UpdatedBefore, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	upsertSavedView(&registry, view)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("view.save", err), err
	}
	projection := domain.NewProjection("view.save", "视图已保存。")
	projection.Facts["view"] = name
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "views.json"))}
	projection.Data = map[string]any{"view": view}
	return projection, nil
}

func (s *Service) ListViews(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.list", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.list", err), err
	}
	projection := domain.NewProjection("view.list", "视图已列出。")
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Data = registry
	return projection, nil
}

func (s *Service) ShowView(ctx context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.show", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.show", err), err
	}
	view, ok := findSavedView(registry, req.Name)
	if !ok {
		err := &domain.CommandError{Code: "view_not_found", Message: "未找到保存视图", Hint: "pinax view list --vault <vault>"}
		return domain.NewErrorProjection("view.show", err), err
	}
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Tags: view.Tags, Group: view.Group, Folder: view.Folder, Kind: view.Kind, Status: view.Status, CreatedAfter: view.CreatedAfter, UpdatedBefore: view.UpdatedBefore, Sort: view.Sort, Limit: view.Limit})
	projection.Command = "view.show"
	projection.Summary = "视图已查询。"
	projection.Facts["view"] = view.Name
	projection.Data = map[string]any{"view": view, "result": projection.Data}
	return projection, err
}

func (s *Service) DeleteView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "view delete 需要 --yes", Hint: "确认后追加 --yes"}
		return domain.NewErrorProjection("view.delete", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.delete", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.delete", err), err
	}
	removed := removeSavedView(&registry, req.Name)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("view.delete", err), err
	}
	projection := domain.NewProjection("view.delete", "视图已删除。")
	projection.Facts["view"] = req.Name
	projection.Facts["removed"] = fmt.Sprint(removed)
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "views.json"))}
	return projection, nil
}

func noteDimensionValues(note domain.Note, dimension string) []string {
	switch dimension {
	case "tag":
		return cleanTags(noteAllTags(note))
	case "folder":
		if strings.TrimSpace(note.Folder) != "" {
			return []string{note.Folder}
		}
		dir := filepath.ToSlash(filepath.Dir(note.Path))
		if dir == "." {
			return []string{""}
		}
		return []string{strings.TrimPrefix(dir, "notes/")}
	case "kind":
		return []string{strings.TrimSpace(note.Kind)}
	case "group":
		return []string{strings.TrimSpace(note.Project)}
	default:
		return []string{""}
	}
}

func loadSavedViews(root string) (domain.SavedViewRegistry, error) {
	registry := domain.SavedViewRegistry{SchemaVersion: "pinax.views.v1", Views: []domain.SavedView{}}
	path := filepath.Join(root, ".pinax", "views.json")
	b, err := os.ReadFile(path)
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
		registry.SchemaVersion = "pinax.views.v1"
	}
	if registry.Views == nil {
		registry.Views = []domain.SavedView{}
	}
	return registry, nil
}

func saveSavedViews(root string, registry domain.SavedViewRegistry) error {
	registry.SchemaVersion = "pinax.views.v1"
	if registry.Views == nil {
		registry.Views = []domain.SavedView{}
	}
	sort.Slice(registry.Views, func(i, j int) bool { return registry.Views[i].Name < registry.Views[j].Name })
	return writeJSONAsset(filepath.Join(root, ".pinax", "views.json"), registry)
}

func upsertSavedView(registry *domain.SavedViewRegistry, view domain.SavedView) {
	for i, existing := range registry.Views {
		if existing.Name == view.Name {
			registry.Views[i] = view
			return
		}
	}
	registry.Views = append(registry.Views, view)
}

func findSavedView(registry domain.SavedViewRegistry, name string) (domain.SavedView, bool) {
	for _, view := range registry.Views {
		if view.Name == strings.TrimSpace(name) {
			return view, true
		}
	}
	return domain.SavedView{}, false
}

func removeSavedView(registry *domain.SavedViewRegistry, name string) bool {
	name = strings.TrimSpace(name)
	for i, view := range registry.Views {
		if view.Name != name {
			continue
		}
		registry.Views = append(registry.Views[:i], registry.Views[i+1:]...)
		return true
	}
	return false
}

func normalizedListSort(sortMode string) string {
	sortMode = strings.TrimSpace(sortMode)
	switch sortMode {
	case "", "updated":
		return "updated"
	case "path", "title":
		return sortMode
	default:
		return "updated"
	}
}

func (s *Service) CreateNote(ctx context.Context, req CreateNoteRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		err := &domain.CommandError{Code: "title_required", Message: "note new 需要标题", Hint: "pinax note new <title> --vault <vault>"}
		return domain.NewErrorProjection("note.new", err), err
	}
	body, err := noteBodyFromRequest(req)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	kind := strings.TrimSpace(req.Kind)
	prefix, err := noteCreatePrefix(root, req)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Title)
	}
	if slug == "" {
		slug = stableNoteID(req.Title)
	}
	if err := validateNoteSlug(slug); err != nil {
		return errorProjection("note.new", err), err
	}
	rel, err := nextNotePath(root, filepath.ToSlash(filepath.Join(prefix, slug+".md")))
	if err != nil {
		return errorProjection("note.new", err), err
	}
	if body == "" {
		body = "# " + req.Title + "\n"
	}
	if strings.TrimSpace(req.Template) != "" && req.Body == "" && req.SourcePath == "" && req.StdinBody == "" {
		rendered, err := renderTemplateBody(root, TemplateRequest{VaultPath: root, Name: req.Template, Title: req.Title, Project: req.Project, Tags: req.Tags, Vars: req.Vars})
		if err != nil {
			return errorProjection("note.new", err), err
		}
		body = rendered
	}
	now := time.Now().UTC().Format(time.RFC3339)
	content := buildNoteContentWithStatus(req.Title, rel, req.Project, folder, kind, cleanTags(req.Tags), req.Status, now, body)
	projection := domain.NewProjection("note.new", "笔记已创建。")
	projection.Facts["path"] = rel
	projection.Facts["planned_path"] = rel
	projection.Facts["title"] = req.Title
	projection.Facts["note_id"] = stableNoteID(rel)
	projection.Facts["tags"] = strings.Join(cleanTags(req.Tags), ",")
	if req.Project != "" {
		projection.Facts["project"] = req.Project
		projection.Facts["group"] = req.Project
	}
	if folder != "" {
		projection.Facts["folder"] = folder
	}
	if kind != "" {
		projection.Facts["kind"] = kind
	}
	if req.Status != "" {
		projection.Facts["status"] = req.Status
	}
	projection.Data = map[string]any{"note": domain.Note{ID: stableNoteID(rel), Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Body: strings.TrimSpace(body), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status, CreatedAt: now, UpdatedAt: now}, "planned_path": rel, "frontmatter_preview": strings.SplitN(content, "---\n\n", 2)[0] + "---", "body_preview": strings.TrimSpace(body)}
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(rel), shellQuote(root))}}
	if req.DryRun {
		projection.Summary = "笔记创建计划已生成。"
		return projection, nil
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("note.new", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("note.new", err), err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errorProjection("note.new", err), err
	}
	dailyIndexRel, dailyErr := appendDailyIndex(root, domain.Note{ID: stableNoteID(rel), Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status})
	if dailyErr != nil {
		return errorProjection("note.new", dailyErr), dailyErr
	}
	if dailyIndexRel != "" {
		projection.Facts["daily_index"] = dailyIndexRel
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.new", err), err
	}
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{dailyIndexRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	_ = appendEvent(root, "note.new", "success", map[string]string{"path": rel, "title": req.Title})
	_ = ctx
	return projection, nil
}

func (s *Service) DailyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.open", err), err
	}
	projection, err := s.EditNote(ctx, NoteEditRequest{VaultPath: root, NoteRef: rel, Editor: req.Editor})
	projection.Command = "daily.open"
	projection.Summary = "Daily note 已打开。"
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = "daily"
	return projection, err
}

func (s *Service) DailyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.show", err), err
	}
	projection, err := s.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
	projection.Command = "daily.show"
	projection.Summary = "Daily note 已读取。"
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = "daily"
	return projection, err
}

func (s *Service) DailyAppend(_ context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.append", err), err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		err := &domain.CommandError{Code: "body_required", Message: "daily append 需要 --body", Hint: "pinax daily append --body <text> --vault <vault>"}
		return domain.NewErrorProjection("daily.append", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection("daily.append", err), err
	}
	if err := appendFile(path, "\n\n"+body+"\n"); err != nil {
		return errorProjection("daily.append", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("daily.append", err), err
	}
	_ = appendEvent(root, "daily.append", "success", map[string]string{"path": rel})
	projection := domain.NewProjection("daily.append", "Daily note 已追加。")
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = "daily"
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) WeeklyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.openJournal(ctx, "weekly", req)
}

func (s *Service) WeeklyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.showJournal(ctx, "weekly", req)
}

func (s *Service) WeeklyAppend(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.appendJournal(ctx, "weekly", req)
}

func (s *Service) MonthlyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.openJournal(ctx, "monthly", req)
}

func (s *Service) MonthlyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.showJournal(ctx, "monthly", req)
}

func (s *Service) MonthlyAppend(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	return s.appendJournal(ctx, "monthly", req)
}

func (s *Service) openJournal(ctx context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".open", err), err
	}
	projection, err := s.EditNote(ctx, NoteEditRequest{VaultPath: root, NoteRef: rel, Editor: req.Editor})
	projection.Command = period + ".open"
	projection.Summary = journalLabel(period) + " 已打开。"
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	return projection, err
}

func (s *Service) showJournal(ctx context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".show", err), err
	}
	projection, err := s.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
	projection.Command = period + ".show"
	projection.Summary = journalLabel(period) + " 已读取。"
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	return projection, err
}

func (s *Service) appendJournal(_ context.Context, period string, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, period, req)
	if err != nil {
		return errorProjection(period+".append", err), err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		err := &domain.CommandError{Code: "body_required", Message: period + " append 需要 --body", Hint: "pinax " + period + " append --body <text> --vault <vault>"}
		return domain.NewErrorProjection(period+".append", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection(period+".append", err), err
	}
	if err := appendFile(path, "\n\n"+body+"\n"); err != nil {
		return errorProjection(period+".append", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection(period+".append", err), err
	}
	_ = appendEvent(root, period+".append", "success", map[string]string{"path": rel})
	projection := domain.NewProjection(period+".append", journalLabel(period)+" 已追加。")
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = period
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{rel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) InboxCapture(ctx context.Context, req CreateNoteRequest) (domain.Projection, error) {
	req.Folder = "inbox"
	req.Kind = "inbox"
	req.Status = "inbox"
	projection, err := s.CreateNote(ctx, req)
	projection.Command = "inbox.capture"
	projection.Summary = "Inbox 笔记已捕获。"
	return projection, err
}

func (s *Service) InboxList(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath, Status: "inbox", Sort: "updated"})
	projection.Command = "inbox.list"
	projection.Summary = "Inbox 笔记已列出。"
	return projection, err
}

func (s *Service) InboxTriage(_ context.Context, req InboxTriageRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		err := &domain.CommandError{Code: "group_required", Message: "inbox triage 需要 --group", Hint: "pinax inbox triage <note> --group <group> --vault <vault>"}
		return domain.NewErrorProjection("inbox.triage", err), err
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if folder == "" {
		folder = "inbox"
	}
	projectPrefix := filepath.ToSlash(filepath.Join("notes", group))
	if project, err := findProject(root, group); err == nil && strings.TrimSpace(project.NotesPrefix) != "" {
		projectPrefix = project.NotesPrefix
	}
	targetRel := filepath.ToSlash(filepath.Join(projectPrefix, folder, filepath.Base(note.Path)))
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if _, err := os.Stat(target); err == nil {
		err := &domain.CommandError{Code: "note_path_conflict", Message: "目标笔记路径已存在", Hint: "换一个 folder 或先处理现有文件"}
		return domain.NewErrorProjection("inbox.triage", err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection("inbox.triage", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	meta["project"] = group
	meta["folder"] = folder
	if strings.TrimSpace(req.Kind) != "" {
		meta["kind"] = strings.TrimSpace(req.Kind)
	}
	if strings.TrimSpace(req.Status) != "" {
		meta["status"] = strings.TrimSpace(req.Status)
	}
	meta["updated_at"] = now
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, target, updated); err != nil {
		return errorProjection("inbox.triage", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("inbox.triage", err), err
	}
	_ = appendEvent(root, "inbox.triage", "success", map[string]string{"from": note.Path, "to": targetRel})
	projection := noteMutationProjection("inbox.triage", "Inbox 笔记已整理。", targetRel, meta)
	projection.Facts["path"] = targetRel
	projection.Facts["group"] = group
	projection.Facts["project"] = group
	projection.Facts["folder"] = folder
	projection.Facts["kind"] = meta["kind"]
	projection.Facts["status"] = meta["status"]
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{targetRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) ResolveNote(_ context.Context, req ShowNoteRequest) (domain.Note, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return domain.Note{}, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return domain.Note{}, err
	}
	return resolveNoteRef(notes, req.NoteRef)
}

func (s *Service) ShowNote(ctx context.Context, req ShowNoteRequest) (domain.Note, error) {
	return s.ResolveNote(ctx, req)
}

func (s *Service) ShowNoteProjection(ctx context.Context, req ShowNoteRequest) (domain.Projection, error) {
	note, err := s.ResolveNote(ctx, req)
	if err != nil {
		projection := errorProjection("note.show", err)
		if amb, ok := err.(*noteRefAmbiguousError); ok {
			projection.Data = map[string]any{"candidates": amb.Candidates}
		}
		return projection, err
	}
	projection := domain.NewProjection("note.show", "已读取本地笔记。")
	projection.Facts["path"] = note.Path
	projection.Facts["title"] = note.Title
	projection.Facts["note_id"] = note.ID
	projection.Data = map[string]any{"note": note}
	return projection, nil
}

func (s *Service) NoteLinks(ctx context.Context, req NoteLinkRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteLinkGraphRequest{VaultPath: req.VaultPath, NoteRef: req.NoteRef}
	projection, err := s.QueryOutgoingLinks(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.links", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.links", graphErr), graphErr
	}
	note, resolveErr := resolveNoteRef(graph.notes, req.NoteRef)
	if resolveErr != nil {
		return errorProjection("note.links", resolveErr), resolveErr
	}
	links := graph.outgoing[note.Path]
	projection = domain.NewProjection("note.links", "笔记链接已列出。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["resolved"] = fmt.Sprint(countResolvedLinks(links))
	projection.Facts["broken"] = fmt.Sprint(countBrokenLinks(links))
	projection.Facts["ambiguous"] = "0"
	projection.Facts["engine"] = "scan"
	projection.Data = map[string]any{"note": note, "links": links}
	return projection, nil
}

func (s *Service) NoteBacklinks(ctx context.Context, req NoteLinkRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteBacklinkGraphRequest{VaultPath: req.VaultPath, NoteRef: req.NoteRef, IncludeBroken: true}
	projection, err := s.QueryBacklinks(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.backlinks", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.backlinks", graphErr), graphErr
	}
	note, resolveErr := resolveNoteRef(graph.notes, req.NoteRef)
	if resolveErr != nil {
		return errorProjection("note.backlinks", resolveErr), resolveErr
	}
	backlinks := graph.incoming[note.Path]
	projection = domain.NewProjection("note.backlinks", "笔记反链已列出。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["backlinks"] = fmt.Sprint(len(backlinks))
	projection.Facts["unresolved"] = "0"
	projection.Facts["engine"] = "scan"
	projection.Data = map[string]any{"note": note, "backlinks": backlinks}
	return projection, nil
}

func (s *Service) NoteOrphans(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	// 尝试使用增强链接图
	enhancedReq := NoteOrphansRequest{VaultPath: req.VaultPath, Mode: "full"}
	projection, err := s.QueryOrphans(ctx, enhancedReq)
	if err == nil {
		return projection, nil
	}
	// fallback: 原有实现
	root, err2 := cleanVaultPath(req.VaultPath)
	if err2 != nil {
		return errorProjection("note.orphans", err2), err2
	}
	graph, graphErr := buildNoteLinkGraph(root)
	if graphErr != nil {
		return errorProjection("note.orphans", graphErr), graphErr
	}
	orphans := make([]domain.Note, 0)
	for _, note := range graph.notes {
		if len(graph.outgoing[note.Path]) == 0 && len(graph.incoming[note.Path]) == 0 {
			orphans = append(orphans, note)
		}
	}
	projection = domain.NewProjection("note.orphans", "孤立笔记已列出。")
	projection.Facts["notes"] = fmt.Sprint(len(graph.notes))
	projection.Facts["orphans"] = fmt.Sprint(len(orphans))
	projection.Facts["engine"] = "scan"
	projection.Data = map[string]any{"orphans": orphans}
	return projection, nil
}

func (s *Service) AttachNoteFile(_ context.Context, req NoteAttachRequest) (domain.Projection, error) {
	root, note, notePath, content, _, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	source := strings.TrimSpace(req.SourcePath)
	if source == "" {
		err := &domain.CommandError{Code: "attachment_source_required", Message: "note attach 需要源文件", Hint: "pinax note attach <note> <file> --vault <vault>"}
		return domain.NewErrorProjection("note.attach", err), err
	}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		commandErr := &domain.CommandError{Code: "attachment_source_missing", Message: "附件源文件不存在", Hint: "检查源文件路径后重试"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if info.IsDir() {
		commandErr := &domain.CommandError{Code: "attachment_source_is_directory", Message: "附件源路径是目录", Hint: "传入单个文件路径"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	attachmentRel, err := uniqueAttachmentRel(root, note, filepath.Base(source))
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	attachmentPath, err := safeJoin(root, attachmentRel)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o755); err != nil {
		return errorProjection("note.attach", err), err
	}
	if err := copyFile(source, attachmentPath); err != nil {
		return errorProjection("note.attach", err), err
	}
	reference := markdownAttachmentReference(note.Path, attachmentRel)
	updated := strings.TrimRight(content, "\n") + "\n\n" + reference + "\n"
	if err := commitNoteContent(notePath, notePath, updated); err != nil {
		return errorProjection("note.attach", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.attach", err), err
	}
	_ = appendEvent(root, "note.attach", "success", map[string]string{"path": note.Path, "attachment_path": attachmentRel})
	projection := domain.NewProjection("note.attach", "附件已添加到笔记。")
	projection.Facts["path"] = note.Path
	projection.Facts["attachment_path"] = attachmentRel
	projection.Facts["source_path"] = source
	projection.Facts["media_type"] = attachmentMediaType(attachmentRel)
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{note.Path, attachmentRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"note": note, "attachment": domain.NoteAttachment{NotePath: note.Path, ReferenceText: reference, TargetPath: attachmentRel, MediaType: attachmentMediaType(attachmentRel), Exists: true}}
	return projection, nil
}

func (s *Service) NoteAttachments(_ context.Context, req NoteLinkRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	note, err := resolveNoteRef(notes, req.NoteRef)
	if err != nil {
		return errorProjection("note.attachments", err), err
	}
	attachments := noteAttachmentsFromBody(root, note)
	projection := domain.NewProjection("note.attachments", "笔记附件已列出。")
	projection.Facts["path"] = note.Path
	projection.Facts["attachments"] = fmt.Sprint(len(attachments))
	projection.Facts["missing"] = fmt.Sprint(countMissingAttachments(attachments))
	projection.Data = map[string]any{"note": note, "attachments": attachments}
	return projection, nil
}

func (s *Service) ImportMarkdown(_ context.Context, req ImportMarkdownRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("import.markdown", err), err
	}
	source, err := cleanVaultPath(req.Source)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	plans, err := planMarkdownImport(root, source, req)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	projection := domain.NewProjection("import.markdown", "Markdown 导入计划已生成。")
	projection.Facts["planned"] = fmt.Sprint(len(plans))
	projection.Facts["written"] = "0"
	projection.Facts["renamed"] = fmt.Sprint(countImportPlans(plans, "rename"))
	projection.Facts["overwritten"] = fmt.Sprint(countImportPlans(plans, "overwrite"))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Data = map[string]any{"plans": plans, "dry_run": req.DryRun}
	if req.DryRun {
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "import markdown 需要 --yes", Hint: "先用 --dry-run 查看计划，确认后追加 --yes"}
		return domain.NewErrorProjection("import.markdown", err), err
	}
	written := 0
	for _, plan := range plans {
		if plan.Status == "skip" {
			continue
		}
		content, err := os.ReadFile(plan.SourcePath)
		if err != nil {
			return errorProjection("import.markdown", err), err
		}
		note := parseNote(filepath.Base(plan.SourcePath), string(content))
		body := note.Body
		if strings.TrimSpace(body) == "" {
			_, body = splitFrontmatter(string(content))
		}
		if strings.TrimSpace(body) == "" {
			body = string(content)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		folder := strings.TrimSpace(req.Folder)
		if folder == "" {
			folder = filepath.ToSlash(filepath.Dir(strings.TrimPrefix(plan.TargetPath, "notes/")))
			if folder == "." || folder == strings.TrimSpace(req.Group) {
				folder = ""
			}
		}
		output := buildNoteContentWithStatus(note.Title, plan.TargetPath, strings.TrimSpace(req.Group), folder, strings.TrimSpace(req.Kind), cleanTags(req.Tags), strings.TrimSpace(req.Status), now, body)
		target, err := safeJoin(root, plan.TargetPath)
		if err != nil {
			return errorProjection("import.markdown", err), err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("import.markdown", err), err
		}
		if err := os.WriteFile(target, []byte(output), 0o644); err != nil {
			return errorProjection("import.markdown", err), err
		}
		written++
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("import.markdown", err), err
	}
	receiptRel, err := writeReceipt(root, "import", map[string]any{"source": source, "plans": plans, "written": written})
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	_ = appendEvent(root, "import.markdown", "success", map[string]string{"written": fmt.Sprint(written), "receipt_path": receiptRel})
	projection.Summary = "Markdown 已导入。"
	projection.Facts["written"] = fmt.Sprint(written)
	projection.Facts["overwritten"] = fmt.Sprint(countImportPlans(plans, "overwrite"))
	projection.Facts["receipt_path"] = receiptRel
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{receiptRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"plans": plans, "written": written, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) ExportMarkdown(_ context.Context, req ExportMarkdownRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	out, err := cleanVaultPath(req.OutputDir)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	filter := NoteListRequest{VaultPath: root, Tags: cleanTags(req.Tags), Group: strings.TrimSpace(req.Group), Folder: strings.TrimSpace(req.Folder), Kind: strings.TrimSpace(req.Kind), Status: strings.TrimSpace(req.Status)}
	selected := make([]domain.Note, 0)
	for _, note := range notes {
		if noteMatchesQuery(note, filter) {
			selected = append(selected, note)
		}
	}
	attachmentsCopied := 0
	for _, note := range selected {
		source, err := safeJoin(root, note.Path)
		if err != nil {
			return errorProjection("export.markdown", err), err
		}
		if err := copyVaultFile(source, filepath.Join(out, filepath.FromSlash(note.Path))); err != nil {
			return errorProjection("export.markdown", err), err
		}
		for _, attachment := range noteAttachmentsFromBody(root, note) {
			if !attachment.Exists {
				continue
			}
			attachmentSource := filepath.Join(root, filepath.FromSlash(attachment.TargetPath))
			if err := copyVaultFile(attachmentSource, filepath.Join(out, filepath.FromSlash(attachment.TargetPath))); err != nil {
				return errorProjection("export.markdown", err), err
			}
			attachmentsCopied++
		}
	}
	receiptRel, err := writeReceipt(root, "export", map[string]any{"output_dir": out, "notes": len(selected), "attachments": attachmentsCopied})
	if err != nil {
		return errorProjection("export.markdown", err), err
	}
	projection := domain.NewProjection("export.markdown", "Markdown 已导出。")
	projection.Facts["output_dir"] = out
	projection.Facts["notes"] = fmt.Sprint(len(selected))
	projection.Facts["attachments"] = fmt.Sprint(attachmentsCopied)
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{out, receiptRel}
	projection.Data = map[string]any{"notes": selected, "attachments": attachmentsCopied, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) EditNote(ctx context.Context, req NoteEditRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	editorText := strings.TrimSpace(req.Editor)
	if editorText == "" {
		editorText = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	editor, err := parseEditorCommand(editorText)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return errorProjection("note.edit", err), err
	}
	args := append(append([]string{}, editor.Args...), path)
	cmd := exec.CommandContext(ctx, editor.Executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		commandErr := &domain.CommandError{Code: "editor_failed", Message: "编辑器执行失败", Hint: "检查 --editor 或 $EDITOR 指向的可执行文件"}
		return domain.NewErrorProjection("note.edit", commandErr), commandErr
	}
	projection := domain.NewProjection("note.edit", "笔记已在编辑器中打开。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["editor"] = editor.Raw
	projection.Facts["editor_executable"] = editor.Executable
	projection.Facts["editor_args"] = strings.Join(editor.Args, " ")
	projection.Data = map[string]any{"note": note, "editor": editor}
	return projection, nil
}

func (s *Service) RenameNote(_ context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.rename", err), err
	}
	newTitle := strings.TrimSpace(req.Title)
	if newTitle == "" {
		err := &domain.CommandError{Code: "title_required", Message: "note rename 需要新标题", Hint: "pinax note rename <note> <title> --vault <vault>"}
		return domain.NewErrorProjection("note.rename", err), err
	}
	meta["title"] = newTitle
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	targetRel := filepath.ToSlash(filepath.Join(filepath.Dir(note.Path), slugify(newTitle)+".md"))
	if targetRel == filepath.ToSlash(filepath.Dir(note.Path))+"/.md" {
		targetRel = filepath.ToSlash(filepath.Join(filepath.Dir(note.Path), stableNoteID(newTitle)+".md"))
	}
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("note.rename", err), err
	}
	if targetRel != note.Path {
		if _, err := os.Stat(target); err == nil {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "目标笔记路径已存在", Hint: "换一个标题或先移动现有文件"}
			return domain.NewErrorProjection("note.rename", err), err
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errorProjection("note.rename", err), err
		}
	}
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, target, updated); err != nil {
		return errorProjection("note.rename", err), err
	}
	_ = appendEvent(root, "note.rename", "success", map[string]string{"from": note.Path, "to": targetRel})
	return noteMutationProjection("note.rename", "笔记已重命名。", targetRel, meta), nil
}

func (s *Service) MoveNote(_ context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, _, _, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	dir, err := validateNoteDir(req.TargetDir)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	targetRel := filepath.ToSlash(filepath.Join(dir, filepath.Base(note.Path)))
	target, err := safeJoin(root, targetRel)
	if err != nil {
		return errorProjection("note.move", err), err
	}
	if _, err := os.Stat(target); err == nil {
		err := &domain.CommandError{Code: "note_path_conflict", Message: "目标笔记路径已存在", Hint: "换一个目录或先移动现有文件"}
		return domain.NewErrorProjection("note.move", err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection("note.move", err), err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errorProjection("note.move", err), err
	}
	if err := os.Rename(path, target); err != nil {
		return errorProjection("note.move", err), err
	}
	_ = appendEvent(root, "note.move", "success", map[string]string{"from": note.Path, "to": targetRel})
	return noteMutationProjection("note.move", "笔记已移动。", targetRel, map[string]string{"note_id": note.ID, "title": note.Title}), nil
}

func (s *Service) ArchiveNote(_ context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.archive", err), err
	}
	meta["status"] = "archived"
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.archive", err), err
	}
	_ = appendEvent(root, "note.archive", "success", map[string]string{"path": note.Path})
	projection := noteMutationProjection("note.archive", "笔记已归档。", note.Path, meta)
	projection.Facts["status"] = "archived"
	return projection, nil
}

func (s *Service) DeleteNote(_ context.Context, req NoteDeleteRequest) (domain.Projection, error) {
	root, note, path, _, _, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "note delete 需要 --yes", Hint: "确认后追加 --yes；hard delete 还需要 --hard"}
		return domain.NewErrorProjection("note.delete", err), err
	}
	projection := domain.NewProjection("note.delete", "笔记已删除。")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	if req.Hard {
		if err := os.Remove(path); err != nil {
			return errorProjection("note.delete", err), err
		}
		_ = appendEvent(root, "note.delete", "success", map[string]string{"path": note.Path, "hard": "true"})
		projection.Facts["hard"] = "true"
		return projection, nil
	}
	trashRel, err := uniqueTrashRel(root, note.Path, time.Now().UTC())
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	trashPath, err := safeJoin(root, trashRel)
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return errorProjection("note.delete", err), err
	}
	if err := os.Rename(path, trashPath); err != nil {
		return errorProjection("note.delete", err), err
	}
	_ = appendEvent(root, "note.delete", "success", map[string]string{"path": note.Path, "trash_path": trashRel})
	projection.Summary = "笔记已移入回收站。"
	projection.Facts["trash_path"] = trashRel
	projection.Data = map[string]any{"note": note, "trash_path": trashRel}
	return projection, nil
}

func (s *Service) TagNote(_ context.Context, req NoteTagRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.tag", err), err
	}
	tags := cleanTags(note.Tags)
	switch req.Operation {
	case "add":
		tags = mergeTags(tags, req.Tags)
	case "remove":
		tags = removeTags(tags, req.Tags)
	case "set":
		tags = cleanTags(req.Tags)
	default:
		err := &domain.CommandError{Code: "invalid_tag_operation", Message: "未知 tag 操作", Hint: "使用 add、remove 或 set"}
		return domain.NewErrorProjection("note.tag", err), err
	}
	meta["tags"] = formatTags(tags)
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.tag", err), err
	}
	_ = appendEvent(root, "note.tag", "success", map[string]string{"path": note.Path, "operation": req.Operation})
	projection := noteMutationProjection("note.tag", "笔记标签已更新。", note.Path, meta)
	projection.Facts["tags"] = strings.Join(tags, ",")
	projection.Data = map[string]any{"note": domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: tags, Project: note.Project, Status: meta["status"]}}
	return projection, nil
}

func (s *Service) SearchNotes(ctx context.Context, req SearchRequest) (SearchResult, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return SearchResult{}, err
	}
	if err := validateSearchDateFilters(req); err != nil {
		return SearchResult{}, err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return SearchResult{}, err
	}
	status, err := noteindex.Inspect(root, notes)
	if err == nil && (status.Status == "fresh" || (status.Status == "stale" && req.AllowStale)) {
		result, searchErr := noteindex.Search(root, noteindex.SearchRequest{Query: req.Query, Tags: cleanTags(req.Tags), Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, LinkTarget: req.LinkTarget, HasAttachment: req.HasAttachment, Limit: req.Limit, Sort: normalizedSearchSort(req.Sort)})
		if searchErr == nil {
			result.IndexStatus = status.Status
			return SearchResult{Engine: result.Engine, IndexStatus: result.IndexStatus, Total: result.Total, Returned: result.Returned, Results: result.Results}, nil
		}
	}
	result := notesearch.Notes(ctx, root, req.Query, notes)
	filtered := filterSearchNotes(result.Notes, req)
	sortFallbackNotes(filtered, normalizedSearchSort(req.Sort))
	limit := req.Limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	items := make([]noteindex.ResultItem, 0, len(filtered))
	for _, note := range filtered {
		items = append(items, noteindex.ResultItem{Note: note, Score: 1, MatchedFields: []string{result.Engine}, Snippet: firstSnippet(note.Body, req.Query)})
	}
	indexStatus := "missing"
	if err == nil && status.Status != "" {
		indexStatus = status.Status
	}
	return SearchResult{Engine: result.Engine, IndexStatus: indexStatus, Total: len(filterSearchNotes(result.Notes, req)), Returned: len(items), Notes: filtered, Results: items}, nil
}

type SearchResult struct {
	Engine      string                 `json:"engine"`
	IndexStatus string                 `json:"index_status,omitempty"`
	Total       int                    `json:"total"`
	Returned    int                    `json:"returned"`
	Notes       []domain.Note          `json:"notes,omitempty"`
	Results     []noteindex.ResultItem `json:"results,omitempty"`
}

func (s *Service) SearchProjection(ctx context.Context, req SearchRequest) (domain.Projection, error) {
	result, err := s.SearchNotes(ctx, req)
	if err != nil {
		return errorProjection("note.search", err), err
	}
	projection := domain.NewProjection("note.search", "搜索完成。")
	projection.Facts["matches"] = fmt.Sprint(result.Returned)
	projection.Facts["total"] = fmt.Sprint(result.Total)
	projection.Facts["returned"] = fmt.Sprint(result.Returned)
	projection.Facts["engine"] = result.Engine
	projection.Facts["sort"] = normalizedSearchSort(req.Sort)
	if result.IndexStatus != "" {
		projection.Facts["index_status"] = result.IndexStatus
	}
	if result.Engine == "index" && result.IndexStatus == "stale" {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(req.VaultPath))}}
	}
	addSearchFilterFacts(projection.Facts, req)
	projection.Data = result
	return projection, nil
}

func normalizedSearchSort(sortMode string) string {
	sortMode = strings.TrimSpace(sortMode)
	switch sortMode {
	case "", "relevance":
		return "relevance"
	case "updated", "created", "title", "path":
		return sortMode
	default:
		return "relevance"
	}
}

func sortFallbackNotes(notes []domain.Note, mode string) {
	sort.Slice(notes, func(i, j int) bool {
		a := notes[i]
		b := notes[j]
		switch mode {
		case "title":
			if a.Title == b.Title {
				return a.Path < b.Path
			}
			return a.Title < b.Title
		case "path":
			return a.Path < b.Path
		case "created":
			return noteTimeDesc(a.CreatedAt, b.CreatedAt, a.Path, b.Path)
		case "updated":
			return noteTimeDesc(a.UpdatedAt, b.UpdatedAt, a.Path, b.Path)
		default:
			return a.Path < b.Path
		}
	})
}

func noteTimeDesc(a, b, pathA, pathB string) bool {
	at, aErr := parseUserDate(a)
	bt, bErr := parseUserDate(b)
	if aErr != nil || bErr != nil || at.Equal(bt) {
		return pathA < pathB
	}
	return at.After(bt)
}

func filterSearchNotes(notes []domain.Note, req SearchRequest) []domain.Note {
	filtered := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if req.Group != "" && note.Project != req.Group {
			continue
		}
		if req.Folder != "" && note.Folder != req.Folder {
			continue
		}
		if req.Kind != "" && note.Kind != req.Kind {
			continue
		}
		if req.Status != "" && note.Status != req.Status {
			continue
		}
		if req.CreatedAfter != "" && !noteTimestampAfterOrEqual(note.CreatedAt, req.CreatedAfter) {
			continue
		}
		if req.UpdatedAfter != "" && !noteTimestampAfterOrEqual(note.UpdatedAt, req.UpdatedAfter) {
			continue
		}
		ok := true
		for _, tag := range cleanTags(req.Tags) {
			if !stringSliceContains(cleanTags(note.Tags), tag) {
				ok = false
				break
			}
		}
		if ok {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func addSearchFilterFacts(facts map[string]string, req SearchRequest) {
	if tags := cleanTags(req.Tags); len(tags) > 0 {
		facts["filter.tag"] = strings.Join(tags, ",")
	}
	if req.Group != "" {
		facts["filter.group"] = req.Group
	}
	if req.Folder != "" {
		facts["filter.folder"] = req.Folder
	}
	if req.Kind != "" {
		facts["filter.kind"] = req.Kind
	}
	if req.Status != "" {
		facts["filter.status"] = req.Status
	}
	if req.CreatedAfter != "" {
		facts["filter.created_after"] = req.CreatedAfter
	}
	if req.UpdatedAfter != "" {
		facts["filter.updated_after"] = req.UpdatedAfter
	}
	if req.LinkTarget != "" {
		facts["filter.link_target"] = req.LinkTarget
	}
	if req.HasAttachment {
		facts["filter.has_attachment"] = "true"
	}
}

func validateSearchDateFilters(req SearchRequest) error {
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "created-after", value: req.CreatedAfter},
		{name: "updated-after", value: req.UpdatedAfter},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := parseUserDate(item.value); err != nil {
			return &domain.CommandError{Code: "invalid_date_filter", Message: "日期过滤条件无效", Hint: "使用 YYYY-MM-DD 或 RFC3339 时间，例如 2026-01-01"}
		}
	}
	return nil
}

func noteTimestampAfterOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseUserDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseUserDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.After(boundaryTime)
}

func parseUserDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func firstSnippet(body, query string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		idx := strings.Index(strings.ToLower(body), query)
		if idx >= 0 {
			start := idx - 30
			if start < 0 {
				start = 0
			}
			end := idx + len(query) + 60
			if end > len(body) {
				end = len(body)
			}
			return strings.TrimSpace(body[start:end])
		}
	}
	if len(body) > 120 {
		return body[:120]
	}
	return body
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *Service) PlanMetadata(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	ops := make([]domain.PlanOperation, 0)
	for _, note := range notes {
		if noteNeedsMetadata(note) {
			ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: note.Path, Reason: "补齐 Pinax frontmatter", Status: "planned"})
		}
	}
	projection := domain.NewProjection("metadata.plan", "Metadata 计划已生成。")
	projection.Facts["planned_updates"] = fmt.Sprint(len(ops))
	projection.Data = map[string]any{"operations": ops}
	if len(ops) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax metadata apply --vault %s --yes", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ApplyMetadata(ctx context.Context, req ApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "metadata apply 需要 --yes", Hint: "先运行 pinax metadata plan，确认后追加 --yes"}
		return domain.NewErrorProjection("metadata.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("metadata.apply", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("metadata.apply", err), err
	}
	applied := 0
	for _, note := range notes {
		if !noteNeedsMetadata(note) {
			continue
		}
		path, err := safeJoin(root, note.Path)
		if err != nil {
			return errorProjection("metadata.apply", err), err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return errorProjection("metadata.apply", err), err
		}
		updated := ensureFrontmatter(note, string(content))
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return errorProjection("metadata.apply", err), err
		}
		applied++
		_ = appendEvent(root, "metadata.apply", "success", map[string]string{"path": note.Path})
	}
	projection := domain.NewProjection("metadata.apply", "Metadata 已应用。")
	projection.Facts["applied_updates"] = fmt.Sprint(applied)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	_ = ctx
	return projection, nil
}

func (s *Service) PlanOrganize(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	ops, err := planOrganize(root)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	moves := 0
	for _, op := range ops {
		if op.Kind == "move" && op.Status == "planned" {
			moves++
		}
	}
	projection := domain.NewProjection("organize.plan", "整理计划已生成。")
	projection.Facts["planned_moves"] = fmt.Sprint(moves)
	projection.Data = map[string]any{"operations": ops}
	if moves > 0 {
		projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax git snapshot --vault %s --message %s", shellQuote(root), shellQuote("整理前快照"))}}
	}
	return projection, nil
}

func (s *Service) SuggestOrganize(_ context.Context, req OrganizeSuggestRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.suggest", err), err
	}
	plan, err := buildOrganizePlan(root)
	if err != nil {
		return errorProjection("organize.suggest", err), err
	}
	if req.Save {
		if err := saveOrganizePlan(root, &plan); err != nil {
			return errorProjection("organize.suggest", err), err
		}
	}
	projection := domain.NewProjection("organize.suggest", "整理建议已生成。")
	if len(plan.Operations) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["automatic"] = fmt.Sprint(countOrganizeOperations(plan.Operations, "automatic"))
	projection.Facts["manual_review"] = fmt.Sprint(countOrganizeOperations(plan.Operations, "manual_review"))
	projection.Facts["risk.low"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "low"))
	projection.Facts["risk.medium"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "medium"))
	projection.Facts["risk.review"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "review"))
	if plan.SavedPath != "" {
		projection.Facts["saved_path"] = plan.SavedPath
		projection.Evidence = []string{plan.SavedPath}
	}
	projection.Data = plan
	if plan.SavedPath == "" && len(plan.Operations) > 0 {
		projection.Actions = []domain.Action{{Name: "save", Command: fmt.Sprintf("pinax organize suggest --vault %s --save", shellQuote(root))}}
	} else if plan.SavedPath != "" && len(plan.Operations) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plan.PlanID), shellQuote("整理前快照"))}}
	}
	return projection, nil
}

func (s *Service) ListOrganizePlans(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.list", err), err
	}
	plans, err := listOrganizePlans(root)
	if err != nil {
		return errorProjection("organize.list", err), err
	}
	projection := domain.NewProjection("organize.list", "整理计划已列出。")
	projection.Facts["plans"] = fmt.Sprint(len(plans))
	projection.Data = map[string]any{"plans": plans}
	if len(plans) == 0 {
		projection.Actions = []domain.Action{{Name: "suggest", Command: fmt.Sprintf("pinax organize suggest --vault %s --save", shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plans[0].PlanID), shellQuote("整理前快照"))}}
	}
	return projection, nil
}

func (s *Service) ApplyOrganize(ctx context.Context, req ApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		vault := strings.TrimSpace(req.VaultPath)
		if vault == "" {
			vault = "."
		}
		if root, err := cleanVaultPath(vault); err == nil {
			vault = root
		}
		hint := fmt.Sprintf("先运行 pinax organize suggest --vault %s --save，审核 plan 后再运行 pinax organize apply --vault %s --plan <plan_id> --yes --snapshot-message %s", shellQuote(vault), shellQuote(vault), shellQuote("整理前快照"))
		err := &domain.CommandError{Code: "approval_required", Message: "organize apply 需要 --yes", Hint: hint}
		return domain.NewErrorProjection("organize.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.apply", err), err
	}
	var savedPlan *domain.OrganizePlan
	if strings.TrimSpace(req.PlanID) != "" {
		plan, err := loadOrganizePlan(root, req.PlanID)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := ensureOrganizePlanFresh(root, plan); err != nil {
			projection := errorProjection("organize.apply", err)
			projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax organize suggest --vault %s --save", shellQuote(root))}}
			projection.Data = map[string]any{"plan_id": plan.PlanID}
			return projection, err
		}
		savedPlan = &plan
	}
	if req.SnapshotMessage != "" {
		if _, err := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: req.SnapshotMessage}); err != nil {
			return errorProjection("organize.apply", err), err
		}
	}
	if !gitstore.HasSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "整理结构前需要显式 Git snapshot", Hint: fmt.Sprintf("pinax git snapshot --vault %s --message %s", shellQuote(root), shellQuote("整理前快照"))}
		projection := domain.NewErrorProjection("organize.apply", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		return projection, err
	}
	ops, err := organizeApplyOperations(root, savedPlan)
	if err != nil {
		return errorProjection("organize.apply", err), err
	}
	appliedMetadata := 0
	for _, op := range ops {
		if op.Status != "planned" || (op.Kind != "tag_patch" && op.Kind != "status_patch") {
			continue
		}
		if err := applyOrganizeMetadataOperation(root, op); err != nil {
			return errorProjection("organize.apply", err), err
		}
		appliedMetadata++
		_ = appendEvent(root, "organize.apply", "success", map[string]string{"kind": op.Kind, "path": op.Path})
	}
	appliedMoves := 0
	skipped := 0
	for _, op := range ops {
		if op.Kind == "tag_patch" || op.Kind == "status_patch" {
			continue
		}
		if op.Kind != "move" || op.Status != "planned" {
			skipped++
			continue
		}
		source, err := safeJoin(root, op.Path)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		target, err := safeJoin(root, op.Target)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := os.Rename(source, target); err != nil {
			return errorProjection("organize.apply", err), err
		}
		appliedMoves++
		_ = appendEvent(root, "organize.apply", "success", map[string]string{"from": op.Path, "to": op.Target})
	}
	if savedPlan != nil {
		_ = refreshIndex(root)
	}
	projection := domain.NewProjection("organize.apply", "整理结构已应用。")
	if savedPlan != nil {
		projection.Facts["plan_id"] = savedPlan.PlanID
	}
	projection.Facts["applied_moves"] = fmt.Sprint(appliedMoves)
	projection.Facts["applied_metadata"] = fmt.Sprint(appliedMetadata)
	projection.Facts["applied"] = fmt.Sprint(appliedMoves + appliedMetadata)
	projection.Facts["skipped"] = fmt.Sprint(skipped)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"applied_moves": appliedMoves, "applied_metadata": appliedMetadata, "skipped": skipped}
	return projection, nil
}

func applyOrganizeMetadataOperation(root string, op domain.PlanOperation) error {
	fields := map[string]string{}
	switch op.Kind {
	case "tag_patch":
		fields["tags"] = formatTags(strings.Split(op.Target, ","))
	case "status_patch":
		fields["status"] = op.Target
	default:
		return nil
	}
	return applyRepairFrontmatterPatch(root, op.Path, fields)
}

type SnapshotRequest struct {
	VaultPath string
	Message   string
}

func (s *Service) GitSnapshot(ctx context.Context, req SnapshotRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("git.snapshot", err), err
	}
	if req.Message == "" {
		err := &domain.CommandError{Code: "message_required", Message: "Git snapshot 需要 --message", Hint: "重新运行并提供 --message"}
		return domain.NewErrorProjection("git.snapshot", err), err
	}
	if err := gitstore.Snapshot(ctx, root, req.Message); err != nil {
		return errorProjection("git.snapshot", err), err
	}
	projection := domain.NewProjection("git.snapshot", "Git snapshot 已记录。")
	projection.Facts["vault"] = root
	projection.Facts["message"] = req.Message
	projection.Evidence = []string{".pinax/last_snapshot"}
	return projection, nil
}

func (s *Service) InitTemplates(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.init", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("template.init", err), err
	}
	created := 0
	for name, body := range builtInTemplates() {
		path := filepath.Join(root, ".pinax", "templates", name+".md")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return errorProjection("template.init", err), err
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return errorProjection("template.init", err), err
			}
			created++
		}
	}
	_ = appendEvent(root, "template.init", "success", map[string]string{"created": fmt.Sprint(created)})
	projection := domain.NewProjection("template.init", "内置模板已初始化。")
	projection.Facts["templates"] = fmt.Sprint(len(builtInTemplates()))
	projection.Facts["created"] = fmt.Sprint(created)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "templates"))}
	return projection, nil
}

func (s *Service) ListTemplates(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.list", err), err
	}
	templates, err := listTemplates(root)
	if err != nil {
		return errorProjection("template.list", err), err
	}
	projection := domain.NewProjection("template.list", "模板列表已读取。")
	projection.Facts["templates"] = fmt.Sprint(len(templates))
	projection.Data = map[string]any{"templates": templates}
	return projection, nil
}

func (s *Service) ShowTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.show", err), err
	}
	body, err := loadTemplate(root, req.Name)
	if err != nil {
		return errorProjection("template.show", err), err
	}
	projection := domain.NewProjection("template.show", "模板已读取。")
	projection.Facts["template"] = req.Name
	projection.Data = map[string]any{"template": req.Name, "body": body}
	return projection, nil
}

func (s *Service) RenderTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.render", err), err
	}
	body, err := renderTemplateBody(root, req)
	if err != nil {
		return errorProjection("template.render", err), err
	}
	projection := domain.NewProjection("template.render", "模板已渲染。")
	projection.Facts["template"] = req.Name
	projection.Facts["title"] = req.Title
	projection.Data = map[string]any{"template": req.Name, "body": body}
	return projection, nil
}

func (s *Service) CreateTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("template.create", err), err
	}
	name, err := cleanTemplateName(req.Name)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	body, err := templateSourceBody(req, name)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	if _, err := os.Stat(path); err == nil && !req.Overwrite {
		err := &domain.CommandError{Code: "template_conflict", Message: "模板已存在", Hint: "使用 --overwrite 覆盖，或换一个模板名称"}
		return domain.NewErrorProjection("template.create", err), err
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorProjection("template.create", err), err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("template.create", err), err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return errorProjection("template.create", err), err
	}
	_ = appendEvent(root, "template.create", "success", map[string]string{"template": name})
	projection := domain.NewProjection("template.create", "模板已创建。")
	projection.Facts["template"] = name
	if templateHasDesignFrontmatter(body) {
		projection.Facts["kind"] = "template_design"
	}
	projection.Facts["path"] = filepath.ToSlash(filepath.Join(".pinax", "templates", name+".md"))
	projection.Data = map[string]any{"template": name, "path": projection.Facts["path"]}
	projection.Actions = []domain.Action{{Name: "render", Command: fmt.Sprintf("pinax template render %s --vault %s", shellQuote(name), shellQuote(root))}}
	return projection, nil
}

func (s *Service) ValidateTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.validate", err), err
	}
	name, err := cleanTemplateName(req.Name)
	if err != nil {
		return errorProjection("template.validate", err), err
	}
	body, err := loadTemplate(root, name)
	if err != nil {
		return errorProjection("template.validate", err), err
	}
	issues := validateTemplateContent(body, req)
	projection := domain.NewProjection("template.validate", "模板校验完成。")
	projection.Facts["template"] = name
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["variables"] = strings.Join(templateVariables(body), ",")
	projection.Data = map[string]any{"issues": issues}
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	_ = appendEvent(root, "template.validate", projection.Status, map[string]string{"template": name, "issues": fmt.Sprint(len(issues))})
	return projection, nil
}

func (s *Service) DeleteTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.delete", err), err
	}
	name, err := cleanTemplateName(req.Name)
	if err != nil {
		return errorProjection("template.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "template delete 需要 --yes", Hint: "确认后追加 --yes"}
		return domain.NewErrorProjection("template.delete", err), err
	}
	if _, ok := builtInTemplates()[name]; ok {
		err := &domain.CommandError{Code: "builtin_template_protected", Message: "内置模板受保护", Hint: "复制为自定义模板后再修改或删除"}
		return domain.NewErrorProjection("template.delete", err), err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return errorProjection("template.delete", err), err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err := &domain.CommandError{Code: "template_not_found", Message: "未找到模板", Hint: "运行 pinax template list 查看模板"}
			return domain.NewErrorProjection("template.delete", err), err
		}
		return errorProjection("template.delete", err), err
	}
	_ = appendEvent(root, "template.delete", "success", map[string]string{"template": name})
	projection := domain.NewProjection("template.delete", "模板已删除。")
	projection.Facts["template"] = name
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	return projection, nil
}

func (s *Service) RebuildIndex(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.rebuild", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.rebuild", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.rebuild", err), err
	}
	counts, err := noteindex.Rebuild(root, notes)
	if err != nil {
		return errorProjection("index.rebuild", err), err
	}
	_ = appendEvent(root, "index.rebuild", "success", map[string]string{"notes": fmt.Sprint(counts.Notes)})
	projection := domain.NewProjection("index.rebuild", "本地索引已重建。")
	projection.Facts["notes"] = fmt.Sprint(counts.Notes)
	projection.Facts["tags"] = fmt.Sprint(counts.Tags)
	projection.Facts["links"] = fmt.Sprint(counts.Links)
	projection.Facts["tokens"] = fmt.Sprint(counts.Tokens)
	projection.Facts["attachments"] = fmt.Sprint(counts.Attachments)
	projection.Facts["dimensions"] = fmt.Sprint(counts.Dimensions)
	projection.Facts["schema_version"] = noteindex.SchemaVersion
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"counts": counts}
	return projection, nil
}

func (s *Service) InitIndex(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.init", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.init", err), err
	}
	status, err := noteindex.Init(root)
	if err != nil {
		return errorProjection("index.init", err), err
	}
	projection := domain.NewProjection("index.init", "本地索引数据库已初始化。")
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	projection.Facts["schema_version"] = status.SchemaVersion
	projection.Evidence = []string{status.Path}
	projection.Data = status
	return projection, nil
}

func (s *Service) IndexStatus(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	projection := domain.NewProjection("index.status", "本地索引状态已检查。")
	if status.Status != "fresh" {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
	}
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	if status.SchemaVersion != "" {
		projection.Facts["schema_version"] = status.SchemaVersion
	}
	if status.Notes > 0 {
		projection.Facts["notes"] = fmt.Sprint(status.Notes)
	}
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = status
	return projection, nil
}

func refreshIndex(root string) error {
	notes, err := scanNotes(root)
	if err != nil {
		return err
	}
	_, err = noteindex.Rebuild(root, notes)
	return err
}

func appendDailyIndex(root string, note domain.Note) (string, error) {
	date := time.Now().UTC().Format("2006-01-02")
	rel := filepath.ToSlash(filepath.Join("notes", "daily", date+".md"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	line := dailyIndexLine(note)
	if existing, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(existing), note.Path) {
			return rel, nil
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return "", err
		}
		defer file.Close()
		_, err = file.WriteString(line)
		return rel, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	content := buildNoteContentWithStatus("Daily Index "+date, rel, "", "daily", "index", []string{"daily", "index"}, "", now, "# Daily Index "+date+"\n\n## Notes\n\n"+strings.TrimPrefix(line, "\n"))
	return rel, os.WriteFile(path, []byte(content), 0o644)
}

func ensureDailyNote(vaultPath string) (string, string, string, error) {
	return ensureJournalNote(vaultPath, "daily", DailyRequest{})
}

func ensureJournalNote(vaultPath, period string, req DailyRequest) (string, string, string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", "", "", err
	}
	if err := ensureVaultAssets(root); err != nil {
		return "", "", "", err
	}
	date, err := journalDate(period, req)
	if err != nil {
		return "", "", "", err
	}
	key := journalKey(period, date)
	rel := filepath.ToSlash(filepath.Join("notes", period, key+".md"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", "", "", err
	}
	if _, err := os.Stat(path); err == nil {
		return root, rel, key, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := journalTitle(period, key)
	content := buildNoteContentWithStatus(title, rel, "", period, period, []string{period}, "active", now, "# "+title+"\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", "", "", err
	}
	if err := refreshIndex(root); err != nil {
		return "", "", "", err
	}
	_ = appendEvent(root, period+".create", "success", map[string]string{"path": rel})
	return root, rel, key, nil
}

func journalDate(period string, req DailyRequest) (time.Time, error) {
	date := time.Now().UTC()
	if value := strings.TrimSpace(req.Date); value != "" {
		parsed, err := parseJournalDateValue(period, value)
		if err != nil {
			return time.Time{}, &domain.CommandError{Code: "invalid_journal_date", Message: "journal date 必须是 YYYY-MM-DD、YYYY-Www 或 YYYY-MM", Hint: "使用 --date 2026-06-06、--date 2026-W23 或 --date 2026-06"}
		}
		date = parsed.UTC()
	}
	if req.Prev {
		date = shiftJournalDate(period, date, -1)
	}
	if req.Next {
		date = shiftJournalDate(period, date, 1)
	}
	return date, nil
}

func shiftJournalDate(period string, date time.Time, direction int) time.Time {
	switch period {
	case "weekly":
		return date.AddDate(0, 0, direction*7)
	case "monthly":
		return date.AddDate(0, direction, 0)
	default:
		return date.AddDate(0, 0, direction)
	}
}

func parseJournalDateValue(period, value string) (time.Time, error) {
	switch period {
	case "weekly":
		if date, err := parseJournalISOWeek(value); err == nil {
			return date, nil
		}
	case "monthly":
		if date, err := time.Parse("2006-01", value); err == nil {
			return date, nil
		}
	}
	return parseUserDate(value)
}

func parseJournalISOWeek(value string) (time.Time, error) {
	var year int
	var week int
	if _, err := fmt.Sscanf(value, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()+6)%7)
	return monday.AddDate(0, 0, (week-1)*7), nil
}

func journalKey(period string, date time.Time) string {
	switch period {
	case "weekly":
		year, week := date.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	case "monthly":
		return date.Format("2006-01")
	default:
		return date.Format("2006-01-02")
	}
}

func journalTitle(period, key string) string {
	return journalLabel(period) + " " + key
}

func journalLabel(period string) string {
	switch period {
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	default:
		return "Daily"
	}
}

func appendFile(path, text string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(text)
	return err
}

func dailyIndexLine(note domain.Note) string {
	parts := []string{fmt.Sprintf("- [[%s]]", note.Title), note.Path}
	for _, tag := range cleanTags(note.Tags) {
		parts = append(parts, "#"+tag)
	}
	if note.Project != "" {
		parts = append(parts, "group="+note.Project)
	}
	if note.Folder != "" {
		parts = append(parts, "folder="+note.Folder)
	}
	if note.Kind != "" {
		parts = append(parts, "kind="+note.Kind)
	}
	if note.Status != "" {
		parts = append(parts, "status="+note.Status)
	}
	return strings.Join(parts, " | ") + "\n"
}

func (s *Service) SyncDiff(_ context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.diff", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("sync.diff", err), err
	}
	profile, _ := loadStorageProfile(root)
	projection := domain.NewProjection("sync.diff", "同步计划已生成。")
	projection.Facts["target"] = target
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["backend_required"] = "false"
	plan := syncPlanData(target, profile)
	if target == "cloud" {
		projection.Status = "partial"
		projection.Facts["backend_required"] = "true"
	}
	projection.Data = map[string]any{"target": target, "plan": plan, "remote_write": false}
	projection.Actions = []domain.Action{{Name: "push", Command: fmt.Sprintf("pinax sync push --target %s --vault %s --yes", target, shellQuote(root))}}
	return projection, nil
}

func (s *Service) SyncPush(_ context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.push", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync push 需要 --yes", Hint: "先运行 pinax sync diff 审核计划，确认后追加 --yes"}
		return domain.NewErrorProjection("sync.push", err), err
	}
	return writeSyncState(root, target, "push")
}

func (s *Service) SyncPull(_ context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.pull", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync pull 需要 --yes", Hint: "先运行 pinax sync diff 审核计划，确认后追加 --yes"}
		return domain.NewErrorProjection("sync.pull", err), err
	}
	return writeSyncState(root, target, "pull")
}

func saveProjectRegistryProjection(root string, registry domain.ProjectRegistry, project domain.Project, created bool) (domain.Projection, error) {
	if err := saveProjectRegistry(root, registry); err != nil {
		return errorProjection("project.create", err), err
	}
	status := "updated"
	if created {
		status = "created"
	}
	_ = appendEvent(root, "project.create", "success", map[string]string{"project": project.Slug, "status": status})
	projection := domain.NewProjection("project.create", "项目已创建。")
	projection.Facts["project"] = project.Slug
	projection.Facts["name"] = project.Name
	projection.Facts["notes_prefix"] = project.NotesPrefix
	projection.Facts["current_project"] = registry.CurrentProject
	projection.Data = map[string]any{"project": project, "registry": registry}
	projection.Actions = []domain.Action{{Name: "switch", Command: fmt.Sprintf("pinax project switch %s --vault %s", shellQuote(project.Slug), shellQuote(root))}}
	return projection, nil
}

func loadProjectRegistry(root string) (domain.ProjectRegistry, error) {
	registry := domain.ProjectRegistry{SchemaVersion: "pinax.projects.v1", Projects: []domain.Project{}}
	path := filepath.Join(root, ".pinax", "projects.json")
	b, err := os.ReadFile(path)
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
		registry.SchemaVersion = "pinax.projects.v1"
	}
	if registry.Projects == nil {
		registry.Projects = []domain.Project{}
	}
	return registry, nil
}

func saveProjectRegistry(root string, registry domain.ProjectRegistry) error {
	registry.SchemaVersion = "pinax.projects.v1"
	if registry.Projects == nil {
		registry.Projects = []domain.Project{}
	}
	sort.Slice(registry.Projects, func(i, j int) bool { return registry.Projects[i].Slug < registry.Projects[j].Slug })
	return writeJSONAsset(filepath.Join(root, ".pinax", "projects.json"), registry)
}

func validateProjectSlug(slug string) error {
	if slug == "" {
		return &domain.CommandError{Code: "project_slug_required", Message: "项目需要 slug", Hint: "运行 pinax project create <slug> --name <name>"}
	}
	for _, r := range slug {
		if unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return &domain.CommandError{Code: "invalid_project_slug", Message: "项目 slug 只能包含小写字母、数字、- 和 _", Hint: "例如 pinax project create research"}
	}
	return nil
}

func validateProjectPrefix(prefix string) error {
	clean := filepath.ToSlash(filepath.Clean(prefix))
	if clean == "." || filepath.IsAbs(prefix) || strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, ".pinax") {
		return &domain.CommandError{Code: "unsafe_project_prefix", Message: "项目 notes prefix 必须位于 vault 内且不能指向 .pinax", Hint: "使用类似 notes/research 的 prefix"}
	}
	return nil
}

func loadStorageProfile(root string) (domain.StorageProfile, error) {
	defaultProfile := domain.StorageProfile{SchemaVersion: "pinax.storage.v1", Backend: "local", Local: &domain.LocalStorage{Root: root}}
	path := filepath.Join(root, ".pinax", "storage.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultProfile, nil
	}
	if err != nil {
		return domain.StorageProfile{}, err
	}
	profile := domain.StorageProfile{}
	if err := json.Unmarshal(b, &profile); err != nil {
		return domain.StorageProfile{}, err
	}
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = "pinax.storage.v1"
	}
	if profile.Backend == "" {
		profile.Backend = "local"
	}
	return profile, nil
}

func saveStorageProfile(root string, profile domain.StorageProfile) error {
	profile.SchemaVersion = "pinax.storage.v1"
	return writeJSONAsset(filepath.Join(root, ".pinax", "storage.json"), profile)
}

func storageProjection(command, summary string, profile domain.StorageProfile) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["backend"] = profile.Backend
	switch profile.Backend {
	case "s3":
		if profile.S3 != nil {
			projection.Facts["bucket"] = profile.S3.Bucket
			projection.Facts["region"] = profile.S3.Region
			if profile.S3.Prefix != "" {
				projection.Facts["prefix"] = profile.S3.Prefix
			}
			if profile.S3.Endpoint != "" {
				projection.Facts["endpoint"] = profile.S3.Endpoint
			}
			credentialSource := "environment"
			if profile.S3.Profile != "" {
				credentialSource = "profile:" + profile.S3.Profile
			}
			projection.Facts["credential_source"] = credentialSource
		}
	case "local":
		if profile.Local != nil {
			projection.Facts["root"] = profile.Local.Root
		}
	}
	projection.Data = map[string]any{"storage": profile, "network_checked": false}
	return projection
}

func writeJSONAsset(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func ensureVaultAssets(root string) error {
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		return err
	}
	return ensureEventLog(root)
}

func planOrganize(root string) ([]domain.PlanOperation, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, note := range notes {
		seen[note.Path] = note.Path
	}
	ops := make([]domain.PlanOperation, 0)
	for _, note := range notes {
		if !isOrganizeRootNoteCandidate(note.Path) {
			continue
		}
		slug := slugify(note.Title)
		if slug == "" {
			slug = strings.TrimSuffix(strings.ToLower(filepath.Base(note.Path)), filepath.Ext(note.Path))
		}
		target := filepath.ToSlash(filepath.Join("notes", slug+".md"))
		if note.Path == target {
			continue
		}
		if existing, ok := seen[target]; ok && existing != note.Path {
			ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "目标路径已存在", Status: "conflict"})
			continue
		}
		ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "按标题归入 notes/", Status: "planned"})
	}
	return ops, nil
}

func buildOrganizePlan(root string) (domain.OrganizePlan, error) {
	ops, err := planOrganize(root)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	facts = organizeCandidateFacts(facts)
	ops = append(ops, organizeFactOperations(root, facts)...)
	created := time.Now().UTC()
	planID := organizePlanID(root, ops, created)
	plan := domain.OrganizePlan{
		SchemaVersion: "pinax.organize_plan.v1",
		PlanID:        planID,
		CreatedAt:     created.Format(time.RFC3339),
		ExpiresAt:     created.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		VaultRoot:     root,
		SourceCommand: fmt.Sprintf("pinax organize suggest --vault %s", shellQuote(root)),
		SourceFacts:   organizeSourceFacts(facts),
		Operations:    make([]domain.OrganizeOperation, 0, len(ops)),
		Status:        "planned",
	}
	for _, op := range ops {
		plan.Operations = append(plan.Operations, organizeOperationFromPlan(planID, op))
	}
	return plan, nil
}

func organizeCandidateFacts(facts []noteFact) []noteFact {
	candidates := make([]noteFact, 0, len(facts))
	for _, fact := range facts {
		if isOrganizeFactCandidate(fact.rel) {
			candidates = append(candidates, fact)
		}
	}
	return candidates
}

func isOrganizeFactCandidate(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if strings.HasPrefix(rel, "notes/") {
		return true
	}
	return isOrganizeRootNoteCandidate(rel)
}

func isOrganizeRootNoteCandidate(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.Contains(rel, "/") || !strings.EqualFold(filepath.Ext(rel), ".md") {
		return false
	}
	switch strings.ToLower(filepath.Base(rel)) {
	case "agents.md", "claude.md", "readme.md", "license.md", "contributing.md":
		return false
	default:
		return true
	}
}

func organizeOperationFromPlan(planID string, op domain.PlanOperation) domain.OrganizeOperation {
	mode := "manual_review"
	risk := "review"
	if op.Status == "planned" && (op.Kind == "move" || op.Kind == "tag_patch" || op.Kind == "status_patch") {
		mode = "automatic"
		risk = "low"
	}
	before := map[string]string{"path": op.Path}
	after := map[string]string{"path": op.Target}
	switch op.Kind {
	case "tag_patch":
		before = map[string]string{"tags": ""}
		after = map[string]string{"tags": op.Target}
	case "kind_patch":
		before = map[string]string{"kind": ""}
		after = map[string]string{"kind": op.Target}
	case "status_patch":
		before = map[string]string{"status": ""}
		after = map[string]string{"status": op.Target}
	case "link_resolution":
		before = map[string]string{"link_target": op.Target}
		after = map[string]string{"resolution": "manual"}
	case "attachment_repair":
		before = map[string]string{"attachment": op.Target}
		after = map[string]string{"repair": "manual"}
	case "manual_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": "required"}
	}
	return domain.OrganizeOperation{
		OperationID: organizeOperationID(planID, op),
		Kind:        op.Kind,
		Mode:        mode,
		Risk:        risk,
		Path:        op.Path,
		Target:      op.Target,
		Before:      before,
		After:       after,
		Reason:      op.Reason,
		Evidence:    []string{"path=" + op.Path, "target=" + op.Target},
		Status:      op.Status,
	}
}

func organizeFactOperations(root string, facts []noteFact) []domain.PlanOperation {
	byTitle := map[string]domain.Note{}
	byPath := map[string]domain.Note{}
	for _, fact := range facts {
		byTitle[strings.ToLower(fact.note.Title)] = fact.note
		byPath[fact.note.Path] = fact.note
	}
	ops := make([]domain.PlanOperation, 0)
	for _, fact := range facts {
		inlineTags := cleanTags(noteAllTags(fact.note))
		if len(fact.note.Tags) == 0 && len(inlineTags) > 0 {
			ops = append(ops, domain.PlanOperation{Kind: "tag_patch", Path: fact.rel, Target: strings.Join(inlineTags, ","), Reason: "从正文 inline tags 补齐 frontmatter tags", Status: "planned"})
		}
		if strings.TrimSpace(fact.note.Kind) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "kind_patch", Path: fact.rel, Target: inferNoteKind(fact.note), Reason: "缺少用途分类，需要确认", Status: "manual_review"})
		}
		if strings.TrimSpace(fact.note.Status) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "status_patch", Path: fact.rel, Target: "active", Reason: "缺少状态，建议设为 active", Status: "planned"})
		}
		for _, link := range noteGraphLinks(fact.note, byTitle, byPath) {
			if link.Broken {
				ops = append(ops, domain.PlanOperation{Kind: "link_resolution", Path: fact.rel, Target: link.Target, Reason: "存在未解析链接，需要人工确认目标", Status: "manual_review"})
			}
		}
		for _, attachment := range noteAttachmentsFromBody(root, fact.note) {
			if !attachment.Exists {
				ops = append(ops, domain.PlanOperation{Kind: "attachment_repair", Path: fact.rel, Target: attachment.TargetPath, Reason: "附件引用缺失，需要修复或移除", Status: "manual_review"})
			}
		}
		if !fact.hasFrontmatter {
			ops = append(ops, domain.PlanOperation{Kind: "manual_review", Path: fact.rel, Target: "frontmatter", Reason: "缺少 Pinax frontmatter，需要确认 metadata", Status: "manual_review"})
		}
	}
	return ops
}

func inferNoteKind(note domain.Note) string {
	path := strings.ToLower(note.Path)
	for _, tag := range noteAllTags(note) {
		switch strings.ToLower(tag) {
		case "daily":
			return "daily"
		case "meeting":
			return "meeting"
		case "project":
			return "project"
		}
	}
	if strings.Contains(path, "daily/") {
		return "daily"
	}
	return "reference"
}

func organizeSourceFacts(facts []noteFact) map[string]string {
	source := map[string]string{"notes": fmt.Sprint(len(facts))}
	for _, fact := range facts {
		path := "note." + fact.rel
		source[path+".mtime"] = fact.modTime.UTC().Format(time.RFC3339Nano)
		source[path+".size"] = fmt.Sprint(fact.size)
		source[path+".sha1"] = noteFactHash(fact)
	}
	return source
}

func organizePlanID(root string, ops []domain.PlanOperation, created time.Time) string {
	parts := []string{root, created.Format(time.RFC3339Nano)}
	for _, op := range ops {
		parts = append(parts, op.Kind, op.Path, op.Target, op.Status)
	}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "organize-" + hex.EncodeToString(h[:])[:12]
}

func organizeOperationID(planID string, op domain.PlanOperation) string {
	h := sha1.Sum([]byte(planID + "\x00" + op.Kind + "\x00" + op.Path + "\x00" + op.Target))
	return "op-" + hex.EncodeToString(h[:])[:12]
}

func saveOrganizePlan(root string, plan *domain.OrganizePlan) error {
	dir, err := safeJoin(root, ".pinax/organize-plans")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "organize-plans", plan.PlanID+".json"))
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

func loadOrganizePlan(root, planRef string) (domain.OrganizePlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.OrganizePlan{}, &domain.CommandError{Code: "plan_required", Message: "organize plan id 不能为空", Hint: "运行 pinax organize suggest --save 生成计划"}
	}
	rel := planRef
	if !strings.HasPrefix(rel, ".pinax/organize-plans/") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "organize-plans", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	var plan domain.OrganizePlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.OrganizePlan{}, err
	}
	if plan.SchemaVersion != "pinax.organize_plan.v1" {
		return domain.OrganizePlan{}, &domain.CommandError{Code: "organize_plan_schema_invalid", Message: "organize plan schema 不受支持", Hint: "重新运行 pinax organize suggest --save"}
	}
	if plan.SavedPath == "" {
		plan.SavedPath = filepath.ToSlash(rel)
	}
	return plan, nil
}

func listOrganizePlans(root string) ([]domain.OrganizePlanSummary, error) {
	dir, err := safeJoin(root, ".pinax/organize-plans")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.OrganizePlanSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	plans := make([]domain.OrganizePlanSummary, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		plan, err := loadOrganizePlan(root, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err != nil {
			continue
		}
		plans = append(plans, domain.OrganizePlanSummary{PlanID: plan.PlanID, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt, Status: plan.Status, Operations: len(plan.Operations), SavedPath: plan.SavedPath})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].CreatedAt > plans[j].CreatedAt })
	return plans, nil
}

func ensureOrganizePlanFresh(root string, plan domain.OrganizePlan) error {
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "organize_plan_not_planned", Message: "organize plan 状态不可应用", Hint: "重新运行 pinax organize suggest --save"}
	}
	expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
	if err == nil && time.Now().UTC().After(expires) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan 已过期", Hint: "pinax organize suggest --vault <vault> --save"}
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return err
	}
	current := organizeSourceFacts(facts)
	if len(current) != len(plan.SourceFacts) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan 与当前 vault facts 不一致", Hint: fmt.Sprintf("pinax organize suggest --vault %s --save", shellQuote(root))}
	}
	for key, value := range plan.SourceFacts {
		if current[key] != value {
			return &domain.CommandError{Code: "plan_stale", Message: "organize plan 与当前 vault facts 不一致", Hint: fmt.Sprintf("pinax organize suggest --vault %s --save", shellQuote(root))}
		}
	}
	return nil
}

func organizeApplyOperations(root string, plan *domain.OrganizePlan) ([]domain.PlanOperation, error) {
	if plan == nil {
		return planOrganize(root)
	}
	ops := make([]domain.PlanOperation, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		status := op.Status
		if status == "" {
			status = "planned"
		}
		if op.Mode == "manual_review" {
			status = "manual_review"
		}
		ops = append(ops, domain.PlanOperation{Kind: op.Kind, Path: op.Path, Target: op.Target, Reason: op.Reason, Status: status})
	}
	return ops, nil
}

func countOrganizeOperations(ops []domain.OrganizeOperation, mode string) int {
	count := 0
	for _, op := range ops {
		if op.Mode == mode {
			count++
		}
	}
	return count
}

func countOrganizeRisks(ops []domain.OrganizeOperation, risk string) int {
	count := 0
	for _, op := range ops {
		if op.Risk == risk {
			count++
		}
	}
	return count
}

func scanNotes(root string) ([]domain.Note, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, err
	}
	notes := make([]domain.Note, 0)
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
		note := parseNote(filepath.ToSlash(rel), string(content))
		if isSystemIndexNote(note) {
			return nil
		}
		notes = append(notes, note)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	return notes, nil
}

func shouldSkipVaultWalkDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "dist"
}

func isSystemIndexNote(note domain.Note) bool {
	return note.Kind == "index" && strings.HasPrefix(filepath.ToSlash(note.Path), "notes/daily/")
}

func parseNote(rel, content string) domain.Note {
	meta, body := splitFrontmatter(content)
	title := meta["title"]
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	return domain.Note{
		ID:        meta["note_id"],
		Title:     title,
		Path:      rel,
		Tags:      parseTags(meta["tags"]),
		Body:      strings.TrimSpace(body),
		Project:   meta["project"],
		Folder:    meta["folder"],
		Kind:      meta["kind"],
		Status:    meta["status"],
		CreatedAt: meta["created_at"],
		UpdatedAt: meta["updated_at"],
	}
}

func splitFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(content, "---\n") {
		return meta, content
	}
	scanner := bufio.NewScanner(strings.NewReader(content[4:]))
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			remaining := strings.TrimPrefix(content, "---\n"+strings.Join(lines, "\n")+"\n---")
			for _, item := range lines {
				key, value, ok := strings.Cut(item, ":")
				if ok {
					meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
				}
			}
			return meta, strings.TrimPrefix(remaining, "\n")
		}
		lines = append(lines, line)
	}
	return meta, content
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "\"")
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

func noteNeedsMetadata(note domain.Note) bool {
	return note.ID == "" || note.Title == "" || len(note.Tags) == 0
}

func ensureFrontmatter(note domain.Note, content string) string {
	meta, body := splitFrontmatter(content)
	if meta["schema_version"] == "" {
		meta["schema_version"] = "pinax.note.v1"
	}
	if meta["note_id"] == "" {
		meta["note_id"] = stableNoteID(note.Path)
	}
	if meta["title"] == "" {
		meta["title"] = note.Title
	}
	if meta["tags"] == "" {
		meta["tags"] = "[]"
	}
	// 固定 frontmatter key 顺序，避免 agent 或用户多次 apply 造成无意义 diff。
	keys := []string{"schema_version", "note_id", "title", "tags"}
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(meta[key])
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	return b.String()
}

func stableNoteID(path string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(path)))
	return "note_" + hex.EncodeToString(sum[:])[:12]
}

func slugify(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func findProject(root, slug string) (domain.Project, error) {
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return domain.Project{}, err
	}
	for _, project := range registry.Projects {
		if project.Slug == slug {
			return project, nil
		}
	}
	return domain.Project{}, &domain.CommandError{Code: "project_not_found", Message: "未找到项目", Hint: "运行 pinax project list 查看可用项目"}
}

func nextNotePath(root, rel string) (string, error) {
	base := strings.TrimSuffix(rel, filepath.Ext(rel))
	ext := filepath.Ext(rel)
	candidate := rel
	for i := 2; ; i++ {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return filepath.ToSlash(candidate), nil
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func buildNoteContentWithStatus(title, rel, project, folder, kind string, tags []string, status, now, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: pinax.note.v1\n")
	b.WriteString("note_id: ")
	b.WriteString(stableNoteID(rel))
	b.WriteString("\n")
	b.WriteString("title: ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString("tags: ")
	b.WriteString(formatTags(cleanTags(tags)))
	b.WriteString("\n")
	if project != "" {
		b.WriteString("project: ")
		b.WriteString(project)
		b.WriteString("\n")
	}
	if folder != "" {
		b.WriteString("folder: ")
		b.WriteString(folder)
		b.WriteString("\n")
	}
	if kind != "" {
		b.WriteString("kind: ")
		b.WriteString(kind)
		b.WriteString("\n")
	}
	if status != "" {
		b.WriteString("status: ")
		b.WriteString(status)
		b.WriteString("\n")
	}
	b.WriteString("created_at: ")
	b.WriteString(now)
	b.WriteString("\nupdated_at: ")
	b.WriteString(now)
	b.WriteString("\n---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return b.String()
}

func noteBodyFromRequest(req CreateNoteRequest) (string, error) {
	sources := 0
	for _, value := range []string{req.Body, req.SourcePath, req.StdinBody} {
		if value != "" {
			sources++
		}
	}
	if sources > 1 {
		return "", &domain.CommandError{Code: "note_source_conflict", Message: "note new 只能选择一个正文来源", Hint: "只保留 --body、--from 或 --stdin 之一"}
	}
	if req.SourcePath != "" {
		b, err := os.ReadFile(req.SourcePath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if req.StdinBody != "" {
		return req.StdinBody, nil
	}
	return req.Body, nil
}

type editorCommand struct {
	Raw        string   `json:"raw"`
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

func parseEditorCommand(value string) (editorCommand, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "未配置编辑器", Hint: "设置 $EDITOR 或传 --editor"}
	}
	parts, err := splitCommandLine(value)
	if err != nil {
		return editorCommand{}, &domain.CommandError{Code: "editor_parse_failed", Message: "编辑器命令无法解析", Hint: "使用简单命令或传 wrapper script"}
	}
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "未配置编辑器", Hint: "设置 $EDITOR 或传 --editor"}
	}
	return editorCommand{Raw: value, Executable: parts[0], Args: parts[1:]}, nil
}

func splitCommandLine(value string) ([]string, error) {
	var parts []string
	var b strings.Builder
	quote := rune(0)
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\n':
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts, nil
}

func noteCreatePrefix(root string, req CreateNoteRequest) (string, error) {
	if req.Dir != "" {
		return validateNoteDir(req.Dir)
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return "", err
	}
	base := "notes"
	if req.Project != "" {
		project, err := findProject(root, req.Project)
		if err != nil {
			return "", err
		}
		base = project.NotesPrefix
	}
	if folder != "" {
		return filepath.ToSlash(filepath.Join(base, folder)), nil
	}
	return base, nil
}

func validateOptionalNoteFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", nil
	}
	clean := filepath.ToSlash(filepath.Clean(folder))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(folder) || clean == ".pinax" || strings.HasPrefix(clean, ".pinax/") || clean == "notes" || strings.HasPrefix(clean, "notes/") {
		return "", &domain.CommandError{Code: "unsafe_note_folder", Message: "note folder 必须是项目或 notes 下的相对目录", Hint: "使用类似 inbox、reference 或 work/research 的 folder"}
	}
	return clean, nil
}

func validateNoteDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "notes", nil
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(dir) || clean == ".pinax" || strings.HasPrefix(clean, ".pinax/") {
		return "", &domain.CommandError{Code: "unsafe_note_path", Message: "note 目录必须位于 vault 的 notes/ 内", Hint: "使用类似 work 或 notes/work 的目录"}
	}
	if clean == "notes" || strings.HasPrefix(clean, "notes/") {
		return clean, nil
	}
	return filepath.ToSlash(filepath.Join("notes", clean)), nil
}

func validateNoteSlug(slug string) error {
	clean := filepath.ToSlash(filepath.Clean(slug))
	if clean == "." || clean == ".." || strings.Contains(clean, "/") || strings.HasPrefix(clean, ".") || filepath.IsAbs(slug) {
		return &domain.CommandError{Code: "invalid_note_slug", Message: "note slug 只能是单个安全文件名", Hint: "使用类似 daily-review 的 slug"}
	}
	return nil
}

type noteRefAmbiguousError struct {
	*domain.CommandError
	Ref        string
	Candidates []domain.Note
}

func (e *noteRefAmbiguousError) Unwrap() error { return e.CommandError }

func resolveNoteRef(notes []domain.Note, ref string) (domain.Note, error) {
	// 只接受确定性匹配：标题必须唯一，否则宁可失败并返回候选，避免误打开或误改用户笔记。
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return domain.Note{}, &domain.CommandError{Code: "note_ref_required", Message: "需要 note 引用", Hint: "传入 note_id、路径或标题"}
	}
	needle := filepath.ToSlash(strings.TrimPrefix(ref, "notes/"))
	var titleMatches []domain.Note
	for _, note := range notes {
		if note.ID == ref || note.Path == ref || strings.TrimPrefix(note.Path, "notes/") == needle {
			return note, nil
		}
		if note.Title == ref {
			titleMatches = append(titleMatches, note)
		}
	}
	if len(titleMatches) == 1 {
		return titleMatches[0], nil
	}
	if len(titleMatches) > 1 {
		return domain.Note{}, &noteRefAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "笔记引用有多个候选", Hint: "使用 note_id 或完整路径重试"}, Ref: ref, Candidates: titleMatches}
	}
	return domain.Note{}, &domain.CommandError{Code: "note_not_found", Message: "未找到笔记", Hint: "运行 pinax note list 查看可用笔记"}
}

func noteMatchesQuery(note domain.Note, req NoteListRequest) bool {
	// list query 只做显式维度过滤，不做模糊标题/正文匹配，保持 CLI 输出可预测。
	if req.Project != "" && note.Project != req.Project {
		return false
	}
	if req.Group != "" && note.Project != req.Group {
		return false
	}
	if req.Folder != "" && note.Folder != req.Folder {
		return false
	}
	if req.Kind != "" && note.Kind != req.Kind {
		return false
	}
	if req.Status != "" && note.Status != req.Status {
		return false
	}
	if req.CreatedAfter != "" && !noteTimestampAfterOrEqual(note.CreatedAt, req.CreatedAfter) {
		return false
	}
	if req.UpdatedBefore != "" && !noteTimestampBeforeOrEqual(note.UpdatedAt, req.UpdatedBefore) {
		return false
	}
	if req.PathPrefix != "" && !strings.HasPrefix(note.Path, filepath.ToSlash(req.PathPrefix)) {
		return false
	}
	for _, tag := range req.Tags {
		if tag != "" && !containsString(noteAllTags(note), strings.TrimPrefix(tag, "#")) {
			return false
		}
	}
	return true
}

func noteTimestampBeforeOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseUserDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseUserDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.Before(boundaryTime)
}

func sortNotes(notes []domain.Note, req NoteListRequest) {
	sort.SliceStable(notes, func(i, j int) bool {
		switch req.Sort {
		case "path":
			return notes[i].Path < notes[j].Path
		case "title":
			return notes[i].Title < notes[j].Title
		default:
			return notes[i].UpdatedAt > notes[j].UpdatedAt
		}
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func loadMutableNote(vaultPath, noteRef string) (string, domain.Note, string, string, map[string]string, string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	note, err := resolveNoteRef(notes, noteRef)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	meta, body := splitFrontmatter(string(b))
	if meta["schema_version"] == "" {
		meta["schema_version"] = "pinax.note.v1"
	}
	if meta["note_id"] == "" {
		meta["note_id"] = stableNoteID(note.Path)
	}
	if meta["title"] == "" {
		meta["title"] = note.Title
	}
	if meta["tags"] == "" {
		meta["tags"] = formatTags(note.Tags)
	}
	return root, note, path, string(b), meta, body, nil
}

func patchFrontmatterFields(content string, fields map[string]string) (string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		meta := map[string]string{}
		for k, v := range fields {
			meta[k] = v
		}
		return renderFrontmatter(meta, strings.TrimLeft(content, "\n")), true
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		meta := map[string]string{}
		for k, v := range fields {
			meta[k] = v
		}
		return renderFrontmatter(meta, content), true
	}
	frontStart := 4
	frontEnd := 4 + end
	front := content[frontStart:frontEnd]
	body := strings.TrimPrefix(content[frontEnd+len("\n---"):], "\n")
	lines := strings.Split(front, "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if value, ok := fields[key]; ok {
			lines[i] = key + ": " + value
			seen[key] = true
		}
	}
	for _, key := range orderedFrontmatterKeys(fields) {
		if !seen[key] && strings.TrimSpace(fields[key]) != "" {
			lines = append(lines, key+": "+fields[key])
		}
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n\n" + strings.TrimLeft(body, "\n"), false
}

func orderedFrontmatterKeys(fields map[string]string) []string {
	preferred := []string{"schema_version", "note_id", "title", "tags", "project", "folder", "kind", "status", "created_at", "updated_at"}
	seen := map[string]bool{}
	var keys []string
	for _, key := range preferred {
		if _, ok := fields[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var extra []string
	for key := range fields {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return append(keys, extra...)
}

func commitNoteContent(currentPath, targetPath, content string) error {
	// 同目录临时文件可让“最终替换”尽量接近原子操作；在 commit 前失败时原文件保持不变。
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".pinax-note-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanup = false
	if filepath.Clean(currentPath) != filepath.Clean(targetPath) {
		if err := os.Remove(currentPath); err != nil {
			return err
		}
	}
	return nil
}

func uniqueTrashRel(root, notePath string, now time.Time) (string, error) {
	base := filepath.ToSlash(filepath.Join(".pinax", "trash", now.UTC().Format("20060102"), strings.TrimPrefix(notePath, "notes/")))
	candidate := base
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
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
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

func renderFrontmatter(meta map[string]string, body string) string {
	keys := []string{"schema_version", "note_id", "title", "tags", "project", "folder", "kind", "status", "created_at", "updated_at"}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		seen[key] = true
		if value := strings.TrimSpace(meta[key]); value != "" {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
	extra := make([]string, 0)
	for key := range meta {
		if !seen[key] && strings.TrimSpace(meta[key]) != "" {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(meta[key])
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	return b.String()
}

func noteMutationProjection(command, summary, path string, meta map[string]string) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["path"] = path
	projection.Facts["note_id"] = meta["note_id"]
	projection.Facts["title"] = meta["title"]
	projection.Data = map[string]any{"path": path, "frontmatter": meta}
	return projection
}

func mergeTags(existing, add []string) []string {
	seen := map[string]bool{}
	for _, tag := range cleanTags(existing) {
		seen[tag] = true
	}
	for _, tag := range cleanTags(add) {
		seen[tag] = true
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func removeTags(existing, remove []string) []string {
	blocked := map[string]bool{}
	for _, tag := range cleanTags(remove) {
		blocked[tag] = true
	}
	out := make([]string, 0)
	for _, tag := range cleanTags(existing) {
		if !blocked[tag] {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

func buildNoteContent(title, rel, project string, tags []string, now, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: pinax.note.v1\n")
	b.WriteString("note_id: ")
	b.WriteString(stableNoteID(rel))
	b.WriteString("\n")
	b.WriteString("title: ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString("tags: ")
	b.WriteString(formatTags(cleanTags(tags)))
	b.WriteString("\n")
	if project != "" {
		b.WriteString("project: ")
		b.WriteString(project)
		b.WriteString("\n")
	}
	b.WriteString("created_at: ")
	b.WriteString(now)
	b.WriteString("\nupdated_at: ")
	b.WriteString(now)
	b.WriteString("\n---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return b.String()
}

var templateVariablePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_:-]*)\s*\}\}`)
var templateVariableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_:-]*$`)

func cleanTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !isTemplateNameSafe(name) {
		return "", &domain.CommandError{Code: "invalid_template_name", Message: "模板名称只能包含字母、数字、- 和 _", Hint: "例如 pinax template create meeting --body '# {{title}}'"}
	}
	return name, nil
}

func isTemplateNameSafe(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			continue
		}
		if i > 0 && (r == '-' || r == '_') {
			continue
		}
		return false
	}
	return true
}

func templatePath(root, name string) (string, error) {
	return safeJoin(root, filepath.ToSlash(filepath.Join(".pinax", "templates", name+".md")))
}

func templateSourceBody(req TemplateRequest, name string) (string, error) {
	sources := 0
	if req.SourcePath != "" {
		sources++
	}
	if req.Body != "" {
		sources++
	}
	if req.UseStdin {
		sources++
	}
	if sources == 0 {
		return templateDesignBody(name), nil
	}
	if sources > 1 {
		return "", &domain.CommandError{Code: "template_source_conflict", Message: "template create 只能选择一个模板来源", Hint: "只保留 --from、--body 或 --stdin 之一"}
	}
	if req.SourcePath != "" {
		b, err := os.ReadFile(req.SourcePath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return req.Body, nil
}

func templateDesignBody(name string) string {
	return fmt.Sprintf("---\nschema_version: pinax.template_design.v1\nkind: template_design\ntitle: %s\n---\n\n## 模板正文\n\n# {{title}}\n", name)
}

func templateHasDesignFrontmatter(body string) bool {
	return strings.Contains(body, "schema_version: pinax.template_design.v1") && strings.Contains(body, "kind: template_design")
}

func validateTemplateVars(vars map[string]string) error {
	for key := range vars {
		if !templateVariableNamePattern.MatchString(key) {
			return &domain.CommandError{Code: "template_variable_invalid", Message: "模板变量 key 非法", Hint: "使用 --var key=value，key 只能包含字母、数字、_、: 或 -，且不能以数字开头"}
		}
	}
	return nil
}

func templateContext(req TemplateRequest) (map[string]string, error) {
	if err := validateTemplateVars(req.Vars); err != nil {
		return nil, err
	}
	if req.Title == "" {
		req.Title = "Untitled"
	}
	now := time.Now().UTC()
	ctx := map[string]string{
		"title":    req.Title,
		"date":     now.Format("2006-01-02"),
		"datetime": now.Format(time.RFC3339),
		"project":  req.Project,
		"tags":     strings.Join(cleanTags(req.Tags), ", "),
	}
	for key, value := range req.Vars {
		ctx[key] = value
	}
	return ctx, nil
}

func templateVariables(body string) []string {
	seen := map[string]bool{}
	for _, match := range templateVariablePattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			seen[match[1]] = true
		}
	}
	vars := make([]string, 0, len(seen))
	for key := range seen {
		vars = append(vars, key)
	}
	sort.Strings(vars)
	return vars
}

func missingTemplateVariables(body string, ctx map[string]string) []string {
	missing := make([]string, 0)
	for _, key := range templateVariables(body) {
		if _, ok := ctx[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func validateTemplateContent(body string, req TemplateRequest) []domain.Issue {
	issues := make([]domain.Issue, 0)
	if strings.TrimSpace(body) == "" {
		issues = append(issues, domain.Issue{Code: "template_empty", Message: "模板为空"})
	}
	if strings.HasPrefix(body, "---\n") {
		_, rest := splitFrontmatter(body)
		if rest == body {
			issues = append(issues, domain.Issue{Code: "template_frontmatter_unclosed", Message: "frontmatter 未闭合"})
		}
	}
	// 代码围栏是 Markdown/Mermaid/YAML 模板最容易破坏生成结果的地方；这里仅跟踪 fence 奇偶，保持实现可审计且不解析 Markdown 全语法。
	fenceOpen := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenceOpen = !fenceOpen
		}
	}
	if fenceOpen {
		issues = append(issues, domain.Issue{Code: "template_fence_unclosed", Message: "Markdown code fence 未闭合"})
	}
	for _, key := range templateVariables(body) {
		if !templateVariableNamePattern.MatchString(key) {
			issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: "模板变量 key 非法: " + key})
		}
	}
	if err := validateTemplateVars(req.Vars); err != nil {
		issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: err.Error()})
	}
	return issues
}

func builtInTemplates() map[string]string {
	return map[string]string{
		"note":    "# {{title}}\n\n",
		"daily":   "# {{date}}\n\n## 今日记录\n\n- \n",
		"project": "# {{title}}\n\nproject: {{project}}\ntags: {{tags}}\n\n## 目标\n\n## 进展\n",
		"yaml":    "```yaml\ntitle: {{title}}\nproject: {{project}}\ntags: [{{tags}}]\nupdated_at: {{datetime}}\n```\n",
		"mermaid": "# {{title}}\n\n```mermaid\nflowchart TD\n    A[{{title}}] --> B[{{project}}]\n```\n",
	}
}

func listTemplates(root string) ([]string, error) {
	dir := filepath.Join(root, ".pinax", "templates")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

func loadTemplate(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "note"
	}
	name, err := cleanTemplateName(name)
	if err != nil {
		return "", err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if body, ok := builtInTemplates()[name]; ok {
			return body, nil
		}
		return "", &domain.CommandError{Code: "template_not_found", Message: "未找到模板", Hint: "运行 pinax template init 初始化内置模板"}
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func renderTemplateBody(root string, req TemplateRequest) (string, error) {
	body, err := loadTemplate(root, req.Name)
	if err != nil {
		return "", err
	}
	for _, issue := range validateTemplateContent(body, TemplateRequest{}) {
		if issue.Code == "template_frontmatter_unclosed" || issue.Code == "template_fence_unclosed" {
			return "", &domain.CommandError{Code: "template_invalid", Message: issue.Message, Hint: "先运行 pinax template validate <name> 修复模板"}
		}
	}
	ctx, err := templateContext(req)
	if err != nil {
		return "", err
	}
	if missing := missingTemplateVariables(body, ctx); len(missing) > 0 {
		return "", &domain.CommandError{Code: "template_variable_missing", Message: "缺少模板变量: " + strings.Join(missing, ","), Hint: "使用 --var key=value 提供缺失变量"}
	}
	return templateVariablePattern.ReplaceAllStringFunc(body, func(token string) string {
		match := templateVariablePattern.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		return ctx[match[1]]
	}), nil
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		for _, part := range strings.Split(tag, ",") {
			part = strings.TrimPrefix(strings.TrimSpace(part), "#")
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	return "[" + strings.Join(tags, ", ") + "]"
}

func cleanSyncRequest(req SyncRequest) (string, string, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", "", err
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = "git"
	}
	switch target {
	case "git", "s3", "cloud":
		return root, target, nil
	default:
		return "", "", &domain.CommandError{Code: "invalid_sync_target", Message: "sync target 只支持 git、s3 或 cloud", Hint: "pinax sync diff --target git"}
	}
}

func syncPlanData(target string, profile domain.StorageProfile) map[string]any {
	plan := map[string]any{
		"target":       target,
		"remote_write": false,
		"steps":        []string{"scan_vault", "compare_manifest", "write_receipt"},
	}
	if target == "s3" {
		plan["storage"] = profile
		plan["adapter_status"] = "planned"
	}
	if target == "cloud" {
		plan["backend_required"] = true
		plan["api_handoff"] = []string{"POST /v1/devices", "PUT /v1/vaults/{vault}/manifest", "GET /v1/vaults/{vault}/manifest", "PUT /v1/vaults/{vault}/objects/{path}", "POST /v1/vaults/{vault}/conflicts"}
	}
	return plan
}

func writeSyncState(root, target, direction string) (domain.Projection, error) {
	state := map[string]any{
		"schema_version": "pinax.sync_state.v1",
		"target":         target,
		"direction":      direction,
		"remote_write":   false,
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
		"status":         "planned_only",
	}
	if err := writeJSONAsset(filepath.Join(root, ".pinax", "sync-state.json"), state); err != nil {
		return errorProjection("sync."+direction, err), err
	}
	_ = appendEvent(root, "sync."+direction, "partial", map[string]string{"target": target, "remote_write": "false"})
	projection := domain.NewProjection("sync."+direction, "同步状态已记录，远端写入尚未执行。")
	projection.Status = "partial"
	projection.Facts["target"] = target
	projection.Facts["remote_write"] = "false"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "sync-state.json"))}
	projection.Data = state
	return projection, nil
}

func cleanVaultPath(path string) (string, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func safeJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.Contains(filepath.ToSlash(rel), "../") || strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "路径越过 vault 边界"}
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	clean := filepath.Clean(path)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "路径越过 vault 边界"}
	}
	return clean, nil
}

func ensureEventLog(root string) error {
	path := filepath.Join(root, ".pinax", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func appendEvent(root, eventType, status string, facts map[string]string) error {
	if err := ensureEventLog(root); err != nil {
		return err
	}
	event := map[string]any{
		"schema_version": "pinax.event.v1",
		"type":           eventType,
		"status":         status,
		"ts":             time.Now().UTC().Format(time.RFC3339),
		"facts":          facts,
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(root, ".pinax", "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(b, '\n'))
	return err
}

func errorProjection(command string, err error) domain.Projection {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return domain.NewErrorProjection(command, commandErr)
	}
	return domain.NewErrorProjection(command, &domain.CommandError{Code: "internal_error", Message: err.Error()})
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`)

func shellQuote(value string) string {
	if shellSafe.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// BackendRequest 描述 backend 命令通用请求。
type BackendRequest struct {
	VaultPath string
	Name      string
}

// BackendAddRequest 描述 backend add 请求。
type BackendAddRequest struct {
	VaultPath string
	Name      string
	Kind      string
	Root      string
	Bucket    string
	Region    string
	Prefix    string
	Endpoint  string
	Profile   string
	Remote    string
}

// BackendPlanRequest 描述 backend diff/push/pull 请求。
type BackendPlanRequest struct {
	VaultPath string
	Name      string
	Direction string // push, pull
	DryRun    bool
	Yes       bool
}

// ListBackends 列出 vault 所有 backend profile。
func (s *Service) ListBackends(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.list", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.list", err), err
	}
	projection := domain.NewProjection("backend.list", "Backend 列表已读取。")
	projection.Facts["vault"] = root
	projection.Facts["backends"] = fmt.Sprint(len(registry.Backends))
	if registry.DefaultBackend != "" {
		projection.Facts["default_backend"] = registry.DefaultBackend
	}
	projection.Data = map[string]any{"registry": registry}
	projection.Actions = []domain.Action{{Name: "add", Command: fmt.Sprintf("pinax backend add <kind> --name <name> --vault %s", shellQuote(root))}}
	return projection, nil
}

// AddBackend 添加或更新 backend profile。
func (s *Service) AddBackend(_ context.Context, req BackendAddRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.add", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("backend.add", err), err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend add 需要 --name", Hint: "pinax backend add <kind> --name <name> --vault <vault>"}
		return domain.NewErrorProjection("backend.add", err), err
	}
	kind := domain.BackendKind(strings.TrimSpace(req.Kind))
	if !domain.IsValidBackendKind(string(kind)) {
		err := &domain.CommandError{Code: "backend_kind_invalid", Message: "未知 backend 类型", Hint: "使用 local、s3、rclone 或 onedrive"}
		return domain.NewErrorProjection("backend.add", err), err
	}
	// 按 kind 校验必填字段。
	if err := validateBackendProfileFields(kind, req); err != nil {
		return errorProjection("backend.add", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.add", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	profile := domain.BackendProfile{
		Name: name, Kind: kind, Root: strings.TrimSpace(req.Root),
		Bucket: strings.TrimSpace(req.Bucket), Region: strings.TrimSpace(req.Region),
		Prefix: strings.TrimSpace(req.Prefix), Endpoint: strings.TrimSpace(req.Endpoint),
		Profile: strings.TrimSpace(req.Profile), Remote: strings.TrimSpace(req.Remote),
		CredentialSource: backendCredentialSource(kind, req),
		Capabilities:     backendCapabilities(kind),
		CreatedAt:        now, UpdatedAt: now,
	}
	// 如果已存在同名 profile 则更新。
	for i, existing := range registry.Backends {
		if existing.Name == name {
			profile.CreatedAt = existing.CreatedAt
			registry.Backends[i] = profile
			return saveBackendRegistryProjection(root, registry, profile, "backend.add", "Backend 已更新。")
		}
	}
	registry.Backends = append(registry.Backends, profile)
	if registry.DefaultBackend == "" {
		registry.DefaultBackend = name
	}
	return saveBackendRegistryProjection(root, registry, profile, "backend.add", "Backend 已添加。")
}

// BackendStatus 查看单个 backend 状态。
func (s *Service) BackendStatus(_ context.Context, req BackendRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.status", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.status", err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend.status", err), err
	}
	projection := domain.NewProjection("backend.status", "Backend 状态已读取。")
	projection.Facts["name"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["capabilities"] = strings.Join(profile.Capabilities, ",")
	projection.Data = map[string]any{"profile": profile}
	return projection, nil
}

// BackendDoctor 诊断 backend 配置。
func (s *Service) BackendDoctor(_ context.Context, req BackendRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.doctor", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.doctor", err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend.doctor", err), err
	}
	issues := make([]domain.Issue, 0)
	// 按 kind 校验必填字段。
	switch profile.Kind {
	case domain.BackendS3:
		if profile.Bucket == "" {
			issues = append(issues, domain.Issue{Code: "missing_bucket", Path: ".pinax/backends.json", Message: "S3 backend 缺少 bucket"})
		}
		if profile.Region == "" {
			issues = append(issues, domain.Issue{Code: "missing_region", Path: ".pinax/backends.json", Message: "S3 backend 缺少 region"})
		}
	case domain.BackendRclone, domain.BackendOneDrive:
		if profile.Remote == "" {
			issues = append(issues, domain.Issue{Code: "missing_remote", Path: ".pinax/backends.json", Message: string(profile.Kind) + " backend 缺少 remote"})
		}
	}
	projection := domain.NewProjection("backend.doctor", "Backend 诊断完成。")
	projection.Facts["name"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["network_checked"] = "false"
	projection.Data = map[string]any{"profile": profile, "issues": issues}
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	return projection, nil
}

// BackendCapabilities 查看 backend 能力列表。
func (s *Service) BackendCapabilities(_ context.Context, req BackendRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.capabilities", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.capabilities", err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend.capabilities", err), err
	}
	capabilities := make([]domain.BackendCapability, 0, len(profile.Capabilities))
	for _, cap := range profile.Capabilities {
		capabilities = append(capabilities, domain.BackendCapability{Name: cap, Supported: true})
	}
	projection := domain.NewProjection("backend.capabilities", "Backend 能力已列出。")
	projection.Facts["name"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["capabilities"] = fmt.Sprint(len(capabilities))
	projection.Data = map[string]any{"profile": profile, "capabilities": capabilities}
	return projection, nil
}

// BackendDiff 生成 dry-run 同步计划。
func (s *Service) BackendDiff(_ context.Context, req BackendPlanRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.diff", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.diff", err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend.diff", err), err
	}
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = "push"
	}
	// MVP: diff 生成空计划，只记录 backend 和方向。
	plan := domain.BackendPlan{
		SchemaVersion: "pinax.backend_plan.v1",
		PlanID:        backendPlanID(root, profile.Name, direction),
		BackendName:   profile.Name,
		Direction:     direction,
		Items:         []domain.BackendDiffItem{},
		ConflictCount: 0,
		TotalCount:    0,
		DryRun:        true,
		Status:        "planned",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	projection := domain.NewProjection("backend.diff", "Backend 差异计划已生成。")
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["direction"] = direction
	projection.Facts["items"] = "0"
	projection.Facts["conflicts"] = "0"
	projection.Facts["dry_run"] = "true"
	projection.Data = map[string]any{"plan": plan, "profile": profile}
	projection.Actions = []domain.Action{
		{Name: "push", Command: fmt.Sprintf("pinax backend push --name %s --vault %s --dry-run", shellQuote(profile.Name), shellQuote(root))},
		{Name: "pull", Command: fmt.Sprintf("pinax backend pull --name %s --vault %s --dry-run", shellQuote(profile.Name), shellQuote(root))},
	}
	return projection, nil
}

// BackendPush 执行 push 同步计划。
func (s *Service) BackendPush(_ context.Context, req BackendPlanRequest) (domain.Projection, error) {
	return s.backendSync(req, "push")
}

// BackendPull 执行 pull 同步计划。
func (s *Service) BackendPull(_ context.Context, req BackendPlanRequest) (domain.Projection, error) {
	return s.backendSync(req, "pull")
}

func (s *Service) backendSync(req BackendPlanRequest, direction string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend."+direction, err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend."+direction, err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend."+direction, err), err
	}
	if req.DryRun {
		// dry-run 只读，不执行写入。
		projection := domain.NewProjection("backend."+direction, "Backend "+direction+" dry-run 已生成。")
		projection.Facts["backend"] = profile.Name
		projection.Facts["kind"] = string(profile.Kind)
		projection.Facts["direction"] = direction
		projection.Facts["dry_run"] = "true"
		projection.Data = map[string]any{"backend": profile.Name, "direction": direction, "dry_run": true}
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "backend " + direction + " 需要 --yes", Hint: fmt.Sprintf("先使用 --dry-run 查看计划，确认后追加 --yes")}
		return domain.NewErrorProjection("backend."+direction, err), err
	}
	// MVP: 真实 push/pull 需要后端 adapter 实现，当前只记录事件。
	_ = appendEvent(root, "backend."+direction, "success", map[string]string{"backend": profile.Name, "kind": string(profile.Kind), "direction": direction})
	projection := domain.NewProjection("backend."+direction, fmt.Sprintf("Backend %s 已记录。", direction))
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["direction"] = direction
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Data = map[string]any{"backend": profile.Name, "direction": direction}
	return projection, nil
}

// RemoveBackend 移除 backend profile。
func (s *Service) RemoveBackend(_ context.Context, req BackendRequest) (domain.Projection, error) {
	if strings.TrimSpace(req.Name) == "" {
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend remove 需要 --name", Hint: "pinax backend remove --name <name> --vault <vault> --yes"}
		return errorProjection("backend.remove", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.remove", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.remove", err), err
	}
	removed := false
	filtered := make([]domain.BackendProfile, 0, len(registry.Backends))
	for _, profile := range registry.Backends {
		if profile.Name == req.Name {
			removed = true
			continue
		}
		filtered = append(filtered, profile)
	}
	if !removed {
		err := &domain.CommandError{Code: "backend_not_found", Message: "未找到 backend", Hint: "运行 pinax backend list 查看可用 backend"}
		return errorProjection("backend.remove", err), err
	}
	registry.Backends = filtered
	if registry.DefaultBackend == req.Name {
		registry.DefaultBackend = ""
		if len(filtered) > 0 {
			registry.DefaultBackend = filtered[0].Name
		}
	}
	if err := saveBackendRegistry(root, registry); err != nil {
		return errorProjection("backend.remove", err), err
	}
	_ = appendEvent(root, "backend.remove", "success", map[string]string{"name": req.Name})
	projection := domain.NewProjection("backend.remove", "Backend 已移除。")
	projection.Facts["name"] = req.Name
	projection.Facts["backends"] = fmt.Sprint(len(registry.Backends))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "backends.json"))}
	return projection, nil
}

// validateBackendProfileFields 按 kind 校验必填字段。
func validateBackendProfileFields(kind domain.BackendKind, req BackendAddRequest) error {
	switch kind {
	case domain.BackendS3:
		if strings.TrimSpace(req.Bucket) == "" || strings.TrimSpace(req.Region) == "" {
			return &domain.CommandError{Code: "backend_config_incomplete", Message: "S3 backend 需要 --bucket 和 --region", Hint: "pinax backend add s3 --name <name> --bucket <bucket> --region <region>"}
		}
	case domain.BackendRclone, domain.BackendOneDrive:
		if strings.TrimSpace(req.Remote) == "" {
			return &domain.CommandError{Code: "backend_config_incomplete", Message: string(kind) + " backend 需要 --remote", Hint: fmt.Sprintf("pinax backend add %s --name <name> --remote <remote>", kind)}
		}
	}
	return nil
}

// backendCredentialSource 返回凭据来源描述（不包含真实凭据）。
func backendCredentialSource(kind domain.BackendKind, req BackendAddRequest) string {
	switch kind {
	case domain.BackendS3:
		source := "aws_profile"
		if strings.TrimSpace(req.Profile) != "" {
			source = "aws_profile:" + strings.TrimSpace(req.Profile)
		}
		return source
	case domain.BackendRclone, domain.BackendOneDrive:
		return "rclone_config"
	default:
		return "none"
	}
}

// backendCapabilities 按 kind 返回 MVP 能力列表。
func backendCapabilities(kind domain.BackendKind) []string {
	switch kind {
	case domain.BackendLocal:
		return []string{"list", "status", "doctor"}
	case domain.BackendS3:
		return []string{"list", "status", "doctor", "diff", "push", "pull", "dry_run"}
	case domain.BackendRclone, domain.BackendOneDrive:
		return []string{"list", "status", "doctor", "diff", "push", "pull", "delete", "dry_run"}
	default:
		return []string{"list", "status"}
	}
}

// backendPlanID 生成确定性 plan id。
func backendPlanID(root, name, direction string) string {
	h := sha1.Sum([]byte(root + "\x00" + name + "\x00" + direction + "\x00" + time.Now().UTC().Format(time.RFC3339Nano)))
	return "bp-" + hex.EncodeToString(h[:])[:12]
}

func findBackendProfile(registry domain.BackendRegistry, name string) (domain.BackendProfile, error) {
	name = strings.TrimSpace(name)
	for _, profile := range registry.Backends {
		if profile.Name == name {
			return profile, nil
		}
	}
	return domain.BackendProfile{}, &domain.CommandError{Code: "backend_not_found", Message: "未找到 backend", Hint: "运行 pinax backend list 查看可用 backend"}
}

func loadBackendRegistry(root string) (domain.BackendRegistry, error) {
	registry := domain.BackendRegistry{SchemaVersion: "pinax.backends.v1", Backends: []domain.BackendProfile{}}
	path := filepath.Join(root, ".pinax", "backends.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// 回退：尝试从 legacy storage.json 投影。
		return legacyStorageProjection(root, registry)
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = "pinax.backends.v1"
	}
	if registry.Backends == nil {
		registry.Backends = []domain.BackendProfile{}
	}
	return registry, nil
}

// legacyStorageProjection 从 .pinax/storage.json 投影为 backend registry。
// 只有当 storage.json 真实存在时才投影，避免把默认 local profile 误当作 legacy。
func legacyStorageProjection(root string, registry domain.BackendRegistry) (domain.BackendRegistry, error) {
	storagePath := filepath.Join(root, ".pinax", "storage.json")
	if _, err := os.Stat(storagePath); errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	profile, err := loadStorageProfile(root)
	if err != nil {
		return registry, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch profile.Backend {
	case "local":
		backend := domain.BackendProfile{Name: "local", Kind: domain.BackendLocal, Root: root, CredentialSource: "none", Capabilities: backendCapabilities(domain.BackendLocal), CreatedAt: now, UpdatedAt: now}
		registry.Backends = append(registry.Backends, backend)
		registry.DefaultBackend = "local"
	case "s3":
		backend := domain.BackendProfile{Name: "default-s3", Kind: domain.BackendS3, Bucket: profile.S3.Bucket, Region: profile.S3.Region, Prefix: profile.S3.Prefix, Endpoint: profile.S3.Endpoint, Profile: profile.S3.Profile, CredentialSource: "aws_profile", Capabilities: backendCapabilities(domain.BackendS3), CreatedAt: now, UpdatedAt: now}
		registry.Backends = append(registry.Backends, backend)
		registry.DefaultBackend = "default-s3"
	}
	return registry, nil
}

func saveBackendRegistry(root string, registry domain.BackendRegistry) error {
	registry.SchemaVersion = "pinax.backends.v1"
	if registry.Backends == nil {
		registry.Backends = []domain.BackendProfile{}
	}
	return writeJSONAsset(filepath.Join(root, ".pinax", "backends.json"), registry)
}

func saveBackendRegistryProjection(root string, registry domain.BackendRegistry, profile domain.BackendProfile, command, summary string) (domain.Projection, error) {
	if err := saveBackendRegistry(root, registry); err != nil {
		return errorProjection(command, err), err
	}
	_ = appendEvent(root, command, "success", map[string]string{"backend": profile.Name, "kind": string(profile.Kind)})
	projection := domain.NewProjection(command, summary)
	projection.Facts["name"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["backends"] = fmt.Sprint(len(registry.Backends))
	projection.Facts["credential_source"] = profile.CredentialSource
	projection.Data = map[string]any{"profile": profile}
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax backend status --name %s --vault %s", shellQuote(profile.Name), shellQuote(root))}}
	return projection, nil
}

// PlanningRequest 描述 plan 命令通用请求。
type PlanningRequest struct {
	VaultPath      string
	Period         string // daily, weekly, monthly
	WithTaskBridge bool
	DryRun         bool
	Yes            bool
	Save           bool
	FromPeriod     string // for plan actions --from
}

// PlanDaily 生成每日计划。
func (s *Service) PlanDaily(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(req, domain.PlanningDaily)
}

// PlanWeekly 生成每周计划。
func (s *Service) PlanWeekly(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(req, domain.PlanningWeekly)
}

// PlanMonthly 生成每月计划。
func (s *Service) PlanMonthly(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(req, domain.PlanningMonthly)
}

func (s *Service) planPeriod(req PlanningRequest, period domain.PlanningPeriod) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	// 生成 planning snapshot。
	now := time.Now().UTC()
	snapshot := domain.PlanningSnapshot{
		SchemaVersion: "pinax.planning.snapshot.v1",
		SnapshotID:    planningSnapshotID(root, string(period), now),
		Source:        "local",
		CapturedAt:    now.Format(time.RFC3339),
		Facts:         map[string]string{},
		Risks:         []domain.PlanningRisk{},
	}
	// 读取 vault planning context。
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	snapshot.Facts["notes"] = fmt.Sprint(len(facts))
	snapshot.Facts["source"] = "local"
	// MVP: 基于笔记数量生成简单容量建议。
	maxCommitments := 3
	if period == domain.PlanningWeekly {
		maxCommitments = 7
	} else if period == domain.PlanningMonthly {
		maxCommitments = 15
	}
	snapshot.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
	// 生成 decision。
	decision := domain.PlanningDecision{
		SchemaVersion: "pinax.planning.decision.v1",
		DecisionID:    planningDecisionID(root, string(period), now),
		Period:        period,
		Selected:      []string{},
		Deferred:      []string{},
		Reasons:       []domain.PlanningReason{},
		NextActions:   []domain.Action{},
		CreatedAt:     now.Format(time.RFC3339),
	}
	// 容量检查：如果笔记过多则添加风险。
	if len(facts) > maxCommitments*5 {
		snapshot.Risks = append(snapshot.Risks, domain.PlanningRisk{
			Code: "OVER_CAPACITY", Message: "vault 笔记数量可能超出计划容量",
			Evidence: []string{fmt.Sprintf("notes=%d max_commitments=%d", len(facts), maxCommitments)},
		})
		decision.Reasons = append(decision.Reasons, domain.PlanningReason{
			Kind: "capacity", Summary: fmt.Sprintf("vault 有 %d 篇笔记，建议优先处理 %d 项", len(facts), maxCommitments),
		})
	}
	command := "plan." + string(period)
	if req.DryRun || !req.Yes {
		projection := domain.NewProjection(command, string(period)+" 计划已预览。")
		projection.Facts["period"] = string(period)
		projection.Facts["dry_run"] = "true"
		projection.Facts["snapshot_id"] = snapshot.SnapshotID
		projection.Facts["decision_id"] = decision.DecisionID
		projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
		projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
		projection.Data = map[string]any{"snapshot": snapshot, "decision": decision}
		projection.Actions = []domain.Action{
			{Name: "apply", Command: fmt.Sprintf("pinax plan %s --vault %s --yes", string(period), shellQuote(root))},
		}
		return projection, nil
	}
	// 写入 snapshot。
	if req.Save {
		snapRel, err := savePlanningSnapshot(root, &snapshot)
		if err != nil {
			return errorProjection(command, err), err
		}
		snapshot.SavedPath = snapRel
	}
	_ = appendEvent(root, command, "success", map[string]string{"period": string(period), "snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection(command, string(period)+" 计划已生成。")
	projection.Facts["period"] = string(period)
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["decision_id"] = decision.DecisionID
	projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
	projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
	if snapshot.SavedPath != "" {
		projection.Facts["saved_path"] = snapshot.SavedPath
		projection.Evidence = []string{snapshot.SavedPath}
	}
	projection.Data = map[string]any{"snapshot": snapshot, "decision": decision}
	projection.Actions = []domain.Action{
		{Name: "open", Command: fmt.Sprintf("pinax %s open --vault %s", string(period), shellQuote(root))},
	}
	return projection, nil
}

// PlanActions 生成 TaskBridge action file 草稿。
func (s *Service) PlanActions(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan.actions", err), err
	}
	now := time.Now().UTC()
	period := strings.TrimSpace(req.FromPeriod)
	if period == "" {
		period = "daily"
	}
	planningPeriod, err := parsePlanningPeriod(period)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	preview, err := s.planPeriod(PlanningRequest{VaultPath: root, Period: period, DryRun: true}, planningPeriod)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	snapshot, decision, err := planningPreviewData(preview)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	draft := buildPlanningActionDraft(period, snapshot, decision, now)
	if req.DryRun || !req.Save {
		projection := domain.NewProjection("plan.actions", "Action 草稿已预览。")
		projection.Facts["period"] = period
		projection.Facts["dry_run"] = "true"
		projection.Facts["action_id"] = draft.ActionID
		projection.Facts["source_decision"] = draft.SourceDecision
		projection.Facts["snapshot_id"] = draft.SourceSnapshot
		projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
		projection.Data = map[string]any{"draft": draft}
		projection.Actions = []domain.Action{
			{Name: "save", Command: fmt.Sprintf("pinax plan actions --from %s --vault %s --save", period, shellQuote(root))},
		}
		return projection, nil
	}
	// 保存 action draft。
	rel, err := savePlanningActionDraft(root, &draft)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	draft.SavedPath = rel
	_ = appendEvent(root, "plan.actions", "success", map[string]string{"action_id": draft.ActionID, "saved_path": rel})
	projection := domain.NewProjection("plan.actions", "Action 草稿已保存。")
	projection.Facts["action_id"] = draft.ActionID
	projection.Facts["source_decision"] = draft.SourceDecision
	projection.Facts["snapshot_id"] = draft.SourceSnapshot
	projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
	projection.Facts["saved_path"] = rel
	projection.Evidence = []string{rel}
	projection.Data = map[string]any{"draft": draft}
	projection.Actions = []domain.Action{
		{Name: "execute", Command: fmt.Sprintf("taskbridge agent execute --action-file %s --dry-run", rel)},
	}
	return projection, nil
}

func parsePlanningPeriod(value string) (domain.PlanningPeriod, error) {
	switch domain.PlanningPeriod(strings.TrimSpace(value)) {
	case domain.PlanningDaily:
		return domain.PlanningDaily, nil
	case domain.PlanningWeekly:
		return domain.PlanningWeekly, nil
	case domain.PlanningMonthly:
		return domain.PlanningMonthly, nil
	default:
		return "", &domain.CommandError{Code: "invalid_planning_period", Message: "不支持的计划期间", Hint: "使用 daily、weekly 或 monthly"}
	}
}

func planningPreviewData(projection domain.Projection) (domain.PlanningSnapshot, domain.PlanningDecision, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "计划预览数据缺失"}
	}
	snapshot, ok := data["snapshot"].(domain.PlanningSnapshot)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "计划快照数据缺失"}
	}
	decision, ok := data["decision"].(domain.PlanningDecision)
	if !ok {
		return domain.PlanningSnapshot{}, domain.PlanningDecision{}, &domain.CommandError{Code: "planning_preview_invalid", Message: "计划决策数据缺失"}
	}
	return snapshot, decision, nil
}

func buildPlanningActionDraft(period string, snapshot domain.PlanningSnapshot, decision domain.PlanningDecision, now time.Time) domain.PlanningActionDraft {
	draftID := planningActionIDFromRefs(period, snapshot.SnapshotID, decision.DecisionID, now)
	draft := domain.PlanningActionDraft{
		SchemaVersion:        "taskbridge.actions.v1",
		ActionID:             draftID,
		SourcePeriod:         period,
		SourceDecision:       decision.DecisionID,
		SourceSnapshot:       snapshot.SnapshotID,
		RequiresConfirmation: false,
		Tasks:                []domain.ActionDraftTask{},
		EvidenceRefs:         []string{"snapshot:" + snapshot.SnapshotID, "decision:" + decision.DecisionID},
		CreatedAt:            now.Format(time.RFC3339),
	}
	reason := planningActionReason(decision)
	for i, taskID := range decision.Deferred {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		// Action draft 只表达待确认的 TaskBridge 写入意图；真实远端写入仍交给 taskbridge agent execute。
		draft.Tasks = append(draft.Tasks, domain.ActionDraftTask{
			ActionID:             planningTaskActionID(draftID, taskID, i),
			TaskID:               taskID,
			Kind:                 "defer",
			Reason:               reason,
			RequiresConfirmation: true,
		})
	}
	draft.RequiresConfirmation = len(draft.Tasks) > 0
	return draft
}

func planningActionReason(decision domain.PlanningDecision) string {
	for _, reason := range decision.Reasons {
		if strings.TrimSpace(reason.Summary) != "" {
			return reason.Summary
		}
	}
	return "计划建议由 TaskBridge 确认后再执行任务写入。"
}

func planningActionIDFromRefs(period, snapshotID, decisionID string, t time.Time) string {
	h := sha1.Sum([]byte(period + "\x00" + snapshotID + "\x00" + decisionID + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_act_" + hex.EncodeToString(h[:])[:16]
}

func planningTaskActionID(draftID, taskID string, index int) string {
	h := sha1.Sum([]byte(draftID + "\x00" + taskID + "\x00" + fmt.Sprint(index)))
	return "act_" + hex.EncodeToString(h[:])[:16]
}

// PlanSnapshot 生成计划快照。
func (s *Service) PlanSnapshot(_ context.Context, req PlanningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	now := time.Now().UTC()
	snapshot := domain.PlanningSnapshot{
		SchemaVersion: "pinax.planning.snapshot.v1",
		SnapshotID:    planningSnapshotID(root, "manual", now),
		Source:        "local",
		CapturedAt:    now.Format(time.RFC3339),
		Facts:         map[string]string{},
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	snapshot.Facts["notes"] = fmt.Sprint(len(facts))
	snapRel, err := savePlanningSnapshot(root, &snapshot)
	if err != nil {
		return errorProjection("plan.snapshot", err), err
	}
	snapshot.SavedPath = snapRel
	_ = appendEvent(root, "plan.snapshot", "success", map[string]string{"snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection("plan.snapshot", "计划快照已保存。")
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["saved_path"] = snapRel
	projection.Data = map[string]any{"snapshot": snapshot}
	projection.Actions = []domain.Action{
		{Name: "plan", Command: fmt.Sprintf("pinax plan daily --vault %s", shellQuote(root))},
	}
	return projection, nil
}

func planningSnapshotID(root, period string, t time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + period + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_snap_" + hex.EncodeToString(h[:])[:16]
}

func planningDecisionID(root, period string, t time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + period + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_dec_" + hex.EncodeToString(h[:])[:16]
}

func planningActionID(root, period string, t time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + period + "\x00" + t.Format(time.RFC3339Nano)))
	return "plan_act_" + hex.EncodeToString(h[:])[:16]
}

func savePlanningSnapshot(root string, snapshot *domain.PlanningSnapshot) (string, error) {
	dir, err := safeJoin(root, ".pinax/planning/snapshots")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "snapshots", snapshot.SnapshotID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	snapshot.SavedPath = rel
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	return rel, os.WriteFile(path, payload, 0o644)
}

func savePlanningActionDraft(root string, draft *domain.PlanningActionDraft) (string, error) {
	dir, err := safeJoin(root, ".pinax/planning/actions")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "planning", "actions", draft.ActionID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	draft.SavedPath = rel
	payload, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	return rel, os.WriteFile(path, payload, 0o644)
}
