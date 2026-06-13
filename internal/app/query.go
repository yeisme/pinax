package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
	ast, err := parsePinaxSQL(req.SQL)
	if err != nil {
		return errorProjection("query.run", err), err
	}
	result := executeQueryAST(notes, ast, req)
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

func executeQueryAST(notes []domain.Note, ast domain.QueryAST, req QueryRequest) domain.TableResult {
	rows := noteindex.ExtractPropertyRows(notes)
	filtered := make([]domain.DatabaseRow, 0, len(rows))
	for _, row := range rows {
		if queryRowMatches(row, ast.Filters) {
			filtered = append(filtered, row)
		}
	}
	sorts := ast.Sorts
	if req.Sort != "" {
		sorts = []domain.QuerySort{{Property: req.Sort, Direction: domain.SortAsc}}
	}
	applyQuerySort(filtered, sorts)
	limit := ast.Limit
	if req.Limit > 0 && (limit == 0 || req.Limit < limit) {
		limit = req.Limit
	}
	if limit == 0 {
		limit = 50
	}
	offset := decodeQueryCursor(req.Cursor)
	if offset > len(filtered) {
		offset = len(filtered)
	}
	pageRows := filtered[offset:]
	hasMore := false
	nextCursor := ""
	if len(pageRows) > limit {
		hasMore = true
		pageRows = pageRows[:limit]
		nextCursor = encodeQueryCursor(offset + limit)
	}
	for i := range pageRows {
		pageRows[i].Note.Body = ""
		pageRows[i].Values = projectQueryValues(pageRows[i].Values, ast.Select)
	}
	return domain.TableResult{Columns: queryColumns(ast.Select), Rows: pageRows, Page: domain.QueryPage{Limit: limit, Cursor: req.Cursor, NextCursor: nextCursor, HasMore: hasMore}}
}

func encodeQueryCursor(offset int) string {
	if offset <= 0 {
		return ""
	}
	return fmt.Sprintf("offset:%d", offset)
}

func decodeQueryCursor(cursor string) int {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0
	}
	value := strings.TrimPrefix(cursor, "offset:")
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func queryRowMatches(row domain.DatabaseRow, filters []domain.QueryFilter) bool {
	for _, filter := range filters {
		value, ok := row.Values[filter.Property]
		if !ok {
			return false
		}
		want := fmt.Sprint(filter.Value)
		got := value.String()
		switch filter.Operator {
		case domain.QueryOperatorEquals:
			if got != want {
				return false
			}
		case domain.QueryOperatorNotEqual:
			if got == want {
				return false
			}
		case domain.QueryOperatorContains:
			if !strings.Contains(got, want) {
				return false
			}
		case domain.QueryOperatorLike:
			if !strings.Contains(strings.ToLower(got), strings.ToLower(strings.Trim(want, "%"))) {
				return false
			}
		default:
			if got != want {
				return false
			}
		}
	}
	return true
}

func applyQuerySort(rows []domain.DatabaseRow, sorts []domain.QuerySort) {
	if len(sorts) == 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for _, s := range sorts {
			left := rows[i].Values[s.Property].String()
			right := rows[j].Values[s.Property].String()
			if left == right {
				continue
			}
			if s.Direction == domain.SortDesc {
				return left > right
			}
			return left < right
		}
		return rows[i].Identity() < rows[j].Identity()
	})
}

func queryColumns(selects []domain.QuerySelect) []string {
	cols := make([]string, 0, len(selects))
	for _, item := range selects {
		if item.Alias != "" {
			cols = append(cols, item.Alias)
			continue
		}
		cols = append(cols, item.Property)
	}
	return cols
}

func (s *Service) QueryExplain(_ context.Context, req QueryRequest) (domain.Projection, error) {
	ast, err := parsePinaxSQL(req.SQL)
	if err != nil {
		return errorProjection("query.explain", err), err
	}
	projection := domain.NewProjection("query.explain", "Query plan parsed.")
	projection.Facts["source"] = string(ast.Source)
	projection.Facts["columns"] = querySelectColumns(ast.Select)
	projection.Facts["filters"] = fmt.Sprint(len(ast.Filters))
	projection.Facts["sorts"] = fmt.Sprint(len(ast.Sorts))
	projection.Facts["groups"] = fmt.Sprint(len(ast.Groups))
	projection.Facts["limit"] = fmt.Sprint(ast.Limit)
	projection.Data = map[string]any{"ast": ast, "warnings": []string{}}
	return projection, nil
}

func querySelectColumns(selects []domain.QuerySelect) string {
	cols := make([]string, 0, len(selects))
	for _, item := range selects {
		if item.Alias != "" {
			cols = append(cols, item.Alias)
			continue
		}
		cols = append(cols, item.Property)
	}
	return strings.Join(cols, ",")
}

