package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

func (s *Service) DatabaseSchemaInfer(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.schema.infer", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("database.schema.infer", err), err
	}
	notes = ordinaryNotes(notes)
	defs := noteindex.InferPropertyDefinitions(noteindex.ExtractPropertyRows(notes))
	projection := domain.NewProjection("database.schema.infer", "Database property schema inferred.")
	projection.Facts["properties"] = fmt.Sprint(len(defs))
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Data = map[string]any{"properties": defs}
	return projection, nil
}

func (s *Service) DatabaseSchemaSet(_ context.Context, req DatabaseSchemaRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.schema.set", err), err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "property_required", Message: "database schema set requires a property name", Hint: "pinax database schema set status --type select --vault <vault>"}
		return domain.NewErrorProjection("database.schema.set", err), err
	}
	propertyType := strings.TrimSpace(req.Type)
	if propertyType == "" {
		err := &domain.CommandError{Code: "property_type_required", Message: "database schema set requires --type", Hint: "Use --type string|number|boolean|date|select|list|link"}
		return domain.NewErrorProjection("database.schema.set", err), err
	}
	values, tagErr := normalizeTagsForWrite(req.Values)
	if tagErr != nil {
		return domain.NewErrorProjection("database.schema.set", tagErr), tagErr
	}
	payload := map[string]any{"schema_version": "pinax.schema_overrides.v1", "properties": map[string]any{name: map[string]any{"type": propertyType, "values": values}}}
	path := filepath.Join(root, ".pinax", "schema-overrides.json")
	if err := writeJSONAsset(path, payload); err != nil {
		return errorProjection("database.schema.set", err), err
	}
	_ = appendEvent(root, "database.schema.set", "success", map[string]string{"property": name, "type": propertyType})
	projection := domain.NewProjection("database.schema.set", "Database property schema saved.")
	projection.Facts["property"] = name
	projection.Facts["type"] = propertyType
	projection.Facts["path"] = filepath.ToSlash(filepath.Join(".pinax", "schema-overrides.json"))
	projection.Evidence = []string{projection.Facts["path"]}
	projection.Data = payload
	return projection, nil
}

func (s *Service) SaveDatabaseView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.view.save", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("database.view.save", err), err
	}
	registry.SchemaVersion = "pinax.views.v2"
	view := domain.SavedView{ID: stableViewID(req.Name), Name: strings.TrimSpace(req.Name), Kind: strings.TrimSpace(req.Kind), Query: strings.TrimSpace(req.Query), Columns: req.Columns, Limit: req.Limit, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if view.Kind == "" {
		view.Kind = "table"
	}
	upsertSavedView(&registry, view)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("database.view.save", err), err
	}
	projection := domain.NewProjection("database.view.save", "Database view saved.")
	projection.Facts["view"] = view.Name
	projection.Facts["schema_version"] = registry.SchemaVersion
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "views.json"))}
	projection.Data = map[string]any{"view": view}
	return projection, nil
}

func (s *Service) ShowDatabaseView(ctx context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.view.show", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("database.view.show", err), err
	}
	view, ok := findSavedView(registry, req.Name)
	if !ok {
		err := &domain.CommandError{Code: "view_not_found", Message: "Saved view not found", Hint: "pinax database view list --vault <vault>"}
		return domain.NewErrorProjection("database.view.show", err), err
	}
	if strings.TrimSpace(view.Query) == "" {
		projection, err := s.ShowView(ctx, req)
		projection.Command = "database.view.show"
		return projection, err
	}
	projection, err := s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: view.Query, Limit: view.Limit, LazyIndex: true})
	projection.Command = "database.view.show"
	projection.Summary = "Database view queried."
	projection.Facts["view"] = view.Name
	projection.Data = map[string]any{"view": view, "result": projection.Data}
	return projection, err
}

func stableViewID(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, " ", "-")
	if clean == "" {
		return "view"
	}
	return "view_" + clean
}

func (s *Service) QueryRun(ctx context.Context, req QueryRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("query.run", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("query.run", err), err
	}
	notes = ordinaryNotes(notes)
	status, _ := noteindex.Inspect(root, notes)
	indexLoaded := ""
	if status.Status != "fresh" {
		if !req.LazyIndex {
			code := "property_index_stale"
			message := "Query index is not fresh"
			if status.Status == "missing" {
				code = "index_required"
				message = "Query requires building the local index first"
			}
			err := &domain.CommandError{Code: code, Message: message, Hint: "Run pinax index rebuild --vault " + shellQuote(root) + ", or add --lazy-index explicitly"}
			return domain.NewErrorProjection("query.run", err), err
		}
		select {
		case <-ctx.Done():
			return errorProjection("query.run", ctx.Err()), ctx.Err()
		default:
		}
		if _, err := noteindex.Rebuild(root, notes); err != nil {
			return errorProjection("query.run", err), err
		}
		status, _ = noteindex.Inspect(root, notes)
		indexLoaded = "lazy_rebuild"
	}
	ast, err := searchops.ParseSQL(req.SQL)
	if err != nil {
		return errorProjection("query.run", err), err
	}
	result := searchops.ExecuteQuery(notes, ast, searchops.QueryRequest{Limit: req.Limit, Sort: req.Sort, Cursor: req.Cursor})
	projection := domain.NewProjection("query.run", "Query executed.")
	projection.Facts["engine"] = "planner"
	projection.Facts["index_status"] = status.Status
	projection.Facts["columns"] = strings.Join(result.Columns, ",")
	projection.Facts["rows"] = fmt.Sprint(result.RowCount())
	projection.Facts["returned"] = fmt.Sprint(result.RowCount())
	projection.Facts["limit"] = fmt.Sprint(result.Page.Limit)
	projection.Facts["has_more"] = fmt.Sprint(result.Page.HasMore)
	if result.Page.NextCursor != "" {
		projection.Facts["next_cursor"] = result.Page.NextCursor
	}
	if indexLoaded != "" {
		projection.Facts["index_loaded"] = indexLoaded
	}
	projection.Data = map[string]any{"result": result, "ast": ast, "warnings": []string{}}
	return projection, nil
}

func (s *Service) QueryExplain(_ context.Context, req QueryRequest) (domain.Projection, error) {
	ast, err := searchops.ParseSQL(req.SQL)
	if err != nil {
		return errorProjection("query.explain", err), err
	}
	projection := domain.NewProjection("query.explain", "Query plan parsed.")
	projection.Facts["source"] = string(ast.Source)
	projection.Facts["columns"] = searchops.QuerySelectColumns(ast.Select)
	projection.Facts["filters"] = fmt.Sprint(len(ast.Filters))
	projection.Facts["sorts"] = fmt.Sprint(len(ast.Sorts))
	projection.Facts["groups"] = fmt.Sprint(len(ast.Groups))
	projection.Facts["limit"] = fmt.Sprint(ast.Limit)
	projection.Data = map[string]any{"ast": ast, "warnings": []string{}}
	return projection, nil
}
