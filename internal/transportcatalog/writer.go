package transportcatalog

// Writer capability and binding IDs for the planned supervised write surface
// (pinax-human-reading-write-v1). The scope enumeration, propose/apply
// schemas and error table are frozen by W01; the design audit is the only
// scope source: capture, edit proposals and organize are projected under
// owner-granted writer scopes. Every binding stays planned with the
// implementation-pending blocker until W02-W04 register real handlers;
// flipping one to available without a backing_ref fails Compile. Structure
// writes, operation.show/reconcile and sync keep their existing contracts and
// are never duplicated here.
const (
	WriterEditorSourceCapabilityID  = "writer.editor_source"
	WriterCaptureCapabilityID       = "writer.capture"
	WriterProposeCapabilityID       = "writer.propose"
	WriterApplyCapabilityID         = "writer.apply"
	WriterOrganizePlanCapabilityID  = "writer.organize.plan"
	WriterOrganizeApplyCapabilityID = "writer.organize.apply"

	RestWriterEditorSourceBindingID  = "rest.writer.editor_source"
	RestWriterCaptureBindingID       = "rest.writer.capture"
	RestWriterProposeBindingID       = "rest.writer.propose"
	RestWriterApplyBindingID         = "rest.writer.apply"
	RestWriterOrganizePlanBindingID  = "rest.writer.organize.plan"
	RestWriterOrganizeApplyBindingID = "rest.writer.organize.apply"

	// WriterImplementationPendingBlocker marks every planned writer binding:
	// W02-W04 implement the handlers, W06 registers real backings and flips
	// availability.
	WriterImplementationPendingBlocker = "implementation_pending_pinax_human_reading_write_v1"
)

// WriterCapabilityDefinitions returns the additive supervised write
// capabilities. Writes are experimental, owner-authorized via writer scopes,
// and follow the frozen gates: capture requires approval plus operation
// identity/idempotency; apply requires the unchanged preview digest and a
// caller-asserted authorization (never a verified approval) and snapshots
// before mutating existing notes; organize apply is approval_and_snapshot.
func WriterCapabilityDefinitions() []CapabilityDefinition {
	return append(append(append(writerTagsCapabilities(), writerDailyEditCapabilities()...), writerDailyCapabilities()...), []CapabilityDefinition{
		{
			ID: WriterEditorSourceCapabilityID, Command: WriterEditorSourceCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.writer.editor_source.request.v1alpha1",
			ResponseSchema: "pinax.writer.editor_source.v1alpha1",
			Errors:         []string{"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable", "token_required", "invalid_token", "loopback_required", "source_changed", "revision_conflict", "body_too_large", "body_invalid", "unsafe_path", "note_not_found", "note_identity_ambiguous"},
			Stability:      StabilityExperimental,
		},
		{
			ID: WriterCaptureCapabilityID, Command: WriterCaptureCapabilityID,
			Readonly: false, BodyAllowed: true,
			ApprovalRequired: true,
			UIGroup:          "human.writing", BodyExposureDefault: "explicit", WriteGate: "write",
			RequestSchema:  "pinax.writer.capture.request.v1alpha1",
			ResponseSchema: "pinax.writer.capture_receipt.v1alpha1",
			Errors: []string{
				"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable",
				"token_required", "invalid_token", "loopback_required", "source_changed",
				"approval_required", "write_disabled", "operation_identity_required",
				"idempotency_conflict", "operation_store_unavailable", "reconcile_required",
				"title_invalid", "body_invalid", "note_path_conflict",
			},
			Stability: StabilityExperimental,
		},
		{
			ID: WriterProposeCapabilityID, Command: WriterProposeCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.writer.propose.request.v1alpha1",
			ResponseSchema: "pinax.writer.proposal_preview.v1alpha1",
			Errors: []string{
				"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable",
				"token_required", "invalid_token", "loopback_required", "source_changed",
				"write_disabled", "change_invalid", "body_invalid", "title_invalid",
				"action_unsupported", "revision_conflict", "path_unavailable",
				"unsafe_path", "body_too_large", "note_not_found",
			},
			Stability: StabilityExperimental,
		},
		{
			ID: WriterApplyCapabilityID, Command: WriterApplyCapabilityID,
			Readonly: false, BodyAllowed: true,
			ApprovalRequired: true, SnapshotRequired: true,
			UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "approval_and_snapshot",
			RequestSchema:  "pinax.writer.apply.request.v1alpha1",
			ResponseSchema: "pinax.writer.apply_receipt.v1alpha1",
			Errors: []string{
				"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable",
				"token_required", "invalid_token", "loopback_required", "source_changed",
				"write_disabled", "authorization_required", "preview_mismatch",
				"change_invalid", "revision_conflict", "operation_identity_required",
				"idempotency_conflict", "operation_store_unavailable",
				"outcome_unknown", "reconcile_required",
				"body_invalid", "title_invalid", "note_not_found", "note_path_conflict",
			},
			Stability: StabilityExperimental,
		},
		{
			ID: WriterOrganizePlanCapabilityID, Command: WriterOrganizePlanCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.writer.organize_plan.request.v1alpha1",
			ResponseSchema: "pinax.writer.organize_plan.v1alpha1",
			Errors: []string{
				"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable",
				"token_required", "invalid_token", "loopback_required", "source_changed",
				"vault_not_initialized", "plan_save_failed",
				"organize_selection_invalid", "organize_identity_required",
				"organize_plan_schema_invalid",
			},
			Stability: StabilityExperimental,
		},
		{
			ID: WriterOrganizeApplyCapabilityID, Command: WriterOrganizeApplyCapabilityID,
			Readonly: false, BodyAllowed: true,
			ApprovalRequired: true, SnapshotRequired: true,
			UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "approval_and_snapshot",
			RequestSchema:  "pinax.writer.organize_apply.request.v1alpha1",
			ResponseSchema: "pinax.writer.organize_apply_receipt.v1alpha1",
			Errors: []string{
				"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable",
				"token_required", "invalid_token", "loopback_required", "source_changed",
				"write_disabled", "approval_required", "snapshot_required",
				"plan_not_found", "plan_stale", "vault_not_initialized",
				"organize_selection_invalid", "organize_identity_required",
				"organize_plan_schema_invalid", "organize_plan_not_planned",
				"scoped_plan_override_forbidden", "organize_operation_failed",
				"plan_save_failed",
				"outcome_unknown", "reconcile_required",
			},
			Stability: StabilityExperimental,
		},
	}...)
}

