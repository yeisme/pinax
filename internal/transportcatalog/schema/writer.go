package schema

// Writer-facing machine contracts frozen by pinax-human-reading-write-v1 W01.
// The design audit in that change is the only input: capture, propose/preview/
// apply, and organize are projected onto the human session under owner-granted
// writer scopes; structure writes, operation.show/reconcile and sync stay on
// their existing contracts. All objects are closed; unknown fields are
// rejected. Writer bindings must stay planned until W02-W04 register real
// handlers and W06 flips availability.
const (
	WriterChangeV1Alpha1               = "pinax.writer.change.v1alpha1"
	WriterProposalPreviewV1Alpha1      = "pinax.writer.proposal_preview.v1alpha1"
	WriterApplyReceiptV1Alpha1         = "pinax.writer.apply_receipt.v1alpha1"
	WriterCaptureReceiptV1Alpha1       = "pinax.writer.capture_receipt.v1alpha1"
	WriterOrganizePlanV1Alpha1         = "pinax.writer.organize_plan.v1alpha1"
	WriterOrganizeApplyReceiptV1Alpha1 = "pinax.writer.organize_apply_receipt.v1alpha1"

	WriterCaptureRequestV1Alpha1       = "pinax.writer.capture.request.v1alpha1"
	WriterProposeRequestV1Alpha1       = "pinax.writer.propose.request.v1alpha1"
	WriterApplyRequestV1Alpha1         = "pinax.writer.apply.request.v1alpha1"
	WriterOrganizePlanRequestV1Alpha1  = "pinax.writer.organize_plan.request.v1alpha1"
	WriterOrganizeApplyRequestV1Alpha1 = "pinax.writer.organize_apply.request.v1alpha1"
)

// Writer scope vocabulary. Scopes are granted only by the owner into the
// supervised session bootstrap; they are disjoint from reader scopes and from
// legacy read/write/admin token scopes. An audience or request field never
// upgrades a reader session or a legacy token to a writer.
const (
	WriterScopeCapture   = "writer.capture"
	WriterScopeEditor    = "writer.editor"
	WriterScopeOrganizer = "writer.organizer"
)

// Budget constants from the supervised write contract. They mirror the frozen
// collaboration/inbox budgets so the human projection never widens them; any
// drift fails contract fixtures and the OpenAPI digest.
const (
	WriterMaxBodyBytes           = 64 * 1024
	WriterTitleMaxBytes          = 512
	WriterRefMaxLength           = 512
	WriterOperationIDMaxLength   = 128
	WriterPreviewDigestMaxLength = 80
	WriterRevisionMaxLength      = 80
	WriterTagsMaxItems           = 32
	WriterTagMaxLength           = 128
	WriterSlugMaxLength          = 128
	WriterPlanIDMaxLength        = 128
	WriterSnapshotMessageMaxLen  = 512
	WriterPlanNoteRefsMaxItems   = 100
	WriterPlanNoteRefMaxLength   = 512
)

// writerChangeValue mirrors app.CollaborationChange with frozen budgets: the
// human editor proposal is the same change object the collaboration surface
// validates (action enum, bounded title/body/tags, expected_revision), never
// a wider HTTP form.
func writerChangeValue() map[string]any {
	return closedObject(map[string]any{
		"operation_id":      map[string]any{"type": "string", "maxLength": WriterOperationIDMaxLength},
		"action":            enumValue("create", "append", "replace", "tags", "archive"),
		"note_ref":          map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
		"expected_revision": map[string]any{"type": "string", "maxLength": WriterRevisionMaxLength},
		"title":             map[string]any{"type": "string", "maxLength": WriterTitleMaxBytes},
		"body":              map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes, "description": "Valid UTF-8 note text; NUL bytes are rejected."},
		"dir":               map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
		"target_path":       map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
		"tags":              map[string]any{"type": "array", "maxItems": WriterTagsMaxItems, "items": map[string]any{"type": "string", "maxLength": WriterTagMaxLength}},
		"tag_operation":     enumValue("add", "remove", "set"),
		"status":            enumValue("draft", "inbox"),
	}, "action")
}

