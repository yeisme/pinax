package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncdaemon"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/publishdocast"
	"gopkg.in/yaml.v3"
)

type publishDocProviderResult struct {
	ID         string
	URL        string
	ObjectType string // docx / doc / file（native inspect 返回）
}

func (s *Service) PublishDocProfileSet(_ context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.doc.profile.set", err), err
	}
	target, cmdErr := publishDocTarget(req.Target)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.doc.profile.set", cmdErr), cmdErr
	}
	profile := domain.NewPublishDocProfile(target)
	if existing, existingErr := readPublishDocProfile(root, target); existingErr == nil {
		profile.IndexObject = existing.IndexObject
	}
	renderer, rendererErr := publishDocRenderer(req.Renderer, target)
	if rendererErr != nil {
		return domain.NewErrorProjection("publish.doc.profile.set", rendererErr), rendererErr
	}
	profile.Renderer = renderer
	profile.As = strings.TrimSpace(req.As)
	profile.Layout = publishDocDefault(req.Layout, profile.Layout, "flat")
	profile.Template = publishDocDefault(req.Template, profile.Template, "plain")
	profile.IndexPage = profile.IndexPage || req.IndexPage
	profile.Workspace = strings.TrimSpace(req.Workspace)
	profile.ParentPage = strings.TrimSpace(req.ParentPage)
	profile.Space = strings.TrimSpace(req.Space)
	profile.Folder = normalizePublishDocFolder(strings.TrimSpace(req.Folder))
	if err := validatePublishDocProfile(profile); err != nil {
		return domain.NewErrorProjection("publish.doc.profile.set", err), err
	}
	if err := writePublishDocProfile(root, profile); err != nil {
		return errorProjection("publish.doc.profile.set", err), err
	}
	projection := domain.NewProjection("publish.doc.profile.set", "Document publish profile configured.")
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["provider"] = profile.Provider
	if profile.Renderer != "" {
		projection.Facts["renderer"] = string(profile.Renderer)
	}
	if profile.As != "" {
		projection.Facts["as"] = profile.As
	}
	if profile.Layout != "" {
		projection.Facts["layout"] = profile.Layout
	}
	if profile.Template != "" {
		projection.Facts["template"] = profile.Template
	}
	if profile.IndexPage {
		projection.Facts["index_page"] = "true"
	}
	projection.Evidence = []string{publishDocProfileRelPath(profile.Target)}
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax publish doc provider doctor --target %s --vault <vault> --json", shellQuote(string(profile.Target)))}}
	projection.Data = map[string]any{"profile": profile}
	return projection, nil
}

func (s *Service) PublishDocProviderList(_ context.Context, _ PublishRequest) (domain.Projection, error) {
	providers := []map[string]string{{"target": "notion-page", "provider": "notion"}, {"target": "lark-doc", "provider": "lark"}}
	projection := domain.NewProjection("publish.doc.provider.list", "Document publish providers listed.")
	projection.Facts["providers"] = fmt.Sprint(len(providers))
	projection.Data = map[string]any{"providers": providers}
	return projection, nil
}

func (s *Service) PublishDocProviderDoctor(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.provider.doctor")
	if err != nil {
		return errorProjection("publish.doc.provider.doctor", err), err
	}
	_ = root
	if err := publishDocProviderDoctor(ctx, profile); err != nil {
		return domain.NewErrorProjection("publish.doc.provider.doctor", err), err
	}
	projection := domain.NewProjection("publish.doc.provider.doctor", "Document publish provider is available.")
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["provider"] = profile.Provider
	if profile.As != "" {
		projection.Facts["as"] = profile.As
	}
	return projection, nil
}

func (s *Service) PublishDocPrepare(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	if req.All && strings.TrimSpace(req.Note) != "" {
		cmdErr := &domain.CommandError{Code: "argument_conflict", Message: "--all cannot be combined with --note", Hint: "Use pinax publish doc prepare --all --target <target> --vault <vault> --json or prepare one --note"}
		return domain.NewErrorProjection("publish.doc.prepare", cmdErr), cmdErr
	}
	if req.All {
		return s.PublishDocPrepareAll(ctx, req)
	}
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.prepare")
	if err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	note, err := s.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.Note})
	if err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	pkg, crossDocSummary, err := buildPublishDocPackage(root, profile, note)
	if err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	if err := writePublishDocPackage(root, pkg); err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	projection := domain.NewProjection("publish.doc.prepare", "Document publish package prepared.")
	projection.Facts["note_id"] = pkg.NoteID
	projection.Facts["target"] = string(pkg.Target)
	projection.Facts["package_id"] = pkg.ID
	projection.Facts["content_digest"] = pkg.ContentDigest
	if pkg.Renderer != "" {
		projection.Facts["renderer"] = string(pkg.Renderer)
	}
	if pkg.RenderRevision != "" {
		projection.Facts["render_revision"] = pkg.RenderRevision
	}
	projection.Facts["render_warnings"] = fmt.Sprint(len(pkg.RenderWarnings))
	publishDocAddCrossDocFacts(&projection, crossDocSummary)
	if pkg.RemotePath != "" {
		projection.Facts["remote_path"] = pkg.RemotePath
	}
	projection.Evidence = []string{publishDocPackageRelPath(pkg.ID)}
	projection.Actions = []domain.Action{{Name: "dry_run", Command: fmt.Sprintf("pinax publish doc push --package %s --target %s --vault <vault> --dry-run --json", shellQuote(pkg.ID), shellQuote(string(pkg.Target)))}}
	projection.Data = map[string]any{"package": pkg}
	return projection, nil
}

func (s *Service) PublishDocPrepareAll(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.prepare")
	if err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("publish.doc.prepare", err), err
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	type packageSummary struct {
		NoteID        string                            `json:"note_id"`
		NotePath      string                            `json:"note_path"`
		Title         string                            `json:"title"`
		PackageID     string                            `json:"package_id"`
		CrossDocLinks *domain.PublishDocCrossDocSummary `json:"cross_doc_links,omitempty"`
	}
	packages := make([]packageSummary, 0, len(notes))
	summary := domain.PublishDocCrossDocSummary{}
	for _, note := range notes {
		projection, err := s.PublishDocPrepare(ctx, PublishRequest{VaultPath: root, Note: note.ID, Target: string(profile.Target)})
		if err != nil {
			return projection, err
		}
		pkg, ok := publishDocProjectionPackage(projection)
		if !ok {
			continue
		}
		packages = append(packages, packageSummary{NoteID: pkg.NoteID, NotePath: pkg.NotePath, Title: pkg.Title, PackageID: pkg.ID, CrossDocLinks: pkg.CrossDocLinks})
		if pkg.CrossDocLinks != nil {
			publishDocMergeCrossDocSummary(&summary, *pkg.CrossDocLinks)
		}
	}
	projection := domain.NewProjection("publish.doc.prepare", "Document publish packages prepared for vault.")
	projection.Facts["all"] = "true"
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["packages"] = fmt.Sprint(len(packages))
	projection.Facts["target"] = string(profile.Target)
	publishDocAddCrossDocFacts(&projection, summary)
	projection.Actions = []domain.Action{{Name: "dry_run", Command: fmt.Sprintf("pinax publish doc push --all --target %s --vault <vault> --dry-run --json", shellQuote(string(profile.Target)))}}
	if summary.Unpublished > 0 {
		projection.Actions = append(projection.Actions, domain.Action{Name: "publish_all", Command: fmt.Sprintf("pinax publish doc push --all --target %s --vault <vault> --yes --json", shellQuote(string(profile.Target)))})
	}
	projection.Data = map[string]any{"packages": packages, "cross_doc_links": summary}
	return projection, nil
}