func parsePinaxSQL(sql string) (domain.QueryAST, error) {
	text := strings.TrimSpace(sql)
	upper := strings.ToUpper(text)
	for _, forbidden := range []string{" ENV(", " EXEC(", " READFILE(", " HTTP(", " FETCH("} {
		if strings.Contains(" "+upper, forbidden) {
			return domain.QueryAST{}, &domain.CommandError{Code: "sql_forbidden_function", Message: "Pinax SQL does not allow function calls: " + strings.TrimSpace(forbidden), Hint: "Use only the safe SELECT/FROM/WHERE/ORDER BY/LIMIT subset"}
		}
	}
	for _, unsupported := range []string{" JOIN ", " UNION ", " INSERT ", " UPDATE ", " DELETE ", " DROP ", " ALTER ", " TABLE "} {
		if strings.Contains(" "+upper+" ", unsupported) {
			return domain.QueryAST{}, &domain.CommandError{Code: "sql_unsupported_clause", Message: "Pinax SQL does not yet support clause: " + strings.TrimSpace(unsupported), Hint: "Use SELECT ... FROM notes WHERE ... ORDER BY ... LIMIT ..."}
		}
	}
	if !strings.HasPrefix(upper, "SELECT ") {
		return domain.QueryAST{}, &domain.CommandError{Code: "sql_parse_failed", Message: "Pinax SQL must start with SELECT", Hint: "For example, SELECT title FROM notes LIMIT 20"}
	}
	fromIdx := indexKeyword(upper, " FROM ")
	if fromIdx < 0 {
		return domain.QueryAST{}, &domain.CommandError{Code: "sql_parse_failed", Message: "Pinax SQL is missing FROM", Hint: "For example, SELECT title FROM notes LIMIT 20"}
	}
	selectPart := strings.TrimSpace(text[len("SELECT "):fromIdx])
	rest := strings.TrimSpace(text[fromIdx+len(" FROM "):])
	source, tail := splitFirst(rest)
	ast := domain.QueryAST{Select: parseSelectList(selectPart), Source: domain.QuerySource(strings.ToLower(source))}
	if ast.Source != domain.QuerySourceNotes && ast.Source != domain.QuerySourceTasks {
		return domain.QueryAST{}, &domain.CommandError{Code: "sql_unsupported_source", Message: "Pinax SQL does not yet support source: " + source, Hint: "Currently supported: FROM notes or FROM tasks"}
	}
	if len(ast.Select) == 0 {
		return domain.QueryAST{}, &domain.CommandError{Code: "sql_parse_failed", Message: "SELECT fields cannot be empty", Hint: "For example, SELECT title,status FROM notes"}
	}
	if err := parseQueryTail(tail, &ast); err != nil {
		return domain.QueryAST{}, err
	}
	return ast, nil
}

func parseSelectList(part string) []domain.QuerySelect {
	items := splitCSVRespectQuotes(part)
	selects := make([]domain.QuerySelect, 0, len(items))
	for _, item := range items {
		fields := strings.Fields(item)
		if len(fields) == 0 {
			continue
		}
		selectItem := domain.QuerySelect{Property: strings.TrimSpace(fields[0])}
		if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
			selectItem.Alias = fields[2]
		}
		selects = append(selects, selectItem)
	}
	return selects
}

func parseQueryTail(tail string, ast *domain.QueryAST) error {
	remaining := strings.TrimSpace(tail)
	if remaining == "" {
		return nil
	}
	upper := strings.ToUpper(remaining)
	wherePart, afterWhere := cutClause(remaining, upper, "WHERE", []string{"ORDER BY", "GROUP BY", "LIMIT"})
	if wherePart != "" {
		filters, err := parseWhereFilters(wherePart)
		if err != nil {
			return err
		}
		ast.Filters = filters
		remaining = strings.TrimSpace(afterWhere)
		upper = strings.ToUpper(remaining)
	}
	orderPart, afterOrder := cutClause(remaining, upper, "ORDER BY", []string{"GROUP BY", "LIMIT"})
	if orderPart != "" {
		ast.Sorts = parseSorts(orderPart)
		remaining = strings.TrimSpace(afterOrder)
		upper = strings.ToUpper(remaining)
	}
	groupPart, afterGroup := cutClause(remaining, upper, "GROUP BY", []string{"LIMIT"})
	if groupPart != "" {
		ast.Groups = parseGroups(groupPart)
		remaining = strings.TrimSpace(afterGroup)
		upper = strings.ToUpper(remaining)
	}
	limitPart, afterLimit := cutClause(remaining, upper, "LIMIT", nil)
	if limitPart != "" {
		limit, err := strconv.Atoi(strings.Fields(limitPart)[0])
		if err != nil || limit < 0 {
			return &domain.CommandError{Code: "sql_parse_failed", Message: "LIMIT must be a non-negative integer", Hint: "For example, LIMIT 20"}
		}
		ast.Limit = limit
		remaining = strings.TrimSpace(afterLimit)
	}
	if strings.TrimSpace(remaining) != "" {
		return &domain.CommandError{Code: "sql_unsupported_clause", Message: "Pinax SQL has unsupported trailing syntax: " + remaining, Hint: "Use SELECT ... FROM notes WHERE ... ORDER BY ... LIMIT ..."}
	}
	return nil
}

