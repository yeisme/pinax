package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	name, nameErr := normalizePropertyKey(req.Name)
	if nameErr != nil {
		return domain.NewErrorProjection("database.schema.set", nameErr), nameErr
	}
	if name == "" {
		err := &domain.CommandError{Code: "property_required", Message: "database schema set requires a property name", Hint: "pinax database schema set status --type select --vault <vault>"}
		return domain.NewErrorProjection("database.schema.set", err), err
	}
	propertyType, typeErr := normalizeDatabasePropertyType(req.Type)
	if typeErr != nil {
		return domain.NewErrorProjection("database.schema.set", typeErr), typeErr
	}
	if propertyType == "" {
		err := &domain.CommandError{Code: "property_type_required", Message: "database schema set requires --type", Hint: "Use --type string|number|boolean|date|select|list|link"}
		return domain.NewErrorProjection("database.schema.set", err), err
	}
	values, tagErr := normalizeTagsForWrite(req.Values)
	if tagErr != nil {
		return domain.NewErrorProjection("database.schema.set", tagErr), tagErr
	}
	registry, err := loadPropertySchemaOverrides(root)
	if err != nil {
		return errorProjection("database.schema.set", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	registry.Properties[name] = domain.PropertySchemaOverride{Type: propertyType, Values: values, UpdatedAt: now}
	path := filepath.Join(root, ".pinax", "schema-overrides.json")
	if err := writeJSONAsset(path, registry); err != nil {
		return errorProjection("database.schema.set", err), err
	}
	validation := validatePropertySchemaAgainstVault(root, name, propertyType, values)
	_ = appendEvent(root, "database.schema.set", "success", map[string]string{"property": name, "type": string(propertyType)})
	projection := domain.NewProjection("database.schema.set", "Database property schema saved.")
	projection.Facts["property"] = name
	projection.Facts["type"] = string(propertyType)
	projection.Facts["path"] = filepath.ToSlash(filepath.Join(".pinax", "schema-overrides.json"))
	addPropertyValidationFacts(&projection, validation)
	projection.Evidence = []string{projection.Facts["path"]}
	projection.Data = map[string]any{"registry": registry, "property": registry.Properties[name], "validation": validation}
	return projection, nil
}

func (s *Service) DatabaseSchemaList(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.schema.list", err), err
	}
	registry, err := loadPropertySchemaOverrides(root)
	if err != nil {
		return errorProjection("database.schema.list", err), err
	}
	names := make([]string, 0, len(registry.Properties))
	for name := range registry.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	properties := make([]map[string]any, 0, len(names))
	for _, name := range names {
		property := registry.Properties[name]
		properties = append(properties, map[string]any{"name": name, "type": property.Type, "values": property.Values, "updated_at": property.UpdatedAt})
	}
	projection := domain.NewProjection("database.schema.list", "Database property schema listed.")
	projection.Facts["properties"] = fmt.Sprint(len(properties))
	projection.Facts["schema_version"] = registry.SchemaVersion
	projection.Data = map[string]any{"properties": properties, "registry": registry}
	return projection, nil
}

func (s *Service) DatabaseSchemaShow(_ context.Context, req DatabaseSchemaRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("database.schema.show", err), err
	}
	name, nameErr := normalizePropertyKey(req.Name)
	if nameErr != nil {
		return domain.NewErrorProjection("database.schema.show", nameErr), nameErr
	}
	registry, err := loadPropertySchemaOverrides(root)
	if err != nil {
		return errorProjection("database.schema.show", err), err
	}
	property, ok := registry.Properties[name]
	if !ok {
		err := &domain.CommandError{Code: "property_schema_not_found", Message: "Property schema override not found", Hint: "Run pinax database schema list --vault <vault>"}
		return domain.NewErrorProjection("database.schema.show", err), err
	}
	validation := validatePropertySchemaAgainstVault(root, name, property.Type, property.Values)
	projection := domain.NewProjection("database.schema.show", "Database property schema read.")
	projection.Facts["property"] = name
	projection.Facts["type"] = string(property.Type)
	projection.Facts["schema_version"] = registry.SchemaVersion
	projection.Facts["values"] = strings.Join(property.Values, ",")
	addPropertyValidationFacts(&projection, validation)
	projection.Data = map[string]any{"property": property, "validation": validation}
	return projection, nil
}

type propertySchemaValidation struct {
	Status        string   `json:"status"`
	CheckedValues int      `json:"checked_values"`
	InvalidValues int      `json:"invalid_values"`
	Warnings      []string `json:"warnings,omitempty"`
}