func (s *Service) PublishDocPushAll(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.push")
	if err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	if !req.DryRun && !req.Yes {
		cmdErr := &domain.CommandError{Code: "approval_required", Message: "Vault-wide document push requires approval", Hint: "Add --yes after reviewing --dry-run output"}
		projection := domain.NewErrorProjection("publish.doc.push", cmdErr)
		projection.Actions = []domain.Action{{Name: "approve", Command: fmt.Sprintf("pinax publish doc push --all --target %s --vault <vault> --yes --json", shellQuote(string(profile.Target)))}}
		return projection, cmdErr
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	passes := 1
	if !req.DryRun && profile.Target == domain.PublishDocTargetLarkDoc && profile.ResolveDocRenderer() == domain.PublishDocRendererNativeDocx {
		passes = 2
	}
	type pushSummary struct {
		Pass        int    `json:"pass"`
		NoteID      string `json:"note_id"`
		NotePath    string `json:"note_path"`
		PackageID   string `json:"package_id"`
		Status      string `json:"status"`
		ExternalURL string `json:"external_url,omitempty"`
	}
	results := make([]pushSummary, 0, len(notes))
	finalCrossDoc := domain.PublishDocCrossDocSummary{}
	// Second pass exists only to re-render cross-doc links once their targets
	// exist remotely. Notes whose pass-1 package had no unresolved cross-doc
	// links gain nothing from a second push, so skip them instead of paying a
	// second provider round trip (and asset re-insertion) for the whole vault.
	needsSecondPass := map[string]bool{}
	fail := func(p domain.Projection, cause error) (domain.Projection, error) {
		if p.Data == nil {
			p.Data = map[string]any{}
		}
		if data, ok := p.Data.(map[string]any); ok {
			data["partial"] = true
			data["results"] = results
		}
		return p, cause
	}
	for pass := 1; pass <= passes; pass++ {
		if pass == passes {
			finalCrossDoc = domain.PublishDocCrossDocSummary{}
		}
		for _, note := range notes {
			if req.DryRun {
				pkg, crossDocSummary, err := buildPublishDocPackage(root, profile, note)
				if err != nil {
					return fail(errorProjection("publish.doc.push", err), err)
				}
				if pass == passes && crossDocSummary.Total > 0 {
					publishDocMergeCrossDocSummary(&finalCrossDoc, crossDocSummary)
				}
				if cmdErr := publishDocProviderPreflight(ctx, profile, pkg, note); cmdErr != nil {
					return fail(domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr)
				}
				if pass == passes {
					results = append(results, pushSummary{Pass: pass, NoteID: pkg.NoteID, NotePath: pkg.NotePath, PackageID: pkg.ID, Status: string(domain.PublishDocStatusDryRunVerified)})
				}
				continue
			}
			prepareProjection, err := s.PublishDocPrepare(ctx, PublishRequest{VaultPath: root, Note: note.ID, Target: string(profile.Target)})
			if err != nil {
				return fail(prepareProjection, err)
			}
			pkg, ok := publishDocProjectionPackage(prepareProjection)
			if !ok {
				cmdErr := &domain.CommandError{Code: "publish_package_missing", Message: "Prepared package was not returned"}
				return fail(domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr)
			}
			if pkg.CrossDocLinks != nil {
				if pass < passes && pkg.CrossDocLinks.Unpublished > 0 {
					needsSecondPass[pkg.NoteID] = true
				}
				if pass == passes {
					publishDocMergeCrossDocSummary(&finalCrossDoc, *pkg.CrossDocLinks)
				}
			}
			if pass > 1 && !needsSecondPass[pkg.NoteID] {
				continue
			}
			pushProjection, err := s.PublishDocPush(ctx, PublishRequest{VaultPath: root, PackageID: pkg.ID, Target: string(profile.Target), DryRun: req.DryRun})
			if err != nil {
				if pushProjection.Error != nil && pushProjection.Error.Code == "publish_object_type_migration_required" {
					pushProjection.Actions = append(pushProjection.Actions, domain.Action{Name: "unlink_all", Command: fmt.Sprintf("pinax publish doc unlink --all --target %s --vault <vault> --json", shellQuote(string(profile.Target)))})
				}
				return fail(pushProjection, err)
			}
			if pass == passes {
				results = append(results, pushSummary{Pass: pass, NoteID: pkg.NoteID, NotePath: pkg.NotePath, PackageID: pkg.ID, Status: pushProjection.Facts["publish_status"], ExternalURL: pushProjection.Facts["external_url"]})
			}
		}
	}
	projection := domain.NewProjection("publish.doc.push", "Document publish vault push completed.")
	projection.Facts["all"] = "true"
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["passes"] = fmt.Sprint(passes)
	if req.DryRun {
		projection.Facts["dry_run"] = "true"
	}
	publishDocAddCrossDocFacts(&projection, finalCrossDoc)
	if !req.DryRun {
		if pruned := prunePublishDocPackages(root); pruned > 0 {
			projection.Facts["packages_pruned"] = fmt.Sprint(pruned)
		}
	}
	projection.Data = map[string]any{"results": results, "cross_doc_links": finalCrossDoc}
	return projection, nil
}

// prunePublishDocPackages bounds the growth of the prepared-package registry:
// every prepare/push writes a new timestamped pubdoc_*.json and nothing ever
// removed them, so repeated vault pushes accumulated unbounded JSON files.
// Keep the newest few packages per note (history for receipts/inspection) and
// delete the rest.
func prunePublishDocPackages(root string) int {
	const keep = 3
	dir := filepath.Join(publishDocRoot(root), "packages")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	type pkgFile struct {
		path    string
		modTime time.Time
	}
	byNote := map[string][]pkgFile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var pkg domain.PublishDocPackage
		if err := json.Unmarshal(body, &pkg); err != nil || strings.TrimSpace(pkg.NoteID) == "" {
			continue
		}
		byNote[pkg.NoteID] = append(byNote[pkg.NoteID], pkgFile{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()})
	}
	pruned := 0
	for _, files := range byNote {
		if len(files) <= keep {
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
		for _, stale := range files[keep:] {
			if err := os.Remove(stale.path); err != nil {
				warnPersistFailure("publish doc package prune", err)
				continue
			}
			pruned++
		}
	}
	return pruned
}

func (s *Service) PublishDocPush(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	if req.All && strings.TrimSpace(req.PackageID) != "" {
		cmdErr := &domain.CommandError{Code: "argument_conflict", Message: "--all cannot be combined with --package", Hint: "Use pinax publish doc push --all --target <target> --vault <vault> --dry-run --json or push one --package"}
		return domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr
	}
	if req.All {
		return s.PublishDocPushAll(ctx, req)
	}
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.push")
	if err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	pkg, err := readPublishDocPackage(root, req.PackageID)
	if err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	// 迁移守卫优先于通用包校验：active mapping 指向 Drive file 但 profile 已切到 native-docx 时，
	// 给出最具体、可操作的迁移错误（unlink 后重发），而不是先被 renderer_mismatch 拦截。
	// dry-run 同样需要探测迁移阻断，所以守卫放在 dry-run 分支之前。
	// Only a genuinely missing mapping counts as "never published"; a corrupt
	// or unreadable mapping MUST fail the push instead of silently creating a
	// second remote document and orphaning the first.
	mapping, mappingReadErr := readPublishDocMapping(root, pkg.NoteID, profile.Target)
	if mappingReadErr != nil {
		var cmdErr *domain.CommandError
		if !errors.As(mappingReadErr, &cmdErr) || cmdErr.Code != "publish_mapping_not_found" {
			return errorProjection("publish.doc.push", mappingReadErr), mappingReadErr
		}
	}
	if mapping.PublishStatus == domain.PublishDocStatusDetached {
		mapping = domain.PublishDocMapping{}
	}
	if cmdErr := publishDocMigrationGuard(profile, mapping, pkg.NoteID); cmdErr != nil {
		proj := domain.NewErrorProjection("publish.doc.push", cmdErr)
		proj.Actions = []domain.Action{
			{Name: "unlink", Command: "pinax publish doc unlink --note " + shellQuote(pkg.NoteID) + " --target lark-doc --vault <vault> --json"},
			{Name: "unlink_all", Command: "pinax publish doc unlink --all --target lark-doc --vault <vault> --json"},
		}
		return proj, cmdErr
	}
	if cmdErr := validatePublishDocPackageForProfile(pkg, profile); cmdErr != nil {
		return domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr
	}
	note, err := s.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: pkg.NoteID})
	if err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	if got := publishDocDigest(note); got != pkg.ContentDigest {
		cmdErr := &domain.CommandError{Code: "publish_stale", Message: "Publish package is stale", Hint: "Run pinax publish doc prepare again"}
		return domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr
	}
	if req.DryRun {
		if err := publishDocProviderPreflight(ctx, profile, pkg, note); err != nil {
			return domain.NewErrorProjection("publish.doc.push", err), err
		}
		projection := publishDocProjection("publish.doc.push", pkg.NoteID, pkg.ID, profile.Target, domain.PublishDocStatusDryRunVerified, "")
		projection.Facts["dry_run"] = "true"
		if pkg.Renderer != "" {
			projection.Facts["renderer"] = string(pkg.Renderer)
		}
		return projection, nil
	}
	remoteFolderToken := profile.Folder
	if profile.Target == domain.PublishDocTargetLarkDoc && publishDocLayout(profile) == "mirror" {
		remotePath := publishDocPackageRemoteDir(pkg)
		if remotePath != "" {
			folderToken, err := ensurePublishDocLarkFolder(ctx, root, profile, remotePath)
			if err != nil {
				return domain.NewErrorProjection("publish.doc.push", err), err
			}
			remoteFolderToken = folderToken
			if mapping.ExternalObject.ID != "" && mapping.RemoteFolderToken != folderToken {
				if err := publishDocProviderMove(ctx, profile, mapping.ExternalObject.ID, folderToken, publishDocMoveObjectType(mapping)); err != nil {
					return domain.NewErrorProjection("publish.doc.push", err), err
				}
			}
		}
	}
	var result publishDocProviderResult
	var providerErr *domain.CommandError
	var assetWarnings []domain.PublishDocRenderWarning
	var assetBlocks []string
	nativeMode := profile.Target == domain.PublishDocTargetLarkDoc && profile.ResolveDocRenderer() == domain.PublishDocRendererNativeDocx
	if mapping.ExternalObject.ID == "" {
		if nativeMode {
			result, assetWarnings, assetBlocks, providerErr = publishDocNativeProviderCreate(ctx, root, profile, pkg, note, remoteFolderToken)
		} else {
			result, providerErr = publishDocProviderCreate(ctx, profile, pkg, note, remoteFolderToken)
		}
	} else {
		if nativeMode {
			result, assetWarnings, assetBlocks, providerErr = publishDocNativeProviderUpdate(ctx, root, profile, mapping, pkg, note)
		} else {
			result, providerErr = publishDocProviderUpdate(ctx, profile, mapping, pkg, note)
		}
	}
	if providerErr != nil {
		_ = writePublishDocReceipt(root, "publish.doc.push", pkg.NoteID, pkg.ID, profile.Target, "failed", "")
		return domain.NewErrorProjection("publish.doc.push", providerErr), providerErr
	}
	if result.ID == "" {
		result.ID = mapping.ExternalObject.ID
	}
	// Guard against a provider CLI that succeeded by exit code but returned no
	// object ID: persisting a mapping without an ID makes the NEXT push create
	// a duplicate remote document (mirrors the folder-token guard below).
	if result.ID == "" {
		cmdErr := &domain.CommandError{Code: "publish_provider_result_invalid", Message: "Provider CLI did not return a document token", Hint: "Rerun the push; if it repeats, inspect the provider CLI output with pinax publish doc doctor"}
		_ = writePublishDocReceipt(root, "publish.doc.push", pkg.NoteID, pkg.ID, profile.Target, "failed", "")
		return domain.NewErrorProjection("publish.doc.push", cmdErr), cmdErr
	}
	if result.URL == "" {
		result.URL = mapping.ExternalObject.URL
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// 合并 prepare 阶段 plan warnings + push 阶段 asset insertion warnings。
	renderWarnings := pkg.RenderWarnings
	renderWarnings = append(renderWarnings, assetWarnings...)
	mapping = domain.PublishDocMapping{SchemaVersion: domain.PublishDocMappingSchemaVersion, NoteID: pkg.NoteID, Target: profile.Target, Provider: profile.Provider, ExternalObject: domain.PublishDocExternalObject{Provider: profile.Provider, Target: string(profile.Target), Type: publishDocExternalObjectType(profile, result), ID: result.ID, URL: result.URL}, RemotePath: pkg.RemotePath, RemoteFolderToken: remoteFolderToken, ContentDigest: pkg.ContentDigest, PublishStatus: domain.PublishDocStatusPublished, LastPublishedAt: now, UpdatedAt: now, Renderer: pkg.Renderer, RenderRevision: pkg.RenderRevision, RenderWarnings: renderWarnings, RenderAssetBlocks: assetBlocks}
	if err := writePublishDocMapping(root, mapping); err != nil {
		return errorProjection("publish.doc.push", err), err
	}
	_ = writePublishDocReceipt(root, "publish.doc.push", pkg.NoteID, pkg.ID, profile.Target, "success", result.URL)
	var indexErr *domain.CommandError
	if profile.IndexPage {
		indexErr = publishDocRebuildIndex(ctx, root, profile)
	}
	projection := publishDocProjection("publish.doc.push", pkg.NoteID, pkg.ID, profile.Target, domain.PublishDocStatusPublished, result.URL)
	projection.Facts["external_object_type"] = mapping.ExternalObject.Type
	if mapping.Renderer != "" {
		projection.Facts["renderer"] = string(mapping.Renderer)
	}
	projection.Facts["render_warnings"] = fmt.Sprint(len(mapping.RenderWarnings))
	if indexErr != nil {
		// The document push itself succeeded; the index rebuild failure must
		// still be visible instead of silently leaving a stale index page.
		projection.Facts["index_rebuild"] = "failed"
		projection.Status = "partial"
		fmt.Fprintf(os.Stderr, "pinax: publish doc index rebuild failed: %s\n", indexErr.Message)
	}
	projection.Data = map[string]any{"mapping": mapping}
	return projection, nil
}

