package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/versionops"
	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

// Version operations: changed/history/diff/show queries, restore plan/apply workflow,
// and version snapshot helpers over the configured version backend. Extracted from
// service.go to isolate the version-control surface.

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
	// AllowStale 显式放行 restore_plan_stale 守卫（逃生门）。
	AllowStale bool
}

// VersionRestoreApply 消费已保存的 restore plan，把历史 revision 的文件内容写回本地
// Markdown。它复用 version backend 的 ReadFile 读取历史内容，只做本地写入：
// remote_write=false、local_write=true，绝不调用 provider/cloud/MCP 写面。
// 必须显式 --yes；plan 的 vault hash 与 revision 必须与当前 vault 一致。
func (s *Service) VersionRestoreApply(ctx context.Context, req VersionRestoreApplyRequest) (domain.Projection, error) {
	tracker := newPipelineStageTracker(domain.PipelineKindRestore, strings.TrimSpace(req.PlanID), "", "apply")
	projection, err := s.versionRestoreApply(ctx, req, tracker)
	tracker.finish(&projection, err, pipelineStageCounts(projection, "restored"))
	return projection, err
}

func (s *Service) versionRestoreApply(ctx context.Context, req VersionRestoreApplyRequest, tracker *pipelineStageTracker) (domain.Projection, error) {
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
	tracker.begin()
	staleOverridden := false
	// 校验目标 vault 与 plan 来源一致：vault hash 漂移说明 vault 已被改动，plan 失效。
	currentHash, hashErr := versionVaultHash(root)
	if hashErr != nil {
		return errorProjection("version.restore.apply", hashErr), hashErr
	}
	if plan.VaultHash != "" && currentHash != plan.VaultHash {
		if req.AllowStale {
			// --allow-stale 逃生门：跳过 vault hash 守卫，投影附 warning。
			staleOverridden = true
		} else {
			err := &domain.CommandError{Code: "restore_plan_stale", Message: "vault changed since restore plan was generated", Hint: "Regenerate the restore plan with pinax version restore --plan before applying"}
			projection := domain.NewErrorProjection("version.restore.apply", err)
			projection.Data = map[string]any{"plan_id": plan.PlanID}
			return projection, err
		}
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
	appendEventWarned(root, "version.restore.apply", "success", map[string]string{"plan_id": plan.PlanID, "path": plan.Path, "revision": plan.Revision})
	projection := domain.NewProjection("version.restore.apply", "Version restore applied to local Markdown.")
	projection.Facts["local_write"] = "true"
	projection.Facts["remote_write"] = "false"
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["path"] = plan.Path
	projection.Facts["revision"] = plan.Revision
	projection.Facts["version_backend"] = restoredBackend
	projection.Facts["restored"] = "1"
	if staleOverridden {
		projection.Facts["allow_stale"] = "true"
		projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{Code: "plan_stale_overridden", Message: "stale plan applied via --allow-stale", Hint: "vault changed after this plan was generated"})
	}
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
