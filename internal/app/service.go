package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/app/briefingops"
	"github.com/yeisme/pinax/internal/app/noteops"
	"github.com/yeisme/pinax/internal/app/planningops"
	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/app/templateops"
	"github.com/yeisme/pinax/internal/app/vaultops"
	"github.com/yeisme/pinax/internal/app/versionops"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/briefing"
	"github.com/yeisme/pinax/internal/cloudclient"
	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/delivery"
	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/markdownnote"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	"github.com/yeisme/pinax/internal/promptasset"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	notesearch "github.com/yeisme/pinax/internal/search"
	syncplan "github.com/yeisme/pinax/internal/sync"
	"github.com/yeisme/pinax/internal/templateengine"
	"github.com/yeisme/pinax/internal/vaultignore"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

type Service struct {
	versionBackend pinaxversion.VersionBackend
}

type InitVaultRequest struct {
	VaultPath string
	Title     string
}

type VaultRequest struct {
	VaultPath string
	Query     string
}

type VaultIgnoreRequest struct {
	VaultPath string
	Yes       bool
}

type IndexRefreshRequest struct {
	VaultPath    string
	ChangedSince string
}

type IndexLookupRequest struct {
	VaultPath string
	Query     string
	Scope     string
	Kind      string
}

type VaultObjectCandidate = domain.VaultObjectCandidate

type AssetRequest struct {
	VaultPath       string
	Source          string
	Ref             string
	Target          string
	PathStyle       string
	ContextNote     string
	IncludePaths    bool
	PreviewAs       string
	MaxPreviewBytes int
}

type PromptAssetRequest struct {
	VaultPath string
	From      string
	Ref       string
	Query     string
	Domain    string
	Tag       string
	Lifecycle string
	To        string
	Reason    string
	Limit     int
}

type IndexRepairRequest struct {
	VaultPath string
	Kind      string
	DryRun    bool
	Yes       bool
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
	Template  string
	Next      bool
}

type InboxTriageRequest struct {
	VaultPath    string
	NoteRef      string
	PathStyle    string
	IncludePaths bool
	Group        string
	Folder       string
	Kind         string
	Status       string
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
	Language      string
	Query         string
	Columns       []string
	Display       string
	GroupBy       string
	CalendarField string
	BoardColumn   string
}

type DatabaseSchemaRequest struct {
	VaultPath string
	Name      string
	Type      string
	Values    []string
}

type QueryRequest struct {
	VaultPath string
	SQL       string
	LazyIndex bool
	Limit     int
	Sort      string
	Cursor    string
}

type DataviewRequest struct {
	VaultPath string
	Query     string
	LazyIndex bool
	Limit     int
	Sort      string
	Cursor    string
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
	Engine        string
	LazyIndex     string
	At            string
	IncludeDirty  bool
	ChangedSince  string
	Revision      string
}

func toSearchOpsRequest(req SearchRequest) searchops.Request {
	return searchops.Request{VaultPath: req.VaultPath, Query: req.Query, Tags: req.Tags, Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, LinkTarget: req.LinkTarget, HasAttachment: req.HasAttachment, Limit: req.Limit, Sort: req.Sort, AllowStale: req.AllowStale, Engine: req.Engine, LazyIndex: req.LazyIndex, At: req.At, IncludeDirty: req.IncludeDirty, ChangedSince: req.ChangedSince, Revision: req.Revision}
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
	Engine     string
	SaveRun    string
	Run        string
	Runs       bool
	Pack       string
	UseCase    string
	Intent     string
}

type IndexPageRequest struct {
	VaultPath string
	Name      string
	Template  string
}

type ShowNoteRequest struct {
	VaultPath        string
	NoteRef          string
	View             string
	Display          string
	Snapshot         string
	Runs             bool
	EmbedAttachments string
	MaxEmbedDepth    int
	MaxEmbedBytes    int
	MaxPreviewBytes  int
}

type NoteRefreshRequest struct {
	VaultPath string
	NoteRef   string
	Rendered  bool
	Yes       bool
	SaveRun   string
	Snapshot  string
}

type NoteLinkRequest struct {
	VaultPath    string
	NoteRef      string
	PathStyle    string
	IncludePaths bool
}

type NoteAttachRequest struct {
	VaultPath  string
	NoteRef    string
	SourcePath string
	Placement  string
	LinkStyle  string
	Embed      bool
	Mode       string
	Rename     string
	Yes        bool
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
	VaultPath        string
	Tags             []string
	Project          string
	Group            string
	Folder           string
	Kind             string
	Status           string
	CreatedAfter     string
	UpdatedAfter     string
	UpdatedBefore    string
	Period           string
	Recent           bool
	Limit            int
	Sort             string
	PathPrefix       string
	Properties       []string
	StrictProperties bool
}

type NoteMutationRequest struct {
	VaultPath string
	NoteRef   string
	Title     string
	TargetDir string
	Yes       bool
	DryRun    bool
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

type NotePropertyRequest struct {
	VaultPath string
	NoteRef   string
	Operation string
	Key       string
	Value     string
}

type NoteTagBulkRequest struct {
	VaultPath string
	Operation string
	OldTag    string
	NewTag    string
	DryRun    bool
	Yes       bool
}

type NoteFolderBulkRequest struct {
	VaultPath string
	Operation string
	OldFolder string
	NewFolder string
	DryRun    bool
	Yes       bool
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
	VaultPath      string
	Target         string
	Yes            bool
	DryRun         bool
	BaseRevision   string
	RemoteRevision string
	Endpoint       string
	WorkspaceID    string
	DeviceID       string
	SecretRef      string
	PathPolicy     string
}

type CloudLoginRequest struct {
	VaultPath           string
	Endpoint            string
	WorkspaceID         string
	DeviceID            string
	SecretRef           string
	EncryptionSecretRef string
}

type CloudBackendSetRequest struct {
	VaultPath   string
	Kind        string
	Bucket      string
	Region      string
	Prefix      string
	Endpoint    string
	Profile     string
	Remote      string
	WorkspaceID string
	DeviceID    string
	SecretRef   string
}

type CloudRequest struct {
	VaultPath string
}

type BriefingRecipeRequest struct {
	VaultPath string
	Topic     string
	Limit     int
	Source    string
}

type BriefingRunRequest struct {
	VaultPath string
	DryRun    bool
	Yes       bool
}

type FeishuDeliveryRequest struct {
	VaultPath  string
	WebhookURL string
	SecretRef  string
	Title      string
	Text       string
	DryRun     bool
	Yes        bool
}

func NewService() *Service { return NewServiceWithVersionBackend(pinaxversion.NewLocalBackend()) }

func NewServiceWithVersionBackend(backend pinaxversion.VersionBackend) *Service {
	if backend == nil {
		backend = pinaxversion.NewLocalBackend()
	}
	return &Service{versionBackend: backend}
}

func currentTimeUTC() time.Time {
	value := strings.TrimSpace(os.Getenv("PINAX_TEST_NOW"))
	if value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

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
	_ = appendEvent(root, "vault.init", "success", map[string]string{"title": req.Title})

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
	_ = appendEvent(root, "vault.ignore.apply", "success", map[string]string{"operations": fmt.Sprint(len(ops))})
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
	project := domain.Project{Slug: req.Slug, Name: req.Name, Description: req.Description, NotesPrefix: filepath.ToSlash(req.NotesPrefix), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	for i, existing := range registry.Projects {
		if existing.Slug != req.Slug {
			continue
		}
		if existing.Name != project.Name || existing.Description != project.Description || existing.NotesPrefix != project.NotesPrefix {
			err := &domain.CommandError{Code: "project_conflict", Message: "Project slug already exists with a different definition", Hint: "Choose another slug, or inspect pinax project list first"}
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
	_ = appendEvent(root, "project.switch", "success", map[string]string{"project": req.Slug})
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
	_ = appendEvent(root, "storage.set_local", "success", map[string]string{"backend": "local"})
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
	_ = appendEvent(root, "storage.set_s3", "success", map[string]string{"backend": "s3", "bucket": req.Bucket, "region": req.Region})
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
	facts = ordinaryNoteFacts(facts)
	stats := vaultops.Stats(root, toVaultOpsFacts(facts), elapsed)
	issues := VaultHealthService{}.Issues(root, facts, stats, 90*24*time.Hour)
	issues = append(issues, projectTrashLifecycleIssues(root)...)
	issues = append(issues, assetAndVersionRepairIssues(root, issues)...)
	plan := buildRepairPlan(root, facts, stats, issues, elapsed)
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
	if err := ensureRepairPlanFresh(root, plan); err != nil {
		projection := errorProjection("repair.apply", err)
		projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax repair plan --vault %s --save", shellQuote(root))}}
		projection.Data = map[string]any{"plan_id": plan.PlanID}
		return projection, err
	}
	requiresSnapshot := repairPlanRequiresSnapshot(plan)
	if req.SnapshotMessage != "" && requiresSnapshot {
		if _, err := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: req.SnapshotMessage}); err != nil {
			return errorProjection("repair.apply", err), err
		}
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
	projection := domain.NewProjection("repair.apply", "Repair plan applied.")
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

func ensureRepairPlanFresh(root string, plan domain.RepairPlan) error {
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "repair_plan_not_planned", Message: "repair plan status is not applicable", Hint: "Rerun pinax repair plan --save"}
	}
	if plan.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
		if err == nil && time.Now().UTC().After(expires) {
			return &domain.CommandError{Code: "plan_stale", Message: "repair plan has expired", Hint: "pinax repair plan --vault <vault> --save"}
		}
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

func uniqueAttachmentRelWithPlacement(root string, note domain.Note, filename string, placement pinaxassets.AttachmentPlacementPolicy) (string, error) {
	return pinaxassets.PlaceAttachment(pinaxassets.AttachmentPlacementRequest{Root: root, NoteID: note.ID, NotePath: note.Path, Filename: filename, Policy: placement})
}

func registeredAttachmentRel(root, source string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absSource)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, ".pinax/") {
		return "", &domain.CommandError{Code: "asset_outside_vault", Message: "register mode only accepts files inside the vault", Hint: "Use a file inside the vault, or switch to --mode copy"}
	}
	if _, err := safeJoin(root, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func normalizedAttachmentMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", "copy":
		return "copy"
	case "move":
		return "move"
	case "register":
		return "register"
	default:
		return ""
	}
}

func normalizedAttachmentPlacement(placement string) pinaxassets.AttachmentPlacementPolicy {
	switch strings.TrimSpace(placement) {
	case "":
		return pinaxassets.AttachmentPlacementPerNote
	default:
		return pinaxassets.AttachmentPlacementPolicy(strings.TrimSpace(placement))
	}
}

func copyFile(source, target string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o644)
}

func attachmentReference(notePath, attachmentRel, style string, embed bool) (string, string, error) {
	style = strings.TrimSpace(style)
	if style == "" || style == "auto" {
		style = "markdown"
	}
	switch style {
	case "markdown":
		return style, markdownAttachmentReferenceWithEmbed(notePath, attachmentRel, embed), nil
	case "wiki":
		return style, wikiAttachmentReference(attachmentRel, embed), nil
	default:
		return "", "", &domain.CommandError{Code: "attachment_link_style_invalid", Message: "Attachment link style is invalid", Hint: "Use --link-style markdown, wiki, or auto"}
	}
}

func markdownAttachmentReferenceWithEmbed(notePath, attachmentRel string, embed bool) string {
	rel, err := filepath.Rel(filepath.Dir(filepath.FromSlash(notePath)), filepath.FromSlash(attachmentRel))
	if err != nil {
		rel = filepath.FromSlash(attachmentRel)
	}
	rel = filepath.ToSlash(rel)
	label := filepath.Base(attachmentRel)
	if embed || attachmentMediaType(attachmentRel) == "image" {
		return fmt.Sprintf("![%s](%s)", label, rel)
	}
	return fmt.Sprintf("[%s](%s)", label, rel)
}

func wikiAttachmentReference(attachmentRel string, embed bool) string {
	if embed || attachmentMediaType(attachmentRel) == "image" {
		return fmt.Sprintf("![[%s]]", attachmentRel)
	}
	return fmt.Sprintf("[[%s]]", attachmentRel)
}

func noteAttachmentsFromBody(root string, note domain.Note) []domain.NoteAttachment {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	attachments := make([]domain.NoteAttachment, 0, len(links))
	for _, link := range links {
		abs := filepath.Join(root, filepath.FromSlash(link.AssetPath))
		_, statErr := os.Stat(abs)
		attachments = append(attachments, domain.NoteAttachment{NotePath: note.Path, ReferenceText: link.RawReference, Path: link.AssetPath, TargetPath: link.AssetPath, MediaType: attachmentMediaType(link.AssetPath), Exists: statErr == nil})
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].TargetPath < attachments[j].TargetPath })
	return attachments
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
		return nil, &domain.CommandError{Code: "import_source_missing", Message: "Import source does not exist", Hint: "Check the Markdown file or directory path"}
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
				return nil, &domain.CommandError{Code: "invalid_import_conflict", Message: "Unknown import conflict policy", Hint: "Use --conflict skip, rename, or overwrite"}
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
	return "", &domain.CommandError{Code: "import_name_conflict", Message: "Too many import filename conflicts", Hint: "Choose another target group or filename, then retry"}
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
	if periodUpdatedAfter, err := noteListPeriodUpdatedAfter(req.Period, req.UpdatedAfter); err != nil {
		return errorProjection("note.list", err), err
	} else if periodUpdatedAfter != "" {
		req.UpdatedAfter = periodUpdatedAfter
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("note.list", err), err
	}
	facts = ordinaryNoteFacts(facts)
	// 默认排除 discarded 笔记，除非显式请求
	if req.Status != "discarded" {
		kept := make([]noteFact, 0, len(facts))
		for _, f := range facts {
			if f.note.Status != "discarded" {
				kept = append(kept, f)
			}
		}
		facts = kept
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
	projection := domain.NewProjection("note.list", "Local notes listed.")
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
	if req.UpdatedAfter != "" {
		projection.Facts["filter.updated_after"] = req.UpdatedAfter
	}
	if req.UpdatedBefore != "" {
		projection.Facts["filter.updated_before"] = req.UpdatedBefore
	}
	if req.Period != "" {
		projection.Facts["filter.period"] = req.Period
	}
	if req.PathPrefix != "" {
		projection.Facts["filter.path_prefix"] = req.PathPrefix
	}
	properties, err := selectedNoteProperties(notes, req.Properties, req.StrictProperties)
	if err != nil {
		return domain.NewErrorProjection("note.list", err.(*domain.CommandError)), err
	}
	data := map[string]any{"notes": notes, "filters": req, "total": total, "returned": len(notes)}
	if len(req.Properties) > 0 {
		projection.Facts["properties"] = strings.Join(req.Properties, ",")
		data["properties"] = properties
	}
	projection.Data = data
	return projection, nil
}

func noteListPeriodUpdatedAfter(period, explicitUpdatedAfter string) (string, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		return strings.TrimSpace(explicitUpdatedAfter), nil
	}
	if strings.TrimSpace(explicitUpdatedAfter) != "" {
		return "", &domain.CommandError{Code: "note_list_period_conflict", Message: "--period and --updated-after cannot be used together", Hint: "Use either pinax note list --period daily or pinax note list --updated-after <date>"}
	}
	now := currentTimeUTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var boundary time.Time
	switch period {
	case "5h":
		boundary = now.Add(-5 * time.Hour)
	case "daily", "day", "today":
		boundary = dayStart
	case "weekly", "week", "this-week":
		weekdayOffset := (int(now.Weekday()) + 6) % 7
		boundary = dayStart.AddDate(0, 0, -weekdayOffset)
	case "monthly", "month", "this-month":
		boundary = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return "", &domain.CommandError{Code: "note_list_period_invalid", Message: "Invalid note list period: " + period, Hint: "Use one of: 5h, daily, weekly, monthly"}
	}
	return boundary.Format(time.RFC3339), nil
}

func selectedNoteProperties(notes []domain.Note, names []string, strict bool) (map[string]map[string]domain.PropertyValue, error) {
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			cleaned = append(cleaned, name)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	found := map[string]bool{}
	out := map[string]map[string]domain.PropertyValue{}
	for _, note := range notes {
		values := noteindex.ExtractProperties(note)
		selected := map[string]domain.PropertyValue{}
		for _, name := range cleaned {
			if value, ok := values[name]; ok {
				selected[name] = value
				found[name] = true
			}
		}
		out[note.Path] = selected
	}
	if strict {
		missing := make([]string, 0)
		for _, name := range cleaned {
			if !found[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return nil, &domain.CommandError{Code: "property_not_found", Message: "Property not found: " + strings.Join(missing, ","), Hint: "Run pinax database schema infer --vault <vault> to view available properties"}
		}
	}
	return out, nil
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
	projection := domain.NewProjection(dimension+".list", "Organize views listed.")
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
		err := &domain.CommandError{Code: "view_name_required", Message: "view save requires a name", Hint: "pinax view save <name> --vault <vault>"}
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
	projection := domain.NewProjection("view.save", "View saved.")
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
	projection := domain.NewProjection("view.list", "Views listed.")
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
		err := &domain.CommandError{Code: "view_not_found", Message: "Saved view not found", Hint: "pinax view list --vault <vault>"}
		return domain.NewErrorProjection("view.show", err), err
	}
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Tags: view.Tags, Group: view.Group, Folder: view.Folder, Kind: view.Kind, Status: view.Status, CreatedAfter: view.CreatedAfter, UpdatedBefore: view.UpdatedBefore, Sort: view.Sort, Limit: view.Limit})
	projection.Command = "view.show"
	projection.Summary = "View queried."
	projection.Facts["view"] = view.Name
	projection.Data = map[string]any{"view": view, "result": projection.Data}
	return projection, err
}

func (s *Service) DeleteView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "view delete requires --yes", Hint: "Add --yes after confirming"}
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
	projection := domain.NewProjection("view.delete", "View deleted.")
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
	if strings.TrimSpace(registry.SchemaVersion) == "" {
		registry.SchemaVersion = "pinax.views.v1"
	}
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
		err := &domain.CommandError{Code: "title_required", Message: "note new requires a title", Hint: "pinax note new <title> --vault <vault>"}
		return domain.NewErrorProjection("note.new", err), err
	}
	safeTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.new", tagErr), tagErr
	}
	req.Tags = safeTags
	templateName := strings.TrimSpace(req.Template)
	var templateDoc templateengine.TemplateDocument
	var templatePathPattern string
	templateDefaults := map[string]string{}
	templateOverrides := []string{}
	templateMeta := templateengine.Metadata{}
	templateSource := ""
	if templateName != "" {
		doc, err := parseTemplateForProjection(root, templateName)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		if templateDocumentIsDesignDraft(doc) {
			err := &domain.CommandError{Code: "template_design_not_executable", Message: "Template is still a draft and cannot be used for note creation", Hint: "Publish the draft as an executable schema_version: pinax.template.v2 template first"}
			return domain.NewErrorProjection("note.new", err), err
		}
		templateDoc = doc
		templateMeta, templateSource = templateProjectionMetadata(root, templateName, doc.Metadata)
		templatePathPattern = doc.Metadata.Output.PathPattern
		templateDefaults = doc.Metadata.Defaults
		if req.Kind == "" && templateDefaults["kind"] != "" {
			req.Kind = templateDefaults["kind"]
		} else if req.Kind != "" && templateDefaults["kind"] != "" && req.Kind != templateDefaults["kind"] {
			templateOverrides = append(templateOverrides, "kind")
		}
		if req.Status == "" && templateDefaults["status"] != "" {
			req.Status = templateDefaults["status"]
		} else if req.Status != "" && templateDefaults["status"] != "" && req.Status != templateDefaults["status"] {
			templateOverrides = append(templateOverrides, "status")
		}
		if req.Dir != "" || req.Folder != "" || req.Slug != "" || req.Project != "" {
			templateOverrides = append(templateOverrides, "path")
		}
		if len(req.Tags) == 0 && templateDefaults["tags"] != "" {
			req.Tags = splitCommaValues(templateDefaults["tags"])
			var tagErr *domain.CommandError
			req.Tags, tagErr = normalizeTagsForWrite(req.Tags)
			if tagErr != nil {
				return domain.NewErrorProjection("note.new", tagErr), tagErr
			}
		} else if len(req.Tags) > 0 && templateDefaults["tags"] != "" {
			templateOverrides = append(templateOverrides, "tags")
		}
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
	var rel string
	if templatePathPattern != "" && req.Dir == "" && req.Folder == "" && req.Slug == "" && req.Project == "" {
		templateRel, err := renderTemplateOutputPath(templateDoc, req)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		rel, err = nextNotePath(root, templateRel)
		if err != nil {
			return errorProjection("note.new", err), err
		}
	} else {
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
		rel, err = nextNotePath(root, filepath.ToSlash(filepath.Join(prefix, slug+".md")))
		if err != nil {
			return errorProjection("note.new", err), err
		}
	}
	if body == "" {
		body = "# " + req.Title + "\n"
	}
	if strings.TrimSpace(req.Template) != "" && req.Body == "" && req.SourcePath == "" && req.StdinBody == "" {
		rendered, err := s.renderTemplateBody(ctx, root, TemplateRequest{VaultPath: root, Name: req.Template, Title: req.Title, Project: req.Project, Tags: req.Tags, Vars: req.Vars}, true)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		body = rendered
	}
	now := time.Now().UTC().Format(time.RFC3339)
	content := buildNoteContentWithStatus(req.Title, rel, req.Project, folder, kind, cleanTags(req.Tags), req.Status, now, body)
	projection := domain.NewProjection("note.new", "Note created.")
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
	if templateName != "" {
		projection.Facts["template"] = templateName
		templateUseID := stableNoteID(templateName + ":" + rel)
		projection.Facts["template_use_id"] = templateUseID
		projection.Facts["effective_path"] = rel
		if templateMeta.ScenarioID != "" {
			projection.Facts["scenario_id"] = templateMeta.ScenarioID
		}
		if templateMeta.Pack.ID != "" {
			projection.Facts["template_pack"] = templateMeta.Pack.ID
		}
		if templateMeta.ProofGate.Status != "" {
			projection.Facts["proof_gate.status"] = templateMeta.ProofGate.Status
		}
		if templatePathPattern != "" {
			projection.Facts["template.path_pattern"] = templatePathPattern
		}
		if len(templateDefaults) > 0 {
			projection.Facts["template.defaults_source"] = templateName
		}
		if len(templateOverrides) > 0 {
			projection.Facts["template.overrides"] = strings.Join(templateOverrides, ",")
		}
	}
	data := map[string]any{"note": domain.Note{ID: stableNoteID(rel), Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Body: strings.TrimSpace(body), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status, CreatedAt: now, UpdatedAt: now}, "planned_path": rel, "frontmatter_preview": strings.SplitN(content, "---\n\n", 2)[0] + "---", "body_preview": strings.TrimSpace(body)}
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(rel), shellQuote(root))}}
	if templateName != "" {
		nextActions := templateMetadataActions(templateMeta.AfterCreateActions)
		nextActions = append(nextActions, projection.Actions...)
		data["template_use"] = map[string]any{
			"template_use_id":      projection.Facts["template_use_id"],
			"template":             templateName,
			"template_pack":        templateMeta.Pack.ID,
			"scenario_id":          templateMeta.ScenarioID,
			"effective_path":       rel,
			"source":               templateSource,
			"proof_gate":           templateMeta.ProofGate,
			"next_actions":         nextActions,
			"after_create_actions": templateMeta.AfterCreateActions,
		}
		projection.Actions = nextActions
	}
	projection.Data = data
	if req.DryRun {
		projection.Summary = "Note create plan generated."
		projection.Facts["ledger_status"] = "preview"
		projection.Facts["record_event"] = string(domain.RecordEventNoteCreated)
		projection.Facts["version_backend"] = "none"
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
		if code := templateengine.ErrorCode(dailyErr); strings.HasPrefix(code, "managed_block_") {
			projection.Status = "partial"
			projection.Facts["daily_index"] = dailyIndexRel
			projection.Facts["daily_index_status"] = code
			projection.Actions = append(projection.Actions, domain.Action{Name: "upgrade_daily_template", Command: fmt.Sprintf("pinax journal daily show --template journal.daily --vault %s --json", shellQuote(root))})
		} else {
			return errorProjection("note.new", dailyErr), dailyErr
		}
	} else if dailyIndexRel != "" {
		projection.Facts["daily_index"] = dailyIndexRel
		projection.Facts["daily_index_status"] = "updated"
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.new", err), err
	}
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{dailyIndexRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	note := domain.Note{ID: stableNoteID(rel), Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Body: strings.TrimSpace(body), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status, CreatedAt: now, UpdatedAt: now}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteCreated, "note.new:"+note.ID+":"+rel, note, "")
	if recordErr != nil {
		return errorProjection("note.new", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	eventFacts := map[string]string{"path": rel, "title": req.Title}
	if templateName != "" {
		eventFacts["template"] = templateName
		eventFacts["template_use_id"] = projection.Facts["template_use_id"]
		eventFacts["scenario_id"] = templateMeta.ScenarioID
	}
	_ = appendEvent(root, "note.new", "success", eventFacts)
	return projection, nil
}

func (s *Service) DailyOpen(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.open", err), err
	}
	projection, err := s.EditNote(ctx, NoteEditRequest{VaultPath: root, NoteRef: rel, Editor: req.Editor})
	projection.Command = "daily.open"
	projection.Summary = "Daily note opened."
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["period"] = "daily"
	projection.Facts["template"] = journalTemplateName("daily", req)
	return projection, err
}

func (s *Service) DailyShow(ctx context.Context, req DailyRequest) (domain.Projection, error) {
	root, rel, key, err := ensureJournalNote(req.VaultPath, "daily", req)
	if err != nil {
		return errorProjection("daily.show", err), err
	}
	projection, err := s.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: rel})
	projection.Command = "daily.show"
	projection.Summary = "Daily note read."
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["template"] = journalTemplateName("daily", req)
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
		err := &domain.CommandError{Code: "body_required", Message: "daily append requires --body", Hint: "pinax daily append --body <text> --vault <vault>"}
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
	projection := domain.NewProjection("daily.append", "Daily note appended.")
	projection.Facts["path"] = rel
	projection.Facts["date"] = key
	projection.Facts["template"] = journalTemplateName("daily", req)
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
	projection.Summary = journalLabel(period) + " opened."
	projection.Facts["template"] = journalTemplateName(period, req)
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
	projection.Facts["template"] = journalTemplateName(period, req)
	projection.Summary = journalLabel(period) + " read."
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
		err := &domain.CommandError{Code: "body_required", Message: period + " append requires --body", Hint: "pinax " + period + " append --body <text> --vault <vault>"}
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
	projection := domain.NewProjection(period+".append", journalLabel(period)+" appended.")
	projection.Facts["path"] = rel
	projection.Facts["template"] = journalTemplateName(period, req)
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
	projection.Summary = "Inbox note captured."
	return projection, err
}

func (s *Service) InboxList(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath, Status: "inbox", Sort: "updated"})
	projection.Command = "inbox.list"
	projection.Summary = "Inbox notes listed."
	return projection, err
}

func (s *Service) InboxTriage(_ context.Context, req InboxTriageRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := loadMutableNote(req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("inbox.triage", err), err
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		err := &domain.CommandError{Code: "group_required", Message: "inbox triage requires --group", Hint: "pinax inbox triage <note> --group <group> --vault <vault>"}
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
		err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another folder or handle the existing file first"}
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
	projection := noteMutationProjection("inbox.triage", "Inbox note triaged.", targetRel, meta)
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

type resolverNoteAmbiguousError struct {
	*domain.CommandError
	Result ResolverResult
}

func (e *resolverNoteAmbiguousError) Unwrap() error { return e.CommandError }

func strongestResolverResult(result ResolverResult) ResolverResult {
	if len(result.Candidates) <= 1 {
		return result
	}
	bestScore := result.Candidates[0].Score
	for _, candidate := range result.Candidates[1:] {
		if candidate.Score > bestScore {
			bestScore = candidate.Score
		}
	}
	strongest := make([]domain.VaultObjectCandidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		if candidate.Score == bestScore {
			strongest = append(strongest, candidate)
		}
	}
	result.Candidates = strongest
	result.Facts.Candidates = len(strongest)
	result.Facts.Ambiguous = len(strongest) > 1
	result.Facts.MatchField = ""
	if len(strongest) > 0 && len(strongest[0].MatchFields) > 0 {
		result.Facts.MatchField = strongest[0].MatchFields[0]
	}
	return result
}

func (s *Service) ResolveNote(ctx context.Context, req ShowNoteRequest) (domain.Note, error) {
	note, _, err := s.ResolveNoteWithResolver(ctx, req)
	return note, err
}

func (s *Service) ResolveNoteWithResolver(ctx context.Context, req ShowNoteRequest) (domain.Note, ResolverResult, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return domain.Note{}, ResolverResult{}, err
	}
	result, err := s.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: req.NoteRef, Scope: "registered", Kind: "note"})
	if err != nil {
		return domain.Note{}, result, err
	}
	result = strongestResolverResult(result)
	if len(result.Candidates) > 1 {
		return domain.Note{}, result, &resolverNoteAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Result: result}
	}
	if len(result.Candidates) == 0 {
		return domain.Note{}, result, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
	}
	notes, err := scanNotes(root)
	if err != nil {
		return domain.Note{}, result, err
	}
	for _, note := range notes {
		if note.Path == result.Candidates[0].Path {
			return note, result, nil
		}
	}
	return domain.Note{}, result, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax index refresh, then retry"}
}

func (s *Service) ShowNote(ctx context.Context, req ShowNoteRequest) (domain.Note, error) {
	return s.ResolveNote(ctx, req)
}

func (s *Service) ShowNoteProjection(ctx context.Context, req ShowNoteRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.show", err), err
	}
	note, resolverResult, err := s.ResolveNoteWithResolver(ctx, req)
	if err != nil {
		projection := errorProjection("note.show", err)
		if amb, ok := err.(*resolverNoteAmbiguousError); ok {
			projection.Data = map[string]any{"candidates": amb.Result.Candidates}
			projection.Facts["candidates"] = fmt.Sprint(len(amb.Result.Candidates))
		}
		return projection, err
	}
	view := strings.TrimSpace(req.View)
	if view == "" {
		view = "source"
	}
	displayRequested := strings.TrimSpace(req.Display) != ""
	display := domain.NoteDisplayKind("")
	if displayRequested {
		var displayErr *domain.CommandError
		display, displayErr = parseNoteDisplayKind(req.Display)
		if displayErr != nil {
			return domain.NewErrorProjection("note.show", displayErr), displayErr
		}
	}
	projection := domain.NewProjection("note.show", "Local note read.")
	projection.Facts["path"] = note.Path
	projection.Facts["title"] = note.Title
	projection.Facts["note_id"] = note.ID
	projection.Facts["view"] = view
	if tags := cleanTags(note.Tags); len(tags) > 0 {
		projection.Facts["tags"] = strings.Join(tags, ",")
	}
	body := note.Body
	queryCount := 0
	projection.Facts["resolver.match_field"] = string(resolverResult.Facts.MatchField)
	projection.Facts["resolver.candidates"] = fmt.Sprint(resolverResult.Facts.Candidates)
	queryFacts := map[string]map[string]string{}
	renderRuns := []renderRunReceipt{}
	embeddedAssets := []pinaxassets.EmbeddedAssetPreview{}
	databaseTabs := []domain.DatabaseTab{}
	if req.Runs {
		runs, err := listNoteRenderRuns(root, note.Path)
		if err != nil {
			return errorProjection("note.show", err), err
		}
		renderRuns = runs
		projection.Facts["runs"] = fmt.Sprint(len(runs))
	}
	if view == "rendered" {
		if req.Snapshot != "" {
			snapshot, run, err := loadNoteRenderedSnapshot(root, note.Path, req.Snapshot)
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = snapshot
			projection.Facts["snapshot"] = req.Snapshot
			projection.Facts["run_id"] = run.RunID
		} else {
			rendered, facts, err := s.renderNoteQueryBlocks(ctx, root, note.Body)
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = rendered.Body
			queryCount = rendered.QueryCount
			queryFacts = facts
			databaseTabs = rendered.DatabaseTabs
		}
		if req.EmbedAttachments != "" {
			preview, err := pinaxassets.RenderEmbeddedPreview(pinaxassets.RenderPreviewRequest{Root: root, SourcePath: note.Path, Body: body, Mode: req.EmbedAttachments, MaxDepth: req.MaxEmbedDepth, MaxBytes: req.MaxEmbedBytes})
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = preview.Body
			embeddedAssets = preview.EmbeddedAssets
			projection.Facts["embedded_assets"] = fmt.Sprint(len(embeddedAssets))
			projection.Facts["embed_attachments"] = req.EmbedAttachments
		}
	} else if view != "source" {
		err := &domain.CommandError{Code: "note_view_invalid", Message: "note show --view only supports source or rendered", Hint: "Use --view source or --view rendered"}
		return domain.NewErrorProjection("note.show", err), err
	}
	projection.Facts["query_count"] = fmt.Sprint(queryCount)
	if len(queryFacts) > 0 {
		projection.Facts["queries"] = fmt.Sprint(len(queryFacts))
	}
	if len(databaseTabs) > 0 {
		projection.Facts["database_tabs"] = fmt.Sprint(len(databaseTabs))
	}
	data := map[string]any{"note": note, "body": body, "view": view, "query_facts": queryFacts, "render_runs": renderRuns}
	if displayRequested {
		displayNote := buildNoteDisplay(note, display, domain.NoteExposureLocalDetail)
		if display == domain.NoteDisplayBody {
			displayNote.Body = body
			displayNote.Exposure = domain.NoteExposureLocalBody
		}
		projection.Facts["display"] = string(display)
		data = map[string]any{"note": displayNote, "view": view, "query_facts": queryFacts, "render_runs": renderRuns}
	}
	if len(embeddedAssets) > 0 {
		data["embedded_assets"] = embeddedAssets
	}
	if len(databaseTabs) > 0 {
		data["database_tabs"] = databaseTabs
	}
	projection.Data = data
	return projection, nil
}

