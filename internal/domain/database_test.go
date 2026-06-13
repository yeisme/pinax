package domain

import "testing"

func TestDatabasePropertyQueryDomainModels(t *testing.T) {
	row := DatabaseRow{
		Source: "notes",
		Note:   Note{ID: "note_1", Title: "Alpha", Path: "notes/alpha.md"},
		Values: map[string]PropertyValue{
			"status": {Name: "status", Type: PropertyTypeSelect, Raw: "active", Value: "active", Source: "frontmatter"},
			"tags":   {Name: "tags", Type: PropertyTypeList, Raw: "[pinax]", Value: []string{"pinax"}, Source: "system"},
		},
	}
	if row.Identity() != "notes/alpha.md" || row.Values["status"].String() != "active" {
		t.Fatalf("row identity/value = %#v", row)
	}

	ast := QueryAST{Source: QuerySourceNotes, Select: []QuerySelect{{Property: "title"}}, Filters: []QueryFilter{{Property: "status", Operator: QueryOperatorEquals, Value: "active"}}, Sorts: []QuerySort{{Property: "updated_at", Direction: SortDesc}}, Limit: 20}
	if ast.Source != "notes" || ast.Limit != 20 || ast.Filters[0].Operator != "=" {
		t.Fatalf("query ast = %#v", ast)
	}

	result := TableResult{Columns: []string{"title", "status"}, Rows: []DatabaseRow{row}, Page: QueryPage{Limit: 20, HasMore: false}}
	if result.RowCount() != 1 || result.Page.Limit != 20 {
		t.Fatalf("table result = %#v", result)
	}
}
