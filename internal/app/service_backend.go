package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

// Backend (cloud storage provider) integration: profile registry, sync planning,
// object/note listing, and doctor/diff/push/pull operations against S3-compatible
// and file backends. Extracted from service.go to keep the backend adapter surface
// in one cohesive file.

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

// BackendNotesRequest describes read-only note-oriented backend inspection.
type BackendNotesRequest struct {
	VaultPath string
	Name      string
	Prefix    string
	Path      string
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
		return backendProviderErrorProjection("backend.object.list", root, profile, err)
	}
	projection := domain.NewProjection("backend.object.list", "Backend object list read.")
	projection.Facts["backend"] = name
	projection.Facts["prefix"] = req.Prefix
	projection.Facts["objects"] = fmt.Sprint(len(objects))
	projection.Data = map[string]any{"backend": name, "prefix": req.Prefix, "objects": backendObjectRows(objects)}
	return projection, nil
}

// BackendNotesSummary summarizes note-like objects available through a backend.
func (s *Service) BackendNotesSummary(ctx context.Context, req BackendNotesRequest) (domain.Projection, error) {
	root, profile, objects, err := s.listBackendObjects(ctx, req.Name, req.VaultPath, req.Prefix, "backend.notes.summary")
	if err != nil {
		return errorProjection("backend.notes.summary", err), err
	}
	notes := backendNoteRows(objects)
	var bytes int64
	newest := ""
	for _, obj := range objects {
		bytes += obj.Size
		if !obj.LastModified.IsZero() {
			updated := obj.LastModified.UTC().Format(time.RFC3339)
			if newest == "" || updated > newest {
				newest = updated
			}
		}
	}
	projection := domain.NewProjection("backend.notes.summary", "Backend note summary read.")
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["prefix"] = req.Prefix
	projection.Facts["objects"] = fmt.Sprint(len(objects))
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["bytes"] = fmt.Sprint(bytes)
	if newest != "" {
		projection.Facts["newest_updated_at"] = newest
	}
	projection.Data = map[string]any{"backend": profile.Name, "kind": string(profile.Kind), "prefix": req.Prefix, "objects": len(objects), "notes": len(notes), "bytes": bytes, "newest_updated_at": newest}
	projection.Actions = []domain.Action{{Name: "list", Command: fmt.Sprintf("pinax backend notes list %s --vault %s", shellQuote(profile.Name), shellQuote(root))}}
	return projection, nil
}

// BackendNotesList lists note-like Markdown objects available through a backend.
func (s *Service) BackendNotesList(ctx context.Context, req BackendNotesRequest) (domain.Projection, error) {
	_, profile, objects, err := s.listBackendObjects(ctx, req.Name, req.VaultPath, req.Prefix, "backend.notes.list")
	if err != nil {
		return errorProjection("backend.notes.list", err), err
	}
	notes := backendNoteRows(objects)
	projection := domain.NewProjection("backend.notes.list", "Backend note list read.")
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	projection.Facts["prefix"] = req.Prefix
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["objects"] = fmt.Sprint(len(objects))
	projection.Data = map[string]any{"backend": profile.Name, "kind": string(profile.Kind), "prefix": req.Prefix, "notes": notes, "objects": len(objects)}
	return projection, nil
}

// BackendNotesStat views one note-like object status through a backend.
func (s *Service) BackendNotesStat(ctx context.Context, req BackendNotesRequest) (domain.Projection, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		err := &domain.CommandError{Code: "note_path_required", Message: "backend notes stat requires a note path", Hint: "pinax backend notes stat <name> <path> --vault <vault>"}
		return errorProjection("backend.notes.stat", err), err
	}
	root, profile, err := s.backendProfile(req.Name, req.VaultPath, "backend.notes.stat")
	if err != nil {
		return errorProjection("backend.notes.stat", err), err
	}
	store, err := backendBlobStore(ctx, root, profile)
	if err != nil {
		return errorProjection("backend.notes.stat", err), err
	}
	key := backendNoteKey(req.Prefix, path)
	revision, err := store.Stat(ctx, key)
	if errors.Is(err, pinaxcloud.ErrObjectNotFound) {
		commandErr := &domain.CommandError{Code: "note_object_not_found", Message: "Backend note object not found: " + path, Hint: "Run pinax backend notes list <name> to view remote note objects"}
		return domain.NewErrorProjection("backend.notes.stat", commandErr), commandErr
	}
	if err != nil {
		return backendProviderErrorProjection("backend.notes.stat", root, profile, err)
	}
	projection := domain.NewProjection("backend.notes.stat", "Backend note status read.")
	projection.Facts["backend"] = profile.Name
	projection.Facts["path"] = path
	projection.Facts["key"] = key
	projection.Facts["revision"] = revision
	projection.Data = map[string]any{"backend": profile.Name, "path": path, "key": key, "revision": revision}
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
		return pinaxcloud.NewS3BackendWithOptions(ctx, profile.Bucket, profile.Prefix, backendS3Options(profile))
	default:
		return nil, &domain.CommandError{Code: "backend_store_unsupported", Message: string(profile.Kind) + " backend does not yet support object read/write", Hint: "Use local or s3 backend, or run backend capabilities to view capabilities"}
	}
}

