package schema

// Reader-facing machine contracts frozen by pinax-human-reading-api-v1 P01.
// The human-readable source of truth is docs/interfaces/human-reading-api.md;
// these schemas encode its routes payload shapes, budgets, scopes and the
// supervised local-session bootstrap frame. They stay additive: legacy Agent
// projection schemas above are untouched, and reader bindings must not be
// marked available before a real adapter exists.
const (
	ReaderSourceV1Alpha1        = "pinax.reader.source.v1alpha1"
	ReaderNoteCardV1Alpha1      = "pinax.reader.note_card.v1alpha1"
	ReaderNoteListV1Alpha1      = "pinax.reader.note_list.v1alpha1"
	ReaderSearchSnippetV1Alpha1 = "pinax.reader.search_snippet.v1alpha1"
	ReaderSearchResultsV1Alpha1 = "pinax.reader.search_results.v1alpha1"
	ReaderOutlineEntryV1Alpha1  = "pinax.reader.outline_entry.v1alpha1"
	ReaderDocumentNodeV1Alpha1  = "pinax.reader.document_node.v1alpha1"
	ReaderDocumentV1Alpha1      = "pinax.reader.document.v1alpha1"
	ReaderLinkItemV1Alpha1      = "pinax.reader.link_item.v1alpha1"
	ReaderLinkPageV1Alpha1      = "pinax.reader.link_page.v1alpha1"
	ReaderAssetRefV1Alpha1      = "pinax.reader.asset_ref.v1alpha1"
	ReaderSourceExportV1Alpha1  = "pinax.reader.source_export.v1alpha1"

	ReaderSourceRequestV1Alpha1       = "pinax.reader.source.request.v1alpha1"
	ReaderNotesRequestV1Alpha1        = "pinax.reader.notes.request.v1alpha1"
	ReaderSearchRequestV1Alpha1       = "pinax.reader.search.request.v1alpha1"
	ReaderDocumentRequestV1Alpha1     = "pinax.reader.document.request.v1alpha1"
	ReaderLinksRequestV1Alpha1        = "pinax.reader.links.request.v1alpha1"
	ReaderAssetContentRequestV1Alpha1 = "pinax.reader.asset_content.request.v1alpha1"
	ReaderSourceExportRequestV1Alpha1 = "pinax.reader.source_export.request.v1alpha1"

	LocalSessionBootstrapV1Alpha1 = "pinax.local_session.bootstrap.v1alpha1"
	LocalSessionEventV1Alpha1     = "pinax.local_session.event.v1alpha1"
)

// Reader scope vocabulary. Scopes are granted only by the owner; an
// audience=human request field never upgrades an Agent token.
const (
	ReaderScopeMetadata  = "reader.metadata"
	ReaderScopeBody      = "reader.body"
	ReaderScopeRelations = "reader.relations"
	ReaderScopeAssets    = "reader.assets"
)

// Budget constants from the human reading contract. They are frozen as
// schema consts so any drift fails contract fixtures and the manifest digest.
const (
	ReaderListDefaultLimit            = 50
	ReaderListMaxLimit                = 200
	ReaderSnippetMaxChars             = 320
	ReaderOutlinePageMaxEntries       = 512
	ReaderDocumentPageMaxTextBytes    = 256 * 1024
	ReaderDocumentPageMaxNodes        = 512
	ReaderDocumentResponseMaxBytes    = 1024 * 1024
	ReaderDocumentSourceMaxBytes      = 8 * 1024 * 1024
	ReaderDocumentTotalMaxNodes       = 65536
	ReaderAssetRangeMaxBytes          = 8 * 1024 * 1024
	ReaderSourceExportRangeMaxBytes   = 1024 * 1024
	ReaderQueryMaxChars               = 512
	ReaderCursorMaxLength             = 2048
	ReaderAnchorMaxLength             = 512
	ReaderRevisionMaxLength           = 128
	ReaderSessionBootstrapMaxFrame    = 16 * 1024
	ReaderSessionBootstrapMaxDeadline = 15
)

func readerScopeValue() map[string]any {
	return enumValue(ReaderScopeMetadata, ReaderScopeBody, ReaderScopeRelations, ReaderScopeAssets)
}

