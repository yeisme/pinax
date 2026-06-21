package searchops

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestExecuteQueryAggregatesGroupsAndPages(t *testing.T) {
	notes := []domain.Note{
		{Title: "A", Path: "notes/a.md", Status: "active", Body: "priority:: 2\ndue:: 2026-06-20\nSECRET_BODY"},
		{Title: "B", Path: "notes/b.md", Status: "active", Body: "priority:: 5\ndue:: 2026-06-21\nSECRET_BODY"},
		{Title: "C", Path: "notes/c.md", Status: "done", Body: "priority:: 3\ndue:: 2026-06-22\nSECRET_BODY"},
	}
	ast, err := ParseSQL(`SELECT status, COUNT(*) AS total, MIN(priority) AS min_priority, MAX(priority) AS max_priority FROM notes WHERE priority >= 2 AND status IN ("active", "done") GROUP BY status ORDER BY total DESC LIMIT 1`)
	if err != nil {
		t.Fatalf("parse sql: %v", err)
	}

	first := ExecuteQuery(notes, ast, QueryRequest{})
	if first.RowCount() != 1 || first.Columns[0] != "status" || first.Columns[1] != "total" || !first.Page.HasMore || first.Page.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	row := first.Rows[0]
	if row.Note.Body != "" {
		t.Fatalf("aggregate row leaked note body: %#v", row.Note)
	}
	if row.Values["status"].String() != "active" || row.Values["total"].String() != "2" || row.Values["min_priority"].String() != "2" || row.Values["max_priority"].String() != "5" {
		t.Fatalf("aggregate values = %#v", row.Values)
	}

	second := ExecuteQuery(notes, ast, QueryRequest{Cursor: first.Page.NextCursor})
	if second.RowCount() != 1 || second.Page.HasMore || second.Rows[0].Values["status"].String() != "done" || second.Rows[0].Values["total"].String() != "1" {
		t.Fatalf("second page = %#v", second)
	}
}

func TestExecuteQueryTypedFiltersAndEmptyChecks(t *testing.T) {
	notes := []domain.Note{
		{Title: "A", Path: "notes/a.md", Status: "active", Body: "priority:: 2\npublished:: true\ndue:: 2026-06-20"},
		{Title: "B", Path: "notes/b.md", Status: "done", Body: "priority:: 1\npublished:: false"},
	}
	ast, err := ParseSQL(`SELECT title FROM notes WHERE priority > 1 AND published = true AND due IS NOT EMPTY`)
	if err != nil {
		t.Fatalf("parse sql: %v", err)
	}

	result := ExecuteQuery(notes, ast, QueryRequest{})
	if result.RowCount() != 1 || result.Rows[0].Values["title"].String() != "A" {
		t.Fatalf("typed filter result = %#v", result)
	}
}

func TestParseDataviewLowersSupportedSubset(t *testing.T) {
	ast, err := ParseDataview(`TABLE title, status FROM #pinax WHERE contains(tags, "project") SORT updated_at DESC GROUP BY status LIMIT 5`)
	if err != nil {
		t.Fatalf("parse dataview table: %v", err)
	}
	if ast.Source != domain.QuerySourceNotes || len(ast.Select) != 2 || ast.Select[0].Property != "title" || ast.Limit != 5 {
		t.Fatalf("ast = %#v", ast)
	}
	if len(ast.Filters) != 2 || ast.Filters[0].Property != "tags" || ast.Filters[0].Operator != domain.QueryOperatorContains || ast.Filters[0].Value != "pinax" || ast.Filters[1].Value != "project" {
		t.Fatalf("filters = %#v", ast.Filters)
	}
	if len(ast.Sorts) != 1 || ast.Sorts[0].Property != "updated_at" || ast.Sorts[0].Direction != domain.SortDesc || len(ast.Groups) != 1 || ast.Groups[0] != "status" {
		t.Fatalf("sort/group = %#v %#v", ast.Sorts, ast.Groups)
	}

	task, err := ParseDataview(`TASK FROM "projects" WHERE status = "active" LIMIT 3`)
	if err != nil {
		t.Fatalf("parse dataview task: %v", err)
	}
	if task.Source != domain.QuerySourceTasks || task.Select[0].Property != "text" || task.Filters[0].Property != "folder" || task.Filters[0].Value != "projects" {
		t.Fatalf("task ast = %#v", task)
	}

	list, err := ParseDataview(`LIST FROM #idea LIMIT 2`)
	if err != nil {
		t.Fatalf("parse dataview list: %v", err)
	}
	if list.Source != domain.QuerySourceNotes || len(list.Select) != 1 || list.Select[0].Property != "title" {
		t.Fatalf("list ast = %#v", list)
	}
}