func (s *Service) PublishDocStatus(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.doc.status", err), err
	}
	note, err := s.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.Note})
	if err != nil {
		return errorProjection("publish.doc.status", err), err
	}
	mappings, err := listPublishDocMappings(root, strings.TrimSpace(req.Target))
	if err != nil {
		return errorProjection("publish.doc.status", err), err
	}
	for _, mapping := range mappings {
		if mapping.NoteID != note.ID {
			continue
		}
		status := mapping.PublishStatus
		if status == domain.PublishDocStatusPublished && mapping.ContentDigest != publishDocDigest(note) {
			status = domain.PublishDocStatusStale
		}
		projection := publishDocProjection("publish.doc.status", note.ID, "", mapping.Target, status, mapping.ExternalObject.URL)
		mapping = publishDocEnrichMappingForOutput(mapping)
		publishDocEnrichProjection(&projection, mapping)
		projection.Data = map[string]any{"mapping": mapping}
		return projection, nil
	}
	cmdErr := &domain.CommandError{Code: "publish_mapping_not_found", Message: "Document publish mapping was not found", Hint: "Run pinax publish doc prepare and push first"}
	return domain.NewErrorProjection("publish.doc.status", cmdErr), cmdErr
}

func (s *Service) PublishDocList(_ context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.doc.list", err), err
	}
	mappings, err := listPublishDocMappings(root, strings.TrimSpace(req.Target))
	if err != nil {
		return errorProjection("publish.doc.list", err), err
	}
	for i := range mappings {
		mappings[i] = publishDocEnrichMappingForOutput(mappings[i])
	}
	projection := domain.NewProjection("publish.doc.list", "Document publish mappings listed.")
	projection.Facts["mappings"] = fmt.Sprint(len(mappings))
	if strings.TrimSpace(req.Target) != "" {
		projection.Facts["target"] = strings.TrimSpace(req.Target)
	}
	projection.Data = map[string]any{"mappings": mappings}
	return projection, nil
}

