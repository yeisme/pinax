package pinaxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Typed human reading surface (pinax-human-reading-api-v1 P06). All calls
// go through the same bounded, redirect-rejecting transport as the legacy
// client: response size limits apply, redirects never follow, unknown
// schema versions fail safely, and there is no local fallback path.

const (
	ReaderSourceSchemaV1Alpha1        = catalogschema.ReaderSourceV1Alpha1
	ReaderDocumentSchemaV1Alpha1      = catalogschema.ReaderDocumentV1Alpha1
	ReaderNoteListSchemaV1Alpha1      = catalogschema.ReaderNoteListV1Alpha1
	ReaderSearchResultsSchemaV1Alpha1 = catalogschema.ReaderSearchResultsV1Alpha1
	ReaderLinkPageSchemaV1Alpha1      = catalogschema.ReaderLinkPageV1Alpha1
	ReaderAssetRefSchemaV1Alpha1      = catalogschema.ReaderAssetRefV1Alpha1
	ReaderSourceExportSchemaV1Alpha1  = catalogschema.ReaderSourceExportV1Alpha1
)

// ReaderLimits mirrors the frozen budget block of the source descriptor.
type ReaderLimits struct {
	ListDefaultLimit          int `json:"list_default_limit"`
	ListMaxLimit              int `json:"list_max_limit"`
	SnippetMaxChars           int `json:"snippet_max_chars"`
	OutlinePageMaxEntries     int `json:"outline_page_max_entries"`
	DocumentPageMaxTextBytes  int `json:"document_page_max_text_bytes"`
	DocumentPageMaxNodes      int `json:"document_page_max_nodes"`
	DocumentResponseMaxBytes  int `json:"document_response_max_bytes"`
	DocumentSourceMaxBytes    int `json:"document_source_max_bytes"`
	DocumentTotalMaxNodes     int `json:"document_total_max_nodes"`
	AssetRangeMaxBytes        int `json:"asset_range_max_bytes"`
	SourceExportRangeMaxBytes int `json:"source_export_range_max_bytes"`
}

type ReaderIndexState struct {
	Status     string `json:"status"`
	Watermark  string `json:"watermark"`
	Generation int64  `json:"generation"`
}

type ReaderSourceDescriptor struct {
	SchemaVersion string           `json:"schema_version"`
	SessionID     string           `json:"session_id"`
	BindingID     string           `json:"binding_id"`
	DisplayName   string           `json:"display_name"`
	Capabilities  []string         `json:"capabilities"`
	Limits        ReaderLimits     `json:"limits"`
	IndexState    ReaderIndexState `json:"index_state"`
	ObservedAt    string           `json:"observed_at"`
}

type ReaderNoteCard struct {
	ObjectID   string   `json:"object_id"`
	Title      string   `json:"title"`
	TitleState string   `json:"title_state,omitempty"`
	Revision   string   `json:"revision"`
	URI        string   `json:"uri"`
	Status     string   `json:"status"`
	ProjectID  *string  `json:"project_id"`
	Tags       []string `json:"tags"`
	UpdatedAt  string   `json:"updated_at"`
}

type ReaderNoteList struct {
	Items          []ReaderNoteCard `json:"items"`
	NextCursor     string           `json:"next_cursor,omitempty"`
	ObservedAt     string           `json:"observed_at"`
	IndexWatermark string           `json:"index_watermark"`
	IndexState     ReaderIndexState `json:"index_state"`
}

type ReaderSearchSnippet struct {
	ObjectID string `json:"object_id"`
	Snippet  string `json:"snippet"`
}

type ReaderSearchResults struct {
	Items          []ReaderNoteCard      `json:"items"`
	Snippets       []ReaderSearchSnippet `json:"snippets"`
	NextCursor     string                `json:"next_cursor,omitempty"`
	ObservedAt     string                `json:"observed_at"`
	IndexWatermark string                `json:"index_watermark"`
	IndexState     ReaderIndexState      `json:"index_state"`
}

type ReaderOutlineEntry struct {
	Anchor string `json:"anchor"`
	Title  string `json:"title"`
	Level  int    `json:"level"`
}

