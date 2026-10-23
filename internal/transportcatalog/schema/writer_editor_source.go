package schema

const (
	WriterEditorSourceV1Alpha1        = "pinax.writer.editor_source.v1alpha1"
	WriterEditorSourceRequestV1Alpha1 = "pinax.writer.editor_source.request.v1alpha1"
)

func writerEditorSourceDefinitions() []Definition {
	return []Definition{
		define(WriterEditorSourceRequestV1Alpha1, closedObject(map[string]any{
			"object_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
			"revision":  writerDigestValue(),
		}, "object_id", "revision")),
		define(WriterEditorSourceV1Alpha1, closedObject(map[string]any{
			"schema_version":        constString(WriterEditorSourceV1Alpha1),
			"object_id":             map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
			"revision":              writerDigestValue(),
			"body":                  map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes},
			"body_digest":           writerDigestValue(),
			"editable":              boolValue(),
			"readonly_reason":       enumValue("none", "frontmatter_unsupported"),
			"frontmatter_protected": map[string]any{"type": "boolean", "const": true},
			"protected_metadata": closedObject(map[string]any{
				"note_id": stringValue(), "title": stringValue(), "tags": stringArrayValue(),
				"status": stringValue(), "kind": stringValue(),
			}, "note_id", "title", "tags", "status", "kind"),
			"budget": closedObject(map[string]any{
				"max_source_bytes": map[string]any{"type": "integer", "const": WriterMaxBodyBytes},
				"max_body_bytes":   map[string]any{"type": "integer", "const": WriterMaxBodyBytes},
				"source_bytes":     map[string]any{"type": "integer", "minimum": 0, "maximum": WriterMaxBodyBytes},
				"body_bytes":       map[string]any{"type": "integer", "minimum": 0, "maximum": WriterMaxBodyBytes},
			}, "max_source_bytes", "max_body_bytes", "source_bytes", "body_bytes"),
		}, "schema_version", "object_id", "revision", "body", "body_digest", "editable", "readonly_reason", "frontmatter_protected", "protected_metadata", "budget")),
	}
}
