package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

type KBImportRequest struct {
	VaultPath string
	Source    string
	Includes  []string
	DryRun    bool
	Yes       bool
}

type KBIndexRequest struct {
	VaultPath string
	Backend   string
	Provider  string
	Model     string
	Limit     int
	Query     string
	// PermissionResolved marks that the caller has completed Pinax-owned
	// permission resolution. When it is true, AllowedIDs is tri-state: nil is
	// an unresolved error, an empty slice is an explicit zero-result decision,
	// and a non-empty slice is passed to Inferrum as the allow-list.
	PermissionResolved bool
	AllowedIDs         []string
	// GenerationID and StoreURI are internal candidate/active pins. Ordinary
	// CLI search leaves them empty and resolves the active descriptor.
	GenerationID      string
	StoreURI          string
	LegacyV1Readonly  bool
	SidecarExecutable string
	SidecarTimeout    time.Duration
	// DiskAdmission is supplied by the CLI/config boundary. A nil policy keeps
	// direct component tests and library callers opt-in; the production CLI
	// supplies the conservative default policy for rebuild/refresh.
	DiskAdmission *KBDiskAdmissionPolicy
}

func (s *Service) KBImport(_ context.Context, req KBImportRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.import", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("kb.import", err), err
	}
	source, err := cleanVaultPath(req.Source)
	if err != nil {
		return errorProjection("kb.import", err), err
	}
	plans, err := planKBImport(root, source, req.Includes)
	if err != nil {
		return errorProjection("kb.import", err), err
	}
	projection := domain.NewProjection("kb.import", "KB import plan generated.")
	projection.Facts["planned"] = fmt.Sprint(len(plans))
	projection.Facts["imported"] = "0"
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Data = map[string]any{"plans": plans, "dry_run": req.DryRun}
	if req.DryRun {
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "kb import requires --yes", Hint: "Preview with --dry-run, then add --yes after confirming"}
		return domain.NewErrorProjection("kb.import", err), err
	}
	imported := 0
	now := time.Now().UTC().Format(time.RFC3339)
	for index, plan := range plans {
		content, err := os.ReadFile(plan.SourcePath)
		if err != nil {
			return errorProjection("kb.import", err), err
		}
		if plan.SourceDigest != "" && kbContentDigest(content) != plan.SourceDigest {
			err := &domain.CommandError{Code: "kb_import_source_changed", Message: "KB import source changed after planning", Hint: "Re-run dry-run and confirm against the current Markdown or text source"}
			return domain.NewErrorProjection("kb.import", err), err
		}
		plans[index].AcquiredAt = now
		plan.AcquiredAt = now
		body := string(content)
		if strings.EqualFold(filepath.Ext(plan.SourcePath), ".txt") {
			body = "# " + plan.Title + "\n\n" + body
		}
		output := buildKBImportedNoteContent(plan, body, now)
		target, err := safeJoin(root, plan.TargetPath)
		if err != nil {
			return errorProjection("kb.import", err), err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("kb.import", err), err
		}
		if err := os.WriteFile(target, []byte(output), 0o644); err != nil {
			return errorProjection("kb.import", err), err
		}
		imported++
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("kb.import", err), err
	}
	receiptRel, err := writeReceipt(root, "kb-import", map[string]any{"source_type": "local_markdown", "source_ref": kbSafeSourceRef(root, source), "imported": imported, "plans": plans})
	if err != nil {
		return errorProjection("kb.import", err), err
	}
	_ = appendEvent(root, "kb.import", "success", map[string]string{"imported": fmt.Sprint(imported), "receipt_path": receiptRel})
	projection.Summary = "KB content imported."
	projection.Facts["imported"] = fmt.Sprint(imported)
	projection.Facts["index_updated"] = "true"
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{receiptRel}
	projection.Data = map[string]any{"plans": plans, "imported": imported, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) KBRebuild(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	return s.kbRebuildStaged(ctx, req)
}