func (s *Service) RefreshNoteRendered(ctx context.Context, req NoteRefreshRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Writing back rendered managed blocks requires --yes", Hint: "Add --yes after confirming"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	rendered, facts, err := s.renderNoteQueryBlocks(ctx, root, note.Body)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	if req.Snapshot != "" {
		snapshot, run, err := loadNoteRenderedSnapshot(root, note.Path, req.Snapshot)
		if err != nil {
			return errorProjection("note.refresh", err), err
		}
		rendered = renderedNoteBody{Body: snapshot, ByName: map[string]string{"active": snapshot}, QueryCount: 0}
		facts = map[string]map[string]string{"snapshot": {"run_id": run.RunID}}
	}
	if rendered.QueryCount == 0 {
		err := &domain.CommandError{Code: "render_query_not_found", Message: "No pinax-sql query block found in the note", Hint: "Add a ```pinax-sql <name> query block, then retry"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	updatedBody, changed := replaceManagedRenderBlocks(note.Body, rendered.ByName)
	if changed == 0 {
		err := &domain.CommandError{Code: "render_block_not_found", Message: "No writable pinax render managed block found", Hint: "Add <!-- pinax:render <name> start --> and end marker"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	oldBody := note.Body
	newContent := strings.Replace(string(content), oldBody, updatedBody, 1)
	if newContent == string(content) {
		err := &domain.CommandError{Code: "render_refresh_failed", Message: "Cannot locate the rendered block to write back in the note body", Hint: "Check whether note frontmatter and body were modified externally"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return errorProjection("note.refresh", err), err
	}
	_ = refreshIndex(root)
	projection := domain.NewProjection("note.refresh", "Rendered managed block refreshed.")
	projection.Facts["path"] = note.Path
	projection.Facts["view"] = "rendered"
	projection.Facts["changed_blocks"] = fmt.Sprint(changed)
	projection.Facts["query_count"] = fmt.Sprint(rendered.QueryCount)
	var savedRun *renderRunReceipt
	if req.SaveRun != "" {
		run, err := saveNoteRenderRun(root, note.Path, req.SaveRun, rendered.Body)
		if err != nil {
			return errorProjection("note.refresh", err), err
		}
		projection.Facts["run_saved"] = "true"
		projection.Facts["run_id"] = run.RunID
		projection.Facts["run_name"] = run.Name
		savedRun = &run
	}
	projection.Data = map[string]any{"path": note.Path, "changed_blocks": changed, "query_facts": facts, "render_run": savedRun}
	projection.Evidence = []string{note.Path}
	return projection, nil
}

type renderedNoteBody struct {
	Body         string
	ByName       map[string]string
	QueryCount   int
	DatabaseTabs []domain.DatabaseTab
}

var noteQueryFencePattern = regexp.MustCompile("(?ms)^```(pinax-sql|pinax-dataview)(?:[ \\t]+(?:name=)?([A-Za-z_][A-Za-z0-9_:-]*))?[ \\t]*\\n(.*?)\\n```[ \\t]*(?:\\n|$)")
var noteDatabaseViewFencePattern = regexp.MustCompile("(?ms)^```pinax-database-view(?:[ \\t]+([A-Za-z_][A-Za-z0-9_:-]*))?[ \\t]*\\n(?:([A-Za-z_][A-Za-z0-9_:-]*)[ \\t]*\\n)?```[ \\t]*(?:\\n|$)")
var managedRenderBlockPattern = regexp.MustCompile("(?ms)<!-- pinax:render ([A-Za-z_][A-Za-z0-9_:-]*) start -->.*?<!-- pinax:render ([A-Za-z_][A-Za-z0-9_:-]*) end -->")
var managedDataviewBlockPattern = regexp.MustCompile("(?ms)<!-- pinax:managed name=([A-Za-z_][A-Za-z0-9_:-]*) -->.*?<!-- /pinax:managed -->")

func (s *Service) renderNoteQueryBlocks(ctx context.Context, root, body string) (renderedNoteBody, map[string]map[string]string, error) {
	byName := map[string]string{}
	facts := map[string]map[string]string{}
	databaseTabs := []domain.DatabaseTab{}
	count := 0
	var firstErr error
	rendered := noteQueryFencePattern.ReplaceAllStringFunc(body, func(block string) string {
		if firstErr != nil {
			return block
		}
		match := noteQueryFencePattern.FindStringSubmatch(block)
		if len(match) < 4 {
			return block
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])
		if name == "" {
			name = fmt.Sprintf("query%d", count+1)
		}
		query := strings.TrimSpace(match[3])
		var projection domain.Projection
		var err error
		if kind == "pinax-dataview" {
			projection, err = s.DataviewRun(ctx, DataviewRequest{VaultPath: root, Query: query, Limit: 50, LazyIndex: true})
		} else {
			projection, err = s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: query, Limit: 50, LazyIndex: true})
		}
		if err != nil {
			firstErr = templateQueryCommandError(name, err)
			return block
		}
		result := templateQueryResultFromProjection(projection)
		markdown := templateops.QueryResultMarkdown(result)
		byName[name] = markdown
		facts[name] = projection.Facts
		count++
		return markdown + "\n"
	})
	rendered = noteDatabaseViewFencePattern.ReplaceAllStringFunc(rendered, func(block string) string {
		if firstErr != nil {
			return block
		}
		match := noteDatabaseViewFencePattern.FindStringSubmatch(block)
		if len(match) < 3 {
			return block
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			name = strings.TrimSpace(match[2])
		}
		if name == "" {
			firstErr = &domain.CommandError{Code: "database_tab_view_required", Message: "pinax-database-view fence requires a saved view name", Hint: "Use ```pinax-database-view <name>"}
			return block
		}
		projection, err := s.RenderDatabaseView(ctx, ViewRequest{VaultPath: root, Name: name})
		if err != nil {
			firstErr = databaseTabCommandError(name, err)
			return block
		}
		tab := databaseTabFromProjection(name, projection)
		markdown := databaseTabMarkdown(tab)
		databaseTabs = append(databaseTabs, tab)
		byName[name] = markdown
		facts[name] = projection.Facts
		count++
		return markdown + "\n"
	})
	if firstErr != nil {
		return renderedNoteBody{}, nil, firstErr
	}
	return renderedNoteBody{Body: rendered, ByName: byName, QueryCount: count, DatabaseTabs: databaseTabs}, facts, nil
}

func databaseTabCommandError(name string, err error) *domain.CommandError {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) && commandErr.Code == "view_not_found" {
		return &domain.CommandError{Code: "database_tab_view_not_found", Message: "Database tab saved view not found: " + name, Hint: "Run pinax database view list --vault <vault>"}
	}
	message := "Database tab render failed: " + name
	if err != nil && err.Error() != "" {
		message += ": " + err.Error()
	}
	return &domain.CommandError{Code: "database_tab_render_failed", Message: message, Hint: "Run pinax database view render " + shellQuote(name) + " --vault <vault> --json"}
}

func databaseTabFromProjection(name string, projection domain.Projection) domain.DatabaseTab {
	data, _ := projection.Data.(map[string]any)
	if tab, ok := data["database_tab"].(domain.DatabaseTab); ok {
		return tab
	}
	render, _ := data["render"].(domain.DatabaseViewRender)
	display := projection.Facts["display"]
	if display == "" {
		display = render.Display
	}
	return domain.DatabaseTab{Name: name, View: projection.Facts["view"], Display: display, Rows: render.RowCount, Render: render, Facts: projection.Facts}
}

func databaseTabMarkdown(tab domain.DatabaseTab) string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(tab.Name)
	b.WriteString("\n\n")
	switch tab.Display {
	case "table":
		if tab.Render.Table != nil {
			b.WriteString(tableResultMarkdown(*tab.Render.Table))
		}
	case "list":
		for _, item := range tab.Render.List {
			b.WriteString("- ")
			b.WriteString(item.Title)
			if item.Path != "" {
				b.WriteString(" (")
				b.WriteString(item.Path)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	case "board":
		for _, column := range tab.Render.Board.Columns {
			b.WriteString("#### ")
			b.WriteString(column.ID)
			b.WriteString("\n")
			for _, item := range column.Items {
				b.WriteString("- ")
				b.WriteString(item.Title)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	case "calendar":
		for _, event := range tab.Render.Calendar.Events {
			b.WriteString("- ")
			b.WriteString(event.Date)
			b.WriteString(" - ")
			b.WriteString(event.Title)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func tableResultMarkdown(result domain.TableResult) string {
	converted := templateengine.QueryResult{Columns: result.Columns, Rows: make([]map[string]string, 0, len(result.Rows))}
	for _, row := range result.Rows {
		values := map[string]string{}
		for _, column := range result.Columns {
			values[column] = databaseViewRowValue(row, column)
		}
		converted.Rows = append(converted.Rows, values)
	}
	return templateops.QueryResultMarkdown(converted)
}

func replaceManagedRenderBlocks(body string, rendered map[string]string) (string, int) {
	changed := 0
	updated := managedRenderBlockPattern.ReplaceAllStringFunc(body, func(block string) string {
		match := managedRenderBlockPattern.FindStringSubmatch(block)
		if len(match) < 3 || match[1] != match[2] {
			return block
		}
		name := match[1]
		content, ok := rendered[name]
		if !ok {
			return block
		}
		changed++
		return fmt.Sprintf("<!-- pinax:render %s start -->\n%s\n<!-- pinax:render %s end -->", name, strings.TrimSpace(content), name)
	})
	updated = managedDataviewBlockPattern.ReplaceAllStringFunc(updated, func(block string) string {
		match := managedDataviewBlockPattern.FindStringSubmatch(block)
		if len(match) < 2 {
			return block
		}
		name := match[1]
		content, ok := rendered[name]
		if !ok {
			return block
		}
		changed++
		return fmt.Sprintf("<!-- pinax:managed name=%s -->\n%s\n<!-- /pinax:managed -->", name, strings.TrimSpace(content))
	})
	return updated, changed
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
	projection = domain.NewProjection("note.links", "Note links listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["resolved"] = fmt.Sprint(countResolvedLinks(links))
	projection.Facts["broken"] = fmt.Sprint(countBrokenLinks(links))
	projection.Facts["ambiguous"] = "0"
	projection.Facts["engine"] = "scan"
	addLinkCompatibilityFacts(projection.Facts)
	if countBrokenLinks(links) > 0 {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax repair plan --vault %s", shellQuote(root))}}
	}
	projection.Data = map[string]any{"note": noteGraphNoteSummary(note), "links": links}
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
	projection = domain.NewProjection("note.backlinks", "Note backlinks listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["backlinks"] = fmt.Sprint(len(backlinks))
	projection.Facts["unresolved"] = "0"
	projection.Facts["engine"] = "scan"
	addLinkCompatibilityFacts(projection.Facts)
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
	projection = domain.NewProjection("note.orphans", "Orphan notes listed.")
	projection.Facts["notes"] = fmt.Sprint(len(graph.notes))
	projection.Facts["orphans"] = fmt.Sprint(len(orphans))
	projection.Facts["engine"] = "scan"
	// 与 QueryOrphans 保持同一套 agent-safe 投影：fallback 路径也剥离 Body。
	summaries := make([]domain.Note, 0, len(orphans))
	for _, note := range orphans {
		summaries = append(summaries, noteGraphNoteSummary(note))
	}
	projection.Data = map[string]any{"orphans": summaries}
	return projection, nil
}
func (s *Service) AttachNoteFile(ctx context.Context, req NoteAttachRequest) (domain.Projection, error) {
	root, note, notePath, content, _, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	source := strings.TrimSpace(req.SourcePath)
	if source == "" {
		err := &domain.CommandError{Code: "attachment_source_required", Message: "note attach requires a source file", Hint: "pinax note attach <note> <file> --vault <vault>"}
		return domain.NewErrorProjection("note.attach", err), err
	}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		commandErr := &domain.CommandError{Code: "attachment_source_missing", Message: "Attachment source file does not exist", Hint: "Check the source file path, then retry"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if info.IsDir() {
		commandErr := &domain.CommandError{Code: "attachment_source_is_directory", Message: "Attachment source path is a directory", Hint: "Provide a single file path"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	mode := normalizedAttachmentMode(req.Mode)
	if mode == "" {
		commandErr := &domain.CommandError{Code: "attachment_mode_invalid", Message: "Attachment write mode is invalid", Hint: "Use --mode copy, move, or register"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	if mode == "move" && !req.Yes {
		commandErr := &domain.CommandError{Code: "approval_required", Message: "Moving the source file requires explicit confirmation", Hint: "pinax note attach " + req.NoteRef + " " + source + " --mode move --yes --vault " + root + " --json"}
		return domain.NewErrorProjection("note.attach", commandErr), commandErr
	}
	filename := filepath.Base(source)
	if strings.TrimSpace(req.Rename) != "" {
		filename = req.Rename
	}
	placement := normalizedAttachmentPlacement(req.Placement)
	attachmentRel := ""
	if mode == "register" {
		if strings.TrimSpace(req.Rename) != "" {
			commandErr := &domain.CommandError{Code: "attachment_rename_requires_copy_or_move", Message: "register mode does not rename files", Hint: "Remove --rename, or switch to --mode copy"}
			return domain.NewErrorProjection("note.attach", commandErr), commandErr
		}
		attachmentRel, err = registeredAttachmentRel(root, source)
		if err != nil {
			return errorProjection("note.attach", err), err
		}
	} else {
		attachmentRel, err = uniqueAttachmentRelWithPlacement(root, note, filename, placement)
		if err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	linkStyle, reference, err := attachmentReference(note.Path, attachmentRel, req.LinkStyle, req.Embed)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	attachmentPath, err := safeJoin(root, attachmentRel)
	if err != nil {
		return errorProjection("note.attach", err), err
	}
	if mode != "register" {
		if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o755); err != nil {
			return errorProjection("note.attach", err), err
		}
		if err := copyFile(source, attachmentPath); err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	updated := strings.TrimRight(content, "\n") + "\n\n" + reference + "\n"
	if err := commitNoteContent(notePath, notePath, updated); err != nil {
		return errorProjection("note.attach", err), err
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.attach", err), err
	}
	if mode == "move" {
		if err := os.Remove(source); err != nil {
			return errorProjection("note.attach", err), err
		}
	}
	_ = appendEvent(root, "note.attach", "success", map[string]string{"path": note.Path, "attachment_path": attachmentRel})
	projection := domain.NewProjection("note.attach", "Attachment added to note.")
	projection.Facts["path"] = note.Path
	projection.Facts["attachment_path"] = attachmentRel
	projection.Facts["source_path"] = source
	projection.Facts["media_type"] = attachmentMediaType(attachmentRel)
	projection.Facts["placement"] = string(placement)
	projection.Facts["link_style"] = linkStyle
	projection.Facts["mode"] = mode
	projection.Facts["reference"] = reference
	projection.Facts["index_status"] = "fresh"
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{note.Path, attachmentRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"note": note, "attachment": domain.NoteAttachment{NotePath: note.Path, ReferenceText: reference, Path: attachmentRel, TargetPath: attachmentRel, MediaType: attachmentMediaType(attachmentRel), Exists: true}}
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
	if req.PathStyle != "" || req.IncludePaths {
		if err := applyAttachmentDisplayPaths(root, note.Path, req.PathStyle, attachments); err != nil {
			return errorProjection("note.attachments", err), err
		}
	}
	projection := domain.NewProjection("note.attachments", "Note attachments listed.")
	projection.Facts["path"] = note.Path
	projection.Facts["attachments"] = fmt.Sprint(len(attachments))
	projection.Facts["missing"] = fmt.Sprint(countMissingAttachments(attachments))
	if req.PathStyle != "" {
		projection.Facts["path_style"] = req.PathStyle
	}
	projection.Data = map[string]any{"note": note, "attachments": attachments}
	return projection, nil
}
func (s *Service) ImportMarkdown(_ context.Context, req ImportMarkdownRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("import.markdown", err), err
	}
	safeTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("import.markdown", tagErr), tagErr
	}
	req.Tags = safeTags
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
	projection := domain.NewProjection("import.markdown", "Markdown import plan generated.")
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
		err := &domain.CommandError{Code: "approval_required", Message: "import markdown requires --yes", Hint: "Preview the plan with --dry-run first, then add --yes after confirming"}
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
	projection.Summary = "Markdown imported."
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
	projection := domain.NewProjection("export.markdown", "Markdown exported.")
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
		commandErr := &domain.CommandError{Code: "editor_failed", Message: "Editor execution failed", Hint: "Check the executable pointed to by --editor or $EDITOR"}
		return domain.NewErrorProjection("note.edit", commandErr), commandErr
	}
	projection := domain.NewProjection("note.edit", "Note opened in editor.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["editor"] = editor.Raw
	projection.Facts["editor_executable"] = editor.Executable
	projection.Facts["editor_args"] = strings.Join(editor.Args, " ")
	projection.Data = map[string]any{"note": note, "editor": editor}
	return projection, nil
}

func (s *Service) RenameNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.rename", err), err
	}
	newTitle := strings.TrimSpace(req.Title)
	if newTitle == "" {
		err := &domain.CommandError{Code: "title_required", Message: "note rename requires a new title", Hint: "pinax note rename <note> <title> --vault <vault>"}
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
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another title or move the existing file first"}
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
	projection := noteMutationProjection("note.rename", "Note renamed.", targetRel, meta)
	recordNote := domain.Note{ID: meta["note_id"], Title: newTitle, Path: targetRel, Body: strings.TrimSpace(strings.TrimPrefix(updated, renderFrontmatter(meta, "")))}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteRenamed, "note.rename:"+recordNote.ID+":"+targetRel, recordNote, note.Path)
	if recordErr != nil {
		return errorProjection("note.rename", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return projection, nil
}

func (s *Service) MoveNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, _, _, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
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
		err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Choose another directory or move the existing file first"}
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
	projection := noteMutationProjection("note.move", "Note moved.", targetRel, map[string]string{"note_id": note.ID, "title": note.Title})
	note.Path = targetRel
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, "note.move:"+note.ID+":"+note.Path+":"+targetRel, note, req.NoteRef)
	if recordErr != nil {
		return errorProjection("note.move", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return projection, nil
}

func (s *Service) ArchiveNote(ctx context.Context, req NoteMutationRequest) (domain.Projection, error) {
	root, note, path, content, meta, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
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
	projection := noteMutationProjection("note.archive", "Note archived.", note.Path, meta)
	projection.Facts["status"] = "archived"
	note.Status = "archived"
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteArchived, "note.archive:"+note.ID+":"+note.Path, note, "")
	if recordErr != nil {
		return errorProjection("note.archive", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return projection, nil
}

func (s *Service) DeleteNote(ctx context.Context, req NoteDeleteRequest) (domain.Projection, error) {
	root, note, path, _, _, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.delete", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "note delete requires --yes", Hint: "Add --yes after confirming; hard delete also requires --hard"}
		return domain.NewErrorProjection("note.delete", err), err
	}
	projection := domain.NewProjection("note.delete", "Note deleted.")
	projection.Facts["path"] = note.Path
	projection.Facts["note_id"] = note.ID
	if req.Hard {
		if err := os.Remove(path); err != nil {
			return errorProjection("note.delete", err), err
		}
		_ = appendEvent(root, "note.delete", "success", map[string]string{"path": note.Path, "hard": "true"})
		projection.Facts["hard"] = "true"
		recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteDeleted, "note.delete:"+note.ID+":"+note.Path, note, "")
		if recordErr != nil {
			return errorProjection("note.delete", recordErr), recordErr
		}
		applyRecordEventFacts(&projection, recordEvent)
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
	projection.Summary = "Note moved to trash."
	projection.Facts["trash_path"] = trashRel
	projection.Data = map[string]any{"note": note, "trash_path": trashRel}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteTrashed, "note.trash:"+note.ID+":"+note.Path+":"+trashRel, note, note.Path)
	if recordErr != nil {
		return errorProjection("note.delete", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	return projection, nil
}

func (s *Service) TagNote(ctx context.Context, req NoteTagRequest) (domain.Projection, error) {
	requestTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.tag", tagErr), tagErr
	}
	root, note, path, content, meta, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.tag", err), err
	}
	tags, tagErr := normalizeTagsForWrite(note.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.tag", tagErr), tagErr
	}
	switch req.Operation {
	case "add":
		tags = mergeTags(tags, requestTags)
	case "remove":
		tags = removeTags(tags, requestTags)
	case "set":
		tags = requestTags
	default:
		err := &domain.CommandError{Code: "invalid_tag_operation", Message: "Unknown tag operation", Hint: "Use add, remove, or set"}
		return domain.NewErrorProjection("note.tag", err), err
	}
	meta["tags"] = formatTags(tags)
	meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, _ := patchFrontmatterFields(content, meta)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.tag", err), err
	}
	_ = appendEvent(root, "note.tag", "success", map[string]string{"path": note.Path, "operation": req.Operation})
	projection := noteMutationProjection("note.tag", "Note tags updated.", note.Path, meta)
	projection.Facts["tags"] = strings.Join(tags, ",")
	projection.Data = map[string]any{"note": domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: tags, Project: note.Project, Status: meta["status"]}}
	recordNote := domain.Note{ID: note.ID, Title: note.Title, Path: note.Path, Tags: tags, Body: strings.TrimSpace(strings.TrimPrefix(updated, renderFrontmatter(meta, ""))), Project: note.Project, Status: meta["status"]}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "note.tag:"+recordNote.ID+":"+req.Operation+":"+strings.Join(tags, ","), recordNote, "")
	if recordErr != nil {
		return errorProjection("note.tag", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if err := applyRecordStateFacts(ctx, &projection, root, recordEvent.NoteID); err != nil {
		return errorProjection("note.tag", err), err
	}
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) PatchNoteProperty(ctx context.Context, req NotePropertyRequest) (domain.Projection, error) {
	key, keyErr := normalizePropertyKey(req.Key)
	if keyErr != nil {
		return domain.NewErrorProjection("note.property", keyErr), keyErr
	}
	operation := strings.TrimSpace(req.Operation)
	root, note, path, content, meta, _, err := s.loadMutableNoteForWrite(ctx, req.VaultPath, req.NoteRef)
	if err != nil {
		return errorProjection("note.property", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	fields := map[string]string{"updated_at": now}
	removed := []string{}
	value := ""
	summary := "Note property updated."
	switch operation {
	case "set":
		formatted, valueErr := formatPropertyValue(req.Value)
		if valueErr != nil {
			return domain.NewErrorProjection("note.property", valueErr), valueErr
		}
		fields[key] = formatted
		meta[key] = formatted
		value = strings.Trim(strings.TrimSpace(formatted), "\"")
	case "remove":
		removed = []string{key}
		delete(meta, key)
		summary = "Note property removed."
	default:
		err := &domain.CommandError{Code: "invalid_property_operation", Message: "Unknown property operation", Hint: "Use set or remove"}
		return domain.NewErrorProjection("note.property", err), err
	}
	meta["updated_at"] = now
	updated, _ := patchFrontmatterFieldsRemoving(content, fields, removed)
	if err := commitNoteContent(path, path, updated); err != nil {
		return errorProjection("note.property", err), err
	}
	_ = appendEvent(root, "note.property", "success", map[string]string{"path": note.Path, "operation": operation, "property": key})
	projection := noteMutationProjection("note.property", summary, note.Path, meta)
	projection.Facts["operation"] = operation
	projection.Facts["property"] = key
	if value != "" {
		projection.Facts["value"] = value
	}
	parsed := parseNote(note.Path, updated)
	projection.Data = map[string]any{"note": parsed, "frontmatter": meta, "property": key, "operation": operation, "value": value}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "note.property:"+parsed.ID+":"+operation+":"+key+":"+value, parsed, "")
	if recordErr != nil {
		return errorProjection("note.property", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	if err := applyRecordStateFacts(ctx, &projection, root, recordEvent.NoteID); err != nil {
		return errorProjection("note.property", err), err
	}
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) BulkTag(ctx context.Context, req NoteTagBulkRequest) (domain.Projection, error) {
	oldTags, tagErr := normalizeTagsForWrite([]string{req.OldTag})
	if tagErr != nil {
		return domain.NewErrorProjection("tag."+req.Operation, tagErr), tagErr
	}
	if len(oldTags) != 1 {
		err := &domain.CommandError{Code: "invalid_tag", Message: "A tag is required", Hint: "pinax note tags rename <old> <new> --vault <vault> --yes"}
		return domain.NewErrorProjection("tag."+req.Operation, err), err
	}
	oldTag := oldTags[0]
	newTag := ""
	if req.Operation == "rename" {
		newTags, tagErr := normalizeTagsForWrite([]string{req.NewTag})
		if tagErr != nil {
			return domain.NewErrorProjection("tag.rename", tagErr), tagErr
		}
		if len(newTags) != 1 {
			err := &domain.CommandError{Code: "invalid_tag", Message: "rename requires a new tag", Hint: "pinax note tags rename <old> <new> --vault <vault> --yes"}
			return domain.NewErrorProjection("tag.rename", err), err
		}
		newTag = newTags[0]
	}
	command := "tag." + req.Operation
	if req.Operation != "rename" && req.Operation != "delete" {
		err := &domain.CommandError{Code: "invalid_tag_operation", Message: "Unknown tag batch operation", Hint: "Use rename or delete"}
		return domain.NewErrorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Batch tag writes require --yes", Hint: "Add --dry-run to preview first, then add --yes after confirming"}
		return domain.NewErrorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	changed := make([]domain.Note, 0)
	for _, note := range notes {
		if !containsString(cleanTags(note.Tags), oldTag) {
			continue
		}
		updatedTags := removeTags(note.Tags, []string{oldTag})
		if req.Operation == "rename" {
			updatedTags = mergeTags(updatedTags, []string{newTag})
		}
		note.Tags = updatedTags
		changed = append(changed, note)
	}
	projection := domain.NewProjection(command, "Tag batch plan generated.")
	projection.Facts["old_tag"] = oldTag
	if newTag != "" {
		projection.Facts["new_tag"] = newTag
	}
	projection.Facts["matched"] = fmt.Sprint(len(changed))
	projection.Facts["changed"] = fmt.Sprint(len(changed))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Data = map[string]any{"notes": changed, "old_tag": oldTag, "new_tag": newTag, "operation": req.Operation, "dry_run": req.DryRun}
	if req.DryRun || len(changed) == 0 {
		return projection, nil
	}
	projection.Summary = "Tag batch update applied."
	recordEvents := 0
	for _, note := range changed {
		_, current, path, content, meta, _, err := loadMutableResolvedNote(root, note)
		if err != nil {
			return errorProjection(command, err), err
		}
		meta["tags"] = formatTags(note.Tags)
		meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		updated, _ := patchFrontmatterFields(content, meta)
		if err := commitNoteContent(path, path, updated); err != nil {
			return errorProjection(command, err), err
		}
		parsed := parseNote(current.Path, updated)
		if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, command+":"+parsed.ID+":"+oldTag+":"+newTag, parsed, ""); err != nil {
			return errorProjection(command, err), err
		}
		recordEvents++
	}
	_ = appendEvent(root, command, "success", map[string]string{"old_tag": oldTag, "new_tag": newTag, "changed": fmt.Sprint(len(changed))})
	projection.Facts["record_events"] = fmt.Sprint(recordEvents)
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func (s *Service) BulkFolder(ctx context.Context, req NoteFolderBulkRequest) (domain.Projection, error) {
	command := "folder." + strings.TrimSpace(req.Operation)
	if command == "folder." {
		command = "folder.rename"
	}
	if strings.TrimSpace(req.Operation) != "rename" {
		err := &domain.CommandError{Code: "invalid_folder_operation", Message: "Unknown folder batch operation", Hint: "Use rename"}
		return domain.NewErrorProjection(command, err), err
	}
	oldFolder, folderErr := validateRequiredNoteFolder(req.OldFolder, "old")
	if folderErr != nil {
		return domain.NewErrorProjection(command, folderErr), folderErr
	}
	newFolder, folderErr := validateRequiredNoteFolder(req.NewFolder, "new")
	if folderErr != nil {
		return domain.NewErrorProjection(command, folderErr), folderErr
	}
	if oldFolder == newFolder {
		err := &domain.CommandError{Code: "invalid_folder", Message: "Old and new folders cannot be the same", Hint: "Provide a different target folder, or use pinax note folders list to view existing folders"}
		return domain.NewErrorProjection(command, err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Batch folder writes require --yes", Hint: "Preview first with pinax note folders rename <old> <new> --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	type folderChange struct {
		Note       domain.Note `json:"note"`
		OldPath    string      `json:"old_path"`
		TargetPath string      `json:"target_path"`
		OldFolder  string      `json:"old_folder"`
		NewFolder  string      `json:"new_folder"`
	}
	changes := []folderChange{}
	seenTargets := map[string]string{}
	for _, note := range notes {
		if !noteHasFolder(note, oldFolder) {
			continue
		}
		targetRel := folderRenameTargetPath(note, oldFolder, newFolder)
		change := folderChange{Note: note, OldPath: note.Path, TargetPath: targetRel, OldFolder: oldFolder, NewFolder: newFolder}
		changes = append(changes, change)
		if previous := seenTargets[targetRel]; previous != "" && previous != note.Path {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Multiple notes would be written to the same target path", Hint: "Rename conflicting notes first or choose a more specific folder"}
			return domain.NewErrorProjection(command, err), err
		}
		seenTargets[targetRel] = note.Path
	}
	for _, change := range changes {
		if change.TargetPath == change.OldPath {
			continue
		}
		target, err := safeJoin(root, change.TargetPath)
		if err != nil {
			return errorProjection(command, err), err
		}
		if _, err := os.Stat(target); err == nil {
			err := &domain.CommandError{Code: "note_path_conflict", Message: "Target note path already exists", Hint: "Move or rename the same-named note in the target directory first"}
			return domain.NewErrorProjection(command, err), err
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errorProjection(command, err), err
		}
	}
	projection := domain.NewProjection(command, "Folder batch rename plan generated.")
	projection.Facts["old_folder"] = oldFolder
	projection.Facts["new_folder"] = newFolder
	projection.Facts["matched"] = fmt.Sprint(len(changes))
	projection.Facts["changed"] = fmt.Sprint(len(changes))
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["writes"] = fmt.Sprint(!req.DryRun)
	projection.Facts["requires_snapshot"] = "true"
	projection.Data = map[string]any{"changes": changes, "old_folder": oldFolder, "new_folder": newFolder, "operation": "rename", "dry_run": req.DryRun}
	if req.DryRun || len(changes) == 0 {
		return projection, nil
	}
	projection.Summary = "Folder batch rename applied."
	recordEvents := 0
	for _, change := range changes {
		_, note, path, content, meta, _, err := loadMutableResolvedNote(root, change.Note)
		if err != nil {
			return errorProjection(command, err), err
		}
		meta["folder"] = newFolder
		meta["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		updated, _ := patchFrontmatterFields(content, meta)
		target, err := safeJoin(root, change.TargetPath)
		if err != nil {
			return errorProjection(command, err), err
		}
		if err := commitNoteContent(path, target, updated); err != nil {
			return errorProjection(command, err), err
		}
		parsed := parseNote(change.TargetPath, updated)
		parsed.Path = change.TargetPath
		if parsed.ID == "" {
			parsed.ID = note.ID
		}
		if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, command+":"+parsed.ID+":"+oldFolder+":"+newFolder+":"+change.TargetPath, parsed, change.OldPath); err != nil {
			return errorProjection(command, err), err
		}
		recordEvents++
	}
	_ = appendEvent(root, command, "success", map[string]string{"old_folder": oldFolder, "new_folder": newFolder, "changed": fmt.Sprint(len(changes))})
	projection.Facts["record_events"] = fmt.Sprint(recordEvents)
	if err := refreshIndex(root); err != nil {
		projection.Status = "partial"
		projection.Facts["index_status"] = "stale"
		projection.Actions = append(projection.Actions, domain.Action{Name: "rebuild_index", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))})
		return projection, nil
	}
	projection.Facts["index_updated"] = "true"
	return projection, nil
}

func validateRequiredNoteFolder(raw, label string) (string, *domain.CommandError) {
	folder, err := validateOptionalNoteFolder(raw)
	if err != nil {
		if commandErr, ok := err.(*domain.CommandError); ok {
			return "", commandErr
		}
		return "", &domain.CommandError{Code: "invalid_folder", Message: err.Error(), Hint: "Use a folder like inbox, reference, or work/research"}
	}
	if folder == "" {
		return "", &domain.CommandError{Code: "invalid_folder", Message: label + " folder cannot be empty", Hint: "pinax note folders rename <old> <new> --vault <vault> --dry-run"}
	}
	return folder, nil
}

func noteHasFolder(note domain.Note, folder string) bool {
	for _, value := range noteDimensionValues(note, "folder") {
		if value == folder {
			return true
		}
	}
	return false
}

func folderRenameTargetPath(note domain.Note, oldFolder, newFolder string) string {
	dir := filepath.ToSlash(filepath.Dir(note.Path))
	if dir == "." {
		dir = ""
	}
	base := filepath.Base(note.Path)
	if dir == oldFolder {
		return filepath.ToSlash(filepath.Join(newFolder, base))
	}
	if strings.HasPrefix(dir, "notes/") && strings.TrimPrefix(dir, "notes/") == oldFolder {
		return filepath.ToSlash(filepath.Join("notes", newFolder, base))
	}
	if strings.HasSuffix(dir, "/"+oldFolder) {
		prefix := strings.TrimSuffix(dir, oldFolder)
		return filepath.ToSlash(filepath.Join(prefix, newFolder, base))
	}
	return filepath.ToSlash(filepath.Join(newFolder, base))
}

func (s *Service) SearchNotes(ctx context.Context, req SearchRequest) (result SearchResult, err error) {
	searchReq := toSearchOpsRequest(req)
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return SearchResult{}, err
	}
	monitorFacts := monitorQueryFacts("query", req.Query)
	monitorFacts["engine_requested"] = searchops.NormalizedEngine(req.Engine)
	monitorFacts["lazy_index"] = searchops.NormalizedLazyIndex(req.LazyIndex)
	monitorFacts["limit"] = fmt.Sprint(req.Limit)
	rec := startMonitorRun(root, "note.search", monitorFacts)
	defer func() { rec.Finish("", err) }()
	searchReq.VaultPath = root
	endStep := rec.BeginStep("search.validate", nil)
	if err := searchops.ValidateVersionAware(searchReq); err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	if err := searchops.ValidateDateFilters(searchReq); err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	endStep(nil)
	engine := searchops.NormalizedEngine(searchReq.Engine)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	endStep = rec.BeginStep("link_filter.build", map[string]string{"active": fmt.Sprint(strings.TrimSpace(req.LinkTarget) != "")})
	linkFilter, err := searchops.BuildLinkTargetFilter(notes, req.LinkTarget, func(notes []domain.Note) map[string][]domain.NoteLink {
		outgoing, _ := BuildEnhancedLinkGraph(notes)
		return outgoing
	})
	if err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	endStep(nil)
	endStep = rec.BeginStep("index.inspect", nil)
	status, err := noteindex.Inspect(root, notes)
	endStep(err)
	indexLoaded := ""
	if err == nil && engine == "auto" && searchops.LazyIndexAllowed(searchReq, status, notes) {
		select {
		case <-ctx.Done():
			return SearchResult{}, ctx.Err()
		default:
		}
		endStep = rec.BeginStep("index.lazy_rebuild", map[string]string{"index_status": status.Status})
		if _, rebuildErr := noteindex.Rebuild(root, notes); rebuildErr == nil {
			endStep(nil)
			endStep = rec.BeginStep("index.inspect_after_lazy_rebuild", nil)
			if rebuiltStatus, inspectErr := noteindex.Inspect(root, notes); inspectErr == nil {
				status = rebuiltStatus
				indexLoaded = "lazy_rebuild"
			}
			endStep(nil)
		} else {
			endStep(rebuildErr)
		}
	}
	if engine == "native" {
		endStep = rec.BeginStep("native.search", nil)
		result := notesearch.Notes(ctx, root, req.Query, notes)
		endStep(nil)
		indexStatus := "missing"
		if err == nil && status.Status != "" {
			indexStatus = status.Status
		}
		return searchops.ResultFromFallback(searchReq, result.Engine, result.Notes, indexStatus, linkFilter), nil
	}
	if engine == "index" && (err != nil || (status.Status != "fresh" && (status.Status != "stale" || !req.AllowStale))) {
		indexStatus := "missing"
		if err == nil && status.Status != "" {
			indexStatus = status.Status
		}
		return SearchResult{}, &domain.CommandError{Code: "search_index_unavailable", Message: "Search index is not available for --engine index: " + indexStatus, Hint: fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root))}
	}
	if err == nil && (status.Status == "fresh" || (status.Status == "stale" && req.AllowStale)) {
		indexReq := searchops.BuildIndexRequest(searchReq, linkFilter)
		endStep = rec.BeginStep("index.search", map[string]string{"index_status": status.Status})
		result, searchErr := noteindex.Search(root, indexReq)
		endStep(searchErr)
		if searchErr == nil && (result.Returned > 0 || strings.TrimSpace(req.Query) == "") {
			result.IndexStatus = status.Status
			return searchops.ResultFromIndex(searchReq, indexLoaded, result, linkFilter), nil
		}
	}
	endStep = rec.BeginStep("native.search", map[string]string{"fallback": "true"})
	nativeResult := notesearch.Notes(ctx, root, req.Query, notes)
	endStep(nil)
	indexStatus := "missing"
	if err == nil && status.Status != "" {
		indexStatus = status.Status
	}
	return searchops.ResultFromFallback(searchReq, nativeResult.Engine, nativeResult.Notes, indexStatus, linkFilter), nil
}

type SearchResult = searchops.Result

func (s *Service) SearchProjection(ctx context.Context, req SearchRequest) (domain.Projection, error) {
	result, err := s.SearchNotes(ctx, req)
	if err != nil {
		return errorProjection("note.search", err), err
	}
	return searchops.Projection(toSearchOpsRequest(req), result, shellQuote), nil
}

func parseUserDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func (s *Service) PlanMetadata(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	ops := make([]domain.PlanOperation, 0)
	query := strings.TrimSpace(req.Query)
	if query != "" {
		resolverResult, err := s.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: query, Scope: "registered_or_adoptable", Kind: "all"})
		if err != nil {
			return errorProjection("metadata.plan", err), err
		}
		candidates := resolverResult.Candidates
		if len(candidates) > 1 {
			err := &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "metadata plan query matched multiple candidates", Hint: "Retry with a more specific note_id, filename, or full path"}
			projection := domain.NewErrorProjection("metadata.plan", err)
			projection.Facts["candidates"] = fmt.Sprint(len(candidates))
			projection.Data = map[string]any{"candidates": candidates}
			return projection, err
		}
		if len(candidates) == 1 {
			candidate := candidates[0]
			if candidate.ObjectKind == "file" {
				ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: candidate.Path, Reason: "Add Pinax metadata to adoptable Markdown", Status: "planned"})
			} else {
				matchedNote := false
				for _, note := range notes {
					if note.Path != candidate.Path {
						continue
					}
					matchedNote = true
					if noteNeedsMetadataInVault(root, note) {
						ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: note.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
					}
					ops = append(ops, durableSourceMetadataOperations(note)...)
				}
				if !matchedNote {
					ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: candidate.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
				}
			}
		}
		projection := domain.NewProjection("metadata.plan", "Metadata plan generated.")
		projection.Facts["query"] = query
		projection.Facts["writes"] = "false"
		projection.Facts["candidates"] = fmt.Sprint(len(candidates))
		projection.Facts["planned_updates"] = fmt.Sprint(len(ops))
		projection.Data = map[string]any{"operations": ops, "candidates": candidates}
		if len(candidates) == 1 && candidates[0].ObjectKind == "file" {
			projection.Actions = []domain.Action{{Name: "adopt", Command: fmt.Sprintf("pinax record adopt %s --plan --vault %s --json", shellQuote(query), shellQuote(root))}}
		} else if len(ops) > 0 {
			projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax metadata apply --vault %s --yes", shellQuote(root))}}
		}
		return projection, nil
	}
	for _, note := range notes {
		if noteNeedsMetadataInVault(root, note) {
			ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: note.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
		}
		ops = append(ops, durableSourceMetadataOperations(note)...)
	}
	projection := domain.NewProjection("metadata.plan", "Metadata plan generated.")
	projection.Facts["planned_updates"] = fmt.Sprint(len(ops))
	projection.Data = map[string]any{"operations": ops}
	if len(ops) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax metadata apply --vault %s --yes", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ApplyMetadata(ctx context.Context, req ApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "metadata apply requires --yes", Hint: "Run pinax metadata plan first, then add --yes after confirming"}
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
	projection := domain.NewProjection("metadata.apply", "Metadata applied.")
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
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	ops = append(ops, organizeFactOperations(root, organizeCandidateFacts(facts))...)
	moves := 0
	manualReview := 0
	for _, op := range ops {
		if op.Kind == "move" && op.Status == "planned" {
			moves++
		}
		if op.Status == "manual_review" {
			manualReview++
		}
	}
	projection := domain.NewProjection("organize.plan", "Organize plan generated.")
	projection.Facts["planned_moves"] = fmt.Sprint(moves)
	projection.Facts["manual_review"] = fmt.Sprint(manualReview)
	projection.Data = map[string]any{"operations": ops}
	if moves > 0 {
		projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}}
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
	projection := domain.NewProjection("organize.suggest", "Organize suggestions generated.")
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
		projection.Actions = []domain.Action{{Name: "save", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
	} else if plan.SavedPath != "" && len(plan.Operations) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plan.PlanID), shellQuote("snapshot before organize"))}}
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
	projection := domain.NewProjection("organize.list", "Organize plans listed.")
	projection.Facts["plans"] = fmt.Sprint(len(plans))
	projection.Data = map[string]any{"plans": plans}
	if len(plans) == 0 {
		projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plans[0].PlanID), shellQuote("snapshot before organize"))}}
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
		hint := fmt.Sprintf("Run pinax organize plan --vault %s --save first, review the plan, then run pinax organize apply --vault %s --plan <plan_id> --yes --snapshot-message %s", shellQuote(vault), shellQuote(vault), shellQuote("snapshot before organize"))
		err := &domain.CommandError{Code: "approval_required", Message: "organize apply requires --yes", Hint: hint}
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
			projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
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
		err := &domain.CommandError{Code: "snapshot_required", Message: "Organizing structure requires an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}
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
	projection := domain.NewProjection("organize.apply", "Organize structure applied.")
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
		tags, err := normalizeTagsForWrite(strings.Split(op.Target, ","))
		if err != nil {
			return err
		}
		fields["tags"] = formatTags(tags)
	case "status_patch":
		fields["status"] = op.Target
	default:
		return nil
	}
	return applyRepairFrontmatterPatch(root, op.Path, fields)
}

