package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/app/searchops"

	"github.com/yeisme/pinax/internal/domain"
)

func TestPinaxSQLParserBuildsQueryAST(t *testing.T) {
	ast, err := searchops.ParseSQL(`SELECT title, status AS state FROM notes WHERE status = "active" AND tags CONTAINS "pinax" ORDER BY updated_at DESC LIMIT 20`)
	if err != nil {
		t.Fatalf("parse sql: %v", err)
	}
	if ast.Source != domain.QuerySourceNotes || ast.Limit != 20 || len(ast.Select) != 2 || ast.Select[1].Alias != "state" {
		t.Fatalf("ast = %#v", ast)
	}
	if len(ast.Filters) != 2 || ast.Filters[0].Property != "status" || ast.Filters[0].Operator != domain.QueryOperatorEquals || ast.Filters[1].Operator != domain.QueryOperatorContains {
		t.Fatalf("filters = %#v", ast.Filters)
	}
	if len(ast.Sorts) != 1 || ast.Sorts[0].Property != "updated_at" || ast.Sorts[0].Direction != domain.SortDesc {
		t.Fatalf("sorts = %#v", ast.Sorts)
	}
}

func TestPinaxSQLV2ParserBuildsAggregateGroupedAST(t *testing.T) {
	ast, err := searchops.ParseSQL(`SELECT status, COUNT(*) AS total, MIN(priority) AS min_priority, MAX(updated_at) AS newest FROM notes WHERE status IN ("active", "done") AND priority >= 2 AND due IS NOT EMPTY GROUP BY status ORDER BY total DESC LIMIT 10`)
	if err != nil {
		t.Fatalf("parse sql v2 aggregate: %v", err)
	}
	if ast.Source != domain.QuerySourceNotes || ast.Limit != 10 || len(ast.Groups) != 1 || ast.Groups[0] != "status" {
		t.Fatalf("ast = %#v", ast)
	}
	if len(ast.Select) != 4 || ast.Select[1].Aggregate != domain.QueryAggregateCount || ast.Select[1].Property != "*" || ast.Select[1].Alias != "total" {
		t.Fatalf("selects = %#v", ast.Select)
	}
	if ast.Select[2].Aggregate != domain.QueryAggregateMin || ast.Select[2].Property != "priority" || ast.Select[3].Aggregate != domain.QueryAggregateMax || ast.Select[3].Property != "updated_at" {
		t.Fatalf("aggregate selects = %#v", ast.Select)
	}
	if len(ast.Filters) != 3 {
		t.Fatalf("filters = %#v", ast.Filters)
	}
	if ast.Filters[0].Property != "status" || ast.Filters[0].Operator != domain.QueryOperatorIn {
		t.Fatalf("IN filter = %#v", ast.Filters[0])
	}
	values, ok := ast.Filters[0].Value.([]string)
	if !ok || len(values) != 2 || values[0] != "active" || values[1] != "done" {
		t.Fatalf("IN values = %#v", ast.Filters[0].Value)
	}
	if ast.Filters[1].Property != "priority" || ast.Filters[1].Operator != domain.QueryOperatorGTE || ast.Filters[1].Value != "2" {
		t.Fatalf("comparison filter = %#v", ast.Filters[1])
	}
	if ast.Filters[2].Property != "due" || ast.Filters[2].Operator != domain.QueryOperatorIsNotEmpty {
		t.Fatalf("empty filter = %#v", ast.Filters[2])
	}
	if len(ast.Sorts) != 1 || ast.Sorts[0].Property != "total" || ast.Sorts[0].Direction != domain.SortDesc {
		t.Fatalf("sorts = %#v", ast.Sorts)
	}
}

func TestPinaxSQLV2ParserSourcesAndExists(t *testing.T) {
	for _, source := range []domain.QuerySource{domain.QuerySourceTasks, domain.QuerySourceLinks, domain.QuerySourceBacklinks, domain.QuerySourceAssets} {
		t.Run(string(source), func(t *testing.T) {
			ast, err := searchops.ParseSQL(`SELECT title FROM ` + string(source) + ` WHERE EXISTS target LIMIT 5`)
			if err != nil {
				t.Fatalf("parse source %s: %v", source, err)
			}
			if ast.Source != source || len(ast.Filters) != 1 || ast.Filters[0].Operator != domain.QueryOperatorExists || ast.Filters[0].Property != "target" {
				t.Fatalf("ast = %#v", ast)
			}
		})
	}
}

func TestDatabaseSchemaSetRejectsUnsafeValues(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.DatabaseSchemaSet(ctx, DatabaseSchemaRequest{VaultPath: root, Name: "status", Type: "select", Values: []string{"bad]"}}); !hasCommandCode(err, "invalid_tag") {
		t.Fatalf("unsafe schema value should fail with invalid_tag, got %v", err)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "schema-overrides.json")) {
		t.Fatalf("unsafe schema value wrote schema overrides")
	}
}