func (s *Service) KBRefresh(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	projection, err := s.KBRebuild(ctx, req)
	projection.Command = "kb.refresh"
	projection.Summary = "KB semantic projection refreshed."
	return projection, err
}

func (s *Service) KBSearch(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	return s.kbSearchProjection(ctx, "kb.search", "KB semantic search completed.", req)
}

func (s *Service) KBContext(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	if req.Limit == 0 {
		req.Limit = 8
	}
	return s.kbSearchProjection(ctx, "kb.context", "KB bounded context generated.", req)
}

func (s *Service) KBDoctor(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.doctor", err), err
	}
	backend := strings.TrimSpace(req.Backend)
	if backend == "" {
		backend = semantic.DefaultBackend
	}
	if legacy, legacyErr := semantic.DetectLegacyV1(root); legacyErr != nil {
		return errorProjection("kb.doctor", legacyErr), legacyErr
	} else if legacy.Present {
		projection := domain.NewProjection("kb.doctor", "Legacy KB projection detected in read-only compatibility mode.")
		projection.Facts["backend"] = backend
		projection.Facts["available"] = "true"
		projection.Facts["protocol"] = legacy.Protocol
		projection.Facts["profile"] = "legacy_v1_readonly"
		projection.Facts["compatibility_status"] = "active"
		projection.Facts["introduced_release"] = "N"
		projection.Facts["compatible_through_release"] = "N+1"
		projection.Facts["removal_eligible_release"] = "N+2"
		projection.Facts["next_action"] = "rebuild_inferrum_v1"
		projection.Facts["sidecar_executable"] = req.SidecarExecutable
		projection.Data = map[string]any{"backend": backend, "available": true, "protocol": legacy.Protocol, "profile": "legacy_v1_readonly", "compatibility_status": "active", "next_action": "pinax kb rebuild --backend lancedb --provider ollama --model pinax-qwen3-embedding:lowmem --vault <vault> --json"}
		return projection, nil
	}
	result, err := semantic.Doctor(ctx, root, backend, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout})
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			projection := domain.NewErrorProjection("kb.doctor", cmdErr)
			projection.Facts["backend"] = backend
			projection.Facts["sidecar_executable"] = req.SidecarExecutable
			projection.Data = map[string]any{"generation": fillKBDoctorGenerationFacts(&projection, root)}
			return projection, cmdErr
		}
		projection := errorProjection("kb.doctor", err)
		projection.Facts["backend"] = backend
		projection.Facts["sidecar_executable"] = req.SidecarExecutable
		projection.Data = map[string]any{"generation": fillKBDoctorGenerationFacts(&projection, root)}
		return projection, err
	}
	projection := domain.NewProjection("kb.doctor", "KB backend check completed.")
	projection.Facts["backend"] = fmt.Sprint(result["backend"])
	projection.Facts["available"] = fmt.Sprint(result["available"])
	projection.Facts["sidecar_executable"] = req.SidecarExecutable
	if dependency := strings.TrimSpace(fmt.Sprint(result["dependency"])); dependency != "" {
		projection.Facts["dependency"] = dependency
	}
	result["generation"] = fillKBDoctorGenerationFacts(&projection, root)
	projection.Data = result
	return projection, nil
}

