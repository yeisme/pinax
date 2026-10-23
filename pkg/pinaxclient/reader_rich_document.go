package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type ReaderRichAnnotation struct {
	Anchor      string   `json:"anchor"`
	PageAnchors []string `json:"page_anchors"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source,omitempty"`
	CalloutKind string   `json:"callout_kind,omitempty"`
	Title       string   `json:"title,omitempty"`
	Fold        string   `json:"fold,omitempty"`
}
type ReaderRichDocumentPage struct {
	SchemaVersion string                 `json:"schema_version"`
	Document      ReaderDocumentPage     `json:"document"`
	Annotations   []ReaderRichAnnotation `json:"annotations"`
}

// ReaderRichDocument returns inert data only. Callers must not execute source,
// inject it as HTML or treat a Mermaid/TeX link as permission to fetch a URL.
func (c *Client) ReaderRichDocument(ctx context.Context, req ReaderDocumentRequest) (ReaderRichDocumentPage, error) {
	var empty ReaderRichDocumentPage
	values := url.Values{"revision": {req.Revision}}
	readerSetIfPresent(values, "cursor", req.Cursor)
	readerSetIfPresent(values, "anchor", req.Anchor)
	readerSetIfPresent(values, "outline_cursor", req.OutlineCursor)
	payload := map[string]any{"object_id": req.ObjectID}
	for key, items := range values {
		payload[key] = items[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRichDocumentRequestV1Alpha1, payload) != nil || req.Cursor != "" && req.Anchor != "" || strings.ContainsAny(req.ObjectID, "/\\?#\x00") || !utf8.ValidString(req.ObjectID) {
		return empty, newError(CodeRequestInvalid, "Rich document requires a fixed identity and valid position", nil)
	}
	// Respect a smaller caller limit, but never accept a larger rich wire body.
	bounded := *c
	if bounded.maxResponseBytes > catalogschema.ReaderDocumentResponseMaxBytes {
		bounded.maxResponseBytes = catalogschema.ReaderDocumentResponseMaxBytes
	}
	projection, err := bounded.getReaderProjection(ctx, "/v1/reader/notes/"+url.PathEscape(req.ObjectID)+"/rich-document", values)
	if err != nil {
		return empty, err
	}
	bad := func() (ReaderRichDocumentPage, error) {
		return empty, newError(CodeUpstreamInvalidResponse, "Rich document identity, annotations or budgets are inconsistent", nil)
	}
	var raw any
	if projection.Command != "reader.rich_document" || json.Unmarshal(projection.Data, &raw) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRichDocumentV1Alpha1, raw) != nil {
		return bad()
	}
	var page ReaderRichDocumentPage
	if json.Unmarshal(projection.Data, &page) != nil {
		return bad()
	}
	if !validRichClientPage(page, req) {
		return bad()
	}
	return page, nil
}

func validRichClientPage(page ReaderRichDocumentPage, req ReaderDocumentRequest) bool {
	doc := page.Document
	if doc.ObjectID != req.ObjectID || doc.Revision != req.Revision || !validSHA256Digest(doc.CurrentRevision) || doc.URI != "pinax://note/"+req.ObjectID+"?v="+req.Revision || doc.Complete && doc.NextCursor != "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, doc.ObservedAt); err != nil {
		return false
	}
	nodes := map[string]bool{}
	count := 0
	textBytes := 0
	var walk func([]ReaderDocumentNode) bool
	walk = func(items []ReaderDocumentNode) bool {
		for _, node := range items {
			count++
			if count > catalogschema.ReaderDocumentPageMaxNodes || node.Anchor == "" || nodes[node.Anchor] || node.Type == "fence" || node.Type == "fence_failed" || node.Fence != "" || len(node.Payload) > 0 {
				return false
			}
			nodes[node.Anchor] = true
			for _, text := range append([]string{node.Text, node.Alt}, node.Header...) {
				if !utf8.ValidString(text) {
					return false
				}
				textBytes += len(text)
			}
			if textBytes > catalogschema.ReaderDocumentPageMaxTextBytes || !walk(node.Children) {
				return false
			}
		}
		return true
	}
	if !walk(doc.Nodes) || doc.ContentBytes < 0 || doc.ContentBytes > catalogschema.ReaderDocumentPageMaxTextBytes {
		return false
	}
	seen := map[string]bool{}
	total := 0
	for _, annotation := range page.Annotations {
		if seen[annotation.Anchor] || !utf8.ValidString(annotation.Source) || !utf8.ValidString(annotation.Title) {
			return false
		}
		seen[annotation.Anchor] = true
		size := len(annotation.Source) + len(annotation.Title)
		if size > catalogschema.ReaderRichMaxPayloadBytes || size > catalogschema.ReaderRichTotalPayloadBytes-total || len(annotation.Title) > 512 {
			return false
		}
		total += size
		for _, anchor := range annotation.PageAnchors {
			if !nodes[anchor] {
				return false
			}
		}
	}
	return true
}