func (s *Service) PublishDocLink(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.link")
	if err != nil {
		return errorProjection("publish.doc.link", err), err
	}
	note, err := s.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.Note})
	if err != nil {
		return errorProjection("publish.doc.link", err), err
	}
	if strings.TrimSpace(req.ExternalURL) == "" {
		cmdErr := &domain.CommandError{Code: "external_url_required", Message: "External URL is required", Hint: "Use --external-url <url>"}
		return domain.NewErrorProjection("publish.doc.link", cmdErr), cmdErr
	}
	now := time.Now().UTC().Format(time.RFC3339)
	mapping := domain.PublishDocMapping{SchemaVersion: domain.PublishDocMappingSchemaVersion, NoteID: note.ID, Target: profile.Target, Provider: profile.Provider, ExternalObject: domain.PublishDocExternalObject{Provider: profile.Provider, Target: string(profile.Target), Type: publishDocExternalType(profile), ID: publishDocExternalIDFromURL(req.ExternalURL), URL: strings.TrimSpace(req.ExternalURL)}, ContentDigest: publishDocDigest(note), PublishStatus: domain.PublishDocStatusLinked, LastPublishedAt: now, UpdatedAt: now}
	if err := writePublishDocMapping(root, mapping); err != nil {
		return errorProjection("publish.doc.link", err), err
	}
	_ = writePublishDocReceipt(root, "publish.doc.link", note.ID, "", profile.Target, "success", mapping.ExternalObject.URL)
	projection := publishDocProjection("publish.doc.link", note.ID, "", profile.Target, domain.PublishDocStatusLinked, mapping.ExternalObject.URL)
	projection.Data = map[string]any{"mapping": mapping}
	return projection, nil
}

func (s *Service) PublishDocUnlink(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	if req.All && strings.TrimSpace(req.Note) != "" {
		cmdErr := &domain.CommandError{Code: "argument_conflict", Message: "--all cannot be combined with --note", Hint: "Use pinax publish doc unlink --all --target <target> --vault <vault> --json or unlink one --note"}
		return domain.NewErrorProjection("publish.doc.unlink", cmdErr), cmdErr
	}
	if req.All {
		return s.PublishDocUnlinkAll(ctx, req)
	}
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.unlink")
	if err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	note, err := s.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.Note})
	if err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	mapping, err := readPublishDocMapping(root, note.ID, profile.Target)
	if err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	mapping.PublishStatus = domain.PublishDocStatusDetached
	mapping.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writePublishDocMapping(root, mapping); err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	_ = writePublishDocReceipt(root, "publish.doc.unlink", note.ID, "", profile.Target, "success", mapping.ExternalObject.URL)
	projection := publishDocProjection("publish.doc.unlink", note.ID, "", profile.Target, domain.PublishDocStatusDetached, mapping.ExternalObject.URL)
	projection.Data = map[string]any{"mapping": mapping}
	return projection, nil
}

func (s *Service) PublishDocUnlinkAll(_ context.Context, req PublishRequest) (domain.Projection, error) {
	root, profile, err := readPublishDocProfileRequest(req, "publish.doc.unlink")
	if err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	mappings, err := listPublishDocMappings(root, string(profile.Target))
	if err != nil {
		return errorProjection("publish.doc.unlink", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	detached := make([]domain.PublishDocMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Target != profile.Target || mapping.PublishStatus == domain.PublishDocStatusDetached {
			continue
		}
		mapping.PublishStatus = domain.PublishDocStatusDetached
		mapping.UpdatedAt = now
		if err := writePublishDocMapping(root, mapping); err != nil {
			return errorProjection("publish.doc.unlink", err), err
		}
		detached = append(detached, mapping)
		_ = writePublishDocReceipt(root, "publish.doc.unlink", mapping.NoteID, "", profile.Target, "success", mapping.ExternalObject.URL)
	}
	projection := domain.NewProjection("publish.doc.unlink", "Document publish mappings detached for vault.")
	projection.Facts["all"] = "true"
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["detached"] = fmt.Sprint(len(detached))
	projection.Actions = []domain.Action{{Name: "prepare_all", Command: fmt.Sprintf("pinax publish doc prepare --all --target %s --vault <vault> --json", shellQuote(string(profile.Target)))}}
	projection.Data = map[string]any{"mappings": detached}
	return projection, nil
}

func publishDocProjection(command, noteID, packageID string, target domain.PublishDocTarget, status domain.PublishDocStatus, externalURL string) domain.Projection {
	projection := domain.NewProjection(command, "Document publish operation completed.")
	projection.Facts["note_id"] = noteID
	projection.Facts["target"] = string(target)
	projection.Facts["publish_status"] = string(status)
	if packageID != "" {
		projection.Facts["package_id"] = packageID
	}
	if externalURL != "" {
		projection.Facts["external_url"] = externalURL
	}
	return projection
}

func publishDocProjectionPackage(projection domain.Projection) (domain.PublishDocPackage, bool) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return domain.PublishDocPackage{}, false
	}
	pkg, ok := data["package"].(domain.PublishDocPackage)
	return pkg, ok
}

// publishDocEnrichProjection 为 status/list 等只读输出补充 renderer / external_object_type / render_warnings。
// 旧 mapping 缺字段时按推断规则填充，保证输出事实稳定可消费。
func publishDocEnrichProjection(projection *domain.Projection, mapping domain.PublishDocMapping) {
	renderer := string(mapping.ResolveDocMappingRenderer())
	if renderer == "" {
		renderer = "unknown"
	}
	projection.Facts["renderer"] = renderer
	if mapping.ExternalObject.Type != "" {
		projection.Facts["external_object_type"] = mapping.ExternalObject.Type
	}
	projection.Facts["render_warnings"] = fmt.Sprint(len(mapping.RenderWarnings))
}

func publishDocEnrichMappingForOutput(mapping domain.PublishDocMapping) domain.PublishDocMapping {
	if mapping.Renderer == "" {
		mapping.Renderer = mapping.ResolveDocMappingRenderer()
	}
	if strings.TrimSpace(mapping.ExternalObject.Type) == "" {
		switch mapping.Renderer {
		case domain.PublishDocRendererNativeDocx:
			mapping.ExternalObject.Type = domain.PublishDocObjectTypeDocx
		case domain.PublishDocRendererMarkdownFile:
			mapping.ExternalObject.Type = domain.PublishDocObjectTypeFile
		case "":
			// Leave unknown non-lark mappings unchanged.
		}
	}
	return mapping
}

