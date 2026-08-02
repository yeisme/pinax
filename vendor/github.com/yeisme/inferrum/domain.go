package inferrum

import "context"

// PermissionFilter is the opaque permission filter handed to a Domain so it can
// compute the set of allowed record IDs for a search. The concrete shape is
// domain-defined (e.g. eikona passes {allowed_item_ids, permission_allow};
// pinax passes {note_status, note_kind}). Inferrum never interprets it.
type PermissionFilter = map[string]any

// Domain is the per-CLI domain adapter interface. Each CLI (eikona, pinax,
// auctra) registers a ~50-80 line implementation that knows its own permission
// rules, table name, and redaction policy. Inferrum is otherwise domain agnostic.
type Domain interface {
	// Name returns the domain identifier: "visual" | "kb" | "story".
	Name() string
	// TableName returns the LanceDB table name for this domain.
	TableName() string
	// Redact strips sensitive fields from a record's metadata before it leaves
	// the domain boundary: image bytes, raw prompts, OCR, provider payloads,
	// full note bodies. The returned map is what gets stored / surfaced.
	Redact(record map[string]any) map[string]any
	// ResolvePermission computes the allowed record IDs from a set of candidate
	// records given an opaque filter. This is the permission gate that runs
	// before the sidecar search — the sidecar never does permission filtering.
	ResolvePermission(ctx context.Context, records []Record, filter PermissionFilter) []string
}