func parseWhereFilters(part string) ([]domain.QueryFilter, error) {
	parts := splitByAND(part)
	filters := make([]domain.QueryFilter, 0, len(parts))
	for _, item := range parts {
		fields := strings.Fields(item)
		if len(fields) < 3 {
			return nil, &domain.CommandError{Code: "sql_parse_failed", Message: "WHERE condition is incomplete: " + item, Hint: "For example, status = \"active\""}
		}
		operator := domain.QueryOperator(strings.ToUpper(fields[1]))
		switch fields[1] {
		case "=", "!=":
			operator = domain.QueryOperator(fields[1])
		case ">", ">=", "<", "<=":
			return nil, &domain.CommandError{Code: "sql_unsupported_operator", Message: "WHERE operator is not supported: " + fields[1], Hint: "Use =, !=, CONTAINS, or LIKE"}
		}
		filters = append(filters, domain.QueryFilter{Property: fields[0], Operator: operator, Value: unquote(strings.Join(fields[2:], " "))})
	}
	return filters, nil
}

func projectQueryValues(values map[string]domain.PropertyValue, selects []domain.QuerySelect) map[string]domain.PropertyValue {
	if len(selects) == 0 {
		return values
	}
	projected := make(map[string]domain.PropertyValue, len(selects))
	for _, selected := range selects {
		if value, ok := values[selected.Property]; ok {
			projected[selected.Property] = value
		}
	}
	return projected
}

func parseSorts(part string) []domain.QuerySort {
	items := splitCSVRespectQuotes(part)
	out := make([]domain.QuerySort, 0, len(items))
	for _, item := range items {
		fields := strings.Fields(item)
		if len(fields) == 0 {
			continue
		}
		direction := domain.SortAsc
		if len(fields) > 1 && strings.EqualFold(fields[1], "DESC") {
			direction = domain.SortDesc
		}
		out = append(out, domain.QuerySort{Property: fields[0], Direction: direction})
	}
	return out
}

func parseGroups(part string) []string {
	items := splitCSVRespectQuotes(part)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func cutClause(text, upper, clause string, next []string) (string, string) {
	prefix := clause + " "
	if !strings.HasPrefix(strings.TrimSpace(upper), prefix) {
		return "", text
	}
	trimmed := strings.TrimSpace(text)[len(prefix):]
	trimmedUpper := strings.ToUpper(trimmed)
	end := len(trimmed)
	for _, n := range next {
		if idx := indexKeyword(trimmedUpper, " "+n+" "); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(trimmed[:end]), strings.TrimSpace(trimmed[end:])
}

func splitByAND(part string) []string {
	// MVP parser only supports AND as a top-level conjunction; OR is rejected later by semantic expansion work.
	return splitKeywordRespectQuotes(part, " AND ")
}

func splitKeywordRespectQuotes(value, keyword string) []string {
	out := []string{}
	start := 0
	inQuote := false
	upper := strings.ToUpper(value)
	for i := 0; i < len(value); i++ {
		if value[i] == '"' {
			inQuote = !inQuote
		}
		if !inQuote && strings.HasPrefix(upper[i:], keyword) {
			out = append(out, strings.TrimSpace(value[start:i]))
			start = i + len(keyword)
		}
	}
	out = append(out, strings.TrimSpace(value[start:]))
	return out
}

func splitCSVRespectQuotes(value string) []string {
	out := []string{}
	start := 0
	inQuote := false
	for i := 0; i < len(value); i++ {
		if value[i] == '"' {
			inQuote = !inQuote
		}
		if !inQuote && value[i] == ',' {
			out = append(out, strings.TrimSpace(value[start:i]))
			start = i + 1
		}
	}
	out = append(out, strings.TrimSpace(value[start:]))
	return out
}

func indexKeyword(upper, keyword string) int {
	return strings.Index(upper, keyword)
}

func splitFirst(value string) (string, string) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.TrimPrefix(value, fields[0]))
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, "\"")
}
