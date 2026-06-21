package searchops

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

type QueryRequest struct {
	Limit  int
	Sort   string
	Cursor string
}

func ExecuteQuery(notes []domain.Note, ast domain.QueryAST, req QueryRequest) domain.TableResult {
	rows := queryRowsForSource(notes, ast.Source)
	filtered := make([]domain.DatabaseRow, 0, len(rows))
	for _, row := range rows {
		if queryRowMatches(row, ast.Filters) {
			filtered = append(filtered, row)
		}
	}
	if queryUsesGrouping(ast) {
		filtered = aggregateQueryRows(filtered, ast)
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
		if !queryUsesGrouping(ast) {
			pageRows[i].Values = projectQueryValues(pageRows[i].Values, ast.Select)
		}
	}
	return domain.TableResult{Columns: QueryColumns(ast.Select), Rows: pageRows, Page: domain.QueryPage{Limit: limit, Cursor: req.Cursor, NextCursor: nextCursor, HasMore: hasMore}}
}

func queryRowsForSource(notes []domain.Note, source domain.QuerySource) []domain.DatabaseRow {
	switch source {
	case domain.QuerySourceTasks:
		return noteindex.ExtractTaskRows(notes)
	case domain.QuerySourceLinks:
		return noteindex.ExtractLinkRows(notes)
	case domain.QuerySourceBacklinks:
		return noteindex.ExtractBacklinkRows(notes)
	case domain.QuerySourceAssets:
		return noteindex.ExtractAssetRows(notes)
	default:
		return noteindex.ExtractPropertyRows(notes)
	}
}

func queryUsesGrouping(ast domain.QueryAST) bool {
	if len(ast.Groups) > 0 {
		return true
	}
	for _, selected := range ast.Select {
		if selected.Aggregate != "" {
			return true
		}
	}
	return false
}

func QueryColumns(selects []domain.QuerySelect) []string {
	cols := make([]string, 0, len(selects))
	for _, item := range selects {
		cols = append(cols, querySelectOutputName(item))
	}
	return cols
}

func querySelectOutputName(item domain.QuerySelect) string {
	if item.Alias != "" {
		return item.Alias
	}
	if item.Aggregate != "" {
		return strings.ToLower(string(item.Aggregate)) + "_" + strings.ReplaceAll(item.Property, "*", "all")
	}
	return item.Property
}

func QuerySelectColumns(selects []domain.QuerySelect) string {
	return strings.Join(QueryColumns(selects), ",")
}

func querySourceSupported(source domain.QuerySource) bool {
	switch source {
	case domain.QuerySourceNotes, domain.QuerySourceTasks, domain.QuerySourceLinks, domain.QuerySourceBacklinks, domain.QuerySourceAssets:
		return true
	default:
		return false
	}
}