// fillKBDoctorGenerationFacts adds only redacted identity and readiness facts
// from Pinax-owned generation state. It deliberately does not infer retrieval
// readiness from sidecar reachability or expose the generation store path.
func fillKBDoctorGenerationFacts(projection *domain.Projection, root string) map[string]any {
	state := map[string]any{}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		projection.Facts["generation_status"] = "invalid"
		projection.Facts["index_status"] = "failed"
		projection.Facts["rollback_available"] = "false"
		state["status"] = "invalid"
		state["index_status"] = "failed"
		state["rollback_available"] = false
		return state
	}
	projection.Facts["activation_sequence"] = fmt.Sprint(descriptor.Sequence)
	state["activation_sequence"] = descriptor.Sequence
	if descriptor.Active == nil {
		indexStatus := "missing"
		if _, statErr := os.Stat(filepath.Join(kbRoot(root), "generations")); statErr == nil {
			indexStatus = "candidate_only"
		}
		projection.Facts["generation_status"] = "none"
		projection.Facts["index_status"] = indexStatus
		projection.Facts["rollback_available"] = "false"
		state["status"] = "none"
		state["index_status"] = indexStatus
		state["rollback_available"] = false
		return state
	}

	manifest, manifestErr := ReadKBGenerationManifest(root, descriptor.Active.GenerationID)
	if manifestErr != nil {
		projection.Facts["generation_id"] = descriptor.Active.GenerationID
		projection.Facts["generation_status"] = "invalid"
		projection.Facts["index_status"] = "failed"
		projection.Facts["rollback_available"] = fmt.Sprint(descriptor.Previous != nil)
		state["generation_id"] = descriptor.Active.GenerationID
		state["status"] = "invalid"
		state["index_status"] = "failed"
		state["rollback_available"] = descriptor.Previous != nil
		return state
	}

	indexStatus := "missing"
	notes, notesErr := scanNotes(root)
	storePath := filepath.Join(kbRoot(root), "generations", manifest.GenerationID, "lancedb")
	if manifest.Status == KBGenerationStatusFailed || manifest.Status == KBGenerationStatusRejected {
		indexStatus = "failed"
	} else if notesErr != nil {
		indexStatus = "unknown"
	} else if kbSourceDigest(notes) != manifest.SourceDigest {
		indexStatus = "stale"
	} else if info, statErr := os.Stat(storePath); statErr == nil && info.IsDir() {
		indexStatus = "fresh"
	}
	projection.Facts["generation_id"] = manifest.GenerationID
	projection.Facts["generation_status"] = string(manifest.Status)
	projection.Facts["index_status"] = indexStatus
	projection.Facts["protocol"] = manifest.Protocol
	projection.Facts["provider"] = manifest.Provider
	projection.Facts["model"] = manifest.Model
	projection.Facts["embedding_dim"] = fmt.Sprint(manifest.EmbeddingDim)
	projection.Facts["source_snapshot"] = manifest.SourceSnapshot
	projection.Facts["source_digest"] = manifest.SourceDigest
	projection.Facts["model_manifest_digest"] = manifest.ModelManifestDigest
	projection.Facts["profile_hash"] = manifest.ProfileHash
	projection.Facts["rollback_available"] = fmt.Sprint(descriptor.Previous != nil)
	if manifest.BaseModelDigest != "" {
		projection.Facts["base_model_digest"] = manifest.BaseModelDigest
	}
	if manifest.DaemonVersion != "" {
		projection.Facts["daemon_version"] = manifest.DaemonVersion
	}
	state["generation_id"] = manifest.GenerationID
	state["status"] = string(manifest.Status)
	state["index_status"] = indexStatus
	state["protocol"] = manifest.Protocol
	state["provider"] = manifest.Provider
	state["model"] = manifest.Model
	state["embedding_dim"] = manifest.EmbeddingDim
	state["source_snapshot"] = manifest.SourceSnapshot
	state["source_digest"] = manifest.SourceDigest
	state["model_manifest_digest"] = manifest.ModelManifestDigest
	state["profile_hash"] = manifest.ProfileHash
	state["rollback_available"] = descriptor.Previous != nil
	return state
}

func (s *Service) KBProviderList(_ context.Context, _ KBIndexRequest) (domain.Projection, error) {
	providers := semantic.ListProviders()
	projection := domain.NewProjection("kb.provider.list", "KB embedding providers listed.")
	projection.Facts["providers"] = fmt.Sprint(len(providers))
	projection.Facts["default_provider"] = semantic.DefaultProvider
	projection.Facts["default_model"] = semantic.DefaultModel
	projection.Data = map[string]any{"providers": providers, "backends": semantic.ListBackends()}
	return projection, nil
}