// writerReceiptStatusValue freezes the operation receipt status vocabulary the
// recovery surface (operation.show/reconcile) already uses.
func writerReceiptStatusValue() map[string]any {
	return enumValue("accepted", "applying", "succeeded", "failed", "reconcile_required")
}

// writerDigestValue is the preview/operation digest shape: sha256 hex.
func writerDigestValue() map[string]any {
	return map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$", "maxLength": WriterPreviewDigestMaxLength}
}

// writerDefinitions returns the machine contracts for the planned supervised
// write surface. Every write path carries an operation receipt and forbids
// blind retries under a fresh identity.
func writerDefinitions() []Definition {
	return []Definition{
		define(WriterChangeV1Alpha1, writerChangeValue()),
		define(WriterProposalPreviewV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(WriterProposalPreviewV1Alpha1),
			"change":         refValue(WriterChangeV1Alpha1),
			"preview_digest": writerDigestValue(),
			"path":           map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
			"before":         map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes},
			"after":          map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes},
			"state":          stringValue(),
		}, "schema_version", "change", "preview_digest", "path", "before", "after", "state")),
		define(WriterApplyReceiptV1Alpha1, closedObject(map[string]any{
			"schema_version":     constString(WriterApplyReceiptV1Alpha1),
			"operation_id":       map[string]any{"type": "string", "minLength": 1, "maxLength": WriterOperationIDMaxLength},
			"capability_id":      stringValue(),
			"binding_id":         stringValue(),
			"request_digest":     writerDigestValue(),
			"status":             writerReceiptStatusValue(),
			"retryable":          boolValue(),
			"replay_safe":        boolValue(),
			"reconcile_required": boolValue(),
			"required_action":    stringValue(),
			"result":             map[string]any{"description": "Bounded apply result (path, note_id, authorization source)."},
			"receipt":            map[string]any{"description": "Bounded recovery receipt (snapshot, record_event_id)."},
			"receipt_ref":        stringValue(),
			"resource_ref":       stringValue(),
			"revision_before":    map[string]any{"type": "string", "maxLength": WriterRevisionMaxLength},
			"revision_after":     map[string]any{"type": "string", "maxLength": WriterRevisionMaxLength},
			"error":              refValue(ErrorV1),
			"created_at":         dateTimeValue(),
			"updated_at":         dateTimeValue(),
			"accepted_at":        dateTimeValue(),
			"applying_at":        dateTimeValue(),
			"completed_at":       dateTimeValue(),
		}, "schema_version", "operation_id", "status", "retryable", "replay_safe", "reconcile_required", "created_at", "updated_at", "accepted_at")),
		define(WriterCaptureReceiptV1Alpha1, closedObject(map[string]any{
			"schema_version":     constString(WriterCaptureReceiptV1Alpha1),
			"operation_id":       map[string]any{"type": "string", "minLength": 1, "maxLength": WriterOperationIDMaxLength},
			"capability_id":      stringValue(),
			"binding_id":         stringValue(),
			"status":             writerReceiptStatusValue(),
			"retryable":          boolValue(),
			"replay_safe":        boolValue(),
			"reconcile_required": boolValue(),
			"note_id":            stringValue(),
			"path":               map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
			"record_event_id":    stringValue(),
			"revision_after":     map[string]any{"type": "string", "maxLength": WriterRevisionMaxLength},
			"resource_ref":       stringValue(),
			"error":              refValue(ErrorV1),
			"created_at":         dateTimeValue(),
			"updated_at":         dateTimeValue(),
			"accepted_at":        dateTimeValue(),
		}, "schema_version", "operation_id", "status", "retryable", "replay_safe", "reconcile_required", "created_at", "updated_at", "accepted_at")),
		define(WriterOrganizePlanV1Alpha1, closedObject(map[string]any{
			"schema_version":  constString(WriterOrganizePlanV1Alpha1),
			"plan_id":         map[string]any{"type": "string", "minLength": 1, "maxLength": WriterPlanIDMaxLength},
			"status":          stringValue(),
			"created_at":      dateTimeValue(),
			"expires_at":      dateTimeValue(),
			"operation_count": map[string]any{"type": "integer", "minimum": 0},
			"operations": arrayValue(closedObject(map[string]any{
				"operation_id": map[string]any{"type": "string", "maxLength": WriterOperationIDMaxLength},
				"object_id":    stringValue(),
				"kind":         stringValue(),
				"mode":         stringValue(),
				"risk":         stringValue(),
				"path":         map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
			}, "operation_id", "kind")),
		}, "schema_version", "plan_id", "status", "created_at", "expires_at", "operation_count", "operations")),
		define(WriterOrganizeApplyReceiptV1Alpha1, closedObject(map[string]any{
			"schema_version":     constString(WriterOrganizeApplyReceiptV1Alpha1),
			"operation_id":       map[string]any{"type": "string", "minLength": 1, "maxLength": WriterOperationIDMaxLength},
			"plan_id":            map[string]any{"type": "string", "minLength": 1, "maxLength": WriterPlanIDMaxLength},
			"capability_id":      stringValue(),
			"binding_id":         stringValue(),
			"status":             writerReceiptStatusValue(),
			"retryable":          boolValue(),
			"replay_safe":        boolValue(),
			"reconcile_required": boolValue(),
			"snapshot_id":        stringValue(),
			"resource_ref":       stringValue(),
			"error":              refValue(ErrorV1),
			"created_at":         dateTimeValue(),
			"updated_at":         dateTimeValue(),
			"accepted_at":        dateTimeValue(),
		}, "schema_version", "operation_id", "plan_id", "status", "retryable", "replay_safe", "reconcile_required", "created_at", "updated_at", "accepted_at")),
		request(WriterCaptureRequestV1Alpha1, map[string]any{
			"title": map[string]any{"type": "string", "minLength": 1, "maxLength": WriterTitleMaxBytes, "description": "Single line; CR/LF/NUL are rejected."},
			"body":  map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes, "description": "Valid UTF-8 capture body; NUL bytes are rejected."},
			"tags":  map[string]any{"type": "array", "maxItems": WriterTagsMaxItems, "items": map[string]any{"type": "string", "maxLength": WriterTagMaxLength}},
			"slug":  map[string]any{"type": "string", "maxLength": WriterSlugMaxLength},
			"dir":   map[string]any{"type": "string", "maxLength": WriterRefMaxLength},
		}, "title"),
		request(WriterProposeRequestV1Alpha1, map[string]any{
			"change": refValue(WriterChangeV1Alpha1),
		}, "change"),
		request(WriterApplyRequestV1Alpha1, map[string]any{
			"change":         refValue(WriterChangeV1Alpha1),
			"preview_digest": writerDigestValue(),
			"authorization":  enumValue("explicit_instruction", "reviewed_proposal"),
		}, "change", "preview_digest", "authorization"),
		request(WriterOrganizePlanRequestV1Alpha1, map[string]any{
			"note_refs": map[string]any{"type": "array", "minItems": 1, "maxItems": WriterPlanNoteRefsMaxItems, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": WriterPlanNoteRefMaxLength}, "description": "Scoped plan when present; whole-vault plan when omitted."},
		}),
		request(WriterOrganizeApplyRequestV1Alpha1, map[string]any{
			"plan_id":          map[string]any{"type": "string", "minLength": 1, "maxLength": WriterPlanIDMaxLength, "description": "Applied as-is; the plan id is never rebased by the request."},
			"snapshot_message": map[string]any{"type": "string", "maxLength": WriterSnapshotMessageMaxLen},
			"allow_stale":      boolValue(),
		}, "plan_id"),
	}
}
