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
	VaultPath         string
	Backend           string
	Provider          string
	Model             string
	Limit             int
	Query             string
	SidecarExecutable string
	SidecarTimeout    time.Duration
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
	for _, plan := range plans {
		content, err := os.ReadFile(plan.SourcePath)
		if err != nil {
			return errorProjection("kb.import", err), err
		}
		body := string(content)
		if strings.EqualFold(filepath.Ext(plan.SourcePath), ".txt") {
			body = "# " + plan.Title + "\n\n" + body
		}
		output := buildNoteContentWithStatus(plan.Title, plan.TargetPath, "", "kb/imports", "reference", []string{"kb", "imported"}, "active", now, body)
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
	receiptRel, err := writeReceipt(root, "kb-import", map[string]any{"source": source, "imported": imported, "plans": plans})
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
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.rebuild", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("kb.rebuild", err), err
	}
	provider, err := semantic.NewProvider(req.Provider, req.Model)
	if err != nil {
		return commandErrorProjection("kb.rebuild", err)
	}
	chunks, err := semantic.BuildChunks(ctx, notes, provider, req.Backend)
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
		}
		cmdErr := &domain.CommandError{Code: "embedding_provider_failed", Message: "Embedding provider failed", Hint: "Check provider credentials or use --provider fake for local validation"}
		return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
	}
	storePath, err := semantic.Save(ctx, root, chunks, req.Backend, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout}, len(notes))
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection("kb.rebuild", cmdErr), cmdErr
		}
		return errorProjection("kb.rebuild", err), err
	}
	projection := domain.NewProjection("kb.rebuild", "KB semantic projection rebuilt.")
	projection.Facts["backend"] = semantic.DefaultBackend
	if strings.TrimSpace(req.Backend) != "" {
		projection.Facts["backend"] = strings.ToLower(strings.TrimSpace(req.Backend))
	}
	projection.Facts["provider"] = provider.Name()
	projection.Facts["model"] = provider.Model()
	projection.Facts["documents"] = fmt.Sprint(len(notes))
	projection.Facts["chunks"] = fmt.Sprint(len(chunks))
	projection.Facts["sync_vectors"] = "false"
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "kb", projection.Facts["backend"]))}
	projection.Data = map[string]any{"documents": len(notes), "chunks": len(chunks), "backend": projection.Facts["backend"], "provider": provider.Name(), "model": provider.Model(), "store_path": storePath}
	return projection, nil
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
	result, err := semantic.Doctor(ctx, root, backend, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout})
	if err != nil {
		if cmdErr, ok := err.(*domain.CommandError); ok {
			return domain.NewErrorProjection("kb.doctor", cmdErr), cmdErr
		}
		return errorProjection("kb.doctor", err), err
	}
	projection := domain.NewProjection("kb.doctor", "KB backend check completed.")
	projection.Facts["backend"] = fmt.Sprint(result["backend"])
	projection.Facts["available"] = fmt.Sprint(result["available"])
	projection.Facts["sidecar_executable"] = req.SidecarExecutable
	if dependency := strings.TrimSpace(fmt.Sprint(result["dependency"])); dependency != "" {
		projection.Facts["dependency"] = dependency
	}
	projection.Data = result
	return projection, nil
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
	result, err := semantic.DoctorProvider(ctx, providerName, req.Model)
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
	for _, key := range []string{"provider", "model", "configured", "available", "credential_source", "local_only"} {
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
	var provider semantic.Provider
	if strings.TrimSpace(req.Provider) != "" || strings.TrimSpace(req.Model) != "" {
		provider, err = semantic.NewProvider(req.Provider, req.Model)
		if err != nil {
			return commandErrorProjection(command, err)
		}
	}
	hits, total, err := semantic.Search(ctx, root, req.Query, provider, req.Backend, req.Limit, semantic.SidecarConfig{Executable: req.SidecarExecutable, Timeout: req.SidecarTimeout})
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
	projection.Data = map[string]any{"query": req.Query, "backend": backend, "provider": providerName, "model": modelName, "total": total, "hits": hits}
	return projection, nil
}

type kbImportPlan struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Title      string `json:"title"`
	Status     string `json:"status"`
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
	add := func(path string) error {
		if !kbPathIncluded(path, includes) {
			return nil
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		rel := filepath.ToSlash(filepath.Join("notes", "kb", "imports", slugify(title)+".md"))
		plans = append(plans, kbImportPlan{SourcePath: path, TargetPath: rel, Title: title, Status: "write"})
		return nil
	}
	if !info.IsDir() {
		if err := add(source); err != nil {
			return nil, err
		}
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
			return add(path)
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
