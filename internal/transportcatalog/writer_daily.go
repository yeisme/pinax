package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

const (
	WriterDailyProposeCapabilityID  = "writer.daily.propose"
	WriterDailyApplyCapabilityID    = "writer.daily.apply"
	RestWriterDailyProposeBindingID = "rest.writer.daily.propose"
	RestWriterDailyApplyBindingID   = "rest.writer.daily.apply"
)

func writerDailyCapabilities() []CapabilityDefinition {
	base := CapabilityDefinition{Readonly: true, BodyAllowed: true, UIGroup: "human.writing", BodyExposureDefault: "explicit", WriteGate: "readonly", Stability: StabilityExperimental,
		Errors: []string{"insufficient_scope", "writer_disabled", "writer_request_invalid", "method_not_allowed", "writer_unavailable", "token_required", "invalid_token", "loopback_required", "source_changed", "reader_request_invalid", "reader_unsupported", "reader_unavailable", "daily_exists", "template_unsupported", "body_too_large", "preview_mismatch", "unsafe_path", "path_unavailable", "authorization_required", "operation_identity_required", "idempotency_conflict", "operation_store_unavailable", "outcome_unknown", "reconcile_required", "lock_held"}}
	propose := base
	propose.ID = WriterDailyProposeCapabilityID
	propose.Command = propose.ID
	propose.RequestSchema = catalogschema.WriterDailyProposeRequestV1Alpha1
	propose.ResponseSchema = catalogschema.WriterDailyProposalV1Alpha1
	apply := base
	apply.ID = WriterDailyApplyCapabilityID
	apply.Command = apply.ID
	apply.Readonly = false
	apply.ApprovalRequired = true
	apply.SnapshotRequired = true
	apply.WriteGate = "approval_and_snapshot"
	apply.RequestSchema = catalogschema.WriterDailyApplyRequestV1Alpha1
	apply.ResponseSchema = catalogschema.WriterApplyReceiptV1Alpha1
	return []CapabilityDefinition{propose, apply}
}

func writerDailyBindings() []TransportBinding {
	bindings := make([]TransportBinding, 0, 2)
	for _, capability := range writerDailyCapabilities() {
		path := "/v1/writer/journals/daily:propose"
		if capability.ID == WriterDailyApplyCapabilityID {
			path = "/v1/writer/journals/daily:apply"
		}
		id := "rest." + capability.ID
		bindings = append(bindings, TransportBinding{ID: id, CapabilityID: capability.ID, Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:" + id, Method: "POST", Path: path, Readonly: capability.Readonly, WriteGate: capability.WriteGate, RequestSchema: capability.RequestSchema, ResponseSchema: capability.ResponseSchema, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestWriterDaily", "test:TestWriterDailyHTTP", "test:TestWriterDailyClient"}}})
	}
	return bindings
}