func publishDocDefault(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func publishDocLayout(profile domain.PublishDocProfile) string {
	layout := strings.TrimSpace(profile.Layout)
	if layout == "" {
		if profile.Target == domain.PublishDocTargetLarkDoc {
			return "mirror"
		}
		return "flat"
	}
	return layout
}

func publishDocTemplate(profile domain.PublishDocProfile) string {
	template := strings.TrimSpace(profile.Template)
	if template == "" {
		if profile.Target == domain.PublishDocTargetLarkDoc {
			return "vault"
		}
		return "plain"
	}
	return template
}

func publishDocRemotePath(note domain.Note, profile domain.PublishDocProfile) string {
	if publishDocLayout(profile) != "mirror" {
		return publishDocFileName(note)
	}
	dir := filepath.ToSlash(filepath.Dir(strings.TrimSpace(note.Path)))
	if dir == "." || dir == "" {
		return publishDocFileName(note)
	}
	return filepath.ToSlash(filepath.Join(dir, publishDocFileName(note)))
}

func publishDocPackageRemoteDir(pkg domain.PublishDocPackage) string {
	dir := filepath.ToSlash(filepath.Dir(strings.TrimSpace(pkg.RemotePath)))
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

func publishDocRenderedMarkdown(note domain.Note, profile domain.PublishDocProfile) string {
	if publishDocTemplate(profile) == "plain" {
		return publishDocMarkdown(note)
	}
	var b strings.Builder
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = note.ID
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("> Published from Pinax. Local vault remains the source of truth.\n\n")
	b.WriteString("## Overview\n\n")
	b.WriteString("| Field | Value |\n| --- | --- |\n")
	writeRow := func(name, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		value = strings.ReplaceAll(value, "|", "\\|")
		b.WriteString("| ")
		b.WriteString(name)
		b.WriteString(" | ")
		b.WriteString(value)
		b.WriteString(" |\n")
	}
	writeRow("Pinax note", note.ID)
	writeRow("Vault path", note.Path)
	writeRow("Kind", note.Kind)
	writeRow("Status", note.Status)
	if len(note.Tags) > 0 {
		writeRow("Tags", strings.Join(note.Tags, ", "))
	} else if tags := strings.TrimSpace(note.Frontmatter["tags"]); tags != "" {
		writeRow("Tags", tags)
	}
	writeRow("Updated", note.UpdatedAt)
	b.WriteString("\n## Reading Notes\n\n")
	body := stripPublishDocDuplicateTitle(strings.TrimSpace(note.Body), title)
	if body == "" {
		b.WriteString("_No body content._\n")
	} else {
		b.WriteString(body)
		b.WriteString("\n")
	}
	b.WriteString("\n---\n\n")
	b.WriteString("_Managed by Pinax publish doc. Update this copy from the local vault instead of editing the published body directly._\n")
	return b.String()
}

func stripPublishDocDuplicateTitle(body, title string) string {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "# ") && strings.TrimSpace(strings.TrimPrefix(first, "# ")) == strings.TrimSpace(title) {
		return strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return body
}

func readPublishDocFolderMapping(root string, target domain.PublishDocTarget, remotePath string) (domain.PublishDocFolderMapping, error) {
	path := filepath.Join(publishDocRoot(root), "folders", string(target), sanitizePublishDocID(remotePath)+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		return domain.PublishDocFolderMapping{}, err
	}
	var mapping domain.PublishDocFolderMapping
	if err := json.Unmarshal(body, &mapping); err != nil {
		return domain.PublishDocFolderMapping{}, err
	}
	return mapping, nil
}

func writePublishDocFolderMapping(root string, mapping domain.PublishDocFolderMapping) error {
	body, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(publishDocRoot(root), "folders", string(mapping.Target), sanitizePublishDocID(mapping.RemotePath)+".json")
	return writePublishFile(path, append(body, '\n'))
}

func ensurePublishDocLarkFolder(ctx context.Context, root string, profile domain.PublishDocProfile, remotePath string) (string, *domain.CommandError) {
	remotePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(remotePath)), "/")
	if remotePath == "" {
		return profile.Folder, nil
	}
	if mapping, err := readPublishDocFolderMapping(root, profile.Target, remotePath); err == nil && mapping.FolderToken != "" {
		return mapping.FolderToken, nil
	}
	// The read-check-create-write cycle below must be serialized across
	// processes: two concurrent pushes that both miss the registry both create
	// the remote folder and the loser's token is orphaned in Drive.
	lock, lockErr := acquirePublishDocRegistryLockWithRetry(root)
	if lockErr != nil {
		return "", &domain.CommandError{Code: "publish_registry_locked", Message: "Another publish operation is updating the folder registry", Hint: "Wait for the concurrent publish to finish and retry"}
	}
	defer lock.Release()
	if mapping, err := readPublishDocFolderMapping(root, profile.Target, remotePath); err == nil && mapping.FolderToken != "" {
		return mapping.FolderToken, nil
	}
	parent := profile.Folder
	parts := strings.Split(remotePath, "/")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		partial := strings.Join(parts[:i+1], "/")
		if mapping, err := readPublishDocFolderMapping(root, profile.Target, partial); err == nil && mapping.FolderToken != "" {
			parent = mapping.FolderToken
			continue
		}
		if existing := findPublishDocLarkFolder(ctx, profile, parent, part); existing != "" {
			parent = existing
		} else {
			out, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "drive", "+create-folder", "--folder-token", parent, "--name", part, "--json"), "")
			if err != nil {
				return "", err
			}
			result := parsePublishDocProviderResult(out, profile.Target)
			if result.ID == "" {
				return "", &domain.CommandError{Code: "provider_preflight_failed", Message: "Provider CLI did not return folder token", Hint: "Check lark-cli drive +create-folder JSON output"}
			}
			parent = result.ID
		}
		mapping := domain.PublishDocFolderMapping{SchemaVersion: domain.PublishDocMappingSchemaVersion, Target: profile.Target, Provider: profile.Provider, RemotePath: partial, FolderToken: parent, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
		if err := writePublishDocFolderMapping(root, mapping); err != nil {
			// Losing the token silently would orphan the remote folder on the
			// next push; surface the persistence failure instead.
			warnPersistFailure("publish doc folder mapping", err)
		}
	}
	return parent, nil
}

// acquirePublishDocRegistryLockWithRetry waits briefly for a concurrent
// registry update instead of failing the whole push outright.
func acquirePublishDocRegistryLockWithRetry(root string) (syncdaemon.Lock, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		lock, err := syncdaemon.AcquirePublishDocRegistryLock(root)
		if err == nil {
			return lock, nil
		}
		var cmdErr *domain.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != "lock_held" || time.Now().After(deadline) {
			return syncdaemon.Lock{}, err
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func findPublishDocLarkFolder(ctx context.Context, profile domain.PublishDocProfile, parentToken, name string) string {
	out, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "drive", "+search", "--query", name, "--doc-types", "folder", "--folder-tokens", parentToken, "--json"), "")
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(extractPublishDocJSON(out), &payload); err != nil {
		return ""
	}
	data, _ := payload["data"].(map[string]any)
	results, _ := data["results"].([]any)
	for _, item := range results {
		entry, _ := item.(map[string]any)
		title := strings.TrimSpace(stripPublishDocHTML(fmt.Sprint(entry["title_highlighted"])))
		meta, _ := entry["result_meta"].(map[string]any)
		if !strings.EqualFold(fmt.Sprint(meta["doc_types"]), "FOLDER") || title != name {
			continue
		}
		if token, ok := meta["token"].(string); ok && token != "" {
			return token
		}
	}
	return ""
}

func stripPublishDocHTML(value string) string {
	value = strings.ReplaceAll(value, "<h>", "")
	value = strings.ReplaceAll(value, "</h>", "")
	return value
}

func publishDocProviderMove(ctx context.Context, profile domain.PublishDocProfile, fileToken, folderToken, objectType string) *domain.CommandError {
	if strings.TrimSpace(fileToken) == "" || strings.TrimSpace(folderToken) == "" {
		return nil
	}
	objectType = strings.TrimSpace(objectType)
	if objectType == "" {
		objectType = domain.PublishDocObjectTypeFile
	}
	_, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "drive", "+move", "--file-token", fileToken, "--folder-token", folderToken, "--type", objectType, "--json"), "")
	return err
}