func (s *Service) PromptImport(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptImport(ctx, req, "prompt.import")
}

func (s *Service) PromptCreate(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptImport(ctx, req, "prompt.create")
}

func (s *Service) PromptSearch(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	assets, err := repo.Search(ctx, promptasset.SearchRequest{Query: req.Query, Domain: req.Domain, Tag: req.Tag, Lifecycle: req.Lifecycle, Limit: req.Limit})
	if err != nil {
		return errorProjection("prompt.search", err), err
	}
	projection := domain.NewProjection("prompt.search", "Prompt asset search completed.")
	projection.Facts["results"] = fmt.Sprint(len(assets))
	if req.Query != "" {
		projection.Facts["query"] = req.Query
	}
	if req.Domain != "" {
		projection.Facts["domain"] = req.Domain
	}
	for i, asset := range assets {
		prefix := fmt.Sprintf("prompt_asset.%d.", i+1)
		projection.Facts[prefix+"id"] = asset.PromptAssetID
		projection.Facts[prefix+"lifecycle"] = asset.Lifecycle
		projection.Facts[prefix+"permission"] = asset.Permission
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_assets": assets}
	return projection, nil
}

func (s *Service) PromptShow(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptDetails(ctx, req, "prompt.show")
}

func (s *Service) PromptResolve(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	return s.promptDetails(ctx, req, "prompt.resolve")
}

func (s *Service) PromptLifecycle(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.lifecycle", err), err
	}
	if strings.TrimSpace(req.Ref) == "" || strings.TrimSpace(req.To) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt lifecycle requires an asset id and --to", Hint: "pinax prompt lifecycle <id> --to tested --reason <reason> --vault <vault> --json"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	if strings.TrimSpace(req.Reason) == "" {
		err := &domain.CommandError{Code: "reason_required", Message: "prompt lifecycle requires --reason", Hint: "Record why Pinax changed the lifecycle"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.lifecycle", err), err
	}
	record, err := repo.UpdateLifecycle(ctx, req.Ref, req.To)
	if err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt search %s --vault %s --json", shellQuote(req.Ref), shellQuote(root))}
			return domain.NewErrorProjection("prompt.lifecycle", err), err
		}
		err := &domain.CommandError{Code: "prompt_lifecycle_invalid", Message: err.Error(), Hint: "Use draft, tested, accepted, promoted, or retired"}
		return domain.NewErrorProjection("prompt.lifecycle", err), err
	}
	feedbackID := lifecycleFeedbackID(record.PromptAssetID, req.To, req.Reason)
	_, _ = repo.ImportFeedback(ctx, promptasset.Feedback{FeedbackID: feedbackID, PromptAssetID: record.PromptAssetID, VersionID: record.CurrentVersionID, PromptTemplateHash: record.PromptTemplateHash, ExternalRunRef: "pinax://prompt/lifecycle", Decision: req.To, Reason: req.Reason})
	projection := domain.NewProjection("prompt.lifecycle", "Prompt asset lifecycle updated.")
	promptAssetFacts(&projection, record)
	projection.Facts["reason_recorded"] = "true"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": record, "feedback_id": feedbackID}
	return projection, nil
}

func (s *Service) PromptFeedbackImport(ctx context.Context, req PromptAssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	if strings.TrimSpace(req.From) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt feedback import requires --from", Hint: "pinax prompt feedback import --from <file> --vault <vault> --json"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	content, err := os.ReadFile(req.From)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_feedback_source_unreadable", Message: "Prompt feedback source cannot be read", Hint: "Check the --from file path and retry"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	var payload promptFeedbackPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		err := &domain.CommandError{Code: "prompt_feedback_invalid", Message: err.Error(), Hint: "Provide a valid Eikona prompt usage feedback JSON file"}
		return domain.NewErrorProjection("prompt.feedback.import", err), err
	}
	feedback, commandErr := payload.toFeedback()
	if commandErr != nil {
		return domain.NewErrorProjection("prompt.feedback.import", commandErr), commandErr
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	if _, err := repo.Resolve(ctx, feedback.PromptAssetID); err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt import --from <asset-file> --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("prompt.feedback.import", err), err
		}
		return errorProjection("prompt.feedback.import", err), err
	}
	result, err := repo.ImportFeedback(ctx, feedback)
	if err != nil {
		return errorProjection("prompt.feedback.import", err), err
	}
	projection := domain.NewProjection("prompt.feedback.import", "Prompt usage feedback imported.")
	projection.Facts["prompt_asset_id"] = result.Record.PromptAssetID
	projection.Facts["feedback_id"] = result.Record.FeedbackID
	projection.Facts["imported"] = fmt.Sprint(result.Imported)
	projection.Facts["decision"] = result.Record.Decision
	if result.Record.Decision != "" {
		projection.Actions = []domain.Action{{Name: "lifecycle", Command: fmt.Sprintf("pinax prompt lifecycle %s --to %s --reason <reason> --json", shellQuote(result.Record.PromptAssetID), shellQuote(result.Record.Decision))}}
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"feedback": result.Record, "imported": result.Imported}
	return projection, nil
}

func (s *Service) promptImport(ctx context.Context, req PromptAssetRequest, command string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if strings.TrimSpace(req.From) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "prompt import requires --from", Hint: "pinax prompt import --from <file> --vault <vault> --json"}
		if command == "prompt.create" {
			err.Message = "prompt create requires --from"
			err.Hint = "pinax prompt create --from <file> --vault <vault> --json"
		}
		return domain.NewErrorProjection(command, err), err
	}
	content, err := os.ReadFile(req.From)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_asset_source_unreadable", Message: "Prompt asset source cannot be read", Hint: "Check the --from file path and retry"}
		return domain.NewErrorProjection(command, err), err
	}
	asset, err := promptasset.Load(content)
	if err != nil {
		err := &domain.CommandError{Code: "prompt_asset_invalid", Message: err.Error(), Hint: "Provide a valid yeisme.prompt_asset.v1 YAML file"}
		return domain.NewErrorProjection(command, err), err
	}
	if err := promptasset.Validate(asset); err != nil {
		err := &domain.CommandError{Code: "prompt_asset_invalid", Message: err.Error(), Hint: "Fix required prompt asset fields and retry"}
		return domain.NewErrorProjection(command, err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	record, err := repo.Create(ctx, asset)
	if err != nil {
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Prompt asset imported.")
	if command == "prompt.create" {
		projection.Summary = "Prompt asset created."
	}
	promptAssetFacts(&projection, record)
	projection.Actions = []domain.Action{{Name: "resolve", Command: fmt.Sprintf("pinax prompt resolve pinax://prompt/%s --agent", shellQuote(record.PromptAssetID))}}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": record}
	return projection, nil
}

func (s *Service) promptDetails(ctx context.Context, req PromptAssetRequest, command string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if strings.TrimSpace(req.Ref) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: command + " requires a prompt asset id or URI", Hint: "pinax prompt show <id> --vault <vault> --json"}
		if command == "prompt.resolve" {
			err.Hint = "pinax prompt resolve pinax://prompt/<id> --vault <vault> --agent"
		}
		return domain.NewErrorProjection(command, err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	details, err := repo.Details(ctx, req.Ref)
	if err != nil {
		if promptasset.IsNotFound(err) {
			err := &domain.CommandError{Code: "prompt_asset_not_found", Message: "Prompt asset not found", Hint: fmt.Sprintf("pinax prompt search %s --vault %s --json", shellQuote(req.Ref), shellQuote(root))}
			return domain.NewErrorProjection(command, err), err
		}
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Prompt asset details read.")
	if command == "prompt.resolve" {
		projection.Summary = "Prompt asset resolved."
	}
	promptAssetFacts(&projection, details.Asset)
	projection.Actions = []domain.Action{{Name: "resolve", Command: fmt.Sprintf("pinax prompt resolve pinax://prompt/%s --agent", shellQuote(details.Asset.PromptAssetID))}}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"prompt_asset": details.Asset, "version": details.Version, "source_refs": details.SourceRefs}
	return projection, nil
}

func promptAssetFacts(projection *domain.Projection, asset noteindex.PromptAssetRecord) {
	projection.Facts["prompt_asset_id"] = asset.PromptAssetID
	projection.Facts["version"] = asset.CurrentVersionID
	projection.Facts["lifecycle"] = asset.Lifecycle
	projection.Facts["permission"] = asset.Permission
	projection.Facts["domain"] = asset.Domain
	if asset.OwnerProject != "" {
		projection.Facts["owner_project"] = asset.OwnerProject
	}
}

type promptFeedbackPayload struct {
	SchemaVersion      string   `json:"schema_version"`
	FeedbackID         string   `json:"feedback_id"`
	PromptAssetID      string   `json:"prompt_asset_id"`
	VersionID          string   `json:"version_id"`
	PromptTemplateHash string   `json:"prompt_template_hash"`
	ExternalRunRef     string   `json:"external_run_ref"`
	Decision           string   `json:"decision"`
	Reason             string   `json:"reason"`
	ArtifactRefs       []string `json:"artifact_refs"`
}

func (p promptFeedbackPayload) toFeedback() (promptasset.Feedback, *domain.CommandError) {
	if strings.TrimSpace(p.FeedbackID) == "" {
		return promptasset.Feedback{}, &domain.CommandError{Code: "prompt_feedback_invalid", Message: "feedback_id is required", Hint: "Add feedback_id to the feedback record"}
	}
	if strings.TrimSpace(p.PromptAssetID) == "" {
		return promptasset.Feedback{}, &domain.CommandError{Code: "prompt_feedback_invalid", Message: "prompt_asset_id is required", Hint: "Add prompt_asset_id to the feedback record"}
	}
	return promptasset.Feedback{FeedbackID: p.FeedbackID, PromptAssetID: p.PromptAssetID, VersionID: p.VersionID, PromptTemplateHash: p.PromptTemplateHash, ExternalRunRef: p.ExternalRunRef, Decision: p.Decision, Reason: p.Reason, ArtifactRefs: p.ArtifactRefs}, nil
}

func lifecycleFeedbackID(assetID, lifecycle, reason string) string {
	sum := sha1.Sum([]byte(assetID + "\x00" + lifecycle + "\x00" + reason))
	return "lifecycle_" + assetID + "_" + hex.EncodeToString(sum[:])[:12]
}

func (s *Service) AssetAdd(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.add", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("asset.add", err), err
	}
	if strings.TrimSpace(req.Source) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "asset add requires a source file", Hint: "pinax asset add <file> --vault <vault>"}
		return domain.NewErrorProjection("asset.add", err), err
	}
	asset, err := pinaxassets.Add(root, req.Source)
	if err != nil {
		return errorProjection("asset.add", err), err
	}
	_ = appendEvent(root, "asset.add", "success", map[string]string{"asset_path": asset.Path})
	projection := domain.NewProjection("asset.add", "Asset added to vault.")
	assetFacts(&projection, asset)
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax asset show %s --vault %s --json", shellQuote(asset.Filename), shellQuote(root))}}
	projection.Evidence = []string{asset.Path, filepath.ToSlash(filepath.Join(".pinax", "assets", "manifest.json"))}
	projection.Data = map[string]any{"asset": asset}
	return projection, nil
}

func (s *Service) AssetList(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.list", err), err
	}
	assets, status, err := noteindex.ListAssets(root)
	if err != nil {
		return errorProjection("asset.list", err), err
	}
	engine := "index"
	evidence := []string{status.Path}
	if status.Status != "fresh" || len(assets) == 0 {
		manifest, err := pinaxassets.Load(root)
		if err != nil {
			return errorProjection("asset.list", err), err
		}
		assets = manifest.Assets
		engine = "manifest"
		evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "assets", "manifest.json"))}
	}
	projection := domain.NewProjection("asset.list", "Asset list generated.")
	projection.Facts["assets"] = fmt.Sprint(len(assets))
	projection.Facts["engine"] = engine
	projection.Facts["index_status"] = status.Status
	for i, asset := range assets {
		prefix := fmt.Sprintf("asset.%d.", i+1)
		projection.Facts[prefix+"path"] = asset.Path
		projection.Facts[prefix+"media_type"] = asset.MediaType
	}
	projection.Evidence = evidence
	projection.Data = map[string]any{"assets": assets}
	return projection, nil
}
func (s *Service) AssetShow(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.show", err), err
	}
	asset, status, err := noteindex.FindAsset(root, req.Ref)
	engine := "index"
	evidence := []string{status.Path}
	if err != nil {
		asset, err = pinaxassets.Find(root, req.Ref)
		if err != nil {
			err := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("asset.show", err), err
		}
		engine = "manifest"
		evidence = []string{asset.Path, filepath.ToSlash(filepath.Join(".pinax", "assets", "manifest.json"))}
	}
	if req.PathStyle != "" || req.IncludePaths {
		contextNotePath, err := assetDisplayContextPath(root, req.ContextNote)
		if err != nil {
			return errorProjection("asset.show", err), err
		}
		display, err := pinaxassets.DisplayPath(pinaxassets.PathDisplayRequest{Root: root, AssetPath: asset.Path, ContextNotePath: contextNotePath, MediaType: asset.MediaType, Label: asset.Filename, Style: pinaxassets.PathDisplayStyle(req.PathStyle)})
		if err != nil {
			if commandErr, ok := err.(*domain.CommandError); ok {
				if commandErr.Code == "path_context_required" {
					commandErr.Hint = fmt.Sprintf("pinax asset show %s --path-style %s --context-note <note> --vault %s --json", shellQuote(req.Ref), shellQuote(req.PathStyle), shellQuote(root))
				}
				return domain.NewErrorProjection("asset.show", commandErr), err
			}
			return errorProjection("asset.show", err), err
		}
		asset.DisplayPath = display
	}
	projection := domain.NewProjection("asset.show", "Asset details read.")
	assetFacts(&projection, asset)
	projection.Facts["engine"] = engine
	projection.Facts["index_status"] = status.Status
	if req.PathStyle != "" {
		projection.Facts["path_style"] = req.PathStyle
	}
	if asset.DisplayPath != "" {
		projection.Facts["display_path"] = asset.DisplayPath
	}
	projection.Evidence = evidence
	projection.Data = map[string]any{"asset": asset}
	return projection, nil
}
func (s *Service) AssetLink(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.link", err), err
	}
	asset, status, err := noteindex.FindAsset(root, req.Ref)
	evidence := []string{status.Path}
	if err != nil {
		asset, err = pinaxassets.Find(root, req.Ref)
		if err != nil {
			err := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("asset.link", err), err
		}
		evidence = []string{asset.Path, filepath.ToSlash(filepath.Join(".pinax", "assets", "manifest.json"))}
	}
	noteRef := strings.TrimSpace(req.ContextNote)
	if noteRef == "" {
		err := &domain.CommandError{Code: "note_ref_required", Message: "asset link requires a target note", Hint: "Provide --note <note>"}
		return domain.NewErrorProjection("asset.link", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("asset.link", err), err
	}
	note, err := resolveNoteRef(notes, noteRef)
	if err != nil {
		return errorProjection("asset.link", err), err
	}
	operation := domain.PlanOperation{Kind: "asset_link", Path: note.Path, Target: asset.Path, Reason: "Adding a Markdown asset reference to the note body requires explicit apply", Status: "planned", Evidence: []string{asset.Path}}
	plan := domain.AssetOperationPlan{PlanID: assetOperationPlanID("link", asset.Path, note.Path), AssetID: asset.ID, Path: asset.Path, Operation: "link", Risk: "medium", RequiresSnapshot: true, Operations: []domain.PlanOperation{operation}}
	projection := domain.NewProjection("asset.link", "Asset link plan generated.")
	projection.Status = "partial"
	projection.Facts["writes"] = "false"
	projection.Facts["asset_path"] = asset.Path
	projection.Facts["note_path"] = note.Path
	projection.Facts["note_id"] = note.ID
	projection.Facts["operations"] = "1"
	projection.Facts["requires_snapshot"] = "true"
	projection.Facts["index_status"] = status.Status
	projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --json", shellQuote(root))}}
	projection.Evidence = append(evidence, note.Path)
	projection.Data = map[string]any{"plan": plan, "asset": asset, "note": note}
	return projection, nil
}

func applyAttachmentDisplayPaths(root, notePath, style string, attachments []domain.NoteAttachment) error {
	if style == "" {
		style = string(pinaxassets.PathStyleVaultRelative)
	}
	for i := range attachments {
		display, err := pinaxassets.DisplayPath(pinaxassets.PathDisplayRequest{Root: root, AssetPath: attachments[i].TargetPath, ContextNotePath: notePath, MediaType: attachments[i].MediaType, Style: pinaxassets.PathDisplayStyle(style)})
		if err != nil {
			return err
		}
		attachments[i].Path = attachments[i].TargetPath
		attachments[i].DisplayPath = display
	}
	return nil
}

func assetDisplayContextPath(root, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", nil
	}
	notes, err := scanNotes(root)
	if err != nil {
		return "", err
	}
	note, err := resolveNoteRef(notes, ref)
	if err != nil {
		return "", err
	}
	return note.Path, nil
}

func (s *Service) AssetVerify(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.verify", err), err
	}
	result, err := pinaxassets.Verify(root)
	if err != nil {
		return errorProjection("asset.verify", err), err
	}
	projection := domain.NewProjection("asset.verify", "Asset verification completed.")
	projection.Facts["verified"] = fmt.Sprint(result.Verified)
	projection.Facts["missing"] = fmt.Sprint(result.Missing)
	projection.Facts["changed"] = fmt.Sprint(result.Changed)
	projection.Facts["unmanaged"] = fmt.Sprint(result.Unmanaged)
	projection.Facts["orphan"] = fmt.Sprint(result.Orphan)
	projection.Facts["failed"] = fmt.Sprint(result.Failed)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "assets", "manifest.json"))}
	projection.Data = result
	return projection, nil
}

func (s *Service) AssetBacklinks(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.backlinks", err), err
	}
	asset, status, err := noteindex.FindAsset(root, req.Ref)
	if err != nil {
		err := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
		return domain.NewErrorProjection("asset.backlinks", err), err
	}
	links, _, err := noteindex.ListAssetLinks(root)
	if err != nil {
		return errorProjection("asset.backlinks", err), err
	}
	matched := make([]noteindex.AssetLinkRecord, 0)
	for _, link := range links {
		if link.AssetPath == asset.Path {
			matched = append(matched, link)
		}
	}
	projection := domain.NewProjection("asset.backlinks", "Asset backlinks listed.")
	projection.Facts["asset_path"] = asset.Path
	projection.Facts["linked_notes"] = fmt.Sprint(len(uniqueAssetLinkSources(matched)))
	projection.Facts["links"] = fmt.Sprint(len(matched))
	projection.Facts["index_status"] = status.Status
	projection.Evidence = []string{status.Path, asset.Path}
	projection.Data = map[string]any{"asset": asset, "links": matched}
	return projection, nil
}

func (s *Service) AssetOrphans(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.orphans", err), err
	}
	assets, status, err := noteindex.ListAssets(root)
	if err != nil {
		return errorProjection("asset.orphans", err), err
	}
	links, _, err := noteindex.ListAssetLinks(root)
	if err != nil {
		return errorProjection("asset.orphans", err), err
	}
	linked := map[string]bool{}
	for _, link := range links {
		if link.Status == "resolved" {
			linked[link.AssetPath] = true
		}
	}
	orphans := make([]domain.Asset, 0)
	visibleAssets := make([]domain.Asset, 0, len(assets))
	for _, asset := range assets {
		if isPinaxAuxiliaryContentPath(asset.Path) {
			continue
		}
		visibleAssets = append(visibleAssets, asset)
		if !linked[asset.Path] {
			orphans = append(orphans, asset)
		}
	}
	projection := domain.NewProjection("asset.orphans", "Orphan assets listed.")
	projection.Facts["assets"] = fmt.Sprint(len(visibleAssets))
	projection.Facts["orphan"] = fmt.Sprint(len(orphans))
	projection.Facts["index_status"] = status.Status
	projection.Actions = []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax asset repair --plan --vault %s --json", shellQuote(root))}}
	projection.Evidence = []string{status.Path}
	projection.Data = map[string]any{"assets": orphans}
	return projection, nil
}

func isPinaxAuxiliaryContentPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	return clean == ".gitignore" || clean == vaultignore.PinaxIgnoreName
}

func (s *Service) AssetMissing(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.missing", err), err
	}
	links, status, err := noteindex.ListAssetLinks(root)
	if err != nil {
		return errorProjection("asset.missing", err), err
	}
	missing := make([]noteindex.AssetLinkRecord, 0)
	for _, link := range links {
		if link.Status == "missing" {
			missing = append(missing, link)
		}
	}
	projection := domain.NewProjection("asset.missing", "Missing attachment references listed.")
	projection.Facts["missing"] = fmt.Sprint(len(missing))
	projection.Facts["index_status"] = status.Status
	projection.Actions = []domain.Action{{Name: "repair_plan", Command: fmt.Sprintf("pinax asset repair --plan --vault %s --json", shellQuote(root))}}
	projection.Evidence = []string{status.Path}
	projection.Data = map[string]any{"links": missing}
	return projection, nil
}