type ReaderDocumentNode struct {
	Type          string               `json:"type"`
	Anchor        string               `json:"anchor"`
	Text          string               `json:"text,omitempty"`
	Children      []ReaderDocumentNode `json:"children,omitempty"`
	Ref           string               `json:"ref,omitempty"`
	Language      string               `json:"language,omitempty"`
	Alt           string               `json:"alt,omitempty"`
	BlockAnchor   string               `json:"block_anchor,omitempty"`
	FragmentIndex int                  `json:"fragment_index,omitempty"`
	LastFragment  bool                 `json:"last_fragment,omitempty"`
	OriginalType  string               `json:"original_type,omitempty"`
	Header        []string             `json:"header,omitempty"`
	// Fence semantics (owner-registered read-only handlers): the fence
	// name, its bounded payload, and explicit failure diagnostics.
	Fence      string          `json:"fence,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	NextAction string          `json:"next_action,omitempty"`
}

type ReaderDocumentWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type ReaderDocumentPage struct {
	SchemaVersion   string                  `json:"schema_version"`
	ObjectID        string                  `json:"object_id"`
	Revision        string                  `json:"revision"`
	CurrentRevision string                  `json:"current_revision"`
	URI             string                  `json:"uri"`
	Nodes           []ReaderDocumentNode    `json:"nodes"`
	Outline         []ReaderOutlineEntry    `json:"outline"`
	OutlineCursor   string                  `json:"outline_cursor,omitempty"`
	NextCursor      string                  `json:"next_cursor,omitempty"`
	Complete        bool                    `json:"complete"`
	ContentBytes    int                     `json:"content_bytes"`
	ObservedAt      string                  `json:"observed_at"`
	Warnings        []ReaderDocumentWarning `json:"warnings,omitempty"`
}

type ReaderLinkItem struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
}

type ReaderLinkPage struct {
	ObjectID           string           `json:"object_id"`
	Revision           string           `json:"revision"`
	Direction          string           `json:"direction"`
	Items              []ReaderLinkItem `json:"items"`
	NextCursor         string           `json:"next_cursor,omitempty"`
	RelationsWatermark string           `json:"relations_watermark"`
	ObservedAt         string           `json:"observed_at"`
}

type ReaderAssetRef struct {
	AssetRef     string `json:"asset_ref"`
	Mime         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	Digest       string `json:"digest"`
	Availability string `json:"availability"`
}

type ReaderSourceExportDescriptor struct {
	ObjectID     string `json:"object_id"`
	Revision     string `json:"revision"`
	MediaType    string `json:"media_type"`
	SizeBytes    int64  `json:"size_bytes"`
	Digest       string `json:"digest"`
	ContentRange string `json:"-"`
}

// ReaderPageQuery carries shared pagination controls for typed calls.
type ReaderPageQuery struct {
	ProjectID string
	Tags      []string
	Status    string
	Sort      string
	Limit     int
	Cursor    string
}

// ReaderSource reads the authorized source descriptor.
func (c *Client) ReaderSource(ctx context.Context) (ReaderSourceDescriptor, error) {
	projection, err := c.getProjection(ctx, "/v1/reader/source")
	if err != nil {
		return ReaderSourceDescriptor{}, err
	}
	var payload struct {
		Source ReaderSourceDescriptor `json:"source"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ReaderSourceDescriptor{}, err
	}
	if payload.Source.SchemaVersion != ReaderSourceSchemaV1Alpha1 {
		return ReaderSourceDescriptor{}, newError(CodeUpstreamInvalidResponse, "Pinax reader source schema version is unsupported", nil)
	}
	return payload.Source, nil
}

// ReaderNotes lists note cards for the authorized session.
func (c *Client) ReaderNotes(ctx context.Context, query ReaderPageQuery) (ReaderNoteList, error) {
	values := readerQueryValues(query)
	projection, err := c.getReaderProjection(ctx, "/v1/reader/notes", values)
	if err != nil {
		return ReaderNoteList{}, err
	}
	var payload struct {
		Notes ReaderNoteList `json:"notes"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ReaderNoteList{}, err
	}
	return payload.Notes, nil
}

// ReaderSearch runs keyword search; the query never leaves this call.
func (c *Client) ReaderSearch(ctx context.Context, queryText string, query ReaderPageQuery) (ReaderSearchResults, error) {
	trimmed := strings.TrimSpace(queryText)
	if trimmed == "" || len([]rune(trimmed)) > catalogschema.ReaderQueryMaxChars {
		return ReaderSearchResults{}, newError(CodeRequestInvalid, "Pinax reader search query must be 1-512 characters after trimming", nil)
	}
	values := readerQueryValues(query)
	values.Set("q", trimmed)
	projection, err := c.getReaderProjection(ctx, "/v1/reader/search", values)
	if err != nil {
		return ReaderSearchResults{}, err
	}
	var payload struct {
		Search ReaderSearchResults `json:"search"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ReaderSearchResults{}, err
	}
	return payload.Search, nil
}