func publishDocMoveObjectType(mapping domain.PublishDocMapping) string {
	if strings.TrimSpace(mapping.ExternalObject.Type) != "" {
		return strings.TrimSpace(mapping.ExternalObject.Type)
	}
	switch mapping.ResolveDocMappingRenderer() {
	case domain.PublishDocRendererNativeDocx:
		return domain.PublishDocObjectTypeDocx
	case domain.PublishDocRendererMarkdownFile:
		return domain.PublishDocObjectTypeFile
	default:
		return domain.PublishDocObjectTypeFile
	}
}

func publishDocRebuildIndex(ctx context.Context, root string, profile domain.PublishDocProfile) *domain.CommandError {
	if profile.Target != domain.PublishDocTargetLarkDoc {
		return nil
	}
	mappings, err := listPublishDocMappings(root, string(profile.Target))
	if err != nil {
		return &domain.CommandError{Code: "publish_index_failed", Message: "Failed to list document publish mappings", Hint: "Run pinax publish doc list first"}
	}
	body := publishDocIndexMarkdown(mappings)
	title := "Pinax Cloud Vault Index"
	note := domain.Note{ID: "pinax_cloud_vault_index", Title: title, Body: body}
	pkg := domain.PublishDocPackage{BodyMarkdown: body}
	nativeMode := profile.ResolveDocRenderer() == domain.PublishDocRendererNativeDocx
	if nativeMode {
		// 索引页也走原生文档 renderer：为索引 note 构建 native plan。
		pkg.Renderer = domain.PublishDocRendererNativeDocx
		doc := publishdocast.Parse(title, body)
		plan := publishdocast.BuildNativePlan(doc, publishdocast.AssetOptions{})
		if planBody, mErr := publishdocast.MarshalNativePlan(plan); mErr == nil {
			pkg.NativePlan = planBody
			pkg.RenderRevision = domain.PublishDocRenderRevision
		}
	}
	if profile.IndexObject != nil && profile.IndexObject.ID != "" {
		mapping := domain.PublishDocMapping{ExternalObject: *profile.IndexObject}
		var result publishDocProviderResult
		var providerErr *domain.CommandError
		if nativeMode {
			result, _, _, providerErr = publishDocNativeProviderUpdate(ctx, root, profile, mapping, pkg, note)
		} else {
			result, providerErr = publishDocProviderUpdate(ctx, profile, mapping, pkg, note)
		}
		if providerErr != nil {
			return providerErr
		}
		if result.ID != "" {
			profile.IndexObject.ID = result.ID
		}
		if result.URL != "" {
			profile.IndexObject.URL = result.URL
		}
		if result.ObjectType != "" {
			profile.IndexObject.Type = result.ObjectType
		}
	} else {
		var result publishDocProviderResult
		var providerErr *domain.CommandError
		if nativeMode {
			result, _, _, providerErr = publishDocNativeProviderCreate(ctx, root, profile, pkg, note, profile.Folder)
		} else {
			result, providerErr = publishDocProviderCreate(ctx, profile, pkg, note, profile.Folder)
		}
		if providerErr != nil {
			return providerErr
		}
		indexType := publishDocExternalObjectType(profile, result)
		profile.IndexObject = &domain.PublishDocExternalObject{Provider: profile.Provider, Target: string(profile.Target), Type: indexType, ID: result.ID, URL: result.URL}
	}
	if err := writePublishDocProfile(root, profile); err != nil {
		// Losing the locally stored index token makes the NEXT push create a
		// duplicate index page remotely; the caller must surface this.
		return &domain.CommandError{Code: "publish_index_failed", Message: "Index page was published but its token could not be stored locally", Hint: "Retry the push; if it repeats, inspect .pinax/publish/doc write permissions"}
	}
	return nil
}

func publishDocIndexMarkdown(mappings []domain.PublishDocMapping) string {
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].RemotePath < mappings[j].RemotePath })
	var b strings.Builder
	b.WriteString("# Pinax Cloud Vault Index\n\n")
	b.WriteString("> This page is generated by Pinax. The local Markdown vault remains the source of truth.\n\n")
	b.WriteString("## Published Notes\n\n")
	b.WriteString("| Folder | Title | Status | Renderer | Link |\n| --- | --- | --- | --- | --- |\n")
	for _, mapping := range mappings {
		if mapping.PublishStatus == domain.PublishDocStatusDetached {
			continue
		}
		folder := filepath.ToSlash(filepath.Dir(mapping.RemotePath))
		if folder == "." || folder == "" {
			folder = "root"
		}
		title := strings.TrimSuffix(filepath.Base(mapping.RemotePath), ".md")
		if title == "." || title == "" {
			title = mapping.NoteID
		}
		renderer := string(mapping.ResolveDocMappingRenderer())
		if renderer == "" {
			renderer = "unknown"
		}
		link := mapping.ExternalObject.URL
		if link != "" {
			link = "[Open](" + link + ")"
		}
		b.WriteString("| ")
		b.WriteString(strings.ReplaceAll(folder, "|", "\\|"))
		b.WriteString(" | ")
		b.WriteString(strings.ReplaceAll(title, "|", "\\|"))
		b.WriteString(" | ")
		b.WriteString(string(mapping.PublishStatus))
		b.WriteString(" | ")
		b.WriteString(renderer)
		b.WriteString(" | ")
		b.WriteString(link)
		b.WriteString(" |\n")
	}
	return b.String()
}

func publishDocTarget(value string) (domain.PublishDocTarget, *domain.CommandError) {
	switch domain.PublishDocTarget(strings.TrimSpace(value)) {
	case domain.PublishDocTargetLarkDoc:
		return domain.PublishDocTargetLarkDoc, nil
	case domain.PublishDocTargetNotionPage:
		return domain.PublishDocTargetNotionPage, nil
	default:
		return "", &domain.CommandError{Code: "publish_doc_target_invalid", Message: "Document publish target is invalid", Hint: "Use notion-page or lark-doc"}
	}
}

func validatePublishDocProfile(profile domain.PublishDocProfile) *domain.CommandError {
	switch profile.As {
	case "", "auto", "user", "bot":
	default:
		return &domain.CommandError{Code: "publish_doc_profile_invalid", Message: "provider identity must be auto, user, or bot", Hint: "Use --as user, --as bot, or --as auto"}
	}
	switch profile.Target {
	case domain.PublishDocTargetLarkDoc:
		if strings.TrimSpace(profile.Folder) == "" {
			return &domain.CommandError{Code: "publish_doc_profile_invalid", Message: "lark-doc folder is required", Hint: "Use --folder <folder-token>"}
		}
	case domain.PublishDocTargetNotionPage:
		if strings.TrimSpace(profile.ParentPage) == "" {
			return &domain.CommandError{Code: "publish_doc_profile_invalid", Message: "notion-page parent page is required", Hint: "Use --parent-page <page-id>"}
		}
	default:
		return &domain.CommandError{Code: "publish_doc_target_invalid", Message: "Document publish target is invalid", Hint: "Use notion-page or lark-doc"}
	}
	return nil
}

func readPublishDocProfileRequest(req PublishRequest, command string) (string, domain.PublishDocProfile, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", domain.PublishDocProfile{}, err
	}
	target, cmdErr := publishDocTarget(req.Target)
	if cmdErr != nil {
		return "", domain.PublishDocProfile{}, cmdErr
	}
	profile, err := readPublishDocProfile(root, target)
	if err != nil {
		return "", domain.PublishDocProfile{}, err
	}
	if err := validatePublishDocProfile(profile); err != nil {
		return "", domain.PublishDocProfile{}, err
	}
	_ = command
	return root, profile, nil
}

