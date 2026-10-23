package schema

const (
	ReaderRichDocumentRequestV1Alpha1 = "pinax.reader.rich_document.request.v1alpha1"
	ReaderRichAnnotationV1Alpha1      = "pinax.reader.rich_annotation.v1alpha1"
	ReaderRichDocumentV1Alpha1        = "pinax.reader.rich_document.v1alpha1"
	ReaderRichMaxAnnotations          = 512
	ReaderRichMaxPayloadBytes         = 64 * 1024
	ReaderRichTotalPayloadBytes       = 256 * 1024
)

func readerRichDefinitions() []Definition {
	anchor := map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderAnchorMaxLength}
	pageAnchors := map[string]any{"type": "array", "minItems": 1, "maxItems": ReaderDocumentPageMaxNodes, "uniqueItems": true, "items": anchor}
	code := closedObject(map[string]any{"anchor": anchor, "page_anchors": pageAnchors, "kind": enumValue("mermaid", "math"), "source": map[string]any{"type": "string", "maxLength": ReaderRichMaxPayloadBytes}}, "anchor", "page_anchors", "kind")
	callout := closedObject(map[string]any{
		"anchor": anchor, "page_anchors": pageAnchors, "kind": constString("callout"),
		"callout_kind": enumValue("note", "abstract", "summary", "tldr", "info", "todo", "tip", "hint", "important", "success", "check", "done", "question", "help", "faq", "warning", "caution", "attention", "failure", "fail", "missing", "danger", "error", "bug", "example", "quote", "cite"),
		"title":        map[string]any{"type": "string", "maxLength": 512}, "fold": enumValue("fixed", "expanded", "collapsed"),
	}, "anchor", "page_anchors", "kind", "callout_kind", "fold")
	return []Definition{
		request(ReaderRichDocumentRequestV1Alpha1, map[string]any{"object_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "revision": writerDigestValue(), "cursor": readerCursorValue(), "anchor": anchor, "outline_cursor": readerCursorValue()}, "object_id", "revision"),
		define(ReaderRichAnnotationV1Alpha1, map[string]any{"oneOf": []any{code, callout}}),
		define(ReaderRichDocumentV1Alpha1, closedObject(map[string]any{"schema_version": constString(ReaderRichDocumentV1Alpha1), "document": refValue(ReaderDocumentV1Alpha1), "annotations": map[string]any{"type": "array", "maxItems": ReaderRichMaxAnnotations, "items": refValue(ReaderRichAnnotationV1Alpha1)}}, "schema_version", "document", "annotations")),
	}
}