func (s *Service) KBProviderDoctor(ctx context.Context, req KBIndexRequest) (domain.Projection, error) {
	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		providerName = semantic.DefaultProvider
	}
	result, err := semantic.DoctorProviderForBackend(ctx, req.Backend, providerName, req.Model)
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			projection := domain.NewErrorProjection("kb.provider.doctor", cmdErr)
			fillProviderDoctorProjection(&projection, result)
			return projection, cmdErr
		}
		return errorProjection("kb.provider.doctor", err), err
	}
	projection := domain.NewProjection("kb.provider.doctor", "KB embedding provider check completed.")
	fillProviderDoctorProjection(&projection, result)
	return projection, nil
}

func fillProviderDoctorProjection(projection *domain.Projection, result map[string]any) {
	if result == nil {
		return
	}
	for _, key := range []string{"provider", "model", "configured", "available", "embed_ready", "embedding_dim", "credential_source", "local_only"} {
		if value, ok := result[key]; ok {
			projection.Facts[key] = fmt.Sprint(value)
		}
	}
	projection.Data = result
}

func (s *Service) kbSearchProjection(ctx context.Context, command, summary string, req KBIndexRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if strings.TrimSpace(req.Query) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "kb query is required", Hint: "Run pinax kb search <query> --vault <vault>"}
		return domain.NewErrorProjection(command, err), err
	}
	if req.PermissionResolved && req.AllowedIDs == nil {
		err := &domain.CommandError{Code: "permission_unresolved", Message: "KB permission candidates could not be resolved", Hint: "Resolve Pinax permissions before searching; do not retry as an unrestricted query"}
		return domain.NewErrorProjection(command, err), err
	}
	if !req.PermissionResolved {
		notes, scanErr := scanNotes(root)
		if scanErr != nil {
			return errorProjection(command, scanErr), scanErr
		}
		req.AllowedIDs = semantic.ChunkIDsForNotes(notes)
		req.PermissionResolved = true
	}
	var generation *KBGenerationManifest
	legacyMode := false
	var legacy semantic.LegacyProjection
	if detected, detectErr := semantic.DetectLegacyV1(root); detectErr != nil {
		return errorProjection(command, detectErr), detectErr
	} else if detected.Present && len(req.AllowedIDs) > 0 {
		activation, activationErr := ReadKBActivationDescriptor(root)
		if activationErr != nil {
			return errorProjection(command, activationErr), activationErr
		}
		if activation.Active == nil {
			if !req.LegacyV1Readonly {
				err := &domain.CommandError{Code: "kb_legacy_v1_readonly", Message: "Legacy v1 KB projection requires the explicit read-only profile", Hint: "Pass --legacy-v1-readonly to read it, or run a normal kb rebuild to create an Inferrum v1 candidate"}
				return domain.NewErrorProjection(command, err), err
			}
			legacyMode = true
			legacy = detected
		}
	}
	var provider semantic.Provider
	if len(req.AllowedIDs) > 0 && !legacyMode {
		generation, req.StoreURI, err = resolveKBGenerationForSearch(root, req)
		if err != nil {
			if cmdErr, ok := err.(*domain.CommandError); ok {
				return domain.NewErrorProjection(command, cmdErr), cmdErr
			}
			return errorProjection(command, err), err
		}
		if generation != nil {
			if strings.TrimSpace(req.Provider) == "" {
				req.Provider = generation.Provider
			} else if req.Provider != generation.Provider {
				err := &domain.CommandError{Code: "kb_model_mismatch", Message: "KB query provider does not match the pinned generation", Hint: "Use the generation provider or omit --provider"}
				return domain.NewErrorProjection(command, err), err
			}
			if strings.TrimSpace(req.Model) == "" {
				req.Model = generation.Model
			} else if req.Model != generation.Model {
				err := &domain.CommandError{Code: "kb_model_mismatch", Message: "KB query model does not match the pinned generation", Hint: "Use the generation model or omit --model"}
				return domain.NewErrorProjection(command, err), err
			}
			provider, err = semantic.NewProviderForBackend(generation.Backend, generation.Provider, generation.Model)
			if err != nil {
				return commandErrorProjection(command, err)
			}
			identity, identityErr := semantic.InspectProviderIdentity(ctx, generation.Provider, generation.Model)
			if identityErr != nil {
				return commandErrorProjection(command, identityErr)
			}
			if identity.ModelManifestDigest != generation.ModelManifestDigest || identity.ProfileHash != generation.ProfileHash || (generation.BaseModelDigest != "" && identity.BaseModelDigest != generation.BaseModelDigest) {
				err := &domain.CommandError{Code: "kb_model_mismatch", Message: "KB query provider identity does not match the pinned generation", Hint: "Rebuild and activate a generation for the currently installed exact model tag/profile"}
				return domain.NewErrorProjection(command, err), err
			}
			canary, canaryErr := provider.Embed(ctx, "pinax query dimension canary")
			if canaryErr != nil {
				return commandErrorProjection(command, canaryErr)
			}
			if len(canary) != generation.EmbeddingDim {
				err := &domain.CommandError{Code: "kb_model_mismatch", Message: "KB query embedding dimension does not match the pinned generation", Hint: "Use the exact embedding model and profile used to build the active generation"}
				return domain.NewErrorProjection(command, err), err
			}
		}
	}
	if strings.TrimSpace(req.Provider) != "" || strings.TrimSpace(req.Model) != "" {
		if provider == nil {
			provider, err = semantic.NewProviderForBackend(req.Backend, req.Provider, req.Model)
			if err != nil {
				return commandErrorProjection(command, err)
			}
		}
	}
	allowedIDs := []string(nil)
	if req.PermissionResolved {
		allowedIDs = req.AllowedIDs
	}
	var hits []semantic.SearchHit
	var total int
	if legacyMode {
		if strings.TrimSpace(req.Provider) == "" && legacy.Provider != "" {
			req.Provider = legacy.Provider
		}
		if strings.TrimSpace(req.Model) == "" && legacy.Model != "" {
			req.Model = legacy.Model
		}
		hits, total, err = semantic.SearchLegacyV1(ctx, root, req.Query, provider, req.Limit, allowedIDs)
	} else {
		hits, total, err = semantic.SearchWithAllowedIDs(ctx, root, req.Query, provider, req.Backend, req.Limit, allowedIDs, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout, StoreURI: req.StoreURI})
	}
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection(command, cmdErr), cmdErr
		}
		return errorProjection(command, err), err
	}
	backend := semantic.DefaultBackend
	if strings.TrimSpace(req.Backend) != "" {
		backend = strings.ToLower(strings.TrimSpace(req.Backend))
	}
	projection := domain.NewProjection(command, summary)
	projection.Facts["backend"] = backend
	providerName := strings.TrimSpace(req.Provider)
	modelName := strings.TrimSpace(req.Model)
	if len(hits) > 0 {
		if hits[0].Provider != "" {
			providerName = hits[0].Provider
		}
		if hits[0].Model != "" {
			modelName = hits[0].Model
		}
	}
	if provider != nil {
		providerName = provider.Name()
		modelName = provider.Model()
	}
	if providerName == "" {
		providerName = "indexed"
	}
	if modelName == "" {
		modelName = "indexed"
	}
	projection.Facts["provider"] = providerName
	projection.Facts["model"] = modelName
	projection.Facts["matches"] = fmt.Sprint(len(hits))
	projection.Facts["total"] = fmt.Sprint(total)
	projection.Facts["sync_vectors"] = "false"
	if generation != nil {
		projection.Facts["generation_id"] = generation.GenerationID
		projection.Facts["generation_status"] = string(generation.Status)
		projection.Facts["protocol"] = generation.Protocol
		projection.Facts["embedding_dim"] = fmt.Sprint(generation.EmbeddingDim)
		projection.Facts["source_snapshot"] = generation.SourceSnapshot
		projection.Facts["source_digest"] = generation.SourceDigest
		projection.Facts["model_manifest_digest"] = generation.ModelManifestDigest
		projection.Facts["profile_hash"] = generation.ProfileHash
		if generation.BaseModelDigest != "" {
			projection.Facts["base_model_digest"] = generation.BaseModelDigest
		}
		if generation.DaemonVersion != "" {
			projection.Facts["daemon_version"] = generation.DaemonVersion
		}
		if activation, activationErr := ReadKBActivationDescriptor(root); activationErr == nil && activation.Active != nil {
			projection.Facts["rollback_available"] = fmt.Sprint(activation.Previous != nil)
		}
	}
	if legacyMode {
		projection.Facts["protocol"] = legacy.Protocol
		projection.Facts["profile"] = "legacy_v1_readonly"
		projection.Facts["embedding_dim"] = fmt.Sprint(legacy.EmbeddingDim)
		projection.Facts["next_action"] = "rebuild_inferrum_v1"
	}
	if req.PermissionResolved {
		permissionStatus := "filtered"
		if len(req.AllowedIDs) == 0 {
			permissionStatus = "empty"
		}
		projection.Facts["permission_status"] = permissionStatus
	}
	projection.Data = map[string]any{"query": req.Query, "backend": backend, "provider": providerName, "model": modelName, "total": total, "hits": hits}
	return projection, nil
}