func publishDocDigest(note domain.Note) string {
	keys := make([]string, 0, len(note.Frontmatter))
	for key := range note.Frontmatter {
		switch key {
		case "title", "kind", "status", "publish", "tags":
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(note.ID)
	b.WriteString("\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(note.Frontmatter[key])
		b.WriteString("\n")
	}
	b.WriteString(strings.TrimSpace(note.Body))
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func publishDocPackageID(noteID string, target domain.PublishDocTarget, ts time.Time) string {
	base := strings.NewReplacer(":", "", "-", "", ".", "").Replace(ts.UTC().Format("20060102T150405.000000000Z"))
	return "pubdoc_" + sanitizePublishDocID(noteID) + "_" + sanitizePublishDocID(string(target)) + "_" + base
}

func sanitizePublishDocID(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == '.' || r == '/' {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}

func publishDocRoot(root string) string { return filepath.Join(root, ".pinax", "publish", "doc") }

func publishDocProfileRelPath(target domain.PublishDocTarget) string {
	return filepath.ToSlash(filepath.Join(".pinax", "publish", "doc", "profiles", string(target)+".yaml"))
}

func publishDocPackageRelPath(id string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "publish", "doc", "packages", id+".json"))
}

func writePublishDocProfile(root string, profile domain.PublishDocProfile) error {
	path := filepath.Join(publishDocRoot(root), "profiles", string(profile.Target)+".yaml")
	body, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	return writePublishFile(path, body)
}

func readPublishDocProfile(root string, target domain.PublishDocTarget) (domain.PublishDocProfile, error) {
	path := filepath.Join(publishDocRoot(root), "profiles", string(target)+".yaml")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.PublishDocProfile{}, &domain.CommandError{Code: "publish_doc_profile_not_found", Message: "Document publish profile not found", Hint: "Run pinax publish doc profile set " + shellQuote(string(target)) + " --vault <vault> --json"}
	}
	if err != nil {
		return domain.PublishDocProfile{}, err
	}
	var profile domain.PublishDocProfile
	if err := yaml.Unmarshal(body, &profile); err != nil {
		return domain.PublishDocProfile{}, &domain.CommandError{Code: "publish_doc_profile_invalid", Message: err.Error(), Hint: "Run pinax publish doc profile set again"}
	}
	return profile, nil
}

func writePublishDocPackage(root string, pkg domain.PublishDocPackage) error {
	body, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(publishDocRoot(root), "packages", pkg.ID+".json"), append(body, '\n'))
}

func readPublishDocPackage(root, id string) (domain.PublishDocPackage, error) {
	if strings.TrimSpace(id) == "" {
		return domain.PublishDocPackage{}, &domain.CommandError{Code: "publish_package_required", Message: "Publish package is required", Hint: "Use --package <package-id>"}
	}
	body, err := os.ReadFile(filepath.Join(publishDocRoot(root), "packages", id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return domain.PublishDocPackage{}, &domain.CommandError{Code: "publish_package_not_found", Message: "Publish package not found", Hint: "Run pinax publish doc prepare first"}
	}
	if err != nil {
		return domain.PublishDocPackage{}, err
	}
	var pkg domain.PublishDocPackage
	if err := json.Unmarshal(body, &pkg); err != nil {
		return domain.PublishDocPackage{}, err
	}
	return pkg, nil
}

func writePublishDocMapping(root string, mapping domain.PublishDocMapping) error {
	body, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(publishDocRoot(root), "mappings", sanitizePublishDocID(mapping.NoteID), string(mapping.Target)+".json")
	return writePublishFile(path, append(body, '\n'))
}

func readPublishDocMapping(root, noteID string, target domain.PublishDocTarget) (domain.PublishDocMapping, error) {
	body, err := os.ReadFile(filepath.Join(publishDocRoot(root), "mappings", sanitizePublishDocID(noteID), string(target)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return domain.PublishDocMapping{}, &domain.CommandError{Code: "publish_mapping_not_found", Message: "Document publish mapping was not found", Hint: "Run pinax publish doc push first"}
	}
	if err != nil {
		return domain.PublishDocMapping{}, err
	}
	var mapping domain.PublishDocMapping
	if err := json.Unmarshal(body, &mapping); err != nil {
		return domain.PublishDocMapping{}, err
	}
	return mapping, nil
}

func listPublishDocMappings(root, target string) ([]domain.PublishDocMapping, error) {
	dir := filepath.Join(publishDocRoot(root), "mappings")
	var mappings []domain.PublishDocMapping
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return mappings, nil
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var mapping domain.PublishDocMapping
		if err := json.Unmarshal(body, &mapping); err != nil {
			return err
		}
		if target == "" || string(mapping.Target) == target {
			mappings = append(mappings, mapping)
		}
		return nil
	})
	return mappings, err
}

func writePublishDocReceipt(root, command, noteID, packageID string, target domain.PublishDocTarget, status, externalURL string) error {
	now := time.Now().UTC()
	receipt := domain.PublishDocReceipt{SchemaVersion: domain.PublishDocReceiptSchemaVersion, RunID: publishRunID(now), Command: command, NoteID: noteID, PackageID: packageID, Target: target, Status: status, StartedAt: now.Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), ExternalURL: externalURL}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(publishDocRoot(root), "runs", receipt.RunID, "receipt.json"), append(body, '\n'))
}

func publishDocProviderDoctor(ctx context.Context, profile domain.PublishDocProfile) *domain.CommandError {
	name := publishDocExecutable(profile.Target)
	if _, err := exec.LookPath(name); err != nil {
		return &domain.CommandError{Code: "provider_cli_not_found", Message: "Provider CLI was not found", Hint: "Install or configure " + name}
	}
	if profile.Target == domain.PublishDocTargetLarkDoc {
		if profile.As == "user" || profile.As == "bot" {
			out, err := runPublishDocCLI(ctx, name, []string{"auth", "status"}, "")
			if err != nil {
				return err
			}
			if !publishDocLarkIdentityReady(out, profile.As) {
				return &domain.CommandError{Code: "provider_auth_failed", Message: "Provider identity is not authorized", Hint: "Re-authorize the requested lark-cli identity outside the vault, then retry the publish command."}
			}
		}
		return nil
	}
	_, err := runPublishDocCLI(ctx, name, []string{"doctor"}, "")
	if err != nil {
		return err
	}
	return nil
}

func publishDocLarkIdentityReady(body []byte, identity string) bool {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	identities, ok := payload["identities"].(map[string]any)
	if !ok {
		return false
	}
	entry, ok := identities[identity].(map[string]any)
	if !ok {
		return false
	}
	if available, ok := entry["available"].(bool); ok && available {
		return true
	}
	status, _ := entry["status"].(string)
	return status == "ready"
}

func publishDocLarkArgs(profile domain.PublishDocProfile, args ...string) []string {
	out := append([]string{}, args...)
	if profile.As != "" && profile.As != "auto" {
		out = append(out, "--as", profile.As)
	}
	return out
}

func publishDocProviderPreflight(ctx context.Context, profile domain.PublishDocProfile, pkg domain.PublishDocPackage, note domain.Note) *domain.CommandError {
	if profile.Target == domain.PublishDocTargetLarkDoc {
		// native-docx dry-run 探测原生文档能力，不走 markdown +create。
		if profile.ResolveDocRenderer() == domain.PublishDocRendererNativeDocx {
			return publishDocNativeCheckCapability(ctx, profile)
		}
		path, cleanup, fileErr := writePublishDocTempMarkdown(note, pkg.BodyMarkdown)
		if fileErr != nil {
			return fileErr
		}
		defer cleanup()
		_, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "markdown", "+create", "--folder-token", profile.Folder, "--name", publishDocFileName(note), "--file", path, "--dry-run", "--json"), "")
		return err
	}
	_, err := runPublishDocCLI(ctx, publishDocExecutable(profile.Target), []string{"preflight"}, "")
	_ = pkg
	return err
}