func (s *Service) AssetPreview(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("asset.preview", err), err
	}
	asset, status, err := noteindex.FindAsset(root, req.Ref)
	if err != nil {
		asset, err = pinaxassets.Find(root, req.Ref)
		if err != nil {
			err := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
			return domain.NewErrorProjection("asset.preview", err), err
		}
	}
	mode := strings.TrimSpace(req.PreviewAs)
	if mode == "" {
		mode = "markdown"
	}
	maxBytes := req.MaxPreviewBytes
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	entry := pinaxassets.EmbeddedAssetPreview{Path: asset.Path, MediaType: asset.MediaType, RenderMode: mode, Status: "placeholder"}
	body := fmt.Sprintf("> [!asset] %s (%s, placeholder)\n> pinax asset show %s --vault <vault> --json", asset.Path, asset.MediaType, asset.Filename)
	if assetPreviewReadable(asset.Path, mode) {
		content, truncated, readErr := readAssetPreviewBody(filepath.Join(root, filepath.FromSlash(asset.Path)), maxBytes)
		if readErr != nil {
			entry.Status = "missing"
			entry.Warning = "attachment_missing"
		} else {
			body = content
			entry.Status = "embedded"
			entry.ByteCount = len([]byte(content))
			entry.Truncated = truncated
		}
	}
	projection := domain.NewProjection("asset.preview", "Asset preview generated.")
	assetFacts(&projection, asset)
	projection.Facts["preview_as"] = mode
	projection.Facts["status"] = entry.Status
	projection.Facts["bytes"] = fmt.Sprint(entry.ByteCount)
	projection.Facts["truncated"] = fmt.Sprint(entry.Truncated)
	projection.Facts["index_status"] = status.Status
	projection.Evidence = []string{asset.Path}
	projection.Data = map[string]any{"asset": asset, "body": body, "embedded_asset": entry}
	return projection, nil
}

func assetPreviewReadable(path, mode string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if mode == "markdown" && ext == ".md" {
		return true
	}
	return ext == ".txt" || ext == ".text" || ext == ".log" || ext == ".csv" || ext == ".json" || ext == ".yaml" || ext == ".yml"
}

func readAssetPreviewBody(path string, maxBytes int) (string, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if len(b) > maxBytes {
		return string(b[:maxBytes]), true, nil
	}
	return string(b), false, nil
}

func (s *Service) AssetRepairPlan(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	missingProjection, err := s.AssetMissing(ctx, req)
	if err != nil {
		return errorProjection("asset.repair", err), err
	}
	orphanProjection, err := s.AssetOrphans(ctx, req)
	if err != nil {
		return errorProjection("asset.repair", err), err
	}
	missingLinks, _ := missingProjection.Data.(map[string]any)["links"].([]noteindex.AssetLinkRecord)
	orphanAssets, _ := orphanProjection.Data.(map[string]any)["assets"].([]domain.Asset)
	ops := make([]domain.PlanOperation, 0, len(missingLinks)+len(orphanAssets))
	for _, link := range missingLinks {
		ops = append(ops, domain.PlanOperation{Kind: "asset_missing", Path: link.AssetPath, Target: link.SourcePath, Reason: "attachment reference target is missing"})
	}
	for _, asset := range orphanAssets {
		ops = append(ops, domain.PlanOperation{Kind: "asset_orphan", Path: asset.Path, Reason: "asset has no resolved note references"})
	}
	projection := domain.NewProjection("asset.repair", "Asset repair plan generated.")
	projection.Status = "partial"
	projection.Facts["writes"] = "false"
	projection.Facts["missing"] = fmt.Sprint(len(missingLinks))
	projection.Facts["orphan"] = fmt.Sprint(len(orphanAssets))
	projection.Facts["operations"] = fmt.Sprint(len(ops))
	projection.Evidence = append(missingProjection.Evidence, orphanProjection.Evidence...)
	projection.Data = map[string]any{"plan": map[string]any{"writes": false, "operations": ops}}
	return projection, nil
}

func (s *Service) AssetMovePlan(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, asset, links, status, err := s.assetPlanInputs(req, "asset.move")
	if err != nil {
		return errorProjection("asset.move", err), err
	}
	target := filepath.ToSlash(strings.TrimSpace(req.Target))
	if target == "" {
		err := &domain.CommandError{Code: "asset_target_required", Message: "Missing target asset path", Hint: "pinax asset move <asset> <target> --plan --vault <vault> --json"}
		return domain.NewErrorProjection("asset.move", err), err
	}
	ops := []domain.PlanOperation{{Kind: "asset_move", Path: asset.Path, Target: target, Reason: "Moving an asset file requires a version snapshot and manual confirmation first", Status: "planned"}}
	for _, link := range links {
		ops = append(ops, domain.PlanOperation{Kind: "asset_reference_rewrite", Path: link.SourcePath, Target: target, Reason: "Asset references must be rewritten after moving the asset", Status: "planned", Evidence: assetLinkEvidence(link)})
	}
	plan := domain.AssetOperationPlan{PlanID: assetOperationPlanID("move", asset.Path, target), AssetID: asset.ID, Path: asset.Path, Operation: "move", Risk: assetPlanRisk(len(links)), RequiresSnapshot: true, Operations: ops}
	projection := domain.NewProjection("asset.move", "Asset move plan generated.")
	projection.Status = "partial"
	projection.Facts["writes"] = "false"
	projection.Facts["asset_path"] = asset.Path
	projection.Facts["target"] = target
	projection.Facts["linked_notes"] = fmt.Sprint(len(uniqueAssetLinkSources(links)))
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["requires_snapshot"] = "true"
	projection.Facts["risk"] = plan.Risk
	projection.Facts["operations"] = fmt.Sprint(len(ops))
	projection.Facts["index_status"] = status.Status
	projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --json", shellQuote(root))}, {Name: "apply", Command: fmt.Sprintf("pinax asset move %s %s --vault %s --yes --json", shellQuote(req.Ref), shellQuote(target), shellQuote(root))}}
	projection.Evidence = []string{status.Path, asset.Path}
	projection.Data = map[string]any{"plan": plan, "asset": asset, "links": links}
	return projection, nil
}

func (s *Service) AssetRemovePlan(_ context.Context, req AssetRequest) (domain.Projection, error) {
	root, asset, links, status, err := s.assetPlanInputs(req, "asset.remove")
	if err != nil {
		return errorProjection("asset.remove", err), err
	}
	linkedNotes := len(uniqueAssetLinkSources(links))
	shared := linkedNotes > 1
	deleteAllowed := !shared && linkedNotes == 0
	ops := make([]domain.PlanOperation, 0, len(links)+1)
	if deleteAllowed {
		ops = append(ops, domain.PlanOperation{Kind: "asset_delete", Path: asset.Path, Reason: "No note references found; asset file can be deleted after confirmation", Status: "planned"})
	} else {
		for _, link := range links {
			ops = append(ops, domain.PlanOperation{Kind: "asset_reference_review", Path: link.SourcePath, Target: asset.Path, Reason: "Asset is referenced by notes; manually confirm unlink or keep before deleting", Status: "manual_review", Evidence: assetLinkEvidence(link)})
		}
	}
	plan := domain.AssetOperationPlan{PlanID: assetOperationPlanID("remove", asset.Path, ""), AssetID: asset.ID, Path: asset.Path, Operation: "remove", Risk: assetPlanRisk(len(links)), RequiresSnapshot: true, Operations: ops}
	projection := domain.NewProjection("asset.remove", "Asset delete plan generated.")
	projection.Status = "partial"
	projection.Facts["writes"] = "false"
	projection.Facts["asset_path"] = asset.Path
	projection.Facts["linked_notes"] = fmt.Sprint(linkedNotes)
	projection.Facts["links"] = fmt.Sprint(len(links))
	projection.Facts["shared"] = fmt.Sprint(shared)
	projection.Facts["delete_allowed"] = fmt.Sprint(deleteAllowed)
	projection.Facts["requires_snapshot"] = "true"
	projection.Facts["risk"] = plan.Risk
	projection.Facts["operations"] = fmt.Sprint(len(ops))
	projection.Facts["index_status"] = status.Status
	projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --json", shellQuote(root))}, {Name: "apply", Command: fmt.Sprintf("pinax asset remove %s --vault %s --yes --json", shellQuote(req.Ref), shellQuote(root))}}
	projection.Evidence = []string{status.Path, asset.Path}
	projection.Data = map[string]any{"plan": plan, "asset": asset, "links": links}
	return projection, nil
}

func (s *Service) assetPlanInputs(req AssetRequest, command string) (string, domain.Asset, []noteindex.AssetLinkRecord, noteindex.Status, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", domain.Asset{}, nil, noteindex.Status{}, err
	}
	asset, status, err := noteindex.FindAsset(root, req.Ref)
	if err != nil {
		err := &domain.CommandError{Code: domain.ErrorCodeAssetNotFound, Message: "Asset not found", Hint: fmt.Sprintf("pinax asset list --vault %s --json", shellQuote(root))}
		return "", domain.Asset{}, nil, status, err
	}
	links, _, err := noteindex.ListAssetLinks(root)
	if err != nil {
		return "", domain.Asset{}, nil, status, err
	}
	matched := make([]noteindex.AssetLinkRecord, 0)
	for _, link := range links {
		if link.AssetPath == asset.Path {
			matched = append(matched, link)
		}
	}
	_ = command
	return root, asset, matched, status, nil
}

func assetLinkEvidence(link noteindex.AssetLinkRecord) []string {
	evidence := []string{"source=" + link.SourcePath, "raw=" + link.RawReference}
	if link.Line > 0 {
		evidence = append(evidence, "line="+fmt.Sprint(link.Line))
	}
	return evidence
}

func assetPlanRisk(linkCount int) string {
	if linkCount > 1 {
		return "high"
	}
	if linkCount == 1 {
		return "medium"
	}
	return "low"
}

func assetOperationPlanID(operation, path, target string) string {
	sum := sha256.Sum256([]byte(operation + "\x00" + path + "\x00" + target))
	return "asset-plan-" + hex.EncodeToString(sum[:8])
}

func uniqueAssetLinkSources(links []noteindex.AssetLinkRecord) map[string]bool {
	sources := map[string]bool{}
	for _, link := range links {
		sources[link.SourcePath] = true
	}
	return sources
}

func assetFacts(projection *domain.Projection, asset pinaxassets.Asset) {
	projection.Facts["asset_id"] = asset.ID
	projection.Facts["asset_path"] = asset.Path
	projection.Facts["filename"] = asset.Filename
	projection.Facts["media_type"] = asset.MediaType
	projection.Facts["size"] = fmt.Sprint(asset.Size)
	projection.Facts["sha256"] = asset.SHA256
	projection.Facts["managed_status"] = asset.ManagedStatus
}

func (s *Service) VersionStatus(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.status", err), err
	}
	status, err := s.versionBackend.Status(context.Background(), pinaxversion.StatusRequest{Root: root})
	if err != nil {
		return errorProjection("version.status", err), err
	}
	projection := domain.NewProjection("version.status", "Version backend status checked.")
	projection.Facts["version_backend"] = status.Backend
	projection.Facts["snapshot_supported"] = fmt.Sprint(status.Capabilities.SnapshotSupported)
	projection.Facts["changed_paths_supported"] = fmt.Sprint(status.Capabilities.ChangedPathsSupported)
	projection.Facts["read_at_revision_supported"] = fmt.Sprint(status.Capabilities.ReadAtRevision)
	projection.Facts["diff_supported"] = fmt.Sprint(status.Capabilities.DiffSupported)
	projection.Facts["worktree_state"] = status.WorktreeState
	if status.CurrentRevision != "" {
		projection.Facts["current_revision"] = status.CurrentRevision
	}
	if status.LastSnapshotID != "" {
		projection.Facts["last_snapshot_id"] = status.LastSnapshotID
	}
	if status.LastSnapshotAt != "" {
		projection.Facts["last_snapshot_at"] = status.LastSnapshotAt
	}
	projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}}
	projection.Data = status
	return projection, nil
}

func (s *Service) VersionBackends(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.backends", err), err
	}
	backends := pinaxversion.AvailableBackends()
	projection := domain.NewProjection("version.backends", "Version backends listed.")
	projection.Facts["active_backend"] = "local"
	projection.Facts["backends"] = fmt.Sprint(len(backends))
	for i, backend := range backends {
		prefix := fmt.Sprintf("backend.%d.", i+1)
		projection.Facts[prefix+"name"] = backend.Name
		projection.Facts[prefix+"active"] = fmt.Sprint(backend.Active)
		projection.Facts[prefix+"snapshot_supported"] = fmt.Sprint(backend.Capabilities.SnapshotSupported)
	}
	projection.Evidence = []string{root}
	projection.Data = map[string]any{"backends": backends}
	return projection, nil
}

func (s *Service) VersionSnapshot(ctx context.Context, req SnapshotRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.snapshot", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("version.snapshot", err), err
	}
	if strings.TrimSpace(req.Message) == "" {
		err := &domain.CommandError{Code: "message_required", Message: "version snapshot requires --message", Hint: "Rerun and provide --message"}
		return domain.NewErrorProjection("version.snapshot", err), err
	}
	snapshot, err := s.versionBackend.Snapshot(ctx, pinaxversion.SnapshotRequest{Root: root, Message: req.Message})
	if err != nil {
		return errorProjection("version.snapshot", err), err
	}
	_ = appendEvent(root, "version.snapshot", "success", map[string]string{"snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection("version.snapshot", "Version snapshot recorded.")
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["version_backend"] = snapshot.Backend
	projection.Facts["message"] = snapshot.Message
	projection.Facts["files"] = fmt.Sprint(snapshot.Files)
	projection.Facts["bytes"] = fmt.Sprint(snapshot.Bytes)
	projection.Facts["content_hash"] = snapshot.ContentHash
	projection.Evidence = snapshot.Evidence
	projection.Data = map[string]any{"snapshot": snapshot}
	return projection, nil
}

type SnapshotRequest struct {
	VaultPath string
	Message   string
}

type VersionChangedRequest struct {
	VaultPath     string
	SinceRevision string
}

type VersionShowRequest struct {
	VaultPath string
	Path      string
	Revision  string
}

type VersionRestorePlanRequest struct {
	VaultPath string
	Path      string
	Revision  string
}

type VersionHistoryRequest struct {
	VaultPath string
	Limit     int
}

type VersionDiffRequest struct {
	VaultPath      string
	BaseRevision   string
	TargetRevision string
}

func (s *Service) VersionChanged(ctx context.Context, req VersionChangedRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.changed", err), err
	}
	since := strings.TrimSpace(req.SinceRevision)
	if since == "" {
		err := &domain.CommandError{Code: "revision_required", Message: "version changed requires a since revision", Hint: "Provide --since <revision>"}
		return domain.NewErrorProjection("version.changed", err), err
	}
	changed, err := s.versionBackend.ChangedSince(ctx, pinaxversion.ChangedSinceRequest{Root: root, SinceRevision: since})
	if err != nil {
		return errorProjection("version.changed", err), err
	}
	projection := domain.NewProjection("version.changed", "Version changed paths read.")
	projection.Facts["since_revision"] = since
	projection.Facts["changed"] = fmt.Sprint(len(changed))
	projection.Data = map[string]any{"changed_paths": changed}
	return projection, nil
}

func (s *Service) VersionHistory(ctx context.Context, req VersionHistoryRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.history", err), err
	}
	status, err := s.versionBackend.Status(ctx, pinaxversion.StatusRequest{Root: root})
	if err != nil {
		return errorProjection("version.history", err), err
	}
	snapshots, err := loadVersionSnapshots(root, req.Limit)
	if err != nil {
		return errorProjection("version.history", err), err
	}
	projection := domain.NewProjection("version.history", "Version snapshot history read.")
	projection.Facts["version_backend"] = status.Backend
	projection.Facts["snapshots"] = fmt.Sprint(len(snapshots))
	if len(snapshots) > 0 {
		projection.Facts["latest_snapshot_id"] = snapshots[0].SnapshotID
	}
	projection.Data = map[string]any{"snapshots": snapshots}
	return projection, nil
}

func (s *Service) VersionDiff(ctx context.Context, req VersionDiffRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.diff", err), err
	}
	base := strings.TrimSpace(req.BaseRevision)
	target := strings.TrimSpace(req.TargetRevision)
	if base == "" || target == "" {
		err := &domain.CommandError{Code: "revision_required", Message: "version diff requires base and target revisions", Hint: "Provide --base <revision> --target <revision>"}
		return domain.NewErrorProjection("version.diff", err), err
	}
	diff, err := s.versionBackend.DiffSummary(ctx, pinaxversion.DiffSummaryRequest{Root: root, BaseRevision: base, TargetRevision: target})
	if err != nil {
		return errorProjection("version.diff", err), err
	}
	projection := domain.NewProjection("version.diff", "Version diff summary read.")
	projection.Facts["base_revision"] = diff.BaseRevision
	projection.Facts["target_revision"] = diff.TargetRevision
	projection.Facts["files_changed"] = fmt.Sprint(diff.FilesChanged)
	projection.Facts["additions"] = fmt.Sprint(diff.Additions)
	projection.Facts["deletions"] = fmt.Sprint(diff.Deletions)
	projection.Data = map[string]any{"diff": diff}
	return projection, nil
}

func (s *Service) VersionShow(ctx context.Context, req VersionShowRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.show", err), err
	}
	path, err := versionops.CleanObjectPath(req.Path)
	if err != nil {
		return errorProjection("version.show", err), err
	}
	revision := strings.TrimSpace(req.Revision)
	if revision == "" {
		err := &domain.CommandError{Code: "revision_required", Message: "version show requires a revision", Hint: "Provide --revision <revision>"}
		return domain.NewErrorProjection("version.show", err), err
	}
	file, err := s.versionBackend.ReadFile(ctx, pinaxversion.ReadFileRequest{Root: root, Path: path, Revision: revision})
	if err != nil {
		return errorProjection("version.show", err), err
	}
	projection := domain.NewProjection("version.show", "Historical file content read.")
	projection.Facts["path"] = file.Path
	projection.Facts["revision"] = file.Revision
	projection.Facts["version_backend"] = file.Backend
	projection.Facts["bytes"] = fmt.Sprint(file.SizeBytes)
	if file.ContentHash != "" {
		projection.Facts["content_hash"] = file.ContentHash
	}
	projection.Evidence = file.Evidence
	projection.Data = map[string]any{"file": file}
	return projection, nil
}

func (s *Service) VersionRestorePlan(ctx context.Context, req VersionRestorePlanRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.restore", err), err
	}
	resolverResult, resolveErr := s.ResolveVaultObjectForWrite(ctx, ResolverRequest{VaultPath: root, Query: req.Path, Scope: "all", Kind: "all"})
	if resolveErr != nil {
		return resolverWriteGuardErrorProjection("version.restore", resolverResult, resolveErr), resolveErr
	}
	path := ""
	if len(resolverResult.Candidates) == 1 {
		path = resolverResult.Candidates[0].Path
	} else {
		path, err = versionops.CleanObjectPath(req.Path)
		if err != nil {
			return errorProjection("version.restore", err), err
		}
	}
	revision := strings.TrimSpace(req.Revision)
	if revision == "" {
		err := &domain.CommandError{Code: "revision_required", Message: "version restore requires a revision", Hint: "Provide --revision <revision>"}
		return domain.NewErrorProjection("version.restore", err), err
	}
	// ReadFile 是首选历史内容来源；local backend 会从 Pinax content objects 读取，
	// legacy Git checkout 仅作为旧计划或手工 Git 内容库的兼容 fallback。
	file, fileErr := s.versionBackend.ReadFile(ctx, pinaxversion.ReadFileRequest{Root: root, Path: path, Revision: revision})
	fileEvidence := []string{}
	fileBackend := "local"
	fileContentHash := ""
	if fileErr == nil {
		fileEvidence = file.Evidence
		fileBackend = file.Backend
		fileContentHash = file.ContentHash
	}
	diff, diffErr := s.versionBackend.DiffSummary(ctx, pinaxversion.DiffSummaryRequest{Root: root, BaseRevision: "HEAD", TargetRevision: revision})
	filesChanged := 0
	if diffErr == nil {
		filesChanged = diff.FilesChanged
	}
	// 生成并持久化只读 restore plan，restore apply 据此把历史内容安全写回本地。
	// plan 记录 vault hash、revision、content hash 和可选 git HEAD commit，apply 时校验目标 vault 未漂移。
	now := time.Now().UTC()
	planID := "restore_" + now.Format("20060102T150405Z")
	vaultHash, hashErr := versionVaultHash(root)
	if hashErr != nil {
		return errorProjection("version.restore", hashErr), hashErr
	}
	snapshotID := latestVersionSnapshotID(root)
	gitCommit, gitErr := gitstore.HeadCommit(ctx, root)
	if gitErr != nil {
		return errorProjection("version.restore", gitErr), gitErr
	}
	// 既无 git 历史又读不到历史内容时，没有可恢复的真源，按 version_read_unavailable 报错。
	if gitCommit == "" && fileErr != nil {
		err := &domain.CommandError{Code: domain.ErrorCodeVersionReadUnavailable, Message: "version backend cannot read historical content for restore", Hint: "Take a git snapshot (pinax version snapshot) before generating a restore plan"}
		return domain.NewErrorProjection("version.restore", err), err
	}
	operation := domain.PlanOperation{Kind: "version_restore", Path: path, Reason: "Restore historical content via the version backend or legacy git checkout", Status: "planned", Evidence: fileEvidence}
	plan := domain.RestorePlan{
		SchemaVersion:  "pinax.restore_plan.v1",
		PlanID:         planID,
		CreatedAt:      now.Format(time.RFC3339),
		ExpiresAt:      now.Add(24 * time.Hour).Format(time.RFC3339),
		VaultRoot:      root,
		VaultHash:      vaultHash,
		Path:           path,
		Revision:       revision,
		GitCommit:      gitCommit,
		VersionBackend: fileBackend,
		SnapshotID:     snapshotID,
		ContentHash:    fileContentHash,
		Operation:      operation,
	}
	if err := saveRestorePlan(root, &plan); err != nil {
		return errorProjection("version.restore", err), err
	}
	projection := domain.NewProjection("version.restore", "Version restore plan generated.")
	projection.Facts["writes"] = "false"
	projection.Facts["operations"] = "1"
	projection.Facts["requires_snapshot"] = "true"
	projection.Facts["path"] = path
	projection.Facts["revision"] = revision
	projection.Facts["version_backend"] = fileBackend
	projection.Facts["files_changed"] = fmt.Sprint(filesChanged)
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["saved_path"] = plan.SavedPath
	if fileContentHash != "" {
		projection.Facts["content_hash"] = fileContentHash
	}
	if gitCommit != "" {
		projection.Facts["git_commit"] = gitCommit
	}
	projection.Actions = []domain.Action{
		{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before restore"))},
		{Name: "apply", Command: fmt.Sprintf("pinax version restore apply --vault %s --plan %s --yes", shellQuote(root), shellQuote(plan.PlanID))},
	}
	projection.Data = map[string]any{"operations": []domain.PlanOperation{operation}, "plan_id": plan.PlanID, "saved_path": plan.SavedPath, "files_changed": filesChanged}
	return projection, nil
}

// VersionRestoreApplyRequest drives version restore apply.
type VersionRestoreApplyRequest struct {
	VaultPath string
	PlanID    string
	Yes       bool
}

// VersionRestoreApply 消费已保存的 restore plan，把历史 revision 的文件内容写回本地
// Markdown。它复用 version backend 的 ReadFile 读取历史内容，只做本地写入：
// remote_write=false、local_write=true，绝不调用 provider/cloud/MCP 写面。
// 必须显式 --yes；plan 的 vault hash 与 revision 必须与当前 vault 一致。
func (s *Service) VersionRestoreApply(ctx context.Context, req VersionRestoreApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "version restore apply requires explicit approval", Hint: "Rerun with --yes after reviewing the restore plan"}
		return domain.NewErrorProjection("version.restore.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.restore.apply", err), err
	}
	plan, err := loadRestorePlan(root, req.PlanID)
	if err != nil {
		return errorProjection("version.restore.apply", err), err
	}
	// 校验目标 vault 与 plan 来源一致：vault hash 漂移说明 vault 已被改动，plan 失效。
	currentHash, hashErr := versionVaultHash(root)
	if hashErr != nil {
		return errorProjection("version.restore.apply", hashErr), hashErr
	}
	if plan.VaultHash != "" && currentHash != plan.VaultHash {
		err := &domain.CommandError{Code: "restore_plan_stale", Message: "vault changed since restore plan was generated", Hint: "Regenerate the restore plan with pinax version restore --plan before applying"}
		projection := domain.NewErrorProjection("version.restore.apply", err)
		projection.Data = map[string]any{"plan_id": plan.PlanID}
		return projection, err
	}
	restoredHash := plan.ContentHash
	restoredBackend := plan.VersionBackend
	if file, err := s.versionBackend.ReadFile(ctx, pinaxversion.ReadFileRequest{Root: root, Path: plan.Path, Revision: plan.Revision}); err == nil {
		path, joinErr := safeJoin(root, file.Path)
		if joinErr != nil {
			return errorProjection("version.restore.apply", joinErr), joinErr
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errorProjection("version.restore.apply", err), err
		}
		mode := os.FileMode(0o600)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(path, []byte(file.Content), mode); err != nil {
			return errorProjection("version.restore.apply", err), err
		}
		restoredHash = file.ContentHash
		restoredBackend = file.Backend
	} else {
		if gitErr := gitstore.RestorePathFromCommit(ctx, root, plan.GitCommit, plan.Path); gitErr != nil {
			failure, _ := writeReceipt(root, "restore", map[string]any{"plan_id": plan.PlanID, "path": plan.Path, "revision": plan.Revision, "status": "failed", "error": gitErr.Error(), "version_error": err.Error()})
			projection := errorProjection("version.restore.apply", gitErr)
			projection.Facts["receipt"] = failure
			return projection, gitErr
		}
	}
	receiptRel, err := writeReceipt(root, "restore", map[string]any{
		"plan_id":         plan.PlanID,
		"path":            plan.Path,
		"revision":        plan.Revision,
		"git_commit":      plan.GitCommit,
		"version_backend": restoredBackend,
		"content_hash":    restoredHash,
		"status":          "applied",
		"local_write":     true,
		"remote_write":    false,
	})
	if err != nil {
		return errorProjection("version.restore.apply", err), err
	}
	_ = appendEvent(root, "version.restore.apply", "success", map[string]string{"plan_id": plan.PlanID, "path": plan.Path, "revision": plan.Revision})
	projection := domain.NewProjection("version.restore.apply", "Version restore applied to local Markdown.")
	projection.Facts["local_write"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["path"] = plan.Path
	projection.Facts["revision"] = plan.Revision
	projection.Facts["version_backend"] = restoredBackend
	if restoredHash != "" {
		projection.Facts["content_hash"] = restoredHash
	}
	projection.Evidence = []string{receiptRel, filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	projection.Actions = []domain.Action{{Name: "history", Command: fmt.Sprintf("pinax version history --vault %s --json", shellQuote(root))}}
	projection.Data = map[string]any{"plan_id": plan.PlanID, "receipt": receiptRel, "path": plan.Path, "revision": plan.Revision}
	return projection, nil
}

func saveRestorePlan(root string, plan *domain.RestorePlan) error {
	dir, err := safeJoin(root, ".pinax/restore-plans")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "restore-plans", plan.PlanID+".json"))
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

func loadRestorePlan(root, planRef string) (domain.RestorePlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.RestorePlan{}, &domain.CommandError{Code: "plan_required", Message: "restore plan id cannot be empty", Hint: "Run pinax version restore --plan to generate a restore plan"}
	}
	rel := planRef
	if !strings.Contains(planRef, "/") && !strings.HasSuffix(planRef, ".json") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "restore-plans", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.RestorePlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.RestorePlan{}, &domain.CommandError{Code: "restore_plan_not_found", Message: "restore plan could not be loaded", Hint: "Run pinax version restore --plan to generate a fresh restore plan"}
	}
	var plan domain.RestorePlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.RestorePlan{}, err
	}
	if plan.SchemaVersion != "pinax.restore_plan.v1" {
		return domain.RestorePlan{}, &domain.CommandError{Code: "restore_plan_schema_invalid", Message: "restore plan schema is not supported", Hint: "Rerun pinax version restore --plan"}
	}
	return plan, nil
}

// versionVaultHash 返回 vault 当前内容指纹，用于 restore plan 时效校验。
// 它递归哈希 vault 下全部 Markdown 与 asset 文件路径+大小+mtime（排除 .pinax/.git），
// 足以检测 plan 生成后 vault 内容是否被改动。
func versionVaultHash(root string) (string, error) {
	h := sha1.New()
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
			if rel == ".pinax" || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		_, _ = h.Write([]byte(rel))
		_, _ = fmt.Fprintf(h, ":%d:%d\n", info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func latestVersionSnapshotID(root string) string {
	snapshots, err := loadVersionSnapshots(root, 1)
	if err != nil || len(snapshots) == 0 {
		return ""
	}
	return snapshots[0].SnapshotID
}

func loadVersionSnapshots(root string, limit int) ([]domain.VersionSnapshot, error) {
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
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if limit <= 0 || limit > len(names) {
		limit = len(names)
	}
	snapshots := make([]domain.VersionSnapshot, 0, limit)
	for _, name := range names[:limit] {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var snapshot domain.VersionSnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}
func (s *Service) GitSnapshot(ctx context.Context, req SnapshotRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("git.snapshot", err), err
	}
	if req.Message == "" {
		err := &domain.CommandError{Code: "message_required", Message: "Git snapshot requires --message", Hint: "Rerun and provide --message"}
		return domain.NewErrorProjection("git.snapshot", err), err
	}
	if err := gitstore.Snapshot(ctx, root, req.Message); err != nil {
		return errorProjection("git.snapshot", err), err
	}
	projection := domain.NewProjection("git.snapshot", "Git snapshot recorded.")
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
	projection := domain.NewProjection("template.init", "Built-in templates initialized.")
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
	projection := domain.NewProjection("template.list", "Template list read.")
	projection.Facts["templates"] = fmt.Sprint(len(templates))
	projection.Data = map[string]any{"templates": templates}
	return projection, nil
}

func (s *Service) ListTemplateCatalog(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.list", err), err
	}
	items := filterTemplateCatalog(templateCatalogItems(root), req.Pack, req.UseCase)
	projection := domain.NewProjection("template.list", "Template list read.")
	projection.Facts["templates"] = fmt.Sprint(len(items))
	if req.Pack != "" {
		projection.Facts["filter.pack"] = req.Pack
	}
	if req.UseCase != "" {
		projection.Facts["filter.use_case"] = req.UseCase
	}
	projection.Data = map[string]any{"templates": items}
	return projection, nil
}

func (s *Service) RecommendTemplate(_ context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.recommend", err), err
	}
	items := templateCatalogItems(root)
	primary := recommendTemplate(items, req.Intent)
	recommendations := workflowRecommendations(items, primary, req.Intent, root)
	projection := domain.NewProjection("template.recommend", "Template recommendations generated.")
	projection.Facts["intent"] = strings.TrimSpace(req.Intent)
	projection.Facts["primary"] = primary.Name
	projection.Facts["templates"] = fmt.Sprint(len(items))
	if primary.ScenarioID != "" {
		projection.Facts["scenario_id"] = primary.ScenarioID
	}
	if primary.Maturity != "" {
		projection.Facts["maturity"] = primary.Maturity
	}
	if primary.Pack.ID != "" {
		projection.Facts["pack"] = primary.Pack.ID
	}
	projection.Data = map[string]any{"primary": primary, "templates": items, "recommendations": recommendations}
	if len(recommendations) > 0 && recommendations[0].CreateCommand != "" {
		projection.Actions = []domain.Action{{Name: "use", Command: recommendations[0].CreateCommand}}
	} else {
		projection.Actions = []domain.Action{{Name: "preview", Command: fmt.Sprintf("pinax template preview %s --vault %s --json", shellQuote(primary.Name), shellQuote(root))}}
	}
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
	projection := domain.NewProjection("template.show", "Template read.")
	projection.Facts["template"] = req.Name
	projection.Data = map[string]any{"template": req.Name, "body": body}
	return projection, nil
}

func (s *Service) RenderTemplate(ctx context.Context, req TemplateRequest) (domain.Projection, error) {
	return s.renderTemplateProjection(ctx, req, "template.render", "Template rendered.")
}

func (s *Service) PreviewTemplate(ctx context.Context, req TemplateRequest) (domain.Projection, error) {
	return s.renderTemplateProjection(ctx, req, "template.preview", "Template preview generated.")
}