func loadPropertySchemaOverrides(root string) (domain.PropertySchemaOverrideRegistry, error) {
	registry := domain.PropertySchemaOverrideRegistry{SchemaVersion: domain.PropertySchemaOverridesVersion, Properties: map[string]domain.PropertySchemaOverride{}}
	path := filepath.Join(root, ".pinax", "schema-overrides.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return registry, err
	}
	if err := json.Unmarshal(payload, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = domain.PropertySchemaOverridesVersion
	}
	if registry.Properties == nil {
		registry.Properties = map[string]domain.PropertySchemaOverride{}
	}
	return registry, nil
}

func normalizeDatabasePropertyType(raw string) (domain.PropertyType, *domain.CommandError) {
	typeName := strings.TrimSpace(raw)
	if typeName == "" {
		return "", nil
	}
	allowed := map[string]domain.PropertyType{
		"text":         domain.PropertyTypeText,
		"string":       domain.PropertyTypeString,
		"number":       domain.PropertyTypeNumber,
		"checkbox":     domain.PropertyTypeCheckbox,
		"boolean":      domain.PropertyTypeBoolean,
		"date":         domain.PropertyTypeDate,
		"select":       domain.PropertyTypeSelect,
		"multi_select": domain.PropertyTypeMultiSelect,
		"list":         domain.PropertyTypeList,
		"url":          domain.PropertyTypeURL,
		"email":        domain.PropertyTypeEmail,
		"person_text":  domain.PropertyTypePersonText,
		"relation":     domain.PropertyTypeRelation,
		"link":         domain.PropertyTypeLink,
		"rollup":       domain.PropertyTypeRollup,
		"formula":      domain.PropertyTypeFormula,
	}
	if typ, ok := allowed[typeName]; ok {
		return typ, nil
	}
	return "", &domain.CommandError{Code: "property_type_unsupported", Message: "Unsupported database property type", Hint: "Use text, number, checkbox, date, select, multi_select, url, email, person_text, relation, rollup, or formula"}
}

func validatePropertySchemaAgainstVault(root, name string, typ domain.PropertyType, values []string) propertySchemaValidation {
	validation := propertySchemaValidation{Status: "ok"}
	notes, err := scanNotes(root)
	if err != nil {
		validation.Status = "warnings"
		validation.Warnings = append(validation.Warnings, "scan_failed")
		return validation
	}
	allowed := map[string]bool{}
	for _, value := range values {
		allowed[value] = true
	}
	for _, row := range noteindex.ExtractPropertyRows(ordinaryNotes(notes)) {
		value, ok := row.Values[name]
		if !ok || strings.TrimSpace(value.String()) == "" {
			continue
		}
		validation.CheckedValues++
		if !propertyValueMatchesType(value.String(), typ, allowed) {
			validation.InvalidValues++
		}
	}
	if validation.InvalidValues > 0 {
		validation.Status = "warnings"
		validation.Warnings = append(validation.Warnings, "property_value_type_mismatch")
	}
	return validation
}

func propertyValueMatchesType(raw string, typ domain.PropertyType, allowed map[string]bool) bool {
	value := strings.TrimSpace(raw)
	switch typ {
	case domain.PropertyTypeText, domain.PropertyTypeString, domain.PropertyTypePersonText, domain.PropertyTypeRelation, domain.PropertyTypeLink, domain.PropertyTypeRollup, domain.PropertyTypeFormula:
		return true
	case domain.PropertyTypeNumber:
		_, err := strconv.ParseFloat(value, 64)
		return err == nil
	case domain.PropertyTypeCheckbox, domain.PropertyTypeBoolean:
		_, err := strconv.ParseBool(strings.ToLower(value))
		return err == nil
	case domain.PropertyTypeDate:
		if _, err := time.Parse("2006-01-02", value); err == nil {
			return true
		}
		_, err := time.Parse(time.RFC3339, value)
		return err == nil
	case domain.PropertyTypeSelect:
		return len(allowed) == 0 || allowed[value]
	case domain.PropertyTypeMultiSelect, domain.PropertyTypeList:
		if len(allowed) == 0 {
			return true
		}
		for _, part := range strings.Split(value, ",") {
			if item := strings.TrimSpace(part); item != "" && !allowed[item] {
				return false
			}
		}
		return true
	case domain.PropertyTypeURL:
		parsed, err := url.Parse(value)
		return err == nil && parsed.Scheme != "" && parsed.Host != ""
	case domain.PropertyTypeEmail:
		_, err := mail.ParseAddress(value)
		return err == nil
	default:
		return true
	}
}

