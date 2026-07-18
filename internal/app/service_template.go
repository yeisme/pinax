package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/templateengine"
)

// Template operations: catalog init/list/recommend, show/render/preview, inspect,
// create/validate/delete, and template action helpers. Extracted from service.go
// to isolate the template-engine surface.

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
