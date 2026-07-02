package searchops

import (
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// ParseDataview supports the safe Dataview subset Pinax can lower to QueryAST.
func ParseDataview(query string) (domain.QueryAST, error) {
	text := strings.TrimSpace(query)
	upper := strings.ToUpper(text)
	if strings.HasPrefix(upper, "DATAVIEWJS") || strings.HasPrefix(upper, "```DATAVIEWJS") {
		return domain.QueryAST{}, &domain.CommandError{Code: "dataview_unsupported_clause", Message: "DataviewJS is not supported", Hint: "Use TABLE, LIST, or TASK Dataview queries"}
	}
	for _, forbidden := range []string{" ENV(", " EXEC(", " READFILE(", " HTTP(", " FETCH("} {
		if strings.Contains(" "+upper, forbidden) {
			return domain.QueryAST{}, &domain.CommandError{Code: "dataview_forbidden_function", Message: "Dataview query contains a forbidden function", Hint: "Use only the safe TABLE/LIST/TASK subset"}
		}
	}
	for _, unsupported := range []string{" FLATTEN ", " JOIN ", " UNION ", " MAP(", " REDUCE("} {
		if strings.Contains(" "+upper+" ", unsupported) {
			return domain.QueryAST{}, &domain.CommandError{Code: "dataview_unsupported_clause", Message: "Dataview query contains unsupported syntax: " + strings.TrimSpace(unsupported), Hint: "Use TABLE, LIST, or TASK with FROM, WHERE, SORT, GROUP BY, and LIMIT"}
		}
	}

	mode, body, ok := dataviewMode(text)
	if !ok {
		return domain.QueryAST{}, &domain.CommandError{Code: "dataview_parse_failed", Message: "Dataview query must start with TABLE, LIST, or TASK", Hint: "For example, TABLE title FROM #pinax LIMIT 5"}
	}
	fromIdx, fromBodyIdx := dataviewFromBounds(body)
	if fromIdx < 0 {
		return domain.QueryAST{}, &domain.CommandError{Code: "dataview_parse_failed", Message: "Dataview query is missing FROM", Hint: "Use FROM #tag or FROM \"folder\""}
	}
	selectPart := strings.TrimSpace(body[:fromIdx])
	rest := strings.TrimSpace(body[fromBodyIdx:])
	fromPart, tail := splitFirst(rest)
	ast := domain.QueryAST{Source: domain.QuerySourceNotes, Select: dataviewSelects(mode, selectPart)}
	if mode == "TASK" {
		ast.Source = domain.QuerySourceTasks
	}
	if err := applyDataviewFrom(fromPart, &ast); err != nil {
		return domain.QueryAST{}, err
	}
	if err := parseDataviewTail(tail, &ast); err != nil {
		return domain.QueryAST{}, err
	}
	return ast, nil
}

func dataviewFromBounds(body string) (int, int) {
	idx := indexClauseKeyword(body, "FROM")
	if idx < 0 {
		return -1, -1
	}
	start, ok := clauseContentStart(body[idx:], "FROM")
	if !ok {
		return -1, -1
	}
	return idx, idx + start
}

func dataviewMode(text string) (string, string, bool) {
	for _, mode := range []string{"TABLE", "LIST", "TASK"} {
		if start, ok := clauseContentStart(text, mode); ok {
			return mode, strings.TrimSpace(text[start:]), true
		}
	}
	return "", "", false
}

func dataviewSelects(mode, selectPart string) []domain.QuerySelect {
	switch mode {
	case "TABLE":
		return parseSelectList(selectPart)
	case "TASK":
		return []domain.QuerySelect{{Property: "text"}, {Property: "completed"}, {Property: "due"}}
	default:
		return []domain.QuerySelect{{Property: "title"}}
	}
}

func applyDataviewFrom(part string, ast *domain.QueryAST) error {
	part = strings.TrimSpace(part)
	if part == "" {
		return &domain.CommandError{Code: "dataview_parse_failed", Message: "FROM source cannot be empty", Hint: "Use FROM #tag or FROM \"folder\""}
	}
	if strings.HasPrefix(part, "#") {
		tag := strings.TrimPrefix(part, "#")
		ast.Filters = append(ast.Filters, domain.QueryFilter{Property: "tags", Operator: domain.QueryOperatorContains, Value: tag})
		return nil
	}
	if strings.HasPrefix(part, "\"") && strings.HasSuffix(part, "\"") {
		folder := unquote(part)
		ast.Filters = append(ast.Filters, domain.QueryFilter{Property: "folder", Operator: domain.QueryOperatorEquals, Value: folder})
		return nil
	}
	return &domain.CommandError{Code: "dataview_unsupported_clause", Message: "Unsupported Dataview FROM source: " + part, Hint: "Pinax supports FROM #tag or FROM \"folder\""}
}

func parseDataviewTail(tail string, ast *domain.QueryAST) error {
	remaining := strings.TrimSpace(tail)
	if remaining == "" {
		return nil
	}
	upper := strings.ToUpper(remaining)
	wherePart, afterWhere := cutClause(remaining, upper, "WHERE", []string{"SORT", "GROUP BY", "LIMIT"})
	if wherePart != "" {
		filters, err := parseDataviewWhere(wherePart)
		if err != nil {
			return err
		}
		ast.Filters = append(ast.Filters, filters...)
		remaining = strings.TrimSpace(afterWhere)
		upper = strings.ToUpper(remaining)
	}
	sortPart, afterSort := cutClause(remaining, upper, "SORT", []string{"GROUP BY", "LIMIT"})
	if sortPart != "" {
		ast.Sorts = parseSorts(sortPart)
		remaining = strings.TrimSpace(afterSort)
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
		limitSQL := "SELECT title FROM notes LIMIT " + limitPart
		limitAST, err := ParseSQL(limitSQL)
		if err != nil {
			return err
		}
		ast.Limit = limitAST.Limit
		remaining = strings.TrimSpace(afterLimit)
	}
	if strings.TrimSpace(remaining) != "" {
		return &domain.CommandError{Code: "dataview_unsupported_clause", Message: "Dataview query has unsupported trailing syntax: " + remaining, Hint: "Use WHERE, SORT, GROUP BY, and LIMIT"}
	}
	return nil
}

func parseDataviewWhere(part string) ([]domain.QueryFilter, error) {
	items := splitByAND(part)
	filters := make([]domain.QueryFilter, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(strings.ToLower(item), "contains(") && strings.HasSuffix(item, ")") {
			args := splitCSVRespectQuotes(item[len("contains(") : len(item)-1])
			if len(args) != 2 {
				return nil, &domain.CommandError{Code: "dataview_parse_failed", Message: "contains() expects property and value", Hint: "For example, WHERE contains(tags, \"pinax\")"}
			}
			filters = append(filters, domain.QueryFilter{Property: strings.TrimSpace(args[0]), Operator: domain.QueryOperatorContains, Value: unquote(args[1])})
			continue
		}
		filter, err := parseWhereFilter(item)
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}
