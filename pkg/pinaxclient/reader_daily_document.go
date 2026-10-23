package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type ReaderDailyDocumentRequest struct {
	Date     string
	Timezone string
	Document ReaderDocumentRequest
}

func (c *Client) ReaderDailyDocument(ctx context.Context, req ReaderDailyDocumentRequest) (ReaderDocumentPage, error) {
	var empty ReaderDocumentPage
	doc := req.Document
	if doc.Cursor != "" && doc.Anchor != "" {
		return empty, newError(CodeRequestInvalid, "Daily cursor and anchor are mutually exclusive", nil)
	}
	values := url.Values{"date": {req.Date}, "timezone": {req.Timezone}, "object_id": {doc.ObjectID}, "revision": {doc.Revision}}
	readerSetIfPresent(values, "cursor", doc.Cursor)
	readerSetIfPresent(values, "anchor", doc.Anchor)
	readerSetIfPresent(values, "outline_cursor", doc.OutlineCursor)
	request := map[string]any{}
	for key, list := range values {
		request[key] = list[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderDailyDocumentRequestV1Alpha1, request) != nil {
		return empty, newError(CodeRequestInvalid, "Daily document request violated its schema", nil)
	}
	projection, err := c.getReaderProjection(ctx, "/v1/reader/journals/daily/document", values)
	if err != nil {
		return empty, err
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(projection.Data, &data) != nil || len(data) != 1 {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily document envelope is invalid", nil)
	}
	var raw any
	if json.Unmarshal(data["document"], &raw) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderDocumentV1Alpha1, raw) != nil {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily document violated its schema", nil)
	}
	var page ReaderDocumentPage
	if json.Unmarshal(data["document"], &page) != nil || page.ObjectID != doc.ObjectID || page.Revision != doc.Revision || page.CurrentRevision != doc.Revision || page.URI != "pinax://note/"+doc.ObjectID+"?v="+doc.Revision {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily document identity or revision did not match", nil)
	}
	return page, nil
}