// ReaderDocumentRequest selects one page of the human reading projection.
type ReaderDocumentRequest struct {
	ObjectID      string
	Revision      string
	Cursor        string
	Anchor        string
	OutlineCursor string
}

// ReaderDocument reads one bounded document page.
func (c *Client) ReaderDocument(ctx context.Context, request ReaderDocumentRequest) (ReaderDocumentPage, error) {
	if strings.TrimSpace(request.ObjectID) == "" {
		return ReaderDocumentPage{}, newError(CodeRequestInvalid, "Pinax reader document requires an object id", nil)
	}
	if request.Cursor != "" && request.Anchor != "" {
		return ReaderDocumentPage{}, newError(CodeRequestInvalid, "Pinax reader document cursor and anchor are mutually exclusive", nil)
	}
	values := url.Values{}
	readerSetIfPresent(values, "revision", request.Revision)
	readerSetIfPresent(values, "cursor", request.Cursor)
	readerSetIfPresent(values, "anchor", request.Anchor)
	readerSetIfPresent(values, "outline_cursor", request.OutlineCursor)
	projection, err := c.getReaderProjection(ctx, "/v1/reader/notes/"+url.PathEscape(request.ObjectID)+"/document", values)
	if err != nil {
		return ReaderDocumentPage{}, err
	}
	var payload struct {
		Document ReaderDocumentPage `json:"document"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ReaderDocumentPage{}, err
	}
	if payload.Document.SchemaVersion != ReaderDocumentSchemaV1Alpha1 {
		return ReaderDocumentPage{}, newError(CodeUpstreamInvalidResponse, "Pinax reader document schema version is unsupported", nil)
	}
	return payload.Document, nil
}

// ReaderLinks lists authorized relation edges for one pinned revision.
func (c *Client) ReaderLinks(ctx context.Context, objectID, revision, direction string, query ReaderPageQuery) (ReaderLinkPage, error) {
	if strings.TrimSpace(objectID) == "" || strings.TrimSpace(revision) == "" {
		return ReaderLinkPage{}, newError(CodeRequestInvalid, "Pinax reader links require an object id and revision", nil)
	}
	values := readerQueryValues(query)
	values.Set("revision", revision)
	if direction != "" {
		values.Set("direction", direction)
	}
	projection, err := c.getReaderProjection(ctx, "/v1/reader/notes/"+url.PathEscape(objectID)+"/links", values)
	if err != nil {
		return ReaderLinkPage{}, err
	}
	var payload struct {
		Links ReaderLinkPage `json:"links"`
	}
	if err := decodeProjectionData(projection, &payload); err != nil {
		return ReaderLinkPage{}, err
	}
	return payload.Links, nil
}

// ReaderByteRange selects one inclusive byte window for ranged reads. The
// zero value (Set=false) means "no Range header": the whole body is
// requested and must fit the per-request window. Set=true sends the
// explicit window as-is — including the legal one-byte window {0, 0}
// (bytes=0-0) — and an out-of-order or negative window fails the call
// safely instead of silently degrading to a full-body download.
type ReaderByteRange struct {
	Start int64
	End   int64
	Set   bool
}

func (r ReaderByteRange) header() (string, error) {
	if !r.Set {
		return "", nil
	}
	if r.Start < 0 || r.End < r.Start {
		return "", fmt.Errorf("the byte range %d-%d is not a valid inclusive window", r.Start, r.End)
	}
	return fmt.Sprintf("bytes=%d-%d", r.Start, r.End), nil
}

// ReaderAssetContent is the resolved asset bytes plus its descriptor. For
// ranged reads ContentRange carries the server's bytes a-b/total reply.
type ReaderAssetContent struct {
	Descriptor   ReaderAssetRef
	Body         []byte
	ContentRange string
}

// ReaderAsset reads one bounded asset body bound to a note revision. Use
// ReaderAssetRange for content above the per-request byte window.
func (c *Client) ReaderAsset(ctx context.Context, assetRef, noteID, noteRevision string) (ReaderAssetContent, error) {
	return c.ReaderAssetRange(ctx, assetRef, noteID, noteRevision, ReaderByteRange{})
}

// ReaderAssetRange reads one explicit byte window of an asset (206) or, with
// an empty range, the whole body when it fits the window budget (200).
func (c *Client) ReaderAssetRange(ctx context.Context, assetRef, noteID, noteRevision string, rng ReaderByteRange) (ReaderAssetContent, error) {
	if strings.TrimSpace(assetRef) == "" || strings.TrimSpace(noteID) == "" || strings.TrimSpace(noteRevision) == "" {
		return ReaderAssetContent{}, newError(CodeRequestInvalid, "Pinax reader asset requires asset ref, note id and note revision", nil)
	}
	values := url.Values{}
	values.Set("note_id", noteID)
	values.Set("note_revision", noteRevision)
	body, contentType, contentRange, totalSize, digest, err := c.getReaderBytes(ctx, "/v1/reader/assets/"+url.PathEscape(assetRef)+"/content", values, rng)
	if err != nil {
		return ReaderAssetContent{}, err
	}
	return ReaderAssetContent{
		Descriptor:   ReaderAssetRef{AssetRef: assetRef, Mime: contentType, SizeBytes: totalSize, Digest: digest, Availability: "available"},
		Body:         body,
		ContentRange: contentRange,
	}, nil
}

// ReaderSourceExport downloads the markdown source of a pinned revision.
// Content above the per-request window must be fetched via
// ReaderSourceExportRange windows; unknown total sizes can be discovered
// from the first range reply's Content-Range total.
func (c *Client) ReaderSourceExport(ctx context.Context, objectID, revision string) (ReaderSourceExportDescriptor, []byte, error) {
	return c.ReaderSourceExportRange(ctx, objectID, revision, ReaderByteRange{})
}

// ReaderSourceExportRange downloads one explicit byte window of the source
// (206) or, with an empty range, the whole body when it fits (200).
func (c *Client) ReaderSourceExportRange(ctx context.Context, objectID, revision string, rng ReaderByteRange) (ReaderSourceExportDescriptor, []byte, error) {
	if strings.TrimSpace(objectID) == "" || strings.TrimSpace(revision) == "" {
		return ReaderSourceExportDescriptor{}, nil, newError(CodeRequestInvalid, "Pinax reader source export requires an object id and revision", nil)
	}
	values := url.Values{}
	values.Set("revision", revision)
	body, contentType, contentRange, totalSize, digest, err := c.getReaderBytes(ctx, "/v1/reader/notes/"+url.PathEscape(objectID)+"/source", values, rng)
	if err != nil {
		return ReaderSourceExportDescriptor{}, nil, err
	}
	descriptor := ReaderSourceExportDescriptor{
		ObjectID:     objectID,
		Revision:     revision,
		MediaType:    contentType,
		SizeBytes:    totalSize,
		Digest:       digest,
		ContentRange: contentRange,
	}
	return descriptor, body, nil
}

func (c *Client) getReaderProjection(ctx context.Context, endpointPath string, values url.Values) (Projection, error) {
	endpoint, err := c.endpoint(endpointPath)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax request path is invalid", err)
	}
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Projection{}, newError(CodeRequestInvalid, "Pinax request could not be created", err)
	}
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.doReaderProjection(request)
}

// getReaderBytes fetches bounded byte bodies (asset/export) with the same
// redirect, size and error discipline as projections, plus an optional
// explicit Range window for content above the per-request budget.
func (c *Client) getReaderBytes(ctx context.Context, endpointPath string, values url.Values, rng ReaderByteRange) ([]byte, string, string, int64, string, error) {
	endpoint, err := c.endpoint(endpointPath)
	if err != nil {
		return nil, "", "", 0, "", newError(CodeRequestInvalid, "Pinax request path is invalid", err)
	}
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", "", 0, "", newError(CodeRequestInvalid, "Pinax request could not be created", err)
	}
	header, headerErr := rng.header()
	if headerErr != nil {
		return nil, "", "", 0, "", newError(CodeRequestInvalid, "The byte range is invalid", headerErr)
	}
	if header != "" {
		request.Header.Set("Range", header)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		clientErr := newError(CodeRequestFailed, "Pinax reader request failed", err)
		clientErr.Retryable = true
		return nil, "", "", 0, "", clientErr
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return nil, "", "", 0, "", &Error{Code: CodeRedirectRejected, Message: "Pinax reader redirect was rejected", HTTPStatus: response.StatusCode}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, "", "", 0, "", newError(CodeRequestFailed, "Pinax reader response could not be read", err)
	}
	if int64(len(encoded)) > c.maxResponseBytes {
		return nil, "", "", 0, "", &Error{Code: CodeResponseTooLarge, Message: "Pinax reader response exceeds the configured size limit", HTTPStatus: response.StatusCode}
	}
	contentType := response.Header.Get("Content-Type")
	contentRange := response.Header.Get("Content-Range")
	digest := response.Header.Get("ETag")
	// The descriptor's size is the TOTAL resource size: from Content-Range
	// on 206 replies, from Content-Length on full 200 bodies — never the
	// served window length.
	totalSize := int64(len(encoded))
	if total, parseErr := parseContentRangeTotal(contentRange); parseErr == nil {
		totalSize = total
	} else if length, convErr := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64); convErr == nil && length > 0 {
		totalSize = length
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Byte endpoints surface projection errors as JSON; keep them typed.
		var projection Projection
		if jsonErr := json.Unmarshal(encoded, &projection); jsonErr == nil && projection.Command != "" {
			return nil, "", "", 0, "", projectionClientError(projection, response)
		}
		return nil, "", "", 0, "", &Error{Code: CodeUpstreamInvalidResponse, Message: fmt.Sprintf("Pinax reader byte request failed with HTTP %d", response.StatusCode), HTTPStatus: response.StatusCode}
	}
	return encoded, contentType, contentRange, totalSize, digest, nil
}

// parseContentRangeTotal extracts the total from "bytes start-end/total".
func parseContentRangeTotal(contentRange string) (int64, error) {
	if contentRange == "" {
		return 0, fmt.Errorf("no content range")
	}
	slash := strings.LastIndex(contentRange, "/")
	if slash < 0 || slash == len(contentRange)-1 {
		return 0, fmt.Errorf("content range without total: %q", contentRange)
	}
	return strconv.ParseInt(contentRange[slash+1:], 10, 64)
}

// doReaderProjection mirrors doProjection for reader GETs with query values.
func (c *Client) doReaderProjection(request *http.Request) (Projection, error) {
	response, err := c.http.Do(request)
	if err != nil {
		clientErr := newError(CodeRequestFailed, "Pinax reader request failed", err)
		clientErr.Retryable = true
		return Projection{}, clientErr
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return Projection{}, &Error{Code: CodeRedirectRejected, Message: "Pinax reader redirect was rejected", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return Projection{}, newError(CodeRequestFailed, "Pinax reader response could not be read", err)
	}
	if int64(len(encoded)) > c.maxResponseBytes {
		return Projection{}, &Error{Code: CodeResponseTooLarge, Message: "Pinax reader response exceeds the configured size limit", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	var projection Projection
	if err := json.Unmarshal(encoded, &projection); err != nil || projection.SpecVersion != ProjectionSpecVersion || projection.Command == "" || projection.Status == "" {
		return Projection{}, &Error{Code: CodeUpstreamInvalidResponse, Message: "Pinax reader returned an invalid projection", HTTPStatus: response.StatusCode, RequestID: response.Header.Get("X-Request-ID")}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || projection.Error != nil || projection.Status == "failed" {
		return projection, projectionClientError(projection, response)
	}
	return projection, nil
}

func readerQueryValues(query ReaderPageQuery) url.Values {
	values := url.Values{}
	readerSetIfPresent(values, "project_id", query.ProjectID)
	for _, tag := range query.Tags {
		if strings.TrimSpace(tag) != "" {
			values.Add("tag", tag)
		}
	}
	readerSetIfPresent(values, "status", query.Status)
	readerSetIfPresent(values, "sort", query.Sort)
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	readerSetIfPresent(values, "cursor", query.Cursor)
	return values
}

func readerSetIfPresent(values url.Values, key, value string) {
	if strings.TrimSpace(value) != "" {
		values.Set(key, value)
	}
}
