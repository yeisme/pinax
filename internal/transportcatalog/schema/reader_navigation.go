package schema

const (
	ReaderDailyDocumentRequestV1Alpha1 = "pinax.reader.daily_document.request.v1alpha1"
	ReaderTagsRequestV1Alpha1          = "pinax.reader.tags.request.v1alpha1"
	ReaderTagsV1Alpha1                 = "pinax.reader.tags.v1alpha1"
	ReaderGraphRequestV1Alpha1         = "pinax.reader.graph.request.v1alpha1"
	ReaderGraphV1Alpha1                = "pinax.reader.graph.v1alpha1"
	ReaderDailyRequestV1Alpha1         = "pinax.reader.daily.request.v1alpha1"
	ReaderDailyV1Alpha1                = "pinax.reader.daily.v1alpha1"
	ReaderGraphMaxNodes                = 200
	ReaderGraphMaxEdges                = 400
)

func readerNavigationDefinitions() []Definition {
	nonempty := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	count := func() map[string]any { return map[string]any{"type": "integer", "minimum": 0} }
	identity := func() map[string]any {
		return closedObject(map[string]any{
			"object_id": nonempty(256), "revision": writerDigestValue(), "title": stringValue(),
		}, "object_id", "revision", "title")
	}
	coverage := func() map[string]any {
		return closedObject(map[string]any{
			"page_complete": boolValue(), "corpus_complete": boolValue(),
		}, "page_complete", "corpus_complete")
	}
	nullableIdentity := identity()
	nullableIdentity["type"] = []any{"object", "null"}
	return []Definition{
		define(ReaderDailyDocumentRequestV1Alpha1, closedObject(map[string]any{
			"date": map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}, "timezone": nonempty(128), "object_id": nonempty(256), "revision": writerDigestValue(), "cursor": readerCursorValue(), "anchor": nonempty(ReaderAnchorMaxLength), "outline_cursor": readerCursorValue(),
		}, "date", "timezone", "object_id", "revision")),
		define(ReaderTagsRequestV1Alpha1, closedObject(map[string]any{"limit": readerLimitValue(), "cursor": readerCursorValue()})),
		define(ReaderTagsV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(ReaderTagsV1Alpha1), "binding_id": nonempty(256),
			"items": map[string]any{"type": "array", "maxItems": ReaderListMaxLimit, "items": closedObject(map[string]any{
				"tag_id": nonempty(80), "path": nonempty(512), "parent_id": stringValue(), "virtual": boolValue(), "direct_count": count(), "descendant_count": count(),
			}, "tag_id", "path", "parent_id", "virtual", "direct_count", "descendant_count")},
			"next_cursor": readerCursorValue(), "watermark": nonempty(128), "observed_at": dateTimeValue(), "total_tags": count(), "coverage": coverage(),
		}, "schema_version", "binding_id", "items", "watermark", "observed_at", "total_tags", "coverage")),
		define(ReaderGraphRequestV1Alpha1, closedObject(map[string]any{
			"mode": enumValue("source", "local"), "object_id": nonempty(256), "revision": writerDigestValue(),
			"hops":  map[string]any{"type": "integer", "minimum": 1, "maximum": 2},
			"limit": map[string]any{"type": "integer", "minimum": 2, "maximum": ReaderGraphMaxNodes, "default": ReaderGraphMaxNodes}, "cursor": readerCursorValue(),
		}, "mode")),
		define(ReaderGraphV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(ReaderGraphV1Alpha1), "binding_id": nonempty(256), "mode": enumValue("source", "local"),
			"object_id": stringValue(), "revision": stringValue(), "hops": map[string]any{"type": "integer", "minimum": 0, "maximum": 2},
			"nodes": map[string]any{"type": "array", "maxItems": ReaderGraphMaxNodes, "items": identity()},
			"edges": map[string]any{"type": "array", "maxItems": ReaderGraphMaxEdges, "items": closedObject(map[string]any{
				"source_id": nonempty(256), "target_id": nonempty(256),
			}, "source_id", "target_id")},
			"next_cursor": readerCursorValue(), "watermark": nonempty(128), "observed_at": dateTimeValue(), "total_nodes": count(), "total_edges": count(), "coverage": coverage(),
		}, "schema_version", "binding_id", "mode", "object_id", "revision", "hops", "nodes", "edges", "watermark", "observed_at", "total_nodes", "total_edges", "coverage")),
		define(ReaderDailyRequestV1Alpha1, closedObject(map[string]any{
			"date": map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}, "timezone": nonempty(128),
		}, "date", "timezone")),
		define(ReaderDailyV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(ReaderDailyV1Alpha1), "binding_id": nonempty(256), "date": nonempty(10), "timezone": nonempty(128),
			"status": enumValue("found", "missing"), "note": nullableIdentity,
			"watermark": nonempty(128), "observed_at": dateTimeValue(),
		}, "schema_version", "binding_id", "date", "timezone", "status", "note", "watermark", "observed_at")),
	}
}