func (s *Service) renderTemplateProjection(ctx context.Context, req TemplateRequest, command, summary string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if req.Run != "" {
		args, run, err := loadTemplateRunArgs(root, req.Name, req.Run)
		if err != nil {
			return errorProjection(command, err), err
		}
		req = applyTemplateRunArgs(req, args)
		req.Run = run.Name
		if req.Run == "" {
			req.Run = run.RunID
		}
	}
	lazyIndex := command != "template.preview"
	body, err := s.renderTemplateBody(ctx, root, req, lazyIndex)
	if err != nil {
		projection := errorProjection(command, err)
		var commandErr *domain.CommandError
		if errors.As(err, &commandErr) && commandErr.Code == "template_index_required" {
			projection.Actions = []domain.Action{{Name: "index_rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
		} else if errors.As(err, &commandErr) && commandErr.Code == "template_variable_missing" {
			projection.Actions = []domain.Action{{Name: "rerun", Command: missingTemplateVariableCommand(root, req, command)}}
		}
		return projection, err
	}
	doc, _ := parseTemplateForProjection(root, req.Name)
	meta, source := templateProjectionMetadata(root, req.Name, doc.Metadata)
	projection := domain.NewProjection(command, summary)
	projection.Facts["template"] = req.Name
	projection.Facts["title"] = req.Title
	projection.Facts["bytes"] = fmt.Sprint(len(body))
	projection.Facts["read_only"] = fmt.Sprint(command == "template.preview")
	projection.Facts["writes"] = "false"
	if source != "" {
		projection.Facts["source"] = source
	}
	if meta.ScenarioID != "" {
		projection.Facts["scenario_id"] = meta.ScenarioID
	}
	if meta.Maturity != "" {
		projection.Facts["maturity"] = meta.Maturity
	}
	if meta.Lifecycle != "" {
		projection.Facts["lifecycle"] = meta.Lifecycle
	}
	if meta.Pack.ID != "" {
		projection.Facts["pack"] = meta.Pack.ID
	}
	tags := cleanTags(req.Tags)
	if len(tags) > 0 {
		projection.Facts["tags"] = strings.Join(tags, ",")
	}
	if doc.Engine != "" {
		projection.Facts["engine"] = doc.Engine
	}
	projection.Facts["query_count"] = "0"
	if len(doc.Metadata.Queries) > 0 {
		projection.Facts["query_count"] = fmt.Sprint(len(doc.Metadata.Queries))
	}
	if req.Run != "" {
		projection.Facts["run"] = req.Run
	}
	var savedRun *renderRunReceipt
	if req.SaveRun != "" {
		run, err := saveTemplateRenderRun(root, req, body)
		if err != nil {
			return errorProjection(command, err), err
		}
		projection.Facts["run_saved"] = "true"
		projection.Facts["run_id"] = run.RunID
		projection.Facts["run_name"] = run.Name
		savedRun = &run
	}
	missingVariables := missingTemplateVariables(meta, req.Vars)
	nextCommand := fmt.Sprintf("pinax note add %s --template %s --vault %s --json", shellQuote(defaultString(req.Title, meta.Title)), shellQuote(req.Name), shellQuote(root))
	projection.Data = map[string]any{"template": req.Name, "body": body, "engine": doc.Engine, "tags": tags, "render_run": savedRun, "workflow": meta, "variable_schema": meta.VariableSchema, "required_variables": requiredTemplateVariables(meta), "missing_variables": missingVariables, "output_policy": meta.OutputPolicy, "proof_gate": meta.ProofGate, "body_exposure": "preview_body", "write_impact": map[string]any{"writes": false, "target_policy": meta.OutputPolicy, "source": source}, "next_command": nextCommand}
	projection.Actions = append(projection.Actions, domain.Action{Name: "create", Command: nextCommand})
	return projection, nil
}

func (s *Service) InspectTemplate(ctx context.Context, req TemplateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.inspect", err), err
	}
	doc, err := parseTemplateForProjection(root, req.Name)
	if err != nil {
		return errorProjection("template.inspect", err), err
	}
	meta, source := templateProjectionMetadata(root, req.Name, doc.Metadata)
	issues := validateTemplateContent(doc.Body, TemplateRequest{Name: req.Name, Vars: req.Vars})
	projection := domain.NewProjection("template.inspect", "Template inspection completed.")
	projection.Facts["template"] = req.Name
	projection.Facts["engine"] = doc.Engine
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	if source != "" {
		projection.Facts["source"] = source
	}
	if meta.ScenarioID != "" {
		projection.Facts["scenario_id"] = meta.ScenarioID
	}
	if meta.TemplateKind != "" {
		projection.Facts["template_kind"] = meta.TemplateKind
	}
	if meta.Maturity != "" {
		projection.Facts["maturity"] = meta.Maturity
	}
	if meta.Lifecycle != "" {
		projection.Facts["lifecycle"] = meta.Lifecycle
	}
	if meta.Pack.ID != "" {
		projection.Facts["pack"] = meta.Pack.ID
	}
	if doc.Metadata.SchemaVersion != "" {
		projection.Facts["schema_version"] = doc.Metadata.SchemaVersion
		if doc.Metadata.Kind != "" {
			projection.Facts["kind"] = doc.Metadata.Kind
		}
		if doc.Metadata.Title != "" {
			projection.Facts["title"] = doc.Metadata.Title
		}
		if doc.Metadata.Output.PathPattern != "" {
			projection.Facts["path_pattern"] = doc.Metadata.Output.PathPattern
			if len(doc.Metadata.UseCases) > 0 {
				projection.Facts["use_cases"] = strings.Join(doc.Metadata.UseCases, ",")
			}
			if len(doc.Metadata.Aliases) > 0 {
				projection.Facts["aliases"] = strings.Join(doc.Metadata.Aliases, ",")
			}
			if doc.Metadata.Difficulty != "" {
				projection.Facts["difficulty"] = doc.Metadata.Difficulty
			}
			if doc.Metadata.Starter != nil {
				projection.Facts["starter"] = fmt.Sprint(*doc.Metadata.Starter)
			}
		}
		projection.Facts["refreshable"] = "false"
		if blocks, err := templateengine.InspectManagedBlocks(doc.Body); err == nil && len(blocks) > 0 {
			projection.Facts["refreshable"] = "true"
		}
		projection.Actions = templateInspectActions(root, req.Name, meta)
		projection.Facts["after_create_action_count"] = fmt.Sprint(len(projection.Actions))
		if blocks, err := templateengine.InspectManagedBlocks(doc.Body); err == nil {
			projection.Facts["managed_blocks"] = fmt.Sprint(len(blocks))
		}
	}
	queryExplain := map[string]domain.Projection{}
	if len(doc.Metadata.Queries) > 0 {
		projection.Facts["queries"] = fmt.Sprint(len(doc.Metadata.Queries))
		queryExplain = s.explainTemplateQueries(ctx, doc.Metadata.Queries)
	}
	renderRuns := []renderRunReceipt{}
	if req.Runs {
		runs, err := listTemplateRenderRuns(root, req.Name)
		if err != nil {
			return errorProjection("template.inspect", err), err
		}
		renderRuns = runs
		projection.Facts["runs"] = fmt.Sprint(len(runs))
	}
	projection.Data = map[string]any{"template": req.Name, "engine": doc.Engine, "metadata": meta, "workflow": meta, "source": source, "variable_schema": meta.VariableSchema, "output_policy": meta.OutputPolicy, "proof_gate": meta.ProofGate, "after_create_actions": meta.AfterCreateActions, "issues": issues, "query_explain": queryExplain, "render_runs": renderRuns}
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	return projection, nil
}

func templateInspectActions(root, name string, meta templateengine.Metadata) []domain.Action {
	switch meta.Kind {
	case "journal_template":
		period := strings.TrimPrefix(name, "journal.")
		if period == "" || period == name {
			period = "daily"
		}
		return []domain.Action{{Name: "create", Command: fmt.Sprintf("pinax journal %s show --template %s --vault %s --json", period, shellQuote(name), shellQuote(root))}}
	case "index_template":
		page := strings.TrimPrefix(name, "index.")
		if page == "" || page == name {
			page = "home"
		}
		return []domain.Action{{Name: "preview", Command: fmt.Sprintf("pinax index page preview %s --template %s --vault %s --json", shellQuote(page), shellQuote(name), shellQuote(root))}}
	case "note_template":
		title := strings.TrimSpace(meta.Title)
		if title == "" {
			title = "Untitled"
		}
		return []domain.Action{{Name: "create", Command: fmt.Sprintf("pinax note add %s --template %s --vault %s --json", shellQuote(title), shellQuote(name), shellQuote(root))}}
	default:
		return []domain.Action{{Name: "preview", Command: fmt.Sprintf("pinax template preview %s --vault %s --json", shellQuote(name), shellQuote(root))}}
	}
}

func templateMetadataActions(actions []templateengine.TemplateActionMetadata) []domain.Action {
	out := make([]domain.Action, 0, len(actions))
	for _, action := range actions {
		if strings.TrimSpace(action.Name) == "" || strings.TrimSpace(action.Command) == "" {
			continue
		}
		out = append(out, domain.Action{Name: action.Name, Command: action.Command})
	}
	return out
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
	body, err = templateBodyWithRequestedEngine(body, req.Engine)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return errorProjection("template.create", err), err
	}
	if _, err := os.Stat(path); err == nil && !req.Overwrite {
		err := &domain.CommandError{Code: "template_conflict", Message: "Template already exists", Hint: "Use --overwrite or choose another template name"}
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
	projection := domain.NewProjection("template.create", "Template created.")
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
	projection := domain.NewProjection("template.validate", "Template validation completed.")
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
		err := &domain.CommandError{Code: "approval_required", Message: "template delete requires --yes", Hint: "Add --yes after confirming"}
		return domain.NewErrorProjection("template.delete", err), err
	}
	if _, ok := builtInTemplates()[name]; ok {
		err := &domain.CommandError{Code: "builtin_template_protected", Message: "Built-in template is protected", Hint: "Copy it as a custom template before modifying or deleting"}
		return domain.NewErrorProjection("template.delete", err), err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return errorProjection("template.delete", err), err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err := &domain.CommandError{Code: "template_not_found", Message: "Template not found", Hint: "Run pinax template list to view templates"}
			return domain.NewErrorProjection("template.delete", err), err
		}
		return errorProjection("template.delete", err), err
	}
	_ = appendEvent(root, "template.delete", "success", map[string]string{"template": name})
	projection := domain.NewProjection("template.delete", "Template deleted.")
	projection.Facts["template"] = name
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	return projection, nil
}

func (s *Service) SyncIndex(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.sync", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	result, err := noteindex.Sync(root, notes)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	_ = appendEvent(root, "index.sync", "success", map[string]string{"created": fmt.Sprint(result.Created), "changed": fmt.Sprint(result.Changed), "moved": fmt.Sprint(result.Moved), "deleted": fmt.Sprint(result.Deleted)})
	projection := domain.NewProjection("index.sync", "Local index synced.")
	projection.Facts["created"] = fmt.Sprint(result.Created)
	projection.Facts["changed"] = fmt.Sprint(result.Changed)
	projection.Facts["moved"] = fmt.Sprint(result.Moved)
	projection.Facts["deleted"] = fmt.Sprint(result.Deleted)
	projection.Facts["restored"] = "0"
	projection.Facts["skipped"] = fmt.Sprint(result.Skipped)
	projection.Facts["candidates"] = "0"
	projection.Facts["failed"] = fmt.Sprint(result.Failed)
	projection.Facts["index_status"] = "fresh"
	projection.Data = result
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) IndexLookup(_ context.Context, req IndexLookupRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.lookup", err), err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "index lookup requires a query", Hint: "pinax index lookup <query> --vault <vault>"}
		return domain.NewErrorProjection("index.lookup", err), err
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "registered"
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "all"
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.lookup", err), err
	}
	status, _ := noteindex.Inspect(root, notes)
	candidates := []VaultObjectCandidate{}
	if scopeAllows(scope, "registered") && kindAllows(kind, "note") {
		for _, note := range notes {
			if fields, score := noteCandidateMatch(note, query); score > 0 {
				candidates = append(candidates, VaultObjectCandidate{ObjectKind: "note", Path: note.Path, Title: note.Title, NoteID: note.ID, ManagedStatus: "registered", MatchFields: fields, Score: score, IndexStatus: status.Status})
			}
		}
	}
	if scopeAllows(scope, "adoptable") && kindAllows(kind, "file") {
		files, err := adoptableMarkdownCandidates(root, notes, query, status.Status)
		if err != nil {
			return errorProjection("index.lookup", err), err
		}
		candidates = append(candidates, files...)
	}
	if scopeAllows(scope, "assets") && kindAllows(kind, "asset") {
		manifest, err := pinaxassets.Load(root)
		if err != nil {
			return errorProjection("index.lookup", err), err
		}
		for _, asset := range manifest.Assets {
			if fields, score := assetCandidateMatch(asset, query); score > 0 {
				candidates = append(candidates, VaultObjectCandidate{ObjectKind: "asset", Path: asset.Path, AssetID: asset.ID, ManagedStatus: asset.ManagedStatus, MatchFields: fields, Score: score, MediaType: asset.MediaType, IndexStatus: status.Status})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
	projection := domain.NewProjection("index.lookup", "Vault object lookup completed.")
	if len(candidates) > 1 {
		projection.Status = "partial"
	}
	projection.Facts["query"] = query
	projection.Facts["scope"] = scope
	projection.Facts["kind"] = kind
	projection.Facts["candidates"] = fmt.Sprint(len(candidates))
	projection.Facts["index_status"] = status.Status
	for i, candidate := range candidates {
		prefix := fmt.Sprintf("candidate.%d.", i+1)
		projection.Facts[prefix+"object_kind"] = candidate.ObjectKind
		projection.Facts[prefix+"path"] = candidate.Path
		projection.Facts[prefix+"managed_status"] = candidate.ManagedStatus
	}
	projection.Actions = []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root))}}
	projection.Evidence = []string{status.Path}
	projection.Data = map[string]any{"candidates": candidates}
	return projection, nil
}

func scopeAllows(scope, target string) bool {
	switch scope {
	case "all":
		return true
	case "registered_or_adoptable":
		return target == "registered" || target == "adoptable"
	default:
		return scope == target
	}
}

func kindAllows(kind, target string) bool {
	return kind == "" || kind == "all" || kind == target
}

func noteCandidateMatch(note domain.Note, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"note_id", note.ID, 100, 60}, {"path", note.Path, 95, 55}, {"filename", filepath.Base(note.Path), 90, 50}, {"stem", strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)), 90, 50}, {"title", note.Title, 85, 45}, {"journal_alias", journalNoteShellFriendlyAlias(note), 85, 45}}
	return matchFields(q, checks)
}

func assetCandidateMatch(asset pinaxassets.Asset, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"asset_id", asset.ID, 100, 60}, {"path", asset.Path, 95, 55}, {"filename", asset.Filename, 90, 50}, {"stem", asset.Stem, 90, 50}}
	return matchFields(q, checks)
}

func matchFields(q string, checks []struct {
	field    string
	value    string
	exact    int
	contains int
}) ([]string, int) {
	type fieldScore struct {
		field string
		score int
		order int
	}
	matchedFields := map[string]fieldScore{}
	score := 0
	for order, check := range checks {
		value := strings.ToLower(strings.TrimSpace(check.value))
		if value == "" {
			continue
		}
		matched := 0
		if value == q {
			matched = check.exact
		} else if strings.Contains(value, q) {
			matched = check.contains
		}
		if matched == 0 {
			continue
		}
		current, ok := matchedFields[check.field]
		if !ok || matched > current.score {
			matchedFields[check.field] = fieldScore{field: check.field, score: matched, order: order}
		}
		if matched > score {
			score = matched
		}
	}
	fields := make([]fieldScore, 0, len(matchedFields))
	for _, field := range matchedFields {
		fields = append(fields, field)
	}
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].score != fields[j].score {
			return fields[i].score > fields[j].score
		}
		return fields[i].order < fields[j].order
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.field)
	}
	return out, score
}

func adoptableMarkdownCandidates(root string, notes []domain.Note, query, indexStatus string) ([]VaultObjectCandidate, error) {
	registered := map[string]bool{}
	for _, note := range notes {
		registered[note.Path] = true
	}
	candidates := []VaultObjectCandidate{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".pinax") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(rel) != ".md" || registered[rel] {
			return nil
		}
		fields, score := fileCandidateMatch(rel, query)
		if score == 0 {
			return nil
		}
		candidates = append(candidates, VaultObjectCandidate{ObjectKind: "file", Path: rel, ManagedStatus: "adoptable", MatchFields: fields, Score: score, IndexStatus: indexStatus})
		return nil
	})
	return candidates, err
}

func fileCandidateMatch(path, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"path", path, 95, 55}, {"filename", filepath.Base(path), 90, 50}, {"stem", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), 90, 50}}
	return matchFields(q, checks)
}
func (s *Service) IndexRefresh(ctx context.Context, req IndexRefreshRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.refresh", err), err
	}
	rec := startMonitorRun(root, "index.refresh", map[string]string{"changed_since": strings.TrimSpace(req.ChangedSince)})
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	validNotes, failedPaths, err := scanIndexRefreshNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	scanned := len(validNotes) + len(failedPaths)
	changedSince := strings.TrimSpace(req.ChangedSince)
	changedCandidates := []pinaxversion.ChangedPath{}
	var result noteindex.RefreshResult
	if changedSince != "" {
		endStep = rec.BeginStep("version.changed_since", nil)
		changedCandidates, err = s.versionBackend.ChangedSince(ctx, pinaxversion.ChangedSinceRequest{Root: root, SinceRevision: changedSince})
		if err != nil {
			endStep(err)
			return errorProjection("index.refresh", err), err
		}
		endStep(nil)
		endStep = rec.BeginStep("index.refresh_changed", map[string]string{"changed_candidates": fmt.Sprint(len(changedCandidates))})
		result, err = noteindex.RefreshChanged(root, validNotes, changedCandidates, noteindex.RefreshOptions{})
	} else {
		endStep = rec.BeginStep("index.refresh", map[string]string{"notes": fmt.Sprint(len(validNotes))})
		result, err = noteindex.Refresh(root, validNotes, noteindex.RefreshOptions{})
		result.Scanned = scanned
	}
	if err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	result.Failed += len(failedPaths)
	result.FailedPaths = append(result.FailedPaths, failedPaths...)
	if result.Failed > 0 {
		result.IndexStatus = "partial"
	}
	status := "success"
	if result.IndexStatus == "partial" {
		status = "partial"
	}
	_ = appendEvent(root, "index.refresh", status, map[string]string{"scanned": fmt.Sprint(result.Scanned), "indexed": fmt.Sprint(result.Indexed), "failed": fmt.Sprint(result.Failed)})
	projection = domain.NewProjection("index.refresh", "Local index refresh completed.")
	projection.Status = status
	projection.Facts["scanned"] = fmt.Sprint(result.Scanned)
	projection.Facts["changed"] = fmt.Sprint(result.Changed)
	projection.Facts["skipped"] = fmt.Sprint(result.Skipped)
	projection.Facts["indexed"] = fmt.Sprint(result.Indexed)
	projection.Facts["created"] = fmt.Sprint(result.Created)
	projection.Facts["moved"] = fmt.Sprint(result.Moved)
	projection.Facts["deleted"] = fmt.Sprint(result.Deleted)
	projection.Facts["failed"] = fmt.Sprint(result.Failed)
	if changedSince != "" {
		projection.Facts["changed_since"] = changedSince
		projection.Facts["changed_candidates"] = fmt.Sprint(len(changedCandidates))
	}
	if len(result.FailedPaths) > 0 {
		projection.Facts["failed_paths"] = strings.Join(result.FailedPaths, ",")
	}
	projection.Facts["batches"] = fmt.Sprint(result.Batches)
	projection.Facts["duration_ms"] = fmt.Sprint(result.DurationMillis)
	projection.Facts["index_status"] = result.IndexStatus
	projection.Facts["schema_version"] = noteindex.SchemaVersion
	projection.Facts["path"] = filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	projection.Evidence = append([]string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}, result.FailedPaths...)
	if status == "partial" {
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", shellQuote(root))}, {Name: "rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
	}
	projection.Data = result
	return projection, nil
}

func refreshableIndexNotes(notes []domain.Note) ([]domain.Note, []string) {
	valid := make([]domain.Note, 0, len(notes))
	failedPaths := make([]string, 0)
	for _, note := range notes {
		if strings.TrimSpace(note.Path) == "" || strings.TrimSpace(note.ID) == "" {
			failedPaths = append(failedPaths, note.Path)
			continue
		}
		valid = append(valid, note)
	}
	return valid, failedPaths
}

func (s *Service) IndexRepair(_ context.Context, req IndexRepairRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.repair", err), err
	}
	rec := startMonitorRun(root, "index.repair", map[string]string{"kind": strings.TrimSpace(req.Kind), "dry_run": fmt.Sprint(req.DryRun)})
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "recreate"
	}
	if kind != "recreate" {
		err := &domain.CommandError{Code: "index_repair_kind_invalid", Message: "index repair kind is unsupported", Hint: "Use --kind recreate"}
		return domain.NewErrorProjection("index.repair", err), err
	}
	indexRel := filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	operation := map[string]string{"kind": kind, "mode": repairMode(req), "risk": "low", "path": indexRel, "reason": "recreate local projection"}
	if req.DryRun {
		projection = domain.NewProjection("index.repair", "Index repair plan generated.")
		projection.Facts["dry_run"] = "true"
		projection.Facts["writes"] = "false"
		projection.Facts["operations"] = "1"
		projection.Facts["kind"] = kind
		projection.Facts["risk.low"] = "1"
		projection.Evidence = []string{indexRel}
		projection.Data = map[string]any{"operations": []map[string]string{operation}}
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "index repair requires --yes or --dry-run", Hint: fmt.Sprintf("Run pinax index repair --vault %s --kind recreate --dry-run first, then add --yes after confirming", shellQuote(root))}
		return domain.NewErrorProjection("index.repair", err), err
	}
	endStep := rec.BeginStep("index.backup", nil)
	backupRel, err := backupIndexProjection(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	validNotes, _ := refreshableIndexNotes(notes)
	endStep = rec.BeginStep("index.rebuild", map[string]string{"notes": fmt.Sprint(len(validNotes))})
	counts, err := noteindex.Rebuild(root, validNotes)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	endStep(nil)
	_ = appendEvent(root, "index.repair", "success", map[string]string{"kind": kind, "writes": "true"})
	projection = domain.NewProjection("index.repair", "Index projection repaired.")
	projection.Facts["dry_run"] = "false"
	projection.Facts["writes"] = "true"
	projection.Facts["operations"] = "1"
	projection.Facts["kind"] = kind
	projection.Facts["risk.low"] = "1"
	projection.Facts["index_status"] = "fresh"
	projection.Facts["notes"] = fmt.Sprint(counts.Notes)
	projection.Facts["path"] = indexRel
	projection.Evidence = []string{indexRel, backupRel}
	projection.Data = map[string]any{"operations": []map[string]string{operation}, "backup_path": backupRel, "counts": counts}
	return projection, nil
}

func repairMode(req IndexRepairRequest) string {
	if req.DryRun || !req.Yes {
		return "preview"
	}
	return "apply"
}

func backupIndexProjection(root string) (string, error) {
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return filepath.ToSlash(filepath.Join(".pinax", "index.sqlite")), nil
		}
		return "", err
	}
	backupDir := filepath.Join(root, ".pinax", "index-backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	backupRel := filepath.ToSlash(filepath.Join(".pinax", "index-backups", "index-"+time.Now().UTC().Format("20060102T150405.000000000")+".sqlite"))
	backupPath := filepath.Join(root, filepath.FromSlash(backupRel))
	if err := os.Rename(indexPath, backupPath); err != nil {
		return "", err
	}
	return backupRel, nil
}

func (s *Service) RebuildIndex(_ context.Context, req VaultRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.rebuild", err), err
	}
	rec := startMonitorRun(root, "index.rebuild", nil)
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	endStep = rec.BeginStep("index.rebuild", map[string]string{"notes": fmt.Sprint(len(notes))})
	counts, err := noteindex.Rebuild(root, notes)
	if err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	endStep(nil)
	_ = appendEvent(root, "index.rebuild", "success", map[string]string{"notes": fmt.Sprint(counts.Notes)})
	projection = domain.NewProjection("index.rebuild", "Local index rebuilt.")
	projection.Facts["notes"] = fmt.Sprint(counts.Notes)
	projection.Facts["tags"] = fmt.Sprint(counts.Tags)
	projection.Facts["links"] = fmt.Sprint(counts.Links)
	projection.Facts["tokens"] = fmt.Sprint(counts.Tokens)
	projection.Facts["attachments"] = fmt.Sprint(counts.Attachments)
	projection.Facts["dimensions"] = fmt.Sprint(counts.Dimensions)
	projection.Facts["folders"] = fmt.Sprint(counts.Folders)
	projection.Facts["schema_version"] = noteindex.SchemaVersion
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"counts": counts}
	return projection, nil
}

func (s *Service) InitIndex(_ context.Context, req VaultRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.init", err), err
	}
	rec := startMonitorRun(root, "index.init", nil)
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.init", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("index.init", nil)
	status, err := noteindex.Init(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.init", err), err
	}
	endStep(nil)
	projection = domain.NewProjection("index.init", "Local index database initialized.")
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	projection.Facts["schema_version"] = status.SchemaVersion
	projection.Evidence = []string{status.Path}
	projection.Data = status
	return projection, nil
}

func (s *Service) IndexDoctor(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	notes = ordinaryNotes(notes)
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	issues := indexDoctorIssues(root, status)
	report := domain.VaultDoctorReport{VaultPath: root, Issues: issues, Counts: countIssuesBySeverity(issues), Stats: domain.VaultStats{VaultPath: root, NoteCount: len(notes), IndexStatus: status.Status, IndexPath: status.Path}}
	projection := domain.NewProjection("index.doctor", "Local index diagnostics completed.")
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["issues.total"] = fmt.Sprint(len(issues))
	if status.SchemaVersion != "" {
		projection.Facts["schema_version"] = status.SchemaVersion
	} else {
		projection.Facts["schema_version"] = noteindex.SchemaVersion
	}
	for severity, count := range report.Counts {
		projection.Facts["issues."+severity] = fmt.Sprint(count)
	}
	if len(issues) > 0 {
		projection.Status = "partial"
		projection.Facts["issue_codes"] = indexIssueCodes(issues)
		projection.Actions = nextActionsFromIssues(issues)
	}
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = report
	return projection, nil
}