// sessionScopeValue is the supervised session allowlist: reader scopes plus
// the pinax-human-reading-write-v1 writer extension slot. The readonly /
// writer-scope correlation is enforced by the bootstrap reader in code.
func sessionScopeValue() map[string]any {
	return enumValue(
		ReaderScopeMetadata, ReaderScopeBody, ReaderScopeRelations, ReaderScopeAssets,
		WriterScopeCapture, WriterScopeEditor, WriterScopeOrganizer,
	)
}

func readerLimitsValue() map[string]any {
	return closedObject(map[string]any{
		"list_default_limit":            constInteger(ReaderListDefaultLimit),
		"list_max_limit":                constInteger(ReaderListMaxLimit),
		"snippet_max_chars":             constInteger(ReaderSnippetMaxChars),
		"outline_page_max_entries":      constInteger(ReaderOutlinePageMaxEntries),
		"document_page_max_text_bytes":  constInteger(ReaderDocumentPageMaxTextBytes),
		"document_page_max_nodes":       constInteger(ReaderDocumentPageMaxNodes),
		"document_response_max_bytes":   constInteger(ReaderDocumentResponseMaxBytes),
		"document_source_max_bytes":     constInteger(ReaderDocumentSourceMaxBytes),
		"document_total_max_nodes":      constInteger(ReaderDocumentTotalMaxNodes),
		"asset_range_max_bytes":         constInteger(ReaderAssetRangeMaxBytes),
		"source_export_range_max_bytes": constInteger(ReaderSourceExportRangeMaxBytes),
	})
}

func readerCursorValue() map[string]any {
	return map[string]any{"type": "string", "maxLength": ReaderCursorMaxLength}
}

func readerLimitValue() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": ReaderListMaxLimit, "default": ReaderListDefaultLimit}
}

