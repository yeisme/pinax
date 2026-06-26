package domain

import "fmt"

type PropertyType string

const (
	PropertyTypeText        PropertyType = "text"
	PropertyTypeString      PropertyType = "string"
	PropertyTypeNumber      PropertyType = "number"
	PropertyTypeCheckbox    PropertyType = "checkbox"
	PropertyTypeBoolean     PropertyType = "boolean"
	PropertyTypeDate        PropertyType = "date"
	PropertyTypeMultiSelect PropertyType = "multi_select"
	PropertyTypeList        PropertyType = "list"
	PropertyTypeLink        PropertyType = "link"
	PropertyTypeURL         PropertyType = "url"
	PropertyTypeEmail       PropertyType = "email"
	PropertyTypePersonText  PropertyType = "person_text"
	PropertyTypeRelation    PropertyType = "relation"
	PropertyTypeRollup      PropertyType = "rollup"
	PropertyTypeFormula     PropertyType = "formula"
	PropertyTypeSelect      PropertyType = "select"
	PropertyTypeMixed       PropertyType = "mixed"
)

const PropertySchemaOverridesVersion = "pinax.schema_overrides.v1"

type PropertySchemaOverrideRegistry struct {
	SchemaVersion string                            `json:"schema_version"`
	Properties    map[string]PropertySchemaOverride `json:"properties"`
}

type PropertySchemaOverride struct {
	Type      PropertyType `json:"type"`
	Values    []string     `json:"values,omitempty"`
	UpdatedAt string       `json:"updated_at,omitempty"`
}

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
	QuerySourceRelations QuerySource = "relations"
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
	QueryAggregateCount         QueryAggregate = "COUNT"
	QueryAggregateMin           QueryAggregate = "MIN"
	QueryAggregateMax           QueryAggregate = "MAX"
	QueryAggregateLatest        QueryAggregate = "LATEST"
	QueryAggregateStatusSummary QueryAggregate = "STATUS_SUMMARY"
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

type DatabaseViewRender struct {
	Display  string                     `json:"display"`
	RowCount int                        `json:"row_count"`
	Table    *TableResult               `json:"table,omitempty"`
	List     []DatabaseViewListItem     `json:"list,omitempty"`
	Board    DatabaseViewBoardRender    `json:"board,omitempty"`
	Calendar DatabaseViewCalendarRender `json:"calendar,omitempty"`
}

type DatabaseTab struct {
	Name    string             `json:"name"`
	View    string             `json:"view"`
	Display string             `json:"display"`
	Rows    int                `json:"rows"`
	Render  DatabaseViewRender `json:"render"`
	Facts   map[string]string  `json:"facts,omitempty"`
}

type DatabaseViewListItem struct {
	Title  string            `json:"title"`
	Path   string            `json:"path,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

type DatabaseViewBoardRender struct {
	GroupBy string                    `json:"group_by"`
	Columns []DatabaseViewBoardColumn `json:"columns"`
}

type DatabaseViewBoardColumn struct {
	ID    string                 `json:"id"`
	Count int                    `json:"count"`
	Items []DatabaseViewListItem `json:"items"`
}

type DatabaseViewCalendarRender struct {
	DateField string                      `json:"date_field"`
	Events    []DatabaseViewCalendarEvent `json:"events"`
}

type DatabaseViewCalendarEvent struct {
	Date   string `json:"date"`
	Title  string `json:"title"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status,omitempty"`
}