func indexDoctorIssues(root string, status noteindex.Status) []domain.VaultIssue {
	switch status.Status {
	case "fresh":
		return nil
	case "missing":
		return []domain.VaultIssue{{Code: "index_missing", Severity: "warning", Path: status.Path, Message: "Local index missing", Evidence: append([]string{"index_status=missing"}, status.Evidence...), NextActions: []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", shellQuote(root))}}}}
	case "stale":
		return []domain.VaultIssue{{Code: "index_stale", Severity: "warning", Path: status.Path, Message: "Local index stale", Evidence: append([]string{"index_status=stale"}, status.Evidence...), NextActions: []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", shellQuote(root))}}}}
	case "unreadable":
		return []domain.VaultIssue{{Code: "index_unreadable", Severity: "error", Path: status.Path, Message: "Local index unreadable", Evidence: append([]string{"index_status=unreadable"}, status.Evidence...), NextActions: []domain.Action{{Name: "repair", Command: fmt.Sprintf("pinax index repair --vault %s --kind recreate --dry-run", shellQuote(root))}, {Name: "rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}}}
	default:
		return []domain.VaultIssue{{Code: "index_" + status.Status, Severity: "warning", Path: status.Path, Message: "Local index status needs review", Evidence: append([]string{"index_status=" + status.Status}, status.Evidence...), NextActions: []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", shellQuote(root))}}}}
	}
}

func indexIssueCodes(issues []domain.VaultIssue) string {
	codes := make([]string, 0, len(issues))
	seen := map[string]bool{}
	for _, issue := range issues {
		if issue.Code == "" || seen[issue.Code] {
			continue
		}
		seen[issue.Code] = true
		codes = append(codes, issue.Code)
	}
	sort.Strings(codes)
	return strings.Join(codes, ",")
}

func (s *Service) IndexSummary(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	action := recommendedIndexAction(root, status.Status)
	projection := domain.NewProjection("index.summary", indexSummaryText(status.Status))
	if status.Status != "fresh" {
		projection.Status = "partial"
	}
	if action.Command != "" {
		projection.Actions = []domain.Action{action}
		projection.Facts["recommended_action"] = action.Command
	}
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	schemaVersion := status.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = noteindex.SchemaVersion
	}
	projection.Facts["schema_version"] = schemaVersion
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["writes"] = "false"
	projection.Facts["affected_workflows"] = "search,query,note_list,organize"
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = status
	return projection, nil
}

func (s *Service) IndexExplain(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.IndexSummary(ctx, req)
	if err != nil {
		return errorProjection("index.explain", err), err
	}
	projection.Command = "index.explain"
	projection.Summary = "Local index explanation generated."
	projection.Facts["explains"] = "index projection status"
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
	notes = ordinaryNotes(notes)
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	projection := domain.NewProjection("index.status", "Local index status checked.")
	if status.Status != "fresh" {
		projection.Status = "partial"
		action := recommendedIndexAction(root, status.Status)
		if action.Command != "" {
			projection.Actions = []domain.Action{action}
		}
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

func indexSummaryText(status string) string {
	switch status {
	case "fresh":
		return "Local index is available. Recommended next step: continue searching or querying."
	case "missing", "stale":
		return "Local index needs maintenance. Recommended next step: run low-cost refresh."
	case "unreadable":
		return "Local index cannot be read. Recommended next step: run doctor or repair dry-run first."
	default:
		return "Local index status summarized. See recommended next steps below."
	}
}

func recommendedIndexAction(root, status string) domain.Action {
	quotedRoot := shellQuote(root)
	switch status {
	case "fresh":
		return domain.Action{Name: "search", Command: fmt.Sprintf("pinax search <query> --vault %s", quotedRoot)}
	case "missing", "stale":
		return domain.Action{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", quotedRoot)}
	case "unreadable":
		return domain.Action{Name: "repair", Command: fmt.Sprintf("pinax index repair --vault %s --kind recreate --dry-run", quotedRoot)}
	default:
		return domain.Action{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", quotedRoot)}
	}
}

func refreshIndex(root string) error {
	notes, err := scanNotes(root)
	if err != nil {
		return err
	}
	notes = ordinaryNotes(notes)
	_, err = noteindex.Rebuild(root, notes)
	return err
}

func appendDailyIndex(root string, note domain.Note) (string, error) {
	date := currentTimeUTC().Format("2006-01-02")
	root, rel, _, err := ensureJournalNote(root, "daily", DailyRequest{Date: date})
	if err != nil {
		return "", err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return rel, err
	}
	content := string(contentBytes)
	blocks, err := templateengine.InspectManagedBlocks(content)
	if err != nil {
		return rel, err
	}
	capture := ""
	for _, block := range blocks {
		if block.Name == "daily-captures" {
			capture = content[block.ContentStart:block.ContentEnd]
			break
		}
	}
	if capture == "" {
		return rel, &templateengine.Error{Code: "managed_block_missing", Message: "daily-captures managed block is missing"}
	}
	if strings.Contains(capture, note.Path) {
		return rel, nil
	}
	line := strings.TrimSpace(dailyIndexLine(note))
	replacement := line
	if existing := strings.TrimSpace(capture); existing != "" {
		replacement = existing + "\n" + line
	}
	updated, err := templateengine.ReplaceManagedBlock(content, "daily-captures", replacement)
	if err != nil {
		return rel, err
	}
	// 缺失、重复或未闭合托管区块时上面已经 fail closed；这里只写回明确的 daily-captures 区块内容。
	return rel, os.WriteFile(path, []byte(updated), 0o644)
}

//nolint:unused // Reserved for daily-specific callers that do not need the generic period parameter.
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
	templateName := journalTemplateName(period, req)
	rel, body, err := journalTemplateRender(root, templateName, period, key)
	if err != nil {
		return "", "", "", err
	}
	if rel == "" {
		rel = filepath.ToSlash(filepath.Join(period, key+".md"))
	}
	for _, candidate := range []string{rel, filepath.ToSlash(filepath.Join("notes", period, key+".md"))} {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", "", "", err
		}
		exists, err := existingJournalNoteCandidate(path, candidate)
		if err != nil {
			return "", "", "", err
		}
		if exists {
			return root, candidate, key, nil
		}
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := journalTitle(period, key)
	content := buildNoteContentWithStatus(title, rel, "", period, period, []string{period}, "journal", now, body)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", "", "", err
	}
	if err := refreshIndex(root); err != nil {
		return "", "", "", err
	}
	_ = appendEvent(root, period+".create", "success", map[string]string{"path": rel, "template": templateName})
	return root, rel, key, nil
}

func existingJournalNoteCandidate(path, rel string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	// legacy `notes/daily/*` 只有真正的 journal note 才复用；旧 daily index 是系统导航页，不能阻止根目录 daily note 创建。
	if strings.HasPrefix(filepath.ToSlash(rel), "notes/daily/") {
		content, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		meta, _ := splitFrontmatter(string(content))
		if isPinaxNoteFrontmatter(meta) && isSystemIndexNote(parseNote(rel, string(content))) {
			return false, nil
		}
	}
	return true, nil
}

func journalTemplateName(period string, req DailyRequest) string {
	if name := strings.TrimSpace(req.Template); name != "" {
		return name
	}
	return "journal." + period
}

func journalTemplateRender(root, templateName, period, key string) (string, string, error) {
	body, err := loadTemplate(root, templateName)
	if err != nil {
		return "", "", err
	}
	doc, err := templateengine.ParseDocument(templateName, body)
	if err != nil {
		return "", "", templateEngineCommandError(err)
	}
	rel := journalPathFromPattern(doc.Metadata.Output.PathPattern, period, key)
	title := journalTitle(period, key)
	rendered, err := templateengine.New().Render(doc, templateengine.Context{Title: title, Date: key, Vars: map[string]string{"date": key}})
	if err != nil {
		return "", "", templateEngineCommandError(err)
	}
	return rel, rendered.Body, nil
}

func journalPathFromPattern(pattern, period, key string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return filepath.ToSlash(filepath.Join(period, key+".md"))
	}
	for _, token := range []string{"{{ .Date }}", "{{.Date}}", "{{ .Week }}", "{{.Week}}", "{{ .Month }}", "{{.Month}}"} {
		pattern = strings.ReplaceAll(pattern, token, key)
	}
	return filepath.ToSlash(pattern)
}

func journalDate(period string, req DailyRequest) (time.Time, error) {
	date := time.Now().UTC()
	if value := strings.TrimSpace(req.Date); value != "" {
		parsed, err := parseJournalDateValue(period, value)
		if err != nil {
			return time.Time{}, &domain.CommandError{Code: "invalid_journal_date", Message: "journal date must be YYYY-MM-DD, YYYY-Www, or YYYY-MM", Hint: "Use --date 2026-06-06, --date 2026-W23, or --date 2026-06"}
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
	if period == "daily" {
		return "Daily-" + key
	}
	return journalLabel(period) + " " + key
}

func journalNoteShellFriendlyAlias(note domain.Note) string {
	path := filepath.ToSlash(strings.TrimPrefix(note.Path, "notes/"))
	if !strings.HasPrefix(path, "daily/") || filepath.Ext(path) != ".md" {
		return ""
	}
	key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if _, err := time.Parse("2006-01-02", key); err != nil {
		return ""
	}
	if note.Title == "Daily "+key || note.Title == "Daily-"+key {
		return "Daily-" + key
	}
	return ""
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
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func dailyIndexLine(note domain.Note) string {
	parts := []string{"- " + note.Path}
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

func (s *Service) DeliverFeishu(ctx context.Context, req FeishuDeliveryRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("briefing.deliver.feishu", err), err
	}
	receipt, err := delivery.DeliverFeishu(ctx, root, delivery.FeishuRequest{WebhookURL: req.WebhookURL, SecretRef: req.SecretRef, Title: req.Title, Text: req.Text, DryRun: req.DryRun, Yes: req.Yes})
	if err != nil {
		return errorProjection("briefing.deliver.feishu", err), err
	}
	projection := domain.NewProjection("briefing.deliver.feishu", "Feishu briefing delivery generated.")
	projection.Facts["provider"] = "feishu"
	projection.Facts["status"] = receipt.Status
	projection.Facts["remote_write"] = fmt.Sprint(receipt.RemoteWrite)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Data = map[string]any{"receipt": receipt}
	return projection, nil
}

func (s *Service) BriefingRun(_ context.Context, req BriefingRunRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("briefing.run", err), err
	}
	if !req.DryRun && !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "briefing run requires --yes to write candidate notes", Hint: "Review candidates first with pinax briefing run --dry-run --vault <vault> --json"}
		return domain.NewErrorProjection("briefing.run", err), err
	}
	recipe, err := briefing.LoadRecipe(root)
	if err != nil {
		return errorProjection("briefing.run", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("briefing.run", err), err
	}
	vaultTexts := make([]string, 0, len(notes))
	for _, note := range notes {
		vaultTexts = append(vaultTexts, note.Title+"\n"+note.Body)
	}
	ledger := briefing.BuildEvidenceLedger(briefing.FakeEvidence(recipe))
	scores := briefing.ScoreEvidence(recipe, ledger, vaultTexts)
	backlinks := briefingBacklinks(notes)
	queue, candidates := briefing.BuildCandidateNotes(recipe, scores, backlinks)
	if req.Yes && !req.DryRun {
		if err := writeBriefingCandidates(root, queue, candidates); err != nil {
			return errorProjection("briefing.run", err), err
		}
		_ = appendEvent(root, "briefing.run", "success", map[string]string{"candidates": fmt.Sprint(len(candidates)), "writes": "true"})
	}
	projection := domain.NewProjection("briefing.run", "Briefing candidates generated.")
	if req.DryRun {
		projection.Summary = "Briefing dry-run generated."
	}
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["candidates"] = fmt.Sprint(len(scores))
	projection.Facts["topic"] = recipe.Topic
	projection.Facts["writes"] = fmt.Sprint(req.Yes && !req.DryRun)
	projection.Data = map[string]any{"recipe": recipe, "candidates": scores, "review_queue": queue, "dry_run": req.DryRun}
	projection.Actions = []domain.Action{{Name: "write_candidates", Command: fmt.Sprintf("pinax briefing run --vault %s --yes", shellQuote(root))}}
	return projection, nil
}

func briefingBacklinks(notes []domain.Note) []string {
	out := make([]string, 0, len(notes))
	for _, note := range notes {
		if strings.TrimSpace(note.Title) != "" {
			out = append(out, note.Title)
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func writeBriefingCandidates(root string, queue briefing.ReviewQueue, candidates []briefing.GeneratedCandidate) error {
	for _, candidate := range candidates {
		path, err := safeJoin(root, candidate.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(candidate.Body), 0o644); err != nil {
			return err
		}
	}
	return writeJSONAsset(filepath.Join(root, ".pinax", "briefing", "review-queue.json"), queue)
}

func (s *Service) BriefingRecipeInit(_ context.Context, req BriefingRecipeRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("briefing.recipe.init", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("briefing.recipe.init", err), err
	}
	recipe, err := briefing.InitRecipe(root, briefing.InitRecipeRequest{Topic: req.Topic, Limit: req.Limit})
	if err != nil {
		return errorProjection("briefing.recipe.init", err), err
	}
	projection := briefingops.RecipeProjection("briefing.recipe.init", "Briefing recipe created.", root, recipe)
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax briefing recipe show --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) BriefingRecipeShow(_ context.Context, req BriefingRecipeRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("briefing.recipe.show", err), err
	}
	recipe, err := briefing.LoadRecipe(root)
	if err != nil {
		return errorProjection("briefing.recipe.show", err), err
	}
	projection := briefingops.RecipeProjection("briefing.recipe.show", "Briefing recipe read.", root, recipe)
	projection.Actions = []domain.Action{{Name: "set", Command: fmt.Sprintf("pinax briefing recipe set --vault %s --topic <topic>", shellQuote(root))}}
	return projection, nil
}

func (s *Service) BriefingRecipeSet(_ context.Context, req BriefingRecipeRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("briefing.recipe.set", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("briefing.recipe.set", err), err
	}
	recipe, err := briefing.SetRecipe(root, briefing.RecipePatch{Topic: req.Topic, Limit: req.Limit, AddSource: req.Source})
	if err != nil {
		return errorProjection("briefing.recipe.set", err), err
	}
	projection := briefingops.RecipeProjection("briefing.recipe.set", "Briefing recipe updated.", root, recipe)
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax briefing recipe show --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudBackendSetS3(_ context.Context, req CloudBackendSetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	bucket := strings.TrimSpace(req.Bucket)
	region := strings.TrimSpace(req.Region)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	deviceID := strings.TrimSpace(req.DeviceID)
	if bucket == "" || region == "" || workspaceID == "" || deviceID == "" {
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "s3 cloud backend configuration is incomplete", Hint: "Provide --bucket, --region, --workspace, and --device"}
		return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
	}
	prefix := strings.Trim(strings.TrimSpace(req.Prefix), "/")
	endpoint := buildS3CloudEndpoint(bucket, prefix, strings.TrimSpace(req.Endpoint), region, strings.TrimSpace(req.Profile))
	secretRef := strings.TrimSpace(req.SecretRef)
	if secretRef == "" && strings.TrimSpace(req.Profile) != "" {
		secretRef = "profile://" + strings.TrimSpace(req.Profile)
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: deviceID, SecretRef: secretRef, BackendKind: "s3-direct", S3: &pinaxcloud.S3Config{Bucket: bucket, Prefix: prefix, Endpoint: strings.TrimSpace(req.Endpoint), Region: region, Profile: strings.TrimSpace(req.Profile), PathStyle: strings.TrimSpace(req.Endpoint) != ""}})
	if err != nil {
		projection, commandErr := cloudBackendSetErrorProjection(err)
		return projection, commandErr
	}
	projection := domain.NewProjection("cloud.backend.set", "S3 direct cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Facts["backend_kind"] = "s3-direct"
	projection.Facts["bucket"] = bucket
	projection.Facts["region"] = region
	if prefix != "" {
		projection.Facts["prefix"] = prefix
	}
	if strings.TrimSpace(req.Profile) != "" {
		projection.Facts["credential_source"] = "profile"
	}
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func cloudBackendSetErrorProjection(err error) (domain.Projection, error) {
	msg := err.Error()
	commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: msg, Hint: "Use a supported cloud backend such as server, s3, or rclone"}
	return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
}

func (s *Service) CloudBackendSetRclone(_ context.Context, req CloudBackendSetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	remoteName := strings.TrimSpace(req.Remote)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	deviceID := strings.TrimSpace(req.DeviceID)
	if remoteName == "" || workspaceID == "" || deviceID == "" {
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "rclone cloud backend configuration is incomplete", Hint: "Provide --remote, --workspace, and --device"}
		return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
	}
	endpoint := rcloneEndpoint(remoteName)
	secretRef := strings.TrimSpace(req.SecretRef)
	if secretRef == "" {
		secretRef = "rclone://" + remoteName
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: deviceID, SecretRef: secretRef, BackendKind: "rclone-direct"})
	if err != nil {
		projection, commandErr := cloudBackendSetErrorProjection(err)
		return projection, commandErr
	}
	projection := domain.NewProjection("cloud.backend.set", "Rclone direct cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Facts["backend_kind"] = "rclone-direct"
	projection.Facts["remote"] = remoteName
	projection.Facts["credential_source"] = "rclone"
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func rcloneEndpoint(remoteName string) string {
	remoteName = strings.TrimSpace(remoteName)
	name, rest, ok := strings.Cut(remoteName, ":")
	if !ok {
		return "rclone://" + strings.Trim(remoteName, "/")
	}
	return "rclone://" + strings.Trim(name, "/") + "/" + strings.Trim(rest, "/")
}

func buildS3CloudEndpoint(bucket, prefix, endpointURL, region, profile string) string {
	endpoint := "s3://" + bucket
	if prefix != "" {
		endpoint += "/" + prefix
	}
	values := url.Values{}
	if endpointURL != "" {
		values.Set("endpoint", endpointURL)
		values.Set("path_style", "true")
	}
	if region != "" {
		values.Set("region", region)
	}
	if profile != "" {
		values.Set("profile", profile)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

func (s *Service) CloudLogin(_ context.Context, req CloudLoginRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.login", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.login", err), err
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: req.Endpoint, WorkspaceID: req.WorkspaceID, DeviceID: req.DeviceID, SecretRef: req.SecretRef, EncryptionSecretRef: req.EncryptionSecretRef})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unsupported remote scheme") || strings.Contains(msg, "invalid endpoint URI") || strings.Contains(msg, "endpoint URI must specify a scheme") {
			commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: msg, Hint: "Use a supported scheme, such as s3:// or file://"}
			return domain.NewErrorProjection("cloud.login", commandErr), commandErr
		}
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "cloud login configuration is incomplete", Hint: "Provide --endpoint, --workspace, --device, and --secret-ref"}
		return domain.NewErrorProjection("cloud.login", commandErr), commandErr
	}
	projection := domain.NewProjection("cloud.login", "Cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax cloud status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudStatus(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.status", err), err
	}
	state, err := pinaxcloud.Load(root)
	if err != nil {
		return cloudStateErrorProjection("cloud.status", root, err)
	}
	projection := domain.NewProjection("cloud.status", "Cloud backend status read.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudLogout(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.logout", err), err
	}
	if err := pinaxcloud.Logout(root); err != nil {
		return cloudStateErrorProjection("cloud.logout", root, err)
	}
	state, err := pinaxcloud.Load(root)
	if err != nil {
		return errorProjection("cloud.logout", err), err
	}
	projection := domain.NewProjection("cloud.logout", "Cloud device session logged out.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "login", Command: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudDoctor(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.doctor", err), err
	}
	result := pinaxcloud.Doctor(root)
	if !result.Configured {
		commandErr := &domain.CommandError{Code: result.Code, Message: result.Message, Hint: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}
		return domain.NewErrorProjection("cloud.doctor", commandErr), commandErr
	}
	projection := domain.NewProjection("cloud.doctor", "Cloud backend diagnostics passed.")
	projection.Facts["configured"] = "true"
	projection.Facts["backend_kind"] = result.BackendKind
	projection.Facts["auth_boundary"] = result.AuthBoundary
	projection.Facts["server_audit"] = fmt.Sprint(result.ServerAudit)
	projection.Facts["endpoint"] = result.Endpoint
	projection.Facts["workspace_id"] = result.Workspace
	projection.Facts["device_id"] = result.DeviceID
	projection.Facts["secret_ref_configured"] = "true"
	projection.Data = result
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax cloud status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func addCloudStateFacts(projection *domain.Projection, state pinaxcloud.State) {
	projection.Facts["configured"] = "true"
	projection.Facts["backend_kind"] = state.Config.BackendKind
	if projection.Facts["backend_kind"] == "" {
		projection.Facts["backend_kind"] = "server"
	}
	projection.Facts["endpoint"] = state.Config.Endpoint
	projection.Facts["workspace_id"] = state.Config.WorkspaceID
	projection.Facts["device_id"] = state.Config.DeviceID
	projection.Facts["session_status"] = state.Session.Status
	projection.Facts["secret_ref_configured"] = fmt.Sprint(strings.TrimSpace(state.Config.SecretRef) != "")
	projection.Facts["encryption_secret_ref_configured"] = fmt.Sprint(strings.TrimSpace(pinaxcloud.EncryptionSecretRef(state.Config)) != "")
}

func cloudStateErrorProjection(command, root string, err error) (domain.Projection, error) {
	if pinaxcloud.IsNotConfigured(err) {
		commandErr := &domain.CommandError{Code: "cloud_not_configured", Message: "cloud backend is not configured", Hint: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}
		return domain.NewErrorProjection(command, commandErr), commandErr
	}
	return errorProjection(command, err), err
}

func (s *Service) SyncDiff(ctx context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.diff", err), err
	}
	if target == "cloud" {
		projection, cloudErr := buildCloudSyncProjection(ctx, "sync.diff", root, req, syncplan.DirectionDiff)
		if cloudErr != nil {
			if pinaxcloud.IsNotConfigured(cloudErr) || isCommandErrorCode(cloudErr, "cloud_not_configured") {
				return cloudSyncNotConfiguredProjection(root), nil
			}
			return projection, cloudErr
		}
		return projection, nil
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("sync.diff", err), err
	}
	profile, _ := loadStorageProfile(root)
	projection := domain.NewProjection("sync.diff", "Sync plan generated.")
	projection.Facts["target"] = target
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["backend_required"] = "false"
	plan := syncPlanData(target, profile)
	projection.Data = map[string]any{"target": target, "plan": plan, "remote_write": false}
	projection.Actions = []domain.Action{{Name: "push", Command: fmt.Sprintf("pinax sync push --target %s --vault %s --yes", target, shellQuote(root))}}
	return projection, nil
}

func (s *Service) SyncPush(ctx context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.push", err), err
	}
	lock, err := syncdaemon.AcquireOperationLock(root, "sync.push")
	if err != nil {
		return commandErrorProjection("sync.push", err)
	}
	defer lock.Release()
	if target == "cloud" {
		if !req.Yes && !req.DryRun {
			err := &domain.CommandError{Code: "approval_required", Message: "sync push requires --yes or --dry-run", Hint: "Review the plan first with pinax sync push --target cloud --dry-run, then add --yes after confirming"}
			projection := domain.NewErrorProjection("sync.push", err)
			_ = writeApprovalRequiredSyncRun(root, req, "sync.push", syncplan.DirectionPush, err, &projection)
			return projection, err
		}
		return buildCloudSyncProjection(ctx, "sync.push", root, req, syncplan.DirectionPush)
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync push requires --yes", Hint: "Review the plan first with pinax sync diff, then add --yes after confirming"}
		return domain.NewErrorProjection("sync.push", err), err
	}
	return writeSyncState(root, target, "push")
}

func (s *Service) SyncPull(ctx context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.pull", err), err
	}
	lock, err := syncdaemon.AcquireOperationLock(root, "sync.pull")
	if err != nil {
		return commandErrorProjection("sync.pull", err)
	}
	defer lock.Release()
	if target == "cloud" {
		if !req.Yes && !req.DryRun {
			err := &domain.CommandError{Code: "approval_required", Message: "sync pull requires --yes or --dry-run", Hint: "Review the plan first with pinax sync pull --target cloud --dry-run, then add --yes after confirming"}
			projection := domain.NewErrorProjection("sync.pull", err)
			_ = writeApprovalRequiredSyncRun(root, req, "sync.pull", syncplan.DirectionPull, err, &projection)
			return projection, err
		}
		return buildCloudSyncProjection(ctx, "sync.pull", root, req, syncplan.DirectionPull)
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "sync pull requires --yes", Hint: "Review the plan first with pinax sync diff, then add --yes after confirming"}
		return domain.NewErrorProjection("sync.pull", err), err
	}
	return writeSyncState(root, target, "pull")
}

func cloudStateForSync(root string, req SyncRequest) (pinaxcloud.State, error) {
	if strings.TrimSpace(req.Endpoint) == "" {
		return pinaxcloud.Load(root)
	}
	return pinaxcloud.State{
		Config: pinaxcloud.Config{
			SchemaVersion: pinaxcloud.ConfigSchemaVersion,
			Endpoint:      strings.TrimRight(strings.TrimSpace(req.Endpoint), "/"),
			WorkspaceID:   strings.TrimSpace(req.WorkspaceID),
			DeviceID:      strings.TrimSpace(req.DeviceID),
			SecretRef:     strings.TrimSpace(req.SecretRef),
		},
		Session: pinaxcloud.DeviceSession{
			SchemaVersion: pinaxcloud.SessionSchemaVersion,
			DeviceID:      strings.TrimSpace(req.DeviceID),
			Status:        "profile",
		},
	}, nil
}

func buildCloudSyncProjection(ctx context.Context, command, root string, req SyncRequest, direction syncplan.Direction) (domain.Projection, error) {
	started := time.Now()
	pathPolicy := syncops.NormalizePathPolicy(req.PathPolicy)
	state, err := cloudStateForSync(root, req)
	if err != nil {
		return cloudStateErrorProjection(command, root, err)
	}
	receipt := syncRunStart(command, direction, state, pathPolicy)
	manifest := pinaxcloud.Manifest{SchemaVersion: pinaxcloud.ManifestSchemaVersion}
	if direction != syncplan.DirectionPull {
		manifest, err = pinaxcloud.BuildManifest(root)
		if err != nil {
			projection := errorProjection(command, err)
			return projection, err
		}
	}
	baseRevision := req.BaseRevision
	remoteRevision := req.RemoteRevision
	if remoteRevision == "" {
		remoteRevision = baseRevision
	}
	plan, planErr := syncplan.BuildPlan(syncplan.Request{Direction: direction, Target: "cloud", LocalManifest: manifest, BaseRevision: baseRevision, RemoteRevision: remoteRevision, DryRun: req.DryRun, Yes: req.Yes})
	if errors.Is(planErr, syncplan.ErrRevisionConflict) {
		commandErr := &domain.CommandError{Code: "REVISION_CONFLICT", Message: "cloud revision conflict", Hint: "Review the conflict queue and resolve manually, then retry sync"}
		projection := domain.NewErrorProjection(command, commandErr)
		projection.Actions = append(syncConflictActions(root, nil), domain.Action{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))})
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
		if receiptErr == nil {
			_ = writeCurrentSyncState(root, state, receipt, "")
			projection.Facts["run_id"] = receipt.RunID
			projection.Evidence = []string{receiptPath}
		}
		addCloudSyncFacts(&projection, state, plan)
		addCloudContentFacts(&projection, manifest)
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
		return projection, commandErr
	}
	if planErr != nil {
		projection := errorProjection(command, planErr)
		return projection, planErr
	}
	if direction == syncplan.DirectionPush && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		commit, execErr := executeCloudPush(ctx, root, state, manifest, req.BaseRevision)
		if execErr != nil {
			plan.RemoteWrite = false
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		plan.RemoteWrite = commit.RemoteWrite
		receipt.RemoteWrite = commit.RemoteWrite
		receipt.RevisionID = commit.RevisionID
		receipt.ManifestBlobID = commit.ManifestBlobID
		receipt.Counts["blobs"] = len(manifest.Entries) + manifestTrashBackupCount(manifest)
		receipt.Counts["delete_markers"] = len(manifest.Deletes)
		receipt.Counts["trash_backup_blobs"] = manifestTrashBackupCount(manifest)
		projection := domain.NewProjection(command, "Cloud sync push completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, commit.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
		addCloudContentFacts(&projection, manifest)
		projection.Facts["run_id"] = receipt.RunID
		projection.Facts["revision_id"] = commit.RevisionID
		projection.Evidence = []string{receiptPath}
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "remote_write": commit.RemoteWrite, "revision_id": commit.RevisionID, "manifest_blob_id": commit.ManifestBlobID, "receipt": receipt}
		return projection, nil
	}
	if direction == syncplan.DirectionPull && req.Yes && !req.DryRun && isExecutableCloudState(state) {
		pullResult, execErr := executeCloudPull(ctx, root, state)
		if execErr != nil {
			commandErr := commandErrorFromError(execErr)
			projection := domain.NewErrorProjection(command, commandErr)
			projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
			receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "failed", commandErr, projection.Actions, pathPolicy, started)
			if receiptErr == nil {
				_ = writeCurrentSyncState(root, state, receipt, "")
				projection.Facts["run_id"] = receipt.RunID
				projection.Evidence = []string{receiptPath}
			}
			projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "receipt": receipt}
			return projection, commandErr
		}
		receipt.LocalWrite = pullResult.FilesApplied > 0 || pullResult.DeletesApplied > 0
		receipt.RevisionID = pullResult.RevisionID
		receipt.ManifestBlobID = pullResult.ManifestBlobID
		receipt.Counts["files_applied"] = pullResult.FilesApplied
		receipt.Counts["delete_markers_applied"] = pullResult.DeletesApplied
		receipt.Counts["conflicts"] = len(pullResult.Conflicts)
		projection := domain.NewProjection(command, "Cloud sync pull completed through configured backend.")
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		if len(pullResult.Conflicts) > 0 {
			projection.Actions = append(projection.Actions, syncConflictActions(root, pullResult.Conflicts)...)
		}
		receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, "success", nil, projection.Actions, pathPolicy, started)
		if receiptErr != nil {
			return errorProjection(command, receiptErr), receiptErr
		}
		if err := writeCurrentSyncState(root, state, receipt, pullResult.RevisionID); err != nil {
			return errorProjection(command, err), err
		}
		addCloudSyncFacts(&projection, state, plan)
		projection.Facts["run_id"] = receipt.RunID
		projection.Facts["files_applied"] = fmt.Sprint(pullResult.FilesApplied)
		projection.Facts["delete_markers_applied"] = fmt.Sprint(pullResult.DeletesApplied)
		projection.Facts["revision_id"] = pullResult.RevisionID
		projection.Facts["conflicts"] = fmt.Sprint(len(pullResult.Conflicts))
		addSyncConflictFacts(&projection, pullResult.Conflicts)
		projection.Evidence = []string{receiptPath}
		projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "remote_write": false, "files_applied": pullResult.FilesApplied, "delete_markers_applied": pullResult.DeletesApplied, "revision_id": pullResult.RevisionID, "manifest_blob_id": pullResult.ManifestBlobID, "conflicts": pullResult.Conflicts, "receipt": receipt}
		return projection, nil
	}
	projection := domain.NewProjection(command, "Cloud sync plan generated; real remote writes are not wired yet.")
	status := "success"
	if plan.RequiresApproval {
		status = "approval_required"
		projection.Status = "failed"
	}
	if direction == syncplan.DirectionPush && req.Yes && !req.DryRun {
		plan.RemoteWrite = false
		status = "partial"
		projection.Status = "partial"
		projection.Facts["blocked_by"] = "cloud_api_unimplemented"
		projection.Actions = []domain.Action{{Name: "handoff", Command: fmt.Sprintf("pinax sync diff --target cloud --vault %s --json", shellQuote(root))}}
	}
	if len(projection.Actions) == 0 {
		projection.Actions = []domain.Action{{Name: "logs", Command: fmt.Sprintf("pinax sync logs list --vault %s --json", shellQuote(root))}}
	}
	var commandErr *domain.CommandError
	if status == "approval_required" {
		commandErr = &domain.CommandError{Code: "approval_required", Message: "sync requires approval", Hint: "Rerun with --yes or --dry-run"}
	}
	receipt, receiptPath, receiptErr := finishSyncRun(root, state, receipt, plan, status, commandErr, projection.Actions, pathPolicy, started)
	if receiptErr != nil {
		return errorProjection(command, receiptErr), receiptErr
	}
	_ = writeCurrentSyncState(root, state, receipt, "")
	addCloudSyncFacts(&projection, state, plan)
	addCloudContentFacts(&projection, manifest)
	projection.Facts["run_id"] = receipt.RunID
	projection.Evidence = []string{receiptPath}
	projection.Data = map[string]any{"plan": syncops.SanitizePlan(plan, pathPolicy), "blocked_by": projection.Facts["blocked_by"], "receipt": receipt}
	return projection, nil
}

func commandErrorFromError(err error) *domain.CommandError {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr
	}
	rawMessage := err.Error()
	message := syncops.SanitizeString(rawMessage)
	switch {
	case strings.Contains(rawMessage, "lock_held"):
		return &domain.CommandError{Code: "lock_held", Message: message, Hint: "Retry after the current cloud sync finishes"}
	case strings.Contains(rawMessage, "transport_unavailable"), isRcloneCommandFailure(rawMessage):
		return &domain.CommandError{Code: "transport_unavailable", Message: message, Hint: "Check the configured cloud transport before retrying"}
	default:
		return &domain.CommandError{Code: "cloud_sync_failed", Message: message, Hint: "Run pinax cloud doctor --vault <vault> --json"}
	}
}

func isRcloneCommandFailure(message string) bool {
	return strings.Contains(message, "rclone ") && strings.Contains(message, " failed")
}

func directBackendKind(state pinaxcloud.State) string {
	if strings.TrimSpace(state.Config.BackendKind) != "" {
		return state.Config.BackendKind
	}
	if strings.HasPrefix(state.Config.Endpoint, "file://") {
		return "embedded"
	}
	if strings.HasPrefix(state.Config.Endpoint, "s3://") {
		return "s3-direct"
	}
	if strings.HasPrefix(state.Config.Endpoint, "rclone://") {
		return "rclone-direct"
	}
	return "direct"
}
func isExecutableCloudState(state pinaxcloud.State) bool {
	endpoint := strings.TrimSpace(state.Config.Endpoint)
	return strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") || strings.HasPrefix(endpoint, "file://") || strings.HasPrefix(endpoint, "s3://") || strings.HasPrefix(endpoint, "rclone://") || state.Config.BackendKind == "server" || state.Config.BackendKind == "s3-direct" || state.Config.BackendKind == "rclone-direct"
}

func cloudTransportForState(ctx context.Context, state pinaxcloud.State) (cloudsync.Transport, error) {
	endpoint := strings.TrimSpace(state.Config.Endpoint)
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") || state.Config.BackendKind == "server" {
		token, err := pinaxprofile.ResolveSecretRef(state.Config.SecretRef)
		if err != nil {
			return nil, &domain.CommandError{Code: "cloud_secret_unavailable", Message: "cloud credential is unavailable", Hint: "Check the configured cloud secret reference before retrying"}
		}
		client, err := cloudclient.New(cloudclient.Config{Endpoint: endpoint, VaultID: state.Config.WorkspaceID, DeviceID: state.Config.DeviceID, Token: token})
		if err != nil {
			return nil, err
		}
		return cloudclient.NewTransport(client), nil
	}
	store, err := state.GetStore(ctx)
	if err != nil {
		return nil, err
	}
	return cloudsync.NewObjectStoreTransport(store, cloudsync.Layout{WorkspaceID: state.Config.WorkspaceID, VaultID: state.Config.WorkspaceID}), nil
}

type directPullResult struct {
	FilesApplied   int
	DeletesApplied int
	RevisionID     string
	ManifestBlobID string
	Conflicts      []domain.SyncConflictEntry
}

func executeCloudPull(ctx context.Context, root string, state pinaxcloud.State) (directPullResult, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return directPullResult{}, err
	}
	head, err := transport.CurrentHead(ctx, state.Config.WorkspaceID)
	if err != nil {
		return directPullResult{}, err
	}
	if strings.TrimSpace(head.CurrentRevision) == "" || strings.TrimSpace(head.ManifestBlobID) == "" {
		return directPullResult{}, &domain.CommandError{Code: "cloud_empty_remote", Message: "cloud backend has no committed revision", Hint: "Run pinax sync push --target cloud --yes from a device with notes first"}
	}
	key, err := pinaxcloud.DeriveKey(pinaxcloud.EncryptionSecretRef(state.Config))
	if err != nil {
		return directPullResult{}, err
	}
	manifestEnvelope, err := transport.GetManifest(ctx, head.ManifestBlobID)
	if err != nil {
		return directPullResult{}, err
	}
	manifest, err := pinaxcloud.DecryptManifest(key, remoteEnvelope(manifestEnvelope))
	if err != nil {
		return directPullResult{}, err
	}
	filesApplied := 0
	deletesApplied := 0
	conflicts := []domain.SyncConflictEntry{}
	for _, deleteMarker := range manifest.Deletes {
		result, err := applyRemoteTrashDelete(root, remoteTrashDeleteMarker{ObjectKind: deleteMarker.ObjectKind, ObjectID: deleteMarker.ObjectID, TombstoneID: deleteMarker.TombstoneID, DeletedAt: deleteMarker.DeletedAt})
		if err != nil {
			return directPullResult{}, err
		}
		if result.Applied {
			deletesApplied++
		}
		if result.Conflict != nil {
			conflicts = append(conflicts, *result.Conflict)
		}
	}
	for _, entry := range manifest.Entries {
		blobEnvelope, err := transport.GetBlob(ctx, entry.BlobID)
		if err != nil {
			return directPullResult{}, err
		}
		content, err := pinaxcloud.DecryptBlob(key, remoteEnvelope(blobEnvelope), []byte(entry.BlobID))
		if err != nil {
			return directPullResult{}, err
		}
		path, err := safeCloudSyncPath(root, entry.Path)
		if err != nil {
			return directPullResult{}, err
		}
		fileMode := os.FileMode(entry.Mode & 0o777)
		if fileMode == 0 {
			fileMode = 0o600
		}
		if existing, err := os.ReadFile(path); err == nil {
			if bytes.Equal(existing, content) {
				if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() != fileMode {
					if err := os.Chmod(path, fileMode); err != nil {
						return directPullResult{}, err
					}
					filesApplied++
				}
				continue
			}
			conflictPath := strings.TrimSuffix(path, filepath.Ext(path)) + "." + time.Now().UTC().Format("20060102150405") + ".conflict" + filepath.Ext(path)
			if err := os.WriteFile(conflictPath, existing, fileMode); err != nil {
				return directPullResult{}, err
			}
			if rel, relErr := filepath.Rel(root, conflictPath); relErr == nil {
				conflictRel := filepath.ToSlash(rel)
				mainRel, mainErr := mainPathForSyncConflict(conflictRel)
				if mainErr != nil {
					return directPullResult{}, mainErr
				}
				conflicts = append(conflicts, domain.SyncConflictEntry{File: conflictRel, MainPath: mainRel})
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return directPullResult{}, err
		}
		if err := os.WriteFile(path, content, fileMode); err != nil {
			return directPullResult{}, err
		}
		filesApplied++
	}
	result := directPullResult{FilesApplied: filesApplied, DeletesApplied: deletesApplied, RevisionID: head.CurrentRevision, ManifestBlobID: head.ManifestBlobID, Conflicts: conflicts}

	return result, nil
}

