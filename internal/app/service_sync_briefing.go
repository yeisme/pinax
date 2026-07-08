package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/briefingops"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/briefing"
	"github.com/yeisme/pinax/internal/delivery"
	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

// Sync, briefing, and delivery operations: cloud sync diff/push/pull state writes,
// daily briefing run with recipe management, and Feishu delivery handoff.
// Extracted from service.go to isolate these cross-provider integration surfaces.

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

func (s *Service) SyncDiff(ctx context.Context, req SyncRequest) (domain.Projection, error) {
	root, target, err := cleanSyncRequest(req)
	if err != nil {
		return errorProjection("sync.diff", err), err
	}
	req.Target = target
	if isCapsaSyncTarget(target) {
		projection, cloudErr := buildCloudSyncProjection(ctx, "sync.diff", root, req, syncplan.DirectionDiff)
		if cloudErr != nil {
			if pinaxcloud.IsNotConfigured(cloudErr) || isCommandErrorCode(cloudErr, "cloud_not_configured") {
				return cloudSyncNotConfiguredProjection(root, target), nil
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
	req.Target = target
	lock, err := syncdaemon.AcquireOperationLock(root, "sync.push")
	if err != nil {
		return commandErrorProjection("sync.push", err)
	}
	defer lock.Release()
	if isCapsaSyncTarget(target) {
		if !req.Yes && !req.DryRun {
			err := &domain.CommandError{Code: "approval_required", Message: "sync push requires --yes or --dry-run", Hint: fmt.Sprintf("Review the plan first with pinax sync push --target %s --dry-run, then add --yes after confirming", syncOutputTarget(target))}
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
	req.Target = target
	lock, err := syncdaemon.AcquireOperationLock(root, "sync.pull")
	if err != nil {
		return commandErrorProjection("sync.pull", err)
	}
	defer lock.Release()
	if isCapsaSyncTarget(target) {
		if !req.Yes && !req.DryRun {
			err := &domain.CommandError{Code: "approval_required", Message: "sync pull requires --yes or --dry-run", Hint: fmt.Sprintf("Review the plan first with pinax sync pull --target %s --dry-run, then add --yes after confirming", syncOutputTarget(target))}
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

// Sync request normalization and sync-state projection helpers.
func cleanSyncRequest(req SyncRequest) (string, string, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", "", err
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = syncTargetCapsa
	}
	switch target {
	case "git", "s3", syncTargetCapsa, syncTargetCloud, syncTargetPinaxCloud:
		return root, target, nil
	default:
		return "", "", &domain.CommandError{Code: "invalid_sync_target", Message: "sync target only supports git, s3, capsa, cloud, or pinax-cloud", Hint: "pinax sync diff --target capsa"}
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
	if isCapsaSyncTarget(target) {
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