type kbImportPlan struct {
	SourcePath    string `json:"-"`
	TargetPath    string `json:"target_path"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	SourceType    string `json:"source_type"`
	SourceRef     string `json:"source_ref"`
	SourceDigest  string `json:"source_digest"`
	SourceVersion string `json:"source_version"`
	AcquiredAt    string `json:"acquired_at,omitempty"`
	Importer      string `json:"importer_version"`
}

func planKBImport(root, source string, includes []string) ([]kbImportPlan, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if len(includes) == 0 {
		includes = []string{"*.md", "*.txt"}
	}
	plans := []kbImportPlan{}
	add := func(path string) {
		if !kbPathIncluded(path, includes) {
			return
		}
		sourceType, sourceRef, sourceDigest, sourceVersion, _, readErr := kbReadSourceLineage(root, path)
		if readErr != nil {
			return
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		rel := filepath.ToSlash(filepath.Join("notes", "kb", "imports", slugify(title)+".md"))
		plans = append(plans, kbImportPlan{SourcePath: path, TargetPath: rel, Title: title, Status: "write", SourceType: sourceType, SourceRef: sourceRef, SourceDigest: sourceDigest, SourceVersion: sourceVersion, Importer: KBImporterVersion})
	}
	if !info.IsDir() {
		add(source)
	} else {
		err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipVaultWalkDir(entry.Name()) && path != source {
					return filepath.SkipDir
				}
				return nil
			}
			add(path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].SourcePath < plans[j].SourcePath })
	for i := range plans {
		target, err := safeJoin(root, plans[i].TargetPath)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(target); err == nil || targetAlreadyPlanned(plans, i, plans[i].TargetPath) {
			plans[i].TargetPath = uniqueKBImportTarget(root, plans, i)
		}
	}
	return plans, nil
}

func commandErrorProjection(command string, err error) (domain.Projection, error) {
	if cmdErr, ok := err.(*domain.CommandError); ok {
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	return errorProjection(command, err), err
}

func targetAlreadyPlanned(plans []kbImportPlan, current int, target string) bool {
	for i := 0; i < current; i++ {
		if plans[i].TargetPath == target {
			return true
		}
	}
	return false
}

func uniqueKBImportTarget(root string, plans []kbImportPlan, current int) string {
	base := slugify(plans[current].Title)
	for n := 2; ; n++ {
		candidate := filepath.ToSlash(filepath.Join("notes", "kb", "imports", base+"-"+fmt.Sprint(n)+".md"))
		if targetAlreadyPlanned(plans, current, candidate) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(candidate))); os.IsNotExist(err) {
			return candidate
		}
	}
}

func kbPathIncluded(path string, includes []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".md" && ext != ".txt" {
		return false
	}
	base := filepath.Base(path)
	for _, pattern := range includes {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, filepath.ToSlash(path)); ok {
			return true
		}
	}
	return false
}