// readerDefinitions returns the machine contracts for the planned human
// reading surface. All objects are closed; unknown fields are rejected.
func readerDefinitions() []Definition {
	return []Definition{
		define(ReaderSourceV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(ReaderSourceV1Alpha1),
			"session_id":     stringValue(),
			"binding_id":     stringValue(),
			"display_name":   stringValue(),
			"capabilities":   arrayValue(readerScopeValue()),
			"limits":         readerLimitsValue(),
			"index_state": closedObject(map[string]any{
				"status":     enumValue("ready", "partial", "stale", "index_unavailable"),
				"watermark":  stringValue(),
				"generation": map[string]any{"type": "integer", "minimum": 0},
			}, "status"),
			"observed_at": dateTimeValue(),
		}, "schema_version", "session_id", "binding_id", "display_name", "capabilities", "limits", "index_state", "observed_at")),
		define(ReaderNoteCardV1Alpha1, closedObject(map[string]any{
			"object_id":   stringValue(),
			"title":       stringValue(),
			"title_state": map[string]any{"type": "string", "enum": []any{"present", "unavailable"}, "default": "present"},
			"revision":    stringValue(),
			"uri":         stringValue(),
			"status":      stringValue(),
			"project_id":  map[string]any{"type": []any{"string", "null"}},
			"tags":        stringArrayValue(),
			"updated_at":  dateTimeValue(),
		}, "object_id", "title", "revision", "uri", "status", "tags", "updated_at")),
		define(ReaderNoteListV1Alpha1, closedObject(map[string]any{
			"items":           arrayValue(refValue(ReaderNoteCardV1Alpha1)),
			"next_cursor":     readerCursorValue(),
			"observed_at":     dateTimeValue(),
			"index_watermark": stringValue(),
			"index_state": closedObject(map[string]any{
				"status":     enumValue("ready", "partial", "stale", "index_unavailable"),
				"watermark":  stringValue(),
				"generation": map[string]any{"type": "integer", "minimum": 0},
			}, "status"),
		}, "items", "observed_at", "index_watermark", "index_state")),
		define(ReaderSearchSnippetV1Alpha1, closedObject(map[string]any{
			"object_id": stringValue(),
			"snippet":   map[string]any{"type": "string", "maxLength": ReaderSnippetMaxChars},
		}, "object_id", "snippet")),
		define(ReaderSearchResultsV1Alpha1, closedObject(map[string]any{
			"items":           arrayValue(refValue(ReaderNoteCardV1Alpha1)),
			"snippets":        arrayValue(refValue(ReaderSearchSnippetV1Alpha1)),
			"next_cursor":     readerCursorValue(),
			"observed_at":     dateTimeValue(),
			"index_watermark": stringValue(),
			"index_state": closedObject(map[string]any{
				"status":     enumValue("ready", "partial", "stale", "index_unavailable"),
				"watermark":  stringValue(),
				"generation": map[string]any{"type": "integer", "minimum": 0},
			}, "status"),
		}, "items", "snippets", "observed_at", "index_watermark", "index_state")),
		define(ReaderOutlineEntryV1Alpha1, closedObject(map[string]any{
			"anchor": stringValue(),
			"title":  stringValue(),
			"level":  map[string]any{"type": "integer", "minimum": 1, "maximum": 6},
		}, "anchor", "title", "level")),
		define(ReaderDocumentNodeV1Alpha1, closedObject(map[string]any{
			"type": enumValue(
				"paragraph", "heading", "list", "quote", "code", "table",
				"image", "link", "continuation", "unsupported", "fence", "fence_failed",
			),
			"fence":          stringValue(),
			"payload":        openObject(),
			"error_code":     stringValue(),
			"next_action":    stringValue(),
			"anchor":         map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderAnchorMaxLength},
			"text":           stringValue(),
			"children":       arrayValue(refValue(ReaderDocumentNodeV1Alpha1)),
			"ref":            stringValue(),
			"language":       stringValue(),
			"alt":            stringValue(),
			"block_anchor":   map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderAnchorMaxLength},
			"fragment_index": map[string]any{"type": "integer", "minimum": 0},
			"last_fragment":  boolValue(),
			"original_type":  enumValue("paragraph", "heading", "list", "quote", "code", "table"),
			"header":         stringArrayValue(),
		}, "type", "anchor")),
		define(ReaderDocumentV1Alpha1, closedObject(map[string]any{
			"schema_version":   constString(ReaderDocumentV1Alpha1),
			"object_id":        stringValue(),
			"revision":         stringValue(),
			"current_revision": stringValue(),
			"uri":              stringValue(),
			"nodes":            arrayValue(refValue(ReaderDocumentNodeV1Alpha1)),
			"outline":          arrayValue(refValue(ReaderOutlineEntryV1Alpha1)),
			"outline_cursor":   readerCursorValue(),
			"next_cursor":      readerCursorValue(),
			"complete":         boolValue(),
			"content_bytes":    map[string]any{"type": "integer", "minimum": 0},
			"observed_at":      dateTimeValue(),
			"warnings":         arrayValue(refValue(projectionWarningV1)),
		}, "schema_version", "object_id", "revision", "current_revision", "uri", "nodes", "outline", "complete", "content_bytes", "observed_at")),
		define(ReaderLinkItemV1Alpha1, closedObject(map[string]any{
			"object_id": stringValue(),
			"revision":  stringValue(),
			"anchor":    stringValue(),
		}, "object_id")),
		define(ReaderLinkPageV1Alpha1, closedObject(map[string]any{
			"object_id":           stringValue(),
			"revision":            stringValue(),
			"direction":           enumValue("outgoing", "backlinks"),
			"items":               arrayValue(refValue(ReaderLinkItemV1Alpha1)),
			"next_cursor":         readerCursorValue(),
			"relations_watermark": stringValue(),
			"observed_at":         dateTimeValue(),
		}, "object_id", "revision", "direction", "items", "relations_watermark", "observed_at")),
		define(ReaderAssetRefV1Alpha1, closedObject(map[string]any{
			"asset_ref":    stringValue(),
			"mime":         stringValue(),
			"size_bytes":   map[string]any{"type": "integer", "minimum": 0},
			"digest":       stringValue(),
			"availability": enumValue("available", "unavailable"),
		}, "asset_ref", "mime", "size_bytes", "digest", "availability")),
		define(ReaderSourceExportV1Alpha1, closedObject(map[string]any{
			"object_id":  stringValue(),
			"revision":   stringValue(),
			"media_type": constString("text/markdown; charset=utf-8"),
			"size_bytes": map[string]any{"type": "integer", "minimum": 0},
			"digest":     stringValue(),
		}, "object_id", "revision", "media_type", "size_bytes", "digest")),
		request(ReaderSourceRequestV1Alpha1, nil),
		request(ReaderNotesRequestV1Alpha1, map[string]any{
			"project_id": stringValue(),
			"tag":        map[string]any{"type": "array", "items": stringValue(), "maxItems": 16, "description": "Multiple tags combine with AND semantics."},
			"status":     stringValue(),
			"sort":       map[string]any{"type": "string", "enum": []any{"updated_desc", "created_desc", "title_asc"}, "default": "updated_desc"},
			"limit":      readerLimitValue(),
			"cursor":     readerCursorValue(),
		}),
		request(ReaderSearchRequestV1Alpha1, map[string]any{
			"q":          map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderQueryMaxChars, "description": "Trimmed before validation; keyword search only, never logged."},
			"project_id": stringValue(),
			"tag":        map[string]any{"type": "array", "items": stringValue(), "maxItems": 16},
			"status":     stringValue(),
			"sort":       map[string]any{"type": "string", "enum": []any{"relevance_desc"}},
			"limit":      readerLimitValue(),
			"cursor":     readerCursorValue(),
		}, "q"),
		request(ReaderDocumentRequestV1Alpha1, map[string]any{
			"object_id":      stringValue(),
			"revision":       map[string]any{"type": "string", "maxLength": ReaderRevisionMaxLength},
			"cursor":         readerCursorValue(),
			"anchor":         map[string]any{"type": "string", "minLength": 1, "maxLength": ReaderAnchorMaxLength, "description": "cursor and anchor are mutually exclusive."},
			"outline_cursor": readerCursorValue(),
		}, "object_id"),
		request(ReaderLinksRequestV1Alpha1, map[string]any{
			"object_id": stringValue(),
			"revision":  map[string]any{"type": "string", "maxLength": ReaderRevisionMaxLength},
			"direction": map[string]any{"type": "string", "enum": []any{"outgoing", "backlinks"}, "default": "outgoing"},
			"limit":     readerLimitValue(),
			"cursor":    readerCursorValue(),
		}, "object_id", "revision"),
		request(ReaderAssetContentRequestV1Alpha1, map[string]any{
			"asset_ref":     stringValue(),
			"note_id":       stringValue(),
			"note_revision": map[string]any{"type": "string", "maxLength": ReaderRevisionMaxLength},
		}, "asset_ref", "note_id", "note_revision"),
		request(ReaderSourceExportRequestV1Alpha1, map[string]any{
			"object_id": stringValue(),
			"revision":  map[string]any{"type": "string", "maxLength": ReaderRevisionMaxLength},
		}, "object_id", "revision"),
		define(LocalSessionEventV1Alpha1, closedObject(map[string]any{
			"schema_version":  constString(LocalSessionEventV1Alpha1),
			"type":            enumValue("ready", "error", "shutdown"),
			"session_id":      stringValue(),
			"session_nonce":   stringValue(),
			"binding_id":      stringValue(),
			"address":         stringValue(),
			"contract_digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			"readonly":        boolValue(),
			"code":            stringValue(),
			"message":         stringValue(),
		}, "schema_version", "type")),
		define(LocalSessionBootstrapV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(LocalSessionBootstrapV1Alpha1),
			"session_nonce":  map[string]any{"type": "string", "minLength": 16, "maxLength": 128, "description": "Handshake correlation label only; never a credential."},
			"bearer":         map[string]any{"type": "string", "minLength": 43, "maxLength": 64, "description": "256-bit random bearer; stays in the private stdin frame and memory only."},
			"binding_id":     stringValue(),
			"vault_root":     map[string]any{"type": "string", "minLength": 1, "description": "Canonical vault root chosen by the trusted host; never echoed to readers."},
			"scopes":         map[string]any{"type": "array", "items": sessionScopeValue(), "minItems": 1, "uniqueItems": true, "description": "Owner-granted reader and writer scopes; writer scopes require readonly=false."},
			"readonly":       boolValue(),
		}, "schema_version", "session_nonce", "bearer", "binding_id", "vault_root", "scopes", "readonly")),
	}
}

func constString(value string) map[string]any {
	return map[string]any{"type": "string", "const": value}
}

func constInteger(value int) map[string]any {
	return map[string]any{"type": "integer", "const": value}
}
