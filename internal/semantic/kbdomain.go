package semantic

import (
	"context"
	"path/filepath"
	"strings"

	inferrum "github.com/yeisme/inferrum"
)

type PermissionFilter = map[string]any

type Record = inferrum.Record

var _ inferrum.Domain = KBDomain{}

// KBDomain is the Pinax knowledge-base adapter for the shared Inferrum vector +
// RAG platform. It knows the table name, the redaction policy for note
// metadata, and the permission rules (filter by note status and kind).
//
// The metadata field names are Pinax-owned safe citation fields. The Inferrum
// sidecar treats them as opaque JSON and never interprets them.
type KBDomain struct{}

// Name returns the domain identifier "kb".
func (KBDomain) Name() string { return "kb" }

// TableName returns the LanceDB table name for knowledge-base chunks.
func (KBDomain) TableName() string { return "note_chunks" }

var safeMetadataFields = map[string]struct{}{
	"chunk_id": {}, "note_id": {}, "source_ref": {}, "title": {}, "heading_path": {},
	"page": {}, "span": {}, "preview": {}, "content_hash": {}, "chunk_hash": {},
	"token_count": {}, "tags": {}, "kind": {}, "status": {}, "source_type": {},
	"source_version": {}, "source_digest": {}, "provider": {}, "model": {},
}

// Redact projects only the known safe citation metadata. A legacy vault_path
// input is converted to a vault-relative source_ref; unknown fields are
// dropped instead of relying on a denylist that would allow future fields.
func (KBDomain) Redact(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	out := make(map[string]any, len(record))
	for key, value := range record {
		if key == "vault_path" {
			if sourceRef := safeSourceRef(value); sourceRef != "" {
				out["source_ref"] = sourceRef
			}
			continue
		}
		if key == "source_ref" {
			if sourceRef := safeSourceRef(value); sourceRef != "" {
				out[key] = sourceRef
			}
			continue
		}
		if _, ok := safeMetadataFields[key]; ok {
			out[key] = value
		}
	}
	return out
}

func safeSourceRef(value any) string {
	raw, ok := value.(string)
	if !ok {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || filepath.IsAbs(raw) {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(raw))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, ":") {
		return ""
	}
	return cleaned
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
func (KBDomain) ResolvePermission(_ context.Context, records []Record, filter PermissionFilter) []string {
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