func safeCloudSyncPath(root, rel string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(rel) || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".pinax/") || strings.HasPrefix(clean, ".git/") {
		return "", &domain.CommandError{Code: "unsafe_cloud_path", Message: "cloud manifest path is outside the vault", Hint: "Inspect the remote manifest and retry after removing unsafe entries"}
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func localCloudBaseRevision(root string, cloudState pinaxcloud.State) string {
	state, err := readCurrentSyncState(root)
	if err != nil || state.Target != "cloud" {
		return ""
	}
	if state.BackendKind != directBackendKind(cloudState) || state.WorkspaceID != cloudState.Config.WorkspaceID || state.Endpoint != cloudState.Config.Endpoint {
		return ""
	}
	return strings.TrimSpace(state.LastSyncedRevision)
}

func executeCloudPush(ctx context.Context, root string, state pinaxcloud.State, manifest pinaxcloud.Manifest, baseRevision string) (cloudsync.CommitResult, error) {
	transport, err := cloudTransportForState(ctx, state)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	if strings.TrimSpace(baseRevision) == "" {
		baseRevision = localCloudBaseRevision(root, state)
	}
	key, err := pinaxcloud.DeriveKey(pinaxcloud.EncryptionSecretRef(state.Config))
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	blobIDs := make([]string, 0, len(manifest.Entries)+len(manifest.Deletes))
	objectRefs := make([]cloudsync.ObjectRef, 0, len(manifest.Entries)+(len(manifest.Deletes)*2))
	for _, entry := range manifest.Entries {
		blobIDs = append(blobIDs, entry.BlobID)
		objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: entry.PathHash, BlobID: entry.BlobID, BlobHash: entry.SHA256, Size: entry.Size})
	}
	for _, deleteMarker := range manifest.Deletes {
		if strings.HasPrefix(deleteMarker.TrashBlobID, "blob_") {
			blobIDs = append(blobIDs, deleteMarker.TrashBlobID)
			objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: deleteMarker.PathHash, BlobID: deleteMarker.TrashBlobID})
		}
		objectRefs = append(objectRefs, cloudsync.ObjectRef{PathHash: deleteMarker.PathHash, BlobID: deleteMarker.TrashBlobID, Deleted: true})
	}
	missing, err := transport.BatchCheck(ctx, blobIDs)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	missingSet := make(map[string]struct{}, len(missing.MissingBlobIDs))
	for _, blobID := range missing.MissingBlobIDs {
		missingSet[blobID] = struct{}{}
	}
	presentFacts := make(map[string]cloudsync.BlobFact, len(missing.Present))
	for _, fact := range missing.Present {
		presentFacts[fact.BlobID] = fact
	}
	for i, entry := range manifest.Entries {
		if _, ok := missingSet[entry.BlobID]; !ok {
			if fact, ok := presentFacts[entry.BlobID]; ok {
				objectRefs[i].BlobHash = fact.BlobHash
				objectRefs[i].Size = fact.Size
			}
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", entry.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		envelope, err := pinaxcloud.EncryptBlob(key, content, []byte(entry.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		cloudBlob := cloudEnvelope(envelope)
		if metadataWriter, ok := transport.(interface {
			PutBlobWithEnvelopeMetadata(context.Context, string, cloudsync.Envelope) (string, int64, error)
		}); ok {
			blobHash, sizeBytes, err := metadataWriter.PutBlobWithEnvelopeMetadata(ctx, entry.BlobID, cloudBlob)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			objectRefs[i].BlobHash = blobHash
			objectRefs[i].Size = sizeBytes
			continue
		}
		if err := transport.PutBlob(ctx, entry.BlobID, cloudBlob); err != nil {
			return cloudsync.CommitResult{}, err
		}
	}
	for i := len(manifest.Entries); i < len(objectRefs); i++ {
		ref := objectRefs[i]
		if ref.Deleted || ref.BlobID == "" || !strings.HasPrefix(ref.BlobID, "blob_") {
			continue
		}
		if _, ok := missingSet[ref.BlobID]; !ok {
			if fact, ok := presentFacts[ref.BlobID]; ok {
				objectRefs[i].BlobHash = fact.BlobHash
				objectRefs[i].Size = fact.Size
			}
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", ref.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		envelope, err := pinaxcloud.EncryptBlob(key, content, []byte(ref.BlobID))
		if err != nil {
			return cloudsync.CommitResult{}, err
		}
		cloudBlob := cloudEnvelope(envelope)
		if metadataWriter, ok := transport.(interface {
			PutBlobWithEnvelopeMetadata(context.Context, string, cloudsync.Envelope) (string, int64, error)
		}); ok {
			blobHash, sizeBytes, err := metadataWriter.PutBlobWithEnvelopeMetadata(ctx, ref.BlobID, cloudBlob)
			if err != nil {
				return cloudsync.CommitResult{}, err
			}
			objectRefs[i].BlobHash = blobHash
			objectRefs[i].Size = sizeBytes
			continue
		}
		if err := transport.PutBlob(ctx, ref.BlobID, cloudBlob); err != nil {
			return cloudsync.CommitResult{}, err
		}
	}
	manifestEnvelope, err := pinaxcloud.EncryptManifest(key, manifest)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return cloudsync.CommitResult{}, err
	}
	manifestBlobID := "manifest_" + strings.TrimPrefix(pinaxcloud.BlobID(manifestBytes), "blob_")
	if err := transport.PutManifest(ctx, manifestBlobID, cloudEnvelope(manifestEnvelope)); err != nil {
		return cloudsync.CommitResult{}, err
	}
	return transport.CommitRevision(ctx, cloudsync.CommitRequest{BaseRevision: baseRevision, RevisionID: "rev_" + time.Now().UTC().Format("20060102150405.000000000"), ManifestBlobID: manifestBlobID, BlobIDs: blobIDs, ObjectRefs: objectRefs, DeviceID: state.Config.DeviceID, RequestID: "pinax-" + time.Now().UTC().Format("20060102150405.000000000")})
}

func remoteEnvelope(envelope cloudsync.Envelope) pinaxcloud.EncryptedEnvelope {
	return pinaxcloud.EncryptedEnvelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func cloudEnvelope(envelope pinaxcloud.EncryptedEnvelope) cloudsync.Envelope {
	return cloudsync.Envelope{SchemaVersion: envelope.SchemaVersion, Alg: envelope.Alg, KeyID: envelope.KeyID, Nonce: envelope.Nonce, Ciphertext: envelope.Ciphertext, PlainSHA256: envelope.PlainSHA256}
}

func cloudSyncNotConfiguredProjection(root string) domain.Projection {
	projection := domain.NewProjection("sync.diff", "Cloud sync requires configuring a backend first.")
	projection.Status = "partial"
	projection.Facts["target"] = "cloud"
	projection.Facts["backend_required"] = "true"
	projection.Facts["configured"] = "false"
	projection.Facts["remote_write"] = "false"
	projection.Data = map[string]any{"target": "cloud", "remote_write": false, "plan": map[string]any{"target": "cloud", "status": "backend_required"}}
	projection.Actions = []domain.Action{{Name: "login", Command: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}}
	return projection
}

func isCommandErrorCode(err error, code string) bool {
	var commandErr *domain.CommandError
	return errors.As(err, &commandErr) && commandErr.Code == code
}

func addCloudSyncFacts(projection *domain.Projection, state pinaxcloud.State, plan syncplan.Plan) {
	projection.Facts["target"] = "cloud"
	projection.Facts["workspace_id"] = state.Config.WorkspaceID
	projection.Facts["device_id"] = state.Config.DeviceID
	projection.Facts["backend_kind"] = directBackendKind(state)
	projection.Facts["dry_run"] = fmt.Sprint(plan.DryRun)
	projection.Facts["remote_write"] = fmt.Sprint(plan.RemoteWrite)
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["base_revision"] = plan.BaseRevision
	projection.Facts["remote_revision"] = plan.RemoteRevision
	projection.Facts["conflicts"] = fmt.Sprint(len(plan.ConflictQueue))
}

func addCloudContentFacts(projection *domain.Projection, manifest pinaxcloud.Manifest) {
	var bytes int64
	scripts := 0
	binaries := 0
	trashBackups := 0
	for _, entry := range manifest.Entries {
		bytes += entry.Size
		if isManifestScript(entry) {
			scripts++
			continue
		}
		if !isManifestText(entry.Path) {
			binaries++
		}
	}
	for _, deleteMarker := range manifest.Deletes {
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" {
			trashBackups++
		}
	}
	projection.Facts["content_files"] = fmt.Sprint(len(manifest.Entries))
	projection.Facts["content_bytes"] = fmt.Sprint(bytes)
	projection.Facts["script_files"] = fmt.Sprint(scripts)
	projection.Facts["binary_files"] = fmt.Sprint(binaries)
	projection.Facts["delete_markers"] = fmt.Sprint(len(manifest.Deletes))
	projection.Facts["trash_backup_blobs"] = fmt.Sprint(trashBackups)
}

func manifestTrashBackupCount(manifest pinaxcloud.Manifest) int {
	count := 0
	for _, deleteMarker := range manifest.Deletes {
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" {
			count++
		}
	}
	return count
}

func isManifestScript(entry pinaxcloud.ManifestEntry) bool {
	return strings.HasPrefix(entry.Path, "scripts/") || entry.Mode&0o111 != 0 || strings.HasSuffix(strings.ToLower(entry.Path), ".sh")
}

func isManifestText(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".yaml", ".yml", ".json", ".jsonl", ".toml", ".csv", ".gitignore", ".pinaxignore":
		return true
	case ".sh", ".bash", ".zsh", ".fish", ".py", ".js", ".ts":
		return true
	}
	base := filepath.Base(path)
	return base == ".gitignore" || base == ".pinaxignore"
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
	projection := domain.NewProjection("project.create", "Project created.")
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
		return &domain.CommandError{Code: "project_slug_required", Message: "Project requires a slug", Hint: "Run pinax project create <slug> --name <name>"}
	}
	for _, r := range slug {
		if unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return &domain.CommandError{Code: "invalid_project_slug", Message: "Project slug may only contain lowercase letters, numbers, -, and _", Hint: "For example, pinax project create research"}
	}
	return nil
}

func validateProjectPrefix(prefix string) error {
	clean := filepath.ToSlash(filepath.Clean(prefix))
	if clean == "." || filepath.IsAbs(prefix) || strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, ".pinax") {
		return &domain.CommandError{Code: "unsafe_project_prefix", Message: "Project notes prefix must be inside the vault and must not point to .pinax", Hint: "Use a prefix like notes/research"}
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
			ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "Target path already exists", Status: "conflict"})
			continue
		}
		ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "Place under notes/ by title", Status: "planned"})
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
		SourceCommand: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root)),
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
	case "link_rewrite":
		before = map[string]string{"link_target": op.Target}
		after = map[string]string{"rewrite": "manual"}
	case "orphan_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": "orphan"}
	case "attachment_repair":
		before = map[string]string{"attachment": op.Target}
		after = map[string]string{"repair": "manual"}
	case "manual_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": "required"}
	case "source_move":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"path": op.Target}
	case "source_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": op.Target}
	}
	evidence := op.Evidence
	if len(evidence) == 0 {
		evidence = []string{"path=" + op.Path, "target=" + op.Target}
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
		Evidence:    evidence,
		Status:      op.Status,
	}
}

func organizeFactOperations(root string, facts []noteFact) []domain.PlanOperation {
	notes := notesFromFacts(facts)
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	ops := make([]domain.PlanOperation, 0)
	for _, fact := range facts {
		if info, ok := durableSourceInfo(fact.note); ok {
			target := durableSourceTargetPath(info)
			if target != "" && fact.rel != target {
				ops = append(ops, domain.PlanOperation{Kind: "source_move", Path: fact.rel, Target: target, Reason: "Move durable GitHub source note under sources/github/", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "repo=" + info.Owner + "/" + info.Repo}})
			}
			if missing := missingDurableSourceSections(fact.note.Body); len(missing) > 0 {
				ops = append(ops, domain.PlanOperation{Kind: "source_review", Path: fact.rel, Target: strings.Join(missing, ","), Reason: "Missing durable source sections", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "missing=" + strings.Join(missing, ",")}})
			}
		}
		inlineTags := cleanTags(noteAllTags(fact.note))
		if len(fact.note.Tags) == 0 && len(inlineTags) > 0 {
			ops = append(ops, domain.PlanOperation{Kind: "tag_patch", Path: fact.rel, Target: strings.Join(inlineTags, ","), Reason: "Add frontmatter tags from inline body tags", Status: "planned"})
		}
		if strings.TrimSpace(fact.note.Kind) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "kind_patch", Path: fact.rel, Target: inferNoteKind(fact.note), Reason: "Missing kind classification; confirmation required", Status: "manual_review"})
		}
		if strings.TrimSpace(fact.note.Status) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "status_patch", Path: fact.rel, Target: "active", Reason: "Missing status; active is recommended", Status: "planned"})
		}
		for _, link := range outgoing[fact.rel] {
			switch {
			case link.Status == string(domain.LinkStatusBroken) || link.Broken:
				ops = append(ops, domain.PlanOperation{Kind: "link_resolution", Path: fact.rel, Target: link.Target, Reason: "Unresolved link requires manual target confirmation", Status: "manual_review", Evidence: linkEvidence(link)})
			case link.Status == string(domain.LinkStatusAmbiguous):
				ops = append(ops, domain.PlanOperation{Kind: "link_rewrite", Path: fact.rel, Target: link.Target, Reason: "Link target has multiple candidates; manually confirm body wording", Status: "manual_review", Evidence: linkEvidence(link)})
			}
		}
		for _, attachment := range noteAttachmentsFromBody(root, fact.note) {
			if !attachment.Exists {
				ops = append(ops, domain.PlanOperation{Kind: "attachment_repair", Path: fact.rel, Target: attachment.TargetPath, Reason: "Attachment reference is missing and needs repair or removal", Status: "manual_review"})
			}
		}
		if len(outgoing[fact.rel]) == 0 && len(incoming[fact.rel]) == 0 {
			ops = append(ops, domain.PlanOperation{Kind: "orphan_review", Path: fact.rel, Target: fact.note.Title, Reason: "Note has no bidirectional links; manually confirm archive or add context", Status: "manual_review", Evidence: []string{"title=" + fact.note.Title, "graph=incoming:0,outgoing:0"}})
		}
		if !fact.hasFrontmatter {
			ops = append(ops, domain.PlanOperation{Kind: "manual_review", Path: fact.rel, Target: "frontmatter", Reason: "Missing Pinax frontmatter; metadata confirmation required", Status: "manual_review"})
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

type durableSourceCandidate struct {
	URL   string
	Owner string
	Repo  string
}

var githubRepoURLPattern = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(?:[/?#][^\s)\]]*)?`)

func durableSourceInfo(note domain.Note) (durableSourceCandidate, bool) {
	if value := strings.TrimSpace(note.Frontmatter["source_url"]); value != "" {
		if candidate, ok := parseGitHubRepoURL(value); ok {
			return candidate, true
		}
	}
	if candidate, ok := firstGitHubRepoURL(note.Body); ok {
		return candidate, true
	}
	if candidate, ok := ownerRepoFromTitle(note.Title); ok && durableSourceTagSignal(note) {
		candidate.URL = "https://github.com/" + candidate.Owner + "/" + candidate.Repo
		return candidate, true
	}
	return durableSourceCandidate{}, false
}

func durableSourceMetadataOperations(note domain.Note) []domain.PlanOperation {
	info, ok := durableSourceInfo(note)
	if !ok {
		return nil
	}
	targets := []string{"source_url=" + info.URL}
	if strings.TrimSpace(note.Kind) != "source" {
		targets = append(targets, "kind=source")
	}
	tags := durableSourceRecommendedTags(note)
	if len(tags) > 0 {
		targets = append(targets, "tags="+strings.Join(tags, ","))
	}
	for _, key := range []string{"last_checked_at", "source_license", "review_after"} {
		if strings.TrimSpace(note.Frontmatter[key]) == "" {
			targets = append(targets, key+"=<review>")
		}
	}
	return []domain.PlanOperation{{Kind: "source_metadata", Path: note.Path, Target: strings.Join(targets, ";"), Reason: "Review durable source metadata for external GitHub reference", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "repo=" + info.Owner + "/" + info.Repo}}}
}

func durableSourceRecommendedTags(note domain.Note) []string {
	recommended := []string{"source/github", "reference/source"}
	existing := map[string]bool{}
	for _, tag := range noteAllTags(note) {
		existing[strings.ToLower(tag)] = true
	}
	missing := make([]string, 0, len(recommended))
	for _, tag := range recommended {
		if !existing[strings.ToLower(tag)] {
			missing = append(missing, tag)
		}
	}
	return missing
}

func durableSourceTargetPath(info durableSourceCandidate) string {
	slug := slugify(info.Owner + "-" + info.Repo)
	if slug == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join("sources", "github", slug+".md"))
}

func firstGitHubRepoURL(body string) (durableSourceCandidate, bool) {
	match := githubRepoURLPattern.FindStringSubmatch(body)
	if len(match) < 3 {
		return durableSourceCandidate{}, false
	}
	raw := match[0]
	if candidate, ok := parseGitHubRepoURL(raw); ok {
		return candidate, true
	}
	return durableSourceCandidate{URL: "https://github.com/" + match[1] + "/" + strings.TrimSuffix(match[2], ".git"), Owner: match[1], Repo: strings.TrimSuffix(match[2], ".git")}, true
}

func parseGitHubRepoURL(raw string) (durableSourceCandidate, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return durableSourceCandidate{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return durableSourceCandidate{}, false
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	return durableSourceCandidate{URL: "https://github.com/" + parts[0] + "/" + repo, Owner: parts[0], Repo: repo}, true
}

func ownerRepoFromTitle(title string) (durableSourceCandidate, bool) {
	parts := strings.Split(strings.TrimSpace(title), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(title, " \t\n") {
		return durableSourceCandidate{}, false
	}
	return durableSourceCandidate{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}, true
}

func durableSourceTagSignal(note domain.Note) bool {
	for _, tag := range noteAllTags(note) {
		switch strings.ToLower(tag) {
		case "github", "source", "reference", "repo", "source/github", "reference/source":
			return true
		}
	}
	return false
}

func missingDurableSourceSections(body string) []string {
	required := []string{"Use decision", "Risk and boundary", "Verification", "Related notes"}
	lower := strings.ToLower(body)
	missing := make([]string, 0, len(required))
	for _, section := range required {
		if !strings.Contains(lower, strings.ToLower(section)) {
			missing = append(missing, section)
		}
	}
	return missing
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
		return domain.OrganizePlan{}, &domain.CommandError{Code: "plan_required", Message: "organize plan id cannot be empty", Hint: "Run pinax organize plan --save to generate a plan"}
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
		return domain.OrganizePlan{}, &domain.CommandError{Code: "organize_plan_schema_invalid", Message: "organize plan schema is not supported", Hint: "Rerun pinax organize plan --save"}
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
		return &domain.CommandError{Code: "organize_plan_not_planned", Message: "organize plan status is not applicable", Hint: "Rerun pinax organize plan --save"}
	}
	expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
	if err == nil && time.Now().UTC().After(expires) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan has expired", Hint: "pinax organize plan --vault <vault> --save"}
	}
	// 校验前必须与 buildOrganizePlan 保持同一套候选事实：计划保存时通过
	// organizeCandidateFacts 过滤掉 daily/journal 等非组织候选笔记，这里如果不
	// 同样过滤，任何包含日志的 vault 都会因为 facts 数量不一致被误判为 stale。
	facts, err := scanNoteFacts(root)
	if err != nil {
		return err
	}
	current := organizeSourceFacts(organizeCandidateFacts(facts))
	if len(current) != len(plan.SourceFacts) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan does not match current vault facts", Hint: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}
	}
	for key, value := range plan.SourceFacts {
		if current[key] != value {
			return &domain.CommandError{Code: "plan_stale", Message: "organize plan does not match current vault facts", Hint: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}
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
		meta, _, _ := markdownnote.ParseFrontmatter(content)
		if !isPinaxNoteFrontmatter(meta) {
			return nil
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

type indexRefreshScanItem struct {
	path       string
	note       domain.Note
	failedPath string
	err        error
}

func scanIndexRefreshNotes(root string) ([]domain.Note, []string, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, nil, nil
	}
	workers := indexRefreshWorkerCount(len(paths))
	jobs := make(chan string)
	results := make(chan indexRefreshScanItem, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				results <- scanIndexRefreshNote(root, path)
			}
		}()
	}
	go func() {
		for _, path := range paths {
			jobs <- path
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	notes := make([]domain.Note, 0, len(paths))
	failedPaths := make([]string, 0)
	var firstErr error
	for item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		if item.failedPath != "" {
			failedPaths = append(failedPaths, item.failedPath)
			continue
		}
		if item.note.Path != "" {
			notes = append(notes, item.note)
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	sort.Strings(failedPaths)
	return notes, failedPaths, nil
}

func scanIndexRefreshNote(root, path string) indexRefreshScanItem {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return indexRefreshScanItem{err: err}
	}
	rel = filepath.ToSlash(rel)
	content, err := os.ReadFile(path)
	if err != nil {
		return indexRefreshScanItem{path: rel, failedPath: rel}
	}
	meta, _, _ := markdownnote.ParseFrontmatter(content)
	if !isPinaxNoteFrontmatter(meta) {
		return indexRefreshScanItem{path: rel}
	}
	note := parseNote(rel, string(content))
	if isSystemIndexNote(note) || isSystemJournalNote(note) {
		return indexRefreshScanItem{path: rel}
	}
	if strings.TrimSpace(note.Path) == "" || strings.TrimSpace(note.ID) == "" {
		return indexRefreshScanItem{path: rel, failedPath: rel}
	}
	return indexRefreshScanItem{path: rel, note: note}
}

func indexRefreshWorkerCount(total int) int {
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	if total > 0 && workers > total {
		workers = total
	}
	return workers
}

func ordinaryNotes(notes []domain.Note) []domain.Note {
	ordinary := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if isSystemIndexNote(note) || isSystemJournalNote(note) {
			continue
		}
		ordinary = append(ordinary, note)
	}
	return ordinary
}

func ordinaryNoteFacts(facts []noteFact) []noteFact {
	ordinary := make([]noteFact, 0, len(facts))
	for _, fact := range facts {
		if isSystemIndexNote(fact.note) || isSystemJournalNote(fact.note) {
			continue
		}
		ordinary = append(ordinary, fact)
	}
	return ordinary
}

func shouldSkipVaultWalkDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "dist"
}

func isPinaxNoteFrontmatter(meta map[string]string) bool {
	return meta["schema_version"] == "pinax.note.v1"
}

func isSystemIndexNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind != "index" {
		return false
	}
	// index page 是系统导航页，不参与普通知识卡片的 search/orphan/stat；旧 notes/daily index 继续按 legacy 系统页过滤。
	return strings.HasPrefix(path, "index/") || strings.HasPrefix(path, "notes/index/") || strings.HasPrefix(path, "notes/daily/")
}

func isSystemJournalNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind != "daily" && note.Kind != "weekly" && note.Kind != "monthly" {
		return false
	}
	return strings.HasPrefix(path, note.Kind+"/") || strings.HasPrefix(path, "notes/"+note.Kind+"/")
}
func parseNote(rel, content string) domain.Note {
	doc, err := markdownnote.ParseFull(rel, []byte(content))
	if err == nil {
		return doc.Note
	}
	meta, body := splitFrontmatter(content)
	title := meta["title"]
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	return domain.Note{ID: meta["note_id"], Title: title, Path: rel, Tags: parseTags(meta["tags"]), Labels: parseTags(meta["labels"]), Body: strings.TrimSpace(body), Frontmatter: meta, Project: meta["project"], Subproject: meta["subproject"], Folder: meta["folder"], Kind: meta["kind"], Status: meta["status"], BoardColumn: meta["board_column"], Milestone: meta["milestone"], Priority: meta["priority"], Due: meta["due"], DueAt: meta["due_at"], BlockedBy: parseTags(meta["blocked_by"]), CreatedAt: meta["created_at"], UpdatedAt: meta["updated_at"]}
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

func noteNeedsMetadataInVault(root string, note domain.Note) bool {
	if noteNeedsMetadata(note) {
		return true
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return true
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	meta, _ := splitFrontmatter(string(payload))
	return strings.TrimSpace(meta["schema_version"]) == ""
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
	return domain.Project{}, &domain.CommandError{Code: "project_not_found", Message: "Project not found", Hint: "Run pinax project list to view available projects"}
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
		return "", &domain.CommandError{Code: "note_source_conflict", Message: "note new can use only one body source", Hint: "Keep only one of --body, --from, or --stdin"}
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
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "Editor is not configured", Hint: "Set $EDITOR or pass --editor"}
	}
	parts, err := splitCommandLine(value)
	if err != nil {
		return editorCommand{}, &domain.CommandError{Code: "editor_parse_failed", Message: "Editor command could not be parsed", Hint: "Use a simple command or pass a wrapper script"}
	}
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "Editor is not configured", Hint: "Set $EDITOR or pass --editor"}
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
	// 默认笔记写在 vault 根内容区；显式 --dir 和 project prefix 继续保留旧 `notes/` 兼容语义。
	base := ""
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
		return "", &domain.CommandError{Code: "unsafe_note_folder", Message: "note folder must be a relative directory under Project or notes", Hint: "Use a folder like inbox, reference, or work/research"}
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
		return "", &domain.CommandError{Code: "unsafe_note_path", Message: "note directory must be under vault notes/", Hint: "Use a directory like work or notes/work"}
	}
	if clean == "notes" || strings.HasPrefix(clean, "notes/") {
		return clean, nil
	}
	return filepath.ToSlash(filepath.Join("notes", clean)), nil
}

func validateNoteSlug(slug string) error {
	clean := filepath.ToSlash(filepath.Clean(slug))
	if clean == "." || clean == ".." || strings.Contains(clean, "/") || strings.HasPrefix(clean, ".") || filepath.IsAbs(slug) {
		return &domain.CommandError{Code: "invalid_note_slug", Message: "note slug must be a single safe filename", Hint: "Use a slug like daily-review"}
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
		return domain.Note{}, &domain.CommandError{Code: "note_ref_required", Message: "note reference is required", Hint: "Provide a note_id, path, or title"}
	}
	needle := filepath.ToSlash(strings.TrimPrefix(ref, "notes/"))
	var titleMatches []domain.Note
	var stemMatches []domain.Note
	for _, note := range notes {
		if note.ID == ref || note.Path == ref || strings.TrimPrefix(note.Path, "notes/") == needle {
			return note, nil
		}
		if strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)) == ref {
			stemMatches = append(stemMatches, note)
		}
		if note.Title == ref || journalNoteShellFriendlyAlias(note) == ref {
			titleMatches = append(titleMatches, note)
		}
	}
	if len(stemMatches) == 1 {
		return stemMatches[0], nil
	}
	if len(stemMatches) > 1 {
		return domain.Note{}, &noteRefAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Ref: ref, Candidates: stemMatches}
	}
	if len(titleMatches) == 1 {
		return titleMatches[0], nil
	}
	if len(titleMatches) > 1 {
		return domain.Note{}, &noteRefAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Ref: ref, Candidates: titleMatches}
	}
	return domain.Note{}, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
}

func noteMatchesQuery(note domain.Note, req NoteListRequest) bool {
	// List query only applies explicit dimensions; it avoids fuzzy title/body matching so CLI output remains predictable.
	return noteops.MatchesList(note, noteops.ListRequest{Project: req.Project, Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, UpdatedBefore: req.UpdatedBefore, PathPrefix: req.PathPrefix, Tags: req.Tags})
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

func (s *Service) loadMutableNoteForWrite(ctx context.Context, vaultPath, noteRef string) (string, domain.Note, string, string, map[string]string, string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	result, err := s.ResolveVaultObjectForWrite(ctx, ResolverRequest{VaultPath: root, Query: noteRef, Scope: "registered", Kind: "note"})
	if err != nil {
		if len(result.Candidates) > 1 {
			return "", domain.Note{}, "", "", nil, "", &resolverNoteAmbiguousError{CommandError: &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "note write query matched multiple candidates", Hint: "Retry with a more specific note_id, filename, or full path"}, Result: result}
		}
		return "", domain.Note{}, "", "", nil, "", err
	}
	if len(result.Candidates) == 0 {
		return "", domain.Note{}, "", "", nil, "", &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
	}
	notes, err := scanNotes(root)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	var note domain.Note
	for _, candidate := range notes {
		if candidate.Path == result.Candidates[0].Path {
			note = candidate
			break
		}
	}
	if note.Path == "" {
		return "", domain.Note{}, "", "", nil, "", &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax index refresh, then retry"}
	}
	return loadMutableResolvedNote(root, note)
}

func loadMutableResolvedNote(root string, note domain.Note) (string, domain.Note, string, string, map[string]string, string, error) {
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
	return root, note, path, string(b), meta, body, nil
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
	return patchFrontmatterFieldsRemoving(content, fields, nil)
}

func patchFrontmatterFieldsRemoving(content string, fields map[string]string, removeKeys []string) (string, bool) {
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
	remove := map[string]bool{}
	for _, key := range removeKeys {
		remove[key] = true
	}
	kept := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			kept = append(kept, line)
			continue
		}
		key = strings.TrimSpace(key)
		if remove[key] {
			continue
		}
		if value, ok := fields[key]; ok {
			line = key + ": " + value
			seen[key] = true
		}
		kept = append(kept, line)
	}
	for _, key := range orderedFrontmatterKeys(fields) {
		if !seen[key] && strings.TrimSpace(fields[key]) != "" {
			kept = append(kept, key+": "+fields[key])
		}
	}
	return "---\n" + strings.Join(kept, "\n") + "\n---\n\n" + strings.TrimLeft(body, "\n"), false
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

//nolint:unused // Kept for legacy note fixtures that still use the pre-status frontmatter shape.
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
		return "", &domain.CommandError{Code: "invalid_template_name", Message: "template name may only contain letters, numbers, -, _, and .", Hint: "For example, pinax template create meeting or index.home"}
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
		if i > 0 && (r == '-' || r == '_' || r == '.') {
			continue
		}
		return false
	}
	return true
}

func templatePath(root, name string) (string, error) {
	return safeJoin(root, filepath.ToSlash(filepath.Join(".pinax", "templates", name+".md")))
}

func templateBodyWithRequestedEngine(body, engine string) (string, error) {
	engine = strings.TrimSpace(engine)
	if engine == "" {
		return body, nil
	}
	if engine != templateengine.EngineSimple && engine != templateengine.EngineGoTemplate {
		return "", &domain.CommandError{Code: "template_engine_invalid", Message: "template engine is unsupported", Hint: "Use simple or go-template"}
	}
	if strings.HasPrefix(body, "---\n") || engine == templateengine.EngineSimple {
		return body, nil
	}
	return "---\nschema_version: pinax.template.v2\nengine: " + engine + "\nkind: note\n---\n\n" + body, nil
}

func parseTemplateForProjection(root, name string) (templateengine.TemplateDocument, error) {
	body, err := loadTemplate(root, name)
	if err != nil {
		return templateengine.TemplateDocument{}, err
	}
	doc, err := templateengine.ParseDocument(name, body)
	if err != nil {
		return templateengine.TemplateDocument{}, templateEngineCommandError(err)
	}
	return doc, nil
}

func templateProjectionMetadata(root, name string, meta templateengine.Metadata) (templateengine.Metadata, string) {
	source := templateSource(root, name)
	workflow := templateWorkflowMetadata(name, source, meta)
	if source == "vault-local" {
		if _, ok := builtInTemplates()[name]; ok {
			workflow.Lifecycle = "overridden"
		}
	}
	return workflow, source
}

func templateSource(root, name string) string {
	path, err := templatePath(root, name)
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return "vault-local"
		}
	}
	if _, ok := builtInTemplates()[name]; ok {
		return "builtin"
	}
	return "unknown"
}