// WriterTransportBindings returns the REST bindings for the supervised
// write surface under the human session, backed by
// real HTTP handlers since W02-W04 and registered available with component
// evidence at W06. Each binding names its HTTP handler backing so manifest
// consumers can trace availability to implementation.
func WriterTransportBindings() []TransportBinding {
	bound := func(id, capabilityID, method, path, requestSchema, responseSchema string, readonly bool, gate string) TransportBinding {
		return TransportBinding{
			ID: id, CapabilityID: capabilityID,
			Transport:    TransportREST,
			Availability: AvailabilityAvailable,
			BackingRef:   "internal/api/http:" + id,
			Method:       method, Path: path,
			Readonly:       readonly,
			WriteGate:      gate,
			RequestSchema:  requestSchema,
			ResponseSchema: responseSchema,
			Readiness: &Readiness{
				Status:   ReadinessReady,
				Maturity: MaturityExploratory,
				EvidenceRefs: []string{
					"test:TestWriterCapture", "test:TestWriterEdit", "test:TestWriterOrganize",
					"test:TestWriterClient", "test:TestWriterComponent",
				},
			},
		}
	}
	editorSource := bound(RestWriterEditorSourceBindingID, WriterEditorSourceCapabilityID, "GET", "/v1/writer/notes/{object_id}/editor-source",
		"pinax.writer.editor_source.request.v1alpha1", "pinax.writer.editor_source.v1alpha1", true, "readonly")
	editorSource.Readiness.EvidenceRefs = []string{"test:TestWriterEditorSource", "test:TestWriterEditorSourceClient"}
	return append(append(append(writerTagsBindings(), writerDailyEditBindings()...), writerDailyBindings()...), []TransportBinding{
		editorSource,
		bound(RestWriterCaptureBindingID, WriterCaptureCapabilityID, "POST", "/v1/writer/capture",
			"pinax.writer.capture.request.v1alpha1", "pinax.writer.capture_receipt.v1alpha1", false, "write"),
		bound(RestWriterProposeBindingID, WriterProposeCapabilityID, "POST", "/v1/writer/proposals",
			"pinax.writer.propose.request.v1alpha1", "pinax.writer.proposal_preview.v1alpha1", true, "readonly"),
		bound(RestWriterApplyBindingID, WriterApplyCapabilityID, "POST", "/v1/writer/proposals:apply",
			"pinax.writer.apply.request.v1alpha1", "pinax.writer.apply_receipt.v1alpha1", false, "approval_and_snapshot"),
		bound(RestWriterOrganizePlanBindingID, WriterOrganizePlanCapabilityID, "POST", "/v1/writer/organize/plan",
			"pinax.writer.organize_plan.request.v1alpha1", "pinax.writer.organize_plan.v1alpha1", true, "readonly"),
		bound(RestWriterOrganizeApplyBindingID, WriterOrganizeApplyCapabilityID, "POST", "/v1/writer/organize/apply",
			"pinax.writer.organize_apply.request.v1alpha1", "pinax.writer.organize_apply_receipt.v1alpha1", false, "approval_and_snapshot"),
	}...)
}

// WriterDeclaredSurfaces declares the REST surface for writer capabilities.
// Declared surfaces are intent only; available_surfaces stay empty until a
// real adapter makes them callable.
func WriterDeclaredSurfaces() map[string][]string {
	declared := make(map[string][]string, 5)
	for _, definition := range WriterCapabilityDefinitions() {
		declared[definition.ID] = []string{"rest"}
	}
	return declared
}

// CompileWriterContract freezes the supervised write catalog slice: writer
// capabilities plus their planned REST bindings. Deterministic and
// digest-stable; used by the W01 drift gate and composed into the full
// transport manifest by internal/transportmanifest.
func CompileWriterContract() (Manifest, error) {
	return Compile(WriterCapabilityDefinitions(), WriterTransportBindings(), WriterDeclaredSurfaces())
}