func TestPinaxSQLRejectsForbiddenAndUnsupportedSyntax(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		code string
	}{
		{name: "forbidden function", sql: `SELECT env(secret) FROM notes`, code: "sql_forbidden_function"},
		{name: "join", sql: `SELECT title FROM notes JOIN tasks ON tasks.note_id = notes.note_id`, code: "sql_unsupported_clause"},
		{name: "source", sql: `SELECT title FROM files`, code: "sql_unsupported_source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := searchops.ParseSQL(tc.sql)
			if err == nil {
				t.Fatal("expected error")
			}
			if !hasCommandCode(err, tc.code) {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}

func TestQueryExplainProjection(t *testing.T) {
	svc := NewService()
	projection, err := svc.QueryExplain(t.Context(), QueryRequest{SQL: `SELECT title, status FROM notes WHERE status = "active" LIMIT 10`})
	if err != nil {
		t.Fatalf("query explain: %v", err)
	}
	if projection.Command != "query.explain" || projection.Facts["source"] != "notes" || projection.Facts["columns"] != "title,status" || projection.Facts["limit"] != "10" {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestQueryRunRequiresIndexUnlessLazy(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Active", Body: "priority:: 2\n", Status: "active", Tags: []string{"pinax"}}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	_, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title, status FROM notes WHERE status = "active" LIMIT 5`})
	if !hasCommandCode(err, "index_required") {
		t.Fatalf("query run without index err = %v", err)
	}

	projection, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title, status FROM notes WHERE status = "active" LIMIT 5`, LazyIndex: true})
	if err != nil {
		t.Fatalf("query run lazy: %v", err)
	}
	if projection.Command != "query.run" || projection.Facts["rows"] != "1" || projection.Facts["index_loaded"] != "lazy_rebuild" || projection.Facts["columns"] != "title,status" {
		t.Fatalf("query run projection = %#v", projection)
	}
}

func TestQueryPlannerPropertyFilterSafeQuery(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Body: "priority:: 1\n", Status: "active", Tags: []string{"pinax"}}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "B", Body: "priority:: 2\n", Status: "done", Tags: []string{"other"}}); err != nil {
		t.Fatalf("create B: %v", err)
	}
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	projection, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title, priority FROM notes WHERE status = "active" AND tags CONTAINS "pinax" ORDER BY priority DESC LIMIT 10`})
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	if projection.Facts["rows"] != "1" || projection.Facts["columns"] != "title,priority" || projection.Facts["index_status"] != "fresh" {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestQueryPaginationCursorAndSelectedProperty(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	for _, title := range []string{"A", "B"} {
		if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: title, Body: "hidden body", Status: "active"}); err != nil {
			t.Fatalf("create note %s: %v", title, err)
		}
	}
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	first, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title FROM notes ORDER BY title ASC LIMIT 1`})
	if err != nil {
		t.Fatalf("query first page: %v", err)
	}
	if first.Facts["has_more"] != "true" || first.Facts["next_cursor"] == "" {
		t.Fatalf("first page facts = %#v", first.Facts)
	}
	result := first.Data.(map[string]any)["result"].(domain.TableResult)
	if result.Rows[0].Note.Body != "" || result.Columns[0] != "title" {
		t.Fatalf("selected row leaked body or columns: %#v", result.Rows[0])
	}
	second, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title FROM notes ORDER BY title ASC LIMIT 1`, Cursor: first.Facts["next_cursor"]})
	if err != nil {
		t.Fatalf("query second page: %v", err)
	}
	if second.Facts["has_more"] != "false" || second.Facts["rows"] != "1" {
		t.Fatalf("second page facts = %#v", second.Facts)
	}
}
func TestQueryRunProjectsSelectedColumnsAndParsesComparisons(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Body: "priority:: 2\nsecret:: hidden\n", Status: "active"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	projection, err := svc.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: `SELECT title FROM notes WHERE status = "active"`})
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	result := projection.Data.(map[string]any)["result"].(domain.TableResult)
	if _, ok := result.Rows[0].Values["secret"]; ok {
		t.Fatalf("unselected property leaked into row values: %#v", result.Rows[0].Values)
	}
	ast, err := searchops.ParseSQL(`SELECT title FROM notes WHERE priority > 1`)
	if err != nil {
		t.Fatalf("comparison operator should parse: %v", err)
	}
	if len(ast.Filters) != 1 || ast.Filters[0].Operator != domain.QueryOperatorGT || ast.Filters[0].Value != "1" {
		t.Fatalf("comparison filter = %#v", ast.Filters)
	}
}

func TestDatabaseViewShowRunsSavedSQLQuery(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Active", Body: "", Status: "active"}); err != nil {
		t.Fatalf("create active: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Done", Body: "", Status: "done"}); err != nil {
		t.Fatalf("create done: %v", err)
	}
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if _, err := svc.SaveDatabaseView(ctx, ViewRequest{VaultPath: root, Name: "active-sql", Query: `SELECT title FROM notes WHERE status = "active"`}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	projection, err := svc.ShowDatabaseView(ctx, ViewRequest{VaultPath: root, Name: "active-sql"})
	if err != nil {
		t.Fatalf("show view: %v", err)
	}
	result := projection.Data.(map[string]any)["result"].(map[string]any)["result"].(domain.TableResult)
	if result.RowCount() != 1 || result.Rows[0].Note.Title != "Active" {
		t.Fatalf("saved SQL view result = %#v", result)
	}
}