func TestParseDataviewAcceptsMultilineClauses(t *testing.T) {
	ast, err := ParseDataview(`TABLE title, status
FROM #pinax
WHERE contains(tags, "project")
SORT updated_at DESC
GROUP BY status
LIMIT 5`)
	if err != nil {
		t.Fatalf("parse multiline dataview table: %v", err)
	}
	if ast.Source != domain.QuerySourceNotes || len(ast.Select) != 2 || ast.Select[0].Property != "title" || ast.Limit != 5 {
		t.Fatalf("ast = %#v", ast)
	}
	if len(ast.Filters) != 2 || ast.Filters[0].Value != "pinax" || ast.Filters[1].Property != "tags" || ast.Filters[1].Value != "project" {
		t.Fatalf("filters = %#v", ast.Filters)
	}
	if len(ast.Sorts) != 1 || ast.Sorts[0].Property != "updated_at" || ast.Sorts[0].Direction != domain.SortDesc || len(ast.Groups) != 1 || ast.Groups[0] != "status" {
		t.Fatalf("sort/group = %#v %#v", ast.Sorts, ast.Groups)
	}

	task, err := ParseDataview(`TASK
FROM "projects"
WHERE status = "active"
LIMIT 3`)
	if err != nil {
		t.Fatalf("parse multiline dataview task: %v", err)
	}
	if task.Source != domain.QuerySourceTasks || task.Select[0].Property != "text" || task.Filters[0].Property != "folder" || task.Filters[0].Value != "projects" || task.Limit != 3 {
		t.Fatalf("task ast = %#v", task)
	}
}

func TestParseDataviewRejectsUnsupportedAndForbiddenSyntax(t *testing.T) {
	for _, query := range []string{`DATAVIEWJS console.log("x")`, `TABLE env(secret) FROM #pinax`, `TABLE title FROM #pinax FLATTEN tags`} {
		if _, err := ParseDataview(query); err == nil {
			t.Fatalf("expected dataview error for %q", query)
		}
	}
}

func TestExecuteTaskSourceFromMarkdownTasks(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_a", Title: "A", Path: "notes/projects/a.md", Folder: "projects", Body: "- [ ] Draft plan #pinax due:: 2026-06-20 priority:: high ^task-a\n- [x] Done item #pinax"},
		{ID: "note_b", Title: "B", Path: "notes/other/b.md", Folder: "other", Body: "- [ ] Other task #pinax"},
	}
	ast, err := ParseSQL(`SELECT text, completed, due, priority, block_id FROM tasks WHERE folder = "projects" AND completed = false AND tags CONTAINS "pinax" LIMIT 5`)
	if err != nil {
		t.Fatalf("parse task sql: %v", err)
	}

	result := ExecuteQuery(notes, ast, QueryRequest{})
	if result.RowCount() != 1 || result.Columns[0] != "text" {
		t.Fatalf("task result = %#v", result)
	}
	row := result.Rows[0]
	if row.Note.Body != "" || row.Source != string(domain.QuerySourceTasks) {
		t.Fatalf("task row leaked body or source: %#v", row)
	}
	if row.Values["text"].String() != "Draft plan #pinax due:: 2026-06-20 priority:: high ^task-a" || row.Values["completed"].String() != "false" || row.Values["due"].String() != "2026-06-20" || row.Values["priority"].String() != "high" || row.Values["block_id"].String() != "task-a" {
		t.Fatalf("task values = %#v", row.Values)
	}
}

func TestExecuteLinkBacklinkAndAssetSources(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Body: "See [[Beta]] and ![Diagram](../assets/diagram.png)."},
		{ID: "note_beta", Title: "Beta", Path: "notes/beta.md", Body: "Target note."},
	}

	linksAST, err := ParseSQL(`SELECT source_path, target, status, kind FROM links WHERE target = "Beta" LIMIT 5`)
	if err != nil {
		t.Fatalf("parse links sql: %v", err)
	}
	links := ExecuteQuery(notes, linksAST, QueryRequest{})
	if links.RowCount() != 1 || links.Rows[0].Values["source_path"].String() != "notes/alpha.md" || links.Rows[0].Values["status"].String() != "resolved" {
		t.Fatalf("links result = %#v", links)
	}

	backlinksAST, err := ParseSQL(`SELECT source_path, target_path FROM backlinks WHERE target_path = "notes/beta.md" LIMIT 5`)
	if err != nil {
		t.Fatalf("parse backlinks sql: %v", err)
	}
	backlinks := ExecuteQuery(notes, backlinksAST, QueryRequest{})
	if backlinks.RowCount() != 1 || backlinks.Rows[0].Values["source_path"].String() != "notes/alpha.md" {
		t.Fatalf("backlinks result = %#v", backlinks)
	}

	assetsAST, err := ParseSQL(`SELECT path, media_type, linked_notes, status FROM assets WHERE media_type = "image" LIMIT 5`)
	if err != nil {
		t.Fatalf("parse assets sql: %v", err)
	}
	assets := ExecuteQuery(notes, assetsAST, QueryRequest{})
	if assets.RowCount() != 1 || assets.Rows[0].Values["path"].String() != "assets/diagram.png" || assets.Rows[0].Values["linked_notes"].String() != "1" {
		t.Fatalf("assets result = %#v", assets)
	}
}
