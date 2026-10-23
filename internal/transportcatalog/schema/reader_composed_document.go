package schema

const (
	ReaderComposedRequestV1Alpha1  = "pinax.reader.composed_document.request.v1alpha1"
	ReaderEmbedTargetV1Alpha1      = "pinax.reader.embed_target.v1alpha1"
	ReaderEmbedV1Alpha1            = "pinax.reader.embed.v1alpha1"
	ReaderNodeOriginV1Alpha1       = "pinax.reader.node_origin.v1alpha1"
	ReaderComposedDocumentV1Alpha1 = "pinax.reader.composed_document.v1alpha1"
	ReaderComposedMaxEmbeds        = 32
)

// Schema registration does not advertise a transport or authorize expansion.
func readerComposedDefinitions() []Definition {
	anchor := map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderAnchorMaxLength}
	parent := map[string]any{"type": "string", "maxLength": ReaderAnchorMaxLength}
	pageAnchors := map[string]any{"type": "array", "minItems": 1, "maxItems": ReaderDocumentPageMaxNodes, "uniqueItems": true, "items": anchor}
	edge := func(status map[string]any, maxDepth int, withTarget bool) map[string]any {
		props := map[string]any{"embed_id": anchor, "parent_embed_id": parent, "source_anchor": anchor, "page_anchors": pageAnchors, "depth": map[string]any{"type": "integer", "minimum": 1, "maximum": maxDepth}, "status": status}
		required := []string{"embed_id", "parent_embed_id", "source_anchor", "page_anchors", "depth", "status"}
		if withTarget {
			props["target"] = refValue(ReaderEmbedTargetV1Alpha1)
			required = append(required, "target")
		}
		return closedObject(props, required...)
	}
	depthLimit := edge(constString("depth_limit"), 3, false)
	depthLimit["properties"].(map[string]any)["depth"] = constInteger(3)
	return []Definition{
		request(ReaderComposedRequestV1Alpha1, map[string]any{"object_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "revision": writerDigestValue(), "cursor": readerCursorValue(), "anchor": anchor, "outline_cursor": readerCursorValue()}, "object_id", "revision"),
		define(ReaderEmbedTargetV1Alpha1, closedObject(map[string]any{"object_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "revision": writerDigestValue(), "anchor": anchor}, "object_id", "revision")),
		define(ReaderEmbedV1Alpha1, map[string]any{"oneOf": []any{edge(enumValue("expanded", "cycle"), 2, true), edge(enumValue("unavailable", "unsupported"), 2, false), depthLimit}}),
		define(ReaderNodeOriginV1Alpha1, closedObject(map[string]any{"page_anchor": anchor, "embed_id": parent, "source_anchor": anchor}, "page_anchor", "embed_id", "source_anchor")),
		define(ReaderComposedDocumentV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(ReaderComposedDocumentV1Alpha1),
			"rich_document":  refValue(ReaderRichDocumentV1Alpha1),
			"embeds":         map[string]any{"type": "array", "maxItems": ReaderComposedMaxEmbeds, "items": refValue(ReaderEmbedV1Alpha1)},
			"origins":        map[string]any{"type": "array", "maxItems": ReaderDocumentPageMaxNodes, "items": refValue(ReaderNodeOriginV1Alpha1)},
		}, "schema_version", "rich_document", "embeds", "origins")),
	}
}