func addPropertyValidationFacts(projection *domain.Projection, validation propertySchemaValidation) {
	projection.Facts["validation_status"] = validation.Status
	projection.Facts["checked_values"] = fmt.Sprint(validation.CheckedValues)
	projection.Facts["invalid_values"] = fmt.Sprint(validation.InvalidValues)
	projection.Facts["warnings"] = fmt.Sprint(len(validation.Warnings))
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
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "sql"
	}
	if language != "sql" && language != "dataview" {
		err := &domain.CommandError{Code: "view_language_unsupported", Message: "database view language is unsupported", Hint: "Use --language sql or --language dataview"}
		return domain.NewErrorProjection("database.view.save", err), err
	}
	registry.SchemaVersion = "pinax.views.v3"
	display := strings.TrimSpace(req.Display)
	if display == "" {
		display = strings.TrimSpace(req.Kind)
	}
	if display == "" {
		display = "table"
	}
	if !isSupportedDatabaseViewDisplay(display) {
		err := &domain.CommandError{Code: "database_view_display_unsupported", Message: "Database view display is unsupported", Hint: "Use --display table|board|list|calendar"}
		return domain.NewErrorProjection("database.view.save", err), err
	}
	viewName := strings.TrimSpace(req.Name)
	view := domain.SavedView{ID: stableViewID(req.Name), Name: viewName, Kind: display, Language: language, Query: strings.TrimSpace(req.Query), Columns: req.Columns, GroupBy: strings.TrimSpace(req.GroupBy), CalendarField: strings.TrimSpace(req.CalendarField), BoardColumn: strings.TrimSpace(req.BoardColumn), Display: map[string]string{"mode": display, "tab": viewName}, Limit: req.Limit, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	upsertSavedView(&registry, view)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("database.view.save", err), err
	}
	projection := domain.NewProjection("database.view.save", "Database view saved.")
	projection.Facts["view"] = view.Name
	projection.Facts["schema_version"] = registry.SchemaVersion
	projection.Facts["language"] = view.Language
	projection.Facts["display"] = view.Kind
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
	var projection domain.Projection
	if view.Language == "dataview" {
		projection, err = s.DataviewRun(ctx, DataviewRequest{VaultPath: root, Query: view.Query, Limit: view.Limit, LazyIndex: true})
	} else {
		projection, err = s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: view.Query, Limit: view.Limit, LazyIndex: true})
	}
	projection.Command = "database.view.show"
	projection.Summary = "Database view queried."
	projection.Facts["view"] = view.Name
	projection.Data = map[string]any{"view": view, "result": projection.Data}
	return projection, err
}

func (s *Service) RenderDatabaseView(ctx context.Context, req ViewRequest) (domain.Projection, error) {
	projection, err := s.ShowDatabaseView(ctx, req)
	if err != nil {
		projection.Command = "database.view.render"
		return projection, err
	}
	projection.Command = "database.view.render"
	projection.Summary = "Database view rendered."
	data, _ := projection.Data.(map[string]any)
	view, _ := data["view"].(domain.SavedView)
	result, ok := tableResultFromDatabaseViewData(data)
	if !ok {
		err := &domain.CommandError{Code: "database_view_result_unavailable", Message: "Database view result is unavailable", Hint: "Run pinax database view show <name> --vault <vault> --json"}
		return domain.NewErrorProjection("database.view.render", err), err
	}
	display := strings.TrimSpace(view.Kind)
	if display == "" {
		display = "table"
	}
	render, renderErr := buildDatabaseViewRender(view, result, display)
	if renderErr != nil {
		errorProjection := domain.NewErrorProjection("database.view.render", renderErr)
		errorProjection.Facts["view"] = view.Name
		errorProjection.Facts["display"] = display
		errorProjection.Actions = []domain.Action{{Name: "edit_view", Command: fmt.Sprintf("pinax database view save %s --display %s --calendar-field <date-property> --vault %s --json", shellQuote(view.Name), shellQuote(display), shellQuote(req.VaultPath))}}
		return errorProjection, renderErr
	}
	projection.Facts["view"] = view.Name
	projection.Facts["display"] = display
	projection.Facts["rows"] = fmt.Sprint(render.RowCount)
	projection.Facts["database.view"] = view.Name
	projection.Facts["database.display"] = display
	projection.Facts["database.rows"] = fmt.Sprint(render.RowCount)
	projection.Facts["database_tab.name"] = view.Name
	projection.Facts["database_tab.view"] = view.Name
	projection.Facts["database_tab.display"] = display
	if render.Table != nil {
		projection.Facts["columns"] = strings.Join(render.Table.Columns, ",")
	}
	if len(render.Board.Columns) > 0 {
		projection.Facts["board_columns"] = fmt.Sprint(len(render.Board.Columns))
		projection.Facts["board_column"] = render.Board.GroupBy
	}
	if len(render.Calendar.Events) > 0 || render.Calendar.DateField != "" {
		projection.Facts["calendar_events"] = fmt.Sprint(len(render.Calendar.Events))
		projection.Facts["calendar_field"] = render.Calendar.DateField
	}
	tab := domain.DatabaseTab{Name: view.Name, View: view.Name, Display: display, Rows: render.RowCount, Render: render, Facts: projection.Facts}
	projection.Data = map[string]any{"view": view, "render": render, "database_view": view, "database_tab": tab}
	return projection, nil
}