func backendS3Options(profile domain.BackendProfile) pinaxcloud.S3BackendOptions {
	endpoint := strings.TrimSpace(profile.Endpoint)
	return pinaxcloud.S3BackendOptions{
		EndpointURL: endpoint,
		Region:      strings.TrimSpace(profile.Region),
		Profile:     strings.TrimSpace(profile.Profile),
		PathStyle:   backendS3PathStyle(endpoint),
	}
}

func backendS3PathStyle(endpoint string) bool {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	if endpoint == "" {
		return false
	}
	if strings.Contains(endpoint, ".myqcloud.com") || strings.Contains(endpoint, ".myqcloud.com.cn") {
		return false
	}
	return true
}

func (s *Service) backendProfile(name, vaultPath, command string) (string, domain.BackendProfile, error) {
	name = strings.TrimSpace(name)
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.BackendProfile{}, err
	}
	registry, err := loadBackendRegistry(root)
	if err != nil {
		return "", domain.BackendProfile{}, err
	}
	if name == "" {
		name = strings.TrimSpace(registry.DefaultBackend)
	}
	if name == "" {
		return "", domain.BackendProfile{}, &domain.CommandError{Code: "backend_name_required", Message: command + " requires a backend name because no default backend is configured", Hint: "pinax backend list --vault <vault>"}
	}
	profile, err := findBackendProfile(registry, name)
	if err != nil {
		return "", domain.BackendProfile{}, err
	}
	return root, profile, nil
}

func (s *Service) listBackendObjects(ctx context.Context, name, vaultPath, prefix, command string) (string, domain.BackendProfile, []pinaxcloud.ObjectInfo, error) {
	root, profile, err := s.backendProfile(name, vaultPath, command)
	if err != nil {
		return "", domain.BackendProfile{}, nil, err
	}
	store, err := backendBlobStore(ctx, root, profile)
	if err != nil {
		return "", domain.BackendProfile{}, nil, err
	}
	extended, ok := store.(pinaxcloud.ExtendedBlobStore)
	if !ok {
		return "", domain.BackendProfile{}, nil, &domain.CommandError{Code: "backend_list_unsupported", Message: "backend does not support object listing", Hint: "Run pinax backend capabilities <name> to view capabilities"}
	}
	objects, err := extended.List(ctx, strings.TrimSpace(prefix))
	if err != nil {
		projection, commandErr := backendProviderErrorProjection(command, root, profile, err)
		if projection.Error != nil {
			return "", domain.BackendProfile{}, nil, commandErr
		}
		return "", domain.BackendProfile{}, nil, err
	}
	return root, profile, objects, nil
}

func backendProviderErrorProjection(command, root string, profile domain.BackendProfile, err error) (domain.Projection, error) {
	message := "Backend provider request failed"
	code := "backend_provider_error"
	if looksLikeInvalidS3Region(err) {
		code = "backend_region_invalid"
		message = "S3 backend region is invalid"
	}
	hint := fmt.Sprintf("Run pinax backend show %s --vault %s and verify bucket, region, endpoint, and profile", shellQuote(profile.Name), shellQuote(root))
	commandErr := &domain.CommandError{Code: code, Message: message, Hint: hint}
	projection := domain.NewErrorProjection(command, commandErr)
	projection.Facts["backend"] = profile.Name
	projection.Facts["kind"] = string(profile.Kind)
	if profile.Region != "" {
		projection.Facts["region"] = profile.Region
	}
	projection.Evidence = []string{"provider_error_class=" + code}
	return projection, commandErr
}

func looksLikeInvalidS3Region(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid region") || strings.Contains(msg, "region was not a valid dns name")
}

func backendObjectRows(objects []pinaxcloud.ObjectInfo) []map[string]any {
	rows := make([]map[string]any, 0, len(objects))
	for _, obj := range objects {
		rows = append(rows, backendObjectRow(obj))
	}
	sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["key"]) < fmt.Sprint(rows[j]["key"]) })
	return rows
}

func backendObjectRow(obj pinaxcloud.ObjectInfo) map[string]any {
	row := map[string]any{"key": filepath.ToSlash(obj.Key), "size_bytes": obj.Size}
	if obj.Revision != "" {
		row["revision"] = strings.Trim(obj.Revision, "\"")
	}
	if !obj.LastModified.IsZero() {
		row["updated_at"] = obj.LastModified.UTC().Format(time.RFC3339)
	}
	return row
}

func backendNoteRows(objects []pinaxcloud.ObjectInfo) []map[string]any {
	rows := make([]map[string]any, 0)
	for _, obj := range objects {
		key := filepath.ToSlash(obj.Key)
		if !strings.HasSuffix(strings.ToLower(key), ".md") {
			continue
		}
		row := backendObjectRow(obj)
		row["path"] = key
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["path"]) < fmt.Sprint(rows[j]["path"]) })
	return rows
}

func backendNoteKey(prefix, path string) string {
	prefix = strings.Trim(strings.TrimSpace(filepath.ToSlash(prefix)), "/")
	path = strings.TrimLeft(strings.TrimSpace(filepath.ToSlash(path)), "/")
	if prefix == "" || strings.HasPrefix(path, prefix+"/") {
		return path
	}
	return prefix + "/" + path
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
