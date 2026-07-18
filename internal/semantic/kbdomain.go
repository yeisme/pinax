package semantic

import (
	"context"

	"github.com/yeisme/lance"
)

// Compile-time guarantee that KBDomain satisfies lance.Domain.
var _ lance.Domain = KBDomain{}

// KBDomain is the pinax knowledge-base adapter for the shared lance vector +
// RAG platform. It knows the table name, the redaction policy for note
// metadata, and the permission rules (filter by note status and kind).
//
// The metadata field names mirror pinax's existing sidecarChunk schema
// (chunk_id, note_id, vault_path, heading_path, status, kind, ...).
type KBDomain struct{}

// Name returns the domain identifier "kb".
func (KBDomain) Name() string { return "kb" }

// TableName returns the LanceDB table name for knowledge-base chunks.
func (KBDomain) TableName() string { return "note_chunks" }

// redactedFields are metadata keys that must never leave the domain boundary:
// full note bodies, raw prompts, provider payloads, OCR text, and secrets.
var redactedFields = []string{
	"note_body",
	"full_text",
	"raw_prompt",
	"provider_payload",
	"authorization",
	"token",
	"secret",
	"ocr_text",
}

// Redact strips sensitive fields from a record's metadata before it leaves the
// knowledge-base domain. The safe-to-surface keys (chunk_id, note_id,
// vault_path, heading_path, title, preview, kind, status, tags, content_hash,
// chunk_hash, token_count) pass through untouched.
func (KBDomain) Redact(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	out := make(map[string]any, len(record))
	for k, v := range record {
		out[k] = v
	}
	for _, field := range redactedFields {
		delete(out, field)
	}
	return out
}

// ResolvePermission computes the set of allowed record IDs given an opaque
// filter. The filter may carry:
//
//   - note_status: []string e.g. ["active"] — only records whose
//     metadata["status"] is in this list are allowed.
//   - note_kind:   []string e.g. ["note","journal"] — only records whose
//     metadata["kind"] is in this list are allowed.
//
// A missing or empty filter dimension matches everything (no restriction).
// Records with no metadata pass unless a filter dimension is set.
func (KBDomain) ResolvePermission(_ context.Context, records []lance.Record, filter lance.PermissionFilter) []string {
	statusSet := toStringSet(filter["note_status"])
	kindSet := toStringSet(filter["note_kind"])

	allowed := make([]string, 0, len(records))
	for _, rec := range records {
		status, _ := rec.Metadata["status"].(string)
		kind, _ := rec.Metadata["kind"].(string)
		if statusSet != nil && !statusSet[status] {
			continue
		}
		if kindSet != nil && !kindSet[kind] {
			continue
		}
		allowed = append(allowed, rec.ID)
	}
	return allowed
}

// toStringSet normalises an opaque filter value into a set lookup. A nil slice
// (or non-slice value) means "no restriction on this dimension".
func toStringSet(v any) map[string]bool {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		if len(s) == 0 {
			return nil
		}
		set := make(map[string]bool, len(s))
		for _, item := range s {
			set[item] = true
		}
		return set
	case []any:
		if len(s) == 0 {
			return nil
		}
		set := make(map[string]bool, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				set[str] = true
			}
		}
		return set
	default:
		return nil
	}
}
