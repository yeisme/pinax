package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	schema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type ReaderEmbedTarget struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
	Anchor   string `json:"anchor,omitempty"`
}
type ReaderEmbed struct {
	EmbedID       string             `json:"embed_id"`
	ParentEmbedID string             `json:"parent_embed_id"`
	SourceAnchor  string             `json:"source_anchor"`
	PageAnchors   []string           `json:"page_anchors"`
	Depth         int                `json:"depth"`
	Status        string             `json:"status"`
	Target        *ReaderEmbedTarget `json:"target,omitempty"`
}
type ReaderNodeOrigin struct {
	PageAnchor   string `json:"page_anchor"`
	EmbedID      string `json:"embed_id"`
	SourceAnchor string `json:"source_anchor"`
}
type ReaderComposedDocumentPage struct {
	SchemaVersion string                 `json:"schema_version"`
	RichDocument  ReaderRichDocumentPage `json:"rich_document"`
	Embeds        []ReaderEmbed          `json:"embeds"`
	Origins       []ReaderNodeOrigin     `json:"origins"`
}

// ReaderComposedDocument returns inert, provenance-bound page data. It does
// not grant asset access, execute annotations or reconstruct writable source.
func (c *Client) ReaderComposedDocument(ctx context.Context, req ReaderDocumentRequest) (ReaderComposedDocumentPage, error) {
	var empty ReaderComposedDocumentPage
	values := url.Values{"revision": {req.Revision}}
	readerSetIfPresent(values, "cursor", req.Cursor)
	readerSetIfPresent(values, "anchor", req.Anchor)
	readerSetIfPresent(values, "outline_cursor", req.OutlineCursor)
	payload := map[string]any{"object_id": req.ObjectID}
	for key, items := range values {
		payload[key] = items[0]
	}
	if schema.ValidatePayload(schema.Default(), schema.ReaderComposedRequestV1Alpha1, payload) != nil || req.Cursor != "" && req.Anchor != "" || strings.ContainsAny(req.ObjectID, "/\\?#\x00") || !utf8.ValidString(req.ObjectID) {
		return empty, newError(CodeRequestInvalid, "Composed document requires a fixed identity and valid position", nil)
	}
	bounded := *c
	if bounded.maxResponseBytes > schema.ReaderDocumentResponseMaxBytes {
		bounded.maxResponseBytes = schema.ReaderDocumentResponseMaxBytes
	}
	projection, err := bounded.getReaderProjection(ctx, "/v1/reader/notes/"+url.PathEscape(req.ObjectID)+"/composed-document", values)
	if err != nil {
		return empty, err
	}
	bad := func() (ReaderComposedDocumentPage, error) {
		return empty, newError(CodeUpstreamInvalidResponse, "Composed document provenance or budget is inconsistent", nil)
	}
	var raw any
	if projection.Command != "reader.composed_document" || json.Unmarshal(projection.Data, &raw) != nil || schema.ValidatePayload(schema.Default(), schema.ReaderComposedDocumentV1Alpha1, raw) != nil {
		return bad()
	}
	var page ReaderComposedDocumentPage
	if json.Unmarshal(projection.Data, &page) != nil || !validRichClientPage(page.RichDocument, req) || !validComposedClientPage(page) {
		return bad()
	}
	return page, nil
}

func validComposedClientPage(page ReaderComposedDocumentPage) bool {
	nodes := map[string]ReaderDocumentNode{}
	var walk func([]ReaderDocumentNode)
	walk = func(ns []ReaderDocumentNode) {
		for _, n := range ns {
			nodes[n.Anchor] = n
			walk(n.Children)
		}
	}
	walk(page.RichDocument.Document.Nodes)
	origins := map[string]ReaderNodeOrigin{}
	for _, o := range page.Origins {
		if _, ok := nodes[o.PageAnchor]; !ok {
			return false
		}
		if _, ok := origins[o.PageAnchor]; ok {
			return false
		}
		if !utf8.ValidString(o.SourceAnchor) || strings.ContainsRune(o.SourceAnchor, 0) {
			return false
		}
		origins[o.PageAnchor] = o
	}
	if len(origins) != len(nodes) {
		return false
	}
	edges := map[string]ReaderEmbed{}
	for _, e := range page.Embeds {
		if _, ok := edges[e.EmbedID]; ok {
			return false
		}
		edges[e.EmbedID] = e
		if e.Target != nil && (!validSHA256Digest(e.Target.Revision) || !utf8.ValidString(e.Target.ObjectID) || strings.ContainsAny(e.Target.ObjectID, "/\\?#\x00")) {
			return false
		}
	}
	for _, e := range page.Embeds {
		if e.ParentEmbedID == "" {
			if e.Depth != 1 {
				return false
			}
		} else {
			parent, ok := edges[e.ParentEmbedID]
			if !ok || parent.Status != "expanded" || e.Depth != parent.Depth+1 {
				return false
			}
		}
		// An expanded target cannot be an ancestor; a cycle must be exactly one.
		if e.Target != nil {
			cyclic := e.Target.ObjectID == page.RichDocument.Document.ObjectID
			revision := page.RichDocument.Document.Revision
			parent := e.ParentEmbedID
			for hops := 0; parent != ""; hops++ {
				if hops >= 2 {
					return false
				}
				p, ok := edges[parent]
				if !ok || p.Target == nil {
					return false
				}
				if p.Target.ObjectID == e.Target.ObjectID {
					cyclic = true
					revision = p.Target.Revision
				}
				parent = p.ParentEmbedID
			}
			if (e.Status == "cycle") != cyclic || cyclic && e.Target.Revision != revision {
				return false
			}
		}
		for _, a := range e.PageAnchors {
			o, ok := origins[a]
			if !ok {
				return false
			}
			// Placeholder belongs to the parent source; expanded descendants belong
			// to this occurrence or its children. Arbitrary root nodes cannot be claimed.
			belongs := o.EmbedID == e.ParentEmbedID && o.SourceAnchor == e.SourceAnchor
			for id, hops := o.EmbedID, 0; id != ""; hops++ {
				if hops >= 3 {
					return false
				}
				if id == e.EmbedID {
					belongs = true
					break
				}
				p, ok := edges[id]
				if !ok {
					return false
				}
				id = p.ParentEmbedID
			}
			if !belongs {
				return false
			}
		}
	}
	for _, o := range origins {
		if o.EmbedID != "" {
			e, ok := edges[o.EmbedID]
			if !ok || e.Status != "expanded" {
				return false
			}
			// Every containing occurrence must explicitly claim this page node.
			for id, hops := o.EmbedID, 0; id != ""; hops++ {
				if hops >= 2 {
					return false
				}
				e, ok := edges[id]
				if !ok {
					return false
				}
				found := false
				for _, a := range e.PageAnchors {
					found = found || a == o.PageAnchor
				}
				if !found {
					return false
				}
				id = e.ParentEmbedID
			}
		}
	}
	return true
}
