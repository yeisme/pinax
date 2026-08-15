package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/vaultignore"
)

// Asset operations: add/list/show/link/verify/backlinks/orphans/missing/preview,
// repair/move/remove plan generation, and supporting asset evidence/facts helpers.
// Extracted from service.go to isolate the attachment-asset surface.

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
	appendEventWarned(root, "asset.add", "success", map[string]string{"asset_path": asset.Path})
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