func isSupportedDatabaseViewDisplay(display string) bool {
	switch display {
	case "table", "board", "list", "calendar":
		return true
	default:
		return false
	}
}

func tableResultFromDatabaseViewData(data map[string]any) (domain.TableResult, bool) {
	resultData, _ := data["result"].(map[string]any)
	if result, ok := resultData["result"].(domain.TableResult); ok {
		return result, true
	}
	if nested, ok := resultData["result"].(map[string]any); ok {
		if result, ok := nested["result"].(domain.TableResult); ok {
			return result, true
		}
	}
	return domain.TableResult{}, false
}

func buildDatabaseViewRender(view domain.SavedView, result domain.TableResult, display string) (domain.DatabaseViewRender, *domain.CommandError) {
	render := domain.DatabaseViewRender{Display: display, RowCount: result.RowCount()}
	switch display {
	case "table":
		render.Table = &result
	case "list":
		render.List = databaseViewListItems(result.Rows)
	case "board":
		groupBy := strings.TrimSpace(firstBoardNonEmpty(view.BoardColumn, view.GroupBy))
		if groupBy == "" {
			groupBy = "status"
		}
		render.Board = buildDatabaseViewBoardRender(result.Rows, groupBy)
	case "calendar":
		dateField := strings.TrimSpace(view.CalendarField)
		if dateField == "" {
			return render, &domain.CommandError{Code: "calendar_field_required", Message: "Calendar database view requires a date property", Hint: "Save the view with --calendar-field <date-property>"}
		}
		render.Calendar = buildDatabaseViewCalendarRender(result.Rows, dateField)
	default:
		return render, &domain.CommandError{Code: "database_view_display_unsupported", Message: "Database view display is unsupported", Hint: "Use table, board, list, or calendar"}
	}
	return render, nil
}

func databaseViewListItems(rows []domain.DatabaseRow) []domain.DatabaseViewListItem {
	items := make([]domain.DatabaseViewListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.DatabaseViewListItem{Title: databaseViewRowTitle(row), Path: row.Note.Path, Values: databaseViewRowValues(row)})
	}
	return items
}

func buildDatabaseViewBoardRender(rows []domain.DatabaseRow, groupBy string) domain.DatabaseViewBoardRender {
	groups := map[string][]domain.DatabaseViewListItem{}
	for _, row := range rows {
		key := databaseViewRowValue(row, groupBy)
		if key == "" {
			key = "empty"
		}
		groups[key] = append(groups[key], domain.DatabaseViewListItem{Title: databaseViewRowTitle(row), Path: row.Note.Path, Values: databaseViewRowValues(row)})
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	columns := make([]domain.DatabaseViewBoardColumn, 0, len(keys))
	for _, key := range keys {
		columns = append(columns, domain.DatabaseViewBoardColumn{ID: key, Count: len(groups[key]), Items: groups[key]})
	}
	return domain.DatabaseViewBoardRender{GroupBy: groupBy, Columns: columns}
}

func buildDatabaseViewCalendarRender(rows []domain.DatabaseRow, dateField string) domain.DatabaseViewCalendarRender {
	events := []domain.DatabaseViewCalendarEvent{}
	for _, row := range rows {
		date := databaseViewRowValue(row, dateField)
		if date == "" {
			continue
		}
		if len(date) > len("2006-01-02") {
			date = date[:len("2006-01-02")]
		}
		events = append(events, domain.DatabaseViewCalendarEvent{Date: date, Title: databaseViewRowTitle(row), Path: row.Note.Path, Status: databaseViewRowValue(row, "status")})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Date != events[j].Date {
			return events[i].Date < events[j].Date
		}
		return events[i].Title < events[j].Title
	})
	return domain.DatabaseViewCalendarRender{DateField: dateField, Events: events}
}

