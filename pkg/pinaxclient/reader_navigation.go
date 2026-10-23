package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

const (
	ReaderTagsSchemaV1Alpha1  = catalogschema.ReaderTagsV1Alpha1
	ReaderGraphSchemaV1Alpha1 = catalogschema.ReaderGraphV1Alpha1
	ReaderDailySchemaV1Alpha1 = catalogschema.ReaderDailyV1Alpha1
)

type ReaderNavigationCoverage struct {
	PageComplete   bool `json:"page_complete"`
	CorpusComplete bool `json:"corpus_complete"`
}

type ReaderNavigationNote struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
	Title    string `json:"title"`
}

type ReaderTag struct {
	TagID           string `json:"tag_id"`
	Path            string `json:"path"`
	ParentID        string `json:"parent_id"`
	Virtual         bool   `json:"virtual"`
	DirectCount     int    `json:"direct_count"`
	DescendantCount int    `json:"descendant_count"`
}

type ReaderTagsPage struct {
	SchemaVersion string                   `json:"schema_version"`
	BindingID     string                   `json:"binding_id"`
	Items         []ReaderTag              `json:"items"`
	NextCursor    string                   `json:"next_cursor,omitempty"`
	Watermark     string                   `json:"watermark"`
	ObservedAt    string                   `json:"observed_at"`
	TotalTags     int                      `json:"total_tags"`
	Coverage      ReaderNavigationCoverage `json:"coverage"`
}

type ReaderGraphEdge struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

type ReaderGraphRequest struct {
	Mode     string
	ObjectID string
	Revision string
	Hops     int
	Limit    int
	Cursor   string
}

type ReaderGraphPage struct {
	SchemaVersion string                   `json:"schema_version"`
	BindingID     string                   `json:"binding_id"`
	Mode          string                   `json:"mode"`
	ObjectID      string                   `json:"object_id"`
	Revision      string                   `json:"revision"`
	Hops          int                      `json:"hops"`
	Nodes         []ReaderNavigationNote   `json:"nodes"`
	Edges         []ReaderGraphEdge        `json:"edges"`
	NextCursor    string                   `json:"next_cursor,omitempty"`
	Watermark     string                   `json:"watermark"`
	ObservedAt    string                   `json:"observed_at"`
	TotalNodes    int                      `json:"total_nodes"`
	TotalEdges    int                      `json:"total_edges"`
	Coverage      ReaderNavigationCoverage `json:"coverage"`
}

type ReaderDaily struct {
	SchemaVersion string                `json:"schema_version"`
	BindingID     string                `json:"binding_id"`
	Date          string                `json:"date"`
	Timezone      string                `json:"timezone"`
	Status        string                `json:"status"`
	Note          *ReaderNavigationNote `json:"note"`
	Watermark     string                `json:"watermark"`
	ObservedAt    string                `json:"observed_at"`
}

func (c *Client) getNavigation(ctx context.Context, path, requestSchema, responseSchema string, values url.Values, target any) error {
	payload := map[string]any{}
	for key, list := range values {
		if key == "limit" || key == "hops" {
			value, err := strconv.Atoi(list[0])
			if err != nil {
				return newError(CodeRequestInvalid, "Reader navigation requires integer limits", nil)
			}
			payload[key] = float64(value)
		} else {
			payload[key] = list[0]
		}
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), requestSchema, payload) != nil {
		return newError(CodeRequestInvalid, "Reader navigation request violated its schema", nil)
	}
	projection, err := c.getReaderProjection(ctx, path, values)
	if err != nil {
		return err
	}
	var raw any
	if json.Unmarshal(projection.Data, &raw) != nil || catalogschema.ValidatePayload(catalogschema.Default(), responseSchema, raw) != nil {
		return newError(CodeUpstreamInvalidResponse, "Reader navigation response violated its schema", nil)
	}
	return decodeProjectionData(projection, target)
}

