package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

func writerTagsCapabilities() []CapabilityDefinition {
	base := CapabilityDefinition{Readonly: true, BodyAllowed: true, UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "readonly", Stability: StabilityExperimental,
		Errors: []string{"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable", "token_required", "invalid_token", "loopback_required", "source_changed", "reader_unsupported", "reader_unavailable", "revision_conflict", "note_not_found", "note_identity_ambiguous", "frontmatter_unsupported", "body_invalid", "body_too_large", "invalid_tag", "unsafe_path", "authorization_required", "operation_identity_required", "idempotency_conflict", "operation_not_found", "operation_store_unavailable", "outcome_unknown", "reconcile_required", "lock_held", "preview_mismatch"}}
	plan := base
	plan.ID = "writer.tags.plan"
	plan.Command = plan.ID
	plan.RequestSchema = catalogschema.WriterTagsPlanRequestV1Alpha1
	plan.ResponseSchema = catalogschema.WriterTagsPlanV1Alpha1
	apply := base
	apply.ID = "writer.tags.apply"
	apply.Command = apply.ID
	apply.Readonly = false
	apply.ApprovalRequired = true
	apply.SnapshotRequired = true
	apply.WriteGate = "approval_and_snapshot"
	apply.RequestSchema = catalogschema.WriterTagsApplyRequestV1Alpha1
	apply.ResponseSchema = catalogschema.WriterTagsReceiptV1Alpha1
	lookup := base
	lookup.ID = "writer.tags.lookup"
	lookup.Command = lookup.ID
	lookup.RequestSchema = catalogschema.WriterTagsLookupRequestV1Alpha1
	lookup.ResponseSchema = catalogschema.WriterTagsReceiptV1Alpha1
	return []CapabilityDefinition{plan, apply, lookup}
}

func writerTagsBindings() []TransportBinding {
	paths := []string{"/v1/writer/tags:plan", "/v1/writer/tags:apply", "/v1/writer/tags:lookup"}
	out := []TransportBinding{}
	for i, c := range writerTagsCapabilities() {
		id := "rest." + c.ID
		out = append(out, TransportBinding{ID: id, CapabilityID: c.ID, Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:" + id, Method: "POST", Path: paths[i], Readonly: c.Readonly, WriteGate: c.WriteGate, RequestSchema: c.RequestSchema, ResponseSchema: c.ResponseSchema, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestWriterTagsHTTP"}}})
	}
	return out
}