func ParseSQL(sql string) (domain.QueryAST, error) {
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
	if !querySourceSupported(ast.Source) {
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
		if !ok && filter.Operator != domain.QueryOperatorIsEmpty {
			return false
		}
		want := fmt.Sprint(filter.Value)
		got := ""
		if ok {
			got = value.String()
		}
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
		case domain.QueryOperatorIn:
			values, _ := filter.Value.([]string)
			if !stringInSlice(got, values) {
				return false
			}
		case domain.QueryOperatorGT, domain.QueryOperatorGTE, domain.QueryOperatorLT, domain.QueryOperatorLTE:
			if !compareQueryValues(got, want, filter.Operator) {
				return false
			}
		case domain.QueryOperatorExists, domain.QueryOperatorIsNotEmpty:
			if strings.TrimSpace(got) == "" {
				return false
			}
		case domain.QueryOperatorIsEmpty:
			if strings.TrimSpace(got) != "" {
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

func compareQueryValues(got, want string, operator domain.QueryOperator) bool {
	left, leftErr := strconv.ParseFloat(got, 64)
	right, rightErr := strconv.ParseFloat(want, 64)
	if leftErr != nil || rightErr != nil {
		return compareStrings(got, want, operator)
	}
	switch operator {
	case domain.QueryOperatorGT:
		return left > right
	case domain.QueryOperatorGTE:
		return left >= right
	case domain.QueryOperatorLT:
		return left < right
	case domain.QueryOperatorLTE:
		return left <= right
	default:
		return false
	}
}

func compareStrings(got, want string, operator domain.QueryOperator) bool {
	switch operator {
	case domain.QueryOperatorGT:
		return got > want
	case domain.QueryOperatorGTE:
		return got >= want
	case domain.QueryOperatorLT:
		return got < want
	case domain.QueryOperatorLTE:
		return got <= want
	default:
		return false
	}
}

func stringInSlice(value string, values []string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
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

func aggregateQueryRows(rows []domain.DatabaseRow, ast domain.QueryAST) []domain.DatabaseRow {
	groups := map[string]*domain.DatabaseRow{}
	order := []string{}
	for _, row := range rows {
		key := queryGroupKey(row, ast.Groups)
		group, ok := groups[key]
		if !ok {
			group = &domain.DatabaseRow{Source: "group:" + key, Values: map[string]domain.PropertyValue{}}
			for _, name := range ast.Groups {
				if value, exists := row.Values[name]; exists {
					group.Values[name] = value
				}
			}
			for _, selected := range ast.Select {
				if selected.Aggregate == "" {
					if value, exists := row.Values[selected.Property]; exists {
						group.Values[querySelectOutputName(selected)] = renameQueryValue(value, querySelectOutputName(selected))
					}
				}
			}
			groups[key] = group
			order = append(order, key)
		}
		applyAggregateSelects(group.Values, row.Values, ast.Select)
	}
	out := make([]domain.DatabaseRow, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return out
}

func queryGroupKey(row domain.DatabaseRow, groups []string) string {
	if len(groups) == 0 {
		return "all"
	}
	parts := make([]string, 0, len(groups))
	for _, name := range groups {
		parts = append(parts, row.Values[name].String())
	}
	return strings.Join(parts, "\x00")
}

func applyAggregateSelects(groupValues, rowValues map[string]domain.PropertyValue, selects []domain.QuerySelect) {
	for _, selected := range selects {
		if selected.Aggregate == "" {
			continue
		}
		name := querySelectOutputName(selected)
		switch selected.Aggregate {
		case domain.QueryAggregateCount:
			current := queryIntValue(groupValues[name])
			groupValues[name] = domain.PropertyValue{Name: name, Type: domain.PropertyTypeNumber, Raw: strconv.Itoa(current + 1), Value: current + 1, Source: "aggregate"}
		case domain.QueryAggregateMin, domain.QueryAggregateMax:
			value, ok := rowValues[selected.Property]
			if !ok || value.String() == "" {
				continue
			}
			current, exists := groupValues[name]
			if !exists || aggregateValueWins(value, current, selected.Aggregate) {
				groupValues[name] = renameQueryValue(value, name)
				groupValues[name] = domain.PropertyValue{Name: name, Type: value.Type, Raw: value.String(), Value: value.Value, Source: "aggregate"}
			}
		}
	}
}

func aggregateValueWins(candidate, current domain.PropertyValue, aggregate domain.QueryAggregate) bool {
	operator := domain.QueryOperatorLT
	if aggregate == domain.QueryAggregateMax {
		operator = domain.QueryOperatorGT
	}
	return compareQueryValues(candidate.String(), current.String(), operator)
}

func queryIntValue(value domain.PropertyValue) int {
	if value.Value == nil && value.Raw == "" {
		return 0
	}
	if n, ok := value.Value.(int); ok {
		return n
	}
	n, _ := strconv.Atoi(value.String())
	return n
}

func renameQueryValue(value domain.PropertyValue, name string) domain.PropertyValue {
	value.Name = name
	return value
}

func parseSelectList(part string) []domain.QuerySelect {
	items := splitCSVRespectQuotes(part)
	selects := make([]domain.QuerySelect, 0, len(items))
	for _, item := range items {
		fields := strings.Fields(item)
		if len(fields) == 0 {
			continue
		}
		selectItem := parseSelectItem(fields[0])
		if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
			selectItem.Alias = fields[2]
		}
		selects = append(selects, selectItem)
	}
	return selects
}

func parseSelectItem(raw string) domain.QuerySelect {
	raw = strings.TrimSpace(raw)
	upper := strings.ToUpper(raw)
	for _, aggregate := range []domain.QueryAggregate{domain.QueryAggregateCount, domain.QueryAggregateMin, domain.QueryAggregateMax} {
		prefix := string(aggregate) + "("
		if strings.HasPrefix(upper, prefix) && strings.HasSuffix(raw, ")") {
			property := strings.TrimSpace(raw[len(prefix) : len(raw)-1])
			if property == "" {
				property = "*"
			}
			return domain.QuerySelect{Property: property, Aggregate: aggregate}
		}
	}
	return domain.QuerySelect{Property: raw}
}

func parseQueryTail(tail string, ast *domain.QueryAST) error {
	remaining := strings.TrimSpace(tail)
	if remaining == "" {
		return nil
	}
	upper := strings.ToUpper(remaining)
	wherePart, afterWhere := cutClause(remaining, upper, "WHERE", []string{"GROUP BY", "ORDER BY", "LIMIT"})
	if wherePart != "" {
		filters, err := parseWhereFilters(wherePart)
		if err != nil {
			return err
		}
		ast.Filters = filters
		remaining = strings.TrimSpace(afterWhere)
		upper = strings.ToUpper(remaining)
	}
	groupPart, afterGroup := cutClause(remaining, upper, "GROUP BY", []string{"ORDER BY", "LIMIT"})
	if groupPart != "" {
		ast.Groups = parseGroups(groupPart)
		remaining = strings.TrimSpace(afterGroup)
		upper = strings.ToUpper(remaining)
	}
	orderPart, afterOrder := cutClause(remaining, upper, "ORDER BY", []string{"LIMIT"})
	if orderPart != "" {
		ast.Sorts = parseSorts(orderPart)
		remaining = strings.TrimSpace(afterOrder)
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
		filter, err := parseWhereFilter(item)
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

func parseWhereFilter(item string) (domain.QueryFilter, error) {
	item = strings.TrimSpace(item)
	fields := strings.Fields(item)
	if len(fields) < 2 {
		return domain.QueryFilter{}, &domain.CommandError{Code: "sql_parse_failed", Message: "WHERE condition is incomplete: " + item, Hint: "For example, status = \"active\""}
	}
	if strings.EqualFold(fields[0], "EXISTS") {
		if len(fields) != 2 {
			return domain.QueryFilter{}, &domain.CommandError{Code: "sql_parse_failed", Message: "EXISTS condition accepts exactly one property", Hint: "For example, EXISTS due"}
		}
		return domain.QueryFilter{Property: fields[1], Operator: domain.QueryOperatorExists}, nil
	}
	if len(fields) >= 3 && strings.EqualFold(fields[1], "IS") {
		if len(fields) == 3 && strings.EqualFold(fields[2], "EMPTY") {
			return domain.QueryFilter{Property: fields[0], Operator: domain.QueryOperatorIsEmpty}, nil
		}
		if len(fields) == 4 && strings.EqualFold(fields[2], "NOT") && strings.EqualFold(fields[3], "EMPTY") {
			return domain.QueryFilter{Property: fields[0], Operator: domain.QueryOperatorIsNotEmpty}, nil
		}
		return domain.QueryFilter{}, &domain.CommandError{Code: "sql_parse_failed", Message: "IS condition is unsupported: " + item, Hint: "Use IS EMPTY or IS NOT EMPTY"}
	}
	if len(fields) < 3 {
		return domain.QueryFilter{}, &domain.CommandError{Code: "sql_parse_failed", Message: "WHERE condition is incomplete: " + item, Hint: "For example, status = \"active\""}
	}
	operator := domain.QueryOperator(strings.ToUpper(fields[1]))
	switch fields[1] {
	case "=", "!=", ">", ">=", "<", "<=":
		operator = domain.QueryOperator(fields[1])
	}
	if operator == domain.QueryOperatorIn {
		values, err := parseInValues(strings.Join(fields[2:], " "))
		if err != nil {
			return domain.QueryFilter{}, err
		}
		return domain.QueryFilter{Property: fields[0], Operator: operator, Value: values}, nil
	}
	if operator != domain.QueryOperatorEquals && operator != domain.QueryOperatorNotEqual && operator != domain.QueryOperatorContains && operator != domain.QueryOperatorLike && operator != domain.QueryOperatorGT && operator != domain.QueryOperatorGTE && operator != domain.QueryOperatorLT && operator != domain.QueryOperatorLTE {
		return domain.QueryFilter{}, &domain.CommandError{Code: "sql_unsupported_operator", Message: "WHERE operator is not supported: " + fields[1], Hint: "Use =, !=, >, >=, <, <=, IN, EXISTS, IS EMPTY, CONTAINS, or LIKE"}
	}
	return domain.QueryFilter{Property: fields[0], Operator: operator, Value: unquote(strings.Join(fields[2:], " "))}, nil
}

func parseInValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "(") || !strings.HasSuffix(raw, ")") {
		return nil, &domain.CommandError{Code: "sql_parse_failed", Message: "IN values must be enclosed in parentheses", Hint: "For example, status IN (\"active\", \"done\")"}
	}
	items := splitCSVRespectQuotes(strings.TrimSpace(raw[1 : len(raw)-1]))
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = unquote(item)
		if item != "" {
			values = append(values, item)
		}
	}
	if len(values) == 0 {
		return nil, &domain.CommandError{Code: "sql_parse_failed", Message: "IN values cannot be empty", Hint: "For example, status IN (\"active\", \"done\")"}
	}
	return values, nil
}

func projectQueryValues(values map[string]domain.PropertyValue, selects []domain.QuerySelect) map[string]domain.PropertyValue {
	if len(selects) == 0 {
		return values
	}
	projected := make(map[string]domain.PropertyValue, len(selects))
	for _, selected := range selects {
		name := querySelectOutputName(selected)
		if value, ok := values[selected.Property]; ok {
			projected[name] = renameQueryValue(value, name)
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
	_ = upper
	trimmed := strings.TrimSpace(text)
	start, ok := clauseContentStart(trimmed, clause)
	if !ok {
		return "", text
	}
	body := trimmed[start:]
	end := len(body)
	for _, n := range next {
		if idx := indexClauseKeyword(body, n); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(body[:end]), strings.TrimSpace(body[end:])
}

func clauseContentStart(text, clause string) (int, bool) {
	i := 0
	parts := strings.Fields(clause)
	for partIndex, part := range parts {
		if partIndex > 0 {
			if i >= len(text) || !isQueryWhitespace(text[i]) {
				return 0, false
			}
			for i < len(text) && isQueryWhitespace(text[i]) {
				i++
			}
		}
		if len(text[i:]) < len(part) || !strings.EqualFold(text[i:i+len(part)], part) {
			return 0, false
		}
		i += len(part)
	}
	if i >= len(text) || !isQueryWhitespace(text[i]) {
		return 0, false
	}
	for i < len(text) && isQueryWhitespace(text[i]) {
		i++
	}
	return i, true
}

func indexClauseKeyword(text, clause string) int {
	inQuote := false
	for i := 0; i < len(text); i++ {
		if text[i] == '"' {
			inQuote = !inQuote
		}
		if inQuote {
			continue
		}
		if i > 0 && !isQueryWhitespace(text[i-1]) {
			continue
		}
		if _, ok := clauseContentStart(text[i:], clause); ok {
			return i
		}
	}
	return -1
}

func isQueryWhitespace(value byte) bool {
	switch value {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func splitByAND(part string) []string {
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