func databaseViewRowTitle(row domain.DatabaseRow) string {
	if title := databaseViewRowValue(row, "title"); title != "" {
		return title
	}
	return row.Note.Title
}

func databaseViewRowValue(row domain.DatabaseRow, name string) string {
	if value, ok := row.Values[name]; ok {
		return value.String()
	}
	switch name {
	case "title":
		return row.Note.Title
	case "path":
		return row.Note.Path
	case "status":
		return row.Note.Status
	}
	return ""
}

func databaseViewRowValues(row domain.DatabaseRow) map[string]string {
	values := map[string]string{}
	for name, value := range row.Values {
		values[name] = value.String()
	}
	if row.Note.Title != "" {
		values["title"] = row.Note.Title
	}
	if row.Note.Path != "" {
		values["path"] = row.Note.Path
	}
	if row.Note.Status != "" {
		values["status"] = row.Note.Status
	}
	return values
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
	ast, err := searchops.ParseSQL(req.SQL)
	if err != nil {
		return errorProjection("query.run", err), err
	}
	return s.runQueryAST(ctx, "query.run", "Query executed.", root, req.LazyIndex, req.Limit, req.Sort, req.Cursor, ast)
}

func (s *Service) DataviewRun(ctx context.Context, req DataviewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("dataview.run", err), err
	}
	ast, err := searchops.ParseDataview(req.Query)
	if err != nil {
		return errorProjection("dataview.run", err), err
	}
	return s.runQueryAST(ctx, "dataview.run", "Dataview query executed.", root, req.LazyIndex, req.Limit, req.Sort, req.Cursor, ast)
}

func (s *Service) runQueryAST(ctx context.Context, command, summary, root string, lazyIndex bool, limit int, sort string, cursor string, ast domain.QueryAST) (domain.Projection, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	status, _ := noteindex.Inspect(root, notes)
	indexLoaded := ""
	if status.Status != "fresh" {
		if !lazyIndex {
			code := "property_index_stale"
			message := "Query index is not fresh"
			if status.Status == "missing" {
				code = "index_required"
				message = "Query requires building the local index first"
			}
			err := &domain.CommandError{Code: code, Message: message, Hint: "Run pinax index rebuild --vault " + shellQuote(root) + ", or add --lazy-index explicitly"}
			return domain.NewErrorProjection(command, err), err
		}
		select {
		case <-ctx.Done():
			return errorProjection(command, ctx.Err()), ctx.Err()
		default:
		}
		if _, err := noteindex.Rebuild(root, notes); err != nil {
			return errorProjection(command, err), err
		}
		status, _ = noteindex.Inspect(root, notes)
		indexLoaded = "lazy_rebuild"
	}
	result := searchops.ExecuteQuery(notes, ast, searchops.QueryRequest{Limit: limit, Sort: sort, Cursor: cursor})
	projection := domain.NewProjection(command, summary)
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
	return queryExplainProjection("query.explain", "Query plan parsed.", ast), nil
}

func (s *Service) DataviewExplain(_ context.Context, req DataviewRequest) (domain.Projection, error) {
	ast, err := searchops.ParseDataview(req.Query)
	if err != nil {
		return errorProjection("dataview.explain", err), err
	}
	return queryExplainProjection("dataview.explain", "Dataview query plan parsed.", ast), nil
}

func queryExplainProjection(command, summary string, ast domain.QueryAST) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["source"] = string(ast.Source)
	projection.Facts["columns"] = searchops.QuerySelectColumns(ast.Select)
	projection.Facts["filters"] = fmt.Sprint(len(ast.Filters))
	projection.Facts["sorts"] = fmt.Sprint(len(ast.Sorts))
	projection.Facts["groups"] = fmt.Sprint(len(ast.Groups))
	projection.Facts["limit"] = fmt.Sprint(ast.Limit)
	projection.Data = map[string]any{"ast": ast, "warnings": []string{}}
	return projection
}