// ReaderTags lists one page of the authorized, counted taxonomy projection.
func (c *Client) ReaderTags(ctx context.Context, limit int, cursor string) (ReaderTagsPage, error) {
	var page ReaderTagsPage
	values := url.Values{}
	if limit != 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	readerSetIfPresent(values, "cursor", cursor)
	if err := c.getNavigation(ctx, "/v1/reader/tags", catalogschema.ReaderTagsRequestV1Alpha1, catalogschema.ReaderTagsV1Alpha1, values, &page); err != nil {
		return ReaderTagsPage{}, err
	}
	seen := map[string]bool{}
	for _, tag := range page.Items {
		if seen[tag.TagID] || tag.Virtual != (tag.DirectCount == 0) {
			return ReaderTagsPage{}, newError(CodeUpstreamInvalidResponse, "Tag page contains inconsistent identities or counts", nil)
		}
		seen[tag.TagID] = true
	}
	if page.Coverage.PageComplete != (page.NextCursor == "") || page.TotalTags < len(page.Items) {
		return ReaderTagsPage{}, newError(CodeUpstreamInvalidResponse, "Tag page coverage is inconsistent", nil)
	}
	return page, nil
}

// ReaderGraph returns a closed graph page: both endpoints of every edge are
// present. Merge repeated nodes across pages by binding_id and object_id.
func (c *Client) ReaderGraph(ctx context.Context, req ReaderGraphRequest) (ReaderGraphPage, error) {
	var page ReaderGraphPage
	if req.Mode == "source" && (req.ObjectID != "" || req.Revision != "" || req.Hops != 0) || req.Mode == "local" && (req.ObjectID == "" || req.Revision == "") {
		return page, newError(CodeRequestInvalid, "Graph center is required only in local mode", nil)
	}
	values := url.Values{"mode": {req.Mode}}
	readerSetIfPresent(values, "object_id", req.ObjectID)
	readerSetIfPresent(values, "revision", req.Revision)
	readerSetIfPresent(values, "cursor", req.Cursor)
	if req.Hops != 0 {
		values.Set("hops", strconv.Itoa(req.Hops))
	}
	if req.Limit != 0 {
		values.Set("limit", strconv.Itoa(req.Limit))
	}
	if err := c.getNavigation(ctx, "/v1/reader/graph", catalogschema.ReaderGraphRequestV1Alpha1, catalogschema.ReaderGraphV1Alpha1, values, &page); err != nil {
		return ReaderGraphPage{}, err
	}
	hops := req.Hops
	if req.Mode == "local" && hops == 0 {
		hops = 1
	}
	if page.Mode != req.Mode || page.ObjectID != req.ObjectID || page.Revision != req.Revision || page.Hops != hops || page.Coverage.PageComplete != (page.NextCursor == "") || page.TotalNodes < len(page.Nodes) || page.TotalEdges < len(page.Edges) {
		return ReaderGraphPage{}, newError(CodeUpstreamInvalidResponse, "Graph response identity or coverage did not match", nil)
	}
	seen := map[string]bool{}
	for _, node := range page.Nodes {
		if seen[node.ObjectID] {
			return ReaderGraphPage{}, newError(CodeUpstreamInvalidResponse, "Graph page repeats a node identity", nil)
		}
		seen[node.ObjectID] = true
	}
	edges := map[[2]string]bool{}
	for _, edge := range page.Edges {
		key := [2]string{edge.SourceID, edge.TargetID}
		if !seen[edge.SourceID] || !seen[edge.TargetID] || edges[key] {
			return ReaderGraphPage{}, newError(CodeUpstreamInvalidResponse, "Graph page has a missing endpoint or duplicate edge", nil)
		}
		edges[key] = true
	}
	return page, nil
}

// ReaderDaily locates a date; this method never creates a journal.
func (c *Client) ReaderDaily(ctx context.Context, date, timezone string) (ReaderDaily, error) {
	var page ReaderDaily
	if err := c.getNavigation(ctx, "/v1/reader/journals/daily", catalogschema.ReaderDailyRequestV1Alpha1, catalogschema.ReaderDailyV1Alpha1, url.Values{"date": {date}, "timezone": {timezone}}, &page); err != nil {
		return ReaderDaily{}, err
	}
	if page.Date != date || page.Timezone != timezone || (page.Status == "found") != (page.Note != nil) {
		return ReaderDaily{}, newError(CodeUpstreamInvalidResponse, "Daily lookup identity or state did not match", nil)
	}
	return page, nil
}
