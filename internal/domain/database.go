package domain

import "fmt"

type PropertyType string

const (
	PropertyTypeString  PropertyType = "string"
	PropertyTypeNumber  PropertyType = "number"
	PropertyTypeBoolean PropertyType = "boolean"
	PropertyTypeDate    PropertyType = "date"
	PropertyTypeList    PropertyType = "list"
	PropertyTypeLink    PropertyType = "link"
	PropertyTypeSelect  PropertyType = "select"
	PropertyTypeMixed   PropertyType = "mixed"
)

type PropertyDefinition struct {
	Name    string       `json:"name"`
	Type    PropertyType `json:"type"`
	Source  string       `json:"source,omitempty"`
	Count   int          `json:"count,omitempty"`
	Samples []string     `json:"samples,omitempty"`
}

type PropertyValue struct {
	Name   string       `json:"name"`
	Type   PropertyType `json:"type"`
	Raw    string       `json:"raw,omitempty"`
	Value  any          `json:"value,omitempty"`
	Source string       `json:"source,omitempty"`
}

func (v PropertyValue) String() string {
	if v.Value == nil {
		return v.Raw
	}
	return fmt.Sprint(v.Value)
}

type DatabaseRow struct {
	Source string                   `json:"source"`
	Note   Note                     `json:"note,omitempty"`
	Values map[string]PropertyValue `json:"values,omitempty"`
}

func (r DatabaseRow) Identity() string {
	if r.Note.Path != "" {
		return r.Note.Path
	}
	if r.Note.ID != "" {
		return r.Note.ID
	}
	return r.Source
}

type QuerySource string

const (
	QuerySourceNotes     QuerySource = "notes"
	QuerySourceTasks     QuerySource = "tasks"
	QuerySourceLinks     QuerySource = "links"
	QuerySourceBacklinks QuerySource = "backlinks"
	QuerySourceAssets    QuerySource = "assets"
)

type QueryOperator string

const (
	QueryOperatorEquals     QueryOperator = "="
	QueryOperatorNotEqual   QueryOperator = "!="
	QueryOperatorContains   QueryOperator = "CONTAINS"
	QueryOperatorLike       QueryOperator = "LIKE"
	QueryOperatorIn         QueryOperator = "IN"
	QueryOperatorGT         QueryOperator = ">"
	QueryOperatorGTE        QueryOperator = ">="
	QueryOperatorLT         QueryOperator = "<"
	QueryOperatorLTE        QueryOperator = "<="
	QueryOperatorExists     QueryOperator = "EXISTS"
	QueryOperatorIsEmpty    QueryOperator = "IS EMPTY"
	QueryOperatorIsNotEmpty QueryOperator = "IS NOT EMPTY"
)

type QueryAggregate string

const (
	QueryAggregateCount QueryAggregate = "COUNT"
	QueryAggregateMin   QueryAggregate = "MIN"
	QueryAggregateMax   QueryAggregate = "MAX"
)

type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

type QuerySelect struct {
	Property  string         `json:"property"`
	Alias     string         `json:"alias,omitempty"`
	Aggregate QueryAggregate `json:"aggregate,omitempty"`
}

type QueryFilter struct {
	Property string        `json:"property"`
	Operator QueryOperator `json:"operator"`
	Value    any           `json:"value,omitempty"`
}

type QuerySort struct {
	Property  string        `json:"property"`
	Direction SortDirection `json:"direction,omitempty"`
}

type QueryAST struct {
	Select  []QuerySelect `json:"select"`
	Source  QuerySource   `json:"source"`
	Filters []QueryFilter `json:"filters,omitempty"`
	Sorts   []QuerySort   `json:"sorts,omitempty"`
	Groups  []string      `json:"groups,omitempty"`
	Limit   int           `json:"limit,omitempty"`
}

type DatabaseViewDefinition struct {
	ID      string            `json:"id,omitempty"`
	Name    string            `json:"name"`
	Kind    string            `json:"kind,omitempty"`
	Query   string            `json:"query,omitempty"`
	Columns []string          `json:"columns,omitempty"`
	Filters map[string]string `json:"filters,omitempty"`
	Sorts   []string          `json:"sorts,omitempty"`
	Limit   int               `json:"limit,omitempty"`
	Display map[string]string `json:"display,omitempty"`
}

type QueryPage struct {
	Limit      int    `json:"limit"`
	Cursor     string `json:"cursor,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type TableResult struct {
	Columns []string             `json:"columns"`
	Rows    []DatabaseRow        `json:"rows"`
	Page    QueryPage            `json:"page"`
	Schema  []PropertyDefinition `json:"schema,omitempty"`
}

func (r TableResult) RowCount() int {
	return len(r.Rows)
}
