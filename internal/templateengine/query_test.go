package templateengine

import (
	"strings"
	"testing"
)

func TestTemplateQueryDeclarationAndFence(t *testing.T) {
	doc, err := ParseDocument("query", "---\nschema_version: pinax.template.v2\nengine: go-template\nqueries:\n  active:\n    language: sql\n    sql: SELECT title FROM notes WHERE status = \"active\" LIMIT 5\n    kind: table\n    max_rows: 5\n    required: true\n---\n# Active\n\n```pinax-sql recent\nSELECT title FROM notes LIMIT 3\n```\n{{ table .Queries.active }}\n")
	if err != nil {
		t.Fatalf("parse query document: %v", err)
	}
	active := doc.Metadata.Queries["active"]
	if active.Language != "sql" || active.MaxRows != 5 || !active.Required || active.SQL == "" {
		t.Fatalf("active query = %#v", active)
	}
	recent := doc.Metadata.Queries["recent"]
	if recent.Language != "sql" || recent.SQL != "SELECT title FROM notes LIMIT 3" {
		t.Fatalf("recent query = %#v", recent)
	}
	if contains(doc.Body, "pinax-sql") {
		t.Fatalf("fenced query leaked into render body:\n%s", doc.Body)
	}
}

func TestQueryResultTableListAndForbiddenDynamicQueryFunc(t *testing.T) {
	result := QueryResult{Columns: []string{"title", "status"}, Rows: []map[string]string{{"title": "A", "status": "active"}}}
	rendered, err := New().Render(TemplateDocument{Name: "helpers", Engine: EngineGoTemplate, Body: "{{ table .Queries.active }}\n{{ list .Queries.active \"title\" }}"}, Context{Queries: map[string]QueryResult{"active": result}})
	if err != nil {
		t.Fatalf("render helpers: %v", err)
	}
	if !contains(rendered.Body, "| title | status |") || !contains(rendered.Body, "- A") {
		t.Fatalf("rendered helpers = %s", rendered.Body)
	}
	_, err = New().Render(TemplateDocument{Name: "bad", Engine: EngineGoTemplate, Body: `{{ query "SELECT title FROM notes" }}`}, Context{})
	if err == nil || ErrorCode(err) != "template_parse_failed" {
		t.Fatalf("dynamic query func err = %v", err)
	}
}

func contains(value, needle string) bool { return strings.Contains(value, needle) }