func publishDocProviderCreate(ctx context.Context, profile domain.PublishDocProfile, pkg domain.PublishDocPackage, note domain.Note, folderToken string) (publishDocProviderResult, *domain.CommandError) {
	if profile.Target == domain.PublishDocTargetLarkDoc {
		path, cleanup, fileErr := writePublishDocTempMarkdown(note, pkg.BodyMarkdown)
		if fileErr != nil {
			return publishDocProviderResult{}, fileErr
		}
		defer cleanup()
		out, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "markdown", "+create", "--folder-token", folderToken, "--name", publishDocFileName(note), "--file", path, "--json"), "")
		if err != nil {
			return publishDocProviderResult{}, err
		}
		return parsePublishDocProviderResult(out, profile.Target), nil
	}
	out, err := runPublishDocCLI(ctx, publishDocExecutable(profile.Target), []string{"create"}, publishDocMarkdown(note))
	if err != nil {
		return publishDocProviderResult{}, err
	}
	_ = pkg
	return parsePublishDocProviderResult(out, profile.Target), nil
}

func publishDocProviderUpdate(ctx context.Context, profile domain.PublishDocProfile, mapping domain.PublishDocMapping, pkg domain.PublishDocPackage, note domain.Note) (publishDocProviderResult, *domain.CommandError) {
	if profile.Target == domain.PublishDocTargetLarkDoc {
		path, cleanup, fileErr := writePublishDocTempMarkdown(note, pkg.BodyMarkdown)
		if fileErr != nil {
			return publishDocProviderResult{}, fileErr
		}
		defer cleanup()
		out, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "markdown", "+overwrite", "--file-token", mapping.ExternalObject.ID, "--name", publishDocFileName(note), "--file", path, "--json"), "")
		if err != nil {
			return publishDocProviderResult{}, err
		}
		result := parsePublishDocProviderResult(out, profile.Target)
		if result.ID == "" {
			result.ID = mapping.ExternalObject.ID
		}
		return result, nil
	}
	out, err := runPublishDocCLI(ctx, publishDocExecutable(profile.Target), []string{"update"}, publishDocMarkdown(note))
	if err != nil {
		return publishDocProviderResult{}, err
	}
	_ = pkg
	return parsePublishDocProviderResult(out, profile.Target), nil
}

func runPublishDocCLI(ctx context.Context, name string, args []string, stdin string) ([]byte, *domain.CommandError) {
	return runPublishDocCLIDir(ctx, "", name, args, stdin)
}

// runPublishDocCLIDir 在指定工作目录运行 provider CLI。
// docs +media-insert 要求 --file 是 cwd 内的相对路径（lark-cli 安全限制），
// 因此上传 vault 内图片时 dir 必须设为 vault root。
func runPublishDocCLIDir(ctx context.Context, dir, name string, args []string, stdin string) ([]byte, *domain.CommandError) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		combined := stderr.String()
		if combined == "" {
			combined = stdout.String()
		}
		code := publishDocProviderErrorCode(combined)
		return nil, &domain.CommandError{Code: code, Message: "Provider CLI failed", Hint: publishDocProviderSafeHint(code)}
	}
	return stdout.Bytes(), nil
}

func parsePublishDocProviderResult(body []byte, target domain.PublishDocTarget) publishDocProviderResult {
	var payload map[string]any
	_ = json.Unmarshal(extractPublishDocJSON(body), &payload)
	id := firstString(payload, "id", "file_token", "folder_token", "token", "obj_token")
	url := firstString(payload, "url", "external_url")
	objType := firstString(payload, "type", "object_type")
	if url == "" && id != "" && target == domain.PublishDocTargetLarkDoc {
		url = "https://www.feishu.cn/drive/file/" + id
	}
	return publishDocProviderResult{ID: id, URL: url, ObjectType: objType}
}

func extractPublishDocJSON(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] == '{' {
		return trimmed
	}
	if i := bytes.IndexByte(trimmed, '{'); i >= 0 {
		return trimmed[i:]
	}
	return trimmed
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
		if data, ok := payload["data"].(map[string]any); ok {
			if value, ok := data[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func publishDocProviderErrorCode(stderr string) string {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "forbidden") || strings.Contains(lower, "permission") || strings.Contains(lower, "write access") {
		return "provider_permission_denied"
	}
	if strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "unauthorized") {
		return "provider_auth_failed"
	}
	return "provider_preflight_failed"
}

func publishDocProviderSafeHint(code string) string {
	switch code {
	case "provider_permission_denied":
		return "Grant the current provider identity write access to the target folder or re-run with an authorized user identity."
	case "provider_auth_failed":
		return "Re-authorize the provider CLI outside the vault, then retry the publish command."
	default:
		return "Check provider CLI configuration and retry with --dry-run first."
	}
}

func publishDocExecutable(target domain.PublishDocTarget) string {
	if target == domain.PublishDocTargetLarkDoc {
		return "lark-cli"
	}
	return "ntn"
}

func publishDocExternalType(profile domain.PublishDocProfile) string {
	if profile.Target == domain.PublishDocTargetLarkDoc {
		// native-docx 产出 docx/doc（由 provider inspect 决定）；markdown-file 产出 file。
		if profile.ResolveDocRenderer() == domain.PublishDocRendererMarkdownFile {
			return domain.PublishDocObjectTypeFile
		}
		return domain.PublishDocObjectTypeDocx
	}
	return domain.PublishDocObjectTypePage
}

// publishDocRenderer 解析 --renderer flag 并按 target 应用默认值和校验。
// lark-doc 新 profile 默认 native-docx；旧 profile（磁盘无 renderer 字段）按 markdown-file 兼容读取。
// 未知 renderer 返回稳定 validation error，不写入 profile。
func publishDocRenderer(value string, target domain.PublishDocTarget) (domain.PublishDocRenderer, *domain.CommandError) {
	value = strings.TrimSpace(value)
	if target == domain.PublishDocTargetLarkDoc {
		switch domain.PublishDocRenderer(value) {
		case "":
			return domain.PublishDocRendererNativeDocx, nil
		case domain.PublishDocRendererNativeDocx, domain.PublishDocRendererMarkdownFile:
			return domain.PublishDocRenderer(value), nil
		default:
			return "", &domain.CommandError{Code: "publish_doc_renderer_invalid", Message: "Document publish renderer is invalid", Hint: "Use --renderer native-docx or --renderer markdown-file"}
		}
	}
	// 非 lark target 不接受 renderer 字段。
	if value != "" {
		return "", &domain.CommandError{Code: "publish_doc_renderer_invalid", Message: "Renderer is only supported for lark-doc", Hint: "Remove --renderer or use target lark-doc"}
	}
	return "", nil
}

func publishDocMarkdown(note domain.Note) string {
	body := strings.TrimSpace(note.Body)
	if body == "" {
		body = "# " + note.Title
	}
	return body + "\n"
}

func writePublishDocTempMarkdown(note domain.Note, bodyMarkdown string) (string, func(), *domain.CommandError) {
	dir, err := os.MkdirTemp(".", ".pinax-publish-doc-*")
	if err != nil {
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to create temporary publish document", Hint: "Check current directory permissions"}
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, sanitizePublishDocID(note.ID)+".md")
	if strings.TrimSpace(bodyMarkdown) == "" {
		bodyMarkdown = publishDocMarkdown(note)
	}
	if err := os.WriteFile(path, []byte(bodyMarkdown), 0o600); err != nil {
		cleanup()
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to write temporary publish document", Hint: "Check current directory permissions"}
	}
	return filepath.ToSlash(path), cleanup, nil
}

func publishDocFileName(note domain.Note) string {
	name := strings.TrimSpace(note.Title)
	if name == "" {
		name = note.ID
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	return name
}

func normalizePublishDocFolder(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, "/drive/folder/"); i >= 0 {
		value = value[i+len("/drive/folder/"):]
		if j := strings.IndexAny(value, "?#/"); j >= 0 {
			value = value[:j]
		}
	}
	return value
}

func publishDocExternalIDFromURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, "/")
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}