func requiredTemplateVariables(meta templateengine.Metadata) []string {
	vars := make([]string, 0, len(meta.VariableSchema))
	for key, variable := range meta.VariableSchema {
		if variable.Required {
			vars = append(vars, key)
		}
	}
	sort.Strings(vars)
	return vars
}

func missingTemplateVariables(meta templateengine.Metadata, values map[string]string) []string {
	required := requiredTemplateVariables(meta)
	missing := make([]string, 0, len(required))
	for _, key := range required {
		if values == nil || strings.TrimSpace(values[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
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
		return "", &domain.CommandError{Code: "template_source_conflict", Message: "template create can use only one template source", Hint: "Keep only one of --from, --body, or --stdin"}
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
	return fmt.Sprintf("---\nschema_version: pinax.template_design.v1\nkind: template_design\ntitle: %s\n---\n\n## Template Body\n\n# {{title}}\n", name)
}

func templateHasDesignFrontmatter(body string) bool {
	return strings.Contains(body, "schema_version: pinax.template_design.v1") && strings.Contains(body, "kind: template_design")
}

func validateTemplateVars(vars map[string]string) error {
	for key := range vars {
		if !templateVariableNamePattern.MatchString(key) {
			return &domain.CommandError{Code: "template_variable_invalid", Message: "template variable key is invalid", Hint: "Use --var key=value; key may only contain letters, numbers, _, :, or -, and cannot start with a number"}
		}
	}
	return nil
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

func validateTemplateContent(body string, req TemplateRequest) []domain.Issue {
	issues := make([]domain.Issue, 0)
	if strings.TrimSpace(body) == "" {
		issues = append(issues, domain.Issue{Code: "template_empty", Message: "Template is empty"})
	}
	doc, err := templateengine.ParseDocument(req.Name, body)
	if err != nil {
		issues = append(issues, domain.Issue{Code: templateengine.ErrorCode(err), Message: err.Error()})
	}
	for _, issue := range doc.Issues {
		issues = append(issues, domain.Issue{Code: issue.Code, Message: issue.Message})
	}
	// 代码围栏是 Markdown/Mermaid/YAML 模板最容易破坏生成结果的地方；这里仅跟踪 fence 奇偶，保持实现可审计且不解析 Markdown 全语法。
	fenceOpen := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenceOpen = !fenceOpen
		}
	}
	if fenceOpen {
		issues = append(issues, domain.Issue{Code: "template_fence_unclosed", Message: "Markdown code fence is unclosed"})
	}
	if err := validateTemplateVars(req.Vars); err != nil {
		issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: err.Error()})
	}
	if doc.Engine != templateengine.EngineGoTemplate {
		for _, key := range templateVariables(body) {
			if !templateVariableNamePattern.MatchString(key) {
				issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: "template variable key is invalid: " + key})
			}
		}
	}
	return issues
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
		return "", &domain.CommandError{Code: "template_not_found", Message: "Template not found", Hint: "Run pinax template init to initialize built-in templates"}
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Service) renderTemplateBody(ctx context.Context, root string, req TemplateRequest, lazyIndex bool) (string, error) {
	body, err := loadTemplate(root, req.Name)
	if err != nil {
		return "", err
	}
	for _, issue := range validateTemplateContent(body, TemplateRequest{Name: req.Name, Vars: req.Vars}) {
		if issue.Code == "template_variable_invalid" {
			return "", &domain.CommandError{Code: issue.Code, Message: issue.Message, Hint: "Use --var key=value; key may only contain letters, numbers, _, :, or -, and cannot start with a number"}
		}
		if issue.Code == "template_frontmatter_unclosed" || issue.Code == "template_fence_unclosed" || issue.Code == "template_schema_invalid" {
			return "", &domain.CommandError{Code: "template_invalid", Message: issue.Message, Hint: "Run pinax template validate <name> first to fix the template"}
		}
	}
	doc, err := templateengine.ParseDocument(req.Name, body)
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	if templateDocumentIsDesignDraft(doc) {
		return "", &domain.CommandError{Code: "template_design_not_executable", Message: "Template is still a draft and cannot be used for preview, render, or note creation", Hint: "Publish the draft as an executable schema_version: pinax.template.v2 template first"}
	}
	req = applyTemplateExample(req, doc.Metadata)
	renderCtx := templateEngineContext(req)
	queries, err := s.executeTemplateQueries(ctx, root, doc.Metadata.Queries, lazyIndex)
	if err != nil {
		return "", err
	}
	renderCtx.Queries = queries
	rendered, err := templateengine.New().Render(doc, renderCtx)
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	return rendered.Body, nil
}

func templateDocumentIsDesignDraft(doc templateengine.TemplateDocument) bool {
	if doc.Metadata.SchemaVersion == "pinax.template_design.v1" || doc.Metadata.Kind == "template_design" || doc.Metadata.Lifecycle == "draft_design" {
		return true
	}
	for _, issue := range doc.Issues {
		if issue.Code == "template_design_legacy" {
			return true
		}
	}
	return false
}

func missingTemplateVariableCommand(root string, req TemplateRequest, command string) string {
	variable := "key"
	if doc, err := parseTemplateForProjection(root, req.Name); err == nil {
		for key, meta := range doc.Metadata.Variables {
			if meta.Required {
				if req.Vars == nil || req.Vars[key] == "" {
					variable = key
					break
				}
			}
		}
	}
	verb := "render"
	if command == "template.preview" {
		verb = "preview"
	}
	return fmt.Sprintf("pinax template %s %s --var %s=... --vault %s --json", verb, shellQuote(req.Name), variable, shellQuote(root))
}

func renderTemplateOutputPath(doc templateengine.TemplateDocument, req CreateNoteRequest) (string, error) {
	pattern := strings.TrimSpace(doc.Metadata.Output.PathPattern)
	if pattern == "" {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template is missing output.path_pattern", Hint: "Check template metadata"}
	}
	rendered, err := templateengine.New().Render(templateengine.TemplateDocument{Name: doc.Name + ":output", Engine: doc.Engine, Body: pattern}, templateengine.Context{Title: req.Title, Project: req.Project, Tags: req.Tags, Vars: req.Vars})
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	rel := strings.TrimSpace(rendered.Body)
	if rel == "" {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template output.path_pattern generated an empty path", Hint: "Check template output.path_pattern"}
	}
	if filepath.Ext(rel) == "" {
		rel += ".md"
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if templateOutputPathForbidden(rel) {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template output path is outside allowed vault content areas", Hint: "Use a root-relative Markdown path like inbox/{{ .Title }}.md"}
	}
	return rel, nil
}

func templateOutputPathForbidden(rel string) bool {
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) || filepath.Ext(rel) != ".md" {
		return true
	}
	first := rel
	if before, _, ok := strings.Cut(rel, "/"); ok {
		first = before
	}
	switch first {
	case ".pinax", ".git", "attachments", "temp", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func (s *Service) explainTemplateQueries(ctx context.Context, queries map[string]templateengine.TemplateQueryDeclaration) map[string]domain.Projection {
	if len(queries) == 0 {
		return nil
	}
	explained := make(map[string]domain.Projection, len(queries))
	for name, query := range queries {
		projection, err := s.QueryExplain(ctx, QueryRequest{SQL: query.SQL})
		if err != nil {
			projection = domain.NewErrorProjection("query.explain", templateQueryCommandError(name, err))
		}
		explained[name] = projection
	}
	return explained
}

func (s *Service) executeTemplateQueries(ctx context.Context, root string, queries map[string]templateengine.TemplateQueryDeclaration, lazyIndex bool) (map[string]templateengine.QueryResult, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if !lazyIndex {
		notes, err := scanNotes(root)
		if err != nil {
			return nil, err
		}
		status, _ := noteindex.Inspect(root, ordinaryNotes(notes))
		if status.Status != "fresh" {
			return nil, &domain.CommandError{Code: "template_index_required", Message: "Template preview query requires a fresh local index", Hint: "Run pinax index rebuild --vault " + shellQuote(root)}
		}
	}
	results := make(map[string]templateengine.QueryResult, len(queries))
	for name, query := range queries {
		limit := query.MaxRows
		if limit <= 0 {
			limit = 50
		}
		projection, err := s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: query.SQL, Limit: limit, LazyIndex: lazyIndex})
		if err != nil {
			if query.Required {
				return nil, templateQueryCommandError(name, err)
			}
			continue
		}
		results[name] = templateQueryResultFromProjection(projection)
	}
	return results, nil
}

func applyTemplateExample(req TemplateRequest, meta templateengine.Metadata) TemplateRequest {
	if req.Title == "" && meta.Example.Title != "" {
		req.Title = meta.Example.Title
	}
	if req.Project == "" && meta.Example.Project != "" {
		req.Project = meta.Example.Project
	}
	if len(req.Tags) == 0 && len(meta.Example.Tags) > 0 {
		req.Tags = append([]string(nil), meta.Example.Tags...)
	}
	if len(meta.Example.Vars) > 0 {
		merged := make(map[string]string, len(meta.Example.Vars)+len(req.Vars))
		for key, value := range meta.Example.Vars {
			merged[key] = value
		}
		for key, value := range req.Vars {
			merged[key] = value
		}
		req.Vars = merged
	}
	return req
}

func templateQueryCommandError(name string, err error) *domain.CommandError {
	message := "Template query failed: " + name
	if err != nil && err.Error() != "" {
		message = message + ": " + err.Error()
	}
	return &domain.CommandError{Code: "template_query_execute_failed", Message: message, Hint: "Run pinax query explain <sql> --vault <vault> to check the query, or run pinax index sync --vault <vault> and retry"}
}

func templateQueryResultFromProjection(projection domain.Projection) templateengine.QueryResult {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return templateengine.QueryResult{}
	}
	result, ok := data["result"].(domain.TableResult)
	if !ok {
		return templateengine.QueryResult{}
	}
	converted := templateengine.QueryResult{Columns: result.Columns, Rows: make([]map[string]string, 0, len(result.Rows))}
	for _, row := range result.Rows {
		values := make(map[string]string, len(result.Columns))
		for _, column := range result.Columns {
			switch column {
			case "title":
				values[column] = row.Note.Title
			case "path":
				values[column] = row.Note.Path
			case "id":
				values[column] = row.Note.ID
			case "project", "group":
				values[column] = row.Note.Project
			case "kind":
				values[column] = row.Note.Kind
			case "status":
				values[column] = row.Note.Status
			case "tags":
				values[column] = strings.Join(row.Note.Tags, ", ")
			default:
				if value, ok := row.Values[column]; ok {
					values[column] = value.String()
				}
			}
		}
		converted.Rows = append(converted.Rows, values)
	}
	return converted
}

func templateEngineContext(req TemplateRequest) templateengine.Context {
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	datetime := now.Format(time.RFC3339)
	if req.Title == "" {
		req.Title = "Untitled"
	}
	return templateengine.Context{
		Title:    req.Title,
		Date:     date,
		DateTime: datetime,
		Project:  req.Project,
		Tags:     cleanTags(req.Tags),
		Vars:     req.Vars,
	}
}

func templateEngineCommandError(err error) error {
	code := templateengine.ErrorCode(err)
	switch code {
	case "template_variable_missing":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Use --var key=value to provide the missing variable"}
	case "template_parse_failed", "template_schema_invalid", "template_frontmatter_unclosed":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Fix the template and retry"}
	case "template_render_failed":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Check template context and function arguments"}
	default:
		return err
	}
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

func splitCommaValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func normalizeTagsForWrite(tags []string) ([]string, *domain.CommandError) {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, raw := range tags {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		tag := strings.TrimPrefix(trimmed, "#")
		if tag == "" {
			return nil, invalidTagError(raw)
		}
		if !isSafeTagValue(tag) {
			return nil, invalidTagError(raw)
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		cleaned = append(cleaned, tag)
	}
	return cleaned, nil
}

func isSafeTagValue(tag string) bool {
	for _, r := range tag {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			continue
		}
		switch r {
		case '_', '-', '/':
			continue
		default:
			return false
		}
	}
	return true
}

func invalidTagError(tag string) *domain.CommandError {
	return &domain.CommandError{Code: "invalid_tag", Message: "tag may only contain letters, numbers, CJK characters, _, -, or /, and cannot contain YAML structural characters, commas, whitespace, or control characters", Hint: "For example, pinax note tag add <note> research/work --vault <vault>"}
}

func normalizePropertyKey(raw string) (string, *domain.CommandError) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", invalidPropertyKeyError(raw)
	}
	blocked := map[string]string{
		"schema_version": "schema_version is managed by Pinax",
		"note_id":        "note_id is managed by the record ledger",
		"tags":           "Use pinax note tag add|remove|set for tags",
		"title":          "Use pinax note rename for title",
		"created_at":     "created_at is maintained by Pinax at creation time",
		"updated_at":     "updated_at is maintained by Pinax write commands",
	}
	if reason := blocked[key]; reason != "" {
		return "", &domain.CommandError{Code: "reserved_property", Message: "Reserved property cannot be modified through note property: " + key, Hint: reason}
	}
	for i, r := range key {
		if i == 0 && r != '_' && !unicode.IsLetter(r) {
			return "", invalidPropertyKeyError(key)
		}
		if r != '_' && r != '-' && r != '.' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", invalidPropertyKeyError(key)
		}
	}
	return key, nil
}

func invalidPropertyKeyError(key string) *domain.CommandError {
	return &domain.CommandError{Code: "invalid_property", Message: "property key may only contain letters, numbers, _, -, or ., and cannot start with a number or symbol", Hint: "For example, pinax note property set <note> priority 2 --vault <vault>"}
}

func formatPropertyValue(raw string) (string, *domain.CommandError) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &domain.CommandError{Code: "invalid_property_value", Message: "property value cannot be empty", Hint: "To delete a property, use pinax note property remove <note> <property> --vault <vault>"}
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return "", &domain.CommandError{Code: "invalid_property_value", Message: "property value cannot contain newlines or control characters", Hint: "Use a single-line scalar value; put complex content in the body"}
		}
	}
	if propertyValueNeedsQuote(value) {
		return quoteFrontmatterValue(value), nil
	}
	return value, nil
}

func propertyValueNeedsQuote(value string) bool {
	if strings.TrimSpace(value) != value {
		return true
	}
	return strings.ContainsAny(value, ":#[]{}&*!|>'\"%@`,")
}

func quoteFrontmatterValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + replacer.Replace(value) + "\""
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
		return "", "", &domain.CommandError{Code: "invalid_sync_target", Message: "sync target only supports git, s3, or cloud", Hint: "pinax sync diff --target git"}
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
	projection := domain.NewProjection("sync."+direction, "Sync status recorded; remote writes have not executed.")
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
		return "", &domain.CommandError{Code: "unsafe_path", Message: "Path escapes the vault boundary"}
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	clean := filepath.Clean(path)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "Path escapes the vault boundary"}
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
	if _, err := file.Write(append(b, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func errorProjection(command string, err error) domain.Projection {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		projection := domain.NewErrorProjection(command, commandErr)
		var resolverAmbiguous *resolverNoteAmbiguousError
		if errors.As(err, &resolverAmbiguous) {
			projection.Facts["candidates"] = fmt.Sprint(len(resolverAmbiguous.Result.Candidates))
			projection.Data = map[string]any{"candidates": resolverAmbiguous.Result.Candidates}
		}
		return projection
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

// BackendObjectListRequest 描述 backend object list 请求。
type BackendObjectListRequest struct {
	VaultPath string
	Name      string
	Prefix    string
}

// BackendObjectStatRequest 描述 backend object stat 请求。
type BackendObjectStatRequest struct {
	VaultPath string
	Name      string
	Key       string
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
	projection := domain.NewProjection("backend.list", "Backend list read.")
	projection.Facts["vault"] = root
	projection.Facts["backends"] = fmt.Sprint(len(registry.Backends))
	if registry.DefaultBackend != "" {
		projection.Facts["default_backend"] = registry.DefaultBackend
	}
	projection.Data = map[string]any{"registry": registry}
	projection.Actions = []domain.Action{{Name: "add", Command: fmt.Sprintf("pinax backend add s3 work-s3 --bucket <bucket> --region <region> --vault %s", shellQuote(root))}}
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
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend add requires a name", Hint: "pinax backend add <kind> <name> --vault <vault>"}
		return domain.NewErrorProjection("backend.add", err), err
	}
	kind := domain.BackendKind(strings.TrimSpace(req.Kind))
	if !domain.IsValidBackendKind(string(kind)) {
		err := &domain.CommandError{Code: "backend_kind_invalid", Message: "Unknown backend type", Hint: "Use local, s3, rclone, or onedrive"}
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
			return saveBackendRegistryProjection(root, registry, profile, "backend.add", "Backend updated.")
		}
	}
	registry.Backends = append(registry.Backends, profile)
	if registry.DefaultBackend == "" {
		registry.DefaultBackend = name
	}
	return saveBackendRegistryProjection(root, registry, profile, "backend.add", "Backend added.")
}

// BackendShow 查看单个 backend 状态。
func (s *Service) BackendShow(_ context.Context, req BackendRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.show", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.show", err), err
	}
	profile, err := findBackendProfile(registry, req.Name)
	if err != nil {
		return errorProjection("backend.show", err), err
	}
	projection := domain.NewProjection("backend.show", "Backend status read.")
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
			issues = append(issues, domain.Issue{Code: "missing_bucket", Path: ".pinax/backends.json", Message: "S3 backend is missing bucket"})
		}
		if profile.Region == "" {
			issues = append(issues, domain.Issue{Code: "missing_region", Path: ".pinax/backends.json", Message: "S3 backend is missing region"})
		}
	case domain.BackendRclone, domain.BackendOneDrive:
		if profile.Remote == "" {
			issues = append(issues, domain.Issue{Code: "missing_remote", Path: ".pinax/backends.json", Message: string(profile.Kind) + " backend is missing remote"})
		}
	}
	projection := domain.NewProjection("backend.doctor", "Backend diagnostics completed.")
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
	projection := domain.NewProjection("backend.capabilities", "Backend capabilities listed.")
	projection.Facts["name"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["capabilities"] = fmt.Sprint(len(capabilities))
	projection.Data = map[string]any{"profile": profile, "capabilities": capabilities}
	return projection, nil
}

// BackendDiff 生成 dry-run SyncPlan。
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
	// MVP: diff 生成空Plan，只记录 backend 和方向。
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
	projection := domain.NewProjection("backend.diff", "Backend diff plan generated.")
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["direction"] = direction
	projection.Facts["items"] = "0"
	projection.Facts["conflicts"] = "0"
	projection.Facts["dry_run"] = "true"
	projection.Data = map[string]any{"plan": plan, "profile": profile}
	projection.Actions = []domain.Action{
		{Name: "push", Command: fmt.Sprintf("pinax backend push %s --vault %s --dry-run", shellQuote(profile.Name), shellQuote(root))},
		{Name: "pull", Command: fmt.Sprintf("pinax backend pull %s --vault %s --dry-run", shellQuote(profile.Name), shellQuote(root))},
	}
	return projection, nil
}

// BackendPush 执行 push SyncPlan。
func (s *Service) BackendPush(_ context.Context, req BackendPlanRequest) (domain.Projection, error) {
	return s.backendSync(req, "push")
}

// BackendPull 执行 pull SyncPlan。
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
		projection := domain.NewProjection("backend."+direction, "Backend "+direction+" dry-run generated.")
		projection.Facts["backend"] = profile.Name
		projection.Facts["kind"] = string(profile.Kind)
		projection.Facts["direction"] = direction
		projection.Facts["dry_run"] = "true"
		projection.Data = map[string]any{"backend": profile.Name, "direction": direction, "dry_run": true}
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "backend " + direction + " requires --yes", Hint: "Preview the plan with --dry-run first, then add --yes after confirming"}
		return domain.NewErrorProjection("backend."+direction, err), err
	}
	// MVP: 真实 push/pull 需要后端 adapter 实现，当前只记录事件。
	_ = appendEvent(root, "backend."+direction, "success", map[string]string{"backend": profile.Name, "kind": string(profile.Kind), "direction": direction})
	projection := domain.NewProjection("backend."+direction, fmt.Sprintf("Backend %s recorded.", direction))
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
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend remove requires a backend name", Hint: "pinax backend remove <name> --vault <vault> --yes"}
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
		err := &domain.CommandError{Code: "backend_not_found", Message: "Backend not found", Hint: "Run pinax backend list to view available backends"}
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
	projection := domain.NewProjection("backend.remove", "Backend removed.")
	projection.Facts["name"] = req.Name
	projection.Facts["backends"] = fmt.Sprint(len(registry.Backends))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "backends.json"))}
	return projection, nil
}

// BackendObjectList 列出 backend 对象。
func (s *Service) BackendObjectList(ctx context.Context, req BackendObjectListRequest) (domain.Projection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend object list requires a backend name", Hint: "pinax backend object list <name> [prefix] --vault <vault>"}
		return errorProjection("backend.object.list", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.object.list", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.object.list", err), err
	}
	profile, err := findBackendProfile(registry, name)
	if err != nil {
		return errorProjection("backend.object.list", err), err
	}
	store, err := backendBlobStore(ctx, root, profile)
	if err != nil {
		return errorProjection("backend.object.list", err), err
	}
	extended, ok := store.(pinaxcloud.ExtendedBlobStore)
	if !ok {
		err := &domain.CommandError{Code: "backend_list_unsupported", Message: "backend does not support object listing", Hint: "Run pinax backend capabilities <name> to view capabilities"}
		return errorProjection("backend.object.list", err), err
	}
	objects, err := extended.List(ctx, req.Prefix)
	if err != nil {
		return errorProjection("backend.object.list", err), err
	}
	projection := domain.NewProjection("backend.object.list", "Backend object list read.")
	projection.Facts["backend"] = name
	projection.Facts["prefix"] = req.Prefix
	projection.Facts["objects"] = fmt.Sprint(len(objects))
	projection.Data = map[string]any{"backend": name, "prefix": req.Prefix, "objects": objects}
	return projection, nil
}

// BackendObjectStat 查看 backend 对象状态。
func (s *Service) BackendObjectStat(ctx context.Context, req BackendObjectStatRequest) (domain.Projection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "backend_name_required", Message: "backend object stat requires a backend name", Hint: "pinax backend object stat <name> <key> --vault <vault>"}
		return errorProjection("backend.object.stat", err), err
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		err := &domain.CommandError{Code: "key_required", Message: "backend object stat requires a key", Hint: "pinax backend object stat <name> <key> --vault <vault>"}
		return errorProjection("backend.object.stat", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("backend.object.stat", err), err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return errorProjection("backend.object.stat", err), err
	}
	profile, err := findBackendProfile(registry, name)
	if err != nil {
		return errorProjection("backend.object.stat", err), err
	}
	store, err := backendBlobStore(ctx, root, profile)
	if err != nil {
		return errorProjection("backend.object.stat", err), err
	}
	revision, err := store.Stat(ctx, key)
	if errors.Is(err, pinaxcloud.ErrObjectNotFound) {
		commandErr := &domain.CommandError{Code: "object_not_found", Message: "Backend object not found: " + key, Hint: "Run pinax backend object list <name> to view objects"}
		return domain.NewErrorProjection("backend.object.stat", commandErr), commandErr
	}
	if err != nil {
		return errorProjection("backend.object.stat", err), err
	}
	projection := domain.NewProjection("backend.object.stat", "Backend object status read.")
	projection.Facts["backend"] = name
	projection.Facts["key"] = key
	projection.Facts["revision"] = revision
	projection.Data = map[string]any{"backend": name, "key": key, "revision": revision}
	return projection, nil
}

func backendBlobStore(ctx context.Context, root string, profile domain.BackendProfile) (pinaxcloud.BlobStore, error) {
	switch profile.Kind {
	case domain.BackendLocal:
		backendRoot := strings.TrimSpace(profile.Root)
		if backendRoot == "" {
			return nil, &domain.CommandError{Code: "backend_config_incomplete", Message: "local backend requires root", Hint: "pinax backend add local <name> --root <path>"}
		}
		if !filepath.IsAbs(backendRoot) {
			backendRoot = filepath.Join(root, backendRoot)
		}
		return pinaxcloud.NewFileBackend(backendRoot)
	case domain.BackendS3:
		if strings.TrimSpace(profile.Bucket) == "" {
			return nil, &domain.CommandError{Code: "backend_config_incomplete", Message: "S3 backend requires bucket", Hint: "pinax backend add s3 <name> --bucket <bucket> --region <region>"}
		}
		return pinaxcloud.NewS3Backend(ctx, profile.Bucket, profile.Prefix)
	default:
		return nil, &domain.CommandError{Code: "backend_store_unsupported", Message: string(profile.Kind) + " backend does not yet support object read/write", Hint: "Use local or s3 backend, or run backend capabilities to view capabilities"}
	}
}

// validateBackendProfileFields 按 kind 校验必填字段。
func validateBackendProfileFields(kind domain.BackendKind, req BackendAddRequest) error {
	switch kind {
	case domain.BackendS3:
		if strings.TrimSpace(req.Bucket) == "" || strings.TrimSpace(req.Region) == "" {
			return &domain.CommandError{Code: "backend_config_incomplete", Message: "S3 backend requires --bucket and --region", Hint: "pinax backend add s3 <name> --bucket <bucket> --region <region>"}
		}
	case domain.BackendRclone, domain.BackendOneDrive:
		if strings.TrimSpace(req.Remote) == "" {
			return &domain.CommandError{Code: "backend_config_incomplete", Message: string(kind) + " backend requires --remote", Hint: fmt.Sprintf("pinax backend add %s <name> --remote <remote>", kind)}
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
	return domain.BackendProfile{}, &domain.CommandError{Code: "backend_not_found", Message: "Backend not found", Hint: "Run pinax backend list to view available backends"}
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
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax backend show %s --vault %s", shellQuote(profile.Name), shellQuote(root))}}
	return projection, nil
}

// PlanningRequest 描述 plan 命令通用请求。
type PlanningRequest struct {
	VaultPath      string
	Period         string // daily, weekly, monthly
	WithTaskBridge bool
	TaskReview     bool
	DryRun         bool
	Yes            bool
	Save           bool
	FromPeriod     string // for plan actions --from
}

// PlanDaily 生成每日Plan。
func (s *Service) PlanDaily(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningDaily)
}

// PlanWeekly 生成每周Plan。
func (s *Service) PlanWeekly(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningWeekly)
}

// PlanMonthly 生成每月Plan。
func (s *Service) PlanMonthly(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	return s.planPeriod(ctx, req, domain.PlanningMonthly)
}

func (s *Service) planPeriod(ctx context.Context, req PlanningRequest, period domain.PlanningPeriod) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan."+string(period), err), err
	}
	// 生成 planning snapshot。
	now := currentTimeUTC()
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
	if board, rel, ok, err := latestProjectBoardSnapshot(root); err != nil {
		return errorProjection("plan."+string(period), err), err
	} else if ok {
		mergeProjectBoardPlanningFacts(&snapshot, board, rel)
	}
	maxCommitments := planningops.MaxCommitments(period)
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
	planningops.AddCapacityRisk(&snapshot, &decision, len(facts), maxCommitments)
	command := "plan." + string(period)
	if req.WithTaskBridge && period == domain.PlanningDaily {
		taskBridge, err := loadTaskBridgeDaily(ctx, now)
		if err != nil {
			return errorProjection(command, err), err
		}
		applyTaskBridgePlanning(&snapshot, &decision, taskBridge, maxCommitments)
	}
	if req.TaskReview && period == domain.PlanningDaily {
		return s.planDailyTaskReview(ctx, root, now, snapshot, decision, req.Yes)
	}
	targetNote := filepath.ToSlash(filepath.Join("daily", now.Format("2006-01-02")+".md"))
	if req.DryRun || !req.Yes {
		projection := domain.NewProjection(command, string(period)+" plan previewed.")
		projection.Facts["period"] = string(period)
		projection.Facts["dry_run"] = "true"
		projection.Facts["snapshot_id"] = snapshot.SnapshotID
		projection.Facts["decision_id"] = decision.DecisionID
		projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
		projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
		if req.WithTaskBridge && period == domain.PlanningDaily {
			projection.Facts["source"] = "taskbridge"
			projection.Facts["captured_at"] = snapshot.CapturedAt
			projection.Facts["target_note"] = targetNote
			projection.Facts["managed_block"] = planningDailyBlockName
			projection.Facts["selected_commitments"] = fmt.Sprint(len(decision.Selected))
			projection.Facts["taskbridge_tasks"] = snapshot.Facts["taskbridge_tasks"]
		}
		copyProjectBoardPlanningFacts(&projection, snapshot)
		projection.Data = map[string]any{"snapshot": snapshot, "decision": decision}
		applyCommand := fmt.Sprintf("pinax plan %s --vault %s --yes", string(period), shellQuote(root))
		if req.WithTaskBridge && period == domain.PlanningDaily {
			applyCommand = fmt.Sprintf("pinax plan daily --taskbridge --vault %s --yes", shellQuote(root))
		}
		projection.Actions = []domain.Action{
			{Name: "apply", Command: applyCommand},
		}
		return projection, nil
	}
	if req.WithTaskBridge && period == domain.PlanningDaily {
		markdown := renderTaskBridgeDailyMarkdown(snapshot, decision)
		dailyRel, err := writeDailyPlanningBlock(root, now, markdown)
		if err != nil {
			return errorProjection(command, err), err
		}
		snapshot.Facts["target_note"] = dailyRel
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
	projection := domain.NewProjection(command, string(period)+" plan generated.")
	projection.Facts["period"] = string(period)
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["decision_id"] = decision.DecisionID
	projection.Facts["max_commitments"] = fmt.Sprint(maxCommitments)
	projection.Facts["risks"] = fmt.Sprint(len(snapshot.Risks))
	if req.WithTaskBridge && period == domain.PlanningDaily {
		projection.Facts["source"] = "taskbridge"
		projection.Facts["captured_at"] = snapshot.CapturedAt
		projection.Facts["target_note"] = snapshot.Facts["target_note"]
		projection.Facts["managed_block"] = planningDailyBlockName
		projection.Facts["selected_commitments"] = fmt.Sprint(len(decision.Selected))
		projection.Facts["taskbridge_tasks"] = snapshot.Facts["taskbridge_tasks"]
	}
	copyProjectBoardPlanningFacts(&projection, snapshot)
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
func (s *Service) PlanActions(ctx context.Context, req PlanningRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("plan.actions", err), err
	}
	now := currentTimeUTC()
	period := strings.TrimSpace(req.FromPeriod)
	if period == "" {
		period = "daily"
	}
	planningPeriod, err := planningops.ParsePeriod(period)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	preview, err := s.planPeriod(ctx, PlanningRequest{VaultPath: root, Period: period, WithTaskBridge: req.WithTaskBridge, DryRun: true}, planningPeriod)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	snapshot, decision, err := planningops.PreviewData(preview)
	if err != nil {
		return errorProjection("plan.actions", err), err
	}
	draft := planningops.BuildActionDraft(period, snapshot, decision, now)
	if req.DryRun || !req.Save {
		projection := domain.NewProjection("plan.actions", "Action draft previewed.")
		projection.Facts["period"] = period
		projection.Facts["dry_run"] = "true"
		projection.Facts["action_id"] = draft.ActionID
		projection.Facts["source_decision"] = draft.SourceDecision
		projection.Facts["snapshot_id"] = draft.SourceSnapshot
		projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
		if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
			projection.Facts["source"] = "taskbridge"
		}
		projection.Data = map[string]any{"draft": draft}
		saveCommand := fmt.Sprintf("pinax plan actions --from %s --vault %s --save", period, shellQuote(root))
		if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
			saveCommand = fmt.Sprintf("pinax plan actions --from daily --taskbridge --vault %s --save", shellQuote(root))
		}
		projection.Actions = []domain.Action{
			{Name: "save", Command: saveCommand},
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
	projection := domain.NewProjection("plan.actions", "Action draft saved.")
	projection.Facts["action_id"] = draft.ActionID
	projection.Facts["source_decision"] = draft.SourceDecision
	projection.Facts["snapshot_id"] = draft.SourceSnapshot
	projection.Facts["tasks"] = fmt.Sprint(len(draft.Tasks))
	projection.Facts["saved_path"] = rel
	if req.WithTaskBridge && planningPeriod == domain.PlanningDaily {
		projection.Facts["source"] = "taskbridge"
	}
	projection.Evidence = []string{rel}
	projection.Data = map[string]any{"draft": draft}
	projection.Actions = []domain.Action{
		{Name: "execute", Command: fmt.Sprintf("taskbridge agent execute --action-file %s --dry-run", rel)},
	}
	return projection, nil
}

// PlanSnapshot 生成Plan快照。
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
	projection := domain.NewProjection("plan.snapshot", "Planning snapshot saved.")
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

//nolint:unused // Retained for deterministic IDs in older planning receipts.
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
