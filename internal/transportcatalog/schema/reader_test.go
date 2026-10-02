package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestReaderContract freezes the human reading machine schemas from
// pinax-human-reading-api-v1 P01. Positive fixtures mirror the human contract
// in docs/interfaces/human-reading-api.md; negative fixtures prove closed
// objects, required fields, enums, lengths and frozen budget consts reject
// drift. Pure contract test: no vault, no model, no sync.
func TestReaderContract(t *testing.T) {
	t.Parallel()

	registry := Default()
	for _, id := range []string{
		ReaderSourceV1Alpha1,
		ReaderNoteCardV1Alpha1,
		ReaderNoteListV1Alpha1,
		ReaderSearchSnippetV1Alpha1,
		ReaderSearchResultsV1Alpha1,
		ReaderOutlineEntryV1Alpha1,
		ReaderDocumentNodeV1Alpha1,
		ReaderDocumentV1Alpha1,
		ReaderLinkItemV1Alpha1,
		ReaderLinkPageV1Alpha1,
		ReaderAssetRefV1Alpha1,
		ReaderSourceExportV1Alpha1,
		ReaderSourceRequestV1Alpha1,
		ReaderNotesRequestV1Alpha1,
		ReaderSearchRequestV1Alpha1,
		ReaderDocumentRequestV1Alpha1,
		ReaderLinksRequestV1Alpha1,
		ReaderAssetContentRequestV1Alpha1,
		ReaderSourceExportRequestV1Alpha1,
		LocalSessionBootstrapV1Alpha1,
		LocalSessionEventV1Alpha1,
	} {
		if !registry.Has(id) {
			t.Fatalf("reader schema %q is not registered in the default registry", id)
		}
	}

	positive := []struct {
		schemaID string
		document string
	}{
		{ReaderSourceV1Alpha1, `{
			"schema_version": "pinax.reader.source.v1alpha1",
			"session_id": "sess-1",
			"binding_id": "binding-7",
			"display_name": "Yeisme Notes",
			"capabilities": ["reader.metadata", "reader.body"],
			"limits": {
				"list_default_limit": 50, "list_max_limit": 200,
				"snippet_max_chars": 320, "outline_page_max_entries": 512,
				"document_page_max_text_bytes": 262144, "document_page_max_nodes": 512,
				"document_response_max_bytes": 1048576, "document_source_max_bytes": 8388608,
				"document_total_max_nodes": 65536, "asset_range_max_bytes": 8388608,
				"source_export_range_max_bytes": 1048576
			},
			"index_state": {"status": "stale", "watermark": "w-42", "generation": 3},
			"observed_at": "2026-10-01T12:00:00Z"
		}`},
		{ReaderNoteCardV1Alpha1, `{
			"object_id": "obj-1", "title": "设计笔记", "title_state": "present",
			"revision": "sha256:aa", "uri": "pinax://note/obj-1?v=sha256:aa",
			"status": "active", "project_id": null, "tags": ["design"],
			"updated_at": "2026-09-30T08:00:00Z"
		}`},
		{ReaderNoteCardV1Alpha1, `{
			"object_id": "obj-2", "title": "", "title_state": "unavailable",
			"revision": "sha256:bb", "uri": "pinax://note/obj-2?v=sha256:bb",
			"status": "inbox", "project_id": "proj-1", "tags": [],
			"updated_at": "2026-09-30T09:00:00Z"
		}`},
		{ReaderNoteListV1Alpha1, `{
			"items": [{"object_id": "obj-1", "title": "t", "revision": "r",
				"uri": "pinax://note/obj-1?v=r", "status": "active", "tags": [],
				"updated_at": "2026-09-30T08:00:00Z"}],
			"next_cursor": "cursor-2",
			"observed_at": "2026-10-01T12:00:00Z",
			"index_watermark": "w-42",
			"index_state": {"status": "ready", "watermark": "w-42", "generation": 7}
		}`},
		{ReaderSearchSnippetV1Alpha1, `{"object_id": "obj-1", "snippet": "&lt;b&gt;匹配片段&lt;/b&gt;"}`},
		{ReaderSearchResultsV1Alpha1, `{
			"items": [{"object_id": "obj-1", "title": "t", "revision": "r",
				"uri": "pinax://note/obj-1?v=r", "status": "active", "tags": [],
				"updated_at": "2026-09-30T08:00:00Z"}],
			"snippets": [{"object_id": "obj-1", "snippet": "片段"}],
			"observed_at": "2026-10-01T12:00:00Z",
			"index_watermark": "w-42",
			"index_state": {"status": "ready", "watermark": "w-42", "generation": 7}
		}`},
		{ReaderOutlineEntryV1Alpha1, `{"anchor": "h-1", "title": "概述", "level": 2}`},
		{ReaderDocumentNodeV1Alpha1, `{
			"type": "paragraph", "anchor": "b-1", "text": "中文段落",
			"children": [{"type": "link", "anchor": "b-1-i1", "text": "内链", "ref": "pinax://note/obj-2"}]
		}`},
		{ReaderDocumentNodeV1Alpha1, `{
			"type": "continuation", "anchor": "b-7-f1", "block_anchor": "b-7",
			"fragment_index": 1, "last_fragment": false, "original_type": "code",
			"language": "go", "text": "func main() {}"
		}`},
		{ReaderDocumentNodeV1Alpha1, `{"type": "unsupported", "anchor": "b-9"}`},
		{ReaderDocumentNodeV1Alpha1, `{"type": "fence", "anchor": "fence-1", "fence": "pinax-graph", "payload": {"row_count": 3}}`},
		{ReaderDocumentNodeV1Alpha1, `{"type": "fence_failed", "anchor": "fence-2", "fence": "pinax-timeline", "error_code": "timeline_unavailable", "next_action": "pinax record history <object-id>"}`},
		{ReaderDocumentV1Alpha1, `{
			"schema_version": "pinax.reader.document.v1alpha1",
			"object_id": "obj-1", "revision": "sha256:aa", "current_revision": "sha256:cc",
			"uri": "pinax://note/obj-1?v=sha256:aa",
			"nodes": [{"type": "heading", "anchor": "h-1", "text": "概述"}],
			"outline": [{"anchor": "h-1", "title": "概述", "level": 1}],
			"outline_cursor": "outline-2", "next_cursor": "page-2", "complete": false,
			"content_bytes": 2048, "observed_at": "2026-10-01T12:00:00Z",
			"warnings": [{"code": "inline_token_downgraded", "message": "long inline token shown as text"}]
		}`},
		{ReaderLinkItemV1Alpha1, `{"object_id": "obj-2", "revision": "sha256:bb", "anchor": "b-3"}`},
		{ReaderLinkPageV1Alpha1, `{
			"object_id": "obj-1", "revision": "sha256:aa", "direction": "backlinks",
			"items": [{"object_id": "obj-2", "anchor": "b-3"}],
			"next_cursor": "links-2", "relations_watermark": "rel-9",
			"observed_at": "2026-10-01T12:00:00Z"
		}`},
		{ReaderAssetRefV1Alpha1, `{"asset_ref": "ar-1", "mime": "image/png", "size_bytes": 4096, "digest": "sha256:dd", "availability": "available"}`},
		{ReaderSourceExportV1Alpha1, `{"object_id": "obj-1", "revision": "sha256:aa", "media_type": "text/markdown; charset=utf-8", "size_bytes": 8192, "digest": "sha256:ee"}`},
		{ReaderSourceRequestV1Alpha1, `{}`},
		{ReaderNotesRequestV1Alpha1, `{"project_id": "proj-1", "tag": ["a", "b"], "status": "active", "sort": "title_asc", "limit": 200, "cursor": "c-1"}`},
		{ReaderNotesRequestV1Alpha1, `{}`},
		{ReaderSearchRequestV1Alpha1, `{"q": "中文 关键词", "limit": 50}`},
		{ReaderDocumentRequestV1Alpha1, `{"object_id": "obj-1", "revision": "sha256:aa", "cursor": "page-2"}`},
		{ReaderDocumentRequestV1Alpha1, `{"object_id": "obj-1", "anchor": "b-7"}`},
		{ReaderLinksRequestV1Alpha1, `{"object_id": "obj-1", "revision": "sha256:aa", "direction": "outgoing", "limit": 50}`},
		{ReaderAssetContentRequestV1Alpha1, `{"asset_ref": "ar-1", "note_id": "obj-1", "note_revision": "sha256:aa"}`},
		{ReaderSourceExportRequestV1Alpha1, `{"object_id": "obj-1", "revision": "sha256:aa"}`},
		{LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "ready", "session_id": "rss-1", "session_nonce": "nonce-0123456789abcdef", "binding_id": "binding-7", "address": "http://127.0.0.1:41234", "contract_digest": "sha256:` + strings.Repeat("a", 64) + `", "readonly": true}`},
		{LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "shutdown", "session_id": "rss-1", "binding_id": "binding-7"}`},
		{LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "error", "code": "session_bootstrap_invalid", "message": "The supervised session bootstrap frame was rejected"}`},
		{LocalSessionBootstrapV1Alpha1, `{
			"schema_version": "pinax.local_session.bootstrap.v1alpha1",
			"session_nonce": "nonce-0123456789abcdef",
			"bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"binding_id": "binding-7",
			"vault_root": "/Users/yeisme/Notes",
			"scopes": ["reader.metadata", "reader.body", "reader.relations", "reader.assets"],
			"readonly": true
		}`},
		// pinax-human-reading-write-v1 W02: writer scopes enter the same
		// allowlist slot; a writer session declares readonly=false. The
		// readonly/writer correlation itself is enforced by the bootstrap
		// reader in code, not by the schema.
		{LocalSessionBootstrapV1Alpha1, `{
			"schema_version": "pinax.local_session.bootstrap.v1alpha1",
			"session_nonce": "nonce-0123456789abcdef",
			"bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"binding_id": "binding-writer",
			"vault_root": "/Users/yeisme/Notes",
			"scopes": ["reader.body", "writer.capture"],
			"readonly": false
		}`},
		{LocalSessionBootstrapV1Alpha1, `{
			"schema_version": "pinax.local_session.bootstrap.v1alpha1",
			"session_nonce": "nonce-0123456789abcdef",
			"bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"binding_id": "binding-editor",
			"vault_root": "/Users/yeisme/Notes",
			"scopes": ["writer.editor", "writer.organizer"],
			"readonly": false
		}`},
	}
	for _, fixture := range positive {
		if err := validateFixture(registry, fixture.schemaID, fixture.document); err != nil {
			t.Errorf("positive fixture for %s rejected: %v", fixture.schemaID, err)
		}
	}

	negative := []struct {
		name     string
		schemaID string
		document string
	}{
		{"source missing session_id", ReaderSourceV1Alpha1, `{"schema_version": "pinax.reader.source.v1alpha1", "binding_id": "b", "display_name": "d", "capabilities": [], "limits": {}, "index_state": {"status": "ready"}, "observed_at": "2026-10-01T12:00:00Z"}`},
		{"source wrong schema_version const", ReaderSourceV1Alpha1, `{"schema_version": "pinax.reader.source.v2", "session_id": "s", "binding_id": "b", "display_name": "d", "capabilities": [], "limits": {}, "index_state": {"status": "ready"}, "observed_at": "2026-10-01T12:00:00Z"}`},
		{"source budget drift rejected", ReaderSourceV1Alpha1, `{"schema_version": "pinax.reader.source.v1alpha1", "session_id": "s", "binding_id": "b", "display_name": "d", "capabilities": [], "limits": {"list_default_limit": 50, "list_max_limit": 500, "snippet_max_chars": 320, "outline_page_max_entries": 512, "document_page_max_text_bytes": 262144, "document_page_max_nodes": 512, "document_response_max_bytes": 1048576, "document_source_max_bytes": 8388608, "document_total_max_nodes": 65536, "asset_range_max_bytes": 8388608, "source_export_range_max_bytes": 1048576}, "index_state": {"status": "ready"}, "observed_at": "2026-10-01T12:00:00Z"}`},
		{"source unknown index status", ReaderSourceV1Alpha1, `{"schema_version": "pinax.reader.source.v1alpha1", "session_id": "s", "binding_id": "b", "display_name": "d", "capabilities": [], "limits": {}, "index_state": {"status": "missing"}, "observed_at": "2026-10-01T12:00:00Z"}`},
		{"note card missing revision", ReaderNoteCardV1Alpha1, `{"object_id": "o", "title": "t", "uri": "u", "status": "active", "tags": [], "updated_at": "2026-10-01T12:00:00Z"}`},
		{"note card closed object", ReaderNoteCardV1Alpha1, `{"object_id": "o", "title": "t", "revision": "r", "uri": "u", "status": "active", "tags": [], "updated_at": "2026-10-01T12:00:00Z", "absolute_path": "/etc/passwd"}`},
		{"snippet over budget", ReaderSearchSnippetV1Alpha1, fmt.Sprintf(`{"object_id": "o", "snippet": "%s"}`, strings.Repeat("字", 321))},
		{"node unknown type", ReaderDocumentNodeV1Alpha1, `{"type": "iframe", "anchor": "b-1"}`},
		{"node fence closed object", ReaderDocumentNodeV1Alpha1, `{"type": "fence", "anchor": "f-1", "executable": true}`},
		{"node missing anchor", ReaderDocumentNodeV1Alpha1, `{"type": "paragraph"}`},
		{"node continuation bad original_type", ReaderDocumentNodeV1Alpha1, `{"type": "continuation", "anchor": "b-7-f1", "original_type": "image"}`},
		{"document missing complete", ReaderDocumentV1Alpha1, `{"schema_version": "pinax.reader.document.v1alpha1", "object_id": "o", "revision": "r", "current_revision": "r2", "uri": "u", "nodes": [], "outline": [], "content_bytes": 1, "observed_at": "2026-10-01T12:00:00Z"}`},
		{"document closed object", ReaderDocumentV1Alpha1, `{"schema_version": "pinax.reader.document.v1alpha1", "object_id": "o", "revision": "r", "current_revision": "r2", "uri": "u", "nodes": [], "outline": [], "complete": true, "content_bytes": 1, "observed_at": "2026-10-01T12:00:00Z", "raw_markdown": "# no"}`},
		{"link page bad direction", ReaderLinkPageV1Alpha1, `{"object_id": "o", "revision": "r", "direction": "both", "items": [], "relations_watermark": "w", "observed_at": "2026-10-01T12:00:00Z"}`},
		{"asset negative size", ReaderAssetRefV1Alpha1, `{"asset_ref": "a", "mime": "image/png", "size_bytes": -1, "digest": "d", "availability": "available"}`},
		{"asset host path rejected", ReaderAssetRefV1Alpha1, `{"asset_ref": "a", "mime": "image/png", "size_bytes": 1, "digest": "d", "availability": "available", "path": "/Users/x/secret.png"}`},
		{"source export wrong media type", ReaderSourceExportV1Alpha1, `{"object_id": "o", "revision": "r", "media_type": "application/pdf", "size_bytes": 1, "digest": "d"}`},
		{"source request closed object", ReaderSourceRequestV1Alpha1, `{"audience": "human"}`},
		{"notes limit zero", ReaderNotesRequestV1Alpha1, `{"limit": 0}`},
		{"notes limit over max", ReaderNotesRequestV1Alpha1, `{"limit": 201}`},
		{"notes unknown sort", ReaderNotesRequestV1Alpha1, `{"sort": "title_desc"}`},
		{"notes cursor too long", ReaderNotesRequestV1Alpha1, fmt.Sprintf(`{"cursor": "%s"}`, strings.Repeat("c", 2049))},
		{"search empty q", ReaderSearchRequestV1Alpha1, `{"q": ""}`},
		{"search q over max", ReaderSearchRequestV1Alpha1, fmt.Sprintf(`{"q": "%s"}`, strings.Repeat("q", 513))},
		{"search relevance only", ReaderSearchRequestV1Alpha1, `{"q": "x", "sort": "updated_desc"}`},
		{"document missing object_id", ReaderDocumentRequestV1Alpha1, `{"revision": "r"}`},
		{"links missing revision", ReaderLinksRequestV1Alpha1, `{"object_id": "o"}`},
		{"asset content missing note binding", ReaderAssetContentRequestV1Alpha1, `{"asset_ref": "a", "note_id": "o"}`},
		{"export missing revision", ReaderSourceExportRequestV1Alpha1, `{"object_id": "o"}`},
		{"bootstrap unknown scope", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "binding_id": "b", "vault_root": "/v", "scopes": ["reader.write"], "readonly": true}`},
		{"bootstrap empty scopes", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "binding_id": "b", "vault_root": "/v", "scopes": [], "readonly": true}`},
		{"bootstrap duplicate scopes", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "binding_id": "b", "vault_root": "/v", "scopes": ["reader.body", "reader.body"], "readonly": true}`},
		{"bootstrap readonly must stay boolean", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "binding_id": "b", "vault_root": "/v", "scopes": ["reader.body"], "readonly": "false"}`},
		{"bootstrap short bearer", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "short", "binding_id": "b", "vault_root": "/v", "scopes": ["reader.body"], "readonly": true}`},
		{"event unknown type", LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "log"}`},
		{"event wrong schema version", LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v2", "type": "ready"}`},
		{"event missing type", LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1"}`},
		{"event closed object", LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "ready", "bearer": "3q2-7AAAA"}`},
		{"event bad digest", LocalSessionEventV1Alpha1, `{"schema_version": "pinax.local_session.event.v1alpha1", "type": "ready", "contract_digest": "md5:zz"}`},
		{"bootstrap extra listener field", LocalSessionBootstrapV1Alpha1, `{"schema_version": "pinax.local_session.bootstrap.v1alpha1", "session_nonce": "nonce-0123456789abcdef", "bearer": "3q2-7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "binding_id": "b", "vault_root": "/v", "scopes": ["reader.body"], "readonly": true, "listen_host": "0.0.0.0"}`},
	}
	for _, fixture := range negative {
		if err := validateFixture(registry, fixture.schemaID, fixture.document); err == nil {
			t.Errorf("negative fixture %q for %s unexpectedly passed", fixture.name, fixture.schemaID)
		}
	}
}

func validateFixture(registry *Registry, schemaID, document string) error {
	var value any
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		return fmt.Errorf("fixture is not JSON: %w", err)
	}
	return ValidatePayload(registry, schemaID, value)
}
