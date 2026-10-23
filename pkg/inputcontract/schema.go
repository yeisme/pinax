package inputcontract

// AttachmentRequestSchema is the canonical closed MCP/SDK request schema.
func AttachmentRequestSchema() map[string]any {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"input_request_id": text(128), "operation_id": text(128), "object_id": text(256),
			"revision":          map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			"placement":         map[string]any{"type": "string", "enum": []string{"per-note", "vault-folder", "note-folder"}},
			"replace_reference": text(2048),
		}, "required": []string{"input_request_id", "operation_id", "object_id", "revision", "placement"}}
}

func AttachmentProposalSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "string", "enum": []string{AttachmentSchema}},
			"request":        AttachmentRequestSchema(),
			"preview_digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			"reference":      map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
			"mime":           map[string]any{"type": "string", "enum": []string{"text/plain", "text/markdown", "image/png", "image/jpeg", "image/webp", "application/pdf"}},
			"size":           map[string]any{"type": "integer", "minimum": 1, "maximum": 2 << 20},
			"sha256":         map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		}, "required": []string{"schema_version", "request", "preview_digest", "reference", "mime", "size", "sha256"}}
}

func AttachmentApplySchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"proposal": AttachmentProposalSchema(), "confirm": map[string]any{"type": "boolean", "enum": []any{true}}},
		"required":   []string{"proposal", "confirm"}}
}
